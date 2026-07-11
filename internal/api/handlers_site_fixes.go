package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/sitefix"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDoctorSiteFixTransitionConflict = errors.New("canonical Site Fix transition conflict")
var ErrDoctorSiteFixCrossLineOwnership = errors.New("Doctor candidate is owned by Opportunities")
var ErrDoctorSiteFixCreateBusy = errors.New("Doctor Site Fix creation is busy")
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
	siteFixTask5Handoff(w, r, s)
}

func (s *Server) verifyDoctorSiteFix(w http.ResponseWriter, r *http.Request) {
	siteFixTask5Handoff(w, r, s)
}

func siteFixTask5Handoff(w http.ResponseWriter, r *http.Request, s *Server) {
	if _, _, ok := s.seoDoctorIDs(w, r, "fixID"); !ok {
		return
	}
	writeErr(w, http.StatusNotImplemented, "canonical Site Fix apply/verify is not enabled until the Task 5 lifecycle cutover")
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
	default:
		if s != nil && s.Log != nil {
			s.Log.Error("canonical Doctor Site Fix request failed", slog.Any("err", err))
		}
		writeErr(w, http.StatusInternalServerError, "Doctor Site Fix service is temporarily unavailable")
	}
}
