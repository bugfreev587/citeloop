package discovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestArbitrationPrepareDeterministicSafeWithoutProvider(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	service := NewArbitrationService(store, comparator)

	prepared, err := service.Prepare(context.Background(), candidate.ID)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if comparator.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", comparator.calls)
	}
	if prepared.Disposition != DispositionDeterministicSafe || prepared.Decision != DecisionCreate || prepared.Status != ArbitrationStatusPrepared {
		t.Fatalf("prepared = %+v", prepared)
	}
	if len(store.saved) != 1 || len(store.holds) != 0 {
		t.Fatalf("saved/holds = %d/%d", len(store.saved), len(store.holds))
	}
}

func TestArbitrationPrepareExactActiveWorkMergesWithoutProvider(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	existingID := uuid.New()
	store.snapshot.ActiveWorks = []SnapshotWork{{
		ID: existingID, ExactSignatureHash: candidate.Identity.ExactSignatureHash,
		SignaturePayload: candidate.Identity.SignaturePayload,
	}}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if comparator.calls != 0 || prepared.Disposition != DispositionExactMerge || prepared.Decision != DecisionMergeEvidence {
		t.Fatalf("calls/prepared = %d/%+v", comparator.calls, prepared)
	}
	if len(prepared.OverlapWorkIDs) != 1 || prepared.OverlapWorkIDs[0] != existingID {
		t.Fatalf("overlap IDs = %v", prepared.OverlapWorkIDs)
	}
}

func TestArbitrationPrepareCallsProviderOutsideTransactionBoundary(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	existingID := uuid.New()
	store.snapshot.ActiveWorks = []SnapshotWork{{
		ID: existingID, ExactSignatureHash: "different", SignaturePayload: candidate.Identity.SignaturePayload,
	}}
	comparator.decision = SemanticDecision{
		Decision: DecisionBlockOnOtherLine, Owner: OwnerDoctor,
		WorkSignature: candidate.Identity.ExactSignatureHash,
		Overlaps:      []uuid.UUID{existingID}, Reason: "overlapping title mutation", Confidence: 0.92,
		SemanticFingerprint: "semantic-candidate",
	}
	comparator.events = &store.events

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []string{"load", "materialize", "snapshot", "config", "ai_start", "compare", "ai_finish", "save"}
	if !equalStrings(store.events, wantEvents) {
		t.Fatalf("events = %v, want %v", store.events, wantEvents)
	}
	if prepared.Decision != DecisionBlockOnOtherLine || prepared.Status != ArbitrationStatusPrepared {
		t.Fatalf("prepared = %+v", prepared)
	}
}

func TestArbitrationPrepareFailsClosedForLowConfidence(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	store.snapshot.ActiveWorks = []SnapshotWork{{
		ID: uuid.New(), ExactSignatureHash: "different", SignaturePayload: candidate.Identity.SignaturePayload,
	}}
	comparator.decision = SemanticDecision{
		Decision: DecisionCreate, Owner: OwnerOpportunities,
		WorkSignature: candidate.Identity.ExactSignatureHash,
		Reason:        "uncertain", Confidence: 0.79, SemanticFingerprint: "semantic-candidate",
	}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Decision != DecisionHold || prepared.Status != ArbitrationStatusHeld || prepared.Disposition != DispositionSemanticComparison {
		t.Fatalf("prepared = %+v", prepared)
	}
	if len(store.holds) != 1 || store.holds[0].State != StatusNeedsArbitration {
		t.Fatalf("holds = %+v", store.holds)
	}
}

func TestArbitrationPrepareFailsClosedForProviderFailure(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	store.snapshot.ActiveWorks = []SnapshotWork{{
		ID: uuid.New(), ExactSignatureHash: "different", SignaturePayload: candidate.Identity.SignaturePayload,
	}}
	comparator.err = errors.New("provider unavailable")

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.ID)
	if err != nil {
		t.Fatalf("provider failure must persist a hold, got %v", err)
	}
	if prepared.Decision != DecisionHold || prepared.Disposition != DispositionProviderFailure || len(store.holds) != 1 {
		t.Fatalf("prepared/holds = %+v/%+v", prepared, store.holds)
	}
	if len(store.aiFinishes) != 1 || store.aiFinishes[0].Status != "failed" {
		t.Fatalf("AI finish = %+v", store.aiFinishes)
	}
}

func TestArbitrationPrepareGatesSemanticSuppressionUntilLaunchReady(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	existingID := uuid.New()
	store.snapshot.ActiveWorks = []SnapshotWork{{ID: existingID, ExactSignatureHash: "different", SignaturePayload: candidate.Identity.SignaturePayload}}
	comparator.decision = SemanticDecision{
		Decision: DecisionSuppress, Owner: OwnerDoctor,
		WorkSignature: candidate.Identity.ExactSignatureHash,
		Overlaps:      []uuid.UUID{existingID}, Reason: "same work", Confidence: 0.98,
		SemanticFingerprint: "semantic-candidate",
	}
	store.config.LaunchReady = false
	store.config.AutomaticSuppressionEnabled = false

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Decision != DecisionHold || prepared.Status != ArbitrationStatusHeld || len(store.holds) != 1 {
		t.Fatalf("suppression escaped launch gate: %+v", prepared)
	}
}

func TestArbitrationPrepareAppliesExactReviewMemoryWithoutProvider(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	store.snapshot.ReviewMemory = []ReviewMemorySnapshot{{
		ID: uuid.New(), Decision: "dismissed",
		ExactSignatureHash:  candidate.Identity.ExactSignatureHash,
		EvidenceFingerprint: candidate.Candidate.EvidenceFingerprint,
		Active:              true,
	}}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if comparator.calls != 0 || prepared.Disposition != DispositionReviewMemory || prepared.Decision != DecisionSuppress || prepared.Status != ArbitrationStatusPrepared {
		t.Fatalf("calls/prepared = %d/%+v", comparator.calls, prepared)
	}
}

func TestArbitrationPrepareFailsClosedForIncompleteActiveWork(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	store.snapshot.ActiveWorks = []SnapshotWork{{
		ID: uuid.New(), ExactSignatureHash: "different", SignaturePayload: nil,
	}}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if comparator.calls != 0 || prepared.Decision != DecisionHold || prepared.Status != ArbitrationStatusHeld {
		t.Fatalf("incomplete active work did not fail closed: calls=%d prepared=%+v", comparator.calls, prepared)
	}
}

func TestArbitrationPrepareAllowsSuppressionOnlyAfterLaunchGate(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	existingID := uuid.New()
	store.snapshot.ActiveWorks = []SnapshotWork{{ID: existingID, ExactSignatureHash: "different", SignaturePayload: candidate.Identity.SignaturePayload}}
	store.config.LaunchReady = true
	store.config.AutomaticSuppressionEnabled = true
	comparator.decision = SemanticDecision{
		Decision: DecisionSuppress, Owner: OwnerDoctor,
		WorkSignature: candidate.Identity.ExactSignatureHash,
		Overlaps:      []uuid.UUID{existingID}, Reason: "same work", Confidence: 0.98,
		SemanticFingerprint: "semantic-candidate",
	}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Decision != DecisionSuppress || prepared.Status != ArbitrationStatusPrepared || len(store.holds) != 0 {
		t.Fatalf("launch-ready suppression was not prepared: %+v", prepared)
	}
}

func arbitrationFixture(t *testing.T) (*arbitrationStoreStub, *semanticComparatorStub, ArbitrationCandidate) {
	t.Helper()
	candidate := Candidate{
		ProjectID:           uuid.New(),
		NormalizedTargetSet: []string{"https://example.com/pricing"},
		ChangeFamily:        "metadata.title",
		ProposedMutations:   []Mutation{{Operation: "update", Field: "title"}},
		ArtifactIntent:      ArtifactUpdateExistingContent,
		SuggestedOwner:      OwnerOpportunities,
		EvidenceFingerprint: "evidence-v1",
		SignatureVersion:    SignatureVersionV1,
		Status:              StatusIdentityReady,
	}
	identity, err := BuildIdentity(candidate)
	if err != nil {
		t.Fatal(err)
	}
	arbitrationCandidate := ArbitrationCandidate{ID: uuid.New(), Version: 1, Candidate: candidate, Identity: identity}
	store := &arbitrationStoreStub{
		candidate: arbitrationCandidate,
		snapshot:  BucketSnapshot{Versions: map[string]int64{identity.ConflictBucketKeys[0]: 0}},
		config:    ArbitrationConfig{ConfidenceThreshold: 0.80, RulesVersion: "discovery-arbitration-v1"},
	}
	comparator := &semanticComparatorStub{
		decision: SemanticDecision{
			Decision: DecisionCreate, Owner: OwnerOpportunities,
			WorkSignature: identity.ExactSignatureHash, Reason: "distinct work", Confidence: 0.95,
			SemanticFingerprint: "semantic-candidate",
		},
		usage: CallUsage{Provider: "tokengate", Model: "claude-sonnet-4-6", PromptVersion: SemanticPromptVersionV1, RequestFingerprint: "request-v1", TotalTokens: 20},
	}
	return store, comparator, arbitrationCandidate
}

type arbitrationStoreStub struct {
	candidate  ArbitrationCandidate
	snapshot   BucketSnapshot
	config     ArbitrationConfig
	events     []string
	saved      []PreparedDecision
	holds      []ReviewHold
	aiFinishes []AICallFinish
}

func (s *arbitrationStoreStub) LoadCandidate(_ context.Context, _ uuid.UUID) (ArbitrationCandidate, error) {
	s.events = append(s.events, "load")
	return s.candidate, nil
}
func (s *arbitrationStoreStub) MaterializeBuckets(_ context.Context, _ uuid.UUID, _ []string) error {
	s.events = append(s.events, "materialize")
	return nil
}
func (s *arbitrationStoreStub) ReadSnapshot(_ context.Context, _ uuid.UUID, _ []string) (BucketSnapshot, error) {
	s.events = append(s.events, "snapshot")
	return s.snapshot, nil
}
func (s *arbitrationStoreStub) LoadConfig(_ context.Context, _ uuid.UUID) (ArbitrationConfig, error) {
	s.events = append(s.events, "config")
	return s.config, nil
}
func (s *arbitrationStoreStub) StartAICall(_ context.Context, start AICallStart) (uuid.UUID, error) {
	s.events = append(s.events, "ai_start")
	return uuid.New(), nil
}
func (s *arbitrationStoreStub) FinishAICall(_ context.Context, finish AICallFinish) error {
	s.events = append(s.events, "ai_finish")
	s.aiFinishes = append(s.aiFinishes, finish)
	return nil
}
func (s *arbitrationStoreStub) SavePreparedDecision(_ context.Context, decision PreparedDecision) (PreparedDecision, error) {
	s.events = append(s.events, "save")
	decision.ID = uuid.New()
	s.saved = append(s.saved, decision)
	return decision, nil
}
func (s *arbitrationStoreStub) SaveReviewHold(_ context.Context, hold ReviewHold) error {
	s.events = append(s.events, "hold")
	s.holds = append(s.holds, hold)
	return nil
}

type semanticComparatorStub struct {
	decision SemanticDecision
	usage    CallUsage
	err      error
	calls    int
	events   *[]string
}

func (s *semanticComparatorStub) Compare(_ context.Context, _ SemanticRequest) (SemanticDecision, CallUsage, error) {
	s.calls++
	if s.events != nil {
		*s.events = append(*s.events, "compare")
	}
	return s.decision, s.usage, s.err
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

var _ = time.Time{}
