package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type schedulerDispatchStore struct {
	subs    []db.NotificationSubscription
	created []db.CreateNotificationDeliveryParams
}

func (s *schedulerDispatchStore) ListEnabledNotificationSubscriptionsForEvent(ctx context.Context, arg db.ListEnabledNotificationSubscriptionsForEventParams) ([]db.NotificationSubscription, error) {
	return s.subs, nil
}

func (s *schedulerDispatchStore) CreateNotificationDelivery(ctx context.Context, arg db.CreateNotificationDeliveryParams) (db.NotificationDelivery, error) {
	s.created = append(s.created, arg)
	return db.NotificationDelivery{}, nil
}

func TestSchedulerDispatchesBudgetStoppedEvent(t *testing.T) {
	projectID := uuid.New()
	store := &schedulerDispatchStore{subs: []db.NotificationSubscription{{
		ID:        uuid.New(),
		ProjectID: projectID,
		EventType: "budget.stopped",
		ChannelID: uuid.New(),
		Enabled:   true,
	}}}
	s := &Scheduler{now: func() time.Time { return time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC) }}

	s.dispatchBudgetStopped(context.Background(), store, projectID, 51, 50)

	if len(store.created) != 1 {
		t.Fatalf("created deliveries = %d, want 1", len(store.created))
	}
	if store.created[0].EventType != "budget.stopped" {
		t.Fatalf("event type = %q", store.created[0].EventType)
	}
}

func TestSchedulerDispatchesPublishFailedEvent(t *testing.T) {
	projectID := uuid.New()
	articleID := uuid.New()
	store := &schedulerDispatchStore{subs: []db.NotificationSubscription{{
		ID:        uuid.New(),
		ProjectID: projectID,
		EventType: "publish.failed",
		ChannelID: uuid.New(),
		Enabled:   true,
	}}}
	seo := json.RawMessage(`{"title":"Article title","slug":"article-slug"}`)
	article := db.Article{ID: articleID, ProjectID: projectID, SeoMeta: seo, PublishAttempts: 2}

	s := &Scheduler{now: func() time.Time { return time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC) }}
	s.dispatchPublishFailed(context.Background(), store, db.Project{ID: projectID}, article, "github_write", "commit failed", true)

	if len(store.created) != 1 {
		t.Fatalf("created deliveries = %d, want 1", len(store.created))
	}
	got := store.created[0]
	if got.EventID != "publish.failed:"+articleID.String()+":github_write:transition:2" {
		t.Fatalf("event id = %q", got.EventID)
	}
	if got.EventType != "publish.failed" {
		t.Fatalf("event type = %q", got.EventType)
	}
}

func TestSchedulerDispatchesGenerationFailedEvent(t *testing.T) {
	projectID := uuid.New()
	topicID := uuid.New()
	store := &schedulerDispatchStore{subs: []db.NotificationSubscription{{
		ID:        uuid.New(),
		ProjectID: projectID,
		EventType: "generation.failed",
		ChannelID: uuid.New(),
		Enabled:   true,
	}}}
	now := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	s := &Scheduler{now: func() time.Time { return now }}

	s.dispatchGenerationFailed(context.Background(), store, projectID, "writer", topicID.String(), "Topic title", "writer failed")

	if len(store.created) != 1 {
		t.Fatalf("created deliveries = %d, want 1", len(store.created))
	}
	got := store.created[0]
	if got.EventID != "generation.failed:"+projectID.String()+":writer:"+topicID.String()+":2026-06-05" {
		t.Fatalf("event id = %q", got.EventID)
	}
	if got.EventType != "generation.failed" {
		t.Fatalf("event type = %q", got.EventType)
	}
}

func TestSchedulerDispatchesReviewOverdueEvent(t *testing.T) {
	projectID := uuid.New()
	articleID := uuid.New()
	store := &schedulerDispatchStore{subs: []db.NotificationSubscription{{
		ID:        uuid.New(),
		ProjectID: projectID,
		EventType: "review.overdue",
		ChannelID: uuid.New(),
		Enabled:   true,
	}}}
	createdAt := time.Date(2026, 6, 2, 8, 0, 0, 0, time.UTC)
	article := db.Article{
		ID:        articleID,
		ProjectID: projectID,
		SeoMeta:   json.RawMessage(`{"title":"Needs eyes"}`),
		CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
	}
	now := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	s := &Scheduler{now: func() time.Time { return now }}

	s.dispatchReviewOverdue(context.Background(), store, article)

	if len(store.created) != 1 {
		t.Fatalf("created deliveries = %d, want 1", len(store.created))
	}
	got := store.created[0]
	if got.EventID != "review.overdue:"+articleID.String()+":2026-06-05" {
		t.Fatalf("event id = %q", got.EventID)
	}
	if got.EventType != "review.overdue" {
		t.Fatalf("event type = %q", got.EventType)
	}
}
