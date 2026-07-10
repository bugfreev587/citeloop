package db

import (
	"os"
	"strings"
	"testing"
)

func TestNotificationSchemaContracts(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0005_notifications.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)

	for _, want := range []string{
		"create table if not exists notification_channels",
		"create table if not exists notification_subscriptions",
		"create table if not exists notification_deliveries",
		"notification_channels_legacy_",
		"notification_subscriptions_legacy_",
		"notification_deliveries_legacy_",
		"owner_id",
		"kind text not null check (kind in ('slack_webhook','discord_webhook','email'))",
		"config jsonb not null",
		"unique (project_id, event_type, channel_id)",
		"unique (event_id, channel_id)",
		"status text not null default 'pending' check (status in ('pending','sent','dead'))",
		"idx_notification_channels_owner",
		"notification_subscription_owner_guard",
		"projects.owner_id",
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("notification migration missing %q", want)
		}
	}
}

func TestNotificationQueriesExposeFoundation(t *testing.T) {
	required := []string{
		createNotificationChannel,
		listNotificationChannels,
		getNotificationChannel,
		markNotificationChannelVerified,
		softDeleteNotificationChannel,
		upsertNotificationSubscription,
		listEnabledNotificationSubscriptionsForEvent,
		createNotificationDelivery,
		listPendingNotificationDeliveries,
		markNotificationDeliverySent,
		markNotificationDeliveryFailed,
		retryNotificationDelivery,
		listNotificationDeliveries,
	}
	for i, query := range required {
		if query == "" {
			t.Fatalf("notification query %d is empty", i)
		}
	}
	if !strings.Contains(createNotificationDelivery, "on conflict (event_id, channel_id) do nothing") {
		t.Fatal("CreateNotificationDelivery must be idempotent by event_id/channel_id")
	}
	if !strings.Contains(listEnabledNotificationSubscriptionsForEvent, "enabled = true") {
		t.Fatal("ListEnabledNotificationSubscriptionsForEvent must only return enabled subscriptions")
	}
	if !strings.Contains(listNotificationChannels, "p.owner_id") {
		t.Fatal("ListNotificationChannels must be account-owner scoped")
	}
	if !strings.Contains(listNotificationChannels, "project_subscription_count") {
		t.Fatal("ListNotificationChannels must expose account-wide channel usage counts")
	}
	if !strings.Contains(getNotificationChannel, "p.owner_id") {
		t.Fatal("GetNotificationChannel must find channels by project owner, not channel project")
	}
	if !strings.Contains(upsertNotificationSubscription, "verified_at is not null") {
		t.Fatal("UpsertNotificationSubscription must require accepted Email channels before enabling")
	}
	if !strings.Contains(markNotificationChannelVerified, "verified_at = now()") {
		t.Fatal("MarkNotificationChannelVerified must mark the channel test accepted after a successful test send")
	}
	if !strings.Contains(listPendingNotificationDeliveries, "for update skip locked") {
		t.Fatal("ListPendingNotificationDeliveries must lock claimed rows")
	}
	if !strings.Contains(markNotificationDeliveryFailed, "status = case when attempts + 1 >= 4 then 'dead' else 'pending' end") {
		t.Fatal("MarkNotificationDeliveryFailed must mark the fourth failure dead")
	}
	if !strings.Contains(retryNotificationDelivery, "status = 'pending'") || !strings.Contains(retryNotificationDelivery, "next_retry_at = now()") {
		t.Fatal("RetryNotificationDelivery must put a delivery back into pending state immediately")
	}
}

func TestReviewOverdueQueryContract(t *testing.T) {
	if listOverdueReviewArticles == "" {
		t.Fatal("ListOverdueReviewArticles query is empty")
	}
	if !strings.Contains(listOverdueReviewArticles, "status = 'pending_review'") {
		t.Fatal("ListOverdueReviewArticles must only select pending_review articles")
	}
	if !strings.Contains(listOverdueReviewArticles, "created_at <= $1") {
		t.Fatal("ListOverdueReviewArticles must use an explicit cutoff timestamp")
	}
	if !strings.Contains(listOverdueReviewArticles, "limit $2") {
		t.Fatal("ListOverdueReviewArticles must be bounded")
	}
}
