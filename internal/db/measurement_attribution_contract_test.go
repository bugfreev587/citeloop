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
		"-- name: InsertActionMeasurementCheckpoint :exec",
		"on conflict (project_id, content_action_id, checkpoint_day) do nothing",
		"-- name: ListActionMeasurementsForProject :many",
		"-- name: ListActionMeasurementsForAction :many",
		"-- name: ListResultsActionRows :many",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("measurement attribution contract missing %q", want)
		}
	}
}

func TestResultsAndVisibilityExposeFiniteMeasurementLifecycle(t *testing.T) {
	queries, err := os.ReadFile("queries/seo.sql")
	if err != nil {
		t.Fatalf("read seo queries: %v", err)
	}
	source := string(queries)
	for _, queryName := range []string{
		"-- name: ListVisibilityActionRows :many",
		"-- name: ListResultsActionRows :many",
		"-- name: GetResultsActionRow :one",
	} {
		start := strings.Index(source, queryName)
		if start < 0 {
			t.Fatalf("query missing %s", queryName)
		}
		block := source[start:]
		if next := strings.Index(block[len(queryName):], "-- name:"); next >= 0 {
			block = block[:len(queryName)+next]
		}
		for _, want := range []string{
			"ca.measurement_policy_version",
			"ca.measurement_policy",
			"ca.measuring_started_at",
			"ca.absolute_terminal_at",
			"ca.measurement_terminal_reason",
		} {
			if !strings.Contains(block, want) {
				t.Fatalf("%s missing %q", queryName, want)
			}
		}
	}
}

func TestResultsActionRowsOnlyReturnActionsWithExecutionEvidence(t *testing.T) {
	queries, err := os.ReadFile("queries/seo.sql")
	if err != nil {
		t.Fatalf("read seo queries: %v", err)
	}
	source := string(queries)
	start := strings.Index(source, "-- name: ListResultsActionRows :many")
	if start < 0 {
		t.Fatal("ListResultsActionRows query missing")
	}
	block := source[start:]
	for _, want := range []string{
		"ca.status in ('published','measuring','completed','verification_failed','recovery_required')",
		"ca.published_at is not null",
		"ca.verified_at is not null",
		"exists (",
		"from action_measurements am",
		"am.content_action_id = ca.id",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("ListResultsActionRows must exclude unexecuted ready_for_review actions; missing %q", want)
		}
	}
}
