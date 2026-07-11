package workflow

import (
	"context"
	"errors"

	"github.com/citeloop/citeloop/internal/db"
)

const (
	EventContextConfirmed            = "context.confirmed"
	EventOpportunityReviewed         = "opportunity.reviewed"
	EventOpportunityFindingRequested = "opportunity_finding.requested"
	EventOpportunityBatchDone        = "opportunity.batch_completed"
	EventContentPlanCreated          = "content_plan.created"
	EventDraftsRequested             = "drafts.requested"
	EventDraftReadyForReview         = "draft.ready_for_review"
	EventDraftApproved               = "draft.approved"
	EventArticlePublished            = "article.published"
	EventMeasurementWindowDue        = "measurement.window_due"
)

type Store interface {
	ReclaimStuckWorkflowEvents(context.Context, int32) ([]db.WorkflowEvent, error)
	ClaimPendingWorkflowEvents(context.Context, int32) ([]db.WorkflowEvent, error)
	MarkWorkflowEventSucceeded(context.Context, db.MarkWorkflowEventSucceededParams) (db.WorkflowEvent, error)
	MarkWorkflowEventFailed(context.Context, db.MarkWorkflowEventFailedParams) (db.WorkflowEvent, error)
	MarkWorkflowEventDead(context.Context, db.MarkWorkflowEventDeadParams) (db.WorkflowEvent, error)
}

type PermanentError struct{ Err error }

func (e PermanentError) Error() string { return e.Err.Error() }
func (e PermanentError) Unwrap() error { return e.Err }

func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return PermanentError{Err: err}
}

func IsPermanent(err error) bool {
	var permanent PermanentError
	return errors.As(err, &permanent)
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
				var markErr error
				if IsPermanent(err) {
					_, markErr = w.Store.MarkWorkflowEventDead(ctx, db.MarkWorkflowEventDeadParams{ID: event.ID, Error: &msg, ExpectedAttempts: event.Attempts})
				} else {
					_, markErr = w.Store.MarkWorkflowEventFailed(ctx, db.MarkWorkflowEventFailedParams{ID: event.ID, Error: &msg, ExpectedAttempts: event.Attempts})
				}
				if markErr != nil {
					return processed, markErr
				}
				continue
			}
		}
		if _, err := w.Store.MarkWorkflowEventSucceeded(ctx, db.MarkWorkflowEventSucceededParams{ID: event.ID, ExpectedAttempts: event.Attempts}); err != nil {
			return processed, err
		}
	}
	return processed, nil
}
