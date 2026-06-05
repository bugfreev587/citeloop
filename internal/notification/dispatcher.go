package notification

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Event struct {
	ProjectID uuid.UUID
	Type      string
	ID        string
	Payload   json.RawMessage
}

type DispatchStore interface {
	ListEnabledNotificationSubscriptionsForEvent(context.Context, db.ListEnabledNotificationSubscriptionsForEventParams) ([]db.NotificationSubscription, error)
	CreateNotificationDelivery(context.Context, db.CreateNotificationDeliveryParams) (db.NotificationDelivery, error)
}

func Dispatch(ctx context.Context, store DispatchStore, event Event) error {
	subs, err := store.ListEnabledNotificationSubscriptionsForEvent(ctx, db.ListEnabledNotificationSubscriptionsForEventParams{
		ProjectID: event.ProjectID,
		EventType: event.Type,
	})
	if err != nil {
		return err
	}
	for _, sub := range subs {
		_, err := store.CreateNotificationDelivery(ctx, db.CreateNotificationDeliveryParams{
			ProjectID:      event.ProjectID,
			SubscriptionID: pgtype.UUID{Bytes: sub.ID, Valid: true},
			ChannelID:      sub.ChannelID,
			EventType:      event.Type,
			EventID:        event.ID,
			Payload:        event.Payload,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}
