package db

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readSiteFixQueryContracts(t *testing.T) (string, string) {
	t.Helper()
	siteFixes, err := os.ReadFile(filepath.Join("queries", "site_fixes.sql"))
	if err != nil {
		t.Fatalf("read canonical Site Fix queries: %v", err)
	}
	discovery, err := os.ReadFile(filepath.Join("queries", "discovery.sql"))
	if err != nil {
		t.Fatalf("read discovery queries: %v", err)
	}
	return strings.ToLower(string(siteFixes)), strings.ToLower(string(discovery))
}

func namedSQL(t *testing.T, all, name string) string {
	t.Helper()
	pattern := regexp.MustCompile(`(?s)-- name:\s*` + regexp.QuoteMeta(strings.ToLower(name)) + `\s+:[a-z]+\s*\n(.*?)(?:\n-- name:|\z)`)
	match := pattern.FindStringSubmatch(all)
	if len(match) != 2 {
		t.Fatalf("missing sqlc query %s", name)
	}
	return match[1]
}

func requireQuerySQL(t *testing.T, sql string, required ...string) {
	t.Helper()
	for _, want := range required {
		if !strings.Contains(sql, strings.ToLower(want)) {
			t.Errorf("query missing %q", want)
		}
	}
}

func TestCanonicalSiteFixQueries(t *testing.T) {
	siteFixes, discovery := readSiteFixQueryContracts(t)

	for _, name := range []string{
		"CreateCanonicalSiteFix",
		"GetCanonicalSiteFix",
		"ListCanonicalSiteFixes",
		"LockCanonicalSiteFixForUpdate",
		"ApproveCanonicalSiteFix",
		"MarkCanonicalSiteFixPreparing",
		"MarkCanonicalSiteFixReadyToApply",
		"ClaimCanonicalSiteFixApplying",
		"MarkCanonicalSiteFixApplied",
		"MarkCanonicalSiteFixAwaitingDeploy",
		"MarkCanonicalSiteFixVerifying",
		"MarkCanonicalSiteFixVerified",
		"MarkCanonicalSiteFixRetryable",
		"ReopenCanonicalSiteFix",
		"TerminalizeCanonicalSiteFix",
		"SupersedeCanonicalSiteFix",
		"AppendCanonicalSiteFixVerification",
		"ListCanonicalSiteFixVerifications",
		"CreateCanonicalSiteFixApplication",
		"RepointApplicationToCanonicalSiteFix",
		"RestoreApplicationToLegacyContentAction",
		"GetProductWriterAuthority",
		"LockProductWriterAuthority",
		"FenceProductWriterAuthority",
		"SwitchProductWriterAuthority",
		"ReleaseProductWriterFence",
		"CreateMigrationBatch",
		"AppendMigrationLedger",
		"CreateMigrationReviewItem",
		"ResolveMigrationReviewItem",
		"AppendMigrationRollbackEvent",
		"CreateLegacyObjectAlias",
		"ResolveLegacyObjectAlias",
		"MarkCanonicalSiteFixMigrationRolledBack",
	} {
		namedSQL(t, siteFixes, name)
	}

	for _, name := range []string{
		"GetEnforcedWorkSignatureForReservedWork",
		"MarkCanonicalWorkSignatureMigrationRolledBack",
	} {
		namedSQL(t, discovery, name)
	}
	migrationSignature := namedSQL(t, discovery, "MarkCanonicalWorkSignatureMigrationRolledBack")
	requireQuerySQL(t, migrationSignature,
		"expected_keys as materialized",
		"locked_buckets as materialized",
		"order by b.bucket_key",
		"for update of b",
		"locked_signature as materialized",
		"for update of w",
		"update work_conflict_buckets",
		"bucket_version = bucket_version + 1",
		"select count(*) from locked_buckets",
		"select count(*) from expected_keys",
		"jsonb_array_length(s.conflict_bucket_keys) > 0",
		"w.status in ('reserved','proposed','approved','preparing','executing','awaiting_deploy','failed_retryable')",
		"active = false",
	)

	create := namedSQL(t, siteFixes, "CreateCanonicalSiteFix")
	requireQuerySQL(t, create,
		"insert into site_fixes",
		"doctor_finding_id",
		"candidate_id",
		"work_signature_id",
		"supersedes_site_fix_id",
		"returning *",
	)
	canonicalOwnerColumn := regexp.MustCompile(`(?m)^\s*(seo_opportunity_id|content_action_id)\s*,`)
	if canonicalOwnerColumn.MatchString(create) {
		t.Error("canonical Site Fix creation must not dual-write Opportunity or Content Action ownership")
	}

	for _, name := range []string{"GetCanonicalSiteFix", "ListCanonicalSiteFixes", "LockCanonicalSiteFixForUpdate"} {
		requireQuerySQL(t, namedSQL(t, siteFixes, name), "project_id")
	}

	appendVerification := namedSQL(t, siteFixes, "AppendCanonicalSiteFixVerification")
	requireQuerySQL(t, appendVerification,
		"insert into site_fix_verifications",
		"project_id",
		"site_fix_id",
		"attempt_number",
		"evidence_read",
		"acceptance_results",
		"ai_call_id",
		"retry_classification",
		"returning *",
	)
	if strings.Contains(appendVerification, " update ") || strings.Contains(appendVerification, "on conflict") {
		t.Error("verification attempts must be append-only")
	}

	createApplication := namedSQL(t, siteFixes, "CreateCanonicalSiteFixApplication")
	requireQuerySQL(t, createApplication,
		"expected_keys as materialized",
		"locked_buckets as materialized",
		"order by b.bucket_key",
		"for update of b",
		"locked_work as materialized",
		"for update of sf, w",
		"sf.status in ('ready_to_apply','applying')",
		"w.status = 'executing'",
		"jsonb_array_length(s.conflict_bucket_keys) > 0",
		"update work_conflict_buckets",
		"bucket_version = bucket_version + 1",
		"insert into site_change_applications",
		"site_fix_id",
		"content_action_id",
		"null",
		"application_kind",
		"'site_fix'",
	)
	if strings.Contains(create, "update work_conflict_buckets") {
		t.Error("Site Fix creation gets its single bucket bump from atomic reservation, not from CreateCanonicalSiteFix")
	}

	repoint := namedSQL(t, siteFixes, "RepointApplicationToCanonicalSiteFix")
	requireQuerySQL(t, repoint,
		"expected_keys as materialized",
		"locked_buckets as materialized",
		"order by b.bucket_key",
		"for update of b",
		"locked_application as materialized",
		"for update of a",
		"locked_content_action as materialized",
		"for update of ca",
		"locked_work as materialized",
		"for update of sf, w",
		"jsonb_array_length(s.conflict_bucket_keys) > 0",
		"update work_conflict_buckets",
		"bucket_version = bucket_version + 1",
		"set site_fix_id = sqlc.arg(site_fix_id)",
		"content_action_id = null",
		"a.project_id = sqlc.arg(project_id)",
		"a.content_action_id = sqlc.arg(content_action_id)",
	)
	restore := namedSQL(t, siteFixes, "RestoreApplicationToLegacyContentAction")
	requireQuerySQL(t, restore,
		"expected_keys as materialized",
		"locked_buckets as materialized",
		"order by b.bucket_key",
		"for update of b",
		"locked_application as materialized",
		"for update of a",
		"locked_content_action as materialized",
		"from content_actions ca",
		"ca.project_id = sqlc.arg(project_id)",
		"ca.id = sqlc.arg(content_action_id)",
		"for update of ca",
		"locked_work as materialized",
		"for update of sf, w",
		"jsonb_array_length(s.conflict_bucket_keys) > 0",
		"update work_conflict_buckets",
		"bucket_version = bucket_version + 1",
		"set content_action_id = sqlc.arg(content_action_id)",
		"site_fix_id = null",
		"a.project_id = sqlc.arg(project_id)",
		"a.site_fix_id = sqlc.arg(site_fix_id)",
	)

	fence := namedSQL(t, siteFixes, "FenceProductWriterAuthority")
	requireQuerySQL(t, fence,
		"write_fenced = true",
		"fence_token = sqlc.arg(fence_token)",
		"where project_id = sqlc.arg(project_id)",
		"and product = sqlc.arg(product)",
		"and write_fenced = false",
	)
	switchAuthority := namedSQL(t, siteFixes, "SwitchProductWriterAuthority")
	requireQuerySQL(t, switchAuthority,
		"writer_authority = sqlc.arg(writer_authority)",
		"and write_fenced = true",
		"and fence_token = sqlc.arg(fence_token)",
		"and writer_authority = sqlc.arg(expected_writer_authority)",
	)

	resolveAlias := namedSQL(t, siteFixes, "ResolveLegacyObjectAlias")
	requireQuerySQL(t, resolveAlias,
		"project_id = sqlc.arg(project_id)",
		"legacy_object_type = sqlc.arg(legacy_object_type)",
		"legacy_object_id = sqlc.arg(legacy_object_id)",
		"alias_state = 'active'",
	)

	for _, name := range []string{
		"ApproveCanonicalSiteFix",
		"MarkCanonicalSiteFixPreparing",
		"MarkCanonicalSiteFixReadyToApply",
		"ClaimCanonicalSiteFixApplying",
		"MarkCanonicalSiteFixApplied",
		"MarkCanonicalSiteFixAwaitingDeploy",
		"MarkCanonicalSiteFixVerifying",
		"MarkCanonicalSiteFixVerified",
		"MarkCanonicalSiteFixRetryable",
		"ReopenCanonicalSiteFix",
		"TerminalizeCanonicalSiteFix",
		"SupersedeCanonicalSiteFix",
		"MarkCanonicalSiteFixMigrationRolledBack",
	} {
		query := namedSQL(t, siteFixes, name)
		requireQuerySQL(t, query,
			"project_id = sqlc.arg(project_id)",
			"expected_keys as materialized",
			"locked_buckets as materialized",
			"order by b.bucket_key",
			"for update of b",
			"locked_work as materialized",
			"expected_fix_status",
			"expected_signature_status",
			"w.conflict_bucket_keys = e.conflict_bucket_keys",
			"jsonb_array_length(e.conflict_bucket_keys) > 0",
			"exists (select 1 from locked_work)",
			"update work_conflict_buckets",
			"bucket_version = bucket_version + 1",
			"update work_signature_registry",
			"select count(*) from locked_buckets",
			"select count(*) from expected_keys",
		)
		if strings.Index(query, "locked_buckets as materialized") > strings.Index(query, "update work_conflict_buckets") {
			t.Errorf("%s must lock buckets in stable order before bumping them", name)
		}
		if strings.Index(query, "locked_buckets as materialized") > strings.Index(query, "locked_work as materialized") ||
			strings.Index(query, "locked_work as materialized") > strings.Index(query, "update work_conflict_buckets") {
			t.Errorf("%s must re-lock and revalidate current work after buckets and before bump", name)
		}
		applicationLockDependsOnBuckets := regexp.MustCompile(`(?s)locked_application as materialized\s*\(.*?select count\(\*\) from locked_buckets.*?select count\(\*\) from expected_keys.*?for update of a`).MatchString(query)
		if !applicationLockDependsOnBuckets {
			t.Errorf("%s application lock must explicitly depend on complete stable bucket locks", name)
		}
		if strings.Contains(query, "(select count(*) from bumped) >= 0") {
			t.Errorf("%s must not allow partial or missing conflict-bucket bumps", name)
		}
		if got := strings.Count(query, "jsonb_array_length(e.conflict_bucket_keys) > 0"); got != 1 {
			t.Errorf("%s must have exactly one non-empty conflict-bucket guard, got %d", name, got)
		}
	}
}

func TestCanonicalSiteFixTransitionStatePairs(t *testing.T) {
	siteFixes, _ := readSiteFixQueryContracts(t)

	pairs := map[string][]string{
		"ApproveCanonicalSiteFix": {
			"sf.status = 'proposed'",
			"w.status in ('reserved','proposed')",
		},
		"MarkCanonicalSiteFixPreparing": {
			"sf.status = 'approved'",
			"w.status = 'approved'",
		},
		"MarkCanonicalSiteFixReadyToApply": {
			"sf.status = 'preparing'",
			"w.status = 'preparing'",
		},
		"ClaimCanonicalSiteFixApplying": {
			"sf.status = 'ready_to_apply'",
			"w.status = 'executing'",
		},
		"MarkCanonicalSiteFixApplied": {
			"sf.status = 'applying'",
			"w.status = 'executing'",
			"a.status = 'github_pr_merged'",
		},
		"MarkCanonicalSiteFixAwaitingDeploy": {
			"sf.status = 'applying'",
			"sf.applied_at is not null",
			"w.status = 'executing'",
			"a.id = sqlc.arg(application_id)",
			"a.status = 'deployment_pending'",
			"a.site_fix_id = sf.id",
			"a.content_action_id is null",
		},
		"MarkCanonicalSiteFixVerifying": {
			"sf.status = 'awaiting_deploy'",
			"w.status = 'awaiting_deploy'",
		},
		"MarkCanonicalSiteFixVerified": {
			"(sf.status = 'verifying' and w.status = 'verifying')",
			"(sf.status = 'failed_retryable' and w.status = 'failed_retryable')",
			"(sf.status = 'reopened' and w.status = 'reopened')",
		},
		"MarkCanonicalSiteFixRetryable": {
			"(sf.status = 'verifying' and w.status = 'verifying')",
			"(sf.status = 'reopened' and w.status = 'reopened')",
		},
		"ReopenCanonicalSiteFix": {
			"sf.status = 'failed_retryable'",
			"w.status = 'failed_retryable'",
		},
		"TerminalizeCanonicalSiteFix": {
			"(sf.status = 'verifying' and w.status = 'verifying')",
			"(sf.status = 'failed_retryable' and w.status = 'failed_retryable')",
			"(sf.status = 'reopened' and w.status = 'reopened')",
		},
		"SupersedeCanonicalSiteFix": {
			"(sf.status = 'proposed' and w.status in ('reserved','proposed'))",
			"(sf.status = 'approved' and w.status = 'approved')",
			"(sf.status in ('ready_to_apply','applying') and w.status = 'executing')",
			"(sf.status = 'reopened' and w.status = 'reopened')",
		},
		"MarkCanonicalSiteFixMigrationRolledBack": {
			"(sf.status = 'proposed' and w.status in ('reserved','proposed'))",
			"(sf.status = 'approved' and w.status = 'approved')",
			"(sf.status in ('ready_to_apply','applying') and w.status = 'executing')",
			"(sf.status = 'failed_retryable' and w.status = 'failed_retryable')",
		},
	}
	for name, required := range pairs {
		requireQuerySQL(t, namedSQL(t, siteFixes, name), required...)
	}

	applied := namedSQL(t, siteFixes, "MarkCanonicalSiteFixApplied")
	for _, forbidden := range []string{"'ready_for_pr'", "'creating_pr'", "'github_pr_open'", "'manual_apply_required'"} {
		if strings.Contains(applied, forbidden) {
			t.Errorf("MarkCanonicalSiteFixApplied must not accept unapplied application state %s", forbidden)
		}
	}
}

func TestDoctorVerificationStopsAtVerified(t *testing.T) {
	siteFixes, _ := readSiteFixQueryContracts(t)
	verifySQL := namedSQL(t, siteFixes, "MarkCanonicalSiteFixVerified")
	if strings.Contains(verifySQL, "content_actions") || strings.Contains(verifySQL, "measuring") {
		t.Fatal("Doctor verification must stop at canonical Site Fix verified")
	}
	requireQuerySQL(t, verifySQL,
		"update site_change_applications",
		"site_fix_id = sqlc.arg(site_fix_id)",
		"content_action_id is null",
		"status = 'verified'",
		"update site_fixes",
		"status in ('verifying','failed_retryable','reopened')",
		"update work_signature_registry",
		"status = 'verified'",
		"active = false",
	)

	retrySQL := namedSQL(t, siteFixes, "MarkCanonicalSiteFixRetryable")
	requireQuerySQL(t, retrySQL,
		"status = 'failed_retryable'",
		"retry_count = retry_count + 1",
		"retry_count < sf.max_retries",
		"active = true",
	)
	reopenSQL := namedSQL(t, siteFixes, "ReopenCanonicalSiteFix")
	requireQuerySQL(t, reopenSQL, "status = 'reopened'", "active = true")
	terminalSQL := namedSQL(t, siteFixes, "TerminalizeCanonicalSiteFix")
	requireQuerySQL(t, terminalSQL, "status = 'failed_terminal'", "active = false")
	supersedeSQL := namedSQL(t, siteFixes, "SupersedeCanonicalSiteFix")
	requireQuerySQL(t, supersedeSQL, "status = 'superseded'", "active = false")
	rollbackSQL := namedSQL(t, siteFixes, "MarkCanonicalSiteFixMigrationRolledBack")
	requireQuerySQL(t, rollbackSQL, "status = 'migration_rolled_back'", "active = false")
}

func TestLegacyApplicationWriterRejectsMissingContentAction(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("queries", "seo.sql"))
	if err != nil {
		t.Fatalf("read legacy SEO queries: %v", err)
	}
	query := namedSQL(t, strings.ToLower(string(raw)), "CreateOrReuseSiteChangeApplication")
	requireQuerySQL(t, query,
		"select",
		"where sqlc.arg(content_action_id)::uuid is not null",
	)
}
