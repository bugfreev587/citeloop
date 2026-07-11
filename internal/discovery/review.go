package discovery

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const MaterialChangePolicyVersionV1 = "discovery-material-change-v1"

type ReviewResolutionAction string

const (
	ReviewActionDismiss          ReviewResolutionAction = "dismissed"
	ReviewActionSnooze           ReviewResolutionAction = "snoozed"
	ReviewActionWatch            ReviewResolutionAction = "watching"
	ReviewActionReopenDoctor     ReviewResolutionAction = "reopen_doctor"
	ReviewActionReopenGrowth     ReviewResolutionAction = "reopen_opportunities"
	ReviewActionMergeEvidence    ReviewResolutionAction = "merge_evidence"
	ReviewActionBlockOnOtherLine ReviewResolutionAction = "block_on_other_line"
)

type ReviewResolutionRequest struct {
	ProjectID                uuid.UUID
	CandidateID              uuid.UUID
	Action                   ReviewResolutionAction
	Owner                    Owner
	OverlapWorkIDs           []uuid.UUID
	ExpectedCandidateVersion int64
	ExpectedBucketVersions   map[string]int64
	ResolvedBy               string
	Reason                   string
	SnoozedUntil             time.Time
}

type ReviewResolutionResult struct {
	DecisionID     uuid.UUID              `json:"decision_id"`
	ReviewMemoryID uuid.UUID              `json:"review_memory_id,omitempty"`
	Action         ReviewResolutionAction `json:"action"`
	Reopened       bool                   `json:"reopened"`
}

type ReviewStore interface {
	ResolveReviewAtomically(context.Context, ReviewResolutionRequest) (ReviewResolutionResult, error)
}

type ReviewService struct {
	store ReviewStore
	now   func() time.Time
}

func NewReviewService(store ReviewStore) *ReviewService {
	return &ReviewService{store: store, now: time.Now}
}

// Resolve records an audited Ops decision. It never creates a Site Fix,
// Opportunity, Growth Action, or other user work; reopen decisions only create
// a prepared arbitration decision for the later canonical writer.
func (s *ReviewService) Resolve(ctx context.Context, request ReviewResolutionRequest) (ReviewResolutionResult, error) {
	if s == nil || s.store == nil {
		return ReviewResolutionResult{}, errors.New("review store is required")
	}
	if request.ProjectID == uuid.Nil || request.CandidateID == uuid.Nil {
		return ReviewResolutionResult{}, errors.New("project and candidate ids are required")
	}
	if request.ExpectedCandidateVersion < 1 {
		return ReviewResolutionResult{}, errors.New("expected candidate version is required")
	}
	if err := validateExpectedBucketVersions(request.ExpectedBucketVersions); err != nil {
		return ReviewResolutionResult{}, err
	}
	request.ResolvedBy = strings.TrimSpace(request.ResolvedBy)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.ResolvedBy == "" || request.Reason == "" {
		return ReviewResolutionResult{}, errors.New("resolution actor and reason are required")
	}
	request.OverlapWorkIDs = canonicalUUIDs(request.OverlapWorkIDs)
	switch request.Action {
	case ReviewActionDismiss, ReviewActionWatch:
	case ReviewActionSnooze:
		if request.SnoozedUntil.IsZero() || !request.SnoozedUntil.After(s.now().UTC()) {
			return ReviewResolutionResult{}, errors.New("snoozed_until must be in the future")
		}
	case ReviewActionReopenDoctor:
		request.Owner = OwnerDoctor
	case ReviewActionReopenGrowth:
		request.Owner = OwnerOpportunities
	case ReviewActionMergeEvidence, ReviewActionBlockOnOtherLine:
		if len(request.OverlapWorkIDs) == 0 {
			return ReviewResolutionResult{}, fmt.Errorf("%s requires an overlapping work id", request.Action)
		}
	default:
		return ReviewResolutionResult{}, fmt.Errorf("unsupported review resolution %q", request.Action)
	}
	result, err := s.store.ResolveReviewAtomically(ctx, request)
	if err != nil {
		return ReviewResolutionResult{}, err
	}
	if result.DecisionID == uuid.Nil {
		return ReviewResolutionResult{}, errors.New("review transaction returned no decision")
	}
	if isReviewMemoryAction(request.Action) && result.ReviewMemoryID == uuid.Nil {
		return ReviewResolutionResult{}, errors.New("review transaction returned no review memory")
	}
	return result, nil
}

func isReviewMemoryAction(action ReviewResolutionAction) bool {
	return action == ReviewActionDismiss || action == ReviewActionSnooze || action == ReviewActionWatch
}

func validateExpectedBucketVersions(versions map[string]int64) error {
	if len(versions) == 0 {
		return errors.New("complete expected bucket versions are required")
	}
	for key, version := range versions {
		if strings.TrimSpace(key) == "" || version < 0 {
			return errors.New("expected bucket versions contain an invalid entry")
		}
	}
	return nil
}

func canonicalUUIDs(values []uuid.UUID) []uuid.UUID {
	set := make(map[uuid.UUID]struct{}, len(values))
	for _, value := range values {
		if value != uuid.Nil {
			set[value] = struct{}{}
		}
	}
	out := make([]uuid.UUID, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}
