package db

import (
	"os"
	"strings"
	"testing"
)

func TestCrossLineRelationshipSchemaPersistsBlockerSemantics(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0061_cross_line_work_relationships.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"create table if not exists work_relationships",
		"dependent_candidate_id",
		"blocking_work_signature_id",
		"dependency_class",
		"hard_blocker",
		"soft_dependency",
		"overlapping_mutation_fields",
		"reassessment_trigger",
		"attribution_confounder",
		"unique nulls not distinct",
		"create constraint trigger seo_opportunities_canonical_growth_reservation",
		"create constraint trigger content_actions_canonical_growth_reservation",
		"deferrable initially deferred",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("cross-line relationship schema missing %q", want)
		}
	}
}

func TestGrowthExecutionGuardUsesCanonicalRelationshipAndSignature(t *testing.T) {
	sql := strings.ToLower(growthOpportunityExecutionGuard)
	for _, want := range []string{
		"work_signature_registry",
		"work_relationships",
		"dependency_class = 'hard_blocker'",
		"relationship.active = true",
		"signature.active = true",
		"reserved_work_type = 'seo_opportunity'",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("Growth execution guard missing %q: %s", want, sql)
		}
	}
}

func TestCanonicalGrowthCutoverConservesLegacyAndFencesNewBypass(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0063_canonical_growth_writer_cutover.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"canonical_growth boolean not null default false",
		"writer_authority = 'canonical'",
		"legacy growth creation is disabled",
		"legacy growth evidence/work identity is read-only",
		"create table if not exists growth_opportunity_work_aliases",
		"disposition in ('canonicalized','duplicate')",
		"before insert or update on seo_opportunities",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("canonical Growth cutover missing %q", want)
		}
	}
	createSQL := strings.ToLower(createCanonicalGrowthOpportunity)
	if !strings.Contains(createSQL, "canonical_growth") || !strings.Contains(createSQL, "true") {
		t.Fatalf("canonical Growth insert does not mark ownership: %s", createSQL)
	}
	guardSQL := strings.ToLower(growthOpportunityExecutionGuard)
	if strings.Contains(guardSQL, "canonical_growth = false") {
		t.Fatalf("canonical authority execution guard lets unreserved legacy work bypass: %s", guardSQL)
	}
}
