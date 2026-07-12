package db

import (
	"os"
	"strings"
	"testing"
)

func TestCanonicalAICallLedgerMigrationTracksAttemptsAndProviderInvocation(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0078_ai_call_records.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"alter table ai_call_records",
		"attempt_number",
		"provider_called",
		"provider_started_at",
		"parent_call_id",
		"request_fingerprint",
		"status = 'skipped'",
		"provider_called = false",
		"provider_started_at is null",
		"nonzero terminal accounting is immutable",
		"late_usage_fill",
		"new.error_code in ('invalid_response', 'invalid_output')",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("canonical AI call migration missing %q", want)
		}
	}
}

func TestMonthlySpendCombinesCanonicalAndRollingLegacySpend(t *testing.T) {
	raw, err := os.ReadFile("queries/runs.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"from ai_call_records call",
		"from generation_runs run",
		"call.status not in ('queued', 'skipped')",
		"run.created_at >= date_trunc('month', now())",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("monthly spend must preserve canonical and rolling legacy cost: missing %q", want)
		}
	}
	if strings.Contains(sql, "ai_call_ledger_cutovers") {
		t.Fatal("cutover snapshots miss spend written by old instances after migration")
	}
}

func TestCanonicalAICallQueriesSupportRetriesSkippedCallsAndRecomputableAggregates(t *testing.T) {
	raw, err := os.ReadFile("queries/ai_calls.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"-- name: createaicallrecord :one",
		"-- name: createskippedaicallrecord :one",
		"provider_called",
		"provider_started_at",
		"parent_call_id",
		"attempt_number",
		"-- name: finishaicallrecord :one",
		"-- name: markaicallproviderstarted :one",
		"and status = 'queued'",
		"-- name: finishqueuedaicallskipped :one",
		"where id = sqlc.arg(id)",
		"status = 'running'",
		"-- name: reclassifyaicallrecordoutputfailure :one",
		"-- name: aggregateaicallsforobject :one",
		"sum(total_tokens)",
		"sum(cost_usd)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("canonical AI call queries missing %q", want)
		}
	}
}
