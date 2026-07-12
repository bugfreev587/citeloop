package growthwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/google/uuid"
)

var (
	ErrWrongOwner           = errors.New("Growth work must be owned by Opportunities")
	ErrIncompleteGrowthWork = errors.New("Growth work is missing canonical arbitration provenance")
)

// OpportunityCreator binds an existing evidence-backed Opportunity to its
// enforced owner-neutral reservation. It runs inside arbitration Phase B, so
// the opportunity lock and all cross-line relationships commit atomically with
// the signature and bucket-version increment.
type OpportunityCreator struct {
	Opportunity          *db.CreateCanonicalGrowthOpportunityParams
	LegacyEvidence       json.RawMessage
	LegacyTargetSnapshot string
	CutoverBatchID       uuid.UUID
	CutoverSequence      int32
}

func (creator OpportunityCreator) CreateInTransaction(ctx context.Context, q *db.Queries, work discovery.ReservedWork) (discovery.WorkReference, error) {
	if work.Owner != discovery.OwnerOpportunities {
		return discovery.WorkReference{}, ErrWrongOwner
	}
	if q == nil || work.ProjectID == uuid.Nil || work.CandidateID == uuid.Nil || work.WorkSignatureID == uuid.Nil || work.DecisionID == uuid.Nil {
		return discovery.WorkReference{}, ErrIncompleteGrowthWork
	}
	if work.Decision != discovery.DecisionCreate && work.Decision != discovery.DecisionBlockOnOtherLine {
		return discovery.WorkReference{}, ErrIncompleteGrowthWork
	}
	candidateRow, err := q.GetDiscoveryCandidateForArbitration(ctx, db.GetDiscoveryCandidateForArbitrationParams{
		ProjectID: work.ProjectID, CandidateID: work.CandidateID,
	})
	if err != nil {
		return discovery.WorkReference{}, fmt.Errorf("load reserved Growth candidate: %w", err)
	}
	if candidateRow.SourceObjectType != "seo_opportunity" || candidateRow.SuggestedOwner != string(discovery.OwnerOpportunities) || candidateRow.VerificationMode != string(discovery.VerificationDelayed) {
		return discovery.WorkReference{}, ErrIncompleteGrowthWork
	}
	opportunityID, err := uuid.Parse(strings.TrimSpace(candidateRow.SourceObjectID))
	if err != nil || opportunityID == uuid.Nil {
		return discovery.WorkReference{}, ErrIncompleteGrowthWork
	}
	var opportunity db.SeoOpportunity
	if creator.Opportunity != nil {
		params := *creator.Opportunity
		if params.ID != opportunityID || params.ProjectID != work.ProjectID || params.ExactSignatureHash != *candidateRow.ExactSignatureHash || params.EvidenceFingerprint != candidateRow.EvidenceFingerprint {
			return discovery.WorkReference{}, discovery.ErrSnapshotStale
		}
		opportunity, err = q.CreateCanonicalGrowthOpportunity(ctx, params)
		if err != nil {
			return discovery.WorkReference{}, fmt.Errorf("create canonical Growth opportunity: %w", err)
		}
	} else {
		opportunity, err = q.LockSEOOpportunityForGrowthReserve(ctx, db.LockSEOOpportunityForGrowthReserveParams{
			ID: opportunityID, ProjectID: work.ProjectID,
		})
		if err != nil {
			return discovery.WorkReference{}, fmt.Errorf("lock Growth opportunity: %w", err)
		}
		lockedTarget, err := q.LockLegacyGrowthIntendedTarget(ctx, db.LockLegacyGrowthIntendedTargetParams{
			ProjectID: work.ProjectID, OpportunityID: opportunity.ID,
		})
		if err != nil {
			return discovery.WorkReference{}, fmt.Errorf("lock legacy Growth execution target: %w", err)
		}
		if creator.LegacyTargetSnapshot == "" || creator.LegacyTargetSnapshot != lockedLegacyGrowthTargetSnapshot(lockedTarget) {
			return discovery.WorkReference{}, discovery.ErrSnapshotStale
		}
		opportunity, err = q.MarkLegacyGrowthOpportunityCanonical(ctx, db.MarkLegacyGrowthOpportunityCanonicalParams{
			ProjectID: work.ProjectID, ID: opportunity.ID,
			Evidence: creator.LegacyEvidence, EvidenceFingerprint: candidateRow.EvidenceFingerprint,
		})
		if err != nil {
			return discovery.WorkReference{}, fmt.Errorf("mark legacy Growth opportunity canonical: %w", err)
		}
	}
	projected := discovery.ProjectSEOOpportunity(opportunity)
	identity, err := discovery.BuildIdentity(projected)
	if err != nil || candidateRow.ExactSignatureHash == nil || *candidateRow.ExactSignatureHash != identity.ExactSignatureHash {
		return discovery.WorkReference{}, discovery.ErrSnapshotStale
	}

	blockers, err := q.ListWorkSignaturesForRelationship(ctx, db.ListWorkSignaturesForRelationshipParams{
		ProjectID: work.ProjectID, SignatureIds: work.OverlapWorkIDs,
	})
	if err != nil {
		return discovery.WorkReference{}, fmt.Errorf("load cross-line signatures: %w", err)
	}
	if len(blockers) != len(work.OverlapWorkIDs) {
		return discovery.WorkReference{}, discovery.ErrSnapshotStale
	}
	hasHardBlocker := false
	for _, blocker := range blockers {
		owner := discovery.Owner("")
		if blocker.Owner != nil {
			owner = discovery.Owner(*blocker.Owner)
		}
		relationship, ok, err := discovery.ClassifyCrossLineDependency(projected, discovery.SnapshotWork{
			ID: blocker.ID, Owner: owner, ExactSignatureHash: blocker.ExactSignatureHash,
			SignaturePayload: blocker.SignaturePayload, SemanticFingerprint: valueOrEmpty(blocker.SemanticFingerprint),
		})
		if err != nil || !ok {
			return discovery.WorkReference{}, discovery.ErrSnapshotStale
		}
		if relationship.Class == discovery.DependencyHardBlocker {
			hasHardBlocker = true
		}
		fields, err := json.Marshal(relationship.OverlappingMutationFields)
		if err != nil {
			return discovery.WorkReference{}, err
		}
		if _, err := q.UpsertWorkRelationship(ctx, db.UpsertWorkRelationshipParams{
			ProjectID: work.ProjectID, DependentCandidateID: work.CandidateID,
			DependentWorkSignatureID: work.WorkSignatureID, DependentWorkType: "seo_opportunity",
			DependentWorkID: opportunity.ID, BlockingWorkSignatureID: blocker.ID,
			RelationshipType: relationshipType(blocker.ReservedWorkType), DependencyClass: string(relationship.Class),
			Reason: relationship.Reason, OverlappingMutationFields: fields, ReassessmentTrigger: relationship.ReassessmentTrigger,
		}); err != nil {
			return discovery.WorkReference{}, fmt.Errorf("persist cross-line relationship: %w", err)
		}
	}
	if hasHardBlocker != (work.Decision == discovery.DecisionBlockOnOtherLine) {
		return discovery.WorkReference{}, discovery.ErrSnapshotStale
	}
	if creator.CutoverBatchID != uuid.Nil {
		if _, err := q.CreateCanonicalizedGrowthOpportunityAlias(ctx, db.CreateCanonicalizedGrowthOpportunityAliasParams{
			ProjectID: work.ProjectID, OpportunityID: opportunity.ID, WorkSignatureID: work.WorkSignatureID,
		}); err != nil {
			return discovery.WorkReference{}, fmt.Errorf("create canonical Growth alias: %w", err)
		}
		afterSnapshot, err := json.Marshal(opportunity)
		if err != nil {
			return discovery.WorkReference{}, err
		}
		inverse, _ := json.Marshal(map[string]any{
			"operation": "rollback_growth_canonicalization", "opportunity_id": opportunity.ID,
			"candidate_id": work.CandidateID, "work_signature_id": work.WorkSignatureID,
		})
		if _, err := q.UpdateGrowthCutoverSessionEntryDecision(ctx, db.UpdateGrowthCutoverSessionEntryDecisionParams{
			BatchID: creator.CutoverBatchID, ProjectID: work.ProjectID, OpportunityID: opportunity.ID,
			ArbitrationDecisionID: pgUUID(work.DecisionID), WorkSignatureID: pgUUID(work.WorkSignatureID),
			Disposition: "canonicalized", EntryStatus: "applied",
			AfterSnapshot: afterSnapshot, InverseOperation: inverse,
		}); err != nil {
			return discovery.WorkReference{}, fmt.Errorf("complete Growth cutover entry: %w", err)
		}
	}
	return discovery.WorkReference{Type: "seo_opportunity", ID: opportunity.ID}, nil
}

func relationshipType(reservedWorkType *string) string {
	if reservedWorkType != nil && strings.TrimSpace(*reservedWorkType) == "site_fix" {
		return "blocked_by_site_fix"
	}
	return "blocked_by_doctor_finding"
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
