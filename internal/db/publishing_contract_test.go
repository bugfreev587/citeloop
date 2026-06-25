package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArticleSchemaHasPublishFailureAndVerificationFields(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)

	required := []string{
		"'publish_failed'",
		"last_publish_error",
		"publish_attempts",
		"next_publish_retry_at",
		"publish_phase",
		"resolved_slug",
		"publish_path",
		"canonical_url_verified_at",
		"last_publish_run_id",
	}
	for _, want := range required {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema missing %q", want)
		}
	}
}

func TestGenerationRunsAllowPublisherAndNotificationAgents(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)

	for _, want := range []string{"'publisher'", "'notification'"} {
		if !strings.Contains(schema, want) {
			t.Fatalf("generation_runs agent check missing %q", want)
		}
	}
}

func TestPublishStateUpgradeMigrationBackfillsExistingDatabases(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0004_article_publish_state_upgrade.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(raw)

	for _, want := range []string{
		"add column if not exists last_publish_error",
		"add column if not exists publish_attempts",
		"add column if not exists next_publish_retry_at",
		"add column if not exists publish_phase",
		"add column if not exists canonical_url_verified_at",
		"publish_failed",
		"pending_url_verification",
		"publisher",
		"notification",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("upgrade migration missing %q", want)
		}
	}
}

func TestPublishPhaseMigrationBackfillsAlreadyUpgradedDatabases(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0006_publish_phase_pending_url.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(raw)

	for _, want := range []string{
		"add column if not exists publish_phase",
		"pending_url_verification",
		"articles_status_check",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("publish phase migration missing %q", want)
		}
	}
}

func TestUniPostCanonicalURLMigrationRewritesLegacyDevURLs(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0020_unipost_prod_canonical_urls.sql"))
	if err != nil {
		t.Fatal(err)
	}
	migration := strings.ToLower(string(raw))

	for _, want := range []string{
		"dev\\.unipost\\.dev",
		"https://unipost.dev",
		"update articles",
		"canonical_url",
		"publish_result",
		"jsonb_set",
		"update publisher_connections",
		"base_url",
		"github_nextjs",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("unipost canonical migration missing %q", want)
		}
	}
}

func TestPublisherConnectionSchemaHasEnabledEligibility(t *testing.T) {
	initialRaw, err := os.ReadFile(filepath.Join("..", "migrations", "0011_publisher_connections.sql"))
	if err != nil {
		t.Fatal(err)
	}
	initial := strings.ToLower(string(initialRaw))
	if !strings.Contains(initial, "enabled boolean not null default false") {
		t.Fatal("publisher_connections schema must include disabled-by-default enabled flag")
	}

	migrationRaw, err := os.ReadFile(filepath.Join("..", "migrations", "0021_publisher_connection_enabled.sql"))
	if err != nil {
		t.Fatal(err)
	}
	migration := strings.ToLower(string(migrationRaw))
	for _, want := range []string{
		"alter table publisher_connections",
		"add column if not exists enabled boolean not null default false",
		"publisher_connections_project_idx",
		"enabled",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("publisher enabled migration missing %q", want)
		}
	}
}

func TestPublisherConnectionQueriesRespectEnabledEligibility(t *testing.T) {
	if !strings.Contains(getEnabledPublisherConnectionForProject, "enabled = true") {
		t.Fatal("GetEnabledPublisherConnectionForProject must only return enabled publisher connections")
	}
	if !strings.Contains(getEnabledPublisherConnectionForProject, "status = 'connected'") {
		t.Fatal("GetEnabledPublisherConnectionForProject must only return connected publisher connections")
	}
	if !strings.Contains(setPublisherConnectionEnabled, "enabled = $3") {
		t.Fatal("SetPublisherConnectionEnabled must update the enabled flag from the request")
	}
}

func TestPublishingQueriesRequireVerifiedCanonicalURL(t *testing.T) {
	if !strings.Contains(markPublished, "canonical_url_verified_at = now()") {
		t.Fatal("MarkPublished must set canonical_url_verified_at")
	}
	if !strings.Contains(markPublished, "resolved_slug = $4") || !strings.Contains(markPublished, "publish_path = $5") {
		t.Fatal("MarkPublished must persist resolved_slug and publish_path")
	}
	if !strings.Contains(preparePublishAttempt, "resolved_slug = $2") || !strings.Contains(preparePublishAttempt, "publish_path = $3") {
		t.Fatal("PreparePublishAttempt must persist resolved_slug and publish_path before GitHub write")
	}
	if !strings.Contains(recordPublishAttemptResult, "status = 'pending_url_verification'") || !strings.Contains(recordPublishAttemptResult, "publish_phase = 'pending_url_verification'") {
		t.Fatal("RecordPublishAttemptResult must move committed articles into pending_url_verification")
	}
	if strings.Contains(recordPublishAttemptResult, "canonical_url =") {
		t.Fatal("RecordPublishAttemptResult must not backfill canonical_url before URL verification")
	}
	if !strings.Contains(markPublishFailed, "canonical_url_verified_at = null") || !strings.Contains(markPublishFailed, "next_publish_retry_at = $3") {
		t.Fatal("MarkPublishFailed must clear canonical_url_verified_at")
	}
	if !strings.Contains(retryPublishArticle, "where id = $1") || !strings.Contains(retryPublishArticle, "and project_id = $2") || !strings.Contains(retryPublishArticle, "status = 'publish_failed'") {
		t.Fatal("RetryPublishArticle must be project-scoped and limited to publish_failed articles")
	}
	if !strings.Contains(retryPublishArticle, "next_publish_retry_at = now()") {
		t.Fatal("RetryPublishArticle must make publish_failed articles immediately due")
	}
	if !strings.Contains(selectUnlockableVariants, "c.canonical_url_verified_at is not null") {
		t.Fatal("SelectUnlockableVariants must require verified canonical URL")
	}
	for _, want := range []string{
		"status in ('approved','publish_failed')",
		"publish_result is not null",
		"status = 'pending_url_verification'",
		"status = 'published'",
		"canonical_url_verified_at is null",
	} {
		if !strings.Contains(selectPublishReconcileCandidates, want) {
			t.Fatalf("SelectPublishReconcileCandidates missing %q", want)
		}
	}
}
