package db

import (
	"os"
	"strings"
	"testing"
)

func TestSiteFixMeasurementMigrationDefinesIsolatedFiniteAggregate(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0087_site_fix_measurements.sql")
	if err != nil {
		t.Fatalf("read Site Fix measurement migration: %v", err)
	}
	sql := strings.ToLower(string(raw))

	for _, want := range []string{
		"add column if not exists fix_type",
		"add column if not exists impact_mode",
		"default 'unclassified'",
		"add column if not exists measurement_policy",
		"default 'verification_only'",
		"classifier_version",
		"decision_origin",
		"decision_confidence",
		"growth_hypothesis",
		"primary_metric",
		"secondary_metrics",
		"measurement_policy_snapshot",
		"create table if not exists site_fix_measurements",
		"foreign key (project_id, site_fix_id)",
		"references site_fixes(project_id, id)",
		"unique (project_id, site_fix_id, measurement_generation)",
		"target_url",
		"normalized_target_url",
		"target_query",
		"target_identity",
		"measurement_policy_version",
		"baseline_window",
		"baseline_snapshot",
		"baseline_status",
		"absolute_terminal_at",
		"terminal_outcome",
		"results_deep_link",
		"create table if not exists site_fix_measurement_checkpoints",
		"unique (measurement_id, checkpoint_key, attempt_number)",
		"required_data_sources",
		"data_availability",
		"minimum_sample",
		"guardrail_results",
		"retry_classification",
		"create table if not exists site_fix_measurement_terminal_outcomes",
		"create table if not exists site_fix_measurement_learnings",
		"create table if not exists site_fix_measurement_quality_records",
		"directional_learning",
		"measurement_quality",
		"foreign key (project_id, terminal_outcome_id, measurement_id)",
		"site_fix_measurement_checkpoints_immutable",
		"site_fix_measurement_terminal_outcomes_immutable",
		"site_fix_measurement_learnings_immutable",
		"site_fix_measurement_quality_records_immutable",
		"create table if not exists site_fix_measurement_handoff_outbox",
		"idempotency_key",
		"attempt_count",
		"next_attempt_at",
		"last_error_classification",
		"not valid",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("Site Fix measurement migration missing %q", want)
		}
	}

	if strings.Contains(sql, "\nupdate site_fixes") {
		t.Fatal("rolling migration must not rewrite existing Site Fix rows")
	}
	if strings.Contains(sql, "references action_measurements") || strings.Contains(sql, "content_action_id") {
		t.Fatal("Site Fix measurements must remain independent from the Content Action write model")
	}
}

func TestSiteFixMeasurementValidationMigrationSeparatesOnlineValidation(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0088_site_fix_measurements_validate.sql")
	if err != nil {
		t.Fatalf("read Site Fix measurement validation migration: %v", err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"validate constraint site_fixes_fix_type_check",
		"validate constraint site_fixes_impact_mode_check",
		"validate constraint site_fixes_measurement_policy_check",
		"validate constraint site_fixes_measurement_readiness_check",
		"validate constraint site_fix_measurements_site_fix_project_fk",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("Site Fix validation migration missing %q", want)
		}
	}
	if strings.Contains(sql, "add constraint") || strings.Contains(sql, "create table") {
		t.Fatal("validation migration must only validate constraints created by the rolling migration")
	}
}

func TestSiteFixMeasurementQueriesCoverLifecycleSchedulerAndResults(t *testing.T) {
	raw, err := os.ReadFile("queries/site_fix_measurements.sql")
	if err != nil {
		t.Fatalf("read Site Fix measurement queries: %v", err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"-- name: createsitefixmeasurement :one",
		"on conflict (project_id, site_fix_id, measurement_generation)",
		"-- name: getsitefixmeasurement :one",
		"-- name: getlatestsitefixmeasurementforfix :one",
		"-- name: updatesitefixmeasurementbaseline :one",
		"-- name: activatesitefixmeasurement :one",
		"-- name: claimduesitefixmeasurement :one",
		"for update skip locked",
		"-- name: insertsitefixmeasurementcheckpoint :one",
		"on conflict (measurement_id, checkpoint_key, attempt_number)",
		"-- name: listsitefixmeasurementcheckpoints :many",
		"-- name: terminalizesitefixmeasurement :one",
		"-- name: createsitefixmeasurementterminaloutcome :one",
		"-- name: createsitefixmeasurementlearning :one",
		"-- name: createsitefixmeasurementqualityrecord :one",
		"-- name: enqueuesitefixmeasurementhandoff :one",
		"-- name: claimsitefixmeasurementhandoff :one",
		"-- name: completesitefixmeasurementhandoff :one",
		"-- name: retrysitefixmeasurementhandoff :one",
		"-- name: listsitefixmeasurementsforresults :many",
		"'site_fix'::text as source_type",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("Site Fix measurement queries missing %q", want)
		}
	}
}
