package db

import (
	"os"
	"strings"
	"testing"
)

func TestManualGrowthPlannerPromptSourceMigration(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0096_geo_prompt_ai_growth_source.sql")
	if err != nil {
		t.Fatalf("read GEO prompt source migration: %v", err)
	}
	migration := strings.ToLower(string(raw))
	for _, required := range []string{
		"drop constraint if exists geo_prompts_source_check",
		"add constraint geo_prompts_source_check",
		"'profile'",
		"'topic'",
		"'competitor'",
		"'manual'",
		"'search_result'",
		"'ai_growth_planner'",
	} {
		if !strings.Contains(migration, required) {
			t.Errorf("GEO prompt source migration missing %q", required)
		}
	}
}
