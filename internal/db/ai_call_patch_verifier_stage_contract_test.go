package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndependentPatchGroundingVerifierHasDistinctCanonicalAICallStage(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0064_fix_grounding_verification_stage.sql"))
	if err != nil {
		t.Fatalf("read verifier stage migration: %v", err)
	}
	migration := strings.ToLower(string(raw))
	for _, want := range []string{
		"alter table ai_call_records",
		"ai_call_records_stage_check",
		"'fix_grounding_verification'",
		"validate constraint ai_call_records_stage_check",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("verifier stage migration missing %q", want)
		}
	}
}
