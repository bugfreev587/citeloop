package sitefix

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMigrationUsesCanonicalDiscoveryIdentityAndIgnoresEvidenceInExactSignature(t *testing.T) {
	projectID := uuid.New()
	first := fixtureLegacyOpportunity(projectID, "schema_gap", "https://example.com/a", `{"source":"crawl-a"}`)
	second := fixtureLegacyOpportunity(projectID, "schema_gap", "https://example.com/a", `{"source":"crawl-b"}`)
	first.ApplicationIDs, second.ApplicationIDs = nil, nil
	report, err := ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{first, second})
	if err != nil {
		t.Fatal(err)
	}
	projected := discovery.ProjectSEOOpportunity(db.SeoOpportunity{ID: first.OpportunityID, ProjectID: projectID, Type: "schema_gap", NormalizedPageUrl: "https://example.com/a", Evidence: first.OpportunityEvidence})
	want, err := discovery.BuildIdentity(projected)
	if err != nil {
		t.Fatal(err)
	}
	if report.Items[0].IdentityHash != want.ExactSignatureHash {
		t.Fatalf("migration exact=%s canonical=%s", report.Items[0].IdentityHash, want.ExactSignatureHash)
	}
	if report.MigratedCount != 1 || report.ArchivedDuplicateCount != 1 {
		t.Fatalf("evidence changes must not split exact work: %#v", report)
	}
	if string(report.Items[0].SignaturePayload) != string(want.SignaturePayload) {
		t.Fatal("migration must persist canonical signature payload")
	}
	if !equalStrings(report.Items[0].ConflictBucketKeys, want.ConflictBucketKeys) {
		t.Fatalf("buckets=%v want %v", report.Items[0].ConflictBucketKeys, want.ConflictBucketKeys)
	}
}

func TestMigrationSourceUniverseIncludesOpportunityOnlyAndRejectsGrowthPatch(t *testing.T) {
	projectID := uuid.New()
	technical := fixtureLegacyOpportunity(projectID, "schema_gap", "https://example.com/a", `{"issue":"schema"}`)
	technical.ActionID = uuid.Nil
	growth := fixtureLegacyOpportunity(projectID, "gsc_low_ctr_query", "https://example.com/b", `{"issue":"low_ctr"}`)
	growth.ChangeFamily = "metadata_rewrite"
	report, err := ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{technical, growth})
	if err != nil {
		t.Fatal(err)
	}
	if report.SourceCount != 2 || report.MigratedCount != 1 || report.ReviewCount != 1 {
		t.Fatalf("full source conservation failed: %#v", report)
	}
	if report.Items[0].LegacyActionID != uuid.Nil || report.Items[0].Disposition != MigrationDispositionMigrate {
		t.Fatalf("opportunity-only technical source not migrated: %#v", report.Items[0])
	}
	if report.Items[1].ReasonCode != "owner_or_verification_ambiguous" {
		t.Fatalf("growth patch must be reviewed: %#v", report.Items[1])
	}
}

func TestMigrationActiveWorkCollisionsAreClassifiedBeforeWrites(t *testing.T) {
	projectID := uuid.New()
	source := fixtureLegacyOpportunity(projectID, "schema_gap", "https://example.com/a", `{"issue":"schema"}`)
	canonical := discovery.ProjectSEOOpportunity(db.SeoOpportunity{ID: source.OpportunityID, ProjectID: projectID, Type: "schema_gap", NormalizedPageUrl: source.TargetURL, Evidence: source.OpportunityEvidence})
	identity, _ := discovery.BuildIdentity(canonical)
	existingFixID := uuid.New()
	source.ActiveWork = []MigrationActiveWork{{WorkSignatureID: uuid.New(), ExactSignatureHash: identity.ExactSignatureHash, Owner: "doctor", WorkType: "site_fix", WorkID: existingFixID, Active: true}}
	report, err := ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{source})
	if err != nil {
		t.Fatal(err)
	}
	if report.ArchivedDuplicateCount != 1 || report.Items[0].ExistingSiteFixID != existingFixID {
		t.Fatalf("existing Doctor exact work must become alias/evidence duplicate: %#v", report)
	}
	source.ActiveWork[0].ActiveApplicationCount = 1
	report, err = ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{source})
	if err != nil {
		t.Fatal(err)
	}
	if report.ReviewCount != 1 || report.Items[0].ReasonCode != "duplicate_active_applications" {
		t.Fatalf("existing active application collision=%#v", report)
	}

	source.ActiveWork[0].ActiveApplicationCount = 0
	source.ActiveWork[0].Owner, source.ActiveWork[0].WorkType = "opportunities", "growth_action"
	report, err = ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{source})
	if err != nil {
		t.Fatal(err)
	}
	if report.ReviewCount != 1 || report.Items[0].ReasonCode != "active_cross_line_collision" {
		t.Fatalf("cross-line collision must review: %#v", report)
	}
}

func TestMigrationExactDuplicatesWithIndependentActiveApplicationsRequireReview(t *testing.T) {
	projectID := uuid.New()
	first := fixtureLegacyOpportunity(projectID, "schema_gap", "https://example.com/a", `{"issue":"schema-a"}`)
	second := fixtureLegacyOpportunity(projectID, "schema_gap", "https://example.com/a", `{"issue":"schema-b"}`)
	first.ApplicationIDs, second.ApplicationIDs = []uuid.UUID{uuid.New()}, []uuid.UUID{uuid.New()}
	report, err := ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if report.ReviewCount != 2 || report.ArchivedDuplicateCount != 0 {
		t.Fatalf("independent active applications require group review: %#v", report)
	}
}

func TestMigrationOneLegacyActionWithMultipleActiveApplicationsRequiresReview(t *testing.T) {
	projectID := uuid.New()
	row := fixtureLegacyOpportunity(projectID, "schema_gap", "https://example.com/a", `{"issue":"missing_schema"}`)
	row.ApplicationIDs = []uuid.UUID{uuid.New(), uuid.New()}
	report, err := ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{row})
	if err != nil {
		t.Fatal(err)
	}
	if report.ReviewCount != 1 || report.Items[0].ReasonCode != "duplicate_active_applications" {
		t.Fatalf("report=%+v", report)
	}
}

func TestMigrationConflictingActiveReviewMemoryRequiresReview(t *testing.T) {
	projectID := uuid.New()
	row := fixtureLegacyOpportunity(projectID, "schema_gap", "https://example.com/a", `{"issue":"missing_schema"}`)
	row.ApplicationIDs = nil
	row.ReviewDecision = "dismissed"
	_, identity, reason := canonicalLegacyMigrationIdentity(row)
	if reason != "" {
		t.Fatal(reason)
	}
	row.ActiveReviewMemory = []MigrationActiveReviewMemory{{ID: uuid.New(), ExactSignatureHash: identity.ExactSignatureHash, Decision: "watching"}}
	report, err := ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{row})
	if err != nil {
		t.Fatal(err)
	}
	if report.ReviewCount != 1 || report.Items[0].ReasonCode != "review_memory_conflict" {
		t.Fatalf("report=%+v", report)
	}
}

func TestMigrationReviewMemoryAliasesAndBucketOverlapNeverReopenWork(t *testing.T) {
	projectID := uuid.New()
	row := fixtureLegacyOpportunity(projectID, "schema_gap", "https://example.com/a", `{"issue":"missing_schema"}`)
	row.ApplicationIDs = nil
	candidate, identity, reason := canonicalLegacyMigrationIdentity(row)
	if reason != "" {
		t.Fatal(reason)
	}
	row.ActiveReviewMemory = []MigrationActiveReviewMemory{{ID: uuid.New(), ExactSignatureHash: identity.ExactSignatureHash, ConflictBucketKeys: identity.ConflictBucketKeys, Decision: "dismissed", EvidenceFingerprint: candidate.EvidenceFingerprint, ViaAlias: true}}
	report, err := ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{row})
	if err != nil {
		t.Fatal(err)
	}
	if report.ReviewCount != 1 || report.Items[0].ReasonCode != "review_memory_applied" {
		t.Fatalf("alias memory report=%+v", report)
	}

	row.ActiveReviewMemory[0].ExactSignatureHash = "different-signature"
	report, err = ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{row})
	if err != nil {
		t.Fatal(err)
	}
	if report.ReviewCount != 1 || report.Items[0].ReasonCode != "review_memory_bucket_overlap" {
		t.Fatalf("bucket memory report=%+v", report)
	}
}

func TestMigrationPlannedCollisionRequiresReview(t *testing.T) {
	projectID := uuid.New()
	row := fixtureLegacyOpportunity(projectID, "schema_gap", "https://example.com/a", `{"issue":"missing_schema"}`)
	row.ApplicationIDs = nil
	_, identity, reason := canonicalLegacyMigrationIdentity(row)
	if reason != "" {
		t.Fatal(reason)
	}
	row.ActiveWork = []MigrationActiveWork{{WorkSignatureID: uuid.New(), ExactSignatureHash: identity.ExactSignatureHash, ConflictBucketKeys: identity.ConflictBucketKeys, Owner: "doctor", WorkType: "planned_candidate", WorkID: uuid.New(), Planned: true}}
	report, err := ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{row})
	if err != nil {
		t.Fatal(err)
	}
	if report.ReviewCount != 1 || report.Items[0].ReasonCode != "planned_work_collision" {
		t.Fatalf("planned report=%+v", report)
	}
}

func TestMigrationAcceptanceTestsUseExecutableTypedFamilies(t *testing.T) {
	projectID := uuid.New()
	for _, opportunityType := range []string{"schema_gap", "canonical_missing", "noindex", "title_missing", "broken_url", "zero_internal_links"} {
		row := fixtureLegacyOpportunity(projectID, opportunityType, "https://example.com/a", `{"issue":"`+opportunityType+`"}`)
		candidate, _, reason := canonicalLegacyMigrationIdentity(row)
		if reason != "" {
			t.Fatalf("%s: %s", opportunityType, reason)
		}
		tests := migrationAcceptanceTests(candidate, row)
		if len(tests) == 0 || strings.TrimSpace(tests[0]["type"].(string)) == "" {
			t.Fatalf("%s tests=%v", opportunityType, tests)
		}
		if _, legacyKind := tests[0]["kind"]; legacyKind {
			t.Fatalf("legacy kind is not executable: %v", tests)
		}
	}
}

func TestMigrationAcceptanceTestsRequireEvidenceOutsidePageVerifierScope(t *testing.T) {
	projectID := uuid.New()
	for _, opportunityType := range []string{"soft_404", "orphan_page", "internal_link_gap", "redirect_chain", "robots_blocked", "title_too_long", "canonical_mismatch"} {
		row := fixtureLegacyOpportunity(projectID, opportunityType, "https://example.com/a", `{"issue":"`+opportunityType+`"}`)
		row.ApplicationIDs = nil
		report, err := ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{row})
		if err != nil {
			t.Fatalf("%s: %v", opportunityType, err)
		}
		if report.ReviewCount != 1 || report.Items[0].ReasonCode != "acceptance_criterion_incomplete" {
			t.Fatalf("%s report=%+v", opportunityType, report)
		}
	}
}

func TestMigrationAcceptanceTestsUseExplicitExpectedTargets(t *testing.T) {
	projectID := uuid.New()
	tests := []struct {
		name            string
		opportunityType string
		evidence        string
		output          json.RawMessage
		wantType        string
	}{
		{name: "redirect chain", opportunityType: "redirect_chain", evidence: `{"issue":"redirect_chain","expected_final_url":"https://example.com/final"}`, wantType: "content_evidence_present"},
		{name: "internal link gap", opportunityType: "internal_link_gap", evidence: `{"issue":"internal_link_gap","target_url":"https://example.com/linked"}`, wantType: "content_evidence_present"},
		{name: "canonical mismatch", opportunityType: "canonical_mismatch", evidence: `{"issue":"canonical_mismatch","expected_canonical":"https://example.com/canonical"}`, wantType: "canonical_equals"},
		{name: "title too long", opportunityType: "title_too_long", evidence: `{"issue":"title_too_long"}`, output: json.RawMessage(`{"expected_title":"Concise title"}`), wantType: "title_equals"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := fixtureLegacyOpportunity(projectID, tt.opportunityType, "https://example.com/a", tt.evidence)
			row.OutputSnapshot = tt.output
			candidate, _, reason := canonicalLegacyMigrationIdentity(row)
			if reason != "" {
				t.Fatal(reason)
			}
			acceptance := migrationAcceptanceTests(candidate, row)
			if len(acceptance) != 1 || acceptance[0]["type"] != tt.wantType {
				t.Fatalf("acceptance=%v", acceptance)
			}
		})
	}
}

func TestMigrationIncompleteAcceptanceCriterionRoutesReview(t *testing.T) {
	projectID := uuid.New()
	row := fixtureLegacyOpportunity(projectID, "broken_internal_link", "https://example.com/a", `{"issue":"broken_internal_link"}`)
	row.ApplicationIDs = nil
	report, err := ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{row})
	if err != nil {
		t.Fatal(err)
	}
	if report.ReviewCount != 1 || report.Items[0].ReasonCode != "acceptance_criterion_incomplete" {
		t.Fatalf("report=%+v", report)
	}
}

func TestMigrationBucketVersionDriftChangesSnapshotAndAllDispositionsLock(t *testing.T) {
	projectID := uuid.New()
	row := fixtureLegacyOpportunity(projectID, "schema_gap", "https://example.com/a", `{"issue":"missing_schema"}`)
	row.ApplicationIDs = nil
	_, identity, reason := canonicalLegacyMigrationIdentity(row)
	if reason != "" {
		t.Fatal(reason)
	}
	row.BucketVersions = map[string]int64{identity.ConflictBucketKeys[0]: 1}
	first, err := ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{row})
	if err != nil {
		t.Fatal(err)
	}
	row.BucketVersions[identity.ConflictBucketKeys[0]] = 2
	second, err := ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{row})
	if err != nil {
		t.Fatal(err)
	}
	if first.SnapshotHash == second.SnapshotHash {
		t.Fatal("bucket version drift must invalidate migration snapshot")
	}
	keys := plannedMigrationBucketKeys([]MigrationClassification{{Disposition: MigrationDispositionReview, ConflictBucketKeys: identity.ConflictBucketKeys}})
	if len(keys) != len(identity.ConflictBucketKeys) {
		t.Fatalf("review buckets were not locked: %v", keys)
	}
}

func TestMigrationConflictingLegacyReviewDecisionsRequireReview(t *testing.T) {
	projectID := uuid.New()
	first := fixtureLegacyOpportunity(projectID, "schema_gap", "https://example.com/a", `{"issue":"missing_schema"}`)
	second := first
	second.OpportunityID, second.ActionID = uuid.New(), uuid.New()
	first.ApplicationIDs, second.ApplicationIDs = nil, nil
	first.ReviewDecision, second.ReviewDecision = "dismissed", "watching"
	report, err := ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if report.ReviewCount != 2 || report.ArchivedDuplicateCount != 0 {
		t.Fatalf("report=%+v", report)
	}
	for _, item := range report.Items {
		if item.ReasonCode != "conflicting_legacy_review_decisions" {
			t.Fatalf("item=%+v", item)
		}
	}
}

func TestMigrationBatchBucketCollisionsRequireReview(t *testing.T) {
	projectID := uuid.New()
	first := fixtureLegacyOpportunity(projectID, "canonical_missing", "https://example.com/a", `{"issue":"canonical_missing"}`)
	second := fixtureLegacyOpportunity(projectID, "canonical_mismatch", "https://example.com/a", `{"issue":"canonical_mismatch","expected_canonical":"https://example.com/canonical"}`)
	second.OpportunityID, second.ActionID = uuid.New(), uuid.New()
	report, err := ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if report.ReviewCount != 2 || report.MigratedCount != 0 {
		t.Fatalf("overlapping migration work=%+v", report)
	}
	for _, item := range report.Items {
		if item.ReasonCode != "migration_batch_bucket_collision" {
			t.Fatalf("item=%+v", item)
		}
	}
}

func TestMigrationRetriesOnlySerializationAndDeadlock(t *testing.T) {
	for _, code := range []string{"40001", "40P01"} {
		if !isRetryableMigrationTx(&pgconn.PgError{Code: code}) {
			t.Fatalf("code %s must retry", code)
		}
	}
	for _, err := range []error{&pgconn.PgError{Code: "23505"}, ErrMigrationSnapshotDrift, errors.New("network")} {
		if isRetryableMigrationTx(err) {
			t.Fatalf("unexpected retry for %v", err)
		}
	}
}

func TestMigrationStatusProjectorUsesApplicationAndNeverMovesBackward(t *testing.T) {
	tests := []struct {
		action, application    string
		pr, deployed, verified bool
		wantFix                string
		active                 bool
	}{
		{action: "measuring", application: "verified", verified: true, wantFix: "verified", active: false},
		{action: "approved", application: "github_pr_open", pr: true, wantFix: "applying", active: true},
		{action: "approved", application: "github_pr_merged", pr: true, wantFix: "awaiting_deploy", active: true},
		{action: "approved", application: "github_pr_closed", pr: true, wantFix: "failed_retryable", active: true},
		{action: "verification_pending", application: "verification_pending", deployed: true, wantFix: "verifying", active: true},
		{action: "manual_apply_required", application: "manual_apply_required", wantFix: "ready_to_apply", active: true},
		{action: "returned", wantFix: "failed_terminal", active: false},
		{action: "dismissed", wantFix: "failed_terminal", active: false},
		{action: "approved", wantFix: "failed_terminal", active: false},
	}
	for _, tc := range tests {
		legacy := LegacyTechnicalAction{Status: tc.action, ApplicationStatus: tc.application, HasPullRequest: tc.pr, DeploymentObserved: tc.deployed, VerificationPassed: tc.verified}
		if tc.action == "approved" && tc.application == "" && tc.wantFix == "failed_terminal" {
			legacy.ReviewDecision = "snoozed"
		}
		fix, _, active := projectMigrationStatus(legacy)
		if fix != tc.wantFix || active != tc.active {
			t.Fatalf("%s/%s => %s/%v want %s/%v", tc.action, tc.application, fix, active, tc.wantFix, tc.active)
		}
	}
}

func TestMigrationPersistenceContracts(t *testing.T) {
	migration, err := os.ReadFile("../migrations/0058_legacy_site_fix_cutover.sql")
	if err != nil {
		t.Fatalf("read cutover migration: %v", err)
	}
	schema := strings.ToLower(string(migration))
	for _, want := range []string{
		"canonical_site_fix_id uuid",
		"canonical_read_only boolean not null default false",
		"legacy_migration_batch_id uuid",
		"legacy technical rows are canonical provenance",
		"migration_rolled_back",
		"enforce_legacy_canonical_read_only",
		"legacy technical rows are canonical-read-only",
		"if tg_op = 'delete'",
		"old.canonical_read_only and not new.canonical_read_only",
		"pwa.writer_authority = 'canonical' and pwa.write_fenced",
		"not old.canonical_read_only and new.canonical_read_only",
		"pwa.writer_authority = 'legacy' and pwa.write_fenced",
		"coalesce(new.output_snapshot->>'output_type', new.diff_snapshot->>'output_type', '')",
		"coalesce(new.work_type, '') in ('','fix_site_issue')",
		"create trigger site_change_applications_legacy_writer_authority",
		"legacy technical application writer is not authoritative",
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("cutover schema missing %q", want)
		}
	}
	queries, err := os.ReadFile("../db/queries/site_fixes.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(queries))
	for _, want := range []string{
		"-- name: listlegacytechnicalactionsformigration :many",
		"-- name: createmigrationdoctorartifacts :one",
		"-- name: repointlegacyapplicationstocanonicalsitefix :many",
		"-- name: rollbacklegacysitefixmigration :one",
		"for update",
		"bucket_version = bucket_version + 1",
		"num_nonnulls",
		"-- name: getmigrationconservation :one",
		"-- name: countmigrationrollbackrelationblockers :one",
		"-- name: listplannedworkforlegacymigration :many",
		"join work_signature_aliases alias",
		"bumped_buckets as",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration queries missing %q", want)
		}
	}
}

func TestMigrationCutoverDDLUsesProjectIdentityAndDatabaseWriterFence(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0058_legacy_site_fix_cutover.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"foreign key (project_id, canonical_site_fix_id)",
		"references site_fixes(project_id, id)",
		"foreign key (project_id, legacy_migration_batch_id)",
		"references migration_batches(project_id, id)",
		"current_setting('citeloop.migration_fence_token', true)",
		"for share",
		"authority is distinct from 'legacy' or authority_fenced is distinct from false",
		"create trigger content_actions_technical_writer_authority",
		"create trigger seo_opportunities_technical_writer_authority",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("cutover DDL missing %q", want)
		}
	}
	indexRaw, err := os.ReadFile("../migrations/0059_01_legacy_cutover_project_indexes.sql")
	if err != nil {
		t.Fatalf("read concurrent project indexes: %v", err)
	}
	indexes := strings.ToLower(string(indexRaw))
	for _, want := range []string{"citeloop:migration-mode=nontransactional", "create unique index concurrently", "on seo_opportunity_review_states (project_id, id)"} {
		if !strings.Contains(indexes, want) {
			t.Fatalf("index migration missing %q", want)
		}
	}
	storeRaw, _ := os.ReadFile("migration_postgres.go")
	storeText := string(storeRaw)
	for _, want := range []string{"citeloop.migration_fence_token", "MaterializeConflictBuckets", "LockConflictBucketsForReserve", "LockMigrationBucketsForBatch", `pgErr.Code == "40001"`, `pgErr.Code == "40P01"`} {
		if !strings.Contains(storeText, want) {
			t.Fatalf("migration store missing concurrency contract %q", want)
		}
	}
	aliasRaw, err := os.ReadFile("../migrations/0059_05_active_legacy_alias_index.sql")
	if err != nil {
		t.Fatal(err)
	}
	aliasIndex := strings.ToLower(string(aliasRaw))
	for _, want := range []string{"create unique index concurrently", "legacy_object_aliases (project_id, legacy_object_type, legacy_object_id)", "alias_state in ('active')"} {
		if !strings.Contains(aliasIndex, want) {
			t.Fatalf("active alias index missing %q", want)
		}
	}
}

func TestMigrationLedgerRollbackIsVersionedReverseAndHashGuarded(t *testing.T) {
	storeRaw, err := os.ReadFile("migration_postgres.go")
	if err != nil {
		t.Fatal(err)
	}
	store := string(storeRaw)
	for _, want := range []string{
		"ListMigrationLedgerForBatch",
		"for index := len(ledger) - 1; index >= 0; index--",
		"operationVersionSiteFixMigrationV1",
		"inverseOperationVersionSiteFixMigrationV1",
		"currentHash != operation.AfterHash",
		"applyMigrationInverse",
		"currentMigrationSnapshotHash",
	} {
		if !strings.Contains(store, want) {
			t.Fatalf("rollback store missing %q", want)
		}
	}
	if strings.Contains(store, `plan.SnapshotHash + ":" + plan.BatchID.String()`) {
		t.Fatal("result snapshot hash must come from actual canonical rows, not batch ID synthesis")
	}
	queriesRaw, _ := os.ReadFile("../db/queries/site_fixes.sql")
	queries := strings.ToLower(string(queriesRaw))
	for _, want := range []string{
		"-- name: getmigrationcurrentsnapshot :one",
		"-- name: getnextmigrationrollbackeventsequence :one",
		"-- name: restorelegacycontentactionfromledger :one",
		"-- name: restorelegacyopportunityfromledger :one",
		"-- name: restorelegacyapplicationfromledger :one",
		"-- name: tombstonelegacymigrationalias :one",
	} {
		if !strings.Contains(queries, want) {
			t.Fatalf("inverse SQL missing %q", want)
		}
	}
}

func TestMigrationEveryDerivedMutationHasLedgerCoverage(t *testing.T) {
	raw, err := os.ReadFile("migration_postgres.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, objectType := range []string{
		`"seo_doctor_run"`, `"seo_doctor_finding"`, `"discovery_shadow_run"`,
		`"discovery_candidate"`, `"work_conflict_bucket"`, `"work_signature_registry"`,
		`"site_fix"`, `"content_action"`, `"seo_opportunity"`, `"site_change_application"`,
		`"legacy_object_alias"`, `"migration_review_item"`, `"work_review_memory"`,
		`"product_writer_authority"`,
	} {
		if !strings.Contains(text, objectType) {
			t.Fatalf("forward ledger coverage missing %s", objectType)
		}
	}
}

func TestMigrationTerminalStatusesReleaseActiveSignature(t *testing.T) {
	for _, tc := range []struct {
		legacy, fix, registry string
		active                bool
	}{
		{"completed", "verified", "verified", false},
		{"failed", "failed_terminal", "failed_terminal", false},
		{"verification_failed", "failed_retryable", "failed_retryable", true},
		{"approved", "approved", "approved", true},
	} {
		fix, registry, active := migrationStatuses(tc.legacy)
		if fix != tc.fix || registry != tc.registry || active != tc.active {
			t.Fatalf("%s => %s/%s/%v", tc.legacy, fix, registry, active)
		}
	}
}

func TestMigrationConflictingActionsOnOneOpportunityEnterReviewTogether(t *testing.T) {
	projectID, opportunityID := uuid.New(), uuid.New()
	first := fixtureLegacy(projectID, "https://example.com/a", "schema_patch", `{"issue":"schema"}`)
	second := fixtureLegacy(projectID, "https://example.com/b", "technical_fix", `{"issue":"noindex"}`)
	first.OpportunityID, second.OpportunityID = opportunityID, opportunityID
	report, err := ClassifyLegacyTechnicalActions(projectID, "legacy", []LegacyTechnicalAction{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if report.ReviewCount != 2 || report.MigratedCount != 0 {
		t.Fatalf("conflicting legacy group must not guess an alias winner: %#v", report)
	}
	for _, item := range report.Items {
		if item.ReasonCode != "ambiguous_legacy_group" {
			t.Fatalf("unexpected reason: %#v", item)
		}
	}
}

func TestMigrationDryRunConservesRows(t *testing.T) {
	projectID := uuid.New()
	rows := []LegacyTechnicalAction{
		fixtureLegacy(projectID, "https://example.com/a", "schema_patch", `{"source":"crawl","issue":"missing_schema"}`),
		fixtureLegacy(projectID, "https://example.com/a", "schema_patch", `{"issue":"missing_schema","source":"crawl"}`),
		fixtureLegacy(projectID, "https://example.com/b", "technical_fix", `{"issue":"noindex"}`),
		fixtureLegacy(projectID, "", "technical_fix", `{"issue":"unknown"}`),
		fixtureLegacy(projectID, "https://example.com/c", "technical_fix", `{}`),
	}
	rows[2].DoctorFindingID = uuid.New()
	rows[0].ApplicationIDs, rows[1].ApplicationIDs = nil, nil

	report, err := ClassifyLegacyTechnicalActions(projectID, "legacy", rows)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := report.SourceCount, 5; got != want {
		t.Fatalf("source count=%d want %d", got, want)
	}
	if got, want := report.MigratedCount, 2; got != want {
		t.Fatalf("migrated=%d want %d", got, want)
	}
	if got, want := report.ArchivedDuplicateCount, 1; got != want {
		t.Fatalf("duplicates=%d want %d", got, want)
	}
	if got, want := report.ReviewCount, 2; got != want {
		t.Fatalf("review=%d want %d", got, want)
	}
	if report.SourceCount != report.MigratedCount+report.ArchivedDuplicateCount+report.ReviewCount {
		t.Fatal("migration classification does not conserve legacy actions")
	}
	if report.Items[0].Disposition != MigrationDispositionMigrate || report.Items[1].Disposition != MigrationDispositionArchiveDuplicate {
		t.Fatalf("exact duplicates must deterministically retain the first action: %#v", report.Items[:2])
	}
	if report.Items[1].CanonicalActionID != report.Items[0].LegacyActionID {
		t.Fatal("duplicate must point to its deterministic canonical action")
	}
	if report.Items[3].ReasonCode != "ambiguous_target" || report.Items[4].ReasonCode != "insufficient_evidence" {
		t.Fatalf("ambiguities must enter migration_review without guessing: %#v", report.Items[3:])
	}

	second, err := ClassifyLegacyTechnicalActions(projectID, "legacy", append([]LegacyTechnicalAction(nil), rows...))
	if err != nil {
		t.Fatal(err)
	}
	if report.SnapshotHash == "" || report.SnapshotHash != second.SnapshotHash {
		t.Fatalf("snapshot hash must be deterministic: %q != %q", report.SnapshotHash, second.SnapshotHash)
	}
	rows[0].Evidence = []byte(`{"issue":"different"}`)
	drifted, err := ClassifyLegacyTechnicalActions(projectID, "legacy", rows)
	if err != nil {
		t.Fatal(err)
	}
	if drifted.SnapshotHash == report.SnapshotHash {
		t.Fatal("material evidence change must drift snapshot hash")
	}
}

func TestMigrationRollbackRestoresSingleWriter(t *testing.T) {
	projectID := uuid.New()
	store := newFakeMigrationStore(projectID, []LegacyTechnicalAction{
		fixtureLegacy(projectID, "https://example.com/a", "technical_fix", `{"issue":"noindex"}`),
		fixtureLegacy(projectID, "https://example.com/b", "schema_patch", `{"issue":"schema"}`),
	})
	service := NewMigrationService(store)
	dry, err := service.DryRun(context.Background(), projectID, "admin")
	if err != nil {
		t.Fatal(err)
	}
	batch, err := service.Apply(context.Background(), projectID, dry.SnapshotHash, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if store.authority != "canonical" || store.fenced {
		t.Fatalf("apply authority=%s fenced=%v", store.authority, store.fenced)
	}
	if len(store.siteFixes) != 2 || len(store.ledger) == 0 {
		t.Fatalf("forward migration incomplete: fixes=%d ledger=%d", len(store.siteFixes), len(store.ledger))
	}
	for _, fix := range store.siteFixes {
		if fix.FindingID == uuid.Nil || fix.SignatureID == uuid.Nil {
			t.Fatal("Site Fix must follow finding and atomic signature creation")
		}
		if !fix.LegacyReadOnly {
			t.Fatal("legacy technical row must become canonical-read-only")
		}
	}
	for _, op := range store.ledger {
		if op.OperationVersion != "site-fix-migration/v1" || op.InverseOperationVersion != "site-fix-migration-inverse/v1" || len(op.InverseOperation) == 0 {
			t.Fatalf("every operation needs a versioned inverse: %#v", op)
		}
	}
	if err := store.assertOneSourceAndAliases(); err != nil {
		t.Fatal(err)
	}

	rolled, err := service.Rollback(context.Background(), projectID, batch.BatchID, batch.ResultSnapshotHash, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if store.authority != "legacy" || store.fenced {
		t.Fatalf("rollback authority=%s fenced=%v", store.authority, store.fenced)
	}
	if !rolled.RolledBack {
		t.Fatal("rollback report must be terminal")
	}
	for _, fix := range store.siteFixes {
		if fix.Status != "migration_rolled_back" || fix.AliasState != "rolled_back_tombstone" {
			t.Fatalf("rollback must retain canonical tombstones: %#v", fix)
		}
	}
	if err := store.assertRestored(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationApplyRejectsSnapshotDriftAndRollbackRejectsLossyProjection(t *testing.T) {
	projectID := uuid.New()
	store := newFakeMigrationStore(projectID, []LegacyTechnicalAction{fixtureLegacy(projectID, "https://example.com/a", "technical_fix", `{"issue":"robots"}`)})
	service := NewMigrationService(store)
	dry, _ := service.DryRun(context.Background(), projectID, "admin")
	store.rows[0].Evidence = []byte(`{"issue":"changed"}`)
	if _, err := service.Apply(context.Background(), projectID, dry.SnapshotHash, "admin"); !errors.Is(err, ErrMigrationSnapshotDrift) {
		t.Fatalf("apply error=%v want snapshot drift", err)
	}
	store.rows[0].Evidence = []byte(`{"issue":"robots"}`)
	dry, _ = service.DryRun(context.Background(), projectID, "admin")
	batch, err := service.Apply(context.Background(), projectID, dry.SnapshotHash, "admin")
	if err != nil {
		t.Fatal(err)
	}
	store.unmappableCanonicalWrite = true
	if _, err := service.Rollback(context.Background(), projectID, batch.BatchID, batch.ResultSnapshotHash, "admin"); !errors.Is(err, ErrMigrationRollbackBlocked) {
		t.Fatalf("rollback error=%v want lossy projection block", err)
	}
	if store.fenced || store.authority != "canonical" {
		t.Fatal("blocked rollback must preserve the original canonical writer without stranding its fence")
	}
}

func fixtureLegacy(projectID uuid.UUID, target, family, evidence string) LegacyTechnicalAction {
	return LegacyTechnicalAction{ProjectID: projectID, OpportunityID: uuid.New(), ActionID: uuid.New(), TargetURL: target,
		ChangeFamily: family, Status: "approved", Evidence: []byte(evidence), ApplicationIDs: []uuid.UUID{uuid.New()}}
}

func fixtureLegacyOpportunity(projectID uuid.UUID, opportunityType, target, evidence string) LegacyTechnicalAction {
	row := fixtureLegacy(projectID, target, opportunityType, evidence)
	row.OpportunityType = opportunityType
	row.OpportunityEvidence = json.RawMessage(evidence)
	return row
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// fakeMigrationStore is deliberately stateful: it verifies the service's fence,
// authority ordering and inverse contract rather than mocking individual calls.
type fakeMigrationStore struct {
	projectID                uuid.UUID
	rows                     []LegacyTechnicalAction
	authority                string
	fenced                   bool
	siteFixes                map[uuid.UUID]*MigrationFixtureState
	ledger                   []MigrationLedgerOperation
	unmappableCanonicalWrite bool
}

func newFakeMigrationStore(projectID uuid.UUID, rows []LegacyTechnicalAction) *fakeMigrationStore {
	return &fakeMigrationStore{projectID: projectID, rows: rows, authority: "legacy", siteFixes: map[uuid.UUID]*MigrationFixtureState{}}
}

func (f *fakeMigrationStore) Snapshot(_ context.Context, projectID uuid.UUID) ([]LegacyTechnicalAction, string, error) {
	if projectID != f.projectID {
		return nil, "", errors.New("project mismatch")
	}
	return append([]LegacyTechnicalAction(nil), f.rows...), f.authority, nil
}
func (f *fakeMigrationStore) Apply(_ context.Context, plan MigrationPlan) (MigrationBatchReport, error) {
	if f.fenced || f.authority != "legacy" {
		return MigrationBatchReport{}, errors.New("invalid initial authority")
	}
	f.fenced = true
	for _, item := range plan.Items {
		if item.Disposition != MigrationDispositionMigrate {
			continue
		}
		if item.SiteFixID == uuid.Nil {
			item.SiteFixID = uuid.New()
		}
		state := &MigrationFixtureState{FindingID: uuid.New(), SignatureID: uuid.New(), SiteFixID: item.SiteFixID, Status: "proposed", LegacyReadOnly: true, AliasState: "active", ApplicationSource: "site_fix"}
		f.siteFixes[item.LegacyActionID] = state
		f.ledger = append(f.ledger, MigrationLedgerOperation{OperationVersion: "site-fix-migration/v1", InverseOperationVersion: "site-fix-migration-inverse/v1", InverseOperation: []byte(`{"operation":"restore_legacy"}`)})
	}
	f.authority = "canonical"
	f.fenced = false
	return MigrationBatchReport{BatchID: plan.BatchID, ProjectID: plan.ProjectID, SourceCount: plan.SourceCount, MigratedCount: plan.MigratedCount, ArchivedDuplicateCount: plan.ArchivedDuplicateCount, ReviewCount: plan.ReviewCount, WriterAuthority: f.authority, ResultSnapshotHash: plan.SnapshotHash}, nil
}
func (f *fakeMigrationStore) Rollback(_ context.Context, projectID, batchID uuid.UUID, expected string, initiatedBy string) (MigrationRollbackReport, error) {
	f.fenced = true
	if f.unmappableCanonicalWrite {
		f.fenced = false
		return MigrationRollbackReport{}, ErrMigrationRollbackBlocked
	}
	for _, state := range f.siteFixes {
		state.Status = "migration_rolled_back"
		state.AliasState = "rolled_back_tombstone"
		state.LegacyReadOnly = false
		state.ApplicationSource = "content_action"
	}
	f.authority = "legacy"
	f.fenced = false
	return MigrationRollbackReport{BatchID: batchID, ProjectID: projectID, RolledBack: true, WriterAuthority: f.authority}, nil
}
func (f *fakeMigrationStore) Report(_ context.Context, projectID, batchID uuid.UUID) (MigrationBatchReport, error) {
	return MigrationBatchReport{ProjectID: projectID, BatchID: batchID}, nil
}
func (f *fakeMigrationStore) assertOneSourceAndAliases() error {
	for _, state := range f.siteFixes {
		if state.ApplicationSource != "site_fix" || state.AliasState != "active" {
			return errors.New("forward source/alias invariant failed")
		}
	}
	return nil
}
func (f *fakeMigrationStore) assertRestored() error {
	for _, state := range f.siteFixes {
		if state.ApplicationSource != "content_action" || state.LegacyReadOnly {
			return errors.New("rollback did not restore legacy source")
		}
	}
	return nil
}
