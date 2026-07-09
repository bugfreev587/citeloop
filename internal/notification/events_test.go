package notification

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBudgetStoppedEventHasStableIDAndPayload(t *testing.T) {
	projectID := uuid.New()
	event := NewBudgetStoppedEvent(projectID, 51.25, 50, time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC), "https://app.test/projects/1")

	if event.Type != "budget.stopped" {
		t.Fatalf("type = %q", event.Type)
	}
	if event.ID != BudgetStoppedEventID(projectID.String(), "2026-06", "50.00") {
		t.Fatalf("event id = %q", event.ID)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["message"] == "" || payload["dashboard_url"] != "https://app.test/projects/1" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestSiteFixPRAwaitingMergeEventBucketsByTwelveHours(t *testing.T) {
	projectID := uuid.New()
	appID := uuid.New()
	morning := time.Date(2026, 6, 5, 9, 30, 0, 0, time.UTC)
	sameWindow := time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC)
	nextWindow := time.Date(2026, 6, 5, 12, 30, 0, 0, time.UTC)

	a := NewSiteFixPRAwaitingMergeEvent(projectID, appID, "https://github.com/o/r/pull/9", 9, 26, morning, "https://app.test/projects/1/seo")
	b := NewSiteFixPRAwaitingMergeEvent(projectID, appID, "https://github.com/o/r/pull/9", 9, 27, sameWindow, "https://app.test/projects/1/seo")
	c := NewSiteFixPRAwaitingMergeEvent(projectID, appID, "https://github.com/o/r/pull/9", 9, 28, nextWindow, "https://app.test/projects/1/seo")

	if a.Type != "sitefix.pr.awaiting_merge" {
		t.Fatalf("type = %q", a.Type)
	}
	if a.ID != b.ID {
		t.Fatalf("same 12h window should share an ID: %q vs %q", a.ID, b.ID)
	}
	if a.ID == c.ID {
		t.Fatalf("different 12h window should differ: %q", a.ID)
	}
	var payload map[string]any
	if err := json.Unmarshal(a.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload["message"].(string), "#9") || payload["dashboard_url"] != "https://app.test/projects/1/seo" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestPublishFailedEventUsesTransitionAndDailyIDs(t *testing.T) {
	projectID := uuid.New()
	articleID := uuid.New()
	now := time.Date(2026, 6, 5, 9, 30, 0, 0, time.UTC)
	transition := NewPublishFailedEvent(projectID, articleID, "Article title", "my-slug", "github_write", 3, "commit failed", now, "https://app.test", true)

	if transition.Type != "publish.failed" {
		t.Fatalf("type = %q", transition.Type)
	}
	if transition.ID != "publish.failed:"+articleID.String()+":github_write:transition:3" {
		t.Fatalf("transition event id = %q", transition.ID)
	}
	var payload map[string]any
	if err := json.Unmarshal(transition.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["title"] != "Article title" || payload["slug"] != "my-slug" || payload["phase"] != "github_write" || payload["error"] != "commit failed" {
		t.Fatalf("payload = %#v", payload)
	}

	daily := NewPublishFailedEvent(projectID, articleID, "Article title", "my-slug", "github_write", 3, "commit failed", now, "https://app.test", false)
	if wantPrefix := "publish.failed:" + articleID.String() + ":github_write:daily:2026-06-05:"; !strings.HasPrefix(daily.ID, wantPrefix) {
		t.Fatalf("daily event id = %q, want prefix %q", daily.ID, wantPrefix)
	}
	if daily.ID == transition.ID {
		t.Fatalf("daily and transition ids must differ: %q", daily.ID)
	}
}

func TestGenerationFailedEventIsDedupedByScopeAndDay(t *testing.T) {
	projectID := uuid.New()
	now := time.Date(2026, 6, 5, 9, 30, 0, 0, time.UTC)
	event := NewGenerationFailedEvent(projectID, "writer", "topic-1", "Draft title", "model failed", now, "https://app.test/projects/1/runs")

	if event.Type != "generation.failed" {
		t.Fatalf("type = %q", event.Type)
	}
	if event.ID != "generation.failed:"+projectID.String()+":writer:topic-1:2026-06-05" {
		t.Fatalf("event id = %q", event.ID)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["agent"] != "writer" || payload["scope"] != "topic-1" || payload["error"] != "model failed" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestReviewOverdueEventIsDedupedByArticleAndDay(t *testing.T) {
	projectID := uuid.New()
	articleID := uuid.New()
	now := time.Date(2026, 6, 5, 9, 30, 0, 0, time.UTC)
	event := NewReviewOverdueEvent(projectID, articleID, "Draft title", 73, now, "https://app.test/projects/1/review")

	if event.Type != "review.overdue" {
		t.Fatalf("type = %q", event.Type)
	}
	if event.ID != "review.overdue:"+articleID.String()+":2026-06-05" {
		t.Fatalf("event id = %q", event.ID)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["title"] != "Draft title" || payload["dashboard_url"] != "https://app.test/projects/1/review" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload["age_hours"] != float64(73) {
		t.Fatalf("age_hours = %#v", payload["age_hours"])
	}
}

func TestWebhookDeliveryDeadEventReferencesOriginalDelivery(t *testing.T) {
	projectID := uuid.New()
	deliveryID := uuid.New()
	channelID := uuid.New()
	event := NewWebhookDeliveryDeadEvent(projectID, deliveryID, channelID, "publish.failed", "webhook 500", "https://app.test/settings")

	if event.Type != "webhook.delivery.dead" {
		t.Fatalf("type = %q", event.Type)
	}
	if event.ID != "webhook.delivery.dead:"+deliveryID.String() {
		t.Fatalf("event id = %q", event.ID)
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["delivery_id"] != deliveryID.String() || payload["channel_id"] != channelID.String() {
		t.Fatalf("payload = %#v", payload)
	}
	if payload["event_type"] != "publish.failed" || payload["last_error"] != "webhook 500" {
		t.Fatalf("payload = %#v", payload)
	}
}
