package db

import (
	"os"
	"strings"
	"testing"
)

func TestStaleParentHandoffCleanupMigrationResetsImpossibleParentLinks(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0100_cleanup_stale_parent_handoff_links.sql")
	if err != nil {
		t.Fatalf("read stale parent handoff cleanup migration: %v", err)
	}
	sql := strings.ToLower(string(raw))

	for _, want := range []string{
		"create or replace function bind_immutable_growth_measurement_policy()",
		"new.status_reason like 'reset stale parent handoff link%'",
		"delete from action_measurements",
		"update content_actions ca",
		"status = 'ready_for_review'",
		"published_at = null",
		"verified_at = null",
		"verification_snapshot = '{}'::jsonb",
		"outcome_summary = '{}'::jsonb",
		"measuring_started_at = null",
		"absolute_terminal_at = null",
		"measurement_terminal_reason = null",
		"measurement_policy_version = 'legacy-v0'",
		"measurement_policy = '{}'::jsonb",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("cleanup migration missing %q", want)
		}
	}

	if strings.Contains(sql, "disable trigger") || strings.Contains(sql, "enable trigger") {
		t.Fatal("cleanup migration must not disable table triggers inside a transactional migration")
	}

	for _, want := range []string{
		"ca.status in ('published','measuring','completed','failed','verification_failed','recovery_required')",
		"article.status in (",
		"'pending_review'",
		"'approved'",
		"article.published_at is null",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("cleanup migration must narrowly target stale unpublished parents; missing %q", want)
		}
	}

	if strings.Contains(sql, "article.status in (\n      'published'") ||
		strings.Contains(sql, "article.status in (\n      'generating',\n      'published'") ||
		strings.Contains(sql, "article.status in (\n      'pending_review',\n      'published'") ||
		strings.Contains(sql, "article.status in (\n      'approved',\n      'published'") {
		t.Fatal("cleanup migration must not reset parent actions whose child article is already published")
	}
}
