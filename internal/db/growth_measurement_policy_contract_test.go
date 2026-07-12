package db

import (
	"os"
	"strings"
	"testing"
)

func TestGrowthMeasurementPolicyMigrationIsFiniteAndImmutable(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0075_growth_action_measurement_policy.sql")
	if err != nil {
		t.Fatalf("read Growth measurement migration: %v", err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"measurement_policy_version",
		"measurement_policy jsonb",
		"measuring_started_at",
		"absolute_terminal_at",
		"measurement_terminal_reason",
		"checkpoint_role",
		"checkpoint_attempt",
		"data_quality_state",
		"source_freshness",
		"baseline",
		"early",
		"primary",
		"follow_up",
		"bind_immutable_growth_measurement_policy",
		"measurement policy is immutable after measuring starts",
		"absolute measurement deadline is immutable",
		"not valid",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("Growth measurement migration missing %q", want)
		}
	}
	if strings.Contains(sql, "create index") {
		t.Fatal("transactional populated-table migration must not build a blocking index")
	}
	if strings.Contains(sql, "\nupdate content_actions") || strings.Contains(sql, "\nupdate action_measurements") {
		t.Fatal("metadata migration must not rewrite populated tables")
	}
	for _, want := range []string{
		"before insert or update",
		"measurement policy cannot be bound before measuring starts",
		"absolute_terminal_at = measuring_started_at",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("Growth measurement binding invariant missing %q", want)
		}
	}
}

func TestMeasurementQueriesPersistCheckpointContractAndDeadline(t *testing.T) {
	raw, err := os.ReadFile("queries/seo.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"absolute_terminal_at <= sqlc.arg(now_at)::timestamptz",
		"-- name: bindlegacymeasuringcontentactionpolicy :one",
		"and measuring_started_at is null",
		"and ca.canonical_read_only = false",
		"checkpoint_role",
		"measurement_policy_version",
		"checkpoint_attempt",
		"data_quality_state",
		"source_freshness",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("measurement queries missing %q", want)
		}
	}
}
