package db

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDoctorSiteFixSchemaContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0048_doctor_site_fixes.sql"))
	if err != nil {
		t.Fatalf("read Doctor Site Fix migration: %v", err)
	}
	migration := strings.ToLower(string(raw))
	var followOn []string
	for _, pattern := range []string{"0049*.sql", "0050*.sql"} {
		matches, globErr := filepath.Glob(filepath.Join("..", "migrations", pattern))
		if globErr != nil {
			t.Fatalf("glob Doctor relationship migrations: %v", globErr)
		}
		followOn = append(followOn, matches...)
	}
	for _, name := range followOn {
		body, readErr := os.ReadFile(name)
		if readErr != nil {
			t.Fatalf("read %s: %v", name, readErr)
		}
		migration += "\n" + strings.ToLower(string(body))
	}

	requireSQL := func(scope, sql string, required ...string) {
		t.Helper()
		for _, want := range required {
			if !strings.Contains(sql, want) {
				t.Fatalf("%s missing %q", scope, want)
			}
		}
	}
	tableDefinition := func(name string) string {
		t.Helper()
		pattern := regexp.MustCompile(`(?s)create table if not exists ` + regexp.QuoteMeta(name) + `\s*\((.*?)\n\);`)
		match := pattern.FindStringSubmatch(migration)
		if len(match) != 2 {
			t.Fatalf("migration must define %s", name)
		}
		return match[1]
	}

	siteFixes := tableDefinition("site_fixes")
	requireSQL(t.Name()+" site_fixes", siteFixes,
		"doctor_finding_id uuid not null references seo_doctor_findings(id) on delete restrict",
		"candidate_id uuid not null references discovery_candidates(id) on delete restrict",
		"work_signature_id uuid not null",
		"supersedes_site_fix_id uuid references site_fixes(id) on delete restrict",
		"references work_signature_registry(id) on delete restrict deferrable initially deferred",
		"check (status in (\n    'proposed','approved','preparing','ready_to_apply','applying',\n    'awaiting_deploy','verifying','verified','failed_retryable',\n    'reopened','failed_terminal','superseded','migration_rolled_back'\n  ))",
		"finding_kind text not null check (finding_kind in ('broken','optimization'))",
		"jsonb_typeof(target_urls) = 'array'",
		"jsonb_typeof(evidence_snapshot) = 'object'",
		"jsonb_typeof(proposed_fix) = 'object'",
		"jsonb_typeof(acceptance_tests) = 'array'",
		"jsonb_typeof(verification_snapshot) = 'object'",
	)
	for _, forbiddenColumn := range []string{"seo_opportunity_id", "content_action_id"} {
		columnPattern := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(forbiddenColumn) + `\s+`)
		if columnPattern.MatchString(siteFixes) {
			t.Fatalf("site_fixes must not canonically own legacy column %q", forbiddenColumn)
		}
	}

	verifications := tableDefinition("site_fix_verifications")
	requireSQL(t.Name()+" site_fix_verifications", verifications,
		"site_fix_id uuid not null references site_fixes(id) on delete restrict",
		"attempt_number int not null",
		"evidence_read jsonb not null",
		"jsonb_typeof(evidence_read) = 'object'",
		"acceptance_results jsonb not null",
		"jsonb_typeof(acceptance_results) = 'array'",
		"ai_call_id uuid references ai_call_records(id) on delete cascade",
		"result text not null check (result in ('passed','failed','inconclusive','error'))",
		"retry_classification text not null check (retry_classification in ('not_applicable','retryable','retry_exhausted','terminal'))",
		"unique (site_fix_id, attempt_number)",
	)
	requireSQL(t.Name()+" append-only verifications", migration,
		"create trigger site_fix_verifications_immutable",
		"before update or delete on site_fix_verifications",
	)

	batches := tableDefinition("migration_batches")
	requireSQL(t.Name()+" migration_batches", batches,
		"project_id uuid not null references projects(id) on delete cascade",
		"source_snapshot jsonb not null",
		"jsonb_typeof(source_snapshot) = 'object'",
		"result_snapshot jsonb not null",
		"jsonb_typeof(result_snapshot) = 'object'",
		"source_count int not null",
		"migrated_count int not null",
		"archived_duplicate_count int not null",
		"review_count int not null",
		"writer_authority_before text not null",
		"writer_authority_after text not null",
	)
	ledger := tableDefinition("migration_ledger")
	requireSQL(t.Name()+" migration_ledger", ledger,
		"migration_batch_id uuid not null references migration_batches(id) on delete cascade",
		"before_hash text not null",
		"after_hash text not null",
		"before_snapshot jsonb not null",
		"after_snapshot jsonb not null",
		"inverse_operation jsonb not null",
		"jsonb_typeof(before_snapshot) = 'object'",
		"jsonb_typeof(after_snapshot) = 'object'",
		"jsonb_typeof(inverse_operation) = 'object'",
	)
	requireSQL(t.Name()+" immutable migration audit", migration,
		"create trigger migration_batches_immutable",
		"before update or delete on migration_batches",
		"create trigger migration_ledger_immutable",
		"before update or delete on migration_ledger",
	)
	requireSQL(t.Name()+" migration support", migration,
		"create table if not exists migration_review_items",
		"create table if not exists legacy_object_aliases",
		"create table if not exists product_writer_authority",
		"writer_authority text not null default 'legacy' check (writer_authority in ('legacy','canonical'))",
		"write_fenced boolean not null default false",
		"insert into product_writer_authority",
		"select p.id, supported.product, 'legacy', false",
	)

	requireSQL(t.Name()+" application source union", migration,
		"alter column content_action_id drop not null",
		"add column if not exists site_fix_id uuid references site_fixes(id) on delete restrict",
		"constraint site_change_applications_exactly_one_source",
		"check (num_nonnulls(site_fix_id, content_action_id) = 1) not valid",
		"validate constraint site_change_applications_exactly_one_source",
		"constraint site_change_applications_kind_source_consistency",
		"validate constraint site_change_applications_kind_source_consistency",
		"create index if not exists idx_active_site_change_application_content_action",
		"where content_action_id is not null and status in (",
		"create unique index if not exists uniq_active_site_change_application_site_fix",
		"where site_fix_id is not null and status in (",
	)
	if strings.Contains(migration, "drop index if exists uniq_active_site_change_application") {
		t.Fatal("legacy opportunity-key application uniqueness must remain until compatibility writers are removed")
	}
	requireSQL(t.Name()+" rollback source union", migration,
		"alter table rollback_records",
		"add column if not exists site_fix_id uuid references site_fixes(id) on delete restrict",
		"constraint rollback_records_at_most_one_source",
		"num_nonnulls(action_id, site_fix_id) <= 1",
	)

	requireSQL(t.Name()+" Doctor and production discovery", migration,
		"add column if not exists finding_kind text not null default 'broken'",
		"finding_kind in ('broken','optimization','healthy')",
		"add column if not exists healthy_coverage jsonb not null default '[]'::jsonb",
		"jsonb_typeof(healthy_coverage) = 'array'",
		"trigger in ('onboarding','manual','weekly','post_publish','migration')",
		"mode in ('shadow','canonical','migration')",
		"constraint discovery_candidates_shadow_run_restrict_fk",
		"foreign key (shadow_run_id) references discovery_shadow_runs(id)",
		"on delete restrict not valid",
		"constraint work_signature_registry_shadow_run_restrict_fk",
	)
	for _, statement := range strings.Split(migration, ";") {
		if strings.Contains(statement, "alter table seo_doctor_findings") &&
			regexp.MustCompile(`\bsite_fix_id\b`).MatchString(statement) {
			t.Fatal("seo_doctor_findings must not have a current Site Fix pointer")
		}
	}
}

func TestDoctorSiteFixSchemaContractReviewGaps(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0048_doctor_site_fixes.sql"))
	if err != nil {
		t.Fatalf("read Doctor Site Fix migration: %v", err)
	}
	migration := strings.ToLower(string(raw))
	var followOn []string
	for _, pattern := range []string{"0049*.sql", "0050*.sql"} {
		matches, globErr := filepath.Glob(filepath.Join("..", "migrations", pattern))
		if globErr != nil {
			t.Fatalf("glob Doctor relationship migrations: %v", globErr)
		}
		followOn = append(followOn, matches...)
	}
	for _, name := range followOn {
		body, readErr := os.ReadFile(name)
		if readErr != nil {
			t.Fatalf("read %s: %v", name, readErr)
		}
		migration += "\n" + strings.ToLower(string(body))
	}
	tableDefinition := func(t *testing.T, name string) string {
		t.Helper()
		pattern := regexp.MustCompile(`(?s)create table if not exists ` + regexp.QuoteMeta(name) + `\s*\((.*?)\n\);`)
		match := pattern.FindStringSubmatch(migration)
		if len(match) != 2 {
			t.Errorf("migration must define %s", name)
			return ""
		}
		return match[1]
	}
	requireSQL := func(t *testing.T, sql string, required ...string) {
		t.Helper()
		for _, want := range required {
			if !strings.Contains(sql, want) {
				t.Errorf("migration missing %q", want)
			}
		}
	}

	t.Run("append-only rollback lifecycle audit", func(t *testing.T) {
		ledger := tableDefinition(t, "migration_ledger")
		requireSQL(t, ledger,
			"cutover_point text not null",
			"rollback_eligibility text not null",
			"'map'",
			"'decision_migrate'",
			"inverse_operation_version text not null",
			"inverse_operation jsonb not null",
		)
		rollbackEvents := tableDefinition(t, "migration_rollback_events")
		requireSQL(t, rollbackEvents,
			"migration_batch_id uuid not null references migration_batches(id) on delete cascade",
			"rollback_eligibility text not null",
			"rollback_blocked_forward_fix_required",
			"rollback_completed",
			"rolled_back_at timestamptz",
		)
		requireSQL(t, migration,
			"create trigger migration_rollback_events_immutable",
			"before update or delete on migration_rollback_events",
		)
	})

	t.Run("review memory migration aliases remain generic", func(t *testing.T) {
		aliases := tableDefinition(t, "legacy_object_aliases")
		if strings.Contains(aliases, "canonical_object_type in") {
			t.Error("legacy_object_aliases must not constrain generic canonical object types")
		}
		requireSQL(t, aliases, "canonical_object_type text not null")
		requireSQL(t, migration, "'decision_migrate'")
	})

	t.Run("future projects receive legacy writer authority", func(t *testing.T) {
		requireSQL(t, migration,
			"create or replace function seed_project_writer_authority()",
			"insert into product_writer_authority",
			"(new.id, 'doctor', 'legacy', false)",
			"(new.id, 'opportunities', 'legacy', false)",
			"on conflict (project_id, product) do nothing",
			"create trigger projects_seed_writer_authority",
			"after insert on projects",
			"execute function seed_project_writer_authority()",
		)
	})

	t.Run("candidate provenance cannot cascade", func(t *testing.T) {
		candidateFK := regexp.MustCompile(`(?s)constraint work_signature_registry_candidate_project_fk\s+foreign key \(project_id, candidate_id\)\s+references discovery_candidates\(project_id, id\)\s+on delete restrict`).MatchString(migration)
		if !candidateFK {
			t.Error("work signature candidate provenance must be project-consistent and ON DELETE RESTRICT")
		}
		requireSQL(t, migration, "drop constraint if exists work_signature_registry_candidate_id_fkey")
	})

	t.Run("shared migration products and writer products are stable", func(t *testing.T) {
		batches := tableDefinition(t, "migration_batches")
		authority := tableDefinition(t, "product_writer_authority")
		requireSQL(t, batches, "product in ('doctor','opportunities','shared')")
		requireSQL(t, authority, "product in ('doctor','opportunities')")
		if strings.Contains(authority, "'shared'") {
			t.Error("shared migration infrastructure must not create a shared product writer")
		}
		requireSQL(t, migration,
			"cross join (values ('doctor'), ('opportunities')) as supported(product)",
		)
	})
}
