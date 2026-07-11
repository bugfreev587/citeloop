package growthwork

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
			result, err := s.mergeExactEvidence(ctx, prepared, params, false)
			if errors.Is(err, discovery.ErrSnapshotStale) {
				continue
			}
			return result, err
		default:
			return db.SeoOpportunity{}, fmt.Errorf("%w: %s", ErrGrowthHeld, prepared.Reason)
		}
	}
	return db.SeoOpportunity{}, discovery.ErrSnapshotStale
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
	params := canonicalParamsFromOpportunity(opportunity)
	candidate, _, err := s.materializeCandidate(ctx, params)
	if err != nil {
		return err
	}
	store := discovery.NewPostgresArbitrationStore(s.pool, s.q).WithWriterFenceToken(s.cutoverToken)
	for attempt := 0; attempt < 3; attempt++ {
		prepared, err := discovery.NewArbitrationService(store, s.comparator).Prepare(ctx, opportunity.ProjectID, candidate.ID)
		if err != nil {
			return err
		}
		switch prepared.Decision {
		case discovery.DecisionCreate, discovery.DecisionBlockOnOtherLine:
			creator := OpportunityCreator{CutoverBatchID: s.cutoverBatchID, CutoverSequence: s.cutoverSequence}
			if _, err := discovery.NewReservationService(store).ReservePrepared(ctx, opportunity.ProjectID, prepared.ID, creator); errors.Is(err, discovery.ErrSnapshotStale) {
				continue
			} else if err != nil {
				return err
			}
			return nil
		case discovery.DecisionMergeEvidence:
			if _, err := s.mergeExactEvidence(ctx, prepared, params, true); errors.Is(err, discovery.ErrSnapshotStale) {
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
	}
}

func (s *Service) materializeCandidate(ctx context.Context, params db.CreateCanonicalGrowthOpportunityParams) (discovery.ArbitrationCandidate, discovery.Identity, error) {
	opportunity := db.SeoOpportunity{
		ID: params.ID, ProjectID: params.ProjectID, Type: params.Type, Status: "open",
		PriorityScore: params.PriorityScore, Confidence: params.Confidence, PageUrl: params.PageUrl,
		NormalizedPageUrl: params.NormalizedPageUrl, ArticleID: params.ArticleID, TopicID: params.TopicID,
		Query: params.Query, Evidence: params.Evidence, RecommendedAction: params.RecommendedAction,
		ExpectedImpact: params.ExpectedImpact, Effort: params.Effort, RiskLevel: params.RiskLevel,
		CreatedByRunID: params.CreatedByRunID,
	}
	candidate := discovery.ProjectSEOOpportunity(opportunity)
	if candidate.Status != discovery.StatusIdentityReady || candidate.SuggestedOwner != discovery.OwnerOpportunities || candidate.VerificationMode != discovery.VerificationDelayed {
		return discovery.ArbitrationCandidate{}, discovery.Identity{}, fmt.Errorf("%w: %s", ErrGrowthHeld, candidate.HoldReason)
	}
	identity, err := discovery.BuildIdentity(candidate)
	if err != nil {
		return discovery.ArbitrationCandidate{}, discovery.Identity{}, err
	}
	runID := canonicalGrowthRunID(params.ProjectID, params.ID, identity.ExactSignatureHash, candidate.EvidenceFingerprint)
	if _, err := s.q.EnsureCanonicalDiscoveryRun(ctx, db.EnsureCanonicalDiscoveryRunParams{
		ID: runID, ProjectID: params.ProjectID,
		CandidateSchemaVersion: discovery.CandidateSchemaVersionV1, SignatureVersion: discovery.SignatureVersionV1,
	}); err != nil {
		return discovery.ArbitrationCandidate{}, discovery.Identity{}, err
	}
	for _, bucket := range identity.ConflictBucketKeys {
		if _, err := s.q.EnsureWorkConflictBucket(ctx, db.EnsureWorkConflictBucketParams{ProjectID: params.ProjectID, BucketKey: bucket}); err != nil {
			return discovery.ArbitrationCandidate{}, discovery.Identity{}, err
		}
	}
	candidateID, err := discovery.NewPostgresRepository(s.q).SaveCandidate(ctx, runID, candidate, &identity)
	if err != nil {
		return discovery.ArbitrationCandidate{}, discovery.Identity{}, err
	}
	if _, err := s.q.CompleteCanonicalDiscoveryRun(ctx, db.CompleteCanonicalDiscoveryRunParams{ID: runID, ProjectID: params.ProjectID}); err != nil {
		return discovery.ArbitrationCandidate{}, discovery.Identity{}, err
	}
	return discovery.ArbitrationCandidate{ID: candidateID, RunID: runID, Version: 1, Candidate: candidate, Identity: identity}, identity, nil
}

func (s *Service) mergeExactEvidence(ctx context.Context, prepared discovery.PreparedDecision, params db.CreateCanonicalGrowthOpportunityParams, aliasLegacy bool) (result db.SeoOpportunity, resultErr error) {
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
	mergedID := uuid.Nil
	if aliasLegacy {
		alias, err := q.CreateDuplicateGrowthOpportunityAlias(ctx, db.CreateDuplicateGrowthOpportunityAliasParams{
			ProjectID: params.ProjectID, LegacyOpportunityID: params.ID,
			WorkSignatureID: prepared.OverlapWorkIDs[0],
		})
		if err != nil {
			return db.SeoOpportunity{}, err
		}
		mergedID = alias.CanonicalOpportunityID
		beforeSnapshot, _ := json.Marshal(map[string]any{"opportunity_id": params.ID, "canonical_growth": false, "evidence": params.Evidence})
		afterSnapshot, _ := json.Marshal(alias)
		inverse, _ := json.Marshal(map[string]any{"operation": "delete_growth_duplicate_alias", "legacy_opportunity_id": params.ID})
		if _, err := q.AppendGrowthCutoverSessionEntry(ctx, db.AppendGrowthCutoverSessionEntryParams{
			BatchID: s.cutoverBatchID, ProjectID: params.ProjectID, SequenceNumber: s.cutoverSequence,
			OpportunityID: params.ID, CandidateID: prepared.CandidateID,
			WorkSignatureID: prepared.OverlapWorkIDs[0], Disposition: "duplicate",
			BeforeSnapshot: beforeSnapshot, AfterSnapshot: afterSnapshot, InverseOperation: inverse,
		}); err != nil {
			return db.SeoOpportunity{}, err
		}
	} else {
		merged, err := q.MergeCanonicalGrowthOpportunityEvidence(ctx, db.MergeCanonicalGrowthOpportunityEvidenceParams{
			ProjectID: params.ProjectID, WorkSignatureID: prepared.OverlapWorkIDs[0], Evidence: params.Evidence,
			IncomingPriorityScore: params.PriorityScore, IncomingConfidence: params.Confidence,
			EvidenceFingerprint: prepared.EvidenceFingerprint,
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
		if entry.Disposition == "duplicate" {
			removed, err := q.RollbackGrowthCutoverDuplicate(ctx, db.RollbackGrowthCutoverDuplicateParams{
				ProjectID: session.ProjectID, OpportunityID: entry.OpportunityID,
				WorkSignatureID: entry.WorkSignatureID, CandidateID: entry.CandidateID,
			})
			if err != nil || removed != 1 {
				return fmt.Errorf("rollback Growth duplicate %s: removed=%d err=%w", entry.OpportunityID, removed, err)
			}
			continue
		}
		removed, err := q.RollbackGrowthCutoverCanonical(ctx, db.RollbackGrowthCutoverCanonicalParams{
			ProjectID: session.ProjectID, WorkSignatureID: entry.WorkSignatureID,
			CandidateID: entry.CandidateID, OpportunityID: pgUUID(entry.OpportunityID),
		})
		if err != nil || removed != 1 {
			return fmt.Errorf("rollback canonical Growth %s: removed=%d err=%w", entry.OpportunityID, removed, err)
		}
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
		if entry.Disposition == "duplicate" {
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
		if entry.Disposition == "duplicate" {
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
			CanonicalObjectID: pgUUID(entry.WorkSignatureID), Operation: operation,
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
