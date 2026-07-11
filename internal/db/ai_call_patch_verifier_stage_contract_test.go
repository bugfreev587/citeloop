package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndependentPatchGroundingVerifierHasDistinctCanonicalAICallStage(t *testing.T) {
	addRaw, err := os.ReadFile(filepath.Join("..", "migrations", "0065_fix_grounding_verification_stage.sql"))
	if err != nil {
		t.Fatalf("read verifier stage migration: %v", err)
	}
	validateRaw, err := os.ReadFile(filepath.Join("..", "migrations", "0066_validate_fix_grounding_verification_stage.sql"))
	if err != nil {
		t.Fatalf("read verifier stage validation migration: %v", err)
	}
	add := strings.ToLower(string(addRaw))
	validate := strings.ToLower(string(validateRaw))
	for _, want := range []string{
		"set local lock_timeout = '5s'",
		"set local statement_timeout = '30s'",
		"alter table ai_call_records",
		"drop constraint if exists ai_call_records_stage_check",
		"ai_call_records_stage_check",
		"'fix_grounding_verification'",
		"not valid",
		"reset statement_timeout",
		"reset lock_timeout",
	} {
		if !strings.Contains(add, want) {
			t.Errorf("verifier stage add migration missing %q", want)
		}
	}
	if strings.Contains(add, "validate constraint") {
		t.Fatal("verifier stage add migration must not scan the populated ai_call_records table")
	}
	for _, want := range []string{
		"set local lock_timeout = '5s'",
		"set local statement_timeout = '30s'",
		"alter table ai_call_records",
		"validate constraint ai_call_records_stage_check",
		"reset statement_timeout",
		"reset lock_timeout",
	} {
		if !strings.Contains(validate, want) {
			t.Errorf("verifier stage validation migration missing %q", want)
		}
	}
	if strings.Contains(validate, "drop constraint") || strings.Contains(validate, "add constraint") {
		t.Fatal("validation migration must not replace the live stage constraint")
	}
}
