package notification

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeDispatchStore struct {
	subs    []db.NotificationSubscription
	created []db.CreateNotificationDeliveryParams
	err     error
}

func (s *fakeDispatchStore) ListEnabledNotificationSubscriptionsForEvent(ctx context.Context, arg db.ListEnabledNotificationSubscriptionsForEventParams) ([]db.NotificationSubscription, error) {
	return s.subs, nil
}

func (s *fakeDispatchStore) CreateNotificationDelivery(ctx context.Context, arg db.CreateNotificationDeliveryParams) (db.NotificationDelivery, error) {
	s.created = append(s.created, arg)
	return db.NotificationDelivery{}, s.err
}

func TestDispatchCreatesDeliveryForEachEnabledSubscription(t *testing.T) {
	projectID := uuid.New()
	channelID := uuid.New()
	subID := uuid.New()
	store := &fakeDispatchStore{subs: []db.NotificationSubscription{{
		ID:        subID,
		ProjectID: projectID,
		EventType: "publish.failed",
		ChannelID: channelID,
		Enabled:   true,
	}}}
	event := Event{
		ProjectID: projectID,
		Type:      "publish.failed",
		ID:        "publish.failed:article-1:1",
		Payload:   json.RawMessage(`{"title":"Failed"}`),
	}

	if err := Dispatch(context.Background(), store, event); err != nil {
		t.Fatal(err)
	}
	if len(store.created) != 1 {
		t.Fatalf("created deliveries = %d, want 1", len(store.created))
	}
	got := store.created[0]
	if got.ProjectID != projectID || got.ChannelID != channelID || got.EventType != event.Type || got.EventID != event.ID {
		t.Fatalf("delivery params mismatch: %+v", got)
	}
	if got.SubscriptionID.Bytes != subID || !got.SubscriptionID.Valid {
		t.Fatalf("subscription id = %+v", got.SubscriptionID)
	}
}

func TestDispatchIgnoresDuplicateDelivery(t *testing.T) {
	projectID := uuid.New()
	store := &fakeDispatchStore{
		subs: []db.NotificationSubscription{{
			ID:        uuid.New(),
			ProjectID: projectID,
			EventType: "budget.stopped",
			ChannelID: uuid.New(),
			Enabled:   true,
		}},
		err: pgx.ErrNoRows,
	}

	err := Dispatch(context.Background(), store, Event{
		ProjectID: projectID,
		Type:      "budget.stopped",
		ID:        BudgetStoppedEventID(projectID.String(), "2026-06", "50"),
		Payload:   json.RawMessage(`{"budget_usd":50}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.created) != 1 {
		t.Fatalf("created deliveries = %d, want 1", len(store.created))
	}
}

func TestDispatchReturnsCreateErrors(t *testing.T) {
	projectID := uuid.New()
	store := &fakeDispatchStore{
		subs: []db.NotificationSubscription{{
			ID:        uuid.New(),
			ProjectID: projectID,
			EventType: "review.overdue",
			ChannelID: uuid.New(),
			Enabled:   true,
		}},
		err: errors.New("db down"),
	}

	err := Dispatch(context.Background(), store, Event{
		ProjectID: projectID,
		Type:      "review.overdue",
		ID:        "review.overdue:article-1:2026-06-05",
		Payload:   json.RawMessage(`{"title":"Review"}`),
	})
	if err == nil {
		t.Fatal("expected create error to be returned")
	}
}
