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
		"-- name: getorcreatesitefixmeasurementcheckpoint :one",
		"on conflict (measurement_id, checkpoint_key, attempt_number)",
		"-- name: listsitefixmeasurementcheckpoints :many",
		"-- name: terminalizesitefixmeasurement :one",
		"-- name: getorcreatesitefixmeasurementterminaloutcome :one",
		"-- name: getorcreatesitefixmeasurementlearning :one",
		"-- name: getorcreatesitefixmeasurementqualityrecord :one",
		"-- name: enqueuesitefixmeasurementhandoff :one",
		"-- name: claimsitefixmeasurementhandoff :one",
		"-- name: completesitefixmeasurementhandoff :one",
		"-- name: retrysitefixmeasurementhandoff :one",
		"-- name: terminalizeexpiredsitefixmeasurementhandoffs :many",
		"-- name: listsitefixmeasurementsforresults :many",
		"'site_fix'::text as source_type",
		"limit least(greatest(sqlc.arg(page_limit)::int, 1), 100)",
		"offset greatest(sqlc.arg(page_offset)::int, 0)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("Site Fix measurement queries missing %q", want)
		}
	}
}

func TestSiteFixMeasurementPolicyIsFullyFiniteAndDeadlineIsBoundOnActivation(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0087_site_fix_measurements.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"site_fix_measurement_policy_is_finite",
		"primary_days > duration_days",
		"follow_up_day <= previous_day",
		"follow_up_day > duration_days",
		"jsonb_typeof(policy->'metric_thresholds')",
		"jsonb_typeof(policy->'guardrails')",
		"jsonb_typeof(policy->'required_data_sources')",
		"is distinct from 'number'",
		"is distinct from 'array'",
		"coalesce(jsonb_typeof(policy->'minimum_sample')",
		"policy->'metric_thresholds'->>'direction' not in ('increase','decrease')",
		"policy->'metric_thresholds'->>'kind' not in ('absolute','relative')",
		"threshold_value < 0",
		"policy->'minimum_sample'->'minimum_after_periods'",
		"policy->'minimum_sample'->'minimum_after_sample'",
		"jsonb_array_elements(policy->'required_data_sources')",
		"nullif(btrim(source_value #>> '{}'), '') is null",
		"site_fix_measurement_data_source_is_supported_v1",
		"not in ('gsc','ga4','geo')",
		"not site_fix_measurement_data_source_is_supported_v1(source_name)",
		"jsonb_array_elements(policy->'guardrails')",
		"guardrail_value->>'max_adverse_relative'",
		"site_fix_measurement_policy_is_finite(measurement_policy_snapshot)",
		"absolute_terminal_at = started_at +",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("finite Site Fix measurement policy contract missing %q", want)
		}
	}

	queries, err := os.ReadFile("queries/site_fix_measurements.sql")
	if err != nil {
		t.Fatal(err)
	}
	querySQL := strings.ToLower(string(queries))
	activation := queryBlock(querySQL, "-- name: activatesitefixmeasurement :one")
	for _, want := range []string{"absolute_terminal_at =", "max_measuring_duration_days", "terminalization_grace_period_days"} {
		if !strings.Contains(activation, want) {
			t.Fatalf("activation must bind an exact finite deadline; missing %q", want)
		}
	}
}

func TestSiteFixMeasurementHandoffAndGenerationAreIdempotentAndMonotonic(t *testing.T) {
	migration, err := os.ReadFile("../migrations/0087_site_fix_measurements.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(migration))
	for _, want := range []string{
		"unique (project_id, site_fix_id, measurement_generation)",
		"creation_idempotency_key",
		"unique (project_id, site_fix_id, creation_idempotency_key)",
		"create table if not exists site_fix_measurement_generation_counters",
		"last_generation",
		"create or replace function create_site_fix_measurement_idempotently",
		"for update",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("measurement allocation/handoff schema missing %q", want)
		}
	}

	queries, err := os.ReadFile("queries/site_fix_measurements.sql")
	if err != nil {
		t.Fatal(err)
	}
	querySQL := strings.ToLower(string(queries))
	create := queryBlock(querySQL, "-- name: createsitefixmeasurement :one")
	for _, want := range []string{
		"create_site_fix_measurement_idempotently",
		"sqlc.arg(creation_idempotency_key)",
	} {
		if !strings.Contains(create, want) {
			t.Fatalf("concurrency-safe generation allocation missing %q", want)
		}
	}
	if strings.Contains(create, "sqlc.arg(measurement_generation)") {
		t.Fatal("caller must not be able to backfill an older measurement generation")
	}
	handoff := queryBlock(querySQL, "-- name: enqueuesitefixmeasurementhandoff :one")
	if !strings.Contains(handoff, "on conflict (project_id, site_fix_id, measurement_generation)") {
		t.Fatal("handoff idempotency must use the stable measurement generation identity")
	}
	claim := queryBlock(querySQL, "-- name: claimsitefixmeasurementhandoff :one")
	for _, want := range []string{"candidate.status = 'processing'", "candidate.locked_until <= sqlc.arg(now_at)", "lock_token = sqlc.arg(lock_token)"} {
		if !strings.Contains(claim, want) {
			t.Fatalf("handoff claim must reclaim expired leases with fencing; missing %q", want)
		}
	}
	for _, want := range []string{
		"create index if not exists idx_site_fix_measurement_handoff_processing_lease",
		"where status = 'processing'",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("handoff lease recovery index missing %q", want)
		}
	}
}

func TestSiteFixMeasurementAppendOnlyReplayReturnsCanonicalRowAtomically(t *testing.T) {
	raw, err := os.ReadFile("queries/site_fix_measurements.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, name := range []string{
		"-- name: getorcreatesitefixmeasurementcheckpoint :one",
		"-- name: getorcreatesitefixmeasurementterminaloutcome :one",
		"-- name: getorcreatesitefixmeasurementlearning :one",
		"-- name: getorcreatesitefixmeasurementqualityrecord :one",
	} {
		insert := queryBlock(sql, name)
		if !strings.Contains(insert, "on conflict") || !strings.Contains(insert, "do update") || !strings.Contains(insert, "returning *") {
			t.Fatalf("%s must atomically return the canonical row on replay", name)
		}
	}
	migration, err := os.ReadFile("../migrations/0087_site_fix_measurements.sql")
	if err != nil {
		t.Fatal(err)
	}
	migrationSQL := strings.ToLower(string(migration))
	for _, want := range []string{
		"reject_site_fix_measurement_append_only_mutation",
		"tg_op = 'update' and to_jsonb(new) = to_jsonb(old)",
		"raise exception 'site fix measurement evidence is append-only'",
	} {
		if !strings.Contains(migrationSQL, want) {
			t.Fatalf("append-only replay trigger missing %q", want)
		}
	}
}

func queryBlock(sql, name string) string {
	start := strings.Index(sql, name)
	if start < 0 {
		return ""
	}
	block := sql[start:]
	if next := strings.Index(block[len(name):], "-- name:"); next >= 0 {
		block = block[:len(name)+next]
	}
	return block
}
