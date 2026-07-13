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
		"ClaimCanonicalSiteFixGitHubPR",
		"FailCanonicalSiteFixGitHubPRClaim",
		"ReopenCanonicalSiteFixApply",
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

func TestLegacyTechnicalMigrationTreatsMissingApplicationAsNotDeployed(t *testing.T) {
	siteFixes, _ := readSiteFixQueryContracts(t)
	query := namedSQL(t, siteFixes, "ListLegacyTechnicalActionsForMigration")
	requireQuerySQL(t, query,
		"coalesce((latest_app.deployed_at is not null or latest_app.status in ('verification_pending','verified')), false)::boolean as deployment_observed",
		"coalesce((latest_app.verified_at is not null or latest_app.status = 'verified'), false)::boolean as verification_passed",
	)
}

func TestMigrationDoctorArtifactsKeepSiteFixParameterUUIDTyped(t *testing.T) {
	siteFixes, _ := readSiteFixQueryContracts(t)
	query := namedSQL(t, siteFixes, "CreateMigrationDoctorArtifacts")
	requireQuerySQL(t, query,
		"'site_fix', sqlc.arg(site_fix_id)::uuid::text, 'site_fix', sqlc.arg(site_fix_id)::uuid",
		"select sqlc.arg(site_fix_id)::uuid, sqlc.arg(project_id), chosen_finding.id",
	)
	if strings.Contains(query, "'site_fix', sqlc.arg(site_fix_id)::text") {
		t.Fatal("casting the shared site_fix_id parameter directly to text makes PostgreSQL infer text for reserved_work_id")
	}
	generated, err := os.ReadFile("site_fixes.sql.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "SiteFixID               uuid.UUID") {
		t.Fatal("sqlc must generate CreateMigrationDoctorArtifactsParams.SiteFixID as uuid.UUID")
	}
}

func TestCanonicalSiteFixListDetailsAreBoundedAndBatchReadable(t *testing.T) {
	siteFixes, _ := readSiteFixQueryContracts(t)
	list := namedSQL(t, siteFixes, "ListCanonicalSiteFixes")
	requireQuerySQL(t, list, "limit 250")
	for _, name := range []string{"ListLatestCanonicalSiteFixApplications", "ListCanonicalSiteFixVerificationsForList", "ListCanonicalSiteFixAliasesForList"} {
		query := namedSQL(t, siteFixes, name)
		requireQuerySQL(t, query, "project_id", "site_fix_id")
	}
	aliases := namedSQL(t, siteFixes, "ListCanonicalSiteFixAliasesForList")
	requireQuerySQL(t, aliases, "canonical_object_type = 'site_fix'", "alias_state = 'active'", "limit 250")
	getAliases := namedSQL(t, siteFixes, "ListCanonicalSiteFixAliasesForFix")
	requireQuerySQL(t, getAliases, "project_id", "canonical_object_id", "canonical_object_type = 'site_fix'", "alias_state = 'active'")
}

func TestCanonicalSiteFixDoctorLinkDismissalIsScopedAndPresentationOnly(t *testing.T) {
	siteFixes, _ := readSiteFixQueryContracts(t)
	dismiss := namedSQL(t, siteFixes, "DismissCanonicalSiteFixDoctorLink")
	requireQuerySQL(t, dismiss,
		"update site_fixes",
		"doctor_link_dismissed_at = coalesce(doctor_link_dismissed_at, sqlc.arg(dismissed_at)::timestamptz)",
		"doctor_link_dismissed_by = coalesce(doctor_link_dismissed_by, sqlc.arg(dismissed_by)::text)",
		"id = sqlc.arg(id)",
		"project_id = sqlc.arg(project_id)",
		"doctor_finding_id is not null",
		"returning *",
	)
	for _, forbidden := range []string{"status =", "doctor_finding_id =", "updated_at =", "delete ", "site_change_applications"} {
		if strings.Contains(dismiss, forbidden) {
			t.Fatalf("dismiss link query must not mutate Site Fix lifecycle or provenance; found %q", forbidden)
		}
	}
}

func TestCurrentDoctorFindingLinksAreCompleteWithoutExpandingTheCanonicalList(t *testing.T) {
	siteFixes, _ := readSiteFixQueryContracts(t)
	links := namedSQL(t, siteFixes, "ListCurrentDoctorSiteFixLinks")
	requireQuerySQL(t, links,
		"distinct on (fix.doctor_finding_id)",
		"from site_fixes fix",
		"join seo_doctor_findings finding",
		"finding.id = fix.doctor_finding_id",
		"finding.project_id = fix.project_id",
		"fix.project_id = sqlc.arg(project_id)",
		"finding.status = 'active'",
		"finding.finding_kind in ('broken','optimization')",
		"order by fix.doctor_finding_id, fix.created_at desc, fix.id desc",
	)
	if strings.Contains(links, "limit ") {
		t.Fatal("current-finding link projection must not inherit the 250-row canonical workspace limit")
	}
	applications := namedSQL(t, siteFixes, "ListCurrentDoctorSiteFixLinkApplications")
	requireQuerySQL(t, applications,
		"distinct on (application.site_fix_id)",
		"join current_links listed on listed.id = application.site_fix_id",
		"application.project_id = sqlc.arg(project_id)",
		"application.site_fix_id is not null",
		"application.content_action_id is null",
	)
}

func TestCanonicalSiteFixPRExternalEffectUsesAuthorityFencedLease(t *testing.T) {
	siteFixes, _ := readSiteFixQueryContracts(t)
	claim := namedSQL(t, siteFixes, "ClaimCanonicalSiteFixGitHubPR")
	requireQuerySQL(t, claim, "status = 'creating_pr'", "pr_claim_token", "pr_claim_expires_at", "authority_changed_at", "app.status = 'ready_for_pr'", "app.pr_claim_expires_at <= clock_timestamp()")
	mark := namedSQL(t, siteFixes, "MarkCanonicalSiteFixGitHubPR")
	requireQuerySQL(t, mark, "writer_authority = 'canonical'", "write_fenced = false", "status = 'creating_pr'", "pr_claim_token = sqlc.arg(pr_claim_token)", "pr_claim_expires_at > clock_timestamp()", "pr_claim_authority_fingerprint = (select fingerprint from authority)")
	fail := namedSQL(t, siteFixes, "FailCanonicalSiteFixGitHubPRClaim")
	requireQuerySQL(t, fail, "status = 'needs_follow_up'", "pr_claim_token = sqlc.arg(pr_claim_token)")
	reopen := namedSQL(t, siteFixes, "ReopenCanonicalSiteFixApply")
	requireQuerySQL(t, reopen, "app.status = 'needs_follow_up'", "status = 'ready_for_pr'", "sf.status = 'applying'", "w.status = 'executing'")
	renew := namedSQL(t, siteFixes, "RenewCanonicalSiteFixGitHubPRClaim")
	requireQuerySQL(t, renew, "pr_claim_expires_at", "pr_claim_token = sqlc.arg(pr_claim_token)", "pr_claim_authority_fingerprint = (select fingerprint from authority)")
	ddlRaw, err := os.ReadFile(filepath.Join("..", "migrations", "0057_01_site_fix_pr_claims.sql"))
	if err != nil {
		t.Fatal(err)
	}
	ddl := strings.ToLower(string(ddlRaw))
	for _, required := range []string{"status <> 'creating_pr'", "status = 'creating_pr'", "not valid", "validate constraint site_change_applications_pr_claim_check", "orphan_count > 10000"} {
		if !strings.Contains(ddl, required) {
			t.Errorf("PR claim DDL missing %q", required)
		}
	}
}

func TestSaveCanonicalSiteFixPreparedPatchIsClaimAndAuthorityFenced(t *testing.T) {
	siteFixes, _ := readSiteFixQueryContracts(t)
	save := namedSQL(t, siteFixes, "SaveCanonicalSiteFixPreparedPatch")
	requireQuerySQL(t, save,
		"app.project_id = sqlc.arg(project_id)",
		"app.id = sqlc.arg(application_id)",
		"app.site_fix_id = sqlc.arg(site_fix_id)",
		"app.status = 'creating_pr'",
		"app.pr_claim_token = sqlc.arg(pr_claim_token)",
		"app.pr_claim_expires_at > clock_timestamp()",
		"app.pr_claim_authority_fingerprint = sqlc.arg(writer_authority_fingerprint)",
		"sqlc.arg(writer_authority_fingerprint) = (select fingerprint from authority)",
		"source_file_paths = sqlc.arg(source_file_paths)::jsonb",
		"source_file_path = sqlc.narg(source_file_path)",
		"base_file_sha = sqlc.narg(base_file_sha)",
		"repo_full_name = sqlc.arg(repo_full_name)",
		"base_branch = sqlc.arg(base_branch)",
		"base_commit_sha = sqlc.arg(base_commit_sha)",
		"patch_snapshot = sqlc.arg(patch_snapshot)::jsonb",
		"diff_snapshot = sqlc.arg(diff_snapshot)::jsonb",
		"base_content_hash = sqlc.arg(base_content_hash)",
		"proposed_content_hash = sqlc.arg(proposed_content_hash)",
	)
}

func TestCanonicalSiteFixPreparationFailurePreservesRetryablePreparingState(t *testing.T) {
	siteFixes, _ := readSiteFixQueryContracts(t)
	record := namedSQL(t, siteFixes, "RecordCanonicalSiteFixPreparationFailure")
	requireQuerySQL(t, record,
		"sf.project_id = sqlc.arg(project_id)",
		"sf.id = sqlc.arg(site_fix_id)",
		"sf.status = 'preparing'",
		"w.status = 'preparing'",
		"w.active = true",
		"failure_reason = sqlc.arg(failure_code)",
	)
	for _, forbidden := range []string{"status = 'failed", "retry_count =", "failure_detail"} {
		if strings.Contains(record, forbidden) {
			t.Fatalf("preparation failure must remain retryable/preparing; found %q", forbidden)
		}
	}
	ready := namedSQL(t, siteFixes, "MarkCanonicalSiteFixReadyToApply")
	requireQuerySQL(t, ready, "set status = 'ready_to_apply', failure_reason = null")
}

func TestResetCanonicalSiteFixSourceConflictForReprepareIsClaimAndAuthorityFenced(t *testing.T) {
	siteFixes, _ := readSiteFixQueryContracts(t)
	reset := namedSQL(t, siteFixes, "ResetCanonicalSiteFixSourceConflictForReprepare")
	if got := strings.Count(reset, "app.pr_claim_expires_at > clock_timestamp()"); got != 2 {
		t.Fatalf("claim expiry must be checked before and while locking, not after dependent writes; got %d checks", got)
	}
	requireQuerySQL(t, reset,
		"app.project_id = sqlc.arg(project_id)",
		"app.id = sqlc.arg(application_id)",
		"app.site_fix_id = sqlc.arg(site_fix_id)",
		"app.status = 'creating_pr'",
		"app.pr_claim_token = sqlc.arg(pr_claim_token)",
		"app.pr_claim_expires_at > clock_timestamp()",
		"app.pr_claim_authority_fingerprint = (select fingerprint from authority)",
		"expected_keys as materialized",
		"locked_buckets as materialized",
		"order by b.bucket_key",
		"for update of b",
		"bucket_version = bucket_version + 1",
		"set status = 'failed', failure_reason = 'repository_source_conflict'",
		"set status = 'preparing', failure_reason = 'repository_source_conflict'",
		"set status = 'preparing', active = true",
	)
}

func TestCanonicalApplyFailureNeverEntersVerificationRetryLifecycle(t *testing.T) {
	siteFixes, _ := readSiteFixQueryContracts(t)
	applyFailure := namedSQL(t, siteFixes, "MarkCanonicalSiteFixApplyFailure")
	requireQuerySQL(t, applyFailure, "sf.status = 'applying'", "w.status = 'executing'", "a.status = 'github_pr_open'", "status = 'needs_follow_up'")
	for _, forbidden := range []string{"update site_fixes", "failed_retryable", "retry_count =", "update work_signature_registry"} {
		if strings.Contains(applyFailure, forbidden) {
			t.Errorf("apply failure must not mutate verification lifecycle; found %q", forbidden)
		}
	}
}

func TestCanonicalUserTerminationSupportsNoApplication(t *testing.T) {
	siteFixes, _ := readSiteFixQueryContracts(t)
	terminate := namedSQL(t, siteFixes, "TerminateCanonicalSiteFixByUser")
	requireQuerySQL(t, terminate, "left join lateral", "a.id as application_id", "e.application_id is null", "for update of sf, w", "update work_conflict_buckets", "status = 'failed_terminal'", "active = false")
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
		"sf.status = 'awaiting_deploy' and w.status = 'awaiting_deploy'",
		"a.status in ('deployment_pending','verification_pending','needs_follow_up')",
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

func TestDoctorAIOnDemandTriggerIsPersistentAndExactlyOnce(t *testing.T) {
	siteFixes, _ := readSiteFixQueryContracts(t)
	claim := namedSQL(t, siteFixes, "ClaimDoctorAIOnDemandTrigger")
	requireQuerySQL(t, claim,
		"insert into doctor_ai_on_demand_triggers",
		"on conflict (request_id) do nothing",
		"doctor_ai_enabled",
		"('manual_only','on_demand','automatic')",
		"site_fix_id = sqlc.arg(site_fix_id)",
	)
	requireQuerySQL(t, claim, "marker.status in ('pending','processing')", "marker.status = 'consumed' and marker.lifecycle_applied_at is null")
	processing := namedSQL(t, siteFixes, "ClaimDoctorAIOnDemandProcessing")
	requireQuerySQL(t, processing,
		"status = 'processing'",
		"for update of marker skip locked",
		"processing_token",
		"processing_expires_at <= clock_timestamp()",
		"doctor_ai_enabled",
		"('manual_only','on_demand','automatic')",
	)
	start := namedSQL(t, siteFixes, "StartDoctorAIOnDemandCall")
	requireQuerySQL(t, start, "insert into ai_call_records", "'queued'", "ai_call_id", "processing_token", "processing_expires_at > clock_timestamp()", "for update of marker")
	consume := namedSQL(t, siteFixes, "ConsumeDoctorAIOnDemandProcessing")
	requireQuerySQL(t, consume,
		"status = 'consumed'", "result_snapshot", "processing_token = sqlc.arg(processing_token)", "ai_call_id = sqlc.arg(ai_call_id)",
		"doctor_ai_enabled", "doctor_ai_run_policy", "sf.status in ('awaiting_deploy','verifying','reopened')",
	)
	namedSQL(t, siteFixes, "GetDoctorAIOnDemandConsumedResult")
	namedSQL(t, siteFixes, "MarkDoctorAIOnDemandLifecycleApplied")
	reject := namedSQL(t, siteFixes, "RejectDoctorAIOnDemandTriggersForSiteFix")
	requireQuerySQL(t, reject,
		"status = 'rejected'", "marker.status in ('pending','processing')", "marker.status = 'consumed' and marker.lifecycle_applied_at is null", "lifecycle_applied_at = now()",
		"processing_token = null", "processing_expires_at = null", "call.status in ('queued','running')", "case when call.status = 'queued' then 'skipped' else 'failed' end", "provider_not_called",
		"when marker.status = 'consumed' then marker.result_snapshot",
	)
	supersede := namedSQL(t, siteFixes, "SupersedeDoctorAIOnDemandSiblingTriggers")
	requireQuerySQL(t, supersede,
		"status = 'superseded'", "marker.status in ('pending','processing')", "marker.status = 'consumed' and marker.lifecycle_applied_at is null", "marker.request_id <> sqlc.arg(applied_request_id)",
		"call.status in ('queued','running')", "case when call.status = 'queued' then 'skipped' else 'failed' end", "provider_not_called",
		"when marker.status = 'consumed' then marker.result_snapshot",
	)
	unauthorized := namedSQL(t, siteFixes, "RejectUnauthorizedDoctorAIOnDemandTriggers")
	requireQuerySQL(t, unauthorized, "marker.status = 'consumed' and marker.lifecycle_applied_at is null", "call.status in ('queued','running')", "provider_not_called")
	consumed := namedSQL(t, siteFixes, "ListDoctorAIOnDemandConsumedUnapplied")
	requireQuerySQL(t, consumed,
		"site_fix_verifications", "verification.ai_call_id = marker.ai_call_id", "has_lifecycle_reference",
	)
	unreferenced := namedSQL(t, siteFixes, "RejectDoctorAIOnDemandConsumedWithoutLifecycleReference")
	requireQuerySQL(t, unreferenced,
		"marker.status = 'consumed'", "status = 'rejected'", "lifecycle_completed_without_this_ai_result",
		"not exists", "site_fix_verifications",
	)
	namedSQL(t, siteFixes, "ListRejectedDoctorAIRunningCalls")

	migration, err := os.ReadFile(filepath.Join("..", "migrations", "0056_doctor_ai_on_demand_triggers.sql"))
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.ToLower(string(migration))
	for _, required := range []string{"request_id uuid primary key", "status in ('pending','processing','consumed','rejected','superseded')", "processing_token", "processing_expires_at", "ai_call_id", "result_snapshot", "rejection_reason", "lifecycle_applied_at", "foreign key (project_id, site_fix_id)"} {
		if !strings.Contains(schema, required) {
			t.Errorf("on-demand trigger schema missing %q", required)
		}
	}
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
