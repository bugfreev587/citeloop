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
