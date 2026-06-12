package workflow

import (
	"context"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

const (
	EventContextConfirmed     = "context.confirmed"
	EventOpportunityReviewed  = "opportunity.reviewed"
	EventOpportunityBatchDone = "opportunity.batch_completed"
	EventContentPlanCreated   = "content_plan.created"
	EventDraftsRequested      = "drafts.requested"
	EventDraftReadyForReview  = "draft.ready_for_review"
	EventDraftApproved        = "draft.approved"
	EventArticlePublished     = "article.published"
	EventMeasurementWindowDue = "measurement.window_due"
)

type Store interface {
	ReclaimStuckWorkflowEvents(context.Context, int32) ([]db.WorkflowEvent, error)
	ClaimPendingWorkflowEvents(context.Context, int32) ([]db.WorkflowEvent, error)
	MarkWorkflowEventSucceeded(context.Context, uuid.UUID) (db.WorkflowEvent, error)
	MarkWorkflowEventFailed(context.Context, db.MarkWorkflowEventFailedParams) (db.WorkflowEvent, error)
}

type Handler func(context.Context, db.WorkflowEvent) error

type Worker struct {
	Store  Store
	Handle Handler
	Limit  int32
}

func (w Worker) ProcessOnce(ctx context.Context) (int, error) {
	limit := w.Limit
	if limit <= 0 {
		limit = 20
	}
	if _, err := w.Store.ReclaimStuckWorkflowEvents(ctx, limit); err != nil {
		return 0, err
	}
	events, err := w.Store.ClaimPendingWorkflowEvents(ctx, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, event := range events {
		processed++
		if w.Handle != nil {
			if err := w.Handle(ctx, event); err != nil {
				msg := err.Error()
				if _, markErr := w.Store.MarkWorkflowEventFailed(ctx, db.MarkWorkflowEventFailedParams{
					ID:    event.ID,
					Error: &msg,
				}); markErr != nil {
					return processed, markErr
				}
				continue
			}
		}
		if _, err := w.Store.MarkWorkflowEventSucceeded(ctx, event.ID); err != nil {
			return processed, err
		}
	}
	return processed, nil
}
