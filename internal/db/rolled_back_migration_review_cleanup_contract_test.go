package db

import (
	"os"
	"strings"
	"testing"
)

func TestRolledBackMigrationReviewsCloseAsAuditHistory(t *testing.T) {
	queryRaw, err := os.ReadFile("queries/site_fixes.sql")
	if err != nil {
		t.Fatal(err)
	}
	serviceRaw, err := os.ReadFile("../growthwork/service.go")
	if err != nil {
		t.Fatal(err)
	}
	migrationRaw, err := os.ReadFile("../migrations/0081_close_rolled_back_migration_reviews.sql")
	if err != nil {
		t.Fatal(err)
	}
	query, service, migration := strings.ToLower(string(queryRaw)), strings.ToLower(string(serviceRaw)), strings.ToLower(string(migrationRaw))
	for _, want := range []string{"dismisspendingmigrationreviewitemsforbatch", "status = 'dismissed'", "migration_rolled_back"} {
		if !strings.Contains(query, want) {
			t.Fatalf("rollback review dismissal query missing %q", want)
		}
	}
	for _, want := range []string{"dismisspendingmigrationreviewitemsforbatch", `status == "rolled_back"`} {
		if !strings.Contains(service, want) {
			t.Fatalf("Growth rollback audit missing %q", want)
		}
	}
	for _, want := range []string{"join migration_batches", "batch.status = 'rolled_back'", "item.status = 'pending'", "migration_rolled_back"} {
		if !strings.Contains(migration, want) {
			t.Fatalf("rollback review backfill missing %q", want)
		}
	}
}
