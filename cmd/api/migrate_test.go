package main

import (
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/migrations"
)

func TestMigrationRunnerSafetyContract(t *testing.T) {
	raw, err := os.ReadFile("migrate.go")
	if err != nil {
		t.Fatal(err)
	}
	source := strings.ToLower(string(raw))
	requireSource := func(t *testing.T, required ...string) {
		t.Helper()
		for _, want := range required {
			if !strings.Contains(source, want) {
				t.Errorf("migration runner missing %q", want)
			}
		}
	}

	t.Run("dedicated connection and session lock", func(t *testing.T) {
		requireSource(t,
			"pool.acquire(ctx)",
			"pg_advisory_lock",
			"pg_advisory_unlock",
			"conn.release()",
		)
	})

	t.Run("ordinary migration marker is transactional", func(t *testing.T) {
		requireSource(t,
			"beginmigration(ctx)",
			"tx.queryrow",
			"tx.exec",
			"insert into schema_migrations",
			"tx.commit",
			"tx.rollback",
		)
	})

	t.Run("nontransactional directive uses a dedicated path", func(t *testing.T) {
		requireSource(t,
			"citeloop:migration-mode=nontransactional",
			"citeloop:index=",
			"migrationmodenontransactional",
			"applynontransactionalmigration",
		)
	})

	t.Run("invalid concurrent index is recovered safely", func(t *testing.T) {
		requireSource(t,
			"from pg_class",
			"join pg_index",
			"indisvalid",
			"drop index concurrently",
			"pgx.identifier",
			".sanitize()",
		)
	})

	t.Run("malformed directives and statements fail closed", func(t *testing.T) {
		requireSource(t,
			"invalid migration directive",
			"invalid concurrent index name",
			"exactly one create index concurrently statement",
		)
	})

	t.Run("nontransactional work has a bounded context", func(t *testing.T) {
		requireSource(t,
			"context.withtimeout",
			"nontransactionalmigrationtimeout",
		)
	})
}

func TestEmbeddedNontransactionalIndexMigrationContract(t *testing.T) {
	expectedConcurrent := map[string]string{
		"0049_01_doctor_findings_project_identity.sql":         "seo_doctor_findings_project_id_kind_key",
		"0049_02_discovery_candidates_project_identity.sql":    "discovery_candidates_project_id_id_key",
		"0049_03_work_signature_registry_project_identity.sql": "work_signature_registry_project_candidate_id_key",
		"0051_01_site_change_active_content_action.sql":        "idx_active_site_change_application_content_action",
		"0051_02_site_change_active_site_fix.sql":              "uniq_active_site_change_application_site_fix",
		"0051_03_site_change_site_fix_history.sql":             "idx_site_change_applications_site_fix",
		"0051_04_rollback_records_site_fix.sql":                "idx_rollback_records_site_fix_id",
		"0051_05_discovery_candidates_shadow_run.sql":          "idx_discovery_candidates_shadow_run_fk",
		"0051_06_work_signature_registry_shadow_run.sql":       "idx_work_signature_registry_shadow_run_fk",
	}

	for name, indexName := range expectedConcurrent {
		name, indexName := name, indexName
		t.Run(name, func(t *testing.T) {
			raw, err := fs.ReadFile(migrations.FS, name)
			if err != nil {
				t.Errorf("read embedded migration: %v", err)
				return
			}
			sql := strings.ToLower(string(raw))
			for _, want := range []string{
				"-- citeloop:migration-mode=nontransactional",
				"-- citeloop:index=" + indexName,
				"index concurrently if not exists " + indexName,
			} {
				if !strings.Contains(sql, want) {
					t.Errorf("migration missing %q", want)
				}
			}
			if got := strings.Count(sql, "create "); got != 1 {
				t.Errorf("nontransactional migration must contain one CREATE statement, got %d", got)
			}
		})
	}

	grouped, err := fs.ReadFile(migrations.FS, "0050_doctor_site_fix_indexes.sql")
	if err != nil {
		t.Fatal(err)
	}
	groupedSQL := strings.ToLower(string(grouped))
	for _, populatedTable := range []string{
		"site_change_applications",
		"rollback_records",
		"discovery_candidates",
		"work_signature_registry",
	} {
		if strings.Contains(groupedSQL, "on "+populatedTable+" ") {
			t.Errorf("grouped migration must not build an ordinary index on populated table %s", populatedTable)
		}
	}
}
