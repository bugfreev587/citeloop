package db

import (
	"os"
	"strings"
	"testing"
)

func TestLegacyDiscoverySettingsAreRetiredAfterCapabilityMigration(t *testing.T) {
	migration, err := os.ReadFile("../migrations/0080_retire_legacy_discovery_settings.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(migration))
	for _, want := range []string{"capability_policy_version", "growth_signal_enabled", "growth_ai_enabled", "growth_ai_run_policy", "opportunity_finding_source_mix", "ai_discovery_automation", " - "} {
		if !strings.Contains(sql, want) {
			t.Fatalf("legacy settings cleanup migration missing %q", want)
		}
	}

	files := []string{"../config/config.go", "../api/handlers_seo.go", "../../web/app/lib/api.ts"}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, retired := range []string{"opportunity_finding_source_mix", "ai_discovery_automation"} {
			if strings.Contains(strings.ToLower(string(raw)), retired) {
				t.Fatalf("%s still exposes retired setting %q", path, retired)
			}
		}
	}
}
