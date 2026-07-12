package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestArbitrationPrepareDeterministicSafeWithoutProvider(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	service := NewArbitrationService(store, comparator)

	prepared, err := service.Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
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

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
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

func TestArbitrationPrepareHardCrossLineOverlapStillUsesSemanticComparator(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	doctor := dependencyCandidate(candidate.Candidate.ProjectID, OwnerDoctor, VerificationImmediate, "metadata.title", "add", "title")
	doctorIdentity, err := BuildIdentity(doctor)
	if err != nil {
		t.Fatal(err)
	}
	doctorSignatureID := uuid.New()
	store.snapshot.ActiveWorks = []SnapshotWork{{
		ID: doctorSignatureID, Owner: OwnerDoctor, ExactSignatureHash: doctorIdentity.ExactSignatureHash,
		SignaturePayload: doctorIdentity.SignaturePayload,
	}}
	comparator.decision = SemanticDecision{
		Decision: DecisionMergeEvidence, Owner: OwnerDoctor,
		WorkSignature: candidate.Identity.ExactSignatureHash, Overlaps: []uuid.UUID{doctorSignatureID},
		Reason: "both candidates describe the same title correction", Confidence: 0.97,
		SemanticFingerprint: "same-title-correction",
	}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if comparator.calls != 1 {
		t.Fatalf("provider calls = %d, want 1 for semantic equivalence check", comparator.calls)
	}
	if prepared.Decision != DecisionMergeEvidence || prepared.Owner != OwnerDoctor || prepared.Status != ArbitrationStatusPrepared {
		t.Fatalf("prepared = %+v", prepared)
	}
	if len(prepared.OverlapWorkIDs) != 1 || prepared.OverlapWorkIDs[0] != doctorSignatureID {
		t.Fatalf("overlaps = %v", prepared.OverlapWorkIDs)
	}
}

func TestArbitrationPrepareSemanticallyDistinctHardOverlapRemainsBlocked(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	doctor := dependencyCandidate(candidate.Candidate.ProjectID, OwnerDoctor, VerificationImmediate, "metadata.title", "add", "title")
	doctorIdentity, err := BuildIdentity(doctor)
	if err != nil {
		t.Fatal(err)
	}
	doctorSignatureID := uuid.New()
	store.snapshot.ActiveWorks = []SnapshotWork{{
		ID: doctorSignatureID, Owner: OwnerDoctor, ExactSignatureHash: doctorIdentity.ExactSignatureHash,
		SignaturePayload: doctorIdentity.SignaturePayload,
	}}
	comparator.decision = SemanticDecision{
		Decision: DecisionCreate, Owner: OwnerOpportunities,
		WorkSignature: candidate.Identity.ExactSignatureHash,
		Reason:        "distinct intent but the same title field", Confidence: 0.96,
		SemanticFingerprint: "distinct-title-work",
	}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if comparator.calls != 1 || prepared.Decision != DecisionBlockOnOtherLine || prepared.Owner != OwnerOpportunities {
		t.Fatalf("calls/prepared = %d/%+v", comparator.calls, prepared)
	}
	if len(prepared.OverlapWorkIDs) != 1 || prepared.OverlapWorkIDs[0] != doctorSignatureID {
		t.Fatalf("hard blocker overlap was lost: %v", prepared.OverlapWorkIDs)
	}
}

func TestArbitrationPrepareSoftCrossLineDependencyStillUsesSemanticComparator(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	candidate.Candidate.ChangeFamily = "content.evidence"
	candidate.Candidate.ProposedMutations = []Mutation{{Operation: "update", Field: "evidence_block"}}
	identity, err := BuildIdentity(candidate.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Identity = identity
	store.candidate = candidate
	store.snapshot.Versions = map[string]int64{identity.ConflictBucketKeys[0]: 0}
	doctor := dependencyCandidate(candidate.Candidate.ProjectID, OwnerDoctor, VerificationImmediate, "content.evidence", "move", "answer_block")
	doctorIdentity, err := BuildIdentity(doctor)
	if err != nil {
		t.Fatal(err)
	}
	doctorSignatureID := uuid.New()
	store.snapshot.ActiveWorks = []SnapshotWork{{
		ID: doctorSignatureID, Owner: OwnerDoctor, ExactSignatureHash: doctorIdentity.ExactSignatureHash,
		SignaturePayload: doctorIdentity.SignaturePayload,
	}}
	comparator.decision = SemanticDecision{
		Decision: DecisionMergeEvidence, Owner: OwnerDoctor,
		WorkSignature: identity.ExactSignatureHash, Overlaps: []uuid.UUID{doctorSignatureID},
		Reason: "answer_block and evidence_block describe the same mutation", Confidence: 0.96,
		SemanticFingerprint: "semantic-equivalent-block",
	}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if comparator.calls != 1 || prepared.Decision != DecisionMergeEvidence || prepared.Owner != OwnerDoctor {
		t.Fatalf("calls/prepared = %d/%+v", comparator.calls, prepared)
	}
	if len(prepared.OverlapWorkIDs) != 1 || prepared.OverlapWorkIDs[0] != doctorSignatureID {
		t.Fatalf("overlaps = %v", prepared.OverlapWorkIDs)
	}
}

func TestArbitrationPreparePersistsSoftRelationshipAfterSemanticDistinct(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	doctor := dependencyCandidate(candidate.Candidate.ProjectID, OwnerDoctor, VerificationImmediate, "metadata.description", "add", "meta_description")
	doctorIdentity, err := BuildIdentity(doctor)
	if err != nil {
		t.Fatal(err)
	}
	doctorSignatureID := uuid.New()
	store.snapshot.ActiveWorks = []SnapshotWork{{
		ID: doctorSignatureID, Owner: OwnerDoctor, ExactSignatureHash: doctorIdentity.ExactSignatureHash,
		SignaturePayload: doctorIdentity.SignaturePayload,
	}}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if comparator.calls != 1 || prepared.Decision != DecisionCreate {
		t.Fatalf("calls/prepared = %d/%+v", comparator.calls, prepared)
	}
	if len(prepared.OverlapWorkIDs) != 1 || prepared.OverlapWorkIDs[0] != doctorSignatureID {
		t.Fatalf("soft relationship overlap was lost after semantic distinct: %v", prepared.OverlapWorkIDs)
	}
}

func TestArbitrationPrepareSoftCrossLineFailsClosedWithoutComparator(t *testing.T) {
	store, _, candidate := arbitrationFixture(t)
	doctor := dependencyCandidate(candidate.Candidate.ProjectID, OwnerDoctor, VerificationImmediate, "metadata.description", "add", "meta_description")
	doctorIdentity, err := BuildIdentity(doctor)
	if err != nil {
		t.Fatal(err)
	}
	store.snapshot.ActiveWorks = []SnapshotWork{{
		ID: uuid.New(), Owner: OwnerDoctor, ExactSignatureHash: doctorIdentity.ExactSignatureHash,
		SignaturePayload: doctorIdentity.SignaturePayload,
	}}

	prepared, err := NewArbitrationService(store, nil).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Decision != DecisionHold || prepared.Status != ArbitrationStatusHeld || prepared.Disposition != DispositionProviderFailure {
		t.Fatalf("soft cross-line overlap did not fail closed: %+v", prepared)
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

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []string{"load", "materialize", "snapshot", "config", "ai_start", "compare", "ai_finish", "save"}
	if !equalStrings(store.events, wantEvents) {
		t.Fatalf("events = %v, want %v", store.events, wantEvents)
	}
	if prepared.Decision != DecisionBlockOnOtherLine || prepared.Owner != OwnerOpportunities || prepared.Status != ArbitrationStatusPrepared {
		t.Fatalf("prepared = %+v", prepared)
	}
}

func TestArbitrationCreateCannotReassignCandidateOwner(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	existingID := uuid.New()
	store.snapshot.ActiveWorks = []SnapshotWork{{
		ID: existingID, Owner: OwnerOpportunities, ExactSignatureHash: "different",
		SignaturePayload: candidate.Identity.SignaturePayload,
	}}
	comparator.decision = SemanticDecision{
		Decision: DecisionCreate, Owner: OwnerDoctor,
		WorkSignature: candidate.Identity.ExactSignatureHash,
		Reason:        "distinct Growth work", Confidence: 0.95, SemanticFingerprint: "distinct-growth-work",
	}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Decision != DecisionCreate || prepared.Owner != OwnerOpportunities || prepared.Status != ArbitrationStatusPrepared {
		t.Fatalf("provider reassigned Growth create owner: %+v", prepared)
	}
}

func TestArbitrationMergeDerivesOwnerFromCanonicalOverlap(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	existingID := uuid.New()
	store.snapshot.ActiveWorks = []SnapshotWork{{
		ID: existingID, Owner: OwnerDoctor, ExactSignatureHash: "different",
		SignaturePayload: candidate.Identity.SignaturePayload,
	}}
	comparator.decision = SemanticDecision{
		Decision: DecisionMergeEvidence, Owner: OwnerOpportunities,
		WorkSignature: candidate.Identity.ExactSignatureHash, Overlaps: []uuid.UUID{existingID},
		Reason: "same canonical work", Confidence: 0.95, SemanticFingerprint: "same-work",
	}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Decision != DecisionMergeEvidence || prepared.Owner != OwnerDoctor || prepared.Status != ArbitrationStatusPrepared {
		t.Fatalf("provider reassigned canonical merge owner: %+v", prepared)
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

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
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

func TestArbitrationPreparePreservesNonSemanticHoldState(t *testing.T) {
	for _, status := range []CandidateStatus{StatusNeedsSpecification, StatusNeedsEvidence} {
		t.Run(string(status), func(t *testing.T) {
			store, comparator, candidate := arbitrationFixture(t)
			candidate.Candidate.Status = status
			candidate.Identity = Identity{}
			store.candidate = candidate

			prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
			if err != nil {
				t.Fatal(err)
			}
			if prepared.Disposition != DispositionIncompleteSpecification || len(store.holds) != 1 {
				t.Fatalf("prepared/holds = %+v/%+v", prepared, store.holds)
			}
			if store.holds[0].State != status {
				t.Fatalf("review state = %q, want %q", store.holds[0].State, status)
			}
		})
	}
}

func TestArbitrationPrepareFailsClosedForProviderFailure(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	store.snapshot.ActiveWorks = []SnapshotWork{{
		ID: uuid.New(), ExactSignatureHash: "different", SignaturePayload: candidate.Identity.SignaturePayload,
	}}
	comparator.err = errors.New("provider unavailable")

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
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

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
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

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if comparator.calls != 0 || prepared.Disposition != DispositionReviewMemory || prepared.Decision != DecisionSuppress || prepared.Status != ArbitrationStatusResolved {
		t.Fatalf("calls/prepared = %d/%+v", comparator.calls, prepared)
	}
}

func TestArbitrationPrepareAppliesExactReviewMemoryAliasWithoutProvider(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	memoryID := uuid.New()
	store.snapshot.ReviewMemory = []ReviewMemorySnapshot{{
		ID: memoryID, Decision: "dismissed", ExactSignatureHash: "old-exact",
		SignaturePayload:    candidate.Identity.SignaturePayload,
		EvidenceFingerprint: candidate.Candidate.EvidenceFingerprint,
		SignatureVersion:    "work-signature-v0", Active: true,
	}}
	store.snapshot.ReviewAliases = []ReviewMemoryAliasSnapshot{{
		ReviewMemoryID: memoryID, ExactSignatureHash: candidate.Identity.ExactSignatureHash,
		SemanticFingerprint: "semantic-alias", SignatureVersion: SignatureVersionV1,
	}}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Disposition != DispositionReviewMemory || prepared.Status != ArbitrationStatusResolved || comparator.calls != 0 {
		t.Fatalf("prepared=%+v comparator calls=%d", prepared, comparator.calls)
	}
}

func TestArbitrationPrepareInheritsHighConfidenceSemanticReviewMemoryBeforeLaunchGate(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	memoryID := uuid.New()
	store.snapshot.ReviewMemory = []ReviewMemorySnapshot{{
		ID: memoryID, Decision: "dismissed", ExactSignatureHash: "different",
		SignaturePayload:    json.RawMessage(`{"change_family":"metadata"}`),
		EvidenceFingerprint: candidate.Candidate.EvidenceFingerprint, SignatureVersion: SignatureVersionV1, Active: true,
	}}
	store.config.LaunchReady = false
	store.config.AutomaticSuppressionEnabled = false
	comparator.decision = SemanticDecision{
		Decision: DecisionSuppress, Owner: OwnerDoctor,
		WorkSignature: candidate.Identity.ExactSignatureHash,
		Overlaps:      []uuid.UUID{memoryID}, Reason: "same reviewed work", Confidence: 0.80,
		SemanticFingerprint: "semantic-candidate",
	}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Decision != DecisionSuppress || prepared.Status != ArbitrationStatusResolved {
		t.Fatalf("prepared = %+v", prepared)
	}
	if len(store.holds) != 0 {
		t.Fatalf("semantic review-memory inheritance created hold: %+v", store.holds)
	}
}

func TestArbitrationPrepareReopensDismissedMemoryAfterMaterialEvidenceChange(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	memoryID := uuid.New()
	store.snapshot.ReviewMemory = []ReviewMemorySnapshot{{
		ID: memoryID, Decision: "dismissed", ExactSignatureHash: candidate.Identity.ExactSignatureHash,
		SignaturePayload:    candidate.Identity.SignaturePayload,
		EvidenceFingerprint: "previous-evidence", SignatureVersion: SignatureVersionV1, Active: true,
	}}
	comparator.decision = SemanticDecision{
		Decision: DecisionSuppress, Owner: OwnerDoctor,
		WorkSignature: candidate.Identity.ExactSignatureHash,
		Overlaps:      []uuid.UUID{memoryID}, Reason: "same work", Confidence: 0.95,
		SemanticFingerprint: "semantic-candidate",
	}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if comparator.calls != 1 || prepared.Decision != DecisionCreate || prepared.Status != ArbitrationStatusPrepared {
		t.Fatalf("material change did not reopen: calls=%d prepared=%+v", comparator.calls, prepared)
	}
}

func TestArbitrationPrepareHoldsLowConfidenceSemanticReviewMemory(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	memoryID := uuid.New()
	store.snapshot.ReviewMemory = []ReviewMemorySnapshot{{
		ID: memoryID, Decision: "dismissed", ExactSignatureHash: "different",
		SignaturePayload:    candidate.Identity.SignaturePayload,
		EvidenceFingerprint: candidate.Candidate.EvidenceFingerprint,
		SignatureVersion:    SignatureVersionV1, Active: true,
	}}
	comparator.decision = SemanticDecision{
		Decision: DecisionSuppress, Owner: OwnerDoctor,
		WorkSignature: candidate.Identity.ExactSignatureHash,
		Overlaps:      []uuid.UUID{memoryID}, Reason: "probably same", Confidence: 0.79,
		SemanticFingerprint: "semantic-candidate",
	}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Decision != DecisionHold || prepared.Status != ArbitrationStatusHeld || len(store.holds) != 1 {
		t.Fatalf("low confidence memory did not hold: %+v", prepared)
	}
}

func TestArbitrationPrepareDoesNotUseReviewMemoryToBypassActiveWorkSuppressionGate(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	memoryID := uuid.New()
	activeID := uuid.New()
	store.snapshot.ReviewMemory = []ReviewMemorySnapshot{{
		ID: memoryID, Decision: "dismissed", ExactSignatureHash: "different-memory",
		SignaturePayload:    candidate.Identity.SignaturePayload,
		EvidenceFingerprint: candidate.Candidate.EvidenceFingerprint,
		SignatureVersion:    SignatureVersionV1, Active: true,
	}}
	store.snapshot.ActiveWorks = []SnapshotWork{{
		ID: activeID, ExactSignatureHash: "different-active", SignaturePayload: candidate.Identity.SignaturePayload,
	}}
	store.config.LaunchReady = false
	store.config.AutomaticSuppressionEnabled = false
	comparator.decision = SemanticDecision{
		Decision: DecisionSuppress, Owner: OwnerDoctor,
		WorkSignature: candidate.Identity.ExactSignatureHash,
		Overlaps:      []uuid.UUID{memoryID, activeID}, Reason: "same work", Confidence: 0.95,
		SemanticFingerprint: "semantic-candidate",
	}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Decision != DecisionHold || prepared.Status != ArbitrationStatusHeld {
		t.Fatalf("mixed active/review overlap bypassed launch gate: %+v", prepared)
	}
}

func TestArbitrationPrepareKeepsUnexpiredSnoozeAfterMaterialEvidenceChange(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	store.snapshot.ReviewMemory = []ReviewMemorySnapshot{{
		ID: uuid.New(), Decision: "snoozed", ExactSignatureHash: candidate.Identity.ExactSignatureHash,
		SignaturePayload:    candidate.Identity.SignaturePayload,
		EvidenceFingerprint: "previous-evidence", SignatureVersion: SignatureVersionV1,
		SnoozedUntil: time.Now().Add(time.Hour), Active: true,
	}}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if comparator.calls != 0 || prepared.Decision != DecisionSuppress || prepared.Status != ArbitrationStatusResolved {
		t.Fatalf("unexpired snooze was reopened: calls=%d prepared=%+v", comparator.calls, prepared)
	}
}

func TestArbitrationPrepareFailsClosedForIncompleteActiveWork(t *testing.T) {
	store, comparator, candidate := arbitrationFixture(t)
	store.snapshot.ActiveWorks = []SnapshotWork{{
		ID: uuid.New(), ExactSignatureHash: "different", SignaturePayload: nil,
	}}

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
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

	prepared, err := NewArbitrationService(store, comparator).Prepare(context.Background(), candidate.Candidate.ProjectID, candidate.ID)
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

func (s *arbitrationStoreStub) LoadCandidate(_ context.Context, _ uuid.UUID, _ uuid.UUID) (ArbitrationCandidate, error) {
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
