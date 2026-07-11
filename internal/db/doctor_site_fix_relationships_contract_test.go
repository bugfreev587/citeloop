package db

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readDoctorMigrationContractFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "migrations", name))
	if err != nil {
		t.Errorf("read %s: %v", name, err)
		return ""
	}
	return strings.ToLower(string(raw))
}

func requireDoctorMigrationSQL(t *testing.T, sql string, required ...string) {
	t.Helper()
	for _, want := range required {
		if !strings.Contains(sql, want) {
			t.Errorf("migration missing %q", want)
		}
	}
}

func TestDoctorSiteFixRelationshipMigrationContracts(t *testing.T) {
	base := readDoctorMigrationContractFile(t, "0048_doctor_site_fixes.sql")
	findingsIdentity := readDoctorMigrationContractFile(t, "0049_01_doctor_findings_project_identity.sql")
	candidatesIdentity := readDoctorMigrationContractFile(t, "0049_02_discovery_candidates_project_identity.sql")
	registryIdentity := readDoctorMigrationContractFile(t, "0049_03_work_signature_registry_project_identity.sql")
	add := readDoctorMigrationContractFile(t, "0049_10_doctor_site_fix_relationships_add.sql")
	validate := readDoctorMigrationContractFile(t, "0049_11_doctor_site_fix_relationships_validate.sql")
	swap := readDoctorMigrationContractFile(t, "0049_12_doctor_site_fix_relationships_swap.sql")
	indexes := readDoctorMigrationContractFile(t, "0050_doctor_site_fix_indexes.sql")

	t.Run("finding rollback status is safely replaced", func(t *testing.T) {
		requireDoctorMigrationSQL(t, add,
			"seo_doctor_findings_status_check_v2",
			"'active','resolved','dismissed','converted','migration_rolled_back'",
			"not valid",
		)
		requireDoctorMigrationSQL(t, validate,
			"validate constraint seo_doctor_findings_status_check_v2",
		)
		requireDoctorMigrationSQL(t, swap,
			"drop constraint if exists seo_doctor_findings_status_check;",
		)
		if strings.Index(add, "seo_doctor_findings_status_check_v2") > strings.Index(add, "not valid") {
			t.Error("replacement finding status check must be added NOT VALID")
		}
	})

	t.Run("canonical relationships are project consistent", func(t *testing.T) {
		requireDoctorMigrationSQL(t, findingsIdentity,
			"create unique index if not exists seo_doctor_findings_project_id_kind_key",
			"on seo_doctor_findings (project_id, id, finding_kind)",
		)
		requireDoctorMigrationSQL(t, candidatesIdentity,
			"create unique index if not exists discovery_candidates_project_id_id_key",
			"on discovery_candidates (project_id, id)",
		)
		requireDoctorMigrationSQL(t, registryIdentity,
			"create unique index if not exists work_signature_registry_project_candidate_id_key",
			"on work_signature_registry (project_id, candidate_id, id)",
		)
		requireDoctorMigrationSQL(t, base,
			"unique (project_id, id)",
			"unique (project_id, candidate_id, id)",
		)
		for _, constraint := range []string{
			"site_fixes_doctor_finding_project_fk",
			"site_fixes_candidate_project_fk",
			"site_fixes_work_signature_project_fk",
			"site_fix_verifications_site_fix_project_fk",
			"site_change_applications_site_fix_project_fk",
			"site_fixes_supersedes_project_fk",
			"site_fixes_migration_batch_project_fk",
			"migration_ledger_batch_project_fk",
			"migration_review_items_batch_project_fk",
			"legacy_object_aliases_batch_project_fk",
			"migration_rollback_events_batch_project_fk",
			"migration_rollback_events_ledger_project_fk",
			"discovery_candidates_shadow_run_restrict_fk",
			"work_signature_registry_shadow_run_restrict_fk",
			"work_signature_registry_candidate_project_fk",
		} {
			requireDoctorMigrationSQL(t, add, constraint, "not valid")
			requireDoctorMigrationSQL(t, validate, "validate constraint "+constraint)
		}
		requireDoctorMigrationSQL(t, add,
			"foreign key (project_id, doctor_finding_id, finding_kind)",
			"references seo_doctor_findings(project_id, id, finding_kind)",
			"foreign key (project_id, candidate_id)",
			"references discovery_candidates(project_id, id)",
			"foreign key (project_id, candidate_id, work_signature_id)",
			"references work_signature_registry(project_id, candidate_id, id)",
			"foreign key (project_id, site_fix_id)",
			"references site_fixes(project_id, id)",
			"foreign key (project_id, migration_batch_id, migration_ledger_id)",
			"references migration_ledger(project_id, migration_batch_id, id)",
			"foreign key (project_id, candidate_id, supersedes_site_fix_id)",
			"references site_fixes(project_id, candidate_id, id)",
		)
		for _, constraint := range []string{
			"site_fixes_doctor_finding_project_fk",
			"site_fixes_candidate_project_fk",
			"site_fixes_work_signature_project_fk",
		} {
			pattern := regexp.MustCompile(`(?s)constraint ` + constraint + `\s+foreign key [^;]*on delete no action deferrable initially deferred not valid`)
			if !pattern.MatchString(add) {
				t.Errorf("%s must preserve provenance while deferring project erasure checks", constraint)
			}
		}
	})

	t.Run("revision graph cannot branch or self reference", func(t *testing.T) {
		requireDoctorMigrationSQL(t, base, "check (id <> supersedes_site_fix_id)")
		requireDoctorMigrationSQL(t, indexes,
			"create unique index if not exists uniq_site_fixes_root_candidate",
			"on site_fixes (project_id, candidate_id)",
			"where supersedes_site_fix_id is null",
			"create unique index if not exists uniq_site_fixes_superseded_predecessor",
			"on site_fixes (project_id, supersedes_site_fix_id)",
			"where supersedes_site_fix_id is not null",
		)
		revisionFK := regexp.MustCompile(`(?s)constraint site_fixes_supersedes_project_fk\s+foreign key \(project_id, candidate_id, supersedes_site_fix_id\)\s+references site_fixes\(project_id, candidate_id, id\)\s+on delete no action deferrable initially deferred not valid`).MatchString(add)
		if !revisionFK {
			t.Error("revision provenance must block direct predecessor deletion while deferring project erasure checks")
		}
		if strings.Contains(base, "unique (project_id, candidate_id, supersedes_site_fix_id)") {
			t.Error("nullable table UNIQUE does not enforce a single revision root")
		}
	})

	t.Run("migration files bound lock duration and order work", func(t *testing.T) {
		if strings.Contains(base, "validate constraint") {
			t.Error("0048 must not validate populated-table constraints")
		}
		if regexp.MustCompile(`(?m)^create (unique )?index`).MatchString(base) {
			t.Error("0048 must not build indexes")
		}
		for name, identitySQL := range map[string]string{
			"findings":   findingsIdentity,
			"candidates": candidatesIdentity,
			"registry":   registryIdentity,
		} {
			if !strings.Contains(identitySQL, "create unique index if not exists") {
				t.Errorf("%s identity index must be rerunnable", name)
			}
			requireDoctorMigrationSQL(t, identitySQL,
				"set local lock_timeout = '5s'",
				"set local statement_timeout = '30s'",
				"reset statement_timeout",
				"reset lock_timeout",
			)
			if strings.Contains(identitySQL, "concurrently") {
				t.Errorf("%s uses concurrent index creation that cannot recover an invalid index with this runner", name)
			}
		}
		if strings.Contains(add, "validate constraint") || strings.Contains(add, "drop constraint if exists seo_doctor_findings_status_check;") {
			t.Error("add phase must leave old protection in place and defer validation/swap")
		}
		requireDoctorMigrationSQL(t, add, "set local lock_timeout = '5s'")
		requireDoctorMigrationSQL(t, swap, "set local lock_timeout = '5s'")
		requireDoctorMigrationSQL(t, indexes, "create index if not exists", "reset lock_timeout")
	})

	t.Run("hard delete erases tenant audit without weakening append only", func(t *testing.T) {
		for _, table := range []string{"migration_batches", "migration_ledger", "migration_rollback_events"} {
			definition := regexp.MustCompile(`(?s)create table if not exists ` + table + `\s*\((.*?)\n\);`).FindStringSubmatch(base)
			if len(definition) != 2 || !strings.Contains(definition[1], "references projects(id) on delete cascade") {
				t.Errorf("%s must follow explicit project hard-delete cascade policy", table)
			}
		}
		requireDoctorMigrationSQL(t, base,
			"if tg_op = 'delete' and not exists (",
			"from projects where id = old.project_id",
			"then return old",
		)
		if strings.Contains(base, "ai_call_records(id) on delete set null") {
			t.Error("append-only verifications cannot use ON DELETE SET NULL")
		}
		requireDoctorMigrationSQL(t, add, "on delete cascade")
		requireDoctorMigrationSQL(t, add, "on delete no action deferrable initially deferred")
	})

	t.Run("ddl is recoverably rerunnable and metadata is consistent", func(t *testing.T) {
		triggerPattern := regexp.MustCompile(`(?m)^create trigger ([a-z0-9_]+)\s*\n(?:before|after) [^\n]+ on ([a-z0-9_]+)`)
		for _, match := range triggerPattern.FindAllStringSubmatch(base, -1) {
			want := "drop trigger if exists " + match[1] + " on " + match[2]
			if !strings.Contains(base, want) {
				t.Errorf("trigger %s must be dropped before recreation", match[1])
			}
		}
		requireDoctorMigrationSQL(t, base,
			"result = 'passed' and retry_classification = 'not_applicable'",
			"status = 'pending' and resolution_snapshot is null and resolved_by is null and resolved_at is null",
			"status in ('resolved','dismissed') and resolution_snapshot is not null and resolved_by is not null and resolved_at is not null",
		)
		requireDoctorMigrationSQL(t, add, "from pg_constraint", "if not exists")
	})
}
