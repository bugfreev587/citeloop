package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/citeloop/citeloop/internal/growthwork"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/citeloop/citeloop/internal/sitefix"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDoctorSiteFixTransitionConflict = errors.New("canonical Site Fix transition conflict")
var ErrDoctorSiteFixCrossLineOwnership = errors.New("Doctor candidate is owned by Opportunities")
var ErrDoctorSiteFixCreateBusy = errors.New("Doctor Site Fix creation is busy")
var ErrDoctorSiteFixManualEvidenceInvalid = errors.New("Doctor Site Fix manual verification evidence is invalid")
var ErrDoctorAIVerificationNotAuthorized = errors.New("Doctor AI verification is not authorized for this request")
var ErrDoctorAIRequestAlreadyHandled = errors.New("Doctor AI verification request was already applied; use a new request id")
var ErrDoctorSiteFixMeasurementOptInConflict = errors.New("Doctor Site Fix is not eligible for optional measurement")
var errDoctorSiteFixPreparationReclaim = errors.New("Doctor Site Fix preparation lease must be reclaimed")
var errDoctorSiteFixPreparationLost = errors.New("Doctor Site Fix preparation lease was lost")
var errCanonicalSiteFixFreshReprepare = errors.New("canonical Site Fix requires one fresh repository preparation")
var errCanonicalSiteFixReprepareExhausted = errors.New("canonical Site Fix repository reprepare allowance exhausted")

// DoctorSiteFixArbitrationHoldError preserves the audited internal decision at
// the service boundary. HTTP responses derive a stable public explanation and
// never expose the raw internal review queue/state wording.
type DoctorSiteFixArbitrationHoldError struct {
	Decision discovery.PreparedDecision
}

func (e *DoctorSiteFixArbitrationHoldError) Error() string {
	if e == nil || strings.TrimSpace(e.Decision.Reason) == "" {
		return "Doctor Site Fix arbitration did not authorize work creation"
	}
	return strings.TrimSpace(e.Decision.Reason)
}

func (e *DoctorSiteFixArbitrationHoldError) publicReason() string {
	if e == nil {
		return "Doctor Site Fix arbitration did not authorize work creation"
	}
	switch e.Decision.Disposition {
	case discovery.DispositionIncompleteSpecification:
		return "The Doctor finding needs more evidence or specification before a Site Fix can be created"
	case discovery.DispositionProviderFailure:
		return "Semantic comparison is temporarily unavailable; no Site Fix was created"
	case discovery.DispositionExactMerge:
		return "An equivalent repair is already active; no duplicate Site Fix was created"
	default:
		return "Doctor Site Fix arbitration did not authorize work creation"
	}
}

// DoctorSiteFixService is the sole API writer/reader boundary for canonical
// Doctor Site Fixes. No legacy Opportunity or Content Action dependency is
// available through this interface.
type DoctorSiteFixService interface {
	CreateFromFinding(context.Context, uuid.UUID, uuid.UUID) (DoctorSiteFixResponse, bool, error)
	List(context.Context, uuid.UUID, *string) ([]DoctorSiteFixResponse, error)
	ListDoctorLinks(context.Context, uuid.UUID) ([]DoctorSiteFixResponse, error)
	Get(context.Context, uuid.UUID, uuid.UUID) (DoctorSiteFixResponse, error)
	DismissDoctorLink(context.Context, uuid.UUID, uuid.UUID, string, time.Time) (db.SiteFix, error)
	Approve(context.Context, uuid.UUID, uuid.UUID, time.Time) (DoctorSiteFixResponse, error)
	OptInMeasurement(context.Context, uuid.UUID, uuid.UUID, time.Time) (DoctorSiteFixMeasurementOptInResponse, error)
}

type DoctorSiteFixMeasurementOptInResponse struct {
	SiteFixID   uuid.UUID                             `json:"site_fix_id"`
	Measurement DoctorSiteFixMeasurementPublic        `json:"measurement"`
	Handoff     DoctorSiteFixMeasurementHandoffPublic `json:"handoff"`
}

type DoctorSiteFixMeasurementPublic struct {
	ID                     uuid.UUID `json:"id"`
	MeasurementGeneration  int32     `json:"measurement_generation"`
	Status                 string    `json:"status"`
	ProspectiveObservation bool      `json:"prospective_observation"`
	BaselineStatus         string    `json:"baseline_status"`
	AttributionConfidence  string    `json:"attribution_confidence"`
	ResultsDeepLink        *string   `json:"results_deep_link,omitempty"`
}

type DoctorSiteFixMeasurementHandoffPublic struct {
	Status string `json:"status"`
}

// DoctorSiteFixResponse is the persistent read model used by every canonical
// Doctor Site Fix endpoint. Embedding keeps the canonical row shape stable
// while application and verification history remain explicit, append-only
// lifecycle evidence.
type DoctorSiteFixResponse struct {
	db.SiteFix
	Application              *db.SiteChangeApplication       `json:"application"`
	Verifications            []db.SiteFixVerification        `json:"verifications"`
	LegacyAliases            []DoctorSiteFixLegacyAlias      `json:"legacy_aliases"`
	MeasurementSummary       *DoctorSiteFixMeasurementPublic `json:"measurement_summary"`
	MeasurementHandoffStatus string                          `json:"measurement_handoff_status"`
}

type DoctorSiteFixLegacyAlias struct {
	ObjectType string    `json:"object_type"`
	ObjectID   uuid.UUID `json:"object_id"`
}

type DoctorSiteFixVerificationRequest struct {
	SiteFix     db.SiteFix               `json:"site_fix"`
	Application db.SiteChangeApplication `json:"application"`
}

type DoctorSiteFixVerificationInput struct {
	ManualEvidence    *DoctorSiteFixManualEvidence `json:"manual_evidence,omitempty"`
	AIReviewRequestID *uuid.UUID                   `json:"ai_review_request_id,omitempty"`
}

type DoctorSiteFixManualEvidence struct {
	HumanConfirmed    bool                                  `json:"human_confirmed"`
	TargetURL         string                                `json:"target_url"`
	AcceptanceResults []DoctorSiteFixManualAcceptanceResult `json:"acceptance_results"`
}

type DoctorSiteFixManualAcceptanceResult struct {
	Index           int             `json:"index"`
	TestFingerprint string          `json:"test_fingerprint"`
	Status          string          `json:"status"`
	Evidence        json.RawMessage `json:"evidence"`
}

type DoctorSiteFixLifecycleService interface {
	Apply(context.Context, uuid.UUID, uuid.UUID) (sitefix.ApplyResult, error)
	RequestVerification(context.Context, uuid.UUID, uuid.UUID, DoctorSiteFixVerificationInput) (DoctorSiteFixVerificationRequest, error)
	Terminate(context.Context, uuid.UUID, uuid.UUID) (DoctorSiteFixVerificationRequest, error)
}

type postgresDoctorSiteFixLifecycleService struct {
	pool  *pgxpool.Pool
	q     *db.Queries
	apply sitefix.ApplyService
}

func (s *postgresDoctorSiteFixLifecycleService) Apply(ctx context.Context, projectID, fixID uuid.UUID) (sitefix.ApplyResult, error) {
	// The authenticated, user-triggered Apply is itself the bounded on-demand
	// authorization for source selection, patch generation, and grounding. It
	// does not enable scheduled Doctor AI or mutate project automation settings.
	return s.apply.Apply(ctx, projectID, fixID)
}

func (s *postgresDoctorSiteFixLifecycleService) RequestVerification(ctx context.Context, projectID, fixID uuid.UUID, input DoctorSiteFixVerificationInput) (DoctorSiteFixVerificationRequest, error) {
	if input.ManualEvidence != nil && input.AIReviewRequestID != nil {
		return DoctorSiteFixVerificationRequest{}, ErrDoctorSiteFixManualEvidenceInvalid
	}
	if input.AIReviewRequestID != nil && *input.AIReviewRequestID == uuid.Nil {
		return DoctorSiteFixVerificationRequest{}, ErrDoctorAIVerificationNotAuthorized
	}
	fix, err := s.q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
	if err != nil {
		return DoctorSiteFixVerificationRequest{}, err
	}
	app, err := s.q.GetLatestCanonicalSiteFixApplication(ctx, db.GetLatestCanonicalSiteFixApplicationParams{ProjectID: projectID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true}})
	if err != nil {
		return DoctorSiteFixVerificationRequest{}, err
	}
	if input.ManualEvidence != nil {
		if _, err := validateDoctorSiteFixManualEvidence(fix, app, *input.ManualEvidence); err != nil {
			return DoctorSiteFixVerificationRequest{}, err
		}
	}
	legalLifecycle := fix.Status == "awaiting_deploy" || fix.Status == "verifying" || fix.Status == "reopened" || fix.Status == "failed_retryable" || (fix.Status == "applying" && app.Status == "manual_apply_required")
	if !legalLifecycle {
		return DoctorSiteFixVerificationRequest{}, fmt.Errorf("%w: cannot verify from %s", sitefix.ErrLifecycleConflict, fix.Status)
	}
	if input.AIReviewRequestID != nil {
		if s.pool == nil {
			return DoctorSiteFixVerificationRequest{}, errors.New("canonical Site Fix database unavailable")
		}
		tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return DoctorSiteFixVerificationRequest{}, err
		}
		defer tx.Rollback(ctx)
		q := db.New(tx)
		if _, err := q.ClaimDoctorAIOnDemandTrigger(ctx, db.ClaimDoctorAIOnDemandTriggerParams{RequestID: *input.AIReviewRequestID, ProjectID: projectID, SiteFixID: fixID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				if marker, markerErr := q.GetDoctorAIOnDemandTrigger(ctx, db.GetDoctorAIOnDemandTriggerParams{RequestID: *input.AIReviewRequestID, ProjectID: projectID, SiteFixID: fixID}); markerErr == nil && (marker.Status == "rejected" || marker.Status == "superseded" || (marker.Status == "consumed" && marker.LifecycleAppliedAt.Valid)) {
					return DoctorSiteFixVerificationRequest{}, ErrDoctorAIRequestAlreadyHandled
				}
				return DoctorSiteFixVerificationRequest{}, ErrDoctorAIVerificationNotAuthorized
			}
			return DoctorSiteFixVerificationRequest{}, err
		}
		if fix.Status == "applying" {
			now := time.Now().UTC()
			if _, err := q.MarkCanonicalSiteFixManualApplied(ctx, db.MarkCanonicalSiteFixManualAppliedParams{
				ProjectID: projectID, SiteFixID: fix.ID, ApplicationID: app.ID,
				DeploymentSnapshot: json.RawMessage(`{"source":"manual_confirmation"}`), ManualAppliedAt: pgutil.TS(now),
			}); err != nil {
				return DoctorSiteFixVerificationRequest{}, canonicalDoctorLifecycleTransitionError(err)
			}
		}
		if fix.Status == "failed_retryable" {
			if _, err := q.ReopenCanonicalSiteFix(ctx, db.ReopenCanonicalSiteFixParams{SiteFixID: fixID, ProjectID: projectID, ApplicationID: app.ID}); err != nil {
				return DoctorSiteFixVerificationRequest{}, canonicalDoctorLifecycleTransitionError(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return DoctorSiteFixVerificationRequest{}, err
		}
		fix, err = s.q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
		if err != nil {
			return DoctorSiteFixVerificationRequest{}, err
		}
		app, err = s.q.GetLatestCanonicalSiteFixApplication(ctx, db.GetLatestCanonicalSiteFixApplicationParams{ProjectID: projectID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true}})
		if err != nil {
			return DoctorSiteFixVerificationRequest{}, err
		}
	}
	if input.AIReviewRequestID == nil && fix.Status == "applying" && app.Status == "manual_apply_required" {
		if err := s.markManualApplicationAwaitingDeploy(ctx, fix, app); err != nil {
			return DoctorSiteFixVerificationRequest{}, err
		}
		fix, err = s.q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
		if err != nil {
			return DoctorSiteFixVerificationRequest{}, err
		}
		app, err = s.q.GetLatestCanonicalSiteFixApplication(ctx, db.GetLatestCanonicalSiteFixApplicationParams{ProjectID: projectID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true}})
		if err != nil {
			return DoctorSiteFixVerificationRequest{}, err
		}
	}
	if input.AIReviewRequestID == nil && fix.Status == "failed_retryable" {
		if _, err := s.q.ReopenCanonicalSiteFix(ctx, db.ReopenCanonicalSiteFixParams{SiteFixID: fixID, ProjectID: projectID, ApplicationID: app.ID}); err != nil {
			return DoctorSiteFixVerificationRequest{}, canonicalDoctorLifecycleTransitionError(err)
		}
		fix, err = s.q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
		if err != nil {
			return DoctorSiteFixVerificationRequest{}, err
		}
		app, err = s.q.GetLatestCanonicalSiteFixApplication(ctx, db.GetLatestCanonicalSiteFixApplicationParams{ProjectID: projectID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true}})
		if err != nil {
			return DoctorSiteFixVerificationRequest{}, err
		}
	}
	if fix.Status != "awaiting_deploy" && fix.Status != "verifying" && fix.Status != "reopened" {
		return DoctorSiteFixVerificationRequest{}, fmt.Errorf("%w: cannot verify from %s", sitefix.ErrLifecycleConflict, fix.Status)
	}
	if !app.SiteFixID.Valid || app.ContentActionID.Valid {
		return DoctorSiteFixVerificationRequest{}, errors.New("canonical Site Fix application source is invalid")
	}
	if input.ManualEvidence != nil {
		return s.verifyWithAuthenticatedManualEvidence(ctx, fix, app, *input.ManualEvidence)
	}
	if err := s.q.SetCanonicalSiteFixNextPollAt(ctx, db.SetCanonicalSiteFixNextPollAtParams{ApplicationID: app.ID, ProjectID: projectID, SiteFixID: app.SiteFixID, NextPollAt: pgutil.TS(time.Now().UTC())}); err != nil {
		return DoctorSiteFixVerificationRequest{}, err
	}
	return DoctorSiteFixVerificationRequest{SiteFix: fix, Application: app}, nil
}

func (s *postgresDoctorSiteFixLifecycleService) Terminate(ctx context.Context, projectID, fixID uuid.UUID) (DoctorSiteFixVerificationRequest, error) {
	fix, err := s.q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
	if err != nil {
		return DoctorSiteFixVerificationRequest{}, err
	}
	app, appErr := s.q.GetLatestCanonicalSiteFixApplication(ctx, db.GetLatestCanonicalSiteFixApplicationParams{ProjectID: projectID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true}})
	if appErr != nil && !errors.Is(appErr, pgx.ErrNoRows) {
		return DoctorSiteFixVerificationRequest{}, appErr
	}
	if fix.Status == "failed_terminal" {
		return DoctorSiteFixVerificationRequest{SiteFix: fix, Application: app}, nil
	}
	reason := "terminated by user"
	snapshot := mustJSONLocal(map[string]any{"source": "user_termination", "terminated_at": time.Now().UTC().Format(time.RFC3339)})
	if _, err := s.q.TerminateCanonicalSiteFixByUser(ctx, db.TerminateCanonicalSiteFixByUserParams{
		ProjectID: projectID, SiteFixID: fix.ID, VerificationSnapshot: snapshot, FailureReason: &reason,
	}); err != nil {
		return DoctorSiteFixVerificationRequest{}, canonicalDoctorLifecycleTransitionError(err)
	}
	fix, err = s.q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
	if err != nil {
		return DoctorSiteFixVerificationRequest{}, err
	}
	app, appErr = s.q.GetLatestCanonicalSiteFixApplication(ctx, db.GetLatestCanonicalSiteFixApplicationParams{ProjectID: projectID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true}})
	if appErr != nil && !errors.Is(appErr, pgx.ErrNoRows) {
		return DoctorSiteFixVerificationRequest{}, appErr
	}
	return DoctorSiteFixVerificationRequest{SiteFix: fix, Application: app}, nil
}

func (s *postgresDoctorSiteFixLifecycleService) verifyWithAuthenticatedManualEvidence(ctx context.Context, fix db.SiteFix, app db.SiteChangeApplication, evidence DoctorSiteFixManualEvidence) (DoctorSiteFixVerificationRequest, error) {
	results, err := validateDoctorSiteFixManualEvidence(fix, app, evidence)
	if err != nil {
		return DoctorSiteFixVerificationRequest{}, err
	}
	if s.pool == nil {
		return DoctorSiteFixVerificationRequest{}, errors.New("canonical Site Fix database unavailable")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return DoctorSiteFixVerificationRequest{}, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	now := time.Now().UTC()
	deployment := mustJSONLocal(map[string]any{"source": "authenticated_manual_evidence", "target_url": evidence.TargetURL, "observed_at": now.Format(time.RFC3339)})
	if fix.Status == "awaiting_deploy" || fix.Status == "reopened" {
		if _, err := q.MarkCanonicalSiteFixVerifying(ctx, db.MarkCanonicalSiteFixVerifyingParams{
			SiteFixID: fix.ID, ProjectID: fix.ProjectID, ApplicationID: app.ID,
			DeploymentSnapshot: deployment, DeployedAt: pgutil.TS(now),
		}); err != nil {
			return DoctorSiteFixVerificationRequest{}, canonicalDoctorLifecycleTransitionError(err)
		}
	}
	snapshot := mustJSONLocal(map[string]any{"source": "authenticated_manual_evidence", "human_confirmed": true, "target_url": evidence.TargetURL, "acceptance_results": json.RawMessage(results), "verified_at": now.Format(time.RFC3339)})
	if _, err := q.AppendCanonicalSiteFixVerification(ctx, db.AppendCanonicalSiteFixVerificationParams{
		ID: uuid.New(), ProjectID: fix.ProjectID, SiteFixID: fix.ID, AttemptNumber: fix.RetryCount + 1,
		EvidenceRead:      mustJSONLocal(map[string]any{"source": "authenticated_manual_evidence", "target_url": evidence.TargetURL}),
		AcceptanceResults: results, Result: "passed", RetryClassification: "not_applicable", AttemptedAt: pgutil.TS(now),
	}); err != nil {
		return DoctorSiteFixVerificationRequest{}, canonicalDoctorLifecycleTransitionError(err)
	}
	if _, err := q.MarkCanonicalSiteFixVerified(ctx, db.MarkCanonicalSiteFixVerifiedParams{
		SiteFixID: fix.ID, ProjectID: fix.ProjectID, ApplicationID: app.ID,
		DeploymentSnapshot: deployment, VerificationSnapshot: snapshot, VerifiedAt: pgutil.TS(now),
	}); err != nil {
		return DoctorSiteFixVerificationRequest{}, canonicalDoctorLifecycleTransitionError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return DoctorSiteFixVerificationRequest{}, err
	}
	verifiedFix, err := s.q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fix.ID, ProjectID: fix.ProjectID})
	if err != nil {
		return DoctorSiteFixVerificationRequest{}, err
	}
	verifiedApp, err := s.q.GetLatestCanonicalSiteFixApplication(ctx, db.GetLatestCanonicalSiteFixApplicationParams{ProjectID: fix.ProjectID, SiteFixID: pgtype.UUID{Bytes: fix.ID, Valid: true}})
	if err != nil {
		return DoctorSiteFixVerificationRequest{}, err
	}
	return DoctorSiteFixVerificationRequest{SiteFix: verifiedFix, Application: verifiedApp}, nil
}

func validateDoctorSiteFixManualEvidence(fix db.SiteFix, app db.SiteChangeApplication, evidence DoctorSiteFixManualEvidence) (json.RawMessage, error) {
	if !evidence.HumanConfirmed || strings.TrimSpace(evidence.TargetURL) == "" || strings.TrimSpace(evidence.TargetURL) != strings.TrimSpace(app.TargetUrl) {
		return nil, ErrDoctorSiteFixManualEvidenceInvalid
	}
	var tests []json.RawMessage
	if json.Unmarshal(fix.AcceptanceTests, &tests) != nil || len(tests) == 0 || len(evidence.AcceptanceResults) != len(tests) {
		return nil, ErrDoctorSiteFixManualEvidenceInvalid
	}
	seen := make(map[int]bool, len(tests))
	for _, result := range evidence.AcceptanceResults {
		if result.Index < 0 || result.Index >= len(tests) || seen[result.Index] || result.Status != "passed" {
			return nil, ErrDoctorSiteFixManualEvidenceInvalid
		}
		sum := sha256.Sum256(tests[result.Index])
		if !strings.EqualFold(result.TestFingerprint, fmt.Sprintf("sha256:%x", sum[:])) {
			return nil, ErrDoctorSiteFixManualEvidenceInvalid
		}
		var detail map[string]any
		if json.Unmarshal(result.Evidence, &detail) != nil || len(detail) == 0 {
			return nil, ErrDoctorSiteFixManualEvidenceInvalid
		}
		seen[result.Index] = true
	}
	return json.Marshal(evidence.AcceptanceResults)
}

func (s *postgresDoctorSiteFixLifecycleService) markManualApplicationAwaitingDeploy(ctx context.Context, fix db.SiteFix, app db.SiteChangeApplication) error {
	now := time.Now().UTC()
	_, err := s.q.MarkCanonicalSiteFixManualApplied(ctx, db.MarkCanonicalSiteFixManualAppliedParams{
		ProjectID: fix.ProjectID, SiteFixID: fix.ID, ApplicationID: app.ID,
		DeploymentSnapshot: json.RawMessage(`{"source":"manual_confirmation"}`), ManualAppliedAt: pgutil.TS(now),
	})
	return canonicalDoctorLifecycleTransitionError(err)
}

type doctorSiteFixPreparationClaim struct {
	ProjectID                   uuid.UUID
	ExactSignatureHash          string
	CandidateID                 uuid.UUID
	EvidenceFingerprint         string
	LeaseToken                  uuid.UUID
	RuntimeAuthorityFingerprint string
	ExpiresAt                   time.Time
	LeaseTTLSeconds             int32
	Leader                      bool
}

type doctorSiteFixPreparationManager interface {
	Claim(context.Context, uuid.UUID, sitefix.CanonicalCandidate) (doctorSiteFixPreparationClaim, error)
	Wait(context.Context, doctorSiteFixPreparationClaim) (discovery.PreparedDecision, error)
	Validate(context.Context, doctorSiteFixPreparationClaim) (doctorSiteFixPreparationClaim, error)
	Complete(context.Context, doctorSiteFixPreparationClaim, discovery.PreparedDecision) error
	Fail(context.Context, doctorSiteFixPreparationClaim, string) error
	Invalidate(context.Context, doctorSiteFixPreparationClaim) error
}

type doctorSiteFixCreationBackend interface {
	LoadFinding(context.Context, uuid.UUID, uuid.UUID) (db.SeoDoctorFinding, error)
	Materialize(context.Context, db.SeoDoctorFinding) (sitefix.CanonicalCandidate, error)
	FindEvidenceMerge(context.Context, uuid.UUID, sitefix.CanonicalCandidate) (db.SiteFix, bool, error)
	Prepare(context.Context, uuid.UUID, sitefix.CanonicalCandidate) (discovery.PreparedDecision, error)
	Reserve(context.Context, uuid.UUID, discovery.PreparedDecision, doctorSiteFixPreparationClaim) (discovery.ReservationResult, error)
	ResolveOverlap(context.Context, uuid.UUID, discovery.PreparedDecision) (db.SiteFix, error)
	RecordEvidenceMerge(context.Context, db.SeoDoctorFinding, sitefix.CanonicalCandidate, discovery.PreparedDecision, db.SiteFix, doctorSiteFixPreparationClaim) error
	LoadSiteFix(context.Context, uuid.UUID, uuid.UUID) (db.SiteFix, error)
}

type doctorSiteFixCreationCoordinator struct {
	preparations doctorSiteFixPreparationManager
	backend      doctorSiteFixCreationBackend
}

func (c doctorSiteFixCreationCoordinator) CreateFromFinding(ctx context.Context, projectID, findingID uuid.UUID) (fix db.SiteFix, created bool, resultErr error) {
	if c.preparations == nil || c.backend == nil {
		return db.SiteFix{}, false, errors.New("canonical Site Fix creation dependencies unavailable")
	}
	for attempt := 0; attempt < 3; attempt++ {
		// Every attempt reloads and rematerializes before Phase A. If Phase B
		// reports a stale snapshot, the previous preparation claim is failed by
		// runPreparationLeader before this loop reruns the provider comparison
		// outside database locks against current evidence and bucket versions.
		finding, err := c.backend.LoadFinding(ctx, projectID, findingID)
		if err != nil {
			return db.SiteFix{}, false, err
		}
		if finding.ProjectID != projectID || finding.ID != findingID {
			return db.SiteFix{}, false, sitefix.ErrProjectMismatch
		}
		if finding.FindingKind == "healthy" || strings.EqualFold(strings.TrimSpace(finding.IssueType), "no_active_technical_blockers") {
			return db.SiteFix{}, false, sitefix.ErrHealthyFinding
		}
		if finding.Status != "active" {
			return db.SiteFix{}, false, fmt.Errorf("%w: finding status %q", sitefix.ErrIncompleteCandidate, finding.Status)
		}
		canonical, err := c.backend.Materialize(ctx, finding)
		if err != nil {
			return db.SiteFix{}, false, err
		}
		if strings.TrimSpace(canonical.Identity.ExactSignatureHash) == "" {
			return db.SiteFix{}, false, sitefix.ErrIncompleteCandidate
		}
		if existing, ok, err := c.backend.FindEvidenceMerge(ctx, projectID, canonical); err != nil {
			return db.SiteFix{}, false, err
		} else if ok {
			return existing, false, nil
		}
		claim, err := c.preparations.Claim(ctx, projectID, canonical)
		if err != nil {
			return db.SiteFix{}, false, err
		}
		if !claim.Leader {
			prepared, waitErr := c.preparations.Wait(ctx, claim)
			if errors.Is(waitErr, errDoctorSiteFixPreparationReclaim) {
				continue
			}
			if waitErr != nil {
				return db.SiteFix{}, false, waitErr
			}
			if existing, ok, findErr := c.backend.FindEvidenceMerge(ctx, projectID, canonical); findErr != nil {
				return db.SiteFix{}, false, findErr
			} else if ok {
				return existing, false, nil
			}
			// A completed create/merge result cannot be mutated by a follower
			// without its own fencing claim. Invalidate and reclaim so the exact
			// deterministic branch records this candidate's evidence atomically.
			if prepared.Owner == discovery.OwnerDoctor && prepared.Status == discovery.ArbitrationStatusPrepared &&
				(prepared.Decision == discovery.DecisionCreate || prepared.Decision == discovery.DecisionMergeEvidence) {
				if err := c.preparations.Invalidate(ctx, claim); err != nil {
					return db.SiteFix{}, false, err
				}
				continue
			}
			return c.consumeFollowerPreparation(ctx, finding, canonical, prepared)
		}
		fix, created, err := c.runPreparationLeader(ctx, finding, canonical, claim)
		if errors.Is(err, discovery.ErrSnapshotStale) {
			continue
		}
		return fix, created, err
	}
	return db.SiteFix{}, false, ErrDoctorSiteFixCreateBusy
}

func (c doctorSiteFixCreationCoordinator) runPreparationLeader(ctx context.Context, finding db.SeoDoctorFinding, canonical sitefix.CanonicalCandidate, claim doctorSiteFixPreparationClaim) (fix db.SiteFix, created bool, resultErr error) {
	completed := false
	defer func() {
		if !completed {
			_ = c.preparations.Fail(context.WithoutCancel(ctx), claim, "leader_failed")
		}
	}()
	prepareCtx := ctx
	cancel := func() {}
	if !claim.ExpiresAt.IsZero() {
		deadline := claim.ExpiresAt.Add(-5 * time.Second)
		if !deadline.After(time.Now()) {
			return db.SiteFix{}, false, errDoctorSiteFixPreparationLost
		}
		prepareCtx, cancel = context.WithDeadline(ctx, deadline)
	}
	defer cancel()
	prepared, err := c.backend.Prepare(prepareCtx, finding.ProjectID, canonical)
	if err != nil {
		return db.SiteFix{}, false, err
	}
	claim, err = c.preparations.Validate(ctx, claim)
	if err != nil {
		return db.SiteFix{}, false, errDoctorSiteFixPreparationLost
	}
	if prepared.Status == discovery.ArbitrationStatusHeld {
		if err := c.preparations.Complete(ctx, claim, prepared); err != nil {
			return db.SiteFix{}, false, err
		}
		completed = true
		return db.SiteFix{}, false, &DoctorSiteFixArbitrationHoldError{Decision: prepared}
	}
	if prepared.Owner == discovery.OwnerOpportunities {
		if err := c.preparations.Complete(ctx, claim, prepared); err != nil {
			return db.SiteFix{}, false, err
		}
		completed = true
		if prepared.Decision == discovery.DecisionCreate || prepared.Decision == discovery.DecisionMergeEvidence || prepared.Decision == discovery.DecisionBlockOnOtherLine {
			return db.SiteFix{}, false, ErrDoctorSiteFixCrossLineOwnership
		}
		return db.SiteFix{}, false, &DoctorSiteFixArbitrationHoldError{Decision: prepared}
	}
	if prepared.Owner != discovery.OwnerDoctor {
		return db.SiteFix{}, false, &DoctorSiteFixArbitrationHoldError{Decision: prepared}
	}
	if prepared.Status == discovery.ArbitrationStatusPrepared && prepared.Decision == discovery.DecisionMergeEvidence {
		existing, err := c.backend.ResolveOverlap(ctx, finding.ProjectID, prepared)
		if err != nil {
			return db.SiteFix{}, false, err
		}
		if err := c.backend.RecordEvidenceMerge(ctx, finding, canonical, prepared, existing, claim); err != nil {
			return db.SiteFix{}, false, err
		}
		if err := c.preparations.Complete(ctx, claim, prepared); err != nil {
			return db.SiteFix{}, false, err
		}
		completed = true
		return existing, false, nil
	}
	if prepared.Decision != discovery.DecisionCreate {
		if err := c.preparations.Complete(ctx, claim, prepared); err != nil {
			return db.SiteFix{}, false, err
		}
		completed = true
		return db.SiteFix{}, false, &DoctorSiteFixArbitrationHoldError{Decision: prepared}
	}

	result, err := c.backend.Reserve(ctx, finding.ProjectID, prepared, claim)
	if err != nil {
		// The outer coordinator retries stale snapshots only after this leader
		// returns and its deferred claim failure releases the old preparation.
		return db.SiteFix{}, false, err
	}
	if result.Work.Type != "site_fix" {
		return db.SiteFix{}, false, fmt.Errorf("canonical reservation returned work type %q", result.Work.Type)
	}
	createdFix, err := c.backend.LoadSiteFix(ctx, finding.ProjectID, result.Work.ID)
	if err != nil {
		return db.SiteFix{}, false, err
	}
	if err := c.preparations.Complete(ctx, claim, prepared); err != nil {
		return db.SiteFix{}, false, err
	}
	completed = true
	return createdFix, true, nil
}

func (c doctorSiteFixCreationCoordinator) consumeFollowerPreparation(ctx context.Context, finding db.SeoDoctorFinding, canonical sitefix.CanonicalCandidate, leaderDecision discovery.PreparedDecision) (db.SiteFix, bool, error) {
	if leaderDecision.Status == discovery.ArbitrationStatusHeld {
		return db.SiteFix{}, false, &DoctorSiteFixArbitrationHoldError{Decision: leaderDecision}
	}
	if leaderDecision.Owner == discovery.OwnerOpportunities && (leaderDecision.Decision == discovery.DecisionCreate || leaderDecision.Decision == discovery.DecisionMergeEvidence || leaderDecision.Decision == discovery.DecisionBlockOnOtherLine) {
		return db.SiteFix{}, false, ErrDoctorSiteFixCrossLineOwnership
	}
	if leaderDecision.Decision != discovery.DecisionCreate && leaderDecision.Decision != discovery.DecisionMergeEvidence {
		return db.SiteFix{}, false, &DoctorSiteFixArbitrationHoldError{Decision: leaderDecision}
	}
	return db.SiteFix{}, false, errDoctorSiteFixPreparationReclaim
}

type postgresDoctorSiteFixService struct {
	q                  *db.Queries
	approvalTx         canonicalSiteFixApprovalTransactionRunner
	measurementOptInTx canonicalSiteFixMeasurementOptInTransactionRunner
	creation           doctorSiteFixCreationCoordinator
	loadProjectConfig  func(context.Context, uuid.UUID) (config.ProjectConfig, error)
	newGrowthCutover   func(discovery.SemanticComparator) growthProjectCutover
	growthProvider     llm.Provider
	growthModel        string
}

type canonicalSiteFixApprovalStore interface {
	GetCanonicalSiteFix(context.Context, db.GetCanonicalSiteFixParams) (db.SiteFix, error)
	ApproveCanonicalSiteFix(context.Context, db.ApproveCanonicalSiteFixParams) (db.ApproveCanonicalSiteFixRow, error)
}

type canonicalSiteFixApprovalMeasurementStore interface {
	canonicalSiteFixApprovalStore
	CreateSiteFixMeasurement(context.Context, db.CreateSiteFixMeasurementParams) (db.SiteFixMeasurement, error)
}

type canonicalSiteFixApprovalTransactionRunner interface {
	Run(context.Context, func(canonicalSiteFixApprovalMeasurementStore) error) error
}

type postgresCanonicalSiteFixApprovalTransactionRunner struct{ pool *pgxpool.Pool }

func (r postgresCanonicalSiteFixApprovalTransactionRunner) Run(ctx context.Context, fn func(canonicalSiteFixApprovalMeasurementStore) error) error {
	if r.pool == nil {
		return errors.New("canonical Site Fix approval database unavailable")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(db.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type canonicalSiteFixMeasurementOptInStore interface {
	GetCanonicalSiteFix(context.Context, db.GetCanonicalSiteFixParams) (db.SiteFix, error)
	CreateSiteFixMeasurement(context.Context, db.CreateSiteFixMeasurementParams) (db.SiteFixMeasurement, error)
	EnqueueSiteFixMeasurementHandoff(context.Context, db.EnqueueSiteFixMeasurementHandoffParams) (db.SiteFixMeasurementHandoffOutbox, error)
}

type canonicalSiteFixMeasurementOptInTransactionRunner interface {
	Run(context.Context, func(canonicalSiteFixMeasurementOptInStore) error) error
}

type postgresCanonicalSiteFixMeasurementOptInTransactionRunner struct{ pool *pgxpool.Pool }

func (r postgresCanonicalSiteFixMeasurementOptInTransactionRunner) Run(ctx context.Context, fn func(canonicalSiteFixMeasurementOptInStore) error) error {
	if r.pool == nil {
		return errors.New("canonical Site Fix measurement database unavailable")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(db.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type growthProjectCutover interface {
	EnsureProjectCutover(context.Context, uuid.UUID) error
}

func NewDoctorSiteFixService(pool *pgxpool.Pool, q *db.Queries, provider llm.Provider, model string) DoctorSiteFixService {
	backend := &postgresDoctorSiteFixBackend{pool: pool, q: q, model: strings.TrimSpace(model)}
	return &postgresDoctorSiteFixService{
		q:                  q,
		approvalTx:         postgresCanonicalSiteFixApprovalTransactionRunner{pool: pool},
		measurementOptInTx: postgresCanonicalSiteFixMeasurementOptInTransactionRunner{pool: pool},
		loadProjectConfig: func(ctx context.Context, projectID uuid.UUID) (config.ProjectConfig, error) {
			project, err := q.GetProject(ctx, projectID)
			if err != nil {
				return config.ProjectConfig{}, err
			}
			return config.Parse(project.Config)
		},
		newGrowthCutover: func(growthComparator discovery.SemanticComparator) growthProjectCutover {
			return growthwork.NewService(pool, q, growthComparator)
		},
		growthProvider: provider,
		growthModel:    strings.TrimSpace(model),
		creation: doctorSiteFixCreationCoordinator{
			preparations: &postgresDoctorSiteFixPreparationManager{
				q: q, requestedModel: strings.TrimSpace(model),
				leaseTTL: 2 * time.Minute, resultTTL: 5 * time.Minute,
				waitTimeout: 30 * time.Second, pollInterval: 100 * time.Millisecond,
			},
			backend: backend,
		},
	}
}

func (s *postgresDoctorSiteFixService) creationForProjectConfig(projectConfig config.ProjectConfig) doctorSiteFixCreationCoordinator {
	creation := s.creation
	allowed := projectConfig.AllowsDoctorAI(config.DoctorAITriggerApplyUser)
	if backend, ok := creation.backend.(*postgresDoctorSiteFixBackend); ok && backend != nil {
		requestBackend := *backend
		requestBackend.comparator = nil
		if allowed && s.growthProvider != nil {
			requestBackend.comparator = discovery.NewLLMSemanticComparator(s.growthProvider, "tokengate", s.growthModel).WithPurpose(llm.PurposeSiteFix)
		}
		creation.backend = &requestBackend
	}
	if manager, ok := creation.preparations.(*postgresDoctorSiteFixPreparationManager); ok && manager != nil {
		requestManager := *manager
		requestManager.runtimeProvider = nil
		if allowed {
			requestManager.runtimeProvider = s.growthProvider
		}
		creation.preparations = &requestManager
	}
	return creation
}

func (s *postgresDoctorSiteFixService) CreateFromFinding(ctx context.Context, projectID, findingID uuid.UUID) (DoctorSiteFixResponse, bool, error) {
	if s == nil || s.q == nil {
		return DoctorSiteFixResponse{}, false, errors.New("canonical Site Fix database unavailable")
	}
	if s.loadProjectConfig == nil || s.newGrowthCutover == nil {
		return DoctorSiteFixResponse{}, false, errors.New("Growth visibility gate unavailable")
	}
	projectConfig, err := s.loadProjectConfig(ctx, projectID)
	if err != nil {
		return DoctorSiteFixResponse{}, false, fmt.Errorf("load Growth AI authority: %w", err)
	}
	growthComparator := (growthwork.ComparatorAuthority{Provider: s.growthProvider, Model: s.growthModel}).ForConfig(projectConfig, config.GrowthAITriggerManual)
	growthCutover := s.newGrowthCutover(growthComparator)
	if growthCutover == nil {
		return DoctorSiteFixResponse{}, false, errors.New("Growth visibility gate unavailable")
	}
	if err := growthCutover.EnsureProjectCutover(ctx, projectID); err != nil {
		return DoctorSiteFixResponse{}, false, fmt.Errorf("ensure Growth reservation visibility: %w", err)
	}
	creation := s.creationForProjectConfig(projectConfig)
	fix, created, err := creation.CreateFromFinding(ctx, projectID, findingID)
	if err != nil {
		return DoctorSiteFixResponse{}, false, err
	}
	response, err := s.loadResponse(ctx, fix)
	return response, created, err
}

type postgresDoctorSiteFixBackend struct {
	pool       *pgxpool.Pool
	q          *db.Queries
	comparator discovery.SemanticComparator
	model      string
}

func (b *postgresDoctorSiteFixBackend) arbitrationStore() *discovery.PostgresArbitrationStore {
	return discovery.NewPostgresArbitrationStore(b.pool, b.q).WithSemanticRuntime("tokengate", b.model)
}

func (b *postgresDoctorSiteFixBackend) LoadFinding(ctx context.Context, projectID, findingID uuid.UUID) (db.SeoDoctorFinding, error) {
	return b.q.GetSEODoctorFinding(ctx, db.GetSEODoctorFindingParams{ID: findingID, ProjectID: projectID})
}

func (b *postgresDoctorSiteFixBackend) Materialize(ctx context.Context, finding db.SeoDoctorFinding) (sitefix.CanonicalCandidate, error) {
	return sitefix.NewCandidateMaterializer(b.q).Materialize(ctx, finding)
}

func (b *postgresDoctorSiteFixBackend) FindEvidenceMerge(ctx context.Context, projectID uuid.UUID, canonical sitefix.CanonicalCandidate) (db.SiteFix, bool, error) {
	merge, err := b.q.GetCanonicalSiteFixEvidenceMerge(ctx, db.GetCanonicalSiteFixEvidenceMergeParams{
		ProjectID: projectID, CandidateID: canonical.ID, EvidenceFingerprint: canonical.Candidate.EvidenceFingerprint,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.SiteFix{}, false, nil
	}
	if err != nil {
		return db.SiteFix{}, false, err
	}
	fix, err := b.q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: merge.SiteFixID, ProjectID: projectID})
	return fix, err == nil, err
}

func (b *postgresDoctorSiteFixBackend) Prepare(ctx context.Context, projectID uuid.UUID, canonical sitefix.CanonicalCandidate) (discovery.PreparedDecision, error) {
	return discovery.NewArbitrationService(b.arbitrationStore(), b.comparator).Prepare(ctx, projectID, canonical.ID)
}

func (b *postgresDoctorSiteFixBackend) Reserve(ctx context.Context, projectID uuid.UUID, prepared discovery.PreparedDecision, claim doctorSiteFixPreparationClaim) (discovery.ReservationResult, error) {
	creator := preparationFencedSiteFixCreator{claim: claim}
	return discovery.NewReservationService(b.arbitrationStore()).ReservePrepared(ctx, projectID, prepared.ID, creator)
}

type preparationFencedSiteFixCreator struct {
	claim doctorSiteFixPreparationClaim
}

func (c preparationFencedSiteFixCreator) CreateInTransaction(ctx context.Context, q *db.Queries, work discovery.ReservedWork) (discovery.WorkReference, error) {
	if work.ProjectID != c.claim.ProjectID || work.CandidateID != c.claim.CandidateID {
		return discovery.WorkReference{}, errDoctorSiteFixPreparationLost
	}
	if q == nil {
		return discovery.WorkReference{}, errors.New("canonical Site Fix database unavailable")
	}
	if _, err := q.LockDoctorSiteFixPreparationLeaseForReserve(ctx, db.LockDoctorSiteFixPreparationLeaseForReserveParams{
		ProjectID: c.claim.ProjectID, ExactSignatureHash: c.claim.ExactSignatureHash,
		LeaseToken: c.claim.LeaseToken, LeaderCandidateID: c.claim.CandidateID,
		LeaseTtlSeconds: c.claim.LeaseTTLSeconds,
	}); err != nil {
		return discovery.WorkReference{}, normalizeDoctorSiteFixPreparationMutationError(err)
	}
	return (sitefix.Creator{}).CreateInTransaction(ctx, q, work)
}

func (b *postgresDoctorSiteFixBackend) ResolveOverlap(ctx context.Context, projectID uuid.UUID, prepared discovery.PreparedDecision) (db.SiteFix, error) {
	if len(prepared.OverlapWorkIDs) != 1 || prepared.OverlapWorkIDs[0] == uuid.Nil {
		return db.SiteFix{}, errors.New("Doctor evidence merge requires exactly one canonical overlap")
	}
	return b.q.GetCanonicalSiteFixByWorkSignature(ctx, db.GetCanonicalSiteFixByWorkSignatureParams{
		ProjectID: projectID, WorkSignatureID: prepared.OverlapWorkIDs[0],
	})
}

func (b *postgresDoctorSiteFixBackend) RecordEvidenceMerge(ctx context.Context, finding db.SeoDoctorFinding, canonical sitefix.CanonicalCandidate, prepared discovery.PreparedDecision, fix db.SiteFix, claim doctorSiteFixPreparationClaim) error {
	var findingEvidence any
	if err := json.Unmarshal(finding.Evidence, &findingEvidence); err != nil {
		return fmt.Errorf("invalid Doctor finding evidence: %w", err)
	}
	snapshot, err := json.Marshal(map[string]any{
		"finding_evidence":               findingEvidence,
		"candidate_evidence_fingerprint": canonical.Candidate.EvidenceFingerprint,
	})
	if err != nil {
		return err
	}
	_, err = b.q.CreateCanonicalSiteFixEvidenceMerge(ctx, db.CreateCanonicalSiteFixEvidenceMergeParams{
		ID: uuid.New(), ProjectID: finding.ProjectID, CandidateID: canonical.ID,
		ArbitrationDecisionID: prepared.ID, SiteFixID: fix.ID,
		DoctorFindingID: finding.ID, FindingKind: finding.FindingKind,
		EvidenceFingerprint: canonical.Candidate.EvidenceFingerprint, EvidenceSnapshot: snapshot,
		ExpectedFindingUpdatedAt: finding.UpdatedAt, LeaseToken: claim.LeaseToken,
		ExactSignatureHash: claim.ExactSignatureHash,
	})
	return normalizeDoctorSiteFixPreparationMutationError(err)
}

func (b *postgresDoctorSiteFixBackend) LoadSiteFix(ctx context.Context, projectID, siteFixID uuid.UUID) (db.SiteFix, error) {
	return b.q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: siteFixID, ProjectID: projectID})
}

type postgresDoctorSiteFixPreparationManager struct {
	q               *db.Queries
	runtimeProvider llm.Provider
	requestedModel  string
	leaseTTL        time.Duration
	resultTTL       time.Duration
	waitTimeout     time.Duration
	pollInterval    time.Duration
}

type doctorSiteFixRuntimeAuthorityProvider interface {
	RuntimeAuthorityFingerprint(context.Context, llm.CompletionPurpose) (string, error)
}

func (m *postgresDoctorSiteFixPreparationManager) runtimeAuthorityFingerprint(ctx context.Context) (string, error) {
	if provider, ok := m.runtimeProvider.(doctorSiteFixRuntimeAuthorityProvider); ok {
		return provider.RuntimeAuthorityFingerprint(ctx, llm.PurposeSiteFix)
	}
	identity := fmt.Sprintf("%T|%s|%s", m.runtimeProvider, strings.TrimSpace(m.requestedModel), llm.PurposeSiteFix)
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(identity))), nil
}

func (m *postgresDoctorSiteFixPreparationManager) Claim(ctx context.Context, projectID uuid.UUID, canonical sitefix.CanonicalCandidate) (doctorSiteFixPreparationClaim, error) {
	if m == nil || m.q == nil {
		return doctorSiteFixPreparationClaim{}, errors.New("Doctor Site Fix preparation store unavailable")
	}
	token := uuid.New()
	runtimeAuthority, err := m.runtimeAuthorityFingerprint(ctx)
	if err != nil {
		return doctorSiteFixPreparationClaim{}, normalizeDoctorSiteFixPreparationMutationError(err)
	}
	row, err := m.q.ClaimDoctorSiteFixPreparationLease(ctx, db.ClaimDoctorSiteFixPreparationLeaseParams{
		ProjectID: projectID, ExactSignatureHash: canonical.Identity.ExactSignatureHash,
		LeaseToken: token, RuntimeAuthorityFingerprint: runtimeAuthority,
		LeaderCandidateID: canonical.ID, LeaseTtlSeconds: durationSeconds(m.leaseTTL, 120),
	})
	if err != nil {
		return doctorSiteFixPreparationClaim{}, err
	}
	leaseTTLSeconds := durationSeconds(m.leaseTTL, 120)
	return doctorSiteFixPreparationClaim{
		ProjectID: row.ProjectID, ExactSignatureHash: row.ExactSignatureHash,
		CandidateID: canonical.ID, EvidenceFingerprint: canonical.Candidate.EvidenceFingerprint,
		LeaseToken: row.LeaseToken, RuntimeAuthorityFingerprint: runtimeAuthority,
		ExpiresAt: row.LeaseExpiresAt.Time, LeaseTTLSeconds: leaseTTLSeconds, Leader: row.IsLeader,
	}, nil
}

func (m *postgresDoctorSiteFixPreparationManager) Wait(ctx context.Context, claim doctorSiteFixPreparationClaim) (discovery.PreparedDecision, error) {
	waitTimeout := m.waitTimeout
	if waitTimeout <= 0 {
		waitTimeout = 30 * time.Second
	}
	pollInterval := m.pollInterval
	if pollInterval <= 0 {
		pollInterval = 100 * time.Millisecond
	}
	waitCtx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		row, err := m.q.GetDoctorSiteFixPreparationLease(waitCtx, db.GetDoctorSiteFixPreparationLeaseParams{
			ProjectID: claim.ProjectID, ExactSignatureHash: claim.ExactSignatureHash,
		})
		if err != nil {
			return discovery.PreparedDecision{}, err
		}
		switch row.Status {
		case "failed":
			return discovery.PreparedDecision{}, errDoctorSiteFixPreparationReclaim
		case "preparing":
			if !row.LeaseExpiresAt.Valid || !row.LeaseExpiresAt.Time.After(time.Now()) {
				return discovery.PreparedDecision{}, errDoctorSiteFixPreparationReclaim
			}
		case "completed":
			prepared, loadErr := m.loadCompletedDecision(waitCtx, claim)
			if errors.Is(loadErr, pgx.ErrNoRows) {
				invalidateCtx, invalidateCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				_, _ = m.q.InvalidateDoctorSiteFixPreparationLease(invalidateCtx, db.InvalidateDoctorSiteFixPreparationLeaseParams{
					ProjectID: claim.ProjectID, ExactSignatureHash: claim.ExactSignatureHash, ObservedLeaseToken: row.LeaseToken,
				})
				invalidateCancel()
				return discovery.PreparedDecision{}, errDoctorSiteFixPreparationReclaim
			}
			return prepared, loadErr
		}
		select {
		case <-waitCtx.Done():
			return discovery.PreparedDecision{}, ErrDoctorSiteFixCreateBusy
		case <-ticker.C:
		}
	}
}

func (m *postgresDoctorSiteFixPreparationManager) loadCompletedDecision(ctx context.Context, claim doctorSiteFixPreparationClaim) (discovery.PreparedDecision, error) {
	rulesVersion := discovery.ArbitrationRulesVersionV1
	if config, err := m.q.GetDiscoveryArbitrationConfig(ctx, claim.ProjectID); err == nil && strings.TrimSpace(config.RulesVersion) != "" {
		rulesVersion = config.RulesVersion
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return discovery.PreparedDecision{}, err
	}
	row, err := m.q.GetCompletedDoctorSiteFixPreparationDecision(ctx, db.GetCompletedDoctorSiteFixPreparationDecisionParams{
		ProjectID: claim.ProjectID, ExactSignatureHash: claim.ExactSignatureHash,
		CandidateID: claim.CandidateID, EvidenceFingerprint: claim.EvidenceFingerprint,
		RuntimeAuthorityFingerprint: claim.RuntimeAuthorityFingerprint,
		RulesVersion:                rulesVersion, PromptVersion: discovery.SemanticPromptVersionV1,
	})
	if err != nil {
		return discovery.PreparedDecision{}, err
	}
	return preparationDecisionFromDB(row)
}

func (m *postgresDoctorSiteFixPreparationManager) Validate(ctx context.Context, claim doctorSiteFixPreparationClaim) (doctorSiteFixPreparationClaim, error) {
	row, err := m.q.ValidateDoctorSiteFixPreparationLease(ctx, db.ValidateDoctorSiteFixPreparationLeaseParams{
		ProjectID: claim.ProjectID, ExactSignatureHash: claim.ExactSignatureHash,
		LeaseToken: claim.LeaseToken, LeaseTtlSeconds: durationSeconds(m.leaseTTL, 120),
	})
	if err != nil {
		return doctorSiteFixPreparationClaim{}, err
	}
	claim.ExpiresAt = row.LeaseExpiresAt.Time
	return claim, nil
}

func (m *postgresDoctorSiteFixPreparationManager) Complete(ctx context.Context, claim doctorSiteFixPreparationClaim, prepared discovery.PreparedDecision) error {
	resultTTL := m.resultTTL
	if prepared.Disposition == discovery.DispositionProviderFailure {
		resultTTL = 15 * time.Second
	}
	resolvedProvider, resolvedModel := prepared.Provider, prepared.Model
	_, err := m.q.CompleteDoctorSiteFixPreparationLease(ctx, db.CompleteDoctorSiteFixPreparationLeaseParams{
		ProjectID: claim.ProjectID, ExactSignatureHash: claim.ExactSignatureHash,
		LeaseToken: claim.LeaseToken, ArbitrationDecisionID: pgtype.UUID{Bytes: prepared.ID, Valid: true},
		ResolvedProvider: &resolvedProvider, ResolvedModel: &resolvedModel,
		ResultTtlSeconds: durationSeconds(resultTTL, 300),
	})
	return normalizeDoctorSiteFixPreparationMutationError(err)
}

func (m *postgresDoctorSiteFixPreparationManager) Invalidate(ctx context.Context, claim doctorSiteFixPreparationClaim) error {
	_, err := m.q.InvalidateDoctorSiteFixPreparationLease(ctx, db.InvalidateDoctorSiteFixPreparationLeaseParams{
		ProjectID: claim.ProjectID, ExactSignatureHash: claim.ExactSignatureHash, ObservedLeaseToken: claim.LeaseToken,
	})
	return normalizeDoctorSiteFixPreparationMutationError(err)
}

func (m *postgresDoctorSiteFixPreparationManager) Fail(ctx context.Context, claim doctorSiteFixPreparationClaim, code string) error {
	failCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	errorCode := strings.TrimSpace(code)
	_, err := m.q.FailDoctorSiteFixPreparationLease(failCtx, db.FailDoctorSiteFixPreparationLeaseParams{
		ProjectID: claim.ProjectID, ExactSignatureHash: claim.ExactSignatureHash,
		LeaseToken: claim.LeaseToken, ErrorCode: &errorCode,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

func durationSeconds(value time.Duration, fallback int32) int32 {
	if value <= 0 {
		return fallback
	}
	seconds := int64(value / time.Second)
	if seconds < 1 {
		return 1
	}
	if seconds > int64(^uint32(0)>>1) {
		return fallback
	}
	return int32(seconds)
}

func normalizeDoctorSiteFixPreparationMutationError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errDoctorSiteFixPreparationLost
	}
	return err
}

func preparationDecisionFromDB(row db.DiscoveryArbitrationDecision) (discovery.PreparedDecision, error) {
	owner := discovery.Owner("")
	if row.Owner != nil {
		owner = discovery.Owner(*row.Owner)
	}
	return discovery.PreparedDecision{
		ID: row.ID, ProjectID: row.ProjectID, CandidateID: row.CandidateID,
		CandidateVersion: row.CandidateVersion, Disposition: discovery.ArbitrationDisposition(row.Disposition),
		Decision: discovery.DecisionKind(row.Decision), Owner: owner, Reason: row.Reason,
		ExactSignatureHash: row.ExactSignatureHash, EvidenceFingerprint: row.EvidenceFingerprint,
		RulesVersion: row.RulesVersion, PromptVersion: row.PromptVersion,
		Provider: row.Provider, Model: row.Model, Status: discovery.ArbitrationStatus(row.Status),
	}, nil
}

func (s *postgresDoctorSiteFixService) List(ctx context.Context, projectID uuid.UUID, status *string) ([]DoctorSiteFixResponse, error) {
	if s == nil || s.q == nil {
		return nil, errors.New("canonical Site Fix database unavailable")
	}
	params := db.ListCanonicalSiteFixesParams{ProjectID: projectID, Status: status}
	fixes, err := s.q.ListCanonicalSiteFixes(ctx, params)
	if err != nil {
		return nil, err
	}
	applications, err := s.q.ListLatestCanonicalSiteFixApplications(ctx, db.ListLatestCanonicalSiteFixApplicationsParams{ProjectID: projectID, Status: status})
	if err != nil {
		return nil, err
	}
	verifications, err := s.q.ListCanonicalSiteFixVerificationsForList(ctx, db.ListCanonicalSiteFixVerificationsForListParams{ProjectID: projectID, Status: status})
	if err != nil {
		return nil, err
	}
	aliases, err := s.q.ListCanonicalSiteFixAliasesForList(ctx, db.ListCanonicalSiteFixAliasesForListParams{ProjectID: projectID, Status: status})
	if err != nil {
		return nil, err
	}
	applicationsByFix := make(map[uuid.UUID]db.SiteChangeApplication, len(applications))
	for _, application := range applications {
		if application.SiteFixID.Valid {
			applicationsByFix[application.SiteFixID.Bytes] = application
		}
	}
	verificationsByFix := make(map[uuid.UUID][]db.SiteFixVerification)
	for _, verification := range verifications {
		verificationsByFix[verification.SiteFixID] = append(verificationsByFix[verification.SiteFixID], verification)
	}
	aliasesByFix := make(map[uuid.UUID][]DoctorSiteFixLegacyAlias)
	for _, alias := range aliases {
		aliasesByFix[alias.CanonicalObjectID] = append(aliasesByFix[alias.CanonicalObjectID], DoctorSiteFixLegacyAlias{ObjectType: alias.LegacyObjectType, ObjectID: alias.LegacyObjectID})
	}
	responses := make([]DoctorSiteFixResponse, 0, len(fixes))
	for _, fix := range fixes {
		response := DoctorSiteFixResponse{SiteFix: fix, Verifications: verificationsByFix[fix.ID], LegacyAliases: aliasesByFix[fix.ID]}
		if response.Verifications == nil {
			response.Verifications = []db.SiteFixVerification{}
		}
		if response.LegacyAliases == nil {
			response.LegacyAliases = []DoctorSiteFixLegacyAlias{}
		}
		if application, ok := applicationsByFix[fix.ID]; ok {
			applicationCopy := application
			response.Application = &applicationCopy
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func (s *postgresDoctorSiteFixService) ListDoctorLinks(ctx context.Context, projectID uuid.UUID) ([]DoctorSiteFixResponse, error) {
	if s == nil || s.q == nil {
		return nil, errors.New("canonical Site Fix database unavailable")
	}
	fixes, err := s.q.ListCurrentDoctorSiteFixLinks(ctx, projectID)
	if err != nil {
		return nil, err
	}
	applications, err := s.q.ListCurrentDoctorSiteFixLinkApplications(ctx, projectID)
	if err != nil {
		return nil, err
	}
	applicationsByFix := make(map[uuid.UUID]db.SiteChangeApplication, len(applications))
	for _, application := range applications {
		if application.SiteFixID.Valid {
			applicationsByFix[application.SiteFixID.Bytes] = application
		}
	}
	responses := make([]DoctorSiteFixResponse, 0, len(fixes))
	for _, fix := range fixes {
		response := DoctorSiteFixResponse{SiteFix: fix, Verifications: []db.SiteFixVerification{}, LegacyAliases: []DoctorSiteFixLegacyAlias{}}
		if application, ok := applicationsByFix[fix.ID]; ok {
			applicationCopy := application
			response.Application = &applicationCopy
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func (s *postgresDoctorSiteFixService) Get(ctx context.Context, projectID, fixID uuid.UUID) (DoctorSiteFixResponse, error) {
	if s == nil || s.q == nil {
		return DoctorSiteFixResponse{}, errors.New("canonical Site Fix database unavailable")
	}
	fix, err := s.q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
	if err != nil {
		return DoctorSiteFixResponse{}, err
	}
	return s.loadResponse(ctx, fix)
}

func (s *postgresDoctorSiteFixService) DismissDoctorLink(ctx context.Context, projectID, fixID uuid.UUID, dismissedBy string, dismissedAt time.Time) (db.SiteFix, error) {
	if s == nil || s.q == nil {
		return db.SiteFix{}, errors.New("canonical Site Fix database unavailable")
	}
	fix, err := s.q.DismissCanonicalSiteFixDoctorLink(ctx, db.DismissCanonicalSiteFixDoctorLinkParams{
		DismissedAt: pgtype.Timestamptz{Time: dismissedAt.UTC(), Valid: true},
		DismissedBy: strings.TrimSpace(dismissedBy),
		ID:          fixID,
		ProjectID:   projectID,
	})
	if err != nil {
		return db.SiteFix{}, err
	}
	return fix, nil
}

func (s *postgresDoctorSiteFixService) Approve(ctx context.Context, projectID, fixID uuid.UUID, approvedAt time.Time) (DoctorSiteFixResponse, error) {
	if s == nil || s.q == nil || s.approvalTx == nil {
		return DoctorSiteFixResponse{}, errors.New("canonical Site Fix database unavailable")
	}
	var fix db.SiteFix
	err := s.approvalTx.Run(ctx, func(store canonicalSiteFixApprovalMeasurementStore) error {
		var txErr error
		fix, txErr = approveCanonicalSiteFixWithMeasurementIdempotently(ctx, store, projectID, fixID, approvedAt)
		return txErr
	})
	if err != nil {
		return DoctorSiteFixResponse{}, err
	}
	return s.loadResponse(ctx, fix)
}

func approveCanonicalSiteFixWithMeasurementIdempotently(ctx context.Context, store canonicalSiteFixApprovalMeasurementStore, projectID, fixID uuid.UUID, approvedAt time.Time) (db.SiteFix, error) {
	current, err := store.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
	if err != nil {
		return db.SiteFix{}, err
	}
	alreadyApproved := canonicalSiteFixRevisionAlreadyApproved(current)
	if !alreadyApproved && current.Status != "proposed" {
		return db.SiteFix{}, ErrDoctorSiteFixTransitionConflict
	}

	planCutoff := approvedAt.UTC()
	if alreadyApproved && current.ApprovedAt.Valid {
		planCutoff = current.ApprovedAt.Time.UTC()
	}
	var frozen sitefix.FrozenSiteFixMeasurementPlan
	if current.MeasurementPolicy == "measurement_required" {
		frozen, err = sitefix.RecoverApprovedSiteFixMeasurementPlan(storedSiteFixMeasurementInput(current), planCutoff)
		if err != nil {
			return db.SiteFix{}, err
		}
	}
	transitioned := false
	if !alreadyApproved {
		if _, err = store.ApproveCanonicalSiteFix(ctx, db.ApproveCanonicalSiteFixParams{
			SiteFixID: fixID, ProjectID: projectID, ApprovedAt: pgutil.TS(approvedAt.UTC()),
		}); err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return db.SiteFix{}, err
			}
			current, err = store.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
			if err != nil {
				return db.SiteFix{}, err
			}
			if !canonicalSiteFixRevisionAlreadyApproved(current) {
				return db.SiteFix{}, ErrDoctorSiteFixTransitionConflict
			}
		} else {
			transitioned = true
		}
	}
	if current.MeasurementPolicy == "measurement_required" {
		if _, err = store.CreateSiteFixMeasurement(ctx, createSiteFixMeasurementParams(current, frozen, "approval-required-v1:"+fixID.String())); err != nil {
			return db.SiteFix{}, err
		}
	}
	if !transitioned {
		return current, nil
	}
	return store.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
}

func storedSiteFixMeasurementInput(fix db.SiteFix) sitefix.StoredSiteFixMeasurementInput {
	return sitefix.StoredSiteFixMeasurementInput{
		TargetURLs: fix.TargetUrls, ProposedFix: fix.ProposedFix, EvidenceSnapshot: fix.EvidenceSnapshot,
		FixType: fix.FixType, ImpactMode: fix.ImpactMode, MeasurementPolicy: fix.MeasurementPolicy,
		ClassifierVersion: fix.ClassifierVersion, DecisionOrigin: fix.DecisionOrigin, DecisionConfidence: fix.DecisionConfidence,
		GrowthHypothesis: fix.GrowthHypothesis, PrimaryMetric: fix.PrimaryMetric, SecondaryMetrics: fix.SecondaryMetrics,
		MeasurementPolicyVersion: fix.MeasurementPolicyVersion, MeasurementPolicySnapshot: fix.MeasurementPolicySnapshot,
		MeasurementPlanSnapshot: fix.MeasurementPlanSnapshot,
	}
}

func createSiteFixMeasurementParams(fix db.SiteFix, frozen sitefix.FrozenSiteFixMeasurementPlan, idempotencyKey string) db.CreateSiteFixMeasurementParams {
	return db.CreateSiteFixMeasurementParams{
		ID: uuid.New(), ProjectID: fix.ProjectID, SiteFixID: fix.ID, CreationIdempotencyKey: idempotencyKey,
		TargetUrl: frozen.TargetURL, NormalizedTargetUrl: frozen.NormalizedTargetURL, TargetQuery: frozen.TargetQuery,
		TargetIdentity: frozen.TargetIdentity, FixType: frozen.FixType, ImpactMode: frozen.ImpactMode,
		ClassifierVersion: frozen.ClassifierVersion, DecisionOrigin: frozen.DecisionOrigin, DecisionConfidence: frozen.DecisionConfidence,
		ProspectiveObservation: frozen.ProspectiveObservation, GrowthHypothesis: frozen.GrowthHypothesis, PrimaryMetric: frozen.PrimaryMetric,
		SecondaryMetrics: frozen.SecondaryMetrics, MeasurementPolicyVersion: frozen.MeasurementPolicyVersion,
		MeasurementPolicySnapshot: frozen.MeasurementPolicySnapshot, BaselineWindow: frozen.BaselineWindow,
		BaselineSnapshot: frozen.BaselineSnapshot, BaselineStatus: frozen.BaselineStatus, Status: frozen.Status,
		AttributionConfidence: frozen.AttributionConfidence,
	}
}

func (s *postgresDoctorSiteFixService) OptInMeasurement(ctx context.Context, projectID, fixID uuid.UUID, optedInAt time.Time) (DoctorSiteFixMeasurementOptInResponse, error) {
	if s == nil || s.measurementOptInTx == nil {
		return DoctorSiteFixMeasurementOptInResponse{}, errors.New("canonical Site Fix measurement database unavailable")
	}
	var response DoctorSiteFixMeasurementOptInResponse
	err := s.measurementOptInTx.Run(ctx, func(store canonicalSiteFixMeasurementOptInStore) error {
		var txErr error
		response, txErr = optInCanonicalSiteFixMeasurementIdempotently(ctx, store, projectID, fixID, optedInAt.UTC())
		return txErr
	})
	return response, err
}

func optInCanonicalSiteFixMeasurementIdempotently(ctx context.Context, store canonicalSiteFixMeasurementOptInStore, projectID, fixID uuid.UUID, optedInAt time.Time) (DoctorSiteFixMeasurementOptInResponse, error) {
	fix, err := store.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
	if err != nil {
		return DoctorSiteFixMeasurementOptInResponse{}, err
	}
	if fix.Status != "verified" || fix.MeasurementPolicy != "measurement_optional" {
		return DoctorSiteFixMeasurementOptInResponse{}, ErrDoctorSiteFixMeasurementOptInConflict
	}
	frozen, err := sitefix.RecoverProspectiveSiteFixMeasurementPlan(storedSiteFixMeasurementInput(fix), optedInAt)
	if err != nil {
		return DoctorSiteFixMeasurementOptInResponse{}, err
	}
	measurement, err := store.CreateSiteFixMeasurement(ctx, createSiteFixMeasurementParams(fix, frozen, "verified-opt-in-v1:"+fixID.String()))
	if err != nil {
		return DoctorSiteFixMeasurementOptInResponse{}, err
	}
	handoff, err := store.EnqueueSiteFixMeasurementHandoff(ctx, db.EnqueueSiteFixMeasurementHandoffParams{
		ID: uuid.New(), ProjectID: projectID, SiteFixID: fixID, MeasurementGeneration: measurement.MeasurementGeneration,
		IdempotencyKey: fmt.Sprintf("activate:%s:%d", fixID, measurement.MeasurementGeneration), MaxAttempts: 3,
		NextAttemptAt: pgutil.TS(optedInAt), OccurredAt: pgutil.TS(optedInAt),
	})
	if err != nil {
		return DoctorSiteFixMeasurementOptInResponse{}, err
	}
	return publicSiteFixMeasurementOptInResponse(fixID, measurement, handoff), nil
}

func publicSiteFixMeasurementOptInResponse(fixID uuid.UUID, measurement db.SiteFixMeasurement, handoff db.SiteFixMeasurementHandoffOutbox) DoctorSiteFixMeasurementOptInResponse {
	deepLink := siteFixResultsDeepLink(measurement.ProjectID, measurement.ID)
	handoffStatus := siteFixMeasurementHandoffStatus(handoff, nil)
	if handoff.Status == "" {
		handoffStatus = "pending"
	}
	return DoctorSiteFixMeasurementOptInResponse{
		SiteFixID: fixID,
		Measurement: DoctorSiteFixMeasurementPublic{
			ID: measurement.ID, MeasurementGeneration: measurement.MeasurementGeneration, Status: measurement.Status,
			ProspectiveObservation: measurement.ProspectiveObservation, BaselineStatus: measurement.BaselineStatus,
			AttributionConfidence: measurement.AttributionConfidence, ResultsDeepLink: &deepLink,
		},
		Handoff: DoctorSiteFixMeasurementHandoffPublic{Status: handoffStatus},
	}
}

func approveCanonicalSiteFixIdempotently(ctx context.Context, store canonicalSiteFixApprovalStore, projectID, fixID uuid.UUID, approvedAt time.Time) (db.SiteFix, error) {
	current, err := store.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
	if err != nil {
		return db.SiteFix{}, err
	}
	if canonicalSiteFixRevisionAlreadyApproved(current) {
		return current, nil
	}
	if current.Status != "proposed" {
		return db.SiteFix{}, ErrDoctorSiteFixTransitionConflict
	}
	_, err = store.ApproveCanonicalSiteFix(ctx, db.ApproveCanonicalSiteFixParams{
		SiteFixID: fixID, ProjectID: projectID,
		ApprovedAt: pgtype.Timestamptz{Time: approvedAt.UTC(), Valid: true},
	})
	if err == nil {
		return store.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.SiteFix{}, err
	}
	current, reloadErr := store.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
	if reloadErr != nil {
		return db.SiteFix{}, reloadErr
	}
	if canonicalSiteFixRevisionAlreadyApproved(current) {
		return current, nil
	}
	return db.SiteFix{}, ErrDoctorSiteFixTransitionConflict
}

func canonicalSiteFixRevisionAlreadyApproved(fix db.SiteFix) bool {
	if !fix.ApprovedAt.Valid {
		return false
	}
	switch fix.Status {
	case "approved", "preparing", "ready_to_apply", "applying", "awaiting_deploy", "verifying", "failed_retryable", "reopened", "verified":
		return true
	default:
		return false
	}
}

func (s *postgresDoctorSiteFixService) loadResponse(ctx context.Context, fix db.SiteFix) (DoctorSiteFixResponse, error) {
	response := DoctorSiteFixResponse{SiteFix: fix, Verifications: []db.SiteFixVerification{}, LegacyAliases: []DoctorSiteFixLegacyAlias{}}
	application, err := s.q.GetLatestCanonicalSiteFixApplication(ctx, db.GetLatestCanonicalSiteFixApplicationParams{
		ProjectID: fix.ProjectID, SiteFixID: pgtype.UUID{Bytes: fix.ID, Valid: true},
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return DoctorSiteFixResponse{}, err
	}
	if err == nil {
		response.Application = &application
	}
	response.Verifications, err = s.q.ListCanonicalSiteFixVerifications(ctx, db.ListCanonicalSiteFixVerificationsParams{ProjectID: fix.ProjectID, SiteFixID: fix.ID})
	if err != nil {
		return DoctorSiteFixResponse{}, err
	}
	aliases, err := s.q.ListCanonicalSiteFixAliasesForFix(ctx, db.ListCanonicalSiteFixAliasesForFixParams{ProjectID: fix.ProjectID, CanonicalObjectID: fix.ID})
	if err != nil {
		return DoctorSiteFixResponse{}, err
	}
	for _, alias := range aliases {
		response.LegacyAliases = append(response.LegacyAliases, DoctorSiteFixLegacyAlias{ObjectType: alias.LegacyObjectType, ObjectID: alias.LegacyObjectID})
	}
	measurement, measurementErr := s.q.GetLatestSiteFixMeasurementForFix(ctx, db.GetLatestSiteFixMeasurementForFixParams{ProjectID: fix.ProjectID, SiteFixID: fix.ID})
	if measurementErr != nil && !errors.Is(measurementErr, pgx.ErrNoRows) {
		return DoctorSiteFixResponse{}, measurementErr
	}
	var handoff db.SiteFixMeasurementHandoffOutbox
	handoffErr := error(pgx.ErrNoRows)
	if measurementErr == nil {
		handoff, handoffErr = s.q.GetLatestSiteFixMeasurementHandoff(ctx, db.GetLatestSiteFixMeasurementHandoffParams{ProjectID: fix.ProjectID, MeasurementID: measurement.ID})
		if handoffErr != nil && !errors.Is(handoffErr, pgx.ErrNoRows) {
			return DoctorSiteFixResponse{}, handoffErr
		}
	}
	response.MeasurementSummary, response.MeasurementHandoffStatus = doctorSiteFixMeasurementSummary(fix, measurement, measurementErr, handoff, handoffErr)
	return response, nil
}

func doctorSiteFixMeasurementSummary(fix db.SiteFix, measurement db.SiteFixMeasurement, measurementErr error, handoff db.SiteFixMeasurementHandoffOutbox, handoffErr error) (*DoctorSiteFixMeasurementPublic, string) {
	if errors.Is(measurementErr, pgx.ErrNoRows) {
		if fix.MeasurementPolicy == "verification_only" {
			return nil, "not_applicable"
		}
		return nil, "not_started"
	}
	if measurementErr != nil {
		return nil, "not_started"
	}
	deepLink := siteFixResultsDeepLink(measurement.ProjectID, measurement.ID)
	summary := &DoctorSiteFixMeasurementPublic{
		ID: measurement.ID, MeasurementGeneration: measurement.MeasurementGeneration, Status: measurement.Status,
		ProspectiveObservation: measurement.ProspectiveObservation, BaselineStatus: measurement.BaselineStatus,
		AttributionConfidence: measurement.AttributionConfidence, ResultsDeepLink: &deepLink,
	}
	return summary, siteFixMeasurementHandoffStatus(handoff, handoffErr)
}

func siteFixResultsDeepLink(projectID, measurementID uuid.UUID) string {
	return fmt.Sprintf("/projects/%s/results?source_type=site_fix&measurement=%s", projectID, measurementID)
}

func (s *Server) doctorSiteFixService() DoctorSiteFixService {
	if s.SiteFixes != nil {
		return s.SiteFixes
	}
	if s.Pool == nil || s.Q == nil {
		return nil
	}
	return NewDoctorSiteFixService(s.Pool, s.Q, s.LLM, s.Env.TokenGateModel)
}

func (s *Server) doctorSiteFixLifecycleService() DoctorSiteFixLifecycleService {
	if s.SiteFixLifecycle != nil {
		return s.SiteFixLifecycle
	}
	if s.Pool == nil || s.Q == nil {
		return nil
	}
	sourceLoader := s.siteFixRepositorySourceLoader()
	return &postgresDoctorSiteFixLifecycleService{
		pool: s.Pool,
		q:    s.Q,
		apply: sitefix.ApplyService{
			Store:          sitefix.PostgresApplyStore{Pool: s.Pool, Q: s.Q},
			SourceLoader:   sourceLoader,
			SourceSelector: sitefix.LLMRepositorySourceSelector{Provider: s.LLM, Model: s.Env.TokenGateModel},
			Generator:      sitefix.LLMApplicationGenerator{Provider: s.LLM, Model: s.Env.TokenGateModel},
			Verifier:       sitefix.LLMPatchGroundingVerifier{Provider: s.LLM, Model: s.Env.TokenGateModel},
		},
	}
}

func (s *Server) doctorSiteFixLifecycleServiceForReadiness(projectID uuid.UUID, target githubPRReadinessTarget) (DoctorSiteFixLifecycleService, error) {
	if s.SiteFixLifecycle != nil {
		return s.SiteFixLifecycle, nil
	}
	if s.Pool == nil || s.Q == nil {
		return nil, errors.New("canonical Site Fix lifecycle service unavailable")
	}
	sourceLoader, err := s.siteFixRepositorySourceLoaderForReadiness(projectID, target)
	if err != nil {
		return nil, err
	}
	return &postgresDoctorSiteFixLifecycleService{
		pool: s.Pool,
		q:    s.Q,
		apply: sitefix.ApplyService{
			Store:          sitefix.PostgresApplyStore{Pool: s.Pool, Q: s.Q},
			SourceLoader:   sourceLoader,
			SourceSelector: sitefix.LLMRepositorySourceSelector{Provider: s.LLM, Model: s.Env.TokenGateModel},
			Generator:      sitefix.LLMApplicationGenerator{Provider: s.LLM, Model: s.Env.TokenGateModel},
			Verifier:       sitefix.LLMPatchGroundingVerifier{Provider: s.LLM, Model: s.Env.TokenGateModel},
		},
	}, nil
}

func (s *Server) createDoctorSiteFix(w http.ResponseWriter, r *http.Request) {
	projectID, findingID, ok := s.seoDoctorIDs(w, r, "findingID")
	if !ok {
		return
	}
	s.createDoctorSiteFixForIDs(w, r, projectID, findingID)
}

func (s *Server) createDoctorSiteFixForIDs(w http.ResponseWriter, r *http.Request, projectID, findingID uuid.UUID) {
	service := s.doctorSiteFixService()
	if service == nil {
		writeErr(w, http.StatusInternalServerError, "canonical Site Fix service unavailable")
		return
	}
	fix, created, err := service.CreateFromFinding(r.Context(), projectID, findingID)
	if err != nil {
		s.writeDoctorSiteFixError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, fix)
}

func (s *Server) listDoctorSiteFixes(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	service := s.doctorSiteFixService()
	if service == nil {
		writeErr(w, http.StatusInternalServerError, "canonical Site Fix service unavailable")
		return
	}
	var status *string
	if value := strings.TrimSpace(r.URL.Query().Get("status")); value != "" {
		status = &value
	}
	fixes, err := service.List(r.Context(), projectID, status)
	if err != nil {
		s.writeDoctorSiteFixError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(fixes))
}

func (s *Server) listCurrentDoctorSiteFixLinks(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	service := s.doctorSiteFixService()
	if service == nil {
		writeErr(w, http.StatusInternalServerError, "canonical Site Fix service unavailable")
		return
	}
	links, err := service.ListDoctorLinks(r.Context(), projectID)
	if err != nil {
		s.writeDoctorSiteFixError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(links))
}

func (s *Server) getDoctorSiteFix(w http.ResponseWriter, r *http.Request) {
	projectID, fixID, ok := s.seoDoctorIDs(w, r, "fixID")
	if !ok {
		return
	}
	service := s.doctorSiteFixService()
	if service == nil {
		writeErr(w, http.StatusInternalServerError, "canonical Site Fix service unavailable")
		return
	}
	fix, err := service.Get(r.Context(), projectID, fixID)
	if err != nil {
		s.writeDoctorSiteFixError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, fix)
}

func (s *Server) dismissDoctorSiteFixLink(w http.ResponseWriter, r *http.Request) {
	projectID, fixID, ok := s.seoDoctorIDs(w, r, "fixID")
	if !ok {
		return
	}
	service := s.doctorSiteFixService()
	if service == nil {
		writeErr(w, http.StatusInternalServerError, "canonical Site Fix service unavailable")
		return
	}
	actor := strings.TrimSpace(s.ownerID(r))
	if actor == "" {
		writeErr(w, http.StatusForbidden, "project owner required")
		return
	}
	fix, err := service.DismissDoctorLink(r.Context(), projectID, fixID, actor, time.Now().UTC())
	if err != nil {
		s.writeDoctorSiteFixError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, fix)
}

func (s *Server) approveDoctorSiteFix(w http.ResponseWriter, r *http.Request) {
	projectID, fixID, ok := s.seoDoctorIDs(w, r, "fixID")
	if !ok {
		return
	}
	target, err := s.authorizeGitHubPRMutation(r.Context(), projectID)
	if err != nil {
		s.writeDoctorSiteFixError(w, err)
		return
	}
	service := s.doctorSiteFixService()
	if service == nil {
		writeErr(w, http.StatusInternalServerError, "canonical Site Fix service unavailable")
		return
	}
	if _, err := service.Approve(r.Context(), projectID, fixID, time.Now().UTC()); err != nil {
		s.writeDoctorSiteFixError(w, err)
		return
	}
	result, err := s.runCanonicalSiteFixPR(r.Context(), projectID, fixID, target)
	if err != nil {
		s.writeDoctorSiteFixError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) optInDoctorSiteFixMeasurement(w http.ResponseWriter, r *http.Request) {
	projectID, fixID, ok := s.seoDoctorIDs(w, r, "fixID")
	if !ok {
		return
	}
	if r.Body != nil {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 4097))
		if err != nil || len(raw) > 4096 {
			writeErr(w, http.StatusUnprocessableEntity, "measurement opt-in request is invalid")
			return
		}
		if len(strings.TrimSpace(string(raw))) > 0 {
			var object map[string]json.RawMessage
			if json.Unmarshal(raw, &object) != nil || object == nil || len(object) != 0 {
				writeErr(w, http.StatusUnprocessableEntity, "measurement opt-in does not accept a mutable plan")
				return
			}
		}
	}
	service := s.doctorSiteFixService()
	if service == nil {
		writeErr(w, http.StatusInternalServerError, "canonical Site Fix service unavailable")
		return
	}
	result, err := service.OptInMeasurement(r.Context(), projectID, fixID, time.Now().UTC())
	if err != nil {
		s.writeDoctorSiteFixError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) applyDoctorSiteFix(w http.ResponseWriter, r *http.Request) {
	projectID, fixID, ok := s.seoDoctorIDs(w, r, "fixID")
	if !ok {
		return
	}
	target, err := s.authorizeGitHubPRMutation(r.Context(), projectID)
	if err != nil {
		s.writeDoctorSiteFixError(w, err)
		return
	}
	result, err := s.runCanonicalSiteFixPR(r.Context(), projectID, fixID, target)
	if err != nil {
		s.writeDoctorSiteFixError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) runCanonicalSiteFixPR(ctx context.Context, projectID, fixID uuid.UUID, target githubPRReadinessTarget) (sitefix.ApplyResult, error) {
	if s.canonicalSiteFixPRRunner != nil {
		return s.canonicalSiteFixPRRunner(ctx, projectID, fixID, target)
	}
	return s.createCanonicalSiteFixPR(ctx, projectID, fixID, target)
}

func (s *Server) createCanonicalSiteFixPR(ctx context.Context, projectID, fixID uuid.UUID, target githubPRReadinessTarget) (sitefix.ApplyResult, error) {
	service, err := s.doctorSiteFixLifecycleServiceForReadiness(projectID, target)
	if err != nil {
		return sitefix.ApplyResult{}, err
	}
	result, err := runCanonicalSiteFixPRAttempts(
		ctx,
		func(callCtx context.Context) (sitefix.ApplyResult, error) {
			return service.Apply(callCtx, projectID, fixID)
		},
		func(callCtx context.Context, result sitefix.ApplyResult) (sitefix.ApplyResult, error) {
			if result.SiteFix.Status == "applying" && canReopenCanonicalSiteFixPRCreation(result.Application) {
				if s.Q == nil {
					return result, errors.New("canonical Site Fix database unavailable")
				}
				result.Application, err = s.Q.ReopenCanonicalSiteFixApply(callCtx, db.ReopenCanonicalSiteFixApplyParams{
					ProjectID: projectID, ApplicationID: result.Application.ID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true},
				})
				if err != nil {
					return result, canonicalDoctorLifecycleTransitionError(err)
				}
			}
			if result.Application.Status == "ready_for_pr" || result.Application.Status == "creating_pr" {
				return s.openCanonicalSiteFixGitHubPR(callCtx, result, target)
			}
			return result, validateCanonicalSiteFixPRResponse(result)
		},
	)
	if err != nil && !errors.Is(err, errCanonicalSiteFixFreshReprepare) {
		if downgradeErr := s.downgradeGitHubPRReadinessAfterMutationFailure(ctx, projectID, target, err); downgradeErr != nil {
			err = errors.Join(err, downgradeErr)
		}
	}
	return result, err
}

func runCanonicalSiteFixPRAttempts(
	ctx context.Context,
	apply func(context.Context) (sitefix.ApplyResult, error),
	open func(context.Context, sitefix.ApplyResult) (sitefix.ApplyResult, error),
) (sitefix.ApplyResult, error) {
	var result sitefix.ApplyResult
	for attempt := 0; attempt < 2; attempt++ {
		var err error
		result, err = apply(ctx)
		if err != nil {
			return result, err
		}
		result, err = open(ctx, result)
		if err == nil || !errors.Is(err, errCanonicalSiteFixFreshReprepare) {
			return result, err
		}
		if attempt == 1 {
			return result, err
		}
	}
	return result, errCanonicalSiteFixFreshReprepare
}

func canReopenCanonicalSiteFixPRCreation(app db.SiteChangeApplication) bool {
	if app.Status != "needs_follow_up" || (app.GithubPrUrl != nil && strings.TrimSpace(*app.GithubPrUrl) != "") ||
		(app.GithubPrState != nil && strings.TrimSpace(*app.GithubPrState) != "") {
		return false
	}
	if app.FailureReason == nil {
		return false
	}
	switch strings.TrimSpace(*app.FailureReason) {
	case "pr_interrupted", "publisher_branch_conflict", "prepared_patch_invalid", "publisher_unavailable", "github_pr_failed":
		return true
	default:
		return false
	}
}

func canonicalSiteFixPRClaimCollisionIsComplete(app db.SiteChangeApplication) bool {
	return app.Status != "creating_pr" && app.GithubPrUrl != nil && strings.TrimSpace(*app.GithubPrUrl) != ""
}

func validateCanonicalSiteFixPRResponse(result sitefix.ApplyResult) error {
	if result.Application.GithubPrUrl != nil && strings.TrimSpace(*result.Application.GithubPrUrl) != "" {
		return nil
	}
	if result.Application.Status == "creating_pr" {
		return fmt.Errorf("%w: repair PR creation is already in progress", sitefix.ErrLifecycleConflict)
	}
	return fmt.Errorf("%w: repair PR is not available for this Site Fix application", sitefix.ErrLifecycleConflict)
}

func (s *Server) openCanonicalSiteFixGitHubPR(ctx context.Context, result sitefix.ApplyResult, target githubPRReadinessTarget) (sitefix.ApplyResult, error) {
	if s.Q == nil {
		return result, errors.New("canonical Site Fix database unavailable")
	}
	prepared, err := sitefix.ParseRepositoryPreparedPatch(result.Application.PatchSnapshot)
	if err != nil {
		return result, err
	}
	if result.SiteFix.ProjectID == uuid.Nil || target.ConnectionID == uuid.Nil || !target.ExpectedUpdatedAt.Valid || strings.TrimSpace(target.token) == "" {
		return result, errors.New("checked GitHub readiness target is incomplete")
	}
	claimToken := uuid.New()
	if err := s.ensureGitHubPRTargetCurrent(ctx, result.SiteFix.ProjectID, target); err != nil {
		return result, err
	}
	claimed, err := s.Q.ClaimCanonicalSiteFixGitHubPR(ctx, db.ClaimCanonicalSiteFixGitHubPRParams{
		PrClaimToken: pgtype.UUID{Bytes: claimToken, Valid: true}, LeaseTtlSeconds: 300,
		ProjectID: result.SiteFix.ProjectID, ApplicationID: result.Application.ID,
		SiteFixID:             pgtype.UUID{Bytes: result.SiteFix.ID, Valid: true},
		PublisherConnectionID: target.ConnectionID, ExpectedConnectionUpdatedAt: target.ExpectedUpdatedAt,
		ExpectedRepoFullName: target.Repo, ExpectedBaseBranch: target.Branch,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return result, err
		}
		app, loadErr := s.Q.GetCanonicalSiteFixApplication(ctx, db.GetCanonicalSiteFixApplicationParams{ProjectID: result.SiteFix.ProjectID, ApplicationID: result.Application.ID, SiteFixID: pgtype.UUID{Bytes: result.SiteFix.ID, Valid: true}})
		if loadErr != nil {
			return result, canonicalDoctorLifecycleTransitionError(loadErr)
		}
		result.Application = app
		if canonicalSiteFixPRClaimCollisionIsComplete(app) {
			return reloadCanonicalSiteFixAfterPRObservation(ctx, s.Q, result)
		}
		return result, validateCanonicalSiteFixPRResponse(result)
	}
	result.Application = claimed
	prepared, err = sitefix.ParseRepositoryPreparedPatch(claimed.PatchSnapshot)
	if err != nil {
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, err)
	}
	if prepared.Repo != target.Repo || prepared.BaseBranch != target.Branch {
		return s.resetCanonicalSiteFixPRForReprepare(
			ctx, result, claimToken, "repository_target_changed",
			errors.New("prepared repository target no longer matches the configured publisher target"),
		)
	}
	workingBranch := siteFixRepositoryWorkingBranch(result.SiteFix.ID, claimed.ID)
	client := publisher.NewGitHubPRClient(target.token, target.Repo, target.Branch, s.Log)
	repositoryClient := &publisherSiteFixRepositoryClient{
		GitHubPRClient: client, token: target.token, repo: target.Repo,
		httpClient: &http.Client{Timeout: 30 * time.Second}, apiBase: "https://api.github.com",
	}
	renewClaim := func(callCtx context.Context) error {
		if currentErr := s.ensureGitHubPRTargetCurrent(callCtx, result.SiteFix.ProjectID, target); currentErr != nil {
			return currentErr
		}
		_, renewErr := s.Q.RenewCanonicalSiteFixGitHubPRClaim(callCtx, db.RenewCanonicalSiteFixGitHubPRClaimParams{
			LeaseTtlSeconds: 300, ProjectID: result.SiteFix.ProjectID, ApplicationID: result.Application.ID,
			SiteFixID: pgtype.UUID{Bytes: result.SiteFix.ID, Valid: true}, PrClaimToken: pgtype.UUID{Bytes: claimToken, Valid: true},
			PublisherConnectionID: target.ConnectionID, ExpectedConnectionUpdatedAt: target.ExpectedUpdatedAt,
			ExpectedRepoFullName: target.Repo, ExpectedBaseBranch: target.Branch,
		})
		return canonicalDoctorLifecycleTransitionError(renewErr)
	}
	client.BeforeMutation = renewClaim
	snapshot := sitefix.RepositorySnapshot{Repo: prepared.Repo, Branch: prepared.BaseBranch, BaseCommitSHA: prepared.BaseCommitSHA}
	for _, file := range prepared.Files {
		if currentErr := s.ensureGitHubPRTargetCurrent(ctx, result.SiteFix.ProjectID, target); currentErr != nil {
			return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, currentErr)
		}
		content, readErr := repositoryClient.ReadBlobBounded(ctx, file.BaseSHA, sitefix.MaxRepositorySourceFileBytes)
		if readErr != nil {
			return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, safeRepositorySourceFailure("prepared repository source blob is unavailable", readErr))
		}
		snapshot.Sources = append(snapshot.Sources, sitefix.RepositorySource{Path: file.Path, SHA: file.BaseSHA, Content: string(content)})
	}
	updates, actualDiff, prepared, err := sitefix.ReapplyRepositoryPreparedPatch(claimed.PatchSnapshot, snapshot)
	if err != nil {
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, err)
	}
	persistedDiff, err := sitefix.PreserveRepositoryActualDiffMetadata(claimed.DiffSnapshot, actualDiff)
	if err != nil {
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, err)
	}
	if !claimed.CreatedAt.Valid || claimed.PrClaimAuthorityFingerprint == nil || strings.TrimSpace(*claimed.PrClaimAuthorityFingerprint) == "" {
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, errors.New("prepared repository claim is incomplete"))
	}
	paths := make([]string, 0, len(prepared.Files))
	files := make([]publisher.GitHubFileUpdate, 0, len(updates))
	for _, file := range prepared.Files {
		paths = append(paths, file.Path)
	}
	for _, update := range updates {
		files = append(files, publisher.GitHubFileUpdate{Path: update.Path, BaseBlobSHA: update.BaseSHA, Content: update.Content})
	}
	pathSnapshot, err := json.Marshal(paths)
	if err != nil {
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, err)
	}
	firstPath, firstBaseSHA := prepared.Files[0].Path, prepared.Files[0].BaseSHA
	repo, branch, baseCommit := prepared.Repo, prepared.BaseBranch, prepared.BaseCommitSHA
	baseHash, proposedHash := prepared.SourceAggregateSHA256, prepared.ResultAggregateSHA256
	if err := s.ensureGitHubPRTargetCurrent(ctx, result.SiteFix.ProjectID, target); err != nil {
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, err)
	}
	saved, err := s.Q.SaveCanonicalSiteFixPreparedPatch(ctx, db.SaveCanonicalSiteFixPreparedPatchParams{
		PublisherConnectionID: pgtype.UUID{Bytes: target.ConnectionID, Valid: true}, RepoFullName: &repo, BaseBranch: &branch,
		BaseCommitSha: &baseCommit, SourceFilePath: &firstPath, SourceFilePaths: pathSnapshot, BaseFileSha: &firstBaseSHA,
		BaseContentHash: &baseHash, ProposedContentHash: &proposedHash,
		SourceMappingConfidence: claimed.SourceMappingConfidence, SourceMappingReason: claimed.SourceMappingReason,
		PatchSnapshot: claimed.PatchSnapshot, DiffSnapshot: persistedDiff, ResolutionCriteria: claimed.ResolutionCriteria,
		ProjectID: result.SiteFix.ProjectID, ApplicationID: claimed.ID, SiteFixID: pgtype.UUID{Bytes: result.SiteFix.ID, Valid: true},
		PrClaimToken: pgtype.UUID{Bytes: claimToken, Valid: true}, WriterAuthorityFingerprint: claimed.PrClaimAuthorityFingerprint,
		ExpectedConnectionUpdatedAt: target.ExpectedUpdatedAt, ExpectedRepoFullName: target.Repo, ExpectedBaseBranch: target.Branch,
	})
	if err != nil {
		return result, canonicalDoctorLifecycleTransitionError(err)
	}
	result.Application = saved
	if err := s.ensureGitHubPRTargetCurrent(ctx, result.SiteFix.ProjectID, target); err != nil {
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, err)
	}
	pr, reconciledWorkingBranch, err := createCanonicalSiteFixPRWithLegacyReconciliation(ctx, client, legacySiteFixRepositoryWorkingBranch(result.SiteFix.ID), publisher.GitHubFileUpdatesPRInput{
		WorkingBranch: workingBranch, BaseCommitSHA: prepared.BaseCommitSHA, Files: files,
		CommitMessage: "fix: apply CiteLoop Doctor Site Fix", CommitDate: claimed.CreatedAt.Time,
		Title: "Apply CiteLoop Doctor Site Fix", Body: "Applies the approved Doctor Site Fix for " + result.Application.TargetUrl + ".\n\nMerging this PR moves the fix to awaiting deployment; verification runs separately.",
	})
	if err != nil {
		if errors.Is(err, publisher.ErrSourceConflict) {
			return s.resetCanonicalSiteFixPRForReprepare(ctx, result, claimToken, "repository_source_conflict", err)
		}
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, err)
	}
	workingBranch = reconciledWorkingBranch
	prNumber := int32(pr.Number)
	if err := renewClaim(ctx); err != nil {
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, err)
	}
	app, err := s.Q.MarkCanonicalSiteFixGitHubPR(ctx, db.MarkCanonicalSiteFixGitHubPRParams{
		PublisherConnectionID: target.ConnectionID, RepoFullName: &repo,
		BaseBranch: &branch, WorkingBranch: &workingBranch, BaseCommitSha: &baseCommit, HeadCommitSha: &pr.HeadCommitSHA,
		SourceFilePath: &firstPath, BaseFileSha: &firstBaseSHA, ProposedContentHash: &proposedHash,
		GithubPrNumber: &prNumber, GithubPrUrl: &pr.URL, GithubPrState: pr.State,
		ProjectID: result.SiteFix.ProjectID, ApplicationID: result.Application.ID,
		SiteFixID:                   pgtype.UUID{Bytes: result.SiteFix.ID, Valid: true},
		PrClaimToken:                pgtype.UUID{Bytes: claimToken, Valid: true},
		ExpectedConnectionUpdatedAt: target.ExpectedUpdatedAt, ExpectedRepoFullName: target.Repo, ExpectedBaseBranch: target.Branch,
	})
	if err != nil {
		cause := canonicalDoctorLifecycleTransitionError(err)
		if errors.Is(err, pgx.ErrNoRows) {
			if currentErr := s.ensureGitHubPRTargetCurrent(ctx, result.SiteFix.ProjectID, target); currentErr != nil {
				cause = currentErr
			}
		}
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, cause)
	}
	result.Application = db.SiteChangeApplication(app)
	return reloadCanonicalSiteFixAfterPRObservation(ctx, s.Q, result)
}

type canonicalSiteFixPRClient interface {
	FindPullRequestByHead(context.Context, string) (publisher.GitHubPRResult, bool, error)
	CreateFileUpdatesPR(context.Context, publisher.GitHubFileUpdatesPRInput) (publisher.GitHubPRResult, error)
}

func createCanonicalSiteFixPRWithLegacyReconciliation(
	ctx context.Context,
	client canonicalSiteFixPRClient,
	legacyWorkingBranch string,
	input publisher.GitHubFileUpdatesPRInput,
) (publisher.GitHubPRResult, string, error) {
	primaryWorkingBranch := input.WorkingBranch
	legacyWorkingBranch = strings.TrimSpace(legacyWorkingBranch)
	if legacyWorkingBranch != "" && legacyWorkingBranch != primaryWorkingBranch {
		if _, found, err := client.FindPullRequestByHead(ctx, legacyWorkingBranch); err != nil {
			return publisher.GitHubPRResult{}, "", err
		} else if found {
			legacyInput := input
			legacyInput.WorkingBranch = legacyWorkingBranch
			pr, createErr := client.CreateFileUpdatesPR(ctx, legacyInput)
			if createErr == nil {
				return pr, legacyWorkingBranch, nil
			}
			if !errors.Is(createErr, publisher.ErrDivergentPullRequest) {
				return publisher.GitHubPRResult{}, "", createErr
			}
		}
	}
	input.WorkingBranch = primaryWorkingBranch
	pr, err := client.CreateFileUpdatesPR(ctx, input)
	return pr, primaryWorkingBranch, err
}

type canonicalSiteFixByIDStore interface {
	GetCanonicalSiteFix(context.Context, db.GetCanonicalSiteFixParams) (db.SiteFix, error)
}

func reloadCanonicalSiteFixAfterPRObservation(
	ctx context.Context,
	store canonicalSiteFixByIDStore,
	result sitefix.ApplyResult,
) (sitefix.ApplyResult, error) {
	fix, err := store.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{
		ID: result.SiteFix.ID, ProjectID: result.SiteFix.ProjectID,
	})
	if err != nil {
		return result, canonicalDoctorLifecycleTransitionError(err)
	}
	result.SiteFix = fix
	return result, nil
}

func (s *Server) resetCanonicalSiteFixPRForReprepare(
	ctx context.Context,
	result sitefix.ApplyResult,
	claimToken uuid.UUID,
	reason string,
	cause error,
) (sitefix.ApplyResult, error) {
	app, err := s.Q.ResetCanonicalSiteFixSourceConflictForReprepare(ctx, db.ResetCanonicalSiteFixSourceConflictForReprepareParams{
		ProjectID: result.SiteFix.ProjectID, ApplicationID: result.Application.ID,
		SiteFixID:    pgtype.UUID{Bytes: result.SiteFix.ID, Valid: true},
		PrClaimToken: pgtype.UUID{Bytes: claimToken, Valid: true}, ReprepareReason: reason,
	})
	if err == nil {
		result.Application = db.SiteChangeApplication(app)
		return result, fmt.Errorf("%w: %s", errCanonicalSiteFixFreshReprepare, reason)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, fmt.Errorf("%w: %s", errCanonicalSiteFixReprepareExhausted, reason))
	}
	return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, errors.Join(cause, canonicalDoctorLifecycleTransitionError(err)))
}

func (s *Server) failCanonicalSiteFixPRClaim(ctx context.Context, result sitefix.ApplyResult, claimToken uuid.UUID, cause error) (sitefix.ApplyResult, error) {
	reason := safeCanonicalSiteFixPRFailureCode(cause)
	app, err := s.Q.FailCanonicalSiteFixGitHubPRClaim(ctx, db.FailCanonicalSiteFixGitHubPRClaimParams{
		FailureReason: &reason, ProjectID: result.SiteFix.ProjectID, ApplicationID: result.Application.ID,
		SiteFixID: pgtype.UUID{Bytes: result.SiteFix.ID, Valid: true}, PrClaimToken: pgtype.UUID{Bytes: claimToken, Valid: true},
	})
	if err != nil {
		return result, errors.Join(cause, canonicalDoctorLifecycleTransitionError(err))
	}
	result.Application = app
	return result, cause
}

func safeCanonicalSiteFixPRFailureCode(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "pr_interrupted"
	}
	if errors.Is(err, publisher.ErrSourceConflict) {
		return "repository_source_conflict"
	}
	if errors.Is(err, errCanonicalSiteFixReprepareExhausted) {
		return "repository_reprepare_exhausted"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "divergent"):
		return "publisher_branch_conflict"
	case strings.Contains(message, "prepared repository"), strings.Contains(message, "exact replacement"), strings.Contains(message, "hash"):
		return "prepared_patch_invalid"
	case strings.Contains(message, "credential"), strings.Contains(message, "publisher"), strings.Contains(message, "github"):
		return "publisher_unavailable"
	default:
		return "github_pr_failed"
	}
}

func siteFixRepositoryWorkingBranch(fixID, applicationID uuid.UUID) string {
	return legacySiteFixRepositoryWorkingBranch(fixID) + "-" + compactSiteFixBranchID(applicationID)
}

func legacySiteFixRepositoryWorkingBranch(fixID uuid.UUID) string {
	return "citeloop/doctor-site-fix-" + compactSiteFixBranchID(fixID)
}

func compactSiteFixBranchID(id uuid.UUID) string {
	compact := strings.ReplaceAll(id.String(), "-", "")
	if len(compact) > 12 {
		compact = compact[:12]
	}
	return compact
}

func canonicalDoctorLifecycleTransitionError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: canonical state changed concurrently", sitefix.ErrLifecycleConflict)
	}
	return err
}

func (s *Server) verifyDoctorSiteFix(w http.ResponseWriter, r *http.Request) {
	projectID, fixID, ok := s.seoDoctorIDs(w, r, "fixID")
	if !ok {
		return
	}
	service := s.doctorSiteFixLifecycleService()
	if service == nil {
		writeErr(w, http.StatusInternalServerError, "canonical Site Fix lifecycle service unavailable")
		return
	}
	var input DoctorSiteFixVerificationInput
	if r.Body != nil {
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&input); err != nil && !errors.Is(err, io.EOF) {
			writeErr(w, http.StatusBadRequest, "invalid verification request")
			return
		}
	}
	result, err := service.RequestVerification(r.Context(), projectID, fixID, input)
	if err != nil {
		s.writeDoctorSiteFixError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) terminateDoctorSiteFix(w http.ResponseWriter, r *http.Request) {
	projectID, fixID, ok := s.seoDoctorIDs(w, r, "fixID")
	if !ok {
		return
	}
	service := s.doctorSiteFixLifecycleService()
	if service == nil {
		writeErr(w, http.StatusInternalServerError, "canonical Site Fix lifecycle service unavailable")
		return
	}
	result, err := service.Terminate(r.Context(), projectID, fixID)
	if err != nil {
		s.writeDoctorSiteFixError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) writeDoctorSiteFixError(w http.ResponseWriter, err error) {
	var hold *DoctorSiteFixArbitrationHoldError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeErr(w, http.StatusNotFound, "Doctor Site Fix resource not found")
	case errors.As(err, &hold):
		status := http.StatusConflict
		if hold.Decision.Disposition == discovery.DispositionIncompleteSpecification {
			status = http.StatusUnprocessableEntity
		}
		writeErr(w, status, hold.publicReason())
	case errors.Is(err, sitefix.ErrHealthyFinding), errors.Is(err, sitefix.ErrIncompleteCandidate), errors.Is(err, sitefix.ErrCandidateFindingMismatch):
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, sitefix.ErrPatchGroundingRejected):
		writeErr(w, http.StatusUnprocessableEntity, "The generated repair patch did not preserve the approved page intent; retry PR creation to generate a new audited patch.")
	case errors.Is(err, ErrDoctorSiteFixCrossLineOwnership):
		writeErr(w, http.StatusConflict, "This work is owned by Opportunities; no Doctor Site Fix was created")
	case errors.Is(err, ErrDoctorSiteFixCreateBusy), errors.Is(err, errDoctorSiteFixPreparationLost), errors.Is(err, errDoctorSiteFixPreparationReclaim):
		writeErr(w, http.StatusConflict, "Another equivalent Doctor Site Fix request is still being evaluated")
	case errors.Is(err, discovery.ErrWriterUnavailable), errors.Is(err, discovery.ErrSnapshotStale), errors.Is(err, sitefix.ErrActivePredecessor), errors.Is(err, ErrDoctorSiteFixTransitionConflict):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, sitefix.ErrLifecycleConflict):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, errGitHubPRNotReady), errors.Is(err, errGitHubPRReadinessChanged):
		writeErr(w, http.StatusConflict, "Connect GitHub and grant repository write access so CiteLoop can create a repair PR automatically.")
	case errors.Is(err, ErrDoctorSiteFixManualEvidenceInvalid):
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, sitefix.ErrSiteFixMeasurementPlanInvariant):
		writeErr(w, http.StatusUnprocessableEntity, "The stored Site Fix measurement plan is incomplete or no longer baseline-ready")
	case errors.Is(err, ErrDoctorSiteFixMeasurementOptInConflict):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrDoctorAIVerificationNotAuthorized):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrDoctorAIRequestAlreadyHandled):
		writeErr(w, http.StatusConflict, err.Error())
	default:
		if s != nil && s.Log != nil {
			s.Log.Error("canonical Doctor Site Fix request failed", slog.Any("err", err))
		}
		writeErr(w, http.StatusInternalServerError, "Doctor Site Fix service is temporarily unavailable")
	}
}
