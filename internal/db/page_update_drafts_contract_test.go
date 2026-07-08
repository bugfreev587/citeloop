package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPageUpdateDraftSchemaDefinesLifecycleAndActiveDraftLock(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0039_page_update_drafts.sql"))
	if err != nil {
		t.Fatalf("read page update drafts migration: %v", err)
	}
	migration := strings.ToLower(string(raw))
	for _, want := range []string{
		"create table if not exists page_update_drafts",
		"content_action_id uuid not null references content_actions(id) on delete cascade",
		"normalized_target_url text not null",
		"base_content_hash text",
		"proposed_content_md text not null default ''",
		"patch jsonb not null default '{}'",
		"diff_snapshot jsonb not null default '{}'",
		"qa_feedback jsonb not null default '{}'",
		"resolution_criteria jsonb not null default '{}'",
		"publisher_result jsonb not null default '{}'",
		"verification_snapshot jsonb not null default '{}'",
		"original_source_snapshot jsonb not null default '{}'",
		"status text not null default 'drafting'",
		"'verification_pending'",
		"'manual_apply_required'",
		"'verified'",
		"'needs_follow_up'",
		"'verification_failed'",
		"where status in ('drafting','ready_for_review','approved','applying','verification_pending','manual_apply_required')",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("page update drafts migration missing %q", want)
		}
	}
}

func TestPageUpdateDraftQueriesExposeLifecycleAndConcurrencyContracts(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("queries", "seo.sql"))
	if err != nil {
		t.Fatalf("read seo queries: %v", err)
	}
	queries := strings.ToLower(string(raw))
	for _, want := range []string{
		"-- name: createorreusepageupdatedraft :one",
		"-- name: getpageupdatedraftforproject :one",
		"-- name: updatepageupdatedraftcontent :one",
		"-- name: updatepageupdatedraftstatus :one",
		"-- name: claimpageupdatedraftforapply :one",
		"-- name: markpageupdatedraftverification :one",
		"-- name: liststalepageupdatedrafts :many",
		"on conflict (project_id, content_action_id)",
		"for update",
		"skip locked",
	} {
		if !strings.Contains(queries, want) {
			t.Fatalf("page update draft queries missing %q", want)
		}
	}
}
