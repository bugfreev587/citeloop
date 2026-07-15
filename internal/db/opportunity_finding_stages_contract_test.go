package db

import (
	"os"
	"strings"
	"testing"
)

func TestOpportunityFindingStageMigrationDefinesDurableFencedCheckpoints(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0079_opportunity_finding_stages.sql")
	if err != nil {
		t.Fatalf("read opportunity finding stage migration: %v", err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"opportunity_finding_stage_checkpoints",
		"workflow_event_id",
		"request_fingerprint",
		"owner_token",
		"attempt_number",
		"evidence_refresh",
		"deterministic_signals",
		"ai_hypotheses",
		"arbitration",
		"materialization",
		"summary",
		"unique (workflow_event_id, stage)",
		"references workflow_events(id)",
		"workflow event project mismatch",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("opportunity finding checkpoint migration missing %q", want)
		}
	}
	if strings.Contains(sql, "create index") && !strings.Contains(sql, "create index concurrently") {
		t.Fatal("checkpoint indexes on populated tables must not take a blocking build path")
	}
}

func TestOpportunityFindingStageQueriesFenceCompletionAndExposeProgress(t *testing.T) {
	raw, err := os.ReadFile("queries/opportunity_finding.sql")
	if err != nil {
		t.Fatalf("read opportunity finding queries: %v", err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"-- name: acquireopportunityfindingstage",
		"-- name: finishopportunityfindingstage",
		"-- name: listopportunityfindingstages",
		"update workflow_events",
		"locked_at = now()",
		"owner_token = sqlc.arg(owner_token)",
		"request_fingerprint",
		"attempt_number",
		"status = 'running'",
		"sqlc.arg(status)::text in ('succeeded','partial','failed','skipped')",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("opportunity finding checkpoint queries missing %q", want)
		}
	}
}
