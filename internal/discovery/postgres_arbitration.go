package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresArbitrationStore implements both phases of arbitration. Phase A uses
// pool-level reads and writes. Phase B is the only method that opens a
// transaction, and its API deliberately has no semantic provider dependency.
type PostgresArbitrationStore struct {
	pool         *pgxpool.Pool
	q            *db.Queries
	providerName string
	model        string
}

func NewPostgresArbitrationStore(pool *pgxpool.Pool, q *db.Queries) *PostgresArbitrationStore {
	if q == nil && pool != nil {
		q = db.New(pool)
	}
	return &PostgresArbitrationStore{
		pool:         pool,
		q:            q,
		providerName: "tokengate",
	}
}

func (s *PostgresArbitrationStore) WithSemanticRuntime(providerName, model string) *PostgresArbitrationStore {
	if s == nil {
		return s
	}
	if strings.TrimSpace(providerName) != "" {
		s.providerName = strings.TrimSpace(providerName)
	}
	s.model = strings.TrimSpace(model)
	return s
}

func (s *PostgresArbitrationStore) LoadCandidate(ctx context.Context, projectID, candidateID uuid.UUID) (ArbitrationCandidate, error) {
	if err := s.requireQueries(); err != nil {
		return ArbitrationCandidate{}, err
	}
	row, err := s.q.GetDiscoveryCandidateForReview(ctx, db.GetDiscoveryCandidateForReviewParams{
		ProjectID: projectID, CandidateID: candidateID,
	})
	if err != nil {
		return ArbitrationCandidate{}, err
	}
	return arbitrationCandidateFromDB(row)
}

func (s *PostgresArbitrationStore) MaterializeBuckets(ctx context.Context, projectID uuid.UUID, bucketKeys []string) error {
	if err := s.requireQueries(); err != nil {
		return err
	}
	keys, err := canonicalBucketKeys(bucketKeys)
	if err != nil {
		return err
	}
	_, err = s.q.MaterializeConflictBuckets(ctx, db.MaterializeConflictBucketsParams{ProjectID: projectID, BucketKeys: keys})
	return err
}

func (s *PostgresArbitrationStore) ReadSnapshot(ctx context.Context, projectID uuid.UUID, bucketKeys []string) (BucketSnapshot, error) {
	if err := s.requireQueries(); err != nil {
		return BucketSnapshot{}, err
	}
	keys, err := canonicalBucketKeys(bucketKeys)
	if err != nil {
		return BucketSnapshot{}, err
	}
	buckets, err := s.q.GetConflictBucketSnapshot(ctx, db.GetConflictBucketSnapshotParams{ProjectID: projectID, BucketKeys: keys})
	if err != nil {
		return BucketSnapshot{}, err
	}
	active, err := s.q.ListSnapshotActiveSignatures(ctx, db.ListSnapshotActiveSignaturesParams{ProjectID: projectID, BucketKeys: keys})
	if err != nil {
		return BucketSnapshot{}, err
	}
	memory, err := s.q.ListSnapshotReviewMemory(ctx, db.ListSnapshotReviewMemoryParams{ProjectID: projectID, BucketKeys: keys})
	if err != nil {
		return BucketSnapshot{}, err
	}
	aliases, err := s.q.ListSnapshotReviewAliases(ctx, db.ListSnapshotReviewAliasesParams{ProjectID: projectID, BucketKeys: keys})
	if err != nil {
		return BucketSnapshot{}, err
	}
	return snapshotFromDB(keys, buckets, active, memory, aliases)
}

func (s *PostgresArbitrationStore) LoadConfig(ctx context.Context, projectID uuid.UUID) (ArbitrationConfig, error) {
	if err := s.requireQueries(); err != nil {
		return ArbitrationConfig{}, err
	}
	row, err := s.q.GetDiscoveryArbitrationConfig(ctx, projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return normalizeArbitrationConfig(ArbitrationConfig{Provider: s.providerName, Model: s.model}), nil
	}
	if err != nil {
		return ArbitrationConfig{}, err
	}
	return ArbitrationConfig{
		ConfidenceThreshold:         numericFloat(row.ConfidenceThreshold),
		LaunchReady:                 row.LaunchReady,
		AutomaticSuppressionEnabled: row.AutomaticSuppressionEnabled,
		RulesVersion:                row.RulesVersion,
		Provider:                    s.providerName,
		Model:                       s.model,
	}, nil
}

func (s *PostgresArbitrationStore) LoadReservationConfig(ctx context.Context, projectID uuid.UUID) (ArbitrationConfig, error) {
	return s.LoadConfig(ctx, projectID)
}

func (s *PostgresArbitrationStore) StartAICall(ctx context.Context, call AICallStart) (uuid.UUID, error) {
	if err := s.requireQueries(); err != nil {
		return uuid.Nil, err
	}
	row, err := s.q.CreateAICallRecord(ctx, db.CreateAICallRecordParams{
		ProjectID: call.ProjectID, RunID: nullableUUID(call.RunID), Stage: "arbitration",
		LinkedObjectType: "discovery_candidate", LinkedObjectID: call.CandidateID,
		Provider: call.Provider, Model: call.Model, PromptVersion: call.PromptVersion,
		RequestFingerprint: call.RequestFingerprint, Status: call.Status,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}

func (s *PostgresArbitrationStore) FinishAICall(ctx context.Context, call AICallFinish) error {
	if err := s.requireQueries(); err != nil {
		return err
	}
	cost, err := numericFromNonNegative(call.CostUSD, 8)
	if err != nil {
		return err
	}
	_, err = s.q.FinishAICallRecord(ctx, db.FinishAICallRecordParams{
		Status: call.Status, ErrorCode: optionalString(call.ErrorCode),
		PromptTokens: boundedInt32(call.PromptTokens), CompletionTokens: boundedInt32(call.CompletionTokens),
		TotalTokens: boundedInt32(call.TotalTokens), CostUsd: cost,
		ID: call.ID, ProjectID: call.ProjectID,
	})
	return err
}

func (s *PostgresArbitrationStore) SavePreparedDecision(ctx context.Context, prepared PreparedDecision) (PreparedDecision, error) {
	if err := s.requireQueries(); err != nil {
		return PreparedDecision{}, err
	}
	params, err := createDecisionParams(prepared)
	if err != nil {
		return PreparedDecision{}, err
	}
	row, err := s.q.CreateArbitrationDecision(ctx, params)
	if err != nil {
		return PreparedDecision{}, err
	}
	return preparedDecisionFromDB(row)
}

func (s *PostgresArbitrationStore) SaveReviewHold(ctx context.Context, hold ReviewHold) error {
	if err := s.requireQueries(); err != nil {
		return err
	}
	versions, err := json.Marshal(hold.ExpectedBucketVersions)
	if err != nil {
		return err
	}
	dueAt := pgtype.Timestamptz{}
	if !hold.DueAt.IsZero() {
		dueAt = pgtype.Timestamptz{Time: hold.DueAt.UTC(), Valid: true}
	}
	_, err = s.q.UpsertDiscoveryReviewItem(ctx, db.UpsertDiscoveryReviewItemParams{
		ProjectID: hold.ProjectID, CandidateID: hold.CandidateID,
		State: string(hold.State), Reason: hold.Reason,
		ExpectedBucketVersions: versions, ExpectedCandidateVersion: hold.CandidateVersion,
		InternalOwner: "discovery_ops", DueAt: dueAt,
		ArbitrationDecisionID: nullableUUID(hold.ArbitrationDecisionID),
	})
	return err
}

func (s *PostgresArbitrationStore) LoadPreparedDecision(ctx context.Context, projectID, decisionID uuid.UUID) (PreparedDecision, error) {
	if err := s.requireQueries(); err != nil {
		return PreparedDecision{}, err
	}
	row, err := s.q.GetArbitrationDecision(ctx, db.GetArbitrationDecisionParams{ProjectID: projectID, ID: decisionID})
	if err != nil {
		return PreparedDecision{}, err
	}
	return preparedDecisionFromDB(row)
}

func (s *PostgresArbitrationStore) ReserveAtomically(ctx context.Context, prepared PreparedDecision, creator WorkCreator) (result ReservationResult, resultErr error) {
	if s == nil || s.pool == nil {
		return ReservationResult{}, errors.New("database pool unavailable")
	}
	keys, err := canonicalBucketKeys(mapKeys(prepared.ExpectedBucketVersions))
	if err != nil {
		return ReservationResult{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return ReservationResult{}, err
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	tq := db.New(tx)

	lockedBuckets, err := tq.LockConflictBucketsForReserve(ctx, db.LockConflictBucketsForReserveParams{
		ProjectID: prepared.ProjectID, BucketKeys: keys,
	})
	if err != nil {
		return ReservationResult{}, staleOnConflict(err)
	}
	lockedDecision, err := tq.LockArbitrationDecisionForReserve(ctx, db.LockArbitrationDecisionForReserveParams{
		ProjectID: prepared.ProjectID, ID: prepared.ID,
	})
	if err != nil {
		return ReservationResult{}, staleOnNoRows(err)
	}
	lockedCandidate, err := tq.LockDiscoveryCandidateForReserve(ctx, db.LockDiscoveryCandidateForReserveParams{
		ProjectID: prepared.ProjectID, CandidateID: prepared.CandidateID,
	})
	if err != nil {
		return ReservationResult{}, staleOnNoRows(err)
	}
	lockedCandidateKeys, err := decodeCanonicalBucketKeys(lockedCandidate.ConflictBucketKeys)
	if err != nil || !reflect.DeepEqual(lockedCandidateKeys, keys) {
		return ReservationResult{}, ErrSnapshotStale
	}
	active, err := tq.ListSnapshotActiveSignatures(ctx, db.ListSnapshotActiveSignaturesParams{
		ProjectID: prepared.ProjectID, BucketKeys: keys,
	})
	if err != nil {
		return ReservationResult{}, err
	}
	memory, err := tq.ListSnapshotReviewMemory(ctx, db.ListSnapshotReviewMemoryParams{
		ProjectID: prepared.ProjectID, BucketKeys: keys,
	})
	if err != nil {
		return ReservationResult{}, err
	}
	aliases, err := tq.ListSnapshotReviewAliases(ctx, db.ListSnapshotReviewAliasesParams{
		ProjectID: prepared.ProjectID, BucketKeys: keys,
	})
	if err != nil {
		return ReservationResult{}, err
	}
	snapshot, err := snapshotFromDB(keys, lockedBuckets, active, memory, aliases)
	if err != nil {
		return ReservationResult{}, ErrSnapshotStale
	}
	lockedPrepared, err := preparedDecisionFromDB(lockedDecision)
	if err != nil {
		return ReservationResult{}, ErrSnapshotStale
	}
	snapshotFingerprint, err := buildSnapshotFingerprint(snapshot)
	if err != nil {
		return ReservationResult{}, err
	}
	if !reservationInputsStillCurrent(prepared, lockedPrepared, lockedCandidate, snapshot, snapshotFingerprint) {
		return ReservationResult{}, ErrSnapshotStale
	}

	work, err := creator.CreateInTransaction(ctx, tq, ReservedWork{
		ProjectID: prepared.ProjectID, CandidateID: prepared.CandidateID,
		DecisionID: prepared.ID, Owner: prepared.Owner,
	})
	if err != nil {
		return ReservationResult{}, fmt.Errorf("create reserved work: %w", err)
	}
	if work.ID == uuid.Nil || strings.TrimSpace(work.Type) == "" {
		return ReservationResult{}, errors.New("creator returned an invalid work reference")
	}
	owner := string(prepared.Owner)
	semantic := prepared.SemanticFingerprint
	workType := strings.TrimSpace(work.Type)
	bucketJSON, err := json.Marshal(keys)
	if err != nil {
		return ReservationResult{}, err
	}
	signature, err := tq.InsertEnforcedWorkSignature(ctx, db.InsertEnforcedWorkSignatureParams{
		ProjectID: prepared.ProjectID, CandidateID: prepared.CandidateID,
		ShadowRunID:         lockedCandidate.ShadowRunID,
		ExactSignatureHash:  prepared.ExactSignatureHash,
		SignaturePayload:    lockedCandidate.SignaturePayload,
		SemanticFingerprint: &semantic, ConflictBucketKeys: bucketJSON,
		SignatureVersion: prepared.SignatureVersion, Owner: &owner,
		SourceObjectType: lockedCandidate.SourceObjectType, SourceObjectID: lockedCandidate.SourceObjectID,
		ArbitrationDecisionID: nullableUUID(prepared.ID), ReservedWorkType: &workType,
		ReservedWorkID: nullableUUID(work.ID), EvidenceFingerprint: prepared.EvidenceFingerprint,
	})
	if err != nil {
		return ReservationResult{}, staleOnConflict(err)
	}
	updatedBuckets, err := tq.IncrementConflictBucketVersions(ctx, db.IncrementConflictBucketVersionsParams{
		ProjectID: prepared.ProjectID, BucketKeys: keys,
	})
	if err != nil {
		return ReservationResult{}, err
	}
	if len(updatedBuckets) != len(keys) {
		return ReservationResult{}, ErrSnapshotStale
	}
	if _, err := tq.MarkArbitrationDecisionReserved(ctx, db.MarkArbitrationDecisionReservedParams{
		ProjectID: prepared.ProjectID, ID: prepared.ID,
	}); err != nil {
		return ReservationResult{}, staleOnNoRows(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return ReservationResult{}, staleOnConflict(err)
	}
	return ReservationResult{SignatureID: signature.ID, Work: work}, nil
}

func (s *PostgresArbitrationStore) ResolveReviewAtomically(ctx context.Context, request ReviewResolutionRequest) (result ReviewResolutionResult, resultErr error) {
	if s == nil || s.pool == nil {
		return ReviewResolutionResult{}, errors.New("database pool unavailable")
	}
	keys, err := canonicalBucketKeys(mapKeys(request.ExpectedBucketVersions))
	if err != nil {
		return ReviewResolutionResult{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return ReviewResolutionResult{}, err
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	tq := db.New(tx)

	lockedBuckets, err := tq.LockConflictBucketsForReserve(ctx, db.LockConflictBucketsForReserveParams{
		ProjectID: request.ProjectID, BucketKeys: keys,
	})
	if err != nil {
		return ReviewResolutionResult{}, staleOnConflict(err)
	}
	reviewItem, err := tq.LockDiscoveryReviewItemForResolve(ctx, db.LockDiscoveryReviewItemForResolveParams{
		ProjectID: request.ProjectID, CandidateID: request.CandidateID,
	})
	if err != nil {
		return ReviewResolutionResult{}, staleOnNoRows(err)
	}
	lockedCandidate, err := tq.LockDiscoveryCandidateForReserve(ctx, db.LockDiscoveryCandidateForReserveParams{
		ProjectID: request.ProjectID, CandidateID: request.CandidateID,
	})
	if err != nil {
		return ReviewResolutionResult{}, staleOnNoRows(err)
	}
	active, err := tq.ListSnapshotActiveSignatures(ctx, db.ListSnapshotActiveSignaturesParams{
		ProjectID: request.ProjectID, BucketKeys: keys,
	})
	if err != nil {
		return ReviewResolutionResult{}, err
	}
	memoryRows, err := tq.ListSnapshotReviewMemory(ctx, db.ListSnapshotReviewMemoryParams{
		ProjectID: request.ProjectID, BucketKeys: keys,
	})
	if err != nil {
		return ReviewResolutionResult{}, err
	}
	aliasRows, err := tq.ListSnapshotReviewAliases(ctx, db.ListSnapshotReviewAliasesParams{
		ProjectID: request.ProjectID, BucketKeys: keys,
	})
	if err != nil {
		return ReviewResolutionResult{}, err
	}
	snapshot, err := snapshotFromDB(keys, lockedBuckets, active, memoryRows, aliasRows)
	if err != nil {
		return ReviewResolutionResult{}, ErrSnapshotStale
	}
	var itemVersions map[string]int64
	if err := unmarshalJSON(reviewItem.ExpectedBucketVersions, &itemVersions); err != nil {
		return ReviewResolutionResult{}, ErrSnapshotStale
	}
	if reviewItem.State == "resolved" || reviewItem.ExpectedCandidateVersion != request.ExpectedCandidateVersion ||
		lockedCandidate.CandidateVersion != request.ExpectedCandidateVersion ||
		!reflect.DeepEqual(itemVersions, request.ExpectedBucketVersions) ||
		!reflect.DeepEqual(snapshot.Versions, request.ExpectedBucketVersions) {
		return ReviewResolutionResult{}, ErrSnapshotStale
	}
	candidate, err := arbitrationCandidateFromDB(lockedCandidate)
	if err != nil || candidate.Identity.ExactSignatureHash == "" || len(candidate.Identity.SignaturePayload) == 0 {
		return ReviewResolutionResult{}, ErrSnapshotStale
	}
	candidateKeys, err := canonicalBucketKeys(candidate.Identity.ConflictBucketKeys)
	if err != nil || !reflect.DeepEqual(candidateKeys, keys) {
		return ReviewResolutionResult{}, ErrSnapshotStale
	}
	snapshotFingerprint, err := buildSnapshotFingerprint(snapshot)
	if err != nil {
		return ReviewResolutionResult{}, err
	}
	semantic, err := semanticFingerprint(candidate.Identity, "manual-review")
	if err != nil {
		return ReviewResolutionResult{}, err
	}
	owner := firstValidOwner(request.Owner, ownerForCandidate(candidate.Candidate))
	decisionKind, status := manualResolutionDecision(request.Action)
	prepared := PreparedDecision{
		ProjectID: request.ProjectID, CandidateID: request.CandidateID,
		CandidateVersion: request.ExpectedCandidateVersion,
		Disposition:      DispositionManualResolution, Decision: decisionKind, Owner: owner,
		OverlapWorkIDs: request.OverlapWorkIDs, ComparedWorkIDs: request.OverlapWorkIDs,
		Reason: request.Reason, Confidence: 1, SemanticFingerprint: semantic,
		ExpectedBucketVersions: cloneVersions(request.ExpectedBucketVersions),
		SnapshotFingerprint:    snapshotFingerprint, ExactSignatureHash: candidate.Identity.ExactSignatureHash,
		SignatureVersion:    candidate.Candidate.SignatureVersion,
		EvidenceFingerprint: candidate.Candidate.EvidenceFingerprint,
		RulesVersion:        ArbitrationRulesVersionV1, PromptVersion: "manual-review-v1",
		Provider: "manual", Model: "manual", Status: status,
	}
	params, err := createDecisionParams(prepared)
	if err != nil {
		return ReviewResolutionResult{}, err
	}
	decisionRow, err := tq.CreateArbitrationDecision(ctx, params)
	if err != nil {
		return ReviewResolutionResult{}, err
	}
	result = ReviewResolutionResult{DecisionID: decisionRow.ID, Action: request.Action,
		Reopened: request.Action == ReviewActionReopenDoctor || request.Action == ReviewActionReopenGrowth}

	if isReviewMemoryAction(request.Action) {
		memoryID, memoryErr := persistReviewMemoryInTransaction(ctx, tq, request, candidate, snapshot, semantic)
		if memoryErr != nil {
			return ReviewResolutionResult{}, memoryErr
		}
		result.ReviewMemoryID = memoryID
	}
	resolution, err := json.Marshal(struct {
		Action         ReviewResolutionAction `json:"action"`
		Owner          Owner                  `json:"owner"`
		Reason         string                 `json:"reason"`
		OverlapWorkIDs []uuid.UUID            `json:"overlap_work_ids"`
		DecisionID     uuid.UUID              `json:"arbitration_decision_id"`
	}{request.Action, owner, request.Reason, request.OverlapWorkIDs, decisionRow.ID})
	if err != nil {
		return ReviewResolutionResult{}, err
	}
	actor := request.ResolvedBy
	if _, err := tq.ResolveDiscoveryReviewItem(ctx, db.ResolveDiscoveryReviewItemParams{
		Resolution: resolution, ResolvedBy: &actor, ArbitrationDecisionID: nullableUUID(decisionRow.ID),
		ProjectID: request.ProjectID, CandidateID: request.CandidateID,
	}); err != nil {
		return ReviewResolutionResult{}, staleOnNoRows(err)
	}
	updatedBuckets, err := tq.IncrementConflictBucketVersions(ctx, db.IncrementConflictBucketVersionsParams{
		ProjectID: request.ProjectID, BucketKeys: keys,
	})
	if err != nil {
		return ReviewResolutionResult{}, err
	}
	if len(updatedBuckets) != len(keys) {
		return ReviewResolutionResult{}, ErrSnapshotStale
	}
	if err := tx.Commit(ctx); err != nil {
		return ReviewResolutionResult{}, staleOnConflict(err)
	}
	return result, nil
}

func (s *PostgresArbitrationStore) LoadGoldCases(ctx context.Context, projectID uuid.UUID, datasetVersion string) ([]SemanticGoldCase, error) {
	if err := s.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := s.q.ListDiscoverySemanticGoldCases(ctx, db.ListDiscoverySemanticGoldCasesParams{
		ProjectID: projectID, DatasetVersion: datasetVersion,
	})
	if err != nil {
		return nil, err
	}
	result := make([]SemanticGoldCase, 0, len(rows))
	for _, row := range rows {
		actual := DecisionHold
		compared := row.ActualDecision != nil
		if compared {
			actual = DecisionKind(*row.ActualDecision)
		}
		result = append(result, SemanticGoldCase{
			ID: row.ID, Label: SemanticGoldLabel(row.Label), ExpectedDecision: DecisionKind(row.ExpectedDecision),
			ActualDecision: actual, Confidence: numericFloat(row.ActualConfidence), Compared: compared,
		})
	}
	return result, nil
}

func (s *PostgresArbitrationStore) SaveSemanticEvaluation(ctx context.Context, projectID uuid.UUID, result SemanticEvaluationResult, automaticSuppression bool, evaluatedBy string) (saved SemanticEvaluationResult, resultErr error) {
	if s == nil || s.pool == nil {
		return SemanticEvaluationResult{}, errors.New("database pool unavailable")
	}
	if automaticSuppression && !result.LaunchReady {
		return SemanticEvaluationResult{}, errors.New("automatic suppression requires a launch-ready evaluation")
	}
	toNumeric := func(value float64) (pgtype.Numeric, error) {
		if value > 1 {
			return pgtype.Numeric{}, fmt.Errorf("evaluation ratio exceeds one: %v", value)
		}
		return numericFromNonNegative(value, 6)
	}
	threshold, err := toNumeric(result.ConfidenceThreshold)
	if err != nil {
		return SemanticEvaluationResult{}, err
	}
	recallTarget, err := toNumeric(result.DuplicateSafetyRecallTarget)
	if err != nil {
		return SemanticEvaluationResult{}, err
	}
	falseTarget, err := toNumeric(result.FalseSuppressionRateTarget)
	if err != nil {
		return SemanticEvaluationResult{}, err
	}
	recall, err := toNumeric(result.DuplicateSafetyRecall)
	if err != nil {
		return SemanticEvaluationResult{}, err
	}
	falseRate, err := toNumeric(result.FalseSuppressionRate)
	if err != nil {
		return SemanticEvaluationResult{}, err
	}
	coverage, err := toNumeric(result.ComparatorCoverage)
	if err != nil {
		return SemanticEvaluationResult{}, err
	}
	automatedCoverage, err := toNumeric(result.AutomatedDispositionCoverage)
	if err != nil {
		return SemanticEvaluationResult{}, err
	}
	holdRate, err := toNumeric(result.HoldRate)
	if err != nil {
		return SemanticEvaluationResult{}, err
	}
	blockers, err := json.Marshal(result.Blockers)
	if err != nil {
		return SemanticEvaluationResult{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return SemanticEvaluationResult{}, err
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	tq := db.New(tx)
	row, err := tq.CreateDiscoverySemanticEvaluation(ctx, db.CreateDiscoverySemanticEvaluationParams{
		ProjectID: projectID, DatasetVersion: result.DatasetVersion, ConfidenceThreshold: threshold,
		DuplicateSafetyRecallTarget: recallTarget, FalseSuppressionRateTarget: falseTarget,
		TotalCases: boundedInt32(result.TotalCases), DuplicateSafetyCases: boundedInt32(result.DuplicateSafetyCases),
		DistinctCases: boundedInt32(result.DistinctCases), DuplicateSafetyRecall: recall,
		FalseSuppressionRate: falseRate, ComparatorCoverage: coverage,
		AutomatedDispositionCoverage: automatedCoverage, HoldRate: holdRate,
		ThresholdBacklog: boundedInt32(result.ThresholdBacklog), WeeklyOpsCapacity: boundedInt32(result.WeeklyOpsCapacity),
		LaunchReady: result.LaunchReady, AutomaticSuppressionEnabled: automaticSuppression,
		Blockers: blockers, EvaluatedBy: evaluatedBy,
	})
	if err != nil {
		return SemanticEvaluationResult{}, err
	}
	if _, err := tq.UpsertDiscoveryArbitrationEvaluationConfig(ctx, db.UpsertDiscoveryArbitrationEvaluationConfigParams{
		ProjectID: projectID, ConfidenceThreshold: threshold,
		DuplicateSafetyRecallTarget: recallTarget, FalseSuppressionRateTarget: falseTarget,
		GoldDatasetVersion: result.DatasetVersion, WeeklyOpsCapacity: boundedInt32(result.WeeklyOpsCapacity),
		LaunchReady: result.LaunchReady, AutomaticSuppressionEnabled: automaticSuppression,
	}); err != nil {
		return SemanticEvaluationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SemanticEvaluationResult{}, staleOnConflict(err)
	}
	result.ID = row.ID
	return result, nil
}

func (s *PostgresArbitrationStore) requireQueries() error {
	if s == nil || s.q == nil {
		return errors.New("database queries unavailable")
	}
	return nil
}

func manualResolutionDecision(action ReviewResolutionAction) (DecisionKind, ArbitrationStatus) {
	switch action {
	case ReviewActionReopenDoctor, ReviewActionReopenGrowth:
		return DecisionCreate, ArbitrationStatusPrepared
	case ReviewActionMergeEvidence:
		return DecisionMergeEvidence, ArbitrationStatusResolved
	case ReviewActionBlockOnOtherLine:
		return DecisionBlockOnOtherLine, ArbitrationStatusResolved
	default:
		return DecisionSuppress, ArbitrationStatusResolved
	}
}

func persistReviewMemoryInTransaction(ctx context.Context, q *db.Queries, request ReviewResolutionRequest, candidate ArbitrationCandidate, snapshot BucketSnapshot, semantic string) (uuid.UUID, error) {
	keys, err := canonicalBucketKeys(candidate.Identity.ConflictBucketKeys)
	if err != nil {
		return uuid.Nil, err
	}
	bucketJSON, err := json.Marshal(keys)
	if err != nil {
		return uuid.Nil, err
	}
	scope, err := json.Marshal(struct {
		Owner          Owner       `json:"owner"`
		Reason         string      `json:"reason"`
		OverlapWorkIDs []uuid.UUID `json:"overlap_work_ids"`
	}{Owner: firstValidOwner(request.Owner, ownerForCandidate(candidate.Candidate)), Reason: request.Reason, OverlapWorkIDs: request.OverlapWorkIDs})
	if err != nil {
		return uuid.Nil, err
	}
	decidedAt := time.Now().UTC()
	snoozedUntil := pgtype.Timestamptz{}
	if request.Action == ReviewActionSnooze {
		snoozedUntil = pgtype.Timestamptz{Time: request.SnoozedUntil.UTC(), Valid: true}
	}
	prior := priorReviewMemoryForSignature(snapshot.ReviewMemory, candidate.Identity.SignaturePayload)
	if prior != nil && prior.ExactSignatureHash != candidate.Identity.ExactSignatureHash {
		if _, err := q.DeactivateWorkReviewMemory(ctx, db.DeactivateWorkReviewMemoryParams{
			ProjectID: request.ProjectID, ID: prior.ID,
		}); err != nil {
			return uuid.Nil, staleOnNoRows(err)
		}
	}
	memory, err := q.UpsertWorkReviewMemory(ctx, db.UpsertWorkReviewMemoryParams{
		ProjectID: request.ProjectID, CandidateID: nullableUUID(request.CandidateID),
		ExactSignatureHashAtDecision:  candidate.Identity.ExactSignatureHash,
		SemanticFingerprintAtDecision: semantic, SignaturePayload: candidate.Identity.SignaturePayload,
		ConflictBucketKeys: bucketJSON, SignatureVersion: candidate.Candidate.SignatureVersion,
		Decision: string(request.Action), DecisionScope: scope,
		EvidenceFingerprintAtDecision: candidate.Candidate.EvidenceFingerprint,
		SnoozedUntil:                  snoozedUntil, MaterialChangePolicyVersion: MaterialChangePolicyVersionV1,
		DecidedBy: request.ResolvedBy, DecidedAt: pgtype.Timestamptz{Time: decidedAt, Valid: true}, Active: true,
	})
	if err != nil {
		return uuid.Nil, err
	}
	if prior != nil && prior.ExactSignatureHash != candidate.Identity.ExactSignatureHash {
		aliases := []ReviewMemoryAliasSnapshot{{
			ReviewMemoryID: prior.ID, ExactSignatureHash: prior.ExactSignatureHash,
			SemanticFingerprint: prior.SemanticFingerprint, SignatureVersion: prior.SignatureVersion,
		}}
		for _, alias := range snapshot.ReviewAliases {
			if alias.ReviewMemoryID == prior.ID {
				aliases = append(aliases, alias)
			}
		}
		for _, alias := range aliases {
			if alias.ExactSignatureHash == "" || alias.SignatureVersion == "" || alias.ExactSignatureHash == candidate.Identity.ExactSignatureHash {
				continue
			}
			if _, err := q.UpsertWorkSignatureAlias(ctx, db.UpsertWorkSignatureAliasParams{
				ProjectID: request.ProjectID, ReviewMemoryID: memory.ID,
				AliasExactSignatureHash:  alias.ExactSignatureHash,
				AliasSemanticFingerprint: alias.SemanticFingerprint,
				AliasSignatureVersion:    alias.SignatureVersion,
			}); err != nil {
				return uuid.Nil, err
			}
		}
	}
	return memory.ID, nil
}

func priorReviewMemoryForSignature(memories []ReviewMemorySnapshot, signaturePayload json.RawMessage) *ReviewMemorySnapshot {
	for i := range memories {
		if memories[i].Active && signatureEquivalentAcrossVersion(memories[i].SignaturePayload, signaturePayload) {
			return &memories[i]
		}
	}
	return nil
}

func signatureEquivalentAcrossVersion(left, right json.RawMessage) bool {
	canonical := func(raw json.RawMessage) []byte {
		var value map[string]any
		if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
			return nil
		}
		delete(value, "signature_version")
		result, _ := json.Marshal(value)
		return result
	}
	leftCanonical := canonical(left)
	rightCanonical := canonical(right)
	return len(leftCanonical) > 0 && bytes.Equal(leftCanonical, rightCanonical)
}

func arbitrationCandidateFromDB(row db.DiscoveryCandidate) (ArbitrationCandidate, error) {
	var targets, topics, audience, evidenceIDs, buckets []string
	var mutations []Mutation
	for _, item := range []struct {
		raw  json.RawMessage
		dest any
		name string
	}{
		{row.NormalizedTargetSet, &targets, "normalized_target_set"},
		{row.ProposedMutations, &mutations, "proposed_mutations"},
		{row.TopicEntityIdentity, &topics, "topic_entity_identity"},
		{row.AudienceIdentity, &audience, "audience_identity"},
		{row.EvidenceIds, &evidenceIDs, "evidence_ids"},
		{row.ConflictBucketKeys, &buckets, "conflict_bucket_keys"},
	} {
		if err := unmarshalJSON(item.raw, item.dest); err != nil {
			return ArbitrationCandidate{}, fmt.Errorf("decode %s: %w", item.name, err)
		}
	}
	signatureVersion := SignatureVersionV1
	if len(row.SignaturePayload) > 0 {
		var payload signaturePayload
		if err := json.Unmarshal(row.SignaturePayload, &payload); err != nil {
			return ArbitrationCandidate{}, fmt.Errorf("decode signature payload: %w", err)
		}
		signatureVersion = firstNonEmpty(payload.SignatureVersion, SignatureVersionV1)
	}
	exact := ""
	if row.ExactSignatureHash != nil {
		exact = *row.ExactSignatureHash
	}
	return ArbitrationCandidate{
		ID: row.ID, RunID: row.ShadowRunID, Version: row.CandidateVersion,
		Candidate: Candidate{
			ProjectID: row.ProjectID, SourceKind: SourceKind(row.SourceKind),
			SourceObjectType: row.SourceObjectType, SourceObjectID: row.SourceObjectID,
			TargetKind: row.TargetKind, NormalizedTargetSet: targets,
			IssueOrHypothesisFamily: row.IssueOrHypothesisFamily, ChangeFamily: row.ChangeFamily,
			ProposedMutations: mutations, ArtifactIntent: ArtifactIntent(row.ArtifactIntent),
			IntendedSlugOrCanonical: valueOrEmpty(row.IntendedSlugOrCanonical),
			TopicEntityIdentity:     topics, AudienceIdentity: audience,
			PrimarySuccessMetric: row.PrimarySuccessMetric, VerificationMode: VerificationMode(row.VerificationMode),
			EvidenceIDs: evidenceIDs, EvidenceFingerprint: row.EvidenceFingerprint,
			SuggestedOwner: Owner(row.SuggestedOwner), Confidence: numericFloat(row.Confidence),
			CandidateSchemaVersion: row.CandidateSchemaVersion, SignatureVersion: signatureVersion,
			Status: CandidateStatus(row.Status), HoldReason: valueOrEmpty(row.HoldReason),
		},
		Identity: Identity{ExactSignatureHash: exact, SignaturePayload: row.SignaturePayload, ConflictBucketKeys: buckets},
	}, nil
}

func snapshotFromDB(keys []string, buckets []db.WorkConflictBucket, active []db.WorkSignatureRegistry, memory []db.WorkReviewMemory, aliases []db.WorkSignatureAlias) (BucketSnapshot, error) {
	versions := make(map[string]int64, len(buckets))
	for _, bucket := range buckets {
		versions[bucket.BucketKey] = bucket.BucketVersion
	}
	if len(versions) != len(keys) {
		return BucketSnapshot{}, fmt.Errorf("bucket snapshot incomplete: got %d of %d", len(versions), len(keys))
	}
	for _, key := range keys {
		if _, ok := versions[key]; !ok {
			return BucketSnapshot{}, fmt.Errorf("bucket snapshot missing %q", key)
		}
	}
	result := BucketSnapshot{Versions: versions}
	result.ActiveWorks = make([]SnapshotWork, 0, len(active))
	for _, row := range active {
		result.ActiveWorks = append(result.ActiveWorks, SnapshotWork{
			ID: row.ID, Owner: Owner(valueOrEmpty(row.Owner)), ExactSignatureHash: row.ExactSignatureHash,
			SignaturePayload: row.SignaturePayload, SemanticFingerprint: valueOrEmpty(row.SemanticFingerprint),
			EvidenceFingerprint: row.EvidenceFingerprint, SignatureVersion: row.SignatureVersion,
		})
	}
	result.ReviewMemory = make([]ReviewMemorySnapshot, 0, len(memory))
	for _, row := range memory {
		result.ReviewMemory = append(result.ReviewMemory, ReviewMemorySnapshot{
			ID: row.ID, Decision: row.Decision, ExactSignatureHash: row.ExactSignatureHashAtDecision,
			SemanticFingerprint: row.SemanticFingerprintAtDecision, SignaturePayload: row.SignaturePayload,
			EvidenceFingerprint: row.EvidenceFingerprintAtDecision, SignatureVersion: row.SignatureVersion,
			SnoozedUntil: timestamp(row.SnoozedUntil), Active: row.Active,
		})
	}
	result.ReviewAliases = make([]ReviewMemoryAliasSnapshot, 0, len(aliases))
	for _, row := range aliases {
		result.ReviewAliases = append(result.ReviewAliases, ReviewMemoryAliasSnapshot{
			ReviewMemoryID: row.ReviewMemoryID, ExactSignatureHash: row.AliasExactSignatureHash,
			SemanticFingerprint: row.AliasSemanticFingerprint, SignatureVersion: row.AliasSignatureVersion,
		})
	}
	return result, nil
}

func createDecisionParams(prepared PreparedDecision) (db.CreateArbitrationDecisionParams, error) {
	confidence, err := numericFromConfidence(prepared.Confidence)
	if err != nil {
		return db.CreateArbitrationDecisionParams{}, err
	}
	overlaps, err := json.Marshal(prepared.OverlapWorkIDs)
	if err != nil {
		return db.CreateArbitrationDecisionParams{}, err
	}
	compared, err := json.Marshal(prepared.ComparedWorkIDs)
	if err != nil {
		return db.CreateArbitrationDecisionParams{}, err
	}
	versions, err := json.Marshal(prepared.ExpectedBucketVersions)
	if err != nil {
		return db.CreateArbitrationDecisionParams{}, err
	}
	var owner *string
	if prepared.Owner == OwnerDoctor || prepared.Owner == OwnerOpportunities {
		value := string(prepared.Owner)
		owner = &value
	}
	return db.CreateArbitrationDecisionParams{
		ProjectID: prepared.ProjectID, CandidateID: prepared.CandidateID,
		CandidateVersion: prepared.CandidateVersion, AiCallID: nullableUUID(prepared.AICallID),
		Disposition: string(prepared.Disposition), Decision: string(prepared.Decision), Owner: owner,
		OverlapWorkIds: overlaps, Reason: prepared.Reason, Confidence: confidence,
		SemanticFingerprint: prepared.SemanticFingerprint, ComparedWorkIds: compared,
		ExpectedBucketVersions: versions, SnapshotFingerprint: prepared.SnapshotFingerprint,
		ExactSignatureHash: prepared.ExactSignatureHash, SignatureVersion: prepared.SignatureVersion,
		EvidenceFingerprint: prepared.EvidenceFingerprint, RulesVersion: prepared.RulesVersion,
		PromptVersion: prepared.PromptVersion, Provider: prepared.Provider, Model: prepared.Model,
		Status: string(prepared.Status),
	}, nil
}

func preparedDecisionFromDB(row db.DiscoveryArbitrationDecision) (PreparedDecision, error) {
	var overlaps, compared []uuid.UUID
	versions := map[string]int64{}
	if err := unmarshalJSON(row.OverlapWorkIds, &overlaps); err != nil {
		return PreparedDecision{}, fmt.Errorf("decode overlap work ids: %w", err)
	}
	if err := unmarshalJSON(row.ComparedWorkIds, &compared); err != nil {
		return PreparedDecision{}, fmt.Errorf("decode compared work ids: %w", err)
	}
	if err := unmarshalJSON(row.ExpectedBucketVersions, &versions); err != nil {
		return PreparedDecision{}, fmt.Errorf("decode expected bucket versions: %w", err)
	}
	return PreparedDecision{
		ID: row.ID, ProjectID: row.ProjectID, CandidateID: row.CandidateID,
		CandidateVersion: row.CandidateVersion, AICallID: uuidFromPG(row.AiCallID),
		Disposition: ArbitrationDisposition(row.Disposition), Decision: DecisionKind(row.Decision),
		Owner: Owner(valueOrEmpty(row.Owner)), OverlapWorkIDs: overlaps, Reason: row.Reason,
		Confidence: numericFloat(row.Confidence), SemanticFingerprint: row.SemanticFingerprint,
		ComparedWorkIDs: compared, ExpectedBucketVersions: versions,
		SnapshotFingerprint: row.SnapshotFingerprint, ExactSignatureHash: row.ExactSignatureHash,
		SignatureVersion: row.SignatureVersion, EvidenceFingerprint: row.EvidenceFingerprint,
		RulesVersion: row.RulesVersion, PromptVersion: row.PromptVersion,
		Provider: row.Provider, Model: row.Model, Status: ArbitrationStatus(row.Status),
	}, nil
}

func reservationInputsStillCurrent(input, persisted PreparedDecision, candidate db.DiscoveryCandidate, snapshot BucketSnapshot, fingerprint string) bool {
	if persisted.ID != input.ID || persisted.ProjectID != input.ProjectID || persisted.CandidateID != input.CandidateID ||
		persisted.CandidateVersion != input.CandidateVersion || persisted.Status != ArbitrationStatusPrepared ||
		persisted.Decision != DecisionCreate || persisted.Confidence != input.Confidence ||
		persisted.ExactSignatureHash != input.ExactSignatureHash || persisted.SemanticFingerprint != input.SemanticFingerprint ||
		persisted.SignatureVersion != input.SignatureVersion || persisted.SnapshotFingerprint != input.SnapshotFingerprint ||
		persisted.Owner != input.Owner || candidate.CandidateVersion != input.CandidateVersion ||
		candidate.ProjectID != input.ProjectID || candidate.ID != input.CandidateID ||
		candidate.ExactSignatureHash == nil || *candidate.ExactSignatureHash != input.ExactSignatureHash ||
		fingerprint != input.SnapshotFingerprint {
		return false
	}
	return reflect.DeepEqual(persisted.ExpectedBucketVersions, input.ExpectedBucketVersions) &&
		reflect.DeepEqual(snapshot.Versions, input.ExpectedBucketVersions)
}

func canonicalBucketKeys(values []string) ([]string, error) {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New("conflict bucket key is required")
		}
		set[value] = struct{}{}
	}
	if len(set) == 0 {
		return nil, errors.New("at least one conflict bucket is required")
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func mapKeys(values map[string]int64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func decodeCanonicalBucketKeys(raw json.RawMessage) ([]string, error) {
	var keys []string
	if err := unmarshalJSON(raw, &keys); err != nil {
		return nil, err
	}
	return canonicalBucketKeys(keys)
}

func unmarshalJSON(raw json.RawMessage, destination any) error {
	if len(raw) == 0 {
		return errors.New("JSON value is empty")
	}
	return json.Unmarshal(raw, destination)
}

func nullableUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil}
}

func uuidFromPG(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	return id.Bytes
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func boundedInt32(value int) int32 {
	if value <= 0 {
		return 0
	}
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(value)
}

func numericFromNonNegative(value float64, precision int) (pgtype.Numeric, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return pgtype.Numeric{}, fmt.Errorf("numeric value must be finite and non-negative, got %v", value)
	}
	var numeric pgtype.Numeric
	if err := numeric.Scan(fmt.Sprintf("%.*f", precision, value)); err != nil {
		return pgtype.Numeric{}, err
	}
	return numeric, nil
}

func staleOnNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSnapshotStale
	}
	return err
}

func staleOnConflict(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "40001" || pgErr.Code == "40P01") {
		return ErrSnapshotStale
	}
	return staleOnNoRows(err)
}
