package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGEOClassificationAuditRecordsSchemaAndQuery(t *testing.T) {
	rawMigration, err := os.ReadFile("../migrations/0099_geo_classification_audit_records.sql")
	if err != nil {
		t.Fatalf("read GEO classification audit migration: %v", err)
	}
	migration := strings.ToLower(string(rawMigration))
	for _, required := range []string{
		"create table if not exists geo_classification_audit_records",
		"project_id uuid not null references projects(id) on delete cascade",
		"run_id uuid not null references geo_runs(id) on delete cascade",
		"observation_id uuid references geo_observations(id) on delete set null",
		"classifier_type text not null",
		"input jsonb not null default '{}'",
		"output jsonb not null default '{}'",
		"reason_codes jsonb not null default '[]'",
	} {
		if !strings.Contains(migration, required) {
			t.Fatalf("classification audit migration missing %q", required)
		}
	}

	rawQueries, err := os.ReadFile(filepath.Join("queries", "geo.sql"))
	if err != nil {
		t.Fatalf("read GEO queries: %v", err)
	}
	query := namedSQL(t, strings.ToLower(string(rawQueries)), "CreateGEOClassificationAuditRecord")
	for _, required := range []string{
		"insert into geo_classification_audit_records",
		"project_id, run_id, observation_id, classifier_type, input, output, reason_codes",
		"returning *",
	} {
		if !strings.Contains(query, required) {
			t.Fatalf("CreateGEOClassificationAuditRecord missing %q", required)
		}
	}
}
