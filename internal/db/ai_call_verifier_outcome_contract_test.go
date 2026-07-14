package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAICallVerifierOutcomeMigrationIsNullableBoundedAndValidatedSeparately(t *testing.T) {
	addRaw, err := os.ReadFile(filepath.Join("..", "migrations", "0085_ai_call_verifier_outcome_add.sql"))
	if err != nil {
		t.Fatalf("read verifier outcome add migration: %v", err)
	}
	validateRaw, err := os.ReadFile(filepath.Join("..", "migrations", "0086_ai_call_verifier_outcome_validate.sql"))
	if err != nil {
		t.Fatalf("read verifier outcome validation migration: %v", err)
	}

	add := strings.ToLower(string(addRaw))
	for _, want := range []string{
		"set local lock_timeout = '5s'",
		"set local statement_timeout = '30s'",
		"alter table ai_call_records",
		"add column if not exists verifier_outcome jsonb",
		"ai_call_records_verifier_outcome_check",
		"verifier_outcome is null",
		"stage = 'fix_grounding_verification'",
		"jsonb_typeof(verifier_outcome) = 'object'",
		"not valid",
		"reset statement_timeout",
		"reset lock_timeout",
	} {
		if !strings.Contains(add, want) {
			t.Errorf("verifier outcome add migration missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"not null",
		"default",
		"update ai_call_records",
		"validate constraint",
	} {
		if strings.Contains(add, forbidden) {
			t.Errorf("verifier outcome add migration must not contain %q", forbidden)
		}
	}

	validate := strings.ToLower(string(validateRaw))
	for _, want := range []string{
		"set local lock_timeout = '5s'",
		"set local statement_timeout = '30s'",
		"alter table ai_call_records",
		"validate constraint ai_call_records_verifier_outcome_check",
		"reset statement_timeout",
		"reset lock_timeout",
	} {
		if !strings.Contains(validate, want) {
			t.Errorf("verifier outcome validation migration missing %q", want)
		}
	}
	for _, forbidden := range []string{"add column", "add constraint", "drop constraint", "update ai_call_records"} {
		if strings.Contains(validate, forbidden) {
			t.Errorf("verifier outcome validation migration must not contain %q", forbidden)
		}
	}
}

func TestAICallVerifierOutcomeIsWrittenAtomicallyByFencedFinish(t *testing.T) {
	raw, err := os.ReadFile("queries/ai_calls.sql")
	if err != nil {
		t.Fatal(err)
	}
	all := strings.ToLower(string(raw))
	query := namedSQL(t, all, "FinishCanonicalAICallFenced")
	requireQuerySQL(t, query,
		"update ai_call_records set",
		"status = case",
		"prompt_tokens = case",
		"cost_usd = case",
		"verifier_outcome = case",
		"when stage = 'fix_grounding_verification'",
		"then coalesce(verifier_outcome, sqlc.narg(verifier_outcome))",
		"else verifier_outcome end",
	)
	if count := strings.Count(all, "verifier_outcome ="); count != 1 {
		t.Fatalf("verifier outcome assignments = %d, want only the stage-guarded fenced finish", count)
	}
}
