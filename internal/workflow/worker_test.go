package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

type fakeStore struct {
	reclaimed       []db.WorkflowEvent
	claimed         []db.WorkflowEvent
	succeeded       []uuid.UUID
	succeededClaims []db.MarkWorkflowEventSucceededParams
	failed          []db.MarkWorkflowEventFailedParams
	dead            []db.MarkWorkflowEventDeadParams
}

func (s *fakeStore) ReclaimStuckWorkflowEvents(ctx context.Context, limit int32) ([]db.WorkflowEvent, error) {
	return s.reclaimed, nil
}

func (s *fakeStore) ClaimPendingWorkflowEvents(ctx context.Context, limit int32) ([]db.WorkflowEvent, error) {
	return s.claimed, nil
}

func (s *fakeStore) MarkWorkflowEventSucceeded(ctx context.Context, arg db.MarkWorkflowEventSucceededParams) (db.WorkflowEvent, error) {
	s.succeeded = append(s.succeeded, arg.ID)
	s.succeededClaims = append(s.succeededClaims, arg)
	return db.WorkflowEvent{ID: arg.ID, Status: "succeeded"}, nil
}

func (s *fakeStore) MarkWorkflowEventFailed(ctx context.Context, arg db.MarkWorkflowEventFailedParams) (db.WorkflowEvent, error) {
	s.failed = append(s.failed, arg)
	return db.WorkflowEvent{ID: arg.ID, Status: "pending"}, nil
}

func (s *fakeStore) MarkWorkflowEventDead(ctx context.Context, arg db.MarkWorkflowEventDeadParams) (db.WorkflowEvent, error) {
	s.dead = append(s.dead, arg)
	return db.WorkflowEvent{ID: arg.ID, Status: "dead"}, nil
}

func TestWorkerProcessesClaimedEventAndMarksSucceeded(t *testing.T) {
	eventID := uuid.New()
	store := &fakeStore{claimed: []db.WorkflowEvent{{
		ID:        eventID,
		ProjectID: uuid.New(),
		EventType: EventOpportunityReviewed,
		Attempts:  2,
	}}}
	handled := []uuid.UUID{}
	worker := Worker{
		Store: store,
		Handle: func(ctx context.Context, event db.WorkflowEvent) error {
			handled = append(handled, event.ID)
			return nil
		},
		Limit: 5,
	}

	processed, err := worker.ProcessOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if len(handled) != 1 || handled[0] != eventID {
		t.Fatalf("handled = %+v, want [%s]", handled, eventID)
	}
	if len(store.succeeded) != 1 || store.succeeded[0] != eventID {
		t.Fatalf("succeeded marks = %+v", store.succeeded)
	}
	if store.succeededClaims[0].ExpectedAttempts != 2 {
		t.Fatalf("success claim attempt = %d", store.succeededClaims[0].ExpectedAttempts)
	}
	if len(store.failed) != 0 {
		t.Fatalf("unexpected failed marks = %+v", store.failed)
	}
}

func TestWorkerMarksFailedWhenHandlerFails(t *testing.T) {
	eventID := uuid.New()
	store := &fakeStore{claimed: []db.WorkflowEvent{{ID: eventID, EventType: EventDraftApproved, Attempts: 3}}}
	worker := Worker{
		Store:  store,
		Handle: func(ctx context.Context, event db.WorkflowEvent) error { return errors.New("model timeout") },
	}

	processed, err := worker.ProcessOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if len(store.succeeded) != 0 {
		t.Fatalf("unexpected succeeded marks = %+v", store.succeeded)
	}
	if len(store.failed) != 1 || store.failed[0].ID != eventID {
		t.Fatalf("failed marks = %+v", store.failed)
	}
	if store.failed[0].Error == nil || *store.failed[0].Error != "model timeout" {
		t.Fatalf("failed error = %+v", store.failed[0].Error)
	}
	if store.failed[0].ExpectedAttempts != 3 {
		t.Fatalf("failed claim attempt = %d", store.failed[0].ExpectedAttempts)
	}
}

func TestWorkerMarksPermanentFailureDeadWithoutRetry(t *testing.T) {
	eventID := uuid.New()
	store := &fakeStore{claimed: []db.WorkflowEvent{{ID: eventID, EventType: EventOpportunityFindingRequested, Attempts: 1}}}
	worker := Worker{
		Store:  store,
		Handle: func(context.Context, db.WorkflowEvent) error { return Permanent(errors.New("partial provider run")) },
	}

	processed, err := worker.ProcessOnce(context.Background())
	if err != nil || processed != 1 {
		t.Fatalf("processed=%d err=%v", processed, err)
	}
	if len(store.failed) != 0 || len(store.dead) != 1 || store.dead[0].ID != eventID {
		t.Fatalf("failed=%+v dead=%+v", store.failed, store.dead)
	}
	if store.dead[0].Error == nil || *store.dead[0].Error != "partial provider run" {
		t.Fatalf("dead error=%+v", store.dead[0].Error)
	}
	if store.dead[0].ExpectedAttempts != 1 {
		t.Fatalf("dead claim attempt = %d", store.dead[0].ExpectedAttempts)
	}
}

func TestWorkerReclaimsStuckEventsBeforeClaiming(t *testing.T) {
	stuckID := uuid.New()
	eventID := uuid.New()
	store := &fakeStore{
		reclaimed: []db.WorkflowEvent{{ID: stuckID}},
		claimed:   []db.WorkflowEvent{{ID: eventID}},
	}
	worker := Worker{
		Store:  store,
		Handle: func(ctx context.Context, event db.WorkflowEvent) error { return nil },
		Limit:  10,
	}

	processed, err := worker.ProcessOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1 claimed event", processed)
	}
	if len(store.succeeded) != 1 || store.succeeded[0] != eventID {
		t.Fatalf("succeeded marks = %+v", store.succeeded)
	}
}
