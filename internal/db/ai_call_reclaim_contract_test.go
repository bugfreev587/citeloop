package db

import (
	"os"
	"strings"
	"testing"
)

func TestAICallReclaimOnlyFinishesRunningAndPreservesUsage(t *testing.T) {
	raw, err := os.ReadFile("queries/ai_calls.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := namedSQL(t, strings.ToLower(string(raw)), "FinishAICallRecordIfRunning")
	requireQuerySQL(t, query, "status = 'failed'", "and status = 'running'")
	for _, forbidden := range []string{"prompt_tokens =", "completion_tokens =", "total_tokens =", "cost_usd =", "provider =", "model =", "verifier_outcome ="} {
		if strings.Contains(query, forbidden) {
			t.Errorf("running-only reclaim must preserve ledger field %q", forbidden)
		}
	}
}

func TestFinishCanonicalAICallFencedPreservesCleanupTerminalStateAndLateUsage(t *testing.T) {
	raw, err := os.ReadFile("queries/ai_calls.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := namedSQL(t, strings.ToLower(string(raw)), "FinishCanonicalAICallFenced")
	for _, required := range []string{
		"status = case when status = 'running' or (status = 'queued'",
		"error_code = case when status = 'running' or (status = 'queued'",
		"provider = coalesce(sqlc.narg(resolved_provider), provider)",
		"model = coalesce(sqlc.narg(resolved_model), model)",
		"else sqlc.arg(prompt_tokens) end",
		"else sqlc.arg(completion_tokens) end",
		"else sqlc.arg(total_tokens) end",
		"else sqlc.arg(cost_usd)::numeric end",
		"verifier_outcome = case when stage = 'fix_grounding_verification'",
		"then coalesce(verifier_outcome, sqlc.narg(verifier_outcome))",
		"else verifier_outcome end",
	} {
		if !strings.Contains(query, required) {
			t.Errorf("fenced canonical finish missing %q", required)
		}
	}
}
