package growthwork

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type executionChainDisposition string

const (
	executionChainNone    executionChainDisposition = "none"
	executionChainRepoint executionChainDisposition = "repoint"
	executionChainReview  executionChainDisposition = "review"
)

func classifyDuplicateExecutionChain(owner discovery.Owner, actions, conflicts int64) executionChainDisposition {
	if actions == 0 {
		return executionChainNone
	}
	if owner != discovery.OwnerOpportunities || conflicts > 0 {
		return executionChainReview
	}
	return executionChainRepoint
}

func (s *Service) persistExecutionChainReview(ctx context.Context, tx pgx.Tx, q *db.Queries, prepared discovery.PreparedDecision, params db.CreateCanonicalGrowthOpportunityParams, review *growthCutoverReviewError, canonicalSnapshot any) error {
	before, _ := json.Marshal(map[string]any{
		"duplicate": params, "canonical": canonicalSnapshot, "execution_chain": json.RawMessage(review.Snapshot),
	})
	after, _ := json.Marshal(map[string]any{"status": "review_required", "reason": review.Reason})
	inverse, _ := json.Marshal(map[string]any{
		"operation": "tombstone_growth_cutover_provenance", "content_action_ids": []uuid.UUID{},
	})
	if _, err := q.UpdateGrowthCutoverSessionEntryDecision(ctx, db.UpdateGrowthCutoverSessionEntryDecisionParams{
		ArbitrationDecisionID: pgUUID(prepared.ID), AiCallID: pgUUID(prepared.AICallID),
		Disposition: "materialized", EntryStatus: "applying", BeforeSnapshot: before,
		AfterSnapshot: after, InverseOperation: inverse, ProjectID: params.ProjectID,
		BatchID: s.cutoverBatchID, OpportunityID: params.ID,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return review
}

type growthCutoverReviewError struct {
	OpportunityID uuid.UUID
	Reason        string
	Snapshot      json.RawMessage
}

func (e *growthCutoverReviewError) Error() string { return e.Reason }

type executionChainResult struct {
	Snapshot           json.RawMessage
	RepointedActionIDs []uuid.UUID
}

func (s *Service) reconcileDuplicateExecutionChain(ctx context.Context, q *db.Queries, owner discovery.Owner, projectID, sourceOpportunityID uuid.UUID, canonicalOpportunityID pgtype.UUID) (executionChainResult, error) {
	chain, err := q.GetGrowthExecutionChainForUpdate(ctx, db.GetGrowthExecutionChainForUpdateParams{
		CanonicalOpportunityID: canonicalOpportunityID, ProjectID: projectID, SourceOpportunityID: sourceOpportunityID,
	})
	if err != nil {
		return executionChainResult{}, err
	}
	switch classifyDuplicateExecutionChain(owner, chain.ActionCount, chain.ConflictingActionCount) {
	case executionChainNone:
		return executionChainResult{Snapshot: chain.ExecutionSnapshot}, nil
	case executionChainReview:
		reason := "duplicate Growth execution descendants cannot be mapped without loss"
		if owner == discovery.OwnerDoctor {
			reason = "Growth execution descendants cannot be repointed to a Doctor Site Fix"
		} else if chain.ConflictingActionCount > 0 {
			reason = "canonical Growth work already has a conflicting content action"
		}
		return executionChainResult{}, &growthCutoverReviewError{OpportunityID: sourceOpportunityID, Reason: reason, Snapshot: chain.ExecutionSnapshot}
	case executionChainRepoint:
		if !canonicalOpportunityID.Valid {
			return executionChainResult{}, fmt.Errorf("canonical Growth opportunity is required for execution-chain repoint")
		}
		repointed, err := q.RepointDuplicateGrowthContentActions(ctx, db.RepointDuplicateGrowthContentActionsParams{
			CanonicalOpportunityID: canonicalOpportunityID.Bytes, ProjectID: projectID, SourceOpportunityID: sourceOpportunityID,
		})
		if err != nil {
			return executionChainResult{}, err
		}
		if repointed.RepointedCount != chain.ActionCount {
			return executionChainResult{}, discovery.ErrSnapshotStale
		}
		return executionChainResult{Snapshot: chain.ExecutionSnapshot, RepointedActionIDs: repointed.RepointedContentActionIds}, nil
	default:
		return executionChainResult{}, discovery.ErrSnapshotStale
	}
}
