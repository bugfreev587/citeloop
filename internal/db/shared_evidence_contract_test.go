package db

import (
	"os"
	"strings"
	"testing"
)

func TestSharedEvidenceMigrationDefinesIdempotentSourceNeutralEnvelope(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0077_shared_evidence.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"create table if not exists evidence_runs",
		"create table if not exists evidence_observations",
		"create table if not exists evidence_run_attempts",
		"create table if not exists evidence_consumptions",
		"collection_spec_fingerprint",
		"collection_owner_token",
		"lease_expires_at",
		"attempt_number",
		"normalized_target",
		"observed", "inferred", "model_assisted", "missing", "provider_unavailable",
		"confidence >= 0 and confidence <= 1",
		"completeness >= 0 and completeness <= 1",
		"unique index", "coalesce(window_start", "coalesce(window_end",
		"foreign key (run_id, project_id)",
		"provider_version",
		"enforce_evidence_observation_run_scope",
		"run.source = new.source",
		"run.normalized_target = new.normalized_target",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("shared evidence migration missing %q", want)
		}
	}
}

func TestSharedEvidenceQueriesAcquireBeforeCollectionAndPersistPartialStates(t *testing.T) {
	raw, err := os.ReadFile("queries/evidence.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"-- name: acquireevidencerun :one",
		"do update set",
		"collection_owner_token",
		"-- name: createevidenceobservation :one",
		"on conflict do nothing",
		"-- name: getevidenceobservation :one",
		"-- name: finishevidencerun :one",
		"collection_owner_token = sqlc.arg(collection_owner_token)",
		"evidence_runs.lease_expires_at <= now()",
		"select run.* from evidence_runs run",
		"for update",
		"-- name: linkevidenceconsumption :one",
		"-- name: listevidencerunsforconsumers :many",
		"attempt.attempt_number as consumed_attempt_number",
		"-- name: listevidenceobservations :many",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("shared evidence query missing %q", want)
		}
	}
}
