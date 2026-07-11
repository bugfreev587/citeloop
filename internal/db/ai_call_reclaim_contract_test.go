package db

import (
	"os"
	"strings"
	"testing"
)

func TestAICallReclaimOnlyFinishesRunningAndPreservesUsage(t *testing.T) {
	raw, err := os.ReadFile("queries/discovery.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := namedSQL(t, strings.ToLower(string(raw)), "FinishAICallRecordIfRunning")
	requireQuerySQL(t, query, "status = 'failed'", "and status = 'running'")
	for _, forbidden := range []string{"prompt_tokens =", "completion_tokens =", "total_tokens =", "cost_usd =", "provider =", "model ="} {
		if strings.Contains(query, forbidden) {
			t.Errorf("running-only reclaim must preserve ledger field %q", forbidden)
		}
	}
}

func TestFinishCanonicalAICallFencedPreservesCleanupTerminalStateAndLateUsage(t *testing.T) {
	raw, err := os.ReadFile("queries/discovery.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := namedSQL(t, strings.ToLower(string(raw)), "FinishCanonicalAICallFenced")
	for _, required := range []string{
		"case when status = 'running' then sqlc.arg(status)",
		"case when status = 'running' then sqlc.narg(error_code)",
		"provider = coalesce(sqlc.narg(resolved_provider), provider)",
		"model = coalesce(sqlc.narg(resolved_model), model)",
		"prompt_tokens = sqlc.arg(prompt_tokens)",
		"completion_tokens = sqlc.arg(completion_tokens)",
		"total_tokens = sqlc.arg(total_tokens)",
		"cost_usd = sqlc.arg(cost_usd)",
	} {
		if !strings.Contains(query, required) {
			t.Errorf("fenced canonical finish missing %q", required)
		}
	}
}
