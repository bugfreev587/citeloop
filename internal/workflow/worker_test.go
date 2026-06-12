package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

type fakeStore struct {
	reclaimed []db.WorkflowEvent
	claimed   []db.WorkflowEvent
	succeeded []uuid.UUID
	failed    []db.MarkWorkflowEventFailedParams
}

func (s *fakeStore) ReclaimStuckWorkflowEvents(ctx context.Context, limit int32) ([]db.WorkflowEvent, error) {
	return s.reclaimed, nil
}

func (s *fakeStore) ClaimPendingWorkflowEvents(ctx context.Context, limit int32) ([]db.WorkflowEvent, error) {
	return s.claimed, nil
}

func (s *fakeStore) MarkWorkflowEventSucceeded(ctx context.Context, id uuid.UUID) (db.WorkflowEvent, error) {
	s.succeeded = append(s.succeeded, id)
	return db.WorkflowEvent{ID: id, Status: "succeeded"}, nil
}

func (s *fakeStore) MarkWorkflowEventFailed(ctx context.Context, arg db.MarkWorkflowEventFailedParams) (db.WorkflowEvent, error) {
	s.failed = append(s.failed, arg)
	return db.WorkflowEvent{ID: arg.ID, Status: "pending"}, nil
}

func TestWorkerProcessesClaimedEventAndMarksSucceeded(t *testing.T) {
	eventID := uuid.New()
	store := &fakeStore{claimed: []db.WorkflowEvent{{
		ID:        eventID,
		ProjectID: uuid.New(),
		EventType: EventOpportunityReviewed,
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
	if len(store.failed) != 0 {
		t.Fatalf("unexpected failed marks = %+v", store.failed)
	}
}

func TestWorkerMarksFailedWhenHandlerFails(t *testing.T) {
	eventID := uuid.New()
	store := &fakeStore{claimed: []db.WorkflowEvent{{ID: eventID, EventType: EventDraftApproved}}}
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
