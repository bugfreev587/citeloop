package db

import (
	"os"
	"strings"
	"testing"
)

func TestMigrationLedgerAcceptsEveryForwardOperationWithBoundedLocks(t *testing.T) {
	addRaw, err := os.ReadFile("../migrations/0069_migration_ledger_operations_add.sql")
	if err != nil {
		t.Fatalf("read migration ledger operation add migration: %v", err)
	}
	validateRaw, err := os.ReadFile("../migrations/0070_migration_ledger_operations_validate.sql")
	if err != nil {
		t.Fatalf("read migration ledger operation validation migration: %v", err)
	}
	add := strings.ToLower(string(addRaw))
	validate := strings.ToLower(string(validateRaw))
	for _, required := range []string{
		"set local lock_timeout = '5s'",
		"set local statement_timeout = '30s'",
		"drop constraint if exists migration_ledger_operation_check",
		"add constraint migration_ledger_operation_check check (operation in",
		"'create'", "'map'", "'decision_migrate'", "'repoint'", "'archive_duplicate'",
		"'authority_switch'", "'rollback'", "'tombstone'", "'mark_canonical_read_only'",
		"'migration_review'", "'migration_bucket_mutation'", "'validate_conservation'",
		"not valid",
	} {
		if !strings.Contains(add, required) {
			t.Errorf("add migration missing %q", required)
		}
	}
	if strings.Contains(add, "validate constraint") {
		t.Error("bounded add migration must not validate the populated ledger")
	}
	if !strings.Contains(validate, "validate constraint migration_ledger_operation_check") {
		t.Error("follow-on migration must validate the ledger operation constraint")
	}
}
