package discovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestReviewResolveRequiresCompleteExpectedSnapshot(t *testing.T) {
	valid := reviewResolutionFixture()
	tests := []struct {
		name   string
		mutate func(*ReviewResolutionRequest)
	}{
		{name: "candidate version", mutate: func(r *ReviewResolutionRequest) { r.ExpectedCandidateVersion = 0 }},
		{name: "bucket versions", mutate: func(r *ReviewResolutionRequest) { r.ExpectedBucketVersions = nil }},
		{name: "empty bucket", mutate: func(r *ReviewResolutionRequest) { r.ExpectedBucketVersions = map[string]int64{"": 1} }},
		{name: "negative bucket", mutate: func(r *ReviewResolutionRequest) { r.ExpectedBucketVersions = map[string]int64{"bucket": -1} }},
		{name: "actor", mutate: func(r *ReviewResolutionRequest) { r.ResolvedBy = "" }},
		{name: "reason", mutate: func(r *ReviewResolutionRequest) { r.Reason = "" }},
		{name: "past snooze", mutate: func(r *ReviewResolutionRequest) {
			r.Action = ReviewActionSnooze
			r.SnoozedUntil = time.Now().Add(-time.Hour)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := valid
			request.ExpectedBucketVersions = cloneVersions(valid.ExpectedBucketVersions)
			tt.mutate(&request)
			store := &reviewStoreStub{}
			if _, err := NewReviewService(store).Resolve(context.Background(), request); err == nil {
				t.Fatal("invalid resolution was accepted")
			}
			if store.calls != 0 {
				t.Fatalf("invalid resolution opened transaction %d times", store.calls)
			}
		})
	}
}

func TestReviewResolvePersistsManualDecisionWithoutCreatingWork(t *testing.T) {
	request := reviewResolutionFixture()
	decisionID := uuid.New()
	memoryID := uuid.New()
	store := &reviewStoreStub{result: ReviewResolutionResult{
		DecisionID: decisionID, ReviewMemoryID: memoryID, Action: ReviewActionDismiss,
	}}

	result, err := NewReviewService(store).Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 || result.DecisionID != decisionID || result.ReviewMemoryID != memoryID {
		t.Fatalf("calls/result = %d/%+v", store.calls, result)
	}
}

func TestReviewResolveReturnsStaleSnapshot(t *testing.T) {
	store := &reviewStoreStub{err: ErrSnapshotStale}
	_, err := NewReviewService(store).Resolve(context.Background(), reviewResolutionFixture())
	if !errors.Is(err, ErrSnapshotStale) {
		t.Fatalf("error = %v, want ErrSnapshotStale", err)
	}
}

func TestReviewResolveRequiresOverlapForMergeOrBlock(t *testing.T) {
	for _, action := range []ReviewResolutionAction{ReviewActionMergeEvidence, ReviewActionBlockOnOtherLine} {
		request := reviewResolutionFixture()
		request.Action = action
		request.OverlapWorkIDs = nil
		store := &reviewStoreStub{}
		if _, err := NewReviewService(store).Resolve(context.Background(), request); err == nil {
			t.Fatalf("%s without overlap was accepted", action)
		}
	}
}

func TestSignatureEquivalentAcrossVersionIgnoresOnlyVersion(t *testing.T) {
	left := []byte(`{"change_family":"metadata","signature_version":"v1","targets":["/pricing"]}`)
	right := []byte(`{"targets":["/pricing"],"signature_version":"v2","change_family":"metadata"}`)
	if !signatureEquivalentAcrossVersion(left, right) {
		t.Fatal("version-only change should retain review memory through an alias")
	}
	changed := []byte(`{"targets":["/about"],"signature_version":"v2","change_family":"metadata"}`)
	if signatureEquivalentAcrossVersion(left, changed) {
		t.Fatal("material signature change was treated as a version-only alias")
	}
}

func reviewResolutionFixture() ReviewResolutionRequest {
	return ReviewResolutionRequest{
		ProjectID: uuid.New(), CandidateID: uuid.New(), Action: ReviewActionDismiss,
		ExpectedCandidateVersion: 2, ExpectedBucketVersions: map[string]int64{"bucket-a": 1, "bucket-b": 3},
		ResolvedBy: "ops@example.com", Reason: "reviewed duplicate",
	}
}

type reviewStoreStub struct {
	calls  int
	result ReviewResolutionResult
	err    error
}

func (s *reviewStoreStub) ResolveReviewAtomically(_ context.Context, request ReviewResolutionRequest) (ReviewResolutionResult, error) {
	s.calls++
	if s.err != nil {
		return ReviewResolutionResult{}, s.err
	}
	result := s.result
	if result.Action == "" {
		result.Action = request.Action
	}
	return result, nil
}
