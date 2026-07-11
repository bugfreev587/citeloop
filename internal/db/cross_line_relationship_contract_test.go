package db

import (
	"os"
	"path/filepath"
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
		"legacy growth creation is disabled",
		"legacy growth evidence/work identity is read-only",
		"create table if not exists growth_opportunity_work_aliases",
		"disposition in ('canonicalized','duplicate','doctor_merge','rolled_back')",
		"before insert or update on seo_opportunities",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("canonical Growth cutover missing %q", want)
		}
	}
	if strings.Contains(sql, "update product_writer_authority\nset writer_authority = 'canonical'") {
		t.Fatal("schema migration must not expose canonical authority before active legacy Growth conservation")
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

func TestCanonicalGrowthLegacyLookupIndexUsesRecoverableConcurrentMigration(t *testing.T) {
	baseRaw, err := os.ReadFile("../migrations/0063_canonical_growth_writer_cutover.sql")
	if err != nil {
		t.Fatal(err)
	}
	indexRaw, err := os.ReadFile(filepath.Join("..", "migrations", "0064_canonical_growth_legacy_index.sql"))
	if err != nil {
		t.Fatal(err)
	}
	base := strings.ToLower(string(baseRaw))
	index := strings.ToLower(string(indexRaw))
	if strings.Contains(base, "idx_seo_opportunities_legacy_growth_migration") {
		t.Fatal("populated seo_opportunities index must not be built in transactional cutover migration")
	}
	for _, want := range []string{
		"-- citeloop:migration-mode=nontransactional",
		"-- citeloop:index=idx_seo_opportunities_legacy_growth_migration",
		"create index concurrently if not exists idx_seo_opportunities_legacy_growth_migration",
		"on seo_opportunities (project_id, created_at, id)",
		"where canonical_growth = false",
		"status in ('open','accepted','converted','snoozed','watching')",
	} {
		if !strings.Contains(index, want) {
			t.Errorf("canonical Growth index migration missing %q", want)
		}
	}
	if got := strings.Count(index, "create "); got != 1 {
		t.Fatalf("concurrent index migration must contain one CREATE statement, got %d", got)
	}
}

func TestCanonicalGrowthPopulatedTableDDLIsSplitAndLockBounded(t *testing.T) {
	baseRaw, err := os.ReadFile("../migrations/0063_canonical_growth_writer_cutover.sql")
	if err != nil {
		t.Fatal(err)
	}
	identityRaw, err := os.ReadFile("../migrations/0062_z_seo_opportunities_project_identity.sql")
	if err != nil {
		t.Fatal(err)
	}
	addRaw, err := os.ReadFile("../migrations/0067_growth_cutover_status_constraints_add.sql")
	if err != nil {
		t.Fatal(err)
	}
	validateRaw, err := os.ReadFile("../migrations/0068_growth_cutover_status_constraints_validate.sql")
	if err != nil {
		t.Fatal(err)
	}
	base := strings.ToLower(string(baseRaw))
	identity := strings.ToLower(string(identityRaw))
	add := strings.ToLower(string(addRaw))
	validate := strings.ToLower(string(validateRaw))
	for _, forbidden := range []string{
		"create unique index if not exists seo_opportunities_project_id_id_key",
		"drop constraint if exists discovery_candidates_status_check",
		"drop constraint if exists work_signature_registry_status_check",
	} {
		if strings.Contains(base, forbidden) {
			t.Errorf("transactional 0063 retains populated-table DDL %q", forbidden)
		}
	}
	for _, want := range []string{
		"-- citeloop:migration-mode=nontransactional",
		"-- citeloop:index=seo_opportunities_project_id_id_key",
		"create unique index concurrently if not exists seo_opportunities_project_id_id_key",
		"on seo_opportunities (project_id, id)",
	} {
		if !strings.Contains(identity, want) {
			t.Errorf("project identity migration missing %q", want)
		}
	}
	for _, want := range []string{
		"set local lock_timeout = '5s'",
		"set local statement_timeout = '30s'",
		"drop constraint if exists discovery_candidates_status_check",
		"add constraint discovery_candidates_status_check",
		"drop constraint if exists work_signature_registry_status_check",
		"add constraint work_signature_registry_status_check",
		"not valid",
		"reset statement_timeout",
		"reset lock_timeout",
	} {
		if !strings.Contains(add, want) {
			t.Errorf("status constraint add migration missing %q", want)
		}
	}
	if strings.Contains(add, "validate constraint") {
		t.Fatal("status constraint add migration must not scan populated tables")
	}
	for _, want := range []string{
		"set local lock_timeout = '5s'",
		"set local statement_timeout = '30s'",
		"validate constraint discovery_candidates_status_check",
		"validate constraint work_signature_registry_status_check",
		"reset statement_timeout",
		"reset lock_timeout",
	} {
		if !strings.Contains(validate, want) {
			t.Errorf("status constraint validation migration missing %q", want)
		}
	}
	if strings.Contains(validate, "drop constraint") || strings.Contains(validate, "add constraint") {
		t.Fatal("status constraint validation migration must not replace constraints")
	}
}
