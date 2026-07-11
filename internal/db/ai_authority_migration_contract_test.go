package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIAuthorityMigrationMapsLegacySettingsWithoutExpansion(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0062_ai_capability_authority.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"growth_signal_enabled",
		"growth_ai_enabled",
		"doctor_ai_enabled",
		"growth_ai_run_policy",
		"doctor_ai_run_policy",
		"scheduled_only",
		"on_demand_recommended",
		"manual_only",
		"capability_policy_version",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
	if !strings.Contains(sql, "config ? 'doctor_ai_enabled'") || !strings.Contains(sql, "else false") {
		t.Fatal("migration must preserve explicit Doctor consent and default only absent consent off")
	}
	if !strings.Contains(sql, "opportunity_finding_source_mix") || !strings.Contains(sql, "ai_discovery_automation") {
		t.Fatal("migration must derive capabilities from both legacy settings")
	}
}
