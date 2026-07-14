package db

import (
	"os"
	"strings"
	"testing"
)

func TestGEOCompetitorDomainBackfillMigration(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0098_geo_competitor_domain_backfill.sql")
	if err != nil {
		t.Fatalf("read GEO competitor domain backfill migration: %v", err)
	}
	migration := strings.ToLower(string(raw))
	for _, required := range []string{
		"update geo_competitors",
		"status = 'active'",
		"domains = '[]'::jsonb",
		"jsonb_array_elements_text(competitor.aliases)",
		"regexp_match",
		"jsonb_build_array",
	} {
		if !strings.Contains(migration, required) {
			t.Fatalf("domain backfill migration missing %q", required)
		}
	}
	if strings.Contains(migration, "domains = excluded.domains") {
		t.Fatal("domain backfill migration must not reuse empty-upsert overwrite semantics")
	}
}
