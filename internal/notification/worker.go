package notification

import (
	"context"
	"encoding/json"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

type WorkerStore interface {
	DispatchStore
	ListPendingNotificationDeliveries(context.Context, int32) ([]db.ListPendingNotificationDeliveriesRow, error)
	MarkNotificationDeliverySent(context.Context, uuid.UUID) (db.NotificationDelivery, error)
	MarkNotificationDeliveryFailed(context.Context, db.MarkNotificationDeliveryFailedParams) (db.NotificationDelivery, error)
}

type Sender interface {
	Send(ctx context.Context, kind, webhookURL string, payload json.RawMessage) error
}

type Worker struct {
	Store  WorkerStore
	Sender Sender
	Secret string
	Limit  int32
}

func (w Worker) ProcessOnce(ctx context.Context) (int, error) {
	limit := w.Limit
	if limit <= 0 {
		limit = 20
	}
	rows, err := w.Store.ListPendingNotificationDeliveries(ctx, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, row := range rows {
		processed++
		var cfg WebhookConfig
		if err := json.Unmarshal(row.ChannelConfig, &cfg); err != nil {
			if markErr := w.markFailed(ctx, row, err); markErr != nil {
				return processed, markErr
			}
			continue
		}
		webhookURL, err := DecryptWebhookURL(cfg, w.Secret)
		if err != nil {
			if markErr := w.markFailed(ctx, row, err); markErr != nil {
				return processed, markErr
			}
			continue
		}
		if err := w.Sender.Send(ctx, row.ChannelKind, webhookURL, row.Payload); err != nil {
			if markErr := w.markFailed(ctx, row, err); markErr != nil {
				return processed, markErr
			}
			continue
		}
		if _, err := w.Store.MarkNotificationDeliverySent(ctx, row.ID); err != nil {
			return processed, err
		}
	}
	return processed, nil
}

func (w Worker) markFailed(ctx context.Context, row db.ListPendingNotificationDeliveriesRow, err error) error {
	msg := err.Error()
	delivery, markErr := w.Store.MarkNotificationDeliveryFailed(ctx, db.MarkNotificationDeliveryFailedParams{
		ID:        row.ID,
		LastError: &msg,
	})
	if markErr != nil {
		return markErr
	}
	if delivery.Status == "dead" && row.EventType != "webhook.delivery.dead" {
		event := NewWebhookDeliveryDeadEvent(
			row.ProjectID,
			row.ID,
			row.ChannelID,
			row.EventType,
			msg,
			"/projects/"+row.ProjectID.String()+"/settings",
		)
		return Dispatch(ctx, w.Store, event)
	}
	return nil
}
