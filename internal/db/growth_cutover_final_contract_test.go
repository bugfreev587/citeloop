package db

import (
	"os"
	"strings"
	"testing"
)

func TestDuplicateGrowthIsMergedHiddenAndExecutionFenced(t *testing.T) {
	queries, err := os.ReadFile("queries/seo.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(queries))
	for _, want := range []string{
		"mergecanonicalgrowthopportunityevidence",
		"createduplicategrowthopportunityalias",
		"merged_cross_source_evidence",
		"priority_score = greatest",
		"confidence = greatest",
		"'duplicate'",
	} {
		if !strings.Contains(strings.ReplaceAll(sql, " ", ""), strings.ReplaceAll(want, " ", "")) {
			t.Fatalf("duplicate cutover SQL missing %q", want)
		}
	}
	for name, query := range map[string]string{
		"list":    listSEOOpportunities,
		"get":     getSEOOpportunity,
		"count":   countOpenSEOOpportunities,
		"status":  updateSEOOpportunityStatus,
		"execute": growthOpportunityExecutionGuard,
	} {
		lower := strings.ToLower(query)
		if !strings.Contains(lower, "growth_opportunity_work_aliases") || !strings.Contains(lower, "'duplicate'") {
			t.Fatalf("%s path exposes an active duplicate: %s", name, lower)
		}
	}
	migration, err := os.ReadFile("../migrations/0063_canonical_growth_writer_cutover.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(string(migration)), "content action cannot execute a duplicate growth opportunity") {
		t.Fatal("direct content action writes are not fenced from duplicate Growth opportunities")
	}
}

func TestGrowthRollbackTombstonesCanonicalProvenance(t *testing.T) {
	queries, err := os.ReadFile("queries/seo.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(queries))
	for _, queryName := range []string{"rollbackgrowthcutoverduplicate", "rollbackgrowthcutovercanonical"} {
		start := strings.Index(strings.ReplaceAll(sql, " ", ""), "--name:"+queryName)
		if start < 0 {
			t.Fatalf("missing %s", queryName)
		}
	}
	rollbackRegion := sql[strings.Index(sql, "-- name: rollbackgrowthcutoverduplicate"):strings.Index(sql, "-- name: updateseoopportunitystatus")]
	if strings.Contains(rollbackRegion, "delete from") {
		t.Fatalf("rollback deletes canonical provenance: %s", rollbackRegion)
	}
	for _, want := range []string{"migration_rolled_back", "active = false", "disposition = 'rolled_back'", "status = 'superseded'"} {
		if !strings.Contains(rollbackRegion, want) {
			t.Fatalf("rollback tombstone SQL missing %q", want)
		}
	}
}
