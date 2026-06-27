package contextmeta

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	KeyConfirmedAt        = "confirmed_at"
	KeyContextConfirmedAt = "context_confirmed_at"
	KeyLastCrawledAt      = "context_last_crawled_at"
	KeyLastManualCrawled  = "context_last_manual_crawled_at"
	KeyCrawlStartedAt     = "context_crawl_started_at"
	KeyCrawlSource        = "context_crawl_source"

	SourceManual = "manual"
	SourceWeekly = "weekly"

	ManualCooldown = 24 * time.Hour
	WeeklyInterval = 7 * 24 * time.Hour
)

func HasConfirmation(raw json.RawMessage) bool {
	for _, key := range []string{KeyContextConfirmedAt, KeyConfirmedAt} {
		if value, ok := StringField(raw, key); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func HasActiveCrawl(raw json.RawMessage) bool {
	value, ok := StringField(raw, KeyCrawlStartedAt)
	return ok && strings.TrimSpace(value) != ""
}

func ManualCooldownActive(raw json.RawMessage, now time.Time) bool {
	last, ok := TimeField(raw, KeyLastManualCrawled)
	return ok && now.Sub(last) < ManualCooldown
}

func WeeklyRefreshDue(raw json.RawMessage, now time.Time) bool {
	last, ok := TimeField(raw, KeyLastCrawledAt)
	return !ok || now.Sub(last) >= WeeklyInterval
}

func StartedProfile(raw json.RawMessage, source string, now time.Time) json.RawMessage {
	profile := objectFromRaw(raw)
	startedAt := now.UTC().Format(time.RFC3339)
	profile[KeyCrawlStartedAt] = startedAt
	profile[KeyCrawlSource] = source
	if source == SourceManual {
		profile[KeyLastManualCrawled] = startedAt
	}
	return mustJSON(profile)
}

func ClearStartedProfile(raw json.RawMessage) json.RawMessage {
	profile := objectFromRaw(raw)
	delete(profile, KeyCrawlStartedAt)
	return mustJSON(profile)
}

func CompletedProfile(profile any, previous json.RawMessage, now time.Time) json.RawMessage {
	next := objectFromAny(profile)
	prev := objectFromRaw(previous)

	for _, key := range []string{KeyContextConfirmedAt, KeyConfirmedAt, KeyLastManualCrawled} {
		if _, exists := next[key]; exists {
			continue
		}
		if value, exists := prev[key]; exists {
			next[key] = value
		}
	}

	completedAt := now.UTC().Format(time.RFC3339)
	next[KeyLastCrawledAt] = completedAt
	if source, _ := prev[KeyCrawlSource].(string); strings.TrimSpace(source) == SourceManual {
		next[KeyLastManualCrawled] = completedAt
	}
	if source, _ := prev[KeyCrawlSource].(string); strings.TrimSpace(source) != "" {
		next[KeyCrawlSource] = strings.TrimSpace(source)
	}
	delete(next, KeyCrawlStartedAt)
	return mustJSON(next)
}

func StringField(raw json.RawMessage, key string) (string, bool) {
	profile := objectFromRaw(raw)
	value, ok := profile[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func TimeField(raw json.RawMessage, key string) (time.Time, bool) {
	value, ok := StringField(raw, key)
	if !ok {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func objectFromRaw(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func objectFromAny(value any) map[string]any {
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	return objectFromRaw(raw)
}

func mustJSON(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}
