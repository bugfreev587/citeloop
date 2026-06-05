package notification

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

type fakeWorkerStore struct {
	rows         []db.ListPendingNotificationDeliveriesRow
	subs         []db.NotificationSubscription
	sent         []uuid.UUID
	failed       []db.MarkNotificationDeliveryFailedParams
	created      []db.CreateNotificationDeliveryParams
	failedReturn db.NotificationDelivery
}

func (s *fakeWorkerStore) ListPendingNotificationDeliveries(ctx context.Context, limit int32) ([]db.ListPendingNotificationDeliveriesRow, error) {
	return s.rows, nil
}

func (s *fakeWorkerStore) MarkNotificationDeliverySent(ctx context.Context, id uuid.UUID) (db.NotificationDelivery, error) {
	s.sent = append(s.sent, id)
	return db.NotificationDelivery{}, nil
}

func (s *fakeWorkerStore) MarkNotificationDeliveryFailed(ctx context.Context, arg db.MarkNotificationDeliveryFailedParams) (db.NotificationDelivery, error) {
	s.failed = append(s.failed, arg)
	if s.failedReturn.ID != uuid.Nil {
		return s.failedReturn, nil
	}
	return db.NotificationDelivery{}, nil
}

func (s *fakeWorkerStore) ListEnabledNotificationSubscriptionsForEvent(ctx context.Context, arg db.ListEnabledNotificationSubscriptionsForEventParams) ([]db.NotificationSubscription, error) {
	return s.subs, nil
}

func (s *fakeWorkerStore) CreateNotificationDelivery(ctx context.Context, arg db.CreateNotificationDeliveryParams) (db.NotificationDelivery, error) {
	s.created = append(s.created, arg)
	return db.NotificationDelivery{}, nil
}

type fakeSender struct {
	err  error
	sent []sentMessage
}

type sentMessage struct {
	kind string
	url  string
	body json.RawMessage
}

func (s *fakeSender) Send(ctx context.Context, kind, webhookURL string, payload json.RawMessage) error {
	s.sent = append(s.sent, sentMessage{kind: kind, url: webhookURL, body: payload})
	return s.err
}

func TestWorkerSendsPendingDeliveryAndMarksSent(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"
	rawURL := "https://hooks.slack.com/services/T000/B000/secret-token"
	cfg, err := PrepareWebhookConfig(KindSlackWebhook, rawURL, secret)
	if err != nil {
		t.Fatal(err)
	}
	deliveryID := uuid.New()
	store := &fakeWorkerStore{rows: []db.ListPendingNotificationDeliveriesRow{{
		ID:            deliveryID,
		ChannelKind:   KindSlackWebhook,
		ChannelConfig: cfg.JSON(),
		Payload:       json.RawMessage(`{"text":"hello"}`),
	}}}
	sender := &fakeSender{}

	processed, err := Worker{Store: store, Sender: sender, Secret: secret, Limit: 10}.ProcessOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if len(sender.sent) != 1 || sender.sent[0].url != rawURL || sender.sent[0].kind != KindSlackWebhook {
		t.Fatalf("sender calls = %+v", sender.sent)
	}
	if len(store.sent) != 1 || store.sent[0] != deliveryID {
		t.Fatalf("sent marks = %+v", store.sent)
	}
	if len(store.failed) != 0 {
		t.Fatalf("unexpected failed marks = %+v", store.failed)
	}
}

func TestWorkerMarksFailedWhenSenderFails(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"
	cfg, err := PrepareWebhookConfig(KindDiscordWebhook, "https://discord.com/api/webhooks/1/token", secret)
	if err != nil {
		t.Fatal(err)
	}
	deliveryID := uuid.New()
	store := &fakeWorkerStore{rows: []db.ListPendingNotificationDeliveriesRow{{
		ID:            deliveryID,
		ChannelKind:   KindDiscordWebhook,
		ChannelConfig: cfg.JSON(),
		Payload:       json.RawMessage(`{"content":"hello"}`),
	}}}
	sender := &fakeSender{err: errors.New("webhook 500")}

	processed, err := Worker{Store: store, Sender: sender, Secret: secret, Limit: 10}.ProcessOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if len(store.sent) != 0 {
		t.Fatalf("unexpected sent marks = %+v", store.sent)
	}
	if len(store.failed) != 1 || store.failed[0].ID != deliveryID {
		t.Fatalf("failed marks = %+v", store.failed)
	}
	if store.failed[0].LastError == nil || *store.failed[0].LastError != "webhook 500" {
		t.Fatalf("last error = %+v", store.failed[0].LastError)
	}
}

func TestWorkerDispatchesDeliveryDeadEventOnFourthFailure(t *testing.T) {
	projectID := uuid.New()
	deliveryID := uuid.New()
	channelID := uuid.New()
	deadChannelID := uuid.New()
	lastErr := "webhook 500"
	store := &fakeWorkerStore{
		rows: []db.ListPendingNotificationDeliveriesRow{{
			ID:          deliveryID,
			ProjectID:   projectID,
			ChannelID:   channelID,
			EventType:   "publish.failed",
			ChannelKind: KindSlackWebhook,
			Payload:     json.RawMessage(`{"text":"hello"}`),
		}},
		subs: []db.NotificationSubscription{{
			ID:        uuid.New(),
			ProjectID: projectID,
			EventType: "webhook.delivery.dead",
			ChannelID: deadChannelID,
			Enabled:   true,
		}},
		failedReturn: db.NotificationDelivery{
			ID:        deliveryID,
			ProjectID: projectID,
			ChannelID: channelID,
			EventType: "publish.failed",
			Status:    "dead",
			LastError: &lastErr,
		},
	}
	sender := &fakeSender{err: errors.New(lastErr)}

	processed, err := Worker{Store: store, Sender: sender, Secret: "bad-secret", Limit: 10}.ProcessOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d, want 1", processed)
	}
	if len(store.created) != 1 {
		t.Fatalf("created deliveries = %d, want 1", len(store.created))
	}
	got := store.created[0]
	if got.EventType != "webhook.delivery.dead" {
		t.Fatalf("event type = %q", got.EventType)
	}
	if got.EventID != "webhook.delivery.dead:"+deliveryID.String() {
		t.Fatalf("event id = %q", got.EventID)
	}
	if got.ChannelID != deadChannelID {
		t.Fatalf("channel id = %s, want %s", got.ChannelID, deadChannelID)
	}
}

func TestWorkerDoesNotRecurseDeliveryDeadEvents(t *testing.T) {
	projectID := uuid.New()
	deliveryID := uuid.New()
	channelID := uuid.New()
	lastErr := "webhook 500"
	store := &fakeWorkerStore{
		rows: []db.ListPendingNotificationDeliveriesRow{{
			ID:          deliveryID,
			ProjectID:   projectID,
			ChannelID:   channelID,
			EventType:   "webhook.delivery.dead",
			ChannelKind: KindSlackWebhook,
			Payload:     json.RawMessage(`{"text":"hello"}`),
		}},
		subs: []db.NotificationSubscription{{
			ID:        uuid.New(),
			ProjectID: projectID,
			EventType: "webhook.delivery.dead",
			ChannelID: uuid.New(),
			Enabled:   true,
		}},
		failedReturn: db.NotificationDelivery{
			ID:        deliveryID,
			ProjectID: projectID,
			ChannelID: channelID,
			EventType: "webhook.delivery.dead",
			Status:    "dead",
			LastError: &lastErr,
		},
	}
	sender := &fakeSender{err: errors.New(lastErr)}

	if _, err := (Worker{Store: store, Sender: sender, Secret: "bad-secret", Limit: 10}).ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.created) != 0 {
		t.Fatalf("unexpected recursive dead event deliveries = %+v", store.created)
	}
}
