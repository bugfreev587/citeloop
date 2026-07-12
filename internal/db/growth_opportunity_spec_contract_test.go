package db

import (
	"os"
	"strings"
	"testing"
)

func TestGrowthOpportunitySpecMigrationRequiresDecisionReadyForwardWork(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0074_growth_opportunity_spec.sql")
	if err != nil {
		t.Fatalf("read growth opportunity spec migration: %v", err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"growth_spec_state",
		"growth_spec_version",
		"growth_spec_origin",
		"growth_spec_missing",
		"decision_ready_at",
		"growth_opportunity_forward_spec_required",
		"hypothesis",
		"audience",
		"baseline",
		"primary_metric",
		"expected_change",
		"measurement_policy",
		"attribution_model",
		"stop_conditions",
		"reconsider_conditions",
		"not valid",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("growth opportunity spec migration missing %q", want)
		}
	}
	if !strings.Contains(sql, "growth_spec_origin = 'forward'") || !strings.Contains(sql, "growth_spec_state = 'decision_ready'") {
		t.Fatal("forward canonical Growth opportunities must be decision-ready")
	}
	if !strings.Contains(sql, "jsonb_typeof(growth_spec_missing) = 'array'") {
		t.Fatal("growth_spec_missing must be a JSON array")
	}
	if strings.Count(sql, "not valid") < 5 {
		t.Fatal("all populated-table Growth spec checks must use the add-NOT-VALID/validate-later rollout")
	}
	if strings.Contains(sql, "create index") {
		t.Fatal("transactional Growth spec migration must not build a blocking index")
	}
}

func TestCanonicalGrowthWriterPersistsTypedSpecification(t *testing.T) {
	raw, err := os.ReadFile("queries/seo.sql")
	if err != nil {
		t.Fatalf("read seo queries: %v", err)
	}
	source := string(raw)
	start := strings.Index(source, "-- name: CreateCanonicalGrowthOpportunity")
	end := strings.Index(source[start:], "-- name: MergeCanonicalGrowthOpportunityEvidence")
	if start < 0 || end < 0 {
		t.Fatal("missing canonical Growth writer query")
	}
	body := source[start : start+end]
	for _, want := range []string{
		"growth_spec_state",
		"growth_spec_version",
		"growth_spec_origin",
		"growth_spec",
		"growth_spec_missing",
		"decision_ready_at",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("canonical Growth writer does not persist %q", want)
		}
	}
}

func TestLegacyGrowthCutoverRecoversTargetFromExecutionArtifact(t *testing.T) {
	raw, err := os.ReadFile("queries/seo.sql")
	if err != nil {
		t.Fatalf("read seo queries: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"-- name: GetLegacyGrowthIntendedTarget",
		"-- name: LockLegacyGrowthIntendedTarget",
		"locked_opportunity as materialized",
		"locked_actions as materialized",
		"locked_articles as materialized",
		"for update of selected",
		"for update of selected_article",
		"article.seo_meta->>'canonical_url'",
		"article.seo_meta->>'slug'",
		"-- name: MarkLegacyGrowthOpportunityCanonical",
		"evidence = sqlc.arg(evidence)::jsonb",
		"evidence_fingerprint = sqlc.arg(evidence_fingerprint)",
		"-- name: RollbackGrowthCutoverCanonical",
		"evidence = sqlc.arg(original_evidence)::jsonb",
		"evidence_fingerprint = sqlc.arg(original_evidence_fingerprint)",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("legacy Growth cutover target recovery missing %q", want)
		}
	}
}
