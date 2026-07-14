package db

import (
	"os"
	"strings"
	"testing"
)

func TestGrowthStageMigrationAndQueries(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0088_growth_stage.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(raw))
	for _, required := range []string{
		"create table if not exists growth_stage_settings",
		"create table if not exists growth_stage_events",
		"foundation", "traction", "scale", "optimize",
		"setting_version", "is_default_unconfirmed",
		"pending", "running", "complete", "failed",
		"internal_sensitive_term", "update geo_prompts",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("growth-stage migration missing %q", required)
		}
	}

	queries, err := os.ReadFile("queries/growth_radar.sql")
	if err != nil {
		t.Fatal(err)
	}
	queryBody := string(queries)
	for _, required := range []string{
		"-- name: GetGrowthStageSetting",
		"-- name: UpsertGrowthStageSetting",
		"-- name: CreateGrowthStageEvent",
		"-- name: UpdateGrowthStageEventStatus",
		"-- name: CountActiveGrowthRadarWatchlist",
		"-- name: UpdateGrowthRadarWatchlistStageScore",
	} {
		if !strings.Contains(queryBody, required) {
			t.Errorf("growth-stage query contract missing %q", required)
		}
	}
}

func TestActiveGEOPromptsExcludeSecretShapesWithoutBanningPublicTechnicalTopics(t *testing.T) {
	raw, err := os.ReadFile("queries/geo.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(raw))
	start := strings.Index(body, "-- name: listactivegeoprompts")
	if start < 0 {
		t.Fatal("missing active GEO prompt query")
	}
	section := body[start:]
	if next := strings.Index(section[len("-- name: listactivegeoprompts"):], "-- name:"); next >= 0 {
		section = section[:len("-- name: listactivegeoprompts")+next]
	}
	for _, required := range []string{"!~", "private key", "[=:]", "postgres(ql)?"} {
		if !strings.Contains(section, required) {
			t.Fatalf("active GEO prompt selection missing secret-shape guard %q", required)
		}
	}
	for _, obsolete := range []string{"|database|postgres|", "|aes|encrypt|", "|kubernetes|docker|"} {
		if strings.Contains(section, obsolete) {
			t.Fatalf("active GEO prompt selection still bans public technical nouns with %q", obsolete)
		}
	}
}

func TestGrowthRadarDemandUsesDeterministicAliases(t *testing.T) {
	raw, err := os.ReadFile("queries/growth_radar.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(raw))
	if !strings.Contains(body, "sqlc.arg(queries)::text[]") || strings.Contains(body, "lower(btrim(query)) = lower(btrim(sqlc.arg(query)))") {
		t.Fatal("Growth Radar demand must match persisted normalized aliases, not only the complete prompt")
	}
}
