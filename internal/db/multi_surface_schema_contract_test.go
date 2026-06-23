package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readMultiSurfaceMigration(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0021_multi_surface_seo_growth.sql"))
	if err != nil {
		t.Fatalf("read multi-surface migration: %v", err)
	}
	return strings.ToLower(string(raw))
}

func TestMultiSurfaceMigrationExtendsExistingTables(t *testing.T) {
	migration := readMultiSurfaceMigration(t)
	for _, want := range []string{
		"create table if not exists seo_asset_types",
		"alter table content_actions",
		"add column if not exists asset_type",
		"add column if not exists target_surface_id",
		"add column if not exists risk_reasons",
		"add column if not exists evidence_snapshot",
		"add column if not exists diff_snapshot",
		"add column if not exists review_required",
		"add column if not exists verified_at",
		"alter table geo_external_surfaces",
		"add column if not exists source_url",
		"add column if not exists canonical_status",
		"add column if not exists indexability_status",
		"add column if not exists publication_status",
		"add column if not exists owner_confidence",
		"alter table geo_asset_briefs",
		"add column if not exists target_queries",
		"add column if not exists expected_citation_mechanism",
		"alter table seo_opportunities",
		"add column if not exists opportunity_key",
		"alter table articles",
		"add column if not exists publication_mode",
		"add column if not exists external_surface_id",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("multi-surface migration missing %q", want)
		}
	}
}

func TestMultiSurfaceMigrationDoesNotCreateParallelWorkflowTables(t *testing.T) {
	migration := readMultiSurfaceMigration(t)
	for _, forbidden := range []string{
		"create table if not exists seo_actions",
		"create table if not exists action_portfolios",
		"create table if not exists distribution_variants",
	} {
		if strings.Contains(migration, forbidden) {
			t.Fatalf("multi-surface migration must extend existing tables, found forbidden %q", forbidden)
		}
	}
}

func TestMultiSurfaceQueriesExposeFoundationRecords(t *testing.T) {
	seoRaw, err := os.ReadFile(filepath.Join("queries", "seo.sql"))
	if err != nil {
		t.Fatalf("read seo queries: %v", err)
	}
	geoRaw, err := os.ReadFile(filepath.Join("queries", "geo.sql"))
	if err != nil {
		t.Fatalf("read geo queries: %v", err)
	}
	queries := strings.ToLower(string(seoRaw) + "\n" + string(geoRaw))
	for _, want := range []string{
		"-- name: upsertseoassettype :one",
		"-- name: listseoassettypes :many",
		"asset_type",
		"target_surface_id",
		"verification_snapshot",
		"source_url",
		"expected_citation_mechanism",
	} {
		if !strings.Contains(queries, want) {
			t.Fatalf("multi-surface queries missing %q", want)
		}
	}
}

func TestGEORunsAgentCheckAllowsExternalSurfaceMonitor(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "migrations", "*.sql"))
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	var combined strings.Builder
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}
		combined.Write(raw)
		combined.WriteByte('\n')
	}
	if !strings.Contains(combined.String(), "'geo_external_surface_monitor'") {
		t.Fatalf("geo_runs agent check must allow %q", "geo_external_surface_monitor")
	}
}
