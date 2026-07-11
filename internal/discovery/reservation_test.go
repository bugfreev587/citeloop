package discovery

import (
	"context"
	"errors"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

func TestReservePreparedUsesAtomicStoreWithoutSemanticProvider(t *testing.T) {
	decision := reservablePreparedDecision()
	store := &reservationStoreStub{decision: decision, config: ArbitrationConfig{ConfidenceThreshold: 0.80}}
	creator := &workCreatorStub{reference: WorkReference{Type: "site_fix", ID: uuid.New()}}

	result, err := NewReservationService(store).ReservePrepared(context.Background(), decision.ProjectID, decision.ID, creator)
	if err != nil {
		t.Fatalf("ReservePrepared: %v", err)
	}
	if store.atomicCalls != 1 || creator.calls != 1 {
		t.Fatalf("atomic/creator calls = %d/%d", store.atomicCalls, creator.calls)
	}
	if result.Work != creator.reference || result.SignatureID == uuid.Nil {
		t.Fatalf("result = %+v", result)
	}
}

func TestReservePreparedRejectsUnsafeDecisionBeforeTransaction(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*PreparedDecision, *ArbitrationConfig)
	}{
		{name: "held", mutate: func(d *PreparedDecision, _ *ArbitrationConfig) { d.Status = ArbitrationStatusHeld }},
		{name: "merge", mutate: func(d *PreparedDecision, _ *ArbitrationConfig) { d.Decision = DecisionMergeEvidence }},
		{name: "low confidence", mutate: func(d *PreparedDecision, c *ArbitrationConfig) { d.Confidence = 0.79; c.ConfidenceThreshold = 0.80 }},
		{name: "missing snapshot", mutate: func(d *PreparedDecision, _ *ArbitrationConfig) { d.ExpectedBucketVersions = nil }},
		{name: "missing project", mutate: func(d *PreparedDecision, _ *ArbitrationConfig) { d.ProjectID = uuid.Nil }},
		{name: "missing creator", mutate: func(_ *PreparedDecision, _ *ArbitrationConfig) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := reservablePreparedDecision()
			config := ArbitrationConfig{ConfidenceThreshold: 0.80}
			tt.mutate(&decision, &config)
			store := &reservationStoreStub{decision: decision, config: config}
			var creator WorkCreator = &workCreatorStub{reference: WorkReference{Type: "site_fix", ID: uuid.New()}}
			if tt.name == "missing creator" {
				creator = nil
			}
			projectID := decision.ProjectID
			if tt.name == "missing project" {
				projectID = uuid.Nil
			}
			if _, err := NewReservationService(store).ReservePrepared(context.Background(), projectID, decision.ID, creator); err == nil {
				t.Fatal("unsafe decision was reserved")
			}
			if store.atomicCalls != 0 {
				t.Fatalf("unsafe decision opened reservation transaction %d times", store.atomicCalls)
			}
		})
	}
}

func TestReservePreparedPropagatesSnapshotStale(t *testing.T) {
	decision := reservablePreparedDecision()
	store := &reservationStoreStub{
		decision:  decision,
		config:    ArbitrationConfig{ConfidenceThreshold: 0.80},
		atomicErr: ErrSnapshotStale,
	}
	_, err := NewReservationService(store).ReservePrepared(context.Background(), decision.ProjectID, decision.ID, &workCreatorStub{})
	if !errors.Is(err, ErrSnapshotStale) {
		t.Fatalf("error = %v, want ErrSnapshotStale", err)
	}
}

func reservablePreparedDecision() PreparedDecision {
	return PreparedDecision{
		ID: uuid.New(), ProjectID: uuid.New(), CandidateID: uuid.New(), CandidateVersion: 1,
		Disposition: DispositionDeterministicSafe, Decision: DecisionCreate,
		Owner: OwnerDoctor, Confidence: 1, Status: ArbitrationStatusPrepared,
		ExpectedBucketVersions: map[string]int64{"bucket-a": 2, "bucket-b": 4},
		SnapshotFingerprint:    "snapshot-v1", ExactSignatureHash: "exact-v1",
		SignatureVersion: SignatureVersionV1, SemanticFingerprint: "semantic-v1",
	}
}

type reservationStoreStub struct {
	decision    PreparedDecision
	config      ArbitrationConfig
	atomicCalls int
	atomicErr   error
}

func (s *reservationStoreStub) LoadPreparedDecision(_ context.Context, projectID, _ uuid.UUID) (PreparedDecision, error) {
	if projectID != s.decision.ProjectID {
		return PreparedDecision{}, errors.New("wrong project")
	}
	return s.decision, nil
}
func (s *reservationStoreStub) LoadReservationConfig(context.Context, uuid.UUID) (ArbitrationConfig, error) {
	return s.config, nil
}
func (s *reservationStoreStub) ReserveAtomically(ctx context.Context, decision PreparedDecision, creator WorkCreator) (ReservationResult, error) {
	s.atomicCalls++
	if s.atomicErr != nil {
		return ReservationResult{}, s.atomicErr
	}
	work, err := creator.CreateInTransaction(ctx, nil, ReservedWork{
		ProjectID: decision.ProjectID, CandidateID: decision.CandidateID,
		DecisionID: decision.ID, Owner: decision.Owner,
	})
	if err != nil {
		return ReservationResult{}, err
	}
	return ReservationResult{SignatureID: uuid.New(), Work: work}, nil
}

type workCreatorStub struct {
	reference WorkReference
	calls     int
	err       error
}

func (s *workCreatorStub) CreateInTransaction(_ context.Context, _ *db.Queries, _ ReservedWork) (WorkReference, error) {
	s.calls++
	return s.reference, s.err
}
