package contextmeta

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCompletedProfilePreservesConfirmationAndManualTimestamp(t *testing.T) {
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	previous := json.RawMessage(`{
		"context_confirmed_at":"2026-06-20T00:00:00Z",
		"context_last_manual_crawled_at":"2026-06-25T00:00:00Z",
		"context_crawl_started_at":"2026-06-27T11:59:00Z",
		"context_crawl_source":"manual"
	}`)

	next := CompletedProfile(
		map[string]any{"positioning": "Updated profile"},
		previous,
		now,
	)

	if !HasConfirmation(next) {
		t.Fatalf("completed profile = %s, want preserved confirmation", next)
	}
	if HasActiveCrawl(next) {
		t.Fatalf("completed profile = %s, want active crawl cleared", next)
	}
	if got, ok := StringField(next, "context_last_crawled_at"); !ok || got != "2026-06-27T12:00:00Z" {
		t.Fatalf("context_last_crawled_at = %q ok=%v", got, ok)
	}
	if got, ok := StringField(next, "context_last_manual_crawled_at"); !ok || got != "2026-06-27T12:00:00Z" {
		t.Fatalf("context_last_manual_crawled_at = %q ok=%v", got, ok)
	}
}

func TestManualCooldownUsesLastManualCrawl(t *testing.T) {
	profile := json.RawMessage(`{"context_last_manual_crawled_at":"2026-06-27T10:30:00Z"}`)
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)

	if !ManualCooldownActive(profile, now) {
		t.Fatal("manual cooldown should be active within 24 hours")
	}
	if ManualCooldownActive(profile, now.Add(25*time.Hour)) {
		t.Fatal("manual cooldown should expire after 24 hours")
	}
}

func TestStartedProfileBeginsManualCooldown(t *testing.T) {
	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	started := StartedProfile(json.RawMessage(`{"positioning":"Ready"}`), SourceManual, now)

	if !ManualCooldownActive(started, now.Add(time.Minute)) {
		t.Fatalf("started profile = %s, want manual cooldown immediately active", started)
	}
	if got, ok := StringField(started, KeyLastManualCrawled); !ok || got != "2026-06-27T12:00:00Z" {
		t.Fatalf("%s = %q ok=%v", KeyLastManualCrawled, got, ok)
	}
}
