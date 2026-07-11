package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type canonicalAIStoredReview struct {
	Decision            string          `json:"decision"`
	Confidence          float64         `json:"confidence"`
	AcceptanceResults   json.RawMessage `json:"acceptance_results"`
	Error               string          `json:"error,omitempty"`
	EvidenceFingerprint string          `json:"evidence_fingerprint"`
}

type canonicalAIAcquisition struct {
	Review    canonicalAIVerificationResult
	ReviewErr error
	Marker    *db.DoctorAiOnDemandTrigger
	Allowed   bool
}

func (s *Scheduler) acquireCanonicalAIReview(ctx context.Context, q *db.Queries, projectID uuid.UUID, fix db.SiteFix, page canonicalPageEvidence, projectConfig config.ProjectConfig, automatic bool) (canonicalAIAcquisition, error) {
	if !projectConfig.AllowsDoctorAI(config.DoctorAITriggerVerificationUser) {
		if err := rejectCanonicalAIMarkers(ctx, q, projectID, fix, page, "Doctor AI policy is disabled"); err != nil {
			return canonicalAIAcquisition{}, err
		}
		return canonicalAIAcquisition{}, nil
	}
	if consumed, err := q.GetDoctorAIOnDemandConsumedResult(ctx, db.GetDoctorAIOnDemandConsumedResultParams{ProjectID: projectID, SiteFixID: fix.ID}); err == nil {
		var stored canonicalAIStoredReview
		if json.Unmarshal(consumed.ResultSnapshot, &stored) != nil {
			return canonicalAIAcquisition{}, errors.New("stored Doctor AI verification result is invalid")
		}
		review := canonicalAIVerificationResult{CallID: uuid.UUID(consumed.AiCallID.Bytes), Decision: stored.Decision, Confidence: stored.Confidence, AcceptanceResults: stored.AcceptanceResults}
		var reviewErr error
		if stored.Error != "" {
			reviewErr = errors.New(stored.Error)
		}
		if stored.EvidenceFingerprint == "" || stored.EvidenceFingerprint != canonicalPageEvidenceFingerprint(page, fix.AcceptanceTests) {
			review.Decision, review.Confidence = "inconclusive", 0
			reviewErr = errors.New("stored Doctor AI result does not match the current PageEvidence")
		}
		return canonicalAIAcquisition{Review: review, ReviewErr: reviewErr, Marker: &consumed, Allowed: true}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return canonicalAIAcquisition{}, err
	}

	if ok, _ := canonicalAIVerificationCapability(fix.AcceptanceTests, page); !ok {
		if err := rejectCanonicalAIMarkers(ctx, q, projectID, fix, page, "acceptance tests require evidence that was not collected"); err != nil {
			return canonicalAIAcquisition{}, err
		}
		return canonicalAIAcquisition{}, nil
	}
	if s.LLM == nil {
		if err := rejectCanonicalAIMarkers(ctx, q, projectID, fix, page, "Doctor AI provider is unavailable"); err != nil {
			return canonicalAIAcquisition{}, err
		}
	}
	reviewer := canonicalAIVerificationReviewer{store: postgresCanonicalAIVerificationStore{q: q}, provider: s.LLM}
	if s.LLM != nil {
		processingToken := uuid.New()
		marker, err := q.ClaimDoctorAIOnDemandProcessing(ctx, db.ClaimDoctorAIOnDemandProcessingParams{
			ProcessingToken: pgtype.UUID{Bytes: processingToken, Valid: true}, LeaseTtlSeconds: 180,
			ProjectID: projectID, SiteFixID: fix.ID,
		})
		if err == nil {
			callID := uuid.Nil
			reclaimedExistingCall := marker.AiCallID.Valid
			if marker.AiCallID.Valid {
				callID = uuid.UUID(marker.AiCallID.Bytes)
				ledger, ledgerErr := q.GetAICallRecord(ctx, db.GetAICallRecordParams{ID: callID, ProjectID: projectID})
				if ledgerErr != nil {
					return canonicalAIAcquisition{}, ledgerErr
				}
				if ledger.Status == "running" {
					errorCode := "processing_reclaimed"
					if _, finishErr := q.FinishAICallRecordIfRunning(ctx, db.FinishAICallRecordIfRunningParams{ErrorCode: &errorCode, ID: callID, ProjectID: projectID}); finishErr != nil && !errors.Is(finishErr, pgx.ErrNoRows) {
						return canonicalAIAcquisition{}, finishErr
					}
				}
			} else {
				fingerprint, _ := json.Marshal(canonicalPageEvidenceMap("", page, nil))
				sum := sha256.Sum256(append(append([]byte{}, fix.AcceptanceTests...), fingerprint...))
				marker, err = q.StartDoctorAIOnDemandCall(ctx, db.StartDoctorAIOnDemandCallParams{
					ProjectID: projectID, SiteFixID: fix.ID, RequestID: marker.RequestID,
					ProcessingToken: pgtype.UUID{Bytes: processingToken, Valid: true}, Provider: "tokengate",
					Model: llm.DefaultTokenGateModel, PromptVersion: "doctor-verification-v1", RequestFingerprint: fmt.Sprintf("%x", sum[:]),
				})
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						if _, rejectErr := q.RejectUnauthorizedDoctorAIOnDemandTriggers(ctx, db.RejectUnauthorizedDoctorAIOnDemandTriggersParams{ProjectID: projectID, SiteFixID: fix.ID}); rejectErr != nil {
							return canonicalAIAcquisition{}, rejectErr
						}
						return canonicalAIAcquisition{}, nil
					}
					return canonicalAIAcquisition{}, err
				}
				callID = uuid.UUID(marker.AiCallID.Bytes)
			}
			review := canonicalAIVerificationResult{CallID: callID, Decision: "inconclusive"}
			var reviewErr error
			if !reclaimedExistingCall {
				review, reviewErr = reviewer.reviewWithCallID(ctx, projectID, fix, page, callID)
			} else {
				reviewErr = errors.New("expired Doctor AI processing was reclaimed without repeating the provider call")
			}
			stored := canonicalAIStoredReview{Decision: review.Decision, Confidence: review.Confidence, AcceptanceResults: review.AcceptanceResults, EvidenceFingerprint: canonicalPageEvidenceFingerprint(page, fix.AcceptanceTests)}
			if reviewErr != nil {
				stored.Error = reviewErr.Error()
			}
			raw, _ := json.Marshal(stored)
			marker, err = q.ConsumeDoctorAIOnDemandProcessing(ctx, db.ConsumeDoctorAIOnDemandProcessingParams{
				ResultSnapshot: raw, ProjectID: projectID, SiteFixID: fix.ID, RequestID: marker.RequestID,
				ProcessingToken: pgtype.UUID{Bytes: processingToken, Valid: true}, AiCallID: pgtype.UUID{Bytes: callID, Valid: true},
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					if _, rejectErr := q.RejectUnauthorizedDoctorAIOnDemandTriggers(ctx, db.RejectUnauthorizedDoctorAIOnDemandTriggersParams{ProjectID: projectID, SiteFixID: fix.ID}); rejectErr != nil {
						return canonicalAIAcquisition{}, rejectErr
					}
					return canonicalAIAcquisition{}, nil
				}
				return canonicalAIAcquisition{}, err
			}
			return canonicalAIAcquisition{Review: review, ReviewErr: reviewErr, Marker: &marker, Allowed: true}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return canonicalAIAcquisition{}, err
		}
		if _, rejectErr := q.RejectUnauthorizedDoctorAIOnDemandTriggers(ctx, db.RejectUnauthorizedDoctorAIOnDemandTriggersParams{ProjectID: projectID, SiteFixID: fix.ID}); rejectErr != nil {
			return canonicalAIAcquisition{}, rejectErr
		}
		if _, activeErr := q.GetDoctorAIOnDemandActive(ctx, db.GetDoctorAIOnDemandActiveParams{ProjectID: projectID, SiteFixID: fix.ID}); activeErr == nil {
			return canonicalAIAcquisition{}, nil
		} else if !errors.Is(activeErr, pgx.ErrNoRows) {
			return canonicalAIAcquisition{}, activeErr
		}
	}
	if automatic && s.LLM != nil {
		review, reviewErr := reviewer.Review(ctx, projectID, fix, page)
		return canonicalAIAcquisition{Review: review, ReviewErr: reviewErr, Allowed: true}, nil
	}
	return canonicalAIAcquisition{}, nil
}

func rejectCanonicalAIMarkers(ctx context.Context, q *db.Queries, projectID uuid.UUID, fix db.SiteFix, page canonicalPageEvidence, reason string) error {
	raw, _ := json.Marshal(map[string]any{"decision": "rejected", "reason": reason, "evidence_fingerprint": canonicalPageEvidenceFingerprint(page, fix.AcceptanceTests)})
	_, err := q.RejectDoctorAIOnDemandTriggersForSiteFix(ctx, db.RejectDoctorAIOnDemandTriggersForSiteFixParams{
		ResultSnapshot: raw, RejectionReason: &reason, ProjectID: projectID, SiteFixID: fix.ID,
	})
	return err
}

func canonicalPageEvidenceFingerprint(page canonicalPageEvidence, tests json.RawMessage) string {
	raw, _ := json.Marshal(canonicalPageEvidenceMap("", page, tests))
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:])
}

func markCanonicalAIReviewAppliedStrict(ctx context.Context, q *db.Queries, marker *db.DoctorAiOnDemandTrigger) error {
	if marker == nil || !marker.AiCallID.Valid {
		return nil
	}
	_, err := q.MarkDoctorAIOnDemandLifecycleApplied(ctx, db.MarkDoctorAIOnDemandLifecycleAppliedParams{
		ProjectID: marker.ProjectID, SiteFixID: marker.SiteFixID, RequestID: marker.RequestID, AiCallID: marker.AiCallID,
	})
	return err
}

func supersedeCanonicalAISiblingMarkers(ctx context.Context, q *db.Queries, projectID, siteFixID uuid.UUID, marker *db.DoctorAiOnDemandTrigger) error {
	if marker == nil {
		var empty canonicalPageEvidence
		return rejectCanonicalAIMarkers(ctx, q, projectID, db.SiteFix{ID: siteFixID}, empty, "verification lifecycle completed without an active AI result")
	}
	reason := "another Doctor AI result completed the verification lifecycle"
	_, err := q.SupersedeDoctorAIOnDemandSiblingTriggers(ctx, db.SupersedeDoctorAIOnDemandSiblingTriggersParams{
		RejectionReason: &reason, ProjectID: projectID, SiteFixID: siteFixID, AppliedRequestID: marker.RequestID,
	})
	return err
}
