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

func TestSiteChangeApplicationsSchemaDefinesSharedApplyLedger(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0041_site_change_applications.sql"))
	if err != nil {
		t.Fatalf("read site change applications migration: %v", err)
	}
	migration := strings.ToLower(string(raw))
	for _, want := range []string{
		"create table if not exists site_change_applications",
		"source_opportunity_id uuid references seo_opportunities(id) on delete set null",
		"content_action_id uuid not null references content_actions(id) on delete cascade",
		"page_update_draft_id uuid references page_update_drafts(id) on delete cascade",
		"application_kind text not null check (application_kind in ('page_update', 'site_fix'))",
		"opportunity_key text not null",
		"publisher_connection_id uuid references publisher_connections(id) on delete set null",
		"source_mapping_confidence text not null default 'low'",
		"base_file_sha text",
		"github_pr_url text",
		"status text not null default 'draft_ready'",
		"'github_pr_open'",
		"'github_pr_closed'",
		"'needs_follow_up'",
		"on site_change_applications (project_id, opportunity_key)",
		"where status in (",
		"'github_pr_closed'",
		"'needs_follow_up'",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("site change applications migration missing %q", want)
		}
	}
}

func TestSiteChangeApplicationQueriesExposeApplyLifecycle(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("queries", "seo.sql"))
	if err != nil {
		t.Fatalf("read seo queries: %v", err)
	}
	queries := strings.ToLower(string(raw))
	for _, want := range []string{
		"-- name: createorreusesitechangeapplication :one",
		"-- name: getsitechangeapplicationforproject :one",
		"-- name: getactivesitechangeapplicationbyopportunitykey :one",
		"-- name: marksitechangeapplicationgithubpr :one",
		"-- name: marksitechangeapplicationstatus :one",
		"on conflict (project_id, opportunity_key)",
		"where status in (",
		"'github_pr_closed'",
		"'needs_follow_up'",
		"when site_change_applications.status in ('github_pr_open','github_pr_merged','deployment_pending','verification_pending') then site_change_applications.working_branch",
	} {
		if !strings.Contains(queries, want) {
			t.Fatalf("site change application queries missing %q", want)
		}
	}
}
