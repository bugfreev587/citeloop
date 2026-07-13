package db

import (
	"os"
	"strings"
	"testing"
)

func TestGitHubPRReadinessSchemaAndQueries(t *testing.T) {
	migration := readGitHubPRReadinessContractFile(t, "../migrations/0084_github_pr_readiness.sql")
	queries := readGitHubPRReadinessContractFile(t, "queries/publisher_connections.sql")

	for _, required := range []string{
		"add column if not exists pr_readiness_status text not null default 'not_connected'",
		"add column if not exists pr_readiness_checked_at timestamptz",
		"add column if not exists pr_readiness_detail text",
	} {
		if !strings.Contains(migration, required) {
			t.Fatalf("GitHub PR readiness migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"pr_readiness_checked_at timestamptz not null",
		"pr_readiness_detail text not null",
	} {
		if strings.Contains(migration, forbidden) {
			t.Fatalf("GitHub PR readiness migration must keep optional evidence nullable; found %q", forbidden)
		}
	}

	const statusConstraint = "check (pr_readiness_status in ('not_connected', 'not_checked', 'ready', 'permission_missing', 'repository_unavailable', 'error'))"
	if !strings.Contains(migration, statusConstraint) {
		t.Fatalf("GitHub PR readiness migration must constrain exactly the six public statuses; missing %q", statusConstraint)
	}

	backfill := githubPRReadinessStatement(t, migration, "update publisher_connections")
	for _, required := range []string{
		"when kind = 'github_nextjs' and status = 'connected' and enabled then 'not_checked'",
		"else 'not_connected'",
	} {
		if !strings.Contains(backfill, required) {
			t.Fatalf("GitHub PR readiness backfill missing %q", required)
		}
	}
	if strings.Contains(backfill, "then 'ready'") {
		t.Fatal("GitHub PR readiness migration must never backfill ready")
	}

	getReadiness := githubPRReadinessQueryBlock(t, queries, "GetGitHubPRReadinessForProject")
	for _, required := range []string{
		"where project_id = sqlc.arg(project_id)",
		"kind = 'github_nextjs'",
		"is_default",
	} {
		if !strings.Contains(getReadiness, required) {
			t.Fatalf("GetGitHubPRReadinessForProject missing %q", required)
		}
	}

	setReadiness := githubPRReadinessQueryBlock(t, queries, "SetGitHubPRReadinessIfUnchanged")
	for _, required := range []string{
		"pr_readiness_status = sqlc.arg(pr_readiness_status)",
		"pr_readiness_checked_at = sqlc.narg(pr_readiness_checked_at)",
		"pr_readiness_detail = sqlc.narg(pr_readiness_detail)",
		"where id = sqlc.arg(connection_id)",
		"project_id = sqlc.arg(project_id)",
		"updated_at = sqlc.arg(expected_updated_at)",
		"returning *",
	} {
		if !strings.Contains(setReadiness, required) {
			t.Fatalf("SetGitHubPRReadinessIfUnchanged missing %q", required)
		}
	}

	for _, queryName := range []string{
		"UpsertDefaultPublisherConnection",
		"SetPublisherConnectionEnabled",
		"MarkPublisherConnectionVerified",
		"MarkPublisherConnectionError",
		"SetPublisherConnectionCredentialRef",
		"ClearPublisherConnectionCredentialRef",
		"UpsertPublisherCredential",
		"RevokePublisherCredentialForConnection",
	} {
		block := githubPRReadinessQueryBlock(t, queries, queryName)
		for _, required := range []string{
			"pr_readiness_status =",
			"pr_readiness_checked_at = null",
			"pr_readiness_detail = null",
			"updated_at = now()",
		} {
			if !strings.Contains(block, required) {
				t.Fatalf("%s must atomically invalidate GitHub PR readiness; missing %q", queryName, required)
			}
		}
	}

	for _, queryName := range []string{
		"UpsertDefaultPublisherConnection",
		"SetPublisherConnectionEnabled",
		"MarkPublisherConnectionVerified",
		"UpsertPublisherCredential",
		"RevokePublisherCredentialForConnection",
	} {
		block := githubPRReadinessQueryBlock(t, queries, queryName)
		for _, required := range []string{"'not_connected'", "'not_checked'"} {
			if !strings.Contains(block, required) {
				t.Fatalf("%s must derive GitHub PR readiness after mutation; missing %q", queryName, required)
			}
		}
	}

	for _, queryName := range []string{
		"MarkPublisherConnectionError",
		"SetPublisherConnectionCredentialRef",
		"ClearPublisherConnectionCredentialRef",
	} {
		block := githubPRReadinessQueryBlock(t, queries, queryName)
		if !strings.Contains(block, "pr_readiness_status = 'not_connected'") {
			t.Fatalf("%s must invalidate readiness as not_connected", queryName)
		}
	}

	for _, queryName := range []string{"UpsertPublisherCredential", "RevokePublisherCredentialForConnection"} {
		block := githubPRReadinessQueryBlock(t, queries, queryName)
		if !strings.Contains(block, "update publisher_connections") {
			t.Fatalf("%s must invalidate its publisher connection in the credential mutation statement", queryName)
		}
	}
}

func readGitHubPRReadinessContractFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.ToLower(strings.Join(strings.Fields(string(body)), " "))
}

func githubPRReadinessStatement(t *testing.T, source, prefix string) string {
	t.Helper()
	start := strings.Index(source, prefix)
	if start < 0 {
		t.Fatalf("SQL statement missing %q", prefix)
	}
	statement := source[start:]
	if end := strings.Index(statement, ";"); end >= 0 {
		statement = statement[:end]
	}
	return statement
}

func githubPRReadinessQueryBlock(t *testing.T, queries, name string) string {
	t.Helper()
	marker := "-- name: " + strings.ToLower(name) + " "
	start := strings.Index(queries, marker)
	if start < 0 {
		t.Fatalf("publisher connection queries missing %s", name)
	}
	block := queries[start:]
	if next := strings.Index(block[len(marker):], "-- name:"); next >= 0 {
		block = block[:len(marker)+next]
	}
	return block
}
