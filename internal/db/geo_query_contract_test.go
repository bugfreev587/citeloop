package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGEOCompetitorUpsertPreservesExistingDomainsOnEmptyInput(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("queries", "geo.sql"))
	if err != nil {
		t.Fatalf("read geo queries: %v", err)
	}
	query := namedSQL(t, strings.ToLower(string(raw)), "UpsertGEOCompetitor")

	requireQuerySQL(t, query,
		"jsonb_array_length(excluded.domains) > 0",
		"else geo_competitors.domains",
	)
}
