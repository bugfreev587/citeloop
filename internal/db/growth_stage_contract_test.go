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

func TestActiveGEOPromptsExcludeInternalSensitiveTerms(t *testing.T) {
	raw, err := os.ReadFile("queries/geo.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(raw))
	start := strings.Index(body, "-- name: listactivegeoprompts")
	if start < 0 || !strings.Contains(body[start:], "!~") || !strings.Contains(body[start:], "aes") {
		t.Fatal("active GEO prompt selection must reject internal-sensitive terms at runtime")
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
