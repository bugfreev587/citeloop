package db

import (
	"os"
	"strings"
	"testing"
)

func TestActionMeasurementsSchemaAndQueriesExist(t *testing.T) {
	migration, err := os.ReadFile("../migrations/0030_action_measurements.sql")
	if err != nil {
		t.Fatalf("read action measurements migration: %v", err)
	}
	queries, err := os.ReadFile("queries/seo.sql")
	if err != nil {
		t.Fatalf("read seo queries: %v", err)
	}
	source := string(migration) + "\n" + string(queries)
	for _, want := range []string{
		"create table if not exists action_measurements",
		"checkpoint_day",
		"window_start",
		"window_end",
		"seo_metrics",
		"ga4_metrics",
		"geo_metrics",
		"execution_metrics",
		"outcome_label",
		"outcome_reason",
		"attribution_confidence",
		"confounders",
		"unique (project_id, content_action_id, checkpoint_day)",
		"-- name: UpsertActionMeasurement :one",
		"-- name: ListActionMeasurementsForProject :many",
		"-- name: ListActionMeasurementsForAction :many",
		"-- name: ListResultsActionRows :many",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("measurement attribution contract missing %q", want)
		}
	}
}
