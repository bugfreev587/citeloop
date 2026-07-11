package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationLedgerEventIdentityCutover(t *testing.T) {
	indexRaw, err := os.ReadFile(filepath.Join("..", "migrations", "0071_migration_ledger_effect_identity_index.sql"))
	if err != nil {
		t.Fatalf("read concurrent effect identity migration: %v", err)
	}
	cutoverRaw, err := os.ReadFile(filepath.Join("..", "migrations", "0072_migration_ledger_effect_identity_cutover.sql"))
	if err != nil {
		t.Fatalf("read bounded effect identity cutover: %v", err)
	}

	indexSQL := strings.ToLower(string(indexRaw))
	for _, want := range []string{
		"-- citeloop:migration-mode=nontransactional",
		"-- citeloop:index=migration_ledger_effect_identity_key",
		"create unique index concurrently if not exists migration_ledger_effect_identity_key",
		"migration_batch_id, source_object_type, source_object_id, operation, canonical_object_type, canonical_object_id",
		"nulls not distinct",
	} {
		if !strings.Contains(indexSQL, want) {
			t.Fatalf("concurrent effect identity migration missing %q", want)
		}
	}

	cutoverSQL := strings.ToLower(string(cutoverRaw))
	for _, want := range []string{
		"set local lock_timeout",
		"set local statement_timeout",
		"pg_get_constraintdef",
		"unique (migration_batch_id, source_object_type, source_object_id, operation)",
		"drop constraint",
		"add constraint migration_ledger_effect_identity_key unique using index migration_ledger_effect_identity_key",
	} {
		if !strings.Contains(cutoverSQL, want) {
			t.Fatalf("bounded effect identity cutover missing %q", want)
		}
	}
	if strings.Contains(cutoverSQL, "drop index") {
		t.Fatal("bounded cutover must retain the concurrently-built replacement index")
	}
}

func TestMigrationLedgerFindingKeepsTruthfulLegacySourceIdentity(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "sitefix", "migration_postgres.go"))
	if err != nil {
		t.Fatalf("read migration store: %v", err)
	}
	source := strings.ReplaceAll(string(raw), " ", "")
	if strings.Contains(source, `sourceType+":finding"`) {
		t.Fatal("Doctor finding ledger must not invent a synthetic legacy source type")
	}
	if !strings.Contains(source, `appendLedger(sourceType,sourceID,"seo_doctor_finding"`) {
		t.Fatal("Doctor finding ledger must use the original legacy source identity")
	}
}
