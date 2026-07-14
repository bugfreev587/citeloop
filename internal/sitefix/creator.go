package sitefix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrWrongOwner               = errors.New("Site Fix work must be owned by Doctor")
	ErrIncompleteCandidate      = errors.New("Doctor candidate is incomplete")
	ErrHealthyFinding           = errors.New("healthy coverage cannot create a Site Fix")
	ErrProjectMismatch          = errors.New("Site Fix project identity mismatch")
	ErrCandidateFindingMismatch = errors.New("candidate does not identify the Doctor finding")
	ErrActivePredecessor        = errors.New("active Site Fix predecessor must finish before revision")
	ErrInvalidMeasurementPolicy = errors.New("Site Fix measurement policy is invalid")
)

// Creator is invoked only from arbitration Phase B and only with the
// transaction-bound Queries supplied by ReservationStore. It owns no pool or
// semantic provider, so it cannot escape the serializable transaction or call
// AI while locks are held.
type Creator struct{}

func (Creator) CreateInTransaction(ctx context.Context, q *db.Queries, work discovery.ReservedWork) (discovery.WorkReference, error) {
	if work.Owner != discovery.OwnerDoctor {
		return discovery.WorkReference{}, ErrWrongOwner
	}
	if q == nil {
		return discovery.WorkReference{}, errors.New("transaction-bound database queries are required")
	}
	if work.ProjectID == uuid.Nil || work.CandidateID == uuid.Nil || work.DecisionID == uuid.Nil || work.WorkSignatureID == uuid.Nil {
		return discovery.WorkReference{}, ErrIncompleteCandidate
	}
	candidate, err := q.GetDiscoveryCandidateForArbitration(ctx, db.GetDiscoveryCandidateForArbitrationParams{
		ProjectID: work.ProjectID, CandidateID: work.CandidateID,
	})
	if err != nil {
		return discovery.WorkReference{}, fmt.Errorf("load reserved Doctor candidate: %w", err)
	}
	if candidate.ProjectID != work.ProjectID {
		return discovery.WorkReference{}, ErrProjectMismatch
	}
	findingID, err := validateDoctorCandidate(candidate)
	if err != nil {
		return discovery.WorkReference{}, err
	}
	finding, err := q.GetSEODoctorFindingForSiteFixForUpdate(ctx, db.GetSEODoctorFindingForSiteFixForUpdateParams{
		ProjectID: work.ProjectID, FindingID: findingID,
	})
	if err != nil {
		return discovery.WorkReference{}, fmt.Errorf("lock Doctor finding: %w", err)
	}
	if finding.ProjectID != work.ProjectID {
		return discovery.WorkReference{}, ErrProjectMismatch
	}
	if finding.ID != findingID {
		return discovery.WorkReference{}, ErrCandidateFindingMismatch
	}
	if err := validateLockedFindingSnapshot(candidate, finding); err != nil {
		return discovery.WorkReference{}, err
	}
	if finding.FindingKind == "healthy" {
		return discovery.WorkReference{}, ErrHealthyFinding
	}
	if finding.FindingKind != "broken" && finding.FindingKind != "optimization" {
		return discovery.WorkReference{}, ErrIncompleteCandidate
	}
	if finding.Status != "active" {
		return discovery.WorkReference{}, fmt.Errorf("%w: finding status %q", ErrIncompleteCandidate, finding.Status)
	}

	if _, err := q.GetActiveCanonicalSiteFixForFindingForUpdate(ctx, db.GetActiveCanonicalSiteFixForFindingForUpdateParams{
		ProjectID: work.ProjectID, DoctorFindingID: finding.ID,
	}); err == nil {
		return discovery.WorkReference{}, ErrActivePredecessor
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return discovery.WorkReference{}, fmt.Errorf("lock active Site Fix predecessor: %w", err)
	}

	supersedes := pgtype.UUID{}
	predecessor, err := q.GetLatestCanonicalSiteFixForFindingForUpdate(ctx, db.GetLatestCanonicalSiteFixForFindingForUpdateParams{
		ProjectID: work.ProjectID, DoctorFindingID: finding.ID,
	})
	switch {
	case err == nil:
		supersedes = pgtype.UUID{Bytes: predecessor.ID, Valid: true}
	case errors.Is(err, pgx.ErrNoRows):
		// First fix attempt for this finding.
	default:
		return discovery.WorkReference{}, fmt.Errorf("lock Site Fix predecessor: %w", err)
	}

	evidence, proposedFix, targetURLs, acceptanceTests, err := canonicalPayloads(candidate, finding)
	if err != nil {
		return discovery.WorkReference{}, err
	}
	now := time.Now().UTC()
	classification := ClassifySiteFixMeasurement(MeasurementClassificationInput{
		ReferenceTime:    now,
		TargetURLs:       targetURLs,
		FindingIssueType: finding.IssueType,
		TargetSurface:    candidate.ChangeFamily,
		FindingEvidence:  finding.Evidence,
		ProposedFix:      proposedFix,
		AcceptanceTests:  acceptanceTests,
	})
	if classification.ValidationError != "" {
		return discovery.WorkReference{}, fmt.Errorf("%w: %s", ErrInvalidMeasurementPolicy, classification.ValidationError)
	}
	measurementPolicySnapshot := classification.MeasurementPolicySnapshot
	if len(measurementPolicySnapshot) == 0 {
		measurementPolicySnapshot = json.RawMessage(`{}`)
	}
	measurementPlanSnapshot := classification.MeasurementPlanSnapshot
	if len(measurementPlanSnapshot) == 0 {
		measurementPlanSnapshot = json.RawMessage(`{}`)
	}
	row, err := q.CreateCanonicalSiteFix(ctx, db.CreateCanonicalSiteFixParams{
		ID: uuid.New(), ProjectID: work.ProjectID, DoctorFindingID: finding.ID,
		CandidateID: work.CandidateID, WorkSignatureID: work.WorkSignatureID,
		SupersedesSiteFixID: supersedes, Status: "proposed", FindingKind: finding.FindingKind,
		TargetUrls: targetURLs, EvidenceSnapshot: evidence, ProposedFix: proposedFix,
		AcceptanceTests: acceptanceTests, VerificationSnapshot: json.RawMessage(`{}`),
		RetryCount: 0, MaxRetries: 3,
		FixType: classification.FixType, ImpactMode: classification.ImpactMode,
		MeasurementPolicy: classification.MeasurementPolicy, ClassifierVersion: classification.ClassifierVersion,
		DecisionOrigin: classification.DecisionOrigin, DecisionConfidence: classification.DecisionConfidence,
		GrowthHypothesis: classification.GrowthHypothesis, PrimaryMetric: classification.PrimaryMetric,
		SecondaryMetrics: classification.SecondaryMetrics, MeasurementPolicyVersion: classification.MeasurementPolicyVersion,
		MeasurementPolicySnapshot: measurementPolicySnapshot, MeasurementPlanSnapshot: measurementPlanSnapshot,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return discovery.WorkReference{}, fmt.Errorf("create canonical Site Fix: %w", err)
	}
	if row.ID == uuid.Nil || row.ProjectID != work.ProjectID || row.CandidateID != work.CandidateID || row.WorkSignatureID != work.WorkSignatureID {
		return discovery.WorkReference{}, errors.New("created Site Fix returned invalid canonical provenance")
	}
	return discovery.WorkReference{Type: "site_fix", ID: row.ID}, nil
}

func validateDoctorCandidate(candidate db.DiscoveryCandidate) (uuid.UUID, error) {
	if candidate.Status != string(discovery.StatusIdentityReady) ||
		candidate.SourceKind != string(discovery.SourceDoctor) ||
		candidate.SourceObjectType != "seo_doctor_finding" ||
		candidate.SuggestedOwner != string(discovery.OwnerDoctor) ||
		candidate.VerificationMode != string(discovery.VerificationImmediate) ||
		candidate.PrimarySuccessMetric != "acceptance_test_pass" ||
		candidate.CandidateSchemaVersion != discovery.CandidateSchemaVersionV1 ||
		candidate.CandidateVersion < 1 || candidate.ExactSignatureHash == nil || strings.TrimSpace(*candidate.ExactSignatureHash) == "" ||
		len(candidate.SignaturePayload) == 0 || len(candidate.ConflictBucketKeys) == 0 || strings.TrimSpace(candidate.EvidenceFingerprint) == "" {
		return uuid.Nil, ErrIncompleteCandidate
	}
	if candidate.ArtifactIntent != string(discovery.ArtifactRepairExistingSurface) {
		return uuid.Nil, ErrIncompleteCandidate
	}
	var mutations []discovery.Mutation
	var targets []string
	var buckets []string
	var evidenceIDs []string
	var signaturePayload struct {
		SignatureVersion string `json:"signature_version"`
	}
	if json.Unmarshal(candidate.ProposedMutations, &mutations) != nil || len(mutations) == 0 ||
		json.Unmarshal(candidate.NormalizedTargetSet, &targets) != nil || len(targets) == 0 ||
		json.Unmarshal(candidate.ConflictBucketKeys, &buckets) != nil || len(buckets) == 0 ||
		json.Unmarshal(candidate.EvidenceIds, &evidenceIDs) != nil || len(evidenceIDs) == 0 ||
		json.Unmarshal(candidate.SignaturePayload, &signaturePayload) != nil || signaturePayload.SignatureVersion != discovery.SignatureVersionV1 {
		return uuid.Nil, ErrIncompleteCandidate
	}
	for _, target := range targets {
		if strings.TrimSpace(target) == "" {
			return uuid.Nil, ErrIncompleteCandidate
		}
	}
	for _, mutation := range mutations {
		if strings.TrimSpace(mutation.Operation) == "" || strings.TrimSpace(mutation.Field) == "" {
			return uuid.Nil, ErrIncompleteCandidate
		}
	}
	findingID, err := uuid.Parse(strings.TrimSpace(candidate.SourceObjectID))
	if err != nil || findingID == uuid.Nil {
		return uuid.Nil, ErrCandidateFindingMismatch
	}
	return findingID, nil
}

func canonicalPayloads(candidate db.DiscoveryCandidate, finding db.SeoDoctorFinding) (json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, error) {
	if !meaningfulJSON(finding.Evidence) {
		return nil, nil, nil, nil, fmt.Errorf("%w: finding evidence is required", ErrIncompleteCandidate)
	}
	var findingEvidence any = map[string]any{}
	if len(finding.Evidence) > 0 && json.Unmarshal(finding.Evidence, &findingEvidence) != nil {
		return nil, nil, nil, nil, fmt.Errorf("%w: invalid finding evidence", ErrIncompleteCandidate)
	}
	var evidenceIDs any = []any{}
	if len(candidate.EvidenceIds) > 0 && json.Unmarshal(candidate.EvidenceIds, &evidenceIDs) != nil {
		return nil, nil, nil, nil, fmt.Errorf("%w: invalid candidate evidence ids", ErrIncompleteCandidate)
	}
	var mutations any
	if json.Unmarshal(candidate.ProposedMutations, &mutations) != nil {
		return nil, nil, nil, nil, fmt.Errorf("%w: invalid proposed mutations", ErrIncompleteCandidate)
	}
	var surfaces any = []any{}
	if len(finding.LikelyFilesOrSurfaces) > 0 && json.Unmarshal(finding.LikelyFilesOrSurfaces, &surfaces) != nil {
		return nil, nil, nil, nil, fmt.Errorf("%w: invalid likely surfaces", ErrIncompleteCandidate)
	}
	var tests []any
	if json.Unmarshal(finding.AcceptanceTests, &tests) != nil || len(tests) == 0 {
		return nil, nil, nil, nil, fmt.Errorf("%w: acceptance tests are required", ErrIncompleteCandidate)
	}
	evidence, err := json.Marshal(map[string]any{
		"finding": findingEvidence, "evidence_ids": evidenceIDs,
		"evidence_fingerprint": candidate.EvidenceFingerprint,
	})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	proposed, err := json.Marshal(map[string]any{
		"mutations": mutations, "fix_intent": finding.FixIntent,
		"developer_instructions":   finding.DeveloperInstructions,
		"likely_files_or_surfaces": surfaces,
	})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return evidence, proposed, candidate.NormalizedTargetSet, finding.AcceptanceTests, nil
}

func validateLockedFindingSnapshot(locked db.DiscoveryCandidate, finding db.SeoDoctorFinding) error {
	projected := discovery.ProjectDoctorFinding(finding)
	if projected.Status != discovery.StatusIdentityReady || projected.SuggestedOwner != discovery.OwnerDoctor || len(projected.EvidenceIDs) == 0 {
		return discovery.ErrSnapshotStale
	}
	identity, err := discovery.BuildIdentity(projected)
	if err != nil {
		return discovery.ErrSnapshotStale
	}
	snapshotFingerprint, err := doctorFindingSnapshotFingerprint(finding, projected, identity)
	if err != nil {
		return fmt.Errorf("%w: fingerprint locked finding: %v", discovery.ErrSnapshotStale, err)
	}
	if locked.ShadowRunID != canonicalRunID(finding.ProjectID, finding.ID, snapshotFingerprint) ||
		locked.ProjectID != projected.ProjectID ||
		locked.SourceKind != string(projected.SourceKind) ||
		locked.SourceObjectType != projected.SourceObjectType ||
		locked.SourceObjectID != projected.SourceObjectID ||
		locked.TargetKind != projected.TargetKind ||
		locked.IssueOrHypothesisFamily != projected.IssueOrHypothesisFamily ||
		locked.ChangeFamily != projected.ChangeFamily ||
		locked.ArtifactIntent != string(projected.ArtifactIntent) ||
		valueOrEmpty(locked.IntendedSlugOrCanonical) != projected.IntendedSlugOrCanonical ||
		locked.PrimarySuccessMetric != projected.PrimarySuccessMetric ||
		locked.VerificationMode != string(projected.VerificationMode) ||
		locked.EvidenceFingerprint != projected.EvidenceFingerprint ||
		locked.SuggestedOwner != string(projected.SuggestedOwner) ||
		locked.CandidateSchemaVersion != projected.CandidateSchemaVersion ||
		locked.Status != string(projected.Status) ||
		locked.HoldReason != nil ||
		locked.ExactSignatureHash == nil || *locked.ExactSignatureHash != identity.ExactSignatureHash ||
		!sameJSON(locked.NormalizedTargetSet, projected.NormalizedTargetSet) ||
		!sameJSON(locked.ProposedMutations, projected.ProposedMutations) ||
		!sameJSON(locked.TopicEntityIdentity, projected.TopicEntityIdentity) ||
		!sameJSON(locked.AudienceIdentity, projected.AudienceIdentity) ||
		!sameJSON(locked.EvidenceIds, projected.EvidenceIDs) ||
		!sameJSON(locked.SignaturePayload, identity.SignaturePayload) ||
		!sameJSON(locked.ConflictBucketKeys, identity.ConflictBucketKeys) ||
		!sameConfidence(locked.Confidence, projected.Confidence) {
		return discovery.ErrSnapshotStale
	}
	return nil
}

func sameJSON(stored json.RawMessage, expected any) bool {
	var left any
	if json.Unmarshal(stored, &left) != nil {
		return false
	}
	raw, err := json.Marshal(expected)
	if err != nil {
		return false
	}
	var right any
	if json.Unmarshal(raw, &right) != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}

func sameConfidence(stored pgtype.Numeric, expected float64) bool {
	value, err := stored.Float64Value()
	return err == nil && value.Valid && value.Float64 == expected
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
