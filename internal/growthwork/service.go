package growthwork

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrGrowthHeld = errors.New("Growth work requires arbitration review")

type Service struct {
	pool            *pgxpool.Pool
	q               *db.Queries
	comparator      discovery.SemanticComparator
	cutoverBatchID  uuid.UUID
	cutoverToken    uuid.UUID
	cutoverSequence int32
}

func NewService(pool *pgxpool.Pool, q *db.Queries, comparator discovery.SemanticComparator) *Service {
	if q == nil && pool != nil {
		q = db.New(pool)
	}
	return &Service{pool: pool, q: q, comparator: comparator}
}

type growthCutoverWorkError struct {
	OpportunityID uuid.UUID
	Err           error
}

func (e *growthCutoverWorkError) Error() string { return e.Err.Error() }
func (e *growthCutoverWorkError) Unwrap() error { return e.Err }

func (s *Service) EnsureProjectCutover(ctx context.Context, projectID uuid.UUID) error {
	if s == nil || s.pool == nil || s.q == nil || projectID == uuid.Nil {
		return errors.New("canonical Growth database is unavailable")
	}
	authority, err := s.q.GetProductWriterAuthority(ctx, db.GetProductWriterAuthorityParams{ProjectID: projectID, Product: "opportunities"})
	if err != nil {
		return err
	}
	if authority.WriterAuthority == "canonical" && !authority.WriteFenced {
		missing, err := s.q.CountUnrepresentedActiveLegacyGrowth(ctx, projectID)
		if err != nil {
			return err
		}
		if missing != 0 {
			return fmt.Errorf("%w: canonical authority has %d unrepresented active legacy Growth rows", ErrGrowthHeld, missing)
		}
		return nil
	}
	if authority.WriteFenced {
		session, err := s.q.GetActiveGrowthCutoverSession(ctx, projectID)
		if err != nil {
			return discovery.ErrWriterUnavailable
		}
		if session.Error != nil && strings.TrimSpace(*session.Error) != "" {
			return fmt.Errorf("%w: Growth cutover review is required: %s", ErrGrowthHeld, *session.Error)
		}
		if session.StartedAt.Valid && time.Since(session.StartedAt.Time) < 5*time.Minute {
			return discovery.ErrWriterUnavailable
		}
		if err := s.rollbackGrowthCutover(ctx, session, errors.New("recover interrupted Growth cutover"), uuid.Nil); err != nil {
			return err
		}
		return s.EnsureProjectCutover(ctx, projectID)
	}
	if authority.WriterAuthority != "legacy" {
		return discovery.ErrWriterUnavailable
	}
	session, err := s.startGrowthCutover(ctx, projectID)
	if err != nil {
		return err
	}
	migration := *s
	migration.cutoverBatchID = session.BatchID
	migration.cutoverToken = session.FenceToken
	if err := migration.migrateLegacyGrowth(ctx, projectID); err != nil {
		failedID := uuid.Nil
		var workErr *growthCutoverWorkError
		if errors.As(err, &workErr) {
			failedID = workErr.OpportunityID
		}
		var reviewErr *growthCutoverReviewError
		if errors.As(err, &reviewErr) {
			if holdErr := migration.holdGrowthCutoverForReview(ctx, session, reviewErr); holdErr != nil {
				return fmt.Errorf("hold Growth cutover review: %w (original: %v)", holdErr, err)
			}
			return err
		}
		if rollbackErr := migration.rollbackGrowthCutover(ctx, session, err, failedID); rollbackErr != nil {
			return fmt.Errorf("Growth cutover failed: %v; rollback failed: %w", err, rollbackErr)
		}
		return err
	}
	if err := migration.completeGrowthCutover(ctx, session); err != nil {
		if rollbackErr := migration.rollbackGrowthCutover(ctx, session, err, uuid.Nil); rollbackErr != nil {
			return fmt.Errorf("complete Growth cutover: %v; rollback failed: %w", err, rollbackErr)
		}
		return err
	}
	return nil
}

func (s *Service) holdGrowthCutoverForReview(ctx context.Context, session db.GrowthCutoverSession, review *growthCutoverReviewError) (resultErr error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	q := db.New(tx)
	authority, err := q.LockProductWriterAuthority(ctx, db.LockProductWriterAuthorityParams{ProjectID: session.ProjectID, Product: "opportunities"})
	if err != nil || authority.WriterAuthority != "canonical" || !authority.WriteFenced || !authority.FenceToken.Valid || authority.FenceToken.Bytes != session.FenceToken {
		return discovery.ErrWriterUnavailable
	}
	entries, err := q.ListGrowthCutoverSessionEntries(ctx, db.ListGrowthCutoverSessionEntriesParams{ProjectID: session.ProjectID, BatchID: session.BatchID})
	if err != nil {
		return err
	}
	var entry db.GrowthCutoverSessionEntry
	for _, candidate := range entries {
		if candidate.OpportunityID == review.OpportunityID {
			entry = candidate
			break
		}
	}
	if entry.OpportunityID == uuid.Nil {
		return errors.New("reviewed Growth opportunity has no cutover journal entry")
	}
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	reviewBatchID := uuid.New()
	resultSnapshot, _ := json.Marshal(map[string]any{"status": "review_required", "reason": review.Reason, "cutover_batch_id": session.BatchID})
	if _, err := q.CreateMigrationBatch(ctx, db.CreateMigrationBatchParams{
		ID: reviewBatchID, ProjectID: session.ProjectID, Product: "opportunities", BatchKind: "forward", Status: "review_required",
		SchemaVersion: "growth-execution-chain-v1", SourceCount: 1, MigratedCount: 0, ArchivedDuplicateCount: 0, ReviewCount: 1,
		WriterAuthorityBefore: "legacy", WriterAuthorityAfter: "canonical", SourceSnapshot: review.Snapshot,
		ResultSnapshot: resultSnapshot, InitiatedBy: "growth_cutover", StartedAt: session.StartedAt, FinishedAt: now,
	}); err != nil {
		return err
	}
	if _, err := q.AppendMigrationLedger(ctx, db.AppendMigrationLedgerParams{
		ID: uuid.New(), ProjectID: session.ProjectID, MigrationBatchID: reviewBatchID, SequenceNumber: 1,
		SourceObjectType: "seo_opportunity", SourceObjectID: review.OpportunityID,
		CanonicalObjectType: "growth_execution_chain", Operation: "map", OperationVersion: "growth-execution-chain-v1",
		CutoverPoint: "writer_fenced", RollbackEligibility: "eligible", BeforeHash: hashJSON(entry.BeforeSnapshot),
		AfterHash: hashJSON(entry.AfterSnapshot), BeforeSnapshot: entry.BeforeSnapshot, AfterSnapshot: entry.AfterSnapshot,
		InverseOperationVersion: "growth-execution-chain-v1", InverseOperation: entry.InverseOperation,
		AppliedBy: "growth_cutover", AppliedAt: now,
	}); err != nil {
		return err
	}
	if _, err := q.CreateMigrationReviewItem(ctx, db.CreateMigrationReviewItemParams{
		ID: uuid.New(), ProjectID: session.ProjectID, MigrationBatchID: reviewBatchID,
		SourceObjectType: "seo_opportunity", SourceObjectID: review.OpportunityID,
		ReasonCode: "growth_execution_chain_conflict", Reason: review.Reason,
		SourceSnapshot: review.Snapshot, ProposedResolution: json.RawMessage(`{"action":"resolve or explicitly map execution descendants"}`),
	}); err != nil {
		return err
	}
	message := review.Reason
	if _, err := q.SetGrowthCutoverSessionReviewRequired(ctx, db.SetGrowthCutoverSessionReviewRequiredParams{
		Error: &message, ProjectID: session.ProjectID, BatchID: session.BatchID,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) startGrowthCutover(ctx context.Context, projectID uuid.UUID) (db.GrowthCutoverSession, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return db.GrowthCutoverSession{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	q := db.New(tx)
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	token, batchID := uuid.New(), uuid.New()
	fencedBy := "growth_cutover"
	if _, err := q.FenceProductWriterAuthority(ctx, db.FenceProductWriterAuthorityParams{
		FenceToken: pgUUID(token), FencedBy: &fencedBy, FencedAt: now,
		ProjectID: projectID, Product: "opportunities",
	}); err != nil {
		return db.GrowthCutoverSession{}, err
	}
	if _, err := q.SwitchProductWriterAuthority(ctx, db.SwitchProductWriterAuthorityParams{
		WriterAuthority: "canonical", AuthorityChangedAt: now, ProjectID: projectID,
		Product: "opportunities", FenceToken: pgUUID(token), ExpectedWriterAuthority: "legacy",
	}); err != nil {
		return db.GrowthCutoverSession{}, err
	}
	session, err := q.StartGrowthCutoverSession(ctx, db.StartGrowthCutoverSessionParams{
		BatchID: batchID, ProjectID: projectID, FenceToken: token,
	})
	if err != nil {
		return db.GrowthCutoverSession{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.GrowthCutoverSession{}, err
	}
	return session, nil
}

func (s *Service) EnsureLegacyReservations(ctx context.Context, projectID uuid.UUID) error {
	if s == nil || s.pool == nil || s.q == nil || projectID == uuid.Nil {
		return errors.New("canonical Growth database is unavailable")
	}
	return s.EnsureProjectCutover(ctx, projectID)
}

func (s *Service) EnsureOpportunityReserved(ctx context.Context, projectID, opportunityID uuid.UUID) error {
	if s == nil || s.pool == nil || s.q == nil || projectID == uuid.Nil || opportunityID == uuid.Nil {
		return errors.New("canonical Growth database is unavailable")
	}
	if err := s.EnsureProjectCutover(ctx, projectID); err != nil {
		return err
	}
	opportunity, err := s.q.GetSEOOpportunity(ctx, db.GetSEOOpportunityParams{ID: opportunityID, ProjectID: projectID})
	if err != nil {
		return err
	}
	if opportunity.CanonicalGrowth {
		return nil
	}
	if _, err := s.q.GetGrowthOpportunityWorkAlias(ctx, db.GetGrowthOpportunityWorkAliasParams{
		ProjectID: projectID, LegacyOpportunityID: opportunityID,
	}); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return s.migrateLegacyOpportunity(ctx, opportunity)
}

func (s *Service) CanExecuteOpportunity(ctx context.Context, projectID, opportunityID uuid.UUID) (bool, error) {
	if s == nil || s.q == nil {
		return false, errors.New("canonical Growth database is unavailable")
	}
	return s.q.GrowthOpportunityExecutionGuard(ctx, db.GrowthOpportunityExecutionGuardParams{
		ProjectID: projectID, OpportunityID: pgUUID(opportunityID),
	})
}

// CreateOpportunity is the only forward Growth writer. Candidate persistence,
// cross-line arbitration, Opportunity insert, relationship rows, signature
// reservation, and bucket increments form one compare-and-reserve flow.
func (s *Service) CreateOpportunity(ctx context.Context, params db.CreateCanonicalGrowthOpportunityParams) (db.SeoOpportunity, error) {
	if s == nil || s.pool == nil || s.q == nil || params.ProjectID == uuid.Nil {
		return db.SeoOpportunity{}, errors.New("canonical Growth database is unavailable")
	}
	if params.ID == uuid.Nil {
		params.ID = uuid.New()
	}
	var err error
	params, _, err = withGrowthSpecification(params, time.Now().UTC())
	if err != nil {
		return db.SeoOpportunity{}, fmt.Errorf("compile Growth specification: %w", err)
	}
	if err := s.EnsureProjectCutover(ctx, params.ProjectID); err != nil {
		return db.SeoOpportunity{}, fmt.Errorf("cut over Growth reservations: %w", err)
	}
	candidate, identity, err := s.materializeCandidate(ctx, params)
	if err != nil {
		return db.SeoOpportunity{}, err
	}
	params.ExactSignatureHash = identity.ExactSignatureHash
	params.EvidenceFingerprint = candidate.Candidate.EvidenceFingerprint
	store := discovery.NewPostgresArbitrationStore(s.pool, s.q).WithWriterFenceToken(s.cutoverToken)
	for attempt := 0; attempt < 3; attempt++ {
		prepared, err := discovery.NewArbitrationService(store, s.comparator).Prepare(ctx, params.ProjectID, candidate.ID)
		if err != nil {
			return db.SeoOpportunity{}, fmt.Errorf("prepare Growth arbitration: %w", err)
		}
		switch prepared.Decision {
		case discovery.DecisionCreate, discovery.DecisionBlockOnOtherLine:
			result, err := discovery.NewReservationService(store).ReservePrepared(ctx, params.ProjectID, prepared.ID, OpportunityCreator{Opportunity: &params})
			if errors.Is(err, discovery.ErrSnapshotStale) {
				continue
			}
			if err != nil {
				return db.SeoOpportunity{}, fmt.Errorf("reserve Growth opportunity: %w", err)
			}
			return s.q.GetSEOOpportunity(ctx, db.GetSEOOpportunityParams{ID: result.Work.ID, ProjectID: params.ProjectID})
		case discovery.DecisionMergeEvidence:
			result, err := s.mergePreparedEvidence(ctx, prepared, params, false, "")
			if errors.Is(err, discovery.ErrSnapshotStale) {
				continue
			}
			return result, err
		default:
			if isInternalGrowthHold(candidate.Candidate, prepared) {
				// The candidate and its internal review item are durable. It is not
				// decision-ready, so keep it out of the user queue without failing
				// unrelated Opportunity Finding stages.
				return db.SeoOpportunity{}, nil
			}
			return db.SeoOpportunity{}, growthArbitrationOutcomeError(prepared)
		}
	}
	return db.SeoOpportunity{}, discovery.ErrSnapshotStale
}

type candidateReviewRequiredError struct {
	reason string
}

func (e candidateReviewRequiredError) Error() string {
	return fmt.Sprintf("%s: %s", ErrGrowthHeld, e.reason)
}

func (e candidateReviewRequiredError) Unwrap() []error {
	return []error{ErrGrowthHeld, discovery.ErrCandidateReviewRequired}
}

func growthArbitrationOutcomeError(prepared discovery.PreparedDecision) error {
	if prepared.Decision == discovery.DecisionSuppress ||
		(prepared.Decision == discovery.DecisionHold && prepared.Disposition == discovery.DispositionSemanticComparison) {
		return candidateReviewRequiredError{reason: prepared.Reason}
	}
	return fmt.Errorf("%w: %s", ErrGrowthHeld, prepared.Reason)
}

func isInternalGrowthHold(candidate discovery.Candidate, prepared discovery.PreparedDecision) bool {
	return prepared.Decision == discovery.DecisionHold &&
		(candidate.Status == discovery.StatusNeedsSpecification || candidate.Status == discovery.StatusNeedsEvidence)
}

func (s *Service) migrateLegacyGrowth(ctx context.Context, projectID uuid.UUID) error {
	legacy, err := s.q.ListActiveLegacyGrowthOpportunities(ctx, db.ListActiveLegacyGrowthOpportunitiesParams{
		ProjectID: projectID, LimitRows: 10000,
	})
	if err != nil {
		return err
	}
	if len(legacy) == 10000 {
		return fmt.Errorf("%w: legacy Growth migration exceeds the bounded batch", ErrGrowthHeld)
	}
	for index, opportunity := range legacy {
		s.cutoverSequence = int32(index + 1)
		if err := s.migrateLegacyOpportunity(ctx, opportunity); err != nil {
			return &growthCutoverWorkError{OpportunityID: opportunity.ID, Err: err}
		}
	}
	return nil
}

func (s *Service) migrateLegacyOpportunity(ctx context.Context, opportunity db.SeoOpportunity) error {
	store := discovery.NewPostgresArbitrationStore(s.pool, s.q).WithWriterFenceToken(s.cutoverToken)
	for attempt := 0; attempt < 3; attempt++ {
		params := canonicalParamsFromOpportunity(opportunity)
		target, err := s.q.GetLegacyGrowthIntendedTarget(ctx, db.GetLegacyGrowthIntendedTargetParams{
			ProjectID: opportunity.ProjectID, OpportunityID: opportunity.ID,
		})
		if err != nil {
			return err
		}
		targetSnapshot := getLegacyGrowthTargetSnapshot(target)
		params.Evidence, err = enrichLegacyGrowthEvidence(params.Evidence, resolveLegacyGrowthIntendedTarget(target))
		if err != nil {
			return err
		}
		candidate, _, err := s.materializeCutoverCandidate(ctx, params, opportunity)
		if err != nil {
			return err
		}
		params.EvidenceFingerprint = candidate.Candidate.EvidenceFingerprint
		prepared, err := discovery.NewArbitrationService(store, s.comparator).Prepare(ctx, opportunity.ProjectID, candidate.ID)
		if err != nil {
			return err
		}
		if err := s.recordCutoverPrepared(ctx, opportunity.ID, prepared); err != nil {
			return err
		}
		switch prepared.Decision {
		case discovery.DecisionCreate, discovery.DecisionBlockOnOtherLine:
			creator := OpportunityCreator{
				LegacyEvidence: params.Evidence, LegacyTargetSnapshot: targetSnapshot,
				CutoverBatchID: s.cutoverBatchID, CutoverSequence: s.cutoverSequence,
			}
			if _, err := discovery.NewReservationService(store).ReservePrepared(ctx, opportunity.ProjectID, prepared.ID, creator); errors.Is(err, discovery.ErrSnapshotStale) {
				continue
			} else if err != nil {
				return err
			}
			return nil
		case discovery.DecisionMergeEvidence:
			if _, err := s.mergePreparedEvidence(ctx, prepared, params, true, targetSnapshot); errors.Is(err, discovery.ErrSnapshotStale) {
				continue
			} else if err != nil {
				return err
			}
			return nil
		default:
			return fmt.Errorf("%w: legacy opportunity %s: %s", ErrGrowthHeld, opportunity.ID, prepared.Reason)
		}
	}
	return discovery.ErrSnapshotStale
}

func canonicalParamsFromOpportunity(opportunity db.SeoOpportunity) db.CreateCanonicalGrowthOpportunityParams {
	return db.CreateCanonicalGrowthOpportunityParams{
		ID: opportunity.ID, ProjectID: opportunity.ProjectID, Type: opportunity.Type,
		PriorityScore: opportunity.PriorityScore, Confidence: opportunity.Confidence,
		PageUrl: opportunity.PageUrl, NormalizedPageUrl: opportunity.NormalizedPageUrl,
		ArticleID: opportunity.ArticleID, TopicID: opportunity.TopicID, Query: opportunity.Query,
		Evidence: opportunity.Evidence, RecommendedAction: opportunity.RecommendedAction,
		ExpectedImpact: opportunity.ExpectedImpact, Effort: opportunity.Effort,
		RiskLevel: opportunity.RiskLevel, CreatedByRunID: opportunity.CreatedByRunID,
		GrowthSpecState: opportunity.GrowthSpecState, GrowthSpecVersion: opportunity.GrowthSpecVersion,
		GrowthSpec: opportunity.GrowthSpec, GrowthSpecMissing: opportunity.GrowthSpecMissing,
		DecisionReadyAt: opportunity.DecisionReadyAt,
	}
}

func (s *Service) materializeCandidate(ctx context.Context, params db.CreateCanonicalGrowthOpportunityParams) (discovery.ArbitrationCandidate, discovery.Identity, error) {
	candidate, identity, runID, err := projectGrowthCandidate(params)
	if err != nil {
		return discovery.ArbitrationCandidate{}, discovery.Identity{}, err
	}
	return persistGrowthCandidate(ctx, s.q, params.ProjectID, candidate, identity, runID)
}

func projectGrowthCandidate(params db.CreateCanonicalGrowthOpportunityParams) (discovery.Candidate, discovery.Identity, uuid.UUID, error) {
	opportunity := db.SeoOpportunity{
		ID: params.ID, ProjectID: params.ProjectID, Type: params.Type, Status: "open",
		PriorityScore: params.PriorityScore, Confidence: params.Confidence, PageUrl: params.PageUrl,
		NormalizedPageUrl: params.NormalizedPageUrl, ArticleID: params.ArticleID, TopicID: params.TopicID,
		Query: params.Query, Evidence: params.Evidence, RecommendedAction: params.RecommendedAction,
		ExpectedImpact: params.ExpectedImpact, Effort: params.Effort, RiskLevel: params.RiskLevel,
		CreatedByRunID: params.CreatedByRunID, CanonicalGrowth: params.GrowthSpecState != "legacy",
		GrowthSpecState: params.GrowthSpecState, GrowthSpecVersion: params.GrowthSpecVersion,
		GrowthSpecOrigin: "forward", GrowthSpec: params.GrowthSpec,
		GrowthSpecMissing: params.GrowthSpecMissing, DecisionReadyAt: params.DecisionReadyAt,
	}
	candidate := discovery.ProjectSEOOpportunity(opportunity)
	if candidate.SuggestedOwner != discovery.OwnerOpportunities || candidate.VerificationMode != discovery.VerificationDelayed {
		return discovery.Candidate{}, discovery.Identity{}, uuid.Nil, fmt.Errorf("%w: %s", ErrGrowthHeld, candidate.HoldReason)
	}
	if candidate.Status != discovery.StatusIdentityReady {
		runID := canonicalGrowthRunID(params.ProjectID, params.ID, "", candidate.EvidenceFingerprint)
		return candidate, discovery.Identity{}, runID, nil
	}
	identity, err := discovery.BuildIdentity(candidate)
	if err != nil {
		return discovery.Candidate{}, discovery.Identity{}, uuid.Nil, err
	}
	runID := canonicalGrowthRunID(params.ProjectID, params.ID, identity.ExactSignatureHash, candidate.EvidenceFingerprint)
	return candidate, identity, runID, nil
}

func persistGrowthCandidate(ctx context.Context, q *db.Queries, projectID uuid.UUID, candidate discovery.Candidate, identity discovery.Identity, runID uuid.UUID) (discovery.ArbitrationCandidate, discovery.Identity, error) {
	if _, err := q.EnsureCanonicalDiscoveryRun(ctx, db.EnsureCanonicalDiscoveryRunParams{
		ID: runID, ProjectID: projectID,
		CandidateSchemaVersion: discovery.CandidateSchemaVersionV1, SignatureVersion: discovery.SignatureVersionV1,
	}); err != nil {
		return discovery.ArbitrationCandidate{}, discovery.Identity{}, err
	}
	var identityRef *discovery.Identity
	if candidate.Status == discovery.StatusIdentityReady {
		identityRef = &identity
		for _, bucket := range identity.ConflictBucketKeys {
			if _, err := q.EnsureWorkConflictBucket(ctx, db.EnsureWorkConflictBucketParams{ProjectID: projectID, BucketKey: bucket}); err != nil {
				return discovery.ArbitrationCandidate{}, discovery.Identity{}, err
			}
		}
	}
	candidateID, err := discovery.NewPostgresRepository(q).SaveCandidate(ctx, runID, candidate, identityRef)
	if err != nil {
		return discovery.ArbitrationCandidate{}, discovery.Identity{}, err
	}
	if _, err := q.CompleteCanonicalDiscoveryRun(ctx, db.CompleteCanonicalDiscoveryRunParams{ID: runID, ProjectID: projectID}); err != nil {
		return discovery.ArbitrationCandidate{}, discovery.Identity{}, err
	}
	return discovery.ArbitrationCandidate{ID: candidateID, RunID: runID, Version: 1, Candidate: candidate, Identity: identity}, identity, nil
}

func (s *Service) materializeCutoverCandidate(ctx context.Context, params db.CreateCanonicalGrowthOpportunityParams, original db.SeoOpportunity) (result discovery.ArbitrationCandidate, identity discovery.Identity, resultErr error) {
	candidate, identity, runID, err := projectGrowthCandidate(params)
	if err != nil {
		return discovery.ArbitrationCandidate{}, discovery.Identity{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return discovery.ArbitrationCandidate{}, discovery.Identity{}, err
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	q := db.New(tx)
	result, identity, err = persistGrowthCandidate(ctx, q, params.ProjectID, candidate, identity, runID)
	if err != nil {
		return discovery.ArbitrationCandidate{}, discovery.Identity{}, err
	}
	before, _ := json.Marshal(original)
	after, _ := json.Marshal(map[string]any{"run_id": runID, "candidate_id": result.ID, "status": "materialized"})
	inverse, _ := json.Marshal(map[string]any{"operation": "tombstone_growth_cutover_provenance", "run_id": runID, "candidate_id": result.ID})
	if _, err := q.AppendGrowthCutoverSessionEntry(ctx, db.AppendGrowthCutoverSessionEntryParams{
		BatchID: s.cutoverBatchID, ProjectID: params.ProjectID, SequenceNumber: s.cutoverSequence,
		OpportunityID: params.ID, RunID: runID, CandidateID: result.ID,
		Disposition: "materialized", BeforeSnapshot: before, AfterSnapshot: after, InverseOperation: inverse,
	}); err != nil {
		return discovery.ArbitrationCandidate{}, discovery.Identity{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return discovery.ArbitrationCandidate{}, discovery.Identity{}, err
	}
	return result, identity, nil
}

func (s *Service) recordCutoverPrepared(ctx context.Context, opportunityID uuid.UUID, prepared discovery.PreparedDecision) error {
	after, _ := json.Marshal(prepared)
	inverse, _ := json.Marshal(map[string]any{"operation": "tombstone_growth_cutover_provenance", "candidate_id": prepared.CandidateID, "arbitration_decision_id": prepared.ID, "ai_call_id": prepared.AICallID})
	_, err := s.q.UpdateGrowthCutoverSessionEntryDecision(ctx, db.UpdateGrowthCutoverSessionEntryDecisionParams{
		ArbitrationDecisionID: pgUUID(prepared.ID), AiCallID: pgUUID(prepared.AICallID),
		Disposition: "materialized", EntryStatus: "applying", AfterSnapshot: after, InverseOperation: inverse,
		ProjectID: prepared.ProjectID, BatchID: s.cutoverBatchID, OpportunityID: opportunityID,
	})
	return err
}

func (s *Service) mergePreparedEvidence(ctx context.Context, prepared discovery.PreparedDecision, params db.CreateCanonicalGrowthOpportunityParams, aliasLegacy bool, legacyTargetSnapshot string) (db.SeoOpportunity, error) {
	switch prepared.Owner {
	case discovery.OwnerDoctor:
		return s.mergeDoctorEvidence(ctx, prepared, params, aliasLegacy, legacyTargetSnapshot)
	case discovery.OwnerOpportunities:
		return s.mergeExactEvidence(ctx, prepared, params, aliasLegacy, legacyTargetSnapshot)
	default:
		return db.SeoOpportunity{}, discovery.ErrSnapshotStale
	}
}

func (s *Service) mergeExactEvidence(ctx context.Context, prepared discovery.PreparedDecision, params db.CreateCanonicalGrowthOpportunityParams, aliasLegacy bool, legacyTargetSnapshot string) (result db.SeoOpportunity, resultErr error) {
	if len(prepared.OverlapWorkIDs) != 1 {
		return db.SeoOpportunity{}, discovery.ErrSnapshotStale
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return db.SeoOpportunity{}, err
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	if s.cutoverToken != uuid.Nil {
		if _, err := tx.Exec(ctx, `select set_config('citeloop.growth_cutover_fence_token', $1, true)`, s.cutoverToken.String()); err != nil {
			return db.SeoOpportunity{}, err
		}
	}
	q := db.New(tx)
	authority, err := q.LockProductWriterAuthority(ctx, db.LockProductWriterAuthorityParams{ProjectID: params.ProjectID, Product: "opportunities"})
	fenceAuthorized := s.cutoverToken != uuid.Nil && authority.WriteFenced && authority.FenceToken.Valid && authority.FenceToken.Bytes == s.cutoverToken
	if err != nil || authority.WriterAuthority != "canonical" || (authority.WriteFenced && !fenceAuthorized) {
		return db.SeoOpportunity{}, discovery.ErrWriterUnavailable
	}
	keys := make([]string, 0, len(prepared.ExpectedBucketVersions))
	for key := range prepared.ExpectedBucketVersions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	buckets, err := q.LockConflictBucketsForReserve(ctx, db.LockConflictBucketsForReserveParams{ProjectID: params.ProjectID, BucketKeys: keys})
	if err != nil || len(buckets) != len(keys) {
		return db.SeoOpportunity{}, discovery.ErrSnapshotStale
	}
	for _, bucket := range buckets {
		if prepared.ExpectedBucketVersions[bucket.BucketKey] != bucket.BucketVersion {
			return db.SeoOpportunity{}, discovery.ErrSnapshotStale
		}
	}
	decision, err := q.LockArbitrationDecisionForReserve(ctx, db.LockArbitrationDecisionForReserveParams{ProjectID: params.ProjectID, ID: prepared.ID})
	if err != nil || decision.Status != string(discovery.ArbitrationStatusPrepared) || decision.CandidateVersion != prepared.CandidateVersion {
		return db.SeoOpportunity{}, discovery.ErrSnapshotStale
	}
	candidate, err := q.LockDiscoveryCandidateForReserve(ctx, db.LockDiscoveryCandidateForReserveParams{ProjectID: params.ProjectID, CandidateID: prepared.CandidateID})
	if err != nil || candidate.CandidateVersion != prepared.CandidateVersion || candidate.ExactSignatureHash == nil || *candidate.ExactSignatureHash != prepared.ExactSignatureHash {
		return db.SeoOpportunity{}, discovery.ErrSnapshotStale
	}
	if aliasLegacy {
		if err := revalidateLegacyGrowthTarget(ctx, q, params.ProjectID, params.ID, legacyTargetSnapshot); err != nil {
			return db.SeoOpportunity{}, err
		}
	}
	mergedID := uuid.Nil
	if aliasLegacy {
		signatureBefore, err := q.GetWorkSignatureForGrowthEvidenceMergeForUpdate(ctx, db.GetWorkSignatureForGrowthEvidenceMergeForUpdateParams{
			ProjectID: params.ProjectID, WorkSignatureID: prepared.OverlapWorkIDs[0],
		})
		if err != nil {
			return db.SeoOpportunity{}, err
		}
		canonicalBefore, err := q.GetCanonicalGrowthOpportunityByWorkSignatureForUpdate(ctx, db.GetCanonicalGrowthOpportunityByWorkSignatureForUpdateParams{
			ProjectID: params.ProjectID, WorkSignatureID: prepared.OverlapWorkIDs[0],
		})
		if err != nil {
			return db.SeoOpportunity{}, err
		}
		chain, err := s.reconcileDuplicateExecutionChain(ctx, q, discovery.OwnerOpportunities, params.ProjectID, params.ID, pgUUID(canonicalBefore.ID))
		if err != nil {
			var review *growthCutoverReviewError
			if errors.As(err, &review) {
				return db.SeoOpportunity{}, s.persistExecutionChainReview(ctx, tx, q, prepared, params, review, canonicalBefore)
			}
			return db.SeoOpportunity{}, err
		}
		merged, err := q.MergeCanonicalGrowthOpportunityEvidence(ctx, db.MergeCanonicalGrowthOpportunityEvidenceParams{
			ProjectID: params.ProjectID, WorkSignatureID: prepared.OverlapWorkIDs[0], Evidence: params.Evidence,
			IncomingPriorityScore: params.PriorityScore, IncomingConfidence: params.Confidence,
			EvidenceFingerprint: prepared.EvidenceFingerprint, GrowthSpecState: params.GrowthSpecState,
			GrowthSpecVersion: params.GrowthSpecVersion, GrowthSpec: params.GrowthSpec,
			GrowthSpecMissing: params.GrowthSpecMissing, DecisionReadyAt: params.DecisionReadyAt,
		})
		if err != nil {
			return db.SeoOpportunity{}, err
		}
		alias, err := q.CreateDuplicateGrowthOpportunityAlias(ctx, db.CreateDuplicateGrowthOpportunityAliasParams{
			ProjectID: params.ProjectID, LegacyOpportunityID: params.ID,
			WorkSignatureID: prepared.OverlapWorkIDs[0],
		})
		if err != nil {
			return db.SeoOpportunity{}, err
		}
		if !alias.CanonicalOpportunityID.Valid {
			return db.SeoOpportunity{}, discovery.ErrSnapshotStale
		}
		mergedID = alias.CanonicalOpportunityID.Bytes
		beforeSnapshot, _ := json.Marshal(map[string]any{"duplicate": params, "canonical": canonicalBefore, "canonical_signature_evidence_fingerprint": signatureBefore.EvidenceFingerprint, "execution_chain": json.RawMessage(chain.Snapshot)})
		afterSnapshot, _ := json.Marshal(map[string]any{"alias": alias, "canonical": merged, "repointed_content_action_ids": chain.RepointedActionIDs})
		inverse, _ := json.Marshal(map[string]any{"operation": "tombstone_duplicate_and_restore_canonical", "canonical": canonicalBefore, "source_opportunity_id": params.ID, "canonical_opportunity_id": canonicalBefore.ID, "content_action_ids": chain.RepointedActionIDs})
		if _, err := q.UpdateGrowthCutoverSessionEntryDecision(ctx, db.UpdateGrowthCutoverSessionEntryDecisionParams{
			BatchID: s.cutoverBatchID, ProjectID: params.ProjectID, OpportunityID: params.ID,
			ArbitrationDecisionID: pgUUID(prepared.ID), AiCallID: pgUUID(prepared.AICallID),
			WorkSignatureID: pgUUID(prepared.OverlapWorkIDs[0]), Disposition: "duplicate", EntryStatus: "applied",
			BeforeSnapshot: beforeSnapshot, AfterSnapshot: afterSnapshot, InverseOperation: inverse,
		}); err != nil {
			return db.SeoOpportunity{}, err
		}
	} else {
		merged, err := q.MergeCanonicalGrowthOpportunityEvidence(ctx, db.MergeCanonicalGrowthOpportunityEvidenceParams{
			ProjectID: params.ProjectID, WorkSignatureID: prepared.OverlapWorkIDs[0], Evidence: params.Evidence,
			IncomingPriorityScore: params.PriorityScore, IncomingConfidence: params.Confidence,
			EvidenceFingerprint: prepared.EvidenceFingerprint, GrowthSpecState: params.GrowthSpecState,
			GrowthSpecVersion: params.GrowthSpecVersion, GrowthSpec: params.GrowthSpec,
			GrowthSpecMissing: params.GrowthSpecMissing, DecisionReadyAt: params.DecisionReadyAt,
		})
		if err != nil {
			return db.SeoOpportunity{}, err
		}
		mergedID = merged.ID
	}
	if updated, err := q.IncrementConflictBucketVersions(ctx, db.IncrementConflictBucketVersionsParams{ProjectID: params.ProjectID, BucketKeys: keys}); err != nil || len(updated) != len(keys) {
		return db.SeoOpportunity{}, discovery.ErrSnapshotStale
	}
	if _, err := q.MarkArbitrationDecisionResolved(ctx, db.MarkArbitrationDecisionResolvedParams{ProjectID: params.ProjectID, ID: prepared.ID}); err != nil {
		return db.SeoOpportunity{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.SeoOpportunity{}, err
	}
	return s.q.GetSEOOpportunity(ctx, db.GetSEOOpportunityParams{ID: mergedID, ProjectID: params.ProjectID})
}

func (s *Service) mergeDoctorEvidence(ctx context.Context, prepared discovery.PreparedDecision, params db.CreateCanonicalGrowthOpportunityParams, aliasLegacy bool, legacyTargetSnapshot string) (result db.SeoOpportunity, resultErr error) {
	if len(prepared.OverlapWorkIDs) != 1 {
		return db.SeoOpportunity{}, discovery.ErrSnapshotStale
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return db.SeoOpportunity{}, err
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	if s.cutoverToken != uuid.Nil {
		if _, err := tx.Exec(ctx, `select set_config('citeloop.growth_cutover_fence_token', $1, true)`, s.cutoverToken.String()); err != nil {
			return db.SeoOpportunity{}, err
		}
	}
	q := db.New(tx)
	authority, err := q.LockProductWriterAuthority(ctx, db.LockProductWriterAuthorityParams{ProjectID: params.ProjectID, Product: "opportunities"})
	fenceAuthorized := s.cutoverToken != uuid.Nil && authority.WriteFenced && authority.FenceToken.Valid && authority.FenceToken.Bytes == s.cutoverToken
	if err != nil || authority.WriterAuthority != "canonical" || (authority.WriteFenced && !fenceAuthorized) {
		return db.SeoOpportunity{}, discovery.ErrWriterUnavailable
	}
	doctorAuthority, err := q.LockProductWriterAuthority(ctx, db.LockProductWriterAuthorityParams{ProjectID: params.ProjectID, Product: "doctor"})
	if err != nil || doctorAuthority.WriterAuthority != "canonical" || doctorAuthority.WriteFenced {
		return db.SeoOpportunity{}, discovery.ErrWriterUnavailable
	}
	keys := make([]string, 0, len(prepared.ExpectedBucketVersions))
	for key := range prepared.ExpectedBucketVersions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	buckets, err := q.LockConflictBucketsForReserve(ctx, db.LockConflictBucketsForReserveParams{ProjectID: params.ProjectID, BucketKeys: keys})
	if err != nil || len(buckets) != len(keys) {
		return db.SeoOpportunity{}, discovery.ErrSnapshotStale
	}
	for _, bucket := range buckets {
		if prepared.ExpectedBucketVersions[bucket.BucketKey] != bucket.BucketVersion {
			return db.SeoOpportunity{}, discovery.ErrSnapshotStale
		}
	}
	decision, err := q.LockArbitrationDecisionForReserve(ctx, db.LockArbitrationDecisionForReserveParams{ProjectID: params.ProjectID, ID: prepared.ID})
	if err != nil || decision.Status != string(discovery.ArbitrationStatusPrepared) || decision.CandidateVersion != prepared.CandidateVersion {
		return db.SeoOpportunity{}, discovery.ErrSnapshotStale
	}
	candidate, err := q.LockDiscoveryCandidateForReserve(ctx, db.LockDiscoveryCandidateForReserveParams{ProjectID: params.ProjectID, CandidateID: prepared.CandidateID})
	if err != nil || candidate.CandidateVersion != prepared.CandidateVersion {
		return db.SeoOpportunity{}, discovery.ErrSnapshotStale
	}
	if aliasLegacy {
		if err := revalidateLegacyGrowthTarget(ctx, q, params.ProjectID, params.ID, legacyTargetSnapshot); err != nil {
			return db.SeoOpportunity{}, err
		}
	}
	signatureBefore, err := q.GetWorkSignatureForGrowthEvidenceMergeForUpdate(ctx, db.GetWorkSignatureForGrowthEvidenceMergeForUpdateParams{ProjectID: params.ProjectID, WorkSignatureID: prepared.OverlapWorkIDs[0]})
	if err != nil {
		return db.SeoOpportunity{}, err
	}
	before, err := q.GetCanonicalSiteFixByWorkSignatureForUpdate(ctx, db.GetCanonicalSiteFixByWorkSignatureForUpdateParams{ProjectID: params.ProjectID, WorkSignatureID: prepared.OverlapWorkIDs[0]})
	if err != nil {
		return db.SeoOpportunity{}, err
	}
	if aliasLegacy {
		if _, err := s.reconcileDuplicateExecutionChain(ctx, q, discovery.OwnerDoctor, params.ProjectID, params.ID, pgtype.UUID{}); err != nil {
			var review *growthCutoverReviewError
			if errors.As(err, &review) {
				return db.SeoOpportunity{}, s.persistExecutionChainReview(ctx, tx, q, prepared, params, review, before)
			}
			return db.SeoOpportunity{}, err
		}
	}
	merged, err := q.MergeCanonicalDoctorSiteFixEvidence(ctx, db.MergeCanonicalDoctorSiteFixEvidenceParams{
		ProjectID: params.ProjectID, WorkSignatureID: prepared.OverlapWorkIDs[0], Evidence: params.Evidence,
		IncomingPriorityScore: params.PriorityScore, IncomingConfidence: params.Confidence,
		SourceOpportunityID: params.ID, EvidenceFingerprint: prepared.EvidenceFingerprint,
	})
	if err != nil {
		return db.SeoOpportunity{}, err
	}
	if aliasLegacy {
		alias, err := q.CreateDoctorGrowthEvidenceAlias(ctx, db.CreateDoctorGrowthEvidenceAliasParams{
			ProjectID: params.ProjectID, LegacyOpportunityID: params.ID, WorkSignatureID: prepared.OverlapWorkIDs[0],
		})
		if err != nil {
			return db.SeoOpportunity{}, err
		}
		beforeSnapshot, _ := json.Marshal(map[string]any{"duplicate": params, "canonical_site_fix": before, "canonical_signature_evidence_fingerprint": signatureBefore.EvidenceFingerprint})
		afterSnapshot, _ := json.Marshal(map[string]any{"alias": alias, "canonical_site_fix": merged})
		inverse, _ := json.Marshal(map[string]any{"operation": "tombstone_doctor_merge_and_restore_site_fix", "canonical_site_fix": before})
		if _, err := q.UpdateGrowthCutoverSessionEntryDecision(ctx, db.UpdateGrowthCutoverSessionEntryDecisionParams{
			BatchID: s.cutoverBatchID, ProjectID: params.ProjectID, OpportunityID: params.ID,
			ArbitrationDecisionID: pgUUID(prepared.ID), AiCallID: pgUUID(prepared.AICallID),
			WorkSignatureID: pgUUID(prepared.OverlapWorkIDs[0]), Disposition: "doctor_merge", EntryStatus: "applied",
			BeforeSnapshot: beforeSnapshot, AfterSnapshot: afterSnapshot, InverseOperation: inverse,
		}); err != nil {
			return db.SeoOpportunity{}, err
		}
	}
	if updated, err := q.IncrementConflictBucketVersions(ctx, db.IncrementConflictBucketVersionsParams{ProjectID: params.ProjectID, BucketKeys: keys}); err != nil || len(updated) != len(keys) {
		return db.SeoOpportunity{}, discovery.ErrSnapshotStale
	}
	if _, err := q.MarkArbitrationDecisionResolved(ctx, db.MarkArbitrationDecisionResolvedParams{ProjectID: params.ProjectID, ID: prepared.ID}); err != nil {
		return db.SeoOpportunity{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.SeoOpportunity{}, err
	}
	return db.SeoOpportunity{}, nil
}

func (s *Service) completeGrowthCutover(ctx context.Context, session db.GrowthCutoverSession) (resultErr error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	if _, err := tx.Exec(ctx, `select set_config('citeloop.growth_cutover_fence_token', $1, true)`, session.FenceToken.String()); err != nil {
		return err
	}
	q := db.New(tx)
	authority, err := q.LockProductWriterAuthority(ctx, db.LockProductWriterAuthorityParams{ProjectID: session.ProjectID, Product: "opportunities"})
	if err != nil || authority.WriterAuthority != "canonical" || !authority.WriteFenced || !authority.FenceToken.Valid || authority.FenceToken.Bytes != session.FenceToken {
		return discovery.ErrWriterUnavailable
	}
	missing, err := q.CountUnrepresentedActiveLegacyGrowth(ctx, session.ProjectID)
	if err != nil {
		return err
	}
	if missing != 0 {
		return fmt.Errorf("%w: %d active legacy Growth rows remain unrepresented", ErrGrowthHeld, missing)
	}
	entries, err := q.ListGrowthCutoverSessionEntries(ctx, db.ListGrowthCutoverSessionEntriesParams{ProjectID: session.ProjectID, BatchID: session.BatchID})
	if err != nil {
		return err
	}
	if err := appendGrowthCutoverAudit(ctx, q, session, entries, "completed", "canonical", "", uuid.Nil); err != nil {
		return err
	}
	if _, err := q.FinishGrowthCutoverSession(ctx, db.FinishGrowthCutoverSessionParams{
		Status: "completed", ProjectID: session.ProjectID, BatchID: session.BatchID,
	}); err != nil {
		return err
	}
	if _, err := q.ReleaseProductWriterFence(ctx, db.ReleaseProductWriterFenceParams{
		ProjectID: session.ProjectID, Product: "opportunities", FenceToken: pgUUID(session.FenceToken),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) rollbackGrowthCutover(ctx context.Context, session db.GrowthCutoverSession, cause error, failedOpportunityID uuid.UUID) (resultErr error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	if _, err := tx.Exec(ctx, `select set_config('citeloop.growth_cutover_fence_token', $1, true)`, session.FenceToken.String()); err != nil {
		return err
	}
	q := db.New(tx)
	authority, err := q.LockProductWriterAuthority(ctx, db.LockProductWriterAuthorityParams{ProjectID: session.ProjectID, Product: "opportunities"})
	if err != nil || authority.WriterAuthority != "canonical" || !authority.WriteFenced || !authority.FenceToken.Valid || authority.FenceToken.Bytes != session.FenceToken {
		return discovery.ErrWriterUnavailable
	}
	entries, err := q.ListGrowthCutoverSessionEntries(ctx, db.ListGrowthCutoverSessionEntriesParams{ProjectID: session.ProjectID, BatchID: session.BatchID})
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Disposition == "materialized" {
			tombstoned, err := q.RollbackGrowthCutoverMaterialized(ctx, db.RollbackGrowthCutoverMaterializedParams{
				ProjectID: session.ProjectID, CandidateID: entry.CandidateID, RunID: entry.RunID,
			})
			if err != nil || tombstoned != 1 {
				return fmt.Errorf("rollback materialized Growth %s: tombstoned=%d err=%w", entry.OpportunityID, tombstoned, err)
			}
			continue
		}
		if entry.Disposition == "duplicate" || entry.Disposition == "doctor_merge" {
			var before struct {
				Canonical                             db.SeoOpportunity `json:"canonical"`
				CanonicalSiteFix                      db.SiteFix        `json:"canonical_site_fix"`
				CanonicalSignatureEvidenceFingerprint string            `json:"canonical_signature_evidence_fingerprint"`
			}
			if err := json.Unmarshal(entry.BeforeSnapshot, &before); err != nil {
				return fmt.Errorf("decode Growth rollback snapshot: %w", err)
			}
			var inverse struct {
				SourceOpportunityID    uuid.UUID   `json:"source_opportunity_id"`
				CanonicalOpportunityID uuid.UUID   `json:"canonical_opportunity_id"`
				ContentActionIDs       []uuid.UUID `json:"content_action_ids"`
			}
			if err := json.Unmarshal(entry.InverseOperation, &inverse); err != nil {
				return fmt.Errorf("decode Growth execution-chain inverse: %w", err)
			}
			canonicalEvidence := before.Canonical.Evidence
			canonicalOpportunityID := pgUUID(before.Canonical.ID)
			canonicalSiteFixID := pgUUID(before.CanonicalSiteFix.ID)
			if entry.Disposition == "doctor_merge" {
				canonicalEvidence = before.CanonicalSiteFix.EvidenceSnapshot
			}
			removed, err := q.RollbackGrowthCutoverDuplicate(ctx, db.RollbackGrowthCutoverDuplicateParams{
				ProjectID: session.ProjectID, OpportunityID: entry.OpportunityID,
				CanonicalEvidence: canonicalEvidence, CanonicalPriorityScore: before.Canonical.PriorityScore,
				CanonicalConfidence: before.Canonical.Confidence, CanonicalEvidenceFingerprint: &before.CanonicalSignatureEvidenceFingerprint,
				CanonicalOpportunityID: canonicalOpportunityID, CanonicalSiteFixID: canonicalSiteFixID,
				CandidateID: entry.CandidateID, RunID: entry.RunID,
			})
			if err != nil || removed != 1 {
				return fmt.Errorf("rollback Growth duplicate %s: removed=%d err=%w", entry.OpportunityID, removed, err)
			}
			if len(inverse.ContentActionIDs) > 0 {
				restored, err := q.RestoreGrowthContentActionRepoints(ctx, db.RestoreGrowthContentActionRepointsParams{
					SourceOpportunityID: inverse.SourceOpportunityID, ProjectID: session.ProjectID,
					CanonicalOpportunityID: inverse.CanonicalOpportunityID, ContentActionIds: inverse.ContentActionIDs,
				})
				if err != nil || restored != int64(len(inverse.ContentActionIDs)) {
					return fmt.Errorf("restore Growth execution descendants %s: restored=%d want=%d err=%w", entry.OpportunityID, restored, len(inverse.ContentActionIDs), err)
				}
			}
			continue
		}
		if !entry.WorkSignatureID.Valid {
			return fmt.Errorf("rollback canonical Growth %s: missing work signature", entry.OpportunityID)
		}
		var before db.SeoOpportunity
		if err := json.Unmarshal(entry.BeforeSnapshot, &before); err != nil {
			return fmt.Errorf("decode canonical Growth rollback snapshot: %w", err)
		}
		removed, err := q.RollbackGrowthCutoverCanonical(ctx, db.RollbackGrowthCutoverCanonicalParams{
			ProjectID: session.ProjectID, WorkSignatureID: entry.WorkSignatureID.Bytes,
			CandidateID: entry.CandidateID, OpportunityID: pgUUID(entry.OpportunityID),
			OriginalEvidence: before.Evidence, OriginalEvidenceFingerprint: before.EvidenceFingerprint,
			RunID: entry.RunID,
		})
		if err != nil || removed != 1 {
			return fmt.Errorf("rollback canonical Growth %s: removed=%d err=%w", entry.OpportunityID, removed, err)
		}
	}
	if _, err := q.MarkGrowthCutoverSessionEntriesRolledBack(ctx, db.MarkGrowthCutoverSessionEntriesRolledBackParams{
		ProjectID: session.ProjectID, BatchID: session.BatchID,
	}); err != nil {
		return err
	}
	message := cause.Error()
	if err := appendGrowthCutoverAudit(ctx, q, session, entries, "rolled_back", "legacy", message, failedOpportunityID); err != nil {
		return err
	}
	if _, err := q.SwitchProductWriterAuthority(ctx, db.SwitchProductWriterAuthorityParams{
		WriterAuthority: "legacy", AuthorityChangedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		ProjectID: session.ProjectID, Product: "opportunities", FenceToken: pgUUID(session.FenceToken),
		ExpectedWriterAuthority: "canonical",
	}); err != nil {
		return err
	}
	if _, err := q.FinishGrowthCutoverSession(ctx, db.FinishGrowthCutoverSessionParams{
		Status: "rolled_back", Error: &message, ProjectID: session.ProjectID, BatchID: session.BatchID,
	}); err != nil {
		return err
	}
	if _, err := q.ReleaseProductWriterFence(ctx, db.ReleaseProductWriterFenceParams{
		ProjectID: session.ProjectID, Product: "opportunities", FenceToken: pgUUID(session.FenceToken),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func appendGrowthCutoverAudit(ctx context.Context, q *db.Queries, session db.GrowthCutoverSession, entries []db.GrowthCutoverSessionEntry, status, authorityAfter, message string, reviewOpportunityID uuid.UUID) error {
	migrated, duplicates := int32(0), int32(0)
	for _, entry := range entries {
		if entry.Disposition == "duplicate" || entry.Disposition == "doctor_merge" {
			duplicates++
		} else {
			migrated++
		}
	}
	reviewCount := int32(0)
	if reviewOpportunityID != uuid.Nil {
		reviewCount = 1
	}
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	sourceSnapshot, _ := json.Marshal(map[string]any{"entry_count": len(entries), "failed_opportunity_id": reviewOpportunityID})
	resultSnapshot, _ := json.Marshal(map[string]any{"status": status, "error": message, "conserved": status == "completed"})
	batchKind := "forward"
	if status == "rolled_back" {
		batchKind = "rollback"
	}
	if _, err := q.CreateMigrationBatch(ctx, db.CreateMigrationBatchParams{
		ID: session.BatchID, ProjectID: session.ProjectID, Product: "opportunities",
		BatchKind: batchKind, Status: status, SchemaVersion: "growth-cutover-v1",
		SourceCount: migrated + duplicates + reviewCount, MigratedCount: migrated,
		ArchivedDuplicateCount: duplicates, ReviewCount: reviewCount,
		WriterAuthorityBefore: "legacy", WriterAuthorityAfter: authorityAfter,
		SourceSnapshot: sourceSnapshot, ResultSnapshot: resultSnapshot,
		InitiatedBy: "growth_cutover", StartedAt: session.StartedAt, FinishedAt: now,
	}); err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].SequenceNumber < entries[j].SequenceNumber })
	for _, entry := range entries {
		operation := "create"
		if entry.Disposition == "duplicate" || entry.Disposition == "doctor_merge" {
			operation = "archive_duplicate"
		}
		cutoverPoint := "canonical_authority"
		if status == "rolled_back" {
			operation = "rollback"
			cutoverPoint = "rollback"
		}
		if _, err := q.AppendMigrationLedger(ctx, db.AppendMigrationLedgerParams{
			ID: uuid.New(), ProjectID: session.ProjectID, MigrationBatchID: session.BatchID,
			SequenceNumber: entry.SequenceNumber, SourceObjectType: "seo_opportunity",
			SourceObjectID: entry.OpportunityID, CanonicalObjectType: "work_signature_registry",
			CanonicalObjectID: entry.WorkSignatureID, Operation: operation,
			OperationVersion: "growth-cutover-v1", CutoverPoint: cutoverPoint,
			RollbackEligibility: "eligible", BeforeHash: hashJSON(entry.BeforeSnapshot),
			AfterHash: hashJSON(entry.AfterSnapshot), BeforeSnapshot: entry.BeforeSnapshot,
			AfterSnapshot: entry.AfterSnapshot, InverseOperationVersion: "growth-cutover-v1",
			InverseOperation: entry.InverseOperation, AppliedBy: "growth_cutover", AppliedAt: now,
		}); err != nil {
			return err
		}
	}
	if reviewOpportunityID != uuid.Nil {
		snapshot, _ := json.Marshal(map[string]any{"opportunity_id": reviewOpportunityID, "error": message})
		if _, err := q.CreateMigrationReviewItem(ctx, db.CreateMigrationReviewItemParams{
			ID: uuid.New(), ProjectID: session.ProjectID, MigrationBatchID: session.BatchID,
			SourceObjectType: "seo_opportunity", SourceObjectID: reviewOpportunityID,
			ReasonCode: "growth_arbitration_review", Reason: message,
			SourceSnapshot: snapshot, ProposedResolution: json.RawMessage(`{"action":"resolve arbitration before retry"}`),
		}); err != nil {
			return err
		}
	}
	if status == "rolled_back" {
		resolvedBy := "growth_cutover"
		if _, err := q.DismissPendingMigrationReviewItemsForBatch(ctx, db.DismissPendingMigrationReviewItemsForBatchParams{
			ProjectID: session.ProjectID, MigrationBatchID: session.BatchID,
			ResolvedBy: &resolvedBy, ResolvedAt: now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func hashJSON(raw json.RawMessage) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func canonicalGrowthRunID(projectID, opportunityID uuid.UUID, exactHash, evidenceFingerprint string) uuid.UUID {
	payload, _ := json.Marshal([]string{projectID.String(), opportunityID.String(), exactHash, evidenceFingerprint})
	sum := sha256.Sum256(payload)
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("citeloop:canonical-growth:"+hex.EncodeToString(sum[:])))
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil}
}
