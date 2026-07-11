//go:build integration

package sitefix

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresMigrationConservationMatrixAndRollbackBlocker(t *testing.T) {
	dsn := os.Getenv("CITELOOP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CITELOOP_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	projectID := uuid.New()
	if _, err := pool.Exec(ctx, `insert into projects(id,owner_id,name,slug,config) values($1,'migration-matrix','migration matrix',$2,'{}')`, projectID, "migration-matrix-"+projectID.String()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `delete from projects where id=$1`, projectID) })

	firstOpportunity, secondOpportunity, ambiguousOpportunity, dismissedOpportunity := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	firstAction, secondAction, applicationID, reviewStateID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	ambiguousAction, ambiguousApplication := uuid.New(), uuid.New()
	statements := []struct {
		sql  string
		args []any
	}{
		{`insert into seo_opportunities(id,project_id,type,status,page_url,normalized_page_url,evidence) values($1,$2,'schema_gap','accepted','https://example.com/a','https://example.com/a','{"issue":"missing_schema"}')`, []any{firstOpportunity, projectID}},
		{`insert into content_actions(id,project_id,opportunity_id,action_type,status,target_url,normalized_target_url,asset_type,evidence_snapshot) values($1,$2,$3,'schema_patch','approved','https://example.com/a','https://example.com/a','schema_patch','{"issue":"missing_schema"}')`, []any{firstAction, projectID, firstOpportunity}},
		{`insert into site_change_applications(id,project_id,content_action_id,application_kind,target_url,normalized_target_url,opportunity_key,status) values($1,$2,$3,'site_fix','https://example.com/a','https://example.com/a',$4,'ready_for_pr')`, []any{applicationID, projectID, firstAction, "legacy-" + applicationID.String()}},
		{`insert into seo_opportunities(id,project_id,type,status,page_url,normalized_page_url,evidence) values($1,$2,'schema_gap','accepted','https://example.com/a','https://example.com/a','{"issue":"missing_schema"}')`, []any{secondOpportunity, projectID}},
		{`insert into content_actions(id,project_id,opportunity_id,action_type,status,target_url,normalized_target_url,asset_type,evidence_snapshot) values($1,$2,$3,'schema_patch','approved','https://example.com/a','https://example.com/a','schema_patch','{"issue":"missing_schema"}')`, []any{secondAction, projectID, secondOpportunity}},
		{`insert into seo_opportunities(id,project_id,type,status,normalized_page_url,evidence) values($1,$2,'schema_gap','open','','{"issue":"missing_schema"}')`, []any{ambiguousOpportunity, projectID}},
		{`insert into content_actions(id,project_id,opportunity_id,action_type,status,asset_type,evidence_snapshot) values($1,$2,$3,'schema_patch','approved','schema_patch','{"issue":"missing_schema"}')`, []any{ambiguousAction, projectID, ambiguousOpportunity}},
		{`insert into site_change_applications(id,project_id,content_action_id,application_kind,target_url,normalized_target_url,opportunity_key,status) values($1,$2,$3,'site_fix','https://example.com/ambiguous','https://example.com/ambiguous',$4,'github_pr_open')`, []any{ambiguousApplication, projectID, ambiguousAction, "legacy-" + ambiguousApplication.String()}},
		{`insert into seo_opportunities(id,project_id,type,status,page_url,normalized_page_url,evidence,opportunity_identity_key,evidence_fingerprint) values($1,$2,'canonical_missing','dismissed','https://example.com/b','https://example.com/b','{"issue":"canonical_missing"}',$3,'review-evidence')`, []any{dismissedOpportunity, projectID, "review-" + dismissedOpportunity.String()}},
		{`insert into seo_opportunity_review_states(id,project_id,opportunity_identity_key,source_opportunity_id,review_status,evidence_fingerprint,reviewed_by) values($1,$2,$3,$4,'dismissed','review-evidence','integration')`, []any{reviewStateID, projectID, "review-" + dismissedOpportunity.String(), dismissedOpportunity}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("fixture %s: %v", statement.sql, err)
		}
	}

	service := NewMigrationService(NewPostgresMigrationStore(pool))
	dry, err := service.DryRun(ctx, projectID, "integration")
	if err != nil {
		t.Fatal(err)
	}
	if dry.SourceCount != 4 || dry.MigratedCount != 2 || dry.ArchivedDuplicateCount != 1 || dry.ReviewCount != 1 {
		t.Fatalf("dry=%+v", dry)
	}
	first, err := service.Apply(ctx, projectID, dry.SnapshotHash, "integration")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Invariants.Passed {
		t.Fatalf("invariants=%+v", first.Invariants)
	}
	var appContentAction, appSiteFix *uuid.UUID
	if err := pool.QueryRow(ctx, `select content_action_id,site_fix_id from site_change_applications where id=$1`, applicationID).Scan(&appContentAction, &appSiteFix); err != nil {
		t.Fatal(err)
	}
	if appContentAction != nil || appSiteFix == nil {
		t.Fatalf("application source action=%v fix=%v", appContentAction, appSiteFix)
	}
	var reviewAliases int
	if err := pool.QueryRow(ctx, `select count(*) from legacy_object_aliases where project_id=$1 and legacy_object_type='seo_opportunity_review_state' and legacy_object_id=$2 and alias_state='active'`, projectID, reviewStateID).Scan(&reviewAliases); err != nil || reviewAliases != 1 {
		t.Fatalf("review aliases=%d err=%v", reviewAliases, err)
	}
	if _, err := pool.Exec(ctx, `insert into seo_opportunities(project_id,type,status,normalized_page_url,evidence) values($1,'schema_gap','open','https://example.com/race','{"issue":"missing_schema"}')`, projectID); err == nil {
		t.Fatal("technical legacy writer escaped canonical authority")
	}
	if _, err := pool.Exec(ctx, `update site_change_applications set status='needs_follow_up' where id=$1`, ambiguousApplication); err == nil {
		t.Fatal("legacy review application worker escaped canonical authority")
	}
	if _, err := service.Rollback(ctx, projectID, first.BatchID, first.ResultSnapshotHash, "integration"); err != nil {
		t.Fatal(err)
	}
	rolledReport, err := service.Report(ctx, projectID, first.BatchID)
	if err != nil || rolledReport.Status != "rolled_back" || len(rolledReport.RollbackBlockers) != 0 {
		t.Fatalf("rolled report=%+v err=%v", rolledReport, err)
	}

	dry, err = service.DryRun(ctx, projectID, "integration")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Apply(ctx, projectID, dry.SnapshotHash, "integration")
	if err != nil {
		t.Fatal(err)
	}
	var migratedFix uuid.UUID
	if err := pool.QueryRow(ctx, `select id from site_fixes where project_id=$1 and migration_batch_id=$2 order by id limit 1`, projectID, second.BatchID).Scan(&migratedFix); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `insert into site_fix_verifications(id,project_id,site_fix_id,attempt_number,evidence_read,acceptance_results,result,retry_classification,attempted_at) values($1,$2,$3,1,'{}','[]','failed','retryable',now())`, uuid.New(), projectID, migratedFix); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Rollback(ctx, projectID, second.BatchID, second.ResultSnapshotHash, "integration"); !errors.Is(err, ErrMigrationRollbackBlocked) {
		t.Fatalf("rollback blocker err=%v", err)
	}
}

func TestPostgresExpandedTechnicalPredicateSelectsAndFencesEveryDoctorType(t *testing.T) {
	dsn := os.Getenv("CITELOOP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CITELOOP_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	projectID := uuid.New()
	if _, err := pool.Exec(ctx, `insert into projects(id,owner_id,name,slug,config) values($1,'predicate-matrix','predicate matrix',$2,'{}')`, projectID, "predicate-matrix-"+projectID.String()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `update product_writer_authority set writer_authority='legacy',write_fenced=false where project_id=$1 and product='doctor'`, projectID)
		_, _ = pool.Exec(context.Background(), `delete from projects where id=$1`, projectID)
	})
	types := []string{
		"geo_crawler_access_blocked", "meta_description_missing", "metadata_description", "h1_missing",
		"important_page_missing_from_sitemap", "unsafe_mdx_detected", "metadata_readability",
		"duplicate_metadata_template", "supported_fact_extractability", "source_association", "entity_naming_consistency",
	}
	for i, opportunityType := range types {
		page := fmt.Sprintf("https://example.com/%d", i)
		evidence := fmt.Sprintf(`{"issue":%q}`, opportunityType)
		if _, err := pool.Exec(ctx, `insert into seo_opportunities(project_id,type,status,page_url,normalized_page_url,evidence) values($1,$2,'open',$3,$3,$4)`, projectID, opportunityType, page, evidence); err != nil {
			t.Fatalf("legacy insert %s: %v", opportunityType, err)
		}
	}
	dry, err := NewMigrationService(NewPostgresMigrationStore(pool)).DryRun(ctx, projectID, "integration")
	if err != nil {
		t.Fatal(err)
	}
	if dry.SourceCount != len(types) {
		t.Fatalf("selected source count = %d, want %d; report=%+v", dry.SourceCount, len(types), dry)
	}
	if dry.MigratedCount != len(types) || dry.ReviewCount != 0 {
		t.Fatalf("expanded types are not migration-ready: %+v", dry)
	}
	if _, err := pool.Exec(ctx, `update product_writer_authority set writer_authority='canonical',authority_changed_at=now(),updated_at=now() where project_id=$1 and product='doctor'`, projectID); err != nil {
		t.Fatal(err)
	}
	for i, opportunityType := range types {
		page := fmt.Sprintf("https://example.com/new-%d", i)
		evidence := fmt.Sprintf(`{"issue":%q}`, opportunityType)
		if _, err := pool.Exec(ctx, `insert into seo_opportunities(project_id,type,status,page_url,normalized_page_url,evidence) values($1,$2,'open',$3,$3,$4)`, projectID, opportunityType, page, evidence); err == nil {
			t.Fatalf("canonical writer fence allowed %s", opportunityType)
		}
	}
}

// TestPostgresMigrationApplyRollbackReapply is the production-shaped Task 6
// rehearsal. Task 9 runs it against an isolated migrated PostgreSQL database;
// ordinary CI remains hermetic when no fixture DSN is supplied.
func TestPostgresMigrationApplyRollbackReapply(t *testing.T) {
	dsn := os.Getenv("CITELOOP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CITELOOP_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	projectID, opportunityID := uuid.New(), uuid.New()
	suffix := projectID.String()
	if _, err := pool.Exec(ctx, `insert into projects(id,owner_id,name,slug,config) values($1,'migration-integration','migration integration',$2,'{}')`, projectID, "migration-"+suffix); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `delete from projects where id=$1`, projectID) })
	if _, err := pool.Exec(ctx, `insert into seo_opportunities(id,project_id,type,status,page_url,normalized_page_url,evidence,recommended_action) values($1,$2,'schema_gap','open','https://example.com/product','https://example.com/product','{"issue":"missing_schema"}','Add Product JSON-LD')`, opportunityID, projectID); err != nil {
		t.Fatal(err)
	}

	service := NewMigrationService(NewPostgresMigrationStore(pool))
	dry, err := service.DryRun(ctx, projectID, "integration")
	if err != nil {
		t.Fatal(err)
	}
	if dry.SourceCount != 1 || dry.MigratedCount != 1 {
		t.Fatalf("dry-run=%+v", dry)
	}
	first, err := service.Apply(ctx, projectID, dry.SnapshotHash, "integration")
	if err != nil {
		t.Fatal(err)
	}
	assertPostgresMigrationState(t, ctx, pool, projectID, opportunityID, first.BatchID, "canonical", true, 1, 0)

	if _, err := service.Rollback(ctx, projectID, first.BatchID, first.ResultSnapshotHash, "integration"); err != nil {
		t.Fatal(err)
	}
	assertPostgresMigrationState(t, ctx, pool, projectID, opportunityID, first.BatchID, "legacy", false, 0, 1)

	dry, err = service.DryRun(ctx, projectID, "integration")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Apply(ctx, projectID, dry.SnapshotHash, "integration")
	if err != nil {
		t.Fatal(err)
	}
	if second.BatchID == first.BatchID {
		t.Fatal("reapply must use a fresh batch")
	}
	assertPostgresMigrationState(t, ctx, pool, projectID, opportunityID, second.BatchID, "canonical", true, 1, 1)
}

func assertPostgresMigrationState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID, opportunityID, batchID uuid.UUID, authority string, readOnly bool, activeAliases, tombstoneAliases int) {
	t.Helper()
	var gotAuthority string
	var gotReadOnly bool
	var gotBatch *uuid.UUID
	if err := pool.QueryRow(ctx, `select p.writer_authority,o.canonical_read_only,o.legacy_migration_batch_id from product_writer_authority p join seo_opportunities o on o.project_id=p.project_id where p.project_id=$1 and p.product='doctor' and o.id=$2`, projectID, opportunityID).Scan(&gotAuthority, &gotReadOnly, &gotBatch); err != nil {
		t.Fatal(err)
	}
	if gotAuthority != authority || gotReadOnly != readOnly {
		t.Fatalf("authority=%s read_only=%v", gotAuthority, gotReadOnly)
	}
	if readOnly && (gotBatch == nil || *gotBatch != batchID) {
		t.Fatalf("migration batch=%v want %s", gotBatch, batchID)
	}
	var active, tombstones int
	if err := pool.QueryRow(ctx, `select count(*) filter(where alias_state='active'),count(*) filter(where alias_state='rolled_back_tombstone') from legacy_object_aliases where project_id=$1 and legacy_object_type='seo_opportunity' and legacy_object_id=$2`, projectID, opportunityID).Scan(&active, &tombstones); err != nil {
		t.Fatal(err)
	}
	if active != activeAliases || tombstones != tombstoneAliases {
		t.Fatalf("aliases active=%d tombstones=%d", active, tombstones)
	}
}
