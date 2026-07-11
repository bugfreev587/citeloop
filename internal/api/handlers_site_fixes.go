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
var errDoctorSiteFixPreparationReclaim = errors.New("Doctor Site Fix preparation lease must be reclaimed")
var errDoctorSiteFixPreparationLost = errors.New("Doctor Site Fix preparation lease was lost")

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
	CreateFromFinding(context.Context, uuid.UUID, uuid.UUID) (db.SiteFix, bool, error)
	List(context.Context, uuid.UUID, *string) ([]db.SiteFix, error)
	Get(context.Context, uuid.UUID, uuid.UUID) (db.SiteFix, error)
	Approve(context.Context, uuid.UUID, uuid.UUID, time.Time) (db.SiteFix, error)
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
	project, err := s.q.GetProject(ctx, projectID)
	if err != nil {
		return sitefix.ApplyResult{}, err
	}
	projectConfig, err := config.Parse(project.Config)
	if err != nil {
		return sitefix.ApplyResult{}, err
	}
	service := s.apply
	if !projectConfig.AllowsDoctorAI(config.DoctorAITriggerApplyUser) {
		service.Generator = sitefix.DeterministicApplicationGenerator{}
	}
	return service.Apply(ctx, projectID, fixID)
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
	finding, err := c.backend.LoadFinding(ctx, projectID, findingID)
	if err != nil {
		return db.SiteFix{}, false, err
	}
	if finding.ProjectID != projectID || finding.ID != findingID {
		return db.SiteFix{}, false, sitefix.ErrProjectMismatch
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
	for attempt := 0; attempt < 3; attempt++ {
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
		return c.runPreparationLeader(ctx, finding, canonical, claim)
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
		// Fail closed on a stale snapshot. A new request rematerializes and
		// persists its own exact-merge decision/evidence association.
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
	q        *db.Queries
	creation doctorSiteFixCreationCoordinator
}

func NewDoctorSiteFixService(pool *pgxpool.Pool, q *db.Queries, provider llm.Provider, model string) DoctorSiteFixService {
	var comparator discovery.SemanticComparator
	if provider != nil {
		comparator = discovery.NewLLMSemanticComparator(provider, "tokengate", model).WithPurpose(llm.PurposeSiteFix)
	}
	backend := &postgresDoctorSiteFixBackend{pool: pool, q: q, comparator: comparator, model: strings.TrimSpace(model)}
	return &postgresDoctorSiteFixService{
		q: q,
		creation: doctorSiteFixCreationCoordinator{
			preparations: &postgresDoctorSiteFixPreparationManager{
				q: q, runtimeProvider: provider, requestedModel: strings.TrimSpace(model),
				leaseTTL: 2 * time.Minute, resultTTL: 5 * time.Minute,
				waitTimeout: 30 * time.Second, pollInterval: 100 * time.Millisecond,
			},
			backend: backend,
		},
	}
}

func (s *postgresDoctorSiteFixService) CreateFromFinding(ctx context.Context, projectID, findingID uuid.UUID) (db.SiteFix, bool, error) {
	if s == nil || s.q == nil {
		return db.SiteFix{}, false, errors.New("canonical Site Fix database unavailable")
	}
	return s.creation.CreateFromFinding(ctx, projectID, findingID)
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

func (s *postgresDoctorSiteFixService) List(ctx context.Context, projectID uuid.UUID, status *string) ([]db.SiteFix, error) {
	if s == nil || s.q == nil {
		return nil, errors.New("canonical Site Fix database unavailable")
	}
	return s.q.ListCanonicalSiteFixes(ctx, db.ListCanonicalSiteFixesParams{ProjectID: projectID, Status: status})
}

func (s *postgresDoctorSiteFixService) Get(ctx context.Context, projectID, fixID uuid.UUID) (db.SiteFix, error) {
	if s == nil || s.q == nil {
		return db.SiteFix{}, errors.New("canonical Site Fix database unavailable")
	}
	return s.q.GetCanonicalSiteFix(ctx, db.GetCanonicalSiteFixParams{ID: fixID, ProjectID: projectID})
}

func (s *postgresDoctorSiteFixService) Approve(ctx context.Context, projectID, fixID uuid.UUID, approvedAt time.Time) (db.SiteFix, error) {
	if s == nil || s.q == nil {
		return db.SiteFix{}, errors.New("canonical Site Fix database unavailable")
	}
	if _, err := s.Get(ctx, projectID, fixID); err != nil {
		return db.SiteFix{}, err
	}
	if _, err := s.q.ApproveCanonicalSiteFix(ctx, db.ApproveCanonicalSiteFixParams{
		SiteFixID: fixID, ProjectID: projectID,
		ApprovedAt: pgtype.Timestamptz{Time: approvedAt.UTC(), Valid: true},
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.SiteFix{}, ErrDoctorSiteFixTransitionConflict
		}
		return db.SiteFix{}, err
	}
	return s.Get(ctx, projectID, fixID)
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
	return &postgresDoctorSiteFixLifecycleService{
		pool: s.Pool,
		q:    s.Q,
		apply: sitefix.ApplyService{
			Store:     sitefix.PostgresApplyStore{Pool: s.Pool, Q: s.Q},
			Generator: sitefix.LLMApplicationGenerator{Provider: s.LLM, Model: s.Env.TokenGateModel},
		},
	}
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

func (s *Server) approveDoctorSiteFix(w http.ResponseWriter, r *http.Request) {
	projectID, fixID, ok := s.seoDoctorIDs(w, r, "fixID")
	if !ok {
		return
	}
	service := s.doctorSiteFixService()
	if service == nil {
		writeErr(w, http.StatusInternalServerError, "canonical Site Fix service unavailable")
		return
	}
	fix, err := service.Approve(r.Context(), projectID, fixID, time.Now().UTC())
	if err != nil {
		s.writeDoctorSiteFixError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, fix)
}

func (s *Server) applyDoctorSiteFix(w http.ResponseWriter, r *http.Request) {
	projectID, fixID, ok := s.seoDoctorIDs(w, r, "fixID")
	if !ok {
		return
	}
	service := s.doctorSiteFixLifecycleService()
	if service == nil {
		writeErr(w, http.StatusInternalServerError, "canonical Site Fix lifecycle service unavailable")
		return
	}
	result, err := service.Apply(r.Context(), projectID, fixID)
	if err != nil {
		s.writeDoctorSiteFixError(w, err)
		return
	}
	if result.Application.Status == "needs_follow_up" && result.SiteFix.Status == "applying" {
		result.Application, err = s.Q.ReopenCanonicalSiteFixApply(r.Context(), db.ReopenCanonicalSiteFixApplyParams{
			ProjectID: projectID, ApplicationID: result.Application.ID, SiteFixID: pgtype.UUID{Bytes: fixID, Valid: true},
		})
		if err != nil {
			s.writeDoctorSiteFixError(w, canonicalDoctorLifecycleTransitionError(err))
			return
		}
	}
	if result.Application.Status == "ready_for_pr" || result.Application.Status == "creating_pr" {
		result, err = s.openCanonicalSiteFixGitHubPR(r.Context(), result)
		if err != nil {
			s.writeDoctorSiteFixError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) openCanonicalSiteFixGitHubPR(ctx context.Context, result sitefix.ApplyResult) (sitefix.ApplyResult, error) {
	if s.Q == nil {
		return result, errors.New("canonical Site Fix database unavailable")
	}
	if canonicalSiteFixPRMutationFamily(result.SiteFix) != "metadata_rewrite" {
		return s.fallbackCanonicalSiteFixManualApply(ctx, result, "this mutation family does not have a deterministic GitHub patch adapter")
	}
	var paths []string
	if json.Unmarshal(result.Application.SourceFilePaths, &paths) != nil || len(paths) != 1 {
		return s.fallbackCanonicalSiteFixManualApply(ctx, result, "an exact single source file was not available")
	}
	sourcePath := strings.TrimSpace(paths[0])
	if sourcePath == "" || strings.HasPrefix(sourcePath, "/") || strings.Contains(sourcePath, "..") || strings.Contains(sourcePath, "\\") {
		return s.fallbackCanonicalSiteFixManualApply(ctx, result, "the generated source path was not safe")
	}
	conn, err := s.Q.GetEnabledPublisherConnectionForProject(ctx, db.GetEnabledPublisherConnectionForProjectParams{ProjectID: result.SiteFix.ProjectID, Kind: publisher.ConnectionKindGitHubNextJS})
	if err != nil {
		return s.fallbackCanonicalSiteFixManualApply(ctx, result, "no connected GitHub publisher was available")
	}
	cfg, err := publisher.ParseGitHubNextJSConfig(conn.Config)
	if err != nil {
		return s.fallbackCanonicalSiteFixManualApply(ctx, result, "the GitHub publisher configuration was invalid")
	}
	if target, ok := publisher.GitHubNextJSTargetForSiteURL(result.Application.TargetUrl); ok {
		cfg.Branch, cfg.BaseURL = target.Branch, target.BaseURL
	}
	assetType := "metadata_rewrite"
	pseudoSourceAction := db.ContentAction{TargetUrl: &result.Application.TargetUrl, NormalizedTargetUrl: &result.Application.NormalizedTargetUrl, AssetType: &assetType}
	allowedSource := false
	for _, mapping := range siteFixMetadataRewriteSourceCandidates(pseudoSourceAction, cfg) {
		if mapping.SourceFilePath == sourcePath {
			allowedSource = true
			break
		}
	}
	if !allowedSource {
		return s.fallbackCanonicalSiteFixManualApply(ctx, result, "the generated source path did not match the deterministic target mapping")
	}
	token, err := s.publisherConnectionToken(ctx, result.SiteFix.ProjectID, conn)
	if err != nil || strings.TrimSpace(token) == "" {
		return s.fallbackCanonicalSiteFixManualApply(ctx, result, "the GitHub publisher credential was unavailable")
	}
	client := publisher.NewGitHubPRClient(token, cfg.Repo, cfg.Branch, s.Log)
	shortID := strings.ReplaceAll(result.SiteFix.ID.String()[:12], "-", "")
	workingBranch := "citeloop/doctor-site-fix-" + shortID
	claimToken := uuid.New()
	claimed, err := s.Q.ClaimCanonicalSiteFixGitHubPR(ctx, db.ClaimCanonicalSiteFixGitHubPRParams{
		PrClaimToken: pgtype.UUID{Bytes: claimToken, Valid: true}, LeaseTtlSeconds: 300,
		ProjectID: result.SiteFix.ProjectID, ApplicationID: result.Application.ID,
		SiteFixID: pgtype.UUID{Bytes: result.SiteFix.ID, Valid: true},
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
		if app.Status == "github_pr_open" || app.Status == "creating_pr" {
			return result, nil
		}
		return result, sitefix.ErrLifecycleConflict
	}
	result.Application = claimed
	renewClaim := func(callCtx context.Context) error {
		_, renewErr := s.Q.RenewCanonicalSiteFixGitHubPRClaim(callCtx, db.RenewCanonicalSiteFixGitHubPRClaimParams{
			LeaseTtlSeconds: 300, ProjectID: result.SiteFix.ProjectID, ApplicationID: result.Application.ID,
			SiteFixID: pgtype.UUID{Bytes: result.SiteFix.ID, Valid: true}, PrClaimToken: pgtype.UUID{Bytes: claimToken, Valid: true},
		})
		return canonicalDoctorLifecycleTransitionError(renewErr)
	}
	client.BeforeMutation = renewClaim
	if existingPR, found, reconcileErr := client.FindPullRequestByHead(ctx, workingBranch); reconcileErr != nil {
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, reconcileErr)
	} else if found {
		if err := renewClaim(ctx); err != nil {
			return result, err
		}
		prNumber := int32(existingPR.Number)
		app, err := s.Q.MarkCanonicalSiteFixGitHubPR(ctx, db.MarkCanonicalSiteFixGitHubPRParams{
			PublisherConnectionID: pgtype.UUID{Bytes: conn.ID, Valid: true}, RepoFullName: &cfg.Repo,
			BaseBranch: &cfg.Branch, WorkingBranch: &workingBranch, HeadCommitSha: emptyStringPointer(existingPR.HeadCommitSHA),
			SourceFilePath: &sourcePath, GithubPrNumber: &prNumber, GithubPrUrl: &existingPR.URL, GithubPrState: &existingPR.State,
			ProjectID: result.SiteFix.ProjectID, ApplicationID: result.Application.ID,
			SiteFixID:    pgtype.UUID{Bytes: result.SiteFix.ID, Valid: true},
			PrClaimToken: pgtype.UUID{Bytes: claimToken, Valid: true},
		})
		if err != nil {
			return result, canonicalDoctorLifecycleTransitionError(err)
		}
		result.Application = app
		return result, nil
	}
	baseContent, baseFileSHA, err := client.ReadFile(ctx, sourcePath, cfg.Branch)
	if err != nil {
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, errors.New("the exact source file could not be read"))
	}
	pseudoAction := db.ContentAction{DiffSnapshot: result.Application.PatchSnapshot, OutputSnapshot: result.Application.DiffSnapshot}
	proposedContent, err := siteFixMetadataRewriteContent(baseContent, pseudoAction)
	if err != nil {
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, errors.New("the generated change could not be applied to the exact source"))
	}
	if proposedContent == baseContent {
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, errors.New("the exact source already contains the proposed change; production verification is required"))
	}
	pr, err := client.CreatePageUpdatePR(ctx, publisher.GitHubPRInput{
		SourcePath: sourcePath, WorkingBranch: workingBranch, BaseFileSHA: baseFileSHA,
		ProposedContentMD: proposedContent, CommitMessage: "fix: apply CiteLoop Doctor Site Fix",
		Title: "Apply CiteLoop Doctor Site Fix", Body: "Applies the approved Doctor Site Fix for " + result.Application.TargetUrl + ".\n\nMerging this PR moves the fix to awaiting deployment; verification runs separately.",
	})
	if err != nil {
		return s.failCanonicalSiteFixPRClaim(ctx, result, claimToken, errors.New("GitHub PR creation failed; retry apply after checking the publisher connection"))
	}
	prNumber := int32(pr.Number)
	proposedHash := pageUpdateContentHash(proposedContent)
	if err := renewClaim(ctx); err != nil {
		return result, err
	}
	app, err := s.Q.MarkCanonicalSiteFixGitHubPR(ctx, db.MarkCanonicalSiteFixGitHubPRParams{
		PublisherConnectionID: pgtype.UUID{Bytes: conn.ID, Valid: true}, RepoFullName: &cfg.Repo,
		BaseBranch: &cfg.Branch, WorkingBranch: &workingBranch, HeadCommitSha: &pr.HeadCommitSHA,
		SourceFilePath: &sourcePath, BaseFileSha: &pr.BaseFileSHA, ProposedContentHash: &proposedHash,
		GithubPrNumber: &prNumber, GithubPrUrl: &pr.URL, GithubPrState: &pr.State,
		ProjectID: result.SiteFix.ProjectID, ApplicationID: result.Application.ID,
		SiteFixID:    pgtype.UUID{Bytes: result.SiteFix.ID, Valid: true},
		PrClaimToken: pgtype.UUID{Bytes: claimToken, Valid: true},
	})
	if err != nil {
		return result, canonicalDoctorLifecycleTransitionError(err)
	}
	result.Application = app
	return result, nil
}

func (s *Server) failCanonicalSiteFixPRClaim(ctx context.Context, result sitefix.ApplyResult, claimToken uuid.UUID, cause error) (sitefix.ApplyResult, error) {
	reason := cause.Error()
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

func canonicalSiteFixPRMutationFamily(fix db.SiteFix) string {
	var payload struct {
		Mutations []struct {
			Field     string `json:"field"`
			Operation string `json:"operation"`
		} `json:"mutations"`
	}
	if json.Unmarshal(fix.ProposedFix, &payload) != nil || len(payload.Mutations) == 0 {
		return ""
	}
	for _, mutation := range payload.Mutations {
		field := strings.ToLower(strings.TrimSpace(mutation.Field))
		op := strings.ToLower(strings.TrimSpace(mutation.Operation))
		if (field != "title" && field != "meta_description") || (op != "add" && op != "replace" && op != "update") {
			return ""
		}
	}
	return "metadata_rewrite"
}

func emptyStringPointer(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	value = strings.TrimSpace(value)
	return &value
}

func canonicalDoctorLifecycleTransitionError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: canonical state changed concurrently", sitefix.ErrLifecycleConflict)
	}
	return err
}

func (s *Server) fallbackCanonicalSiteFixManualApply(ctx context.Context, result sitefix.ApplyResult, reason string) (sitefix.ApplyResult, error) {
	app, err := s.Q.MarkCanonicalSiteFixManualHandoff(ctx, db.MarkCanonicalSiteFixManualHandoffParams{
		ProjectID: result.SiteFix.ProjectID, SiteFixID: pgtype.UUID{Bytes: result.SiteFix.ID, Valid: true},
		ApplicationID: result.Application.ID, FailureReason: &reason,
	})
	if err != nil {
		return result, canonicalDoctorLifecycleTransitionError(err)
	}
	result.Application = app
	return result, nil
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
	case errors.Is(err, ErrDoctorSiteFixCrossLineOwnership):
		writeErr(w, http.StatusConflict, "This work is owned by Opportunities; no Doctor Site Fix was created")
	case errors.Is(err, ErrDoctorSiteFixCreateBusy), errors.Is(err, errDoctorSiteFixPreparationLost), errors.Is(err, errDoctorSiteFixPreparationReclaim):
		writeErr(w, http.StatusConflict, "Another equivalent Doctor Site Fix request is still being evaluated")
	case errors.Is(err, discovery.ErrWriterUnavailable), errors.Is(err, discovery.ErrSnapshotStale), errors.Is(err, sitefix.ErrActivePredecessor), errors.Is(err, ErrDoctorSiteFixTransitionConflict):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, sitefix.ErrLifecycleConflict):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrDoctorSiteFixManualEvidenceInvalid):
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
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
