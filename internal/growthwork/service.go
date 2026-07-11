package growthwork

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrGrowthHeld = errors.New("Growth work requires arbitration review")

type Service struct {
	pool       *pgxpool.Pool
	q          *db.Queries
	comparator discovery.SemanticComparator
}

func NewService(pool *pgxpool.Pool, q *db.Queries, comparator discovery.SemanticComparator) *Service {
	if q == nil && pool != nil {
		q = db.New(pool)
	}
	return &Service{pool: pool, q: q, comparator: comparator}
}

func (s *Service) EnsureLegacyReservations(ctx context.Context, projectID uuid.UUID) error {
	if s == nil || s.pool == nil || s.q == nil || projectID == uuid.Nil {
		return errors.New("canonical Growth database is unavailable")
	}
	return s.migrateLegacyGrowth(ctx, projectID)
}

func (s *Service) EnsureOpportunityReserved(ctx context.Context, projectID, opportunityID uuid.UUID) error {
	if s == nil || s.pool == nil || s.q == nil || projectID == uuid.Nil || opportunityID == uuid.Nil {
		return errors.New("canonical Growth database is unavailable")
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
	if err := s.migrateLegacyGrowth(ctx, params.ProjectID); err != nil {
		return db.SeoOpportunity{}, fmt.Errorf("migrate legacy Growth reservations: %w", err)
	}
	candidate, identity, err := s.materializeCandidate(ctx, params)
	if err != nil {
		return db.SeoOpportunity{}, err
	}
	params.ExactSignatureHash = identity.ExactSignatureHash
	params.EvidenceFingerprint = candidate.Candidate.EvidenceFingerprint
	store := discovery.NewPostgresArbitrationStore(s.pool, s.q)
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
	for _, opportunity := range legacy {
		if err := s.migrateLegacyOpportunity(ctx, opportunity); err != nil {
			return err
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
	store := discovery.NewPostgresArbitrationStore(s.pool, s.q)
	for attempt := 0; attempt < 3; attempt++ {
		prepared, err := discovery.NewArbitrationService(store, s.comparator).Prepare(ctx, opportunity.ProjectID, candidate.ID)
		if err != nil {
			return err
		}
		switch prepared.Decision {
		case discovery.DecisionCreate, discovery.DecisionBlockOnOtherLine:
			if _, err := discovery.NewReservationService(store).ReservePrepared(ctx, opportunity.ProjectID, prepared.ID, OpportunityCreator{}); errors.Is(err, discovery.ErrSnapshotStale) {
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
	q := db.New(tx)
	authority, err := q.LockProductWriterAuthority(ctx, db.LockProductWriterAuthorityParams{ProjectID: params.ProjectID, Product: "opportunities"})
	if err != nil || authority.WriterAuthority != "canonical" || authority.WriteFenced {
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
	merged, err := q.MergeCanonicalGrowthOpportunityEvidence(ctx, db.MergeCanonicalGrowthOpportunityEvidenceParams{
		ProjectID: params.ProjectID, WorkSignatureID: prepared.OverlapWorkIDs[0], Evidence: params.Evidence,
		IncomingPriorityScore: params.PriorityScore, IncomingConfidence: params.Confidence,
		EvidenceFingerprint: prepared.EvidenceFingerprint,
	})
	if err != nil {
		return db.SeoOpportunity{}, err
	}
	if aliasLegacy && merged.ID != params.ID {
		if _, err := q.CreateDuplicateGrowthOpportunityAlias(ctx, db.CreateDuplicateGrowthOpportunityAliasParams{
			ProjectID: params.ProjectID, LegacyOpportunityID: params.ID,
			WorkSignatureID: prepared.OverlapWorkIDs[0],
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
	return s.q.GetSEOOpportunity(ctx, db.GetSEOOpportunityParams{ID: merged.ID, ProjectID: params.ProjectID})
}

func canonicalGrowthRunID(projectID, opportunityID uuid.UUID, exactHash, evidenceFingerprint string) uuid.UUID {
	payload, _ := json.Marshal([]string{projectID.String(), opportunityID.String(), exactHash, evidenceFingerprint})
	sum := sha256.Sum256(payload)
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("citeloop:canonical-growth:"+hex.EncodeToString(sum[:])))
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil}
}
