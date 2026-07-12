package db

import (
	"os"
	"strings"
	"testing"
)

func TestMigrationReviewQueueHasOwnerSLAAndOperationalQueries(t *testing.T) {
	migration, err := os.ReadFile("../migrations/0082_migration_review_operations.sql")
	if err != nil {
		t.Fatal(err)
	}
	migrationSQL := strings.ToLower(string(migration))
	for _, required := range []string{
		"add column if not exists internal_owner",
		"migration_ops",
		"add column if not exists due_at",
		"created_at + interval '7 days'",
		"alter column internal_owner set not null",
		"alter column due_at set not null",
		"idx_migration_review_items_operations",
	} {
		if !strings.Contains(migrationSQL, required) {
			t.Fatalf("migration review operations migration missing %q", required)
		}
	}

	queries, err := os.ReadFile("queries/site_fixes.sql")
	if err != nil {
		t.Fatal(err)
	}
	querySQL := strings.ToLower(string(queries))
	for _, required := range []string{
		"-- name: listoperationalmigrationreviewitems :many",
		"internal_owner = sqlc.narg(internal_owner)",
		"created_at <= now() - (sqlc.arg(min_age_seconds)",
		"sqlc.arg(overdue_only)::boolean = false or due_at <= now()",
		"-- name: getmigrationreviewitem :one",
		"-- name: resolvemigrationreviewitem :one",
	} {
		if !strings.Contains(querySQL, required) {
			t.Fatalf("migration review operational queries missing %q", required)
		}
	}
}
