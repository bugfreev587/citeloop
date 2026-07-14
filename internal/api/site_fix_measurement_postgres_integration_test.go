//go:build integration

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSiteFixMeasurementTransactionsPostgres(t *testing.T) {
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

	t.Run("approval rolls back lifecycle when measurement insert fails then concurrent retry is idempotent", func(t *testing.T) {
		approvedAt := time.Now().UTC().Truncate(time.Second)
		projectID, fixID := insertMeasurementTransactionFixture(t, ctx, pool, "proposed", "reserved", true, approvedAt)
		service := &postgresDoctorSiteFixService{
			q:          poolQueries(pool),
			approvalTx: faultAfterApprovalRunner{pool: pool},
		}
		if _, err := service.Approve(ctx, projectID, fixID, approvedAt); err == nil {
			t.Fatal("faulted approval unexpectedly committed")
		}
		assertFixAndMeasurementCounts(t, ctx, pool, projectID, fixID, "proposed", 0, 0)

		service.approvalTx = postgresCanonicalSiteFixApprovalTransactionRunner{pool: pool}
		responses := make([]DoctorSiteFixResponse, 2)
		errorsSeen := make([]error, 2)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for index := range responses {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				<-start
				responses[index], errorsSeen[index] = service.Approve(ctx, projectID, fixID, approvedAt)
			}(index)
		}
		close(start)
		wg.Wait()
		for index, approveErr := range errorsSeen {
			if approveErr != nil || responses[index].Status != "approved" {
				t.Fatalf("approval %d response=%+v err=%v", index, responses[index], approveErr)
			}
		}
		assertFixAndMeasurementCounts(t, ctx, pool, projectID, fixID, "approved", 1, 0)
	})

	t.Run("optional opt-in rolls back generation when outbox insert fails then concurrent retry is idempotent", func(t *testing.T) {
		optedInAt := time.Now().UTC().Truncate(time.Second)
		projectID, fixID := insertMeasurementTransactionFixture(t, ctx, pool, "verified", "verified", false, optedInAt.Add(-24*time.Hour))
		service := &postgresDoctorSiteFixService{measurementOptInTx: faultAfterMeasurementCreateRunner{pool: pool}}
		if _, err := service.OptInMeasurement(ctx, projectID, fixID, optedInAt); err == nil {
			t.Fatal("faulted opt-in unexpectedly committed")
		}
		assertFixAndMeasurementCounts(t, ctx, pool, projectID, fixID, "verified", 0, 0)

		service.measurementOptInTx = postgresCanonicalSiteFixMeasurementOptInTransactionRunner{pool: pool}
		responses := make([]DoctorSiteFixMeasurementOptInResponse, 2)
		errorsSeen := make([]error, 2)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for index := range responses {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				<-start
				responses[index], errorsSeen[index] = service.OptInMeasurement(ctx, projectID, fixID, optedInAt)
			}(index)
		}
		close(start)
		wg.Wait()
		for index, optInErr := range errorsSeen {
			if optInErr != nil || responses[index].Measurement.ID == uuid.Nil || responses[index].Handoff.Status != "pending" {
				t.Fatalf("opt-in %d response=%+v err=%v", index, responses[index], optInErr)
			}
		}
		if responses[0].Measurement.ID != responses[1].Measurement.ID || responses[0].Measurement.MeasurementGeneration != 1 || responses[1].Measurement.MeasurementGeneration != 1 {
			t.Fatalf("concurrent opt-in allocated multiple generations: %+v %+v", responses[0], responses[1])
		}
		assertFixAndMeasurementCounts(t, ctx, pool, projectID, fixID, "verified", 1, 1)
	})
}

type faultAfterApprovalRunner struct{ pool *pgxpool.Pool }

func (r faultAfterApprovalRunner) Run(ctx context.Context, fn func(canonicalSiteFixApprovalMeasurementStore) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	err = fn(&faultAfterApprovalStore{Queries: db.New(tx)})
	if err == nil {
		return errors.New("expected injected measurement insert failure")
	}
	return err
}

type faultAfterApprovalStore struct{ *db.Queries }

func (s *faultAfterApprovalStore) CreateSiteFixMeasurement(context.Context, db.CreateSiteFixMeasurementParams) (db.SiteFixMeasurement, error) {
	return db.SiteFixMeasurement{}, errors.New("injected measurement insert failure after approval write")
}

type faultAfterMeasurementCreateRunner struct{ pool *pgxpool.Pool }

func (r faultAfterMeasurementCreateRunner) Run(ctx context.Context, fn func(canonicalSiteFixMeasurementOptInStore) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	err = fn(&faultAfterMeasurementCreateStore{Queries: db.New(tx)})
	if err == nil {
		return errors.New("expected injected outbox failure")
	}
	return err
}

type faultAfterMeasurementCreateStore struct{ *db.Queries }

func (s *faultAfterMeasurementCreateStore) EnqueueSiteFixMeasurementHandoff(context.Context, db.EnqueueSiteFixMeasurementHandoffParams) (db.SiteFixMeasurementHandoffOutbox, error) {
	return db.SiteFixMeasurementHandoffOutbox{}, errors.New("injected outbox failure after measurement write")
}

func poolQueries(pool *pgxpool.Pool) *db.Queries { return db.New(pool) }

func insertMeasurementTransactionFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fixStatus, signatureStatus string, required bool, referenceTime time.Time) (uuid.UUID, uuid.UUID) {
	t.Helper()
	projectID, runID, findingID := uuid.New(), uuid.New(), uuid.New()
	shadowID, candidateID, bucketID, signatureID, fixID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	suffix := strings.ReplaceAll(projectID.String(), "-", "")
	policy := json.RawMessage(`{"policy_version":"site-fix-growth-v1","early_signal_offset_days":7,"primary_checkpoint_offset_days":28,"follow_up_offsets_days":[42],"max_follow_up_attempts":1,"max_measuring_duration_days":56,"minimum_sample":{"minimum_after_periods":7,"minimum_after_sample":100},"metric_thresholds":{"direction":"increase","kind":"relative","value":0.05},"guardrails":[{"metric":"impressions","max_adverse_relative":0.15}],"required_data_sources":["gsc"],"terminalization_grace_period_days":2}`)
	hypothesis := "A clearer title will improve qualified organic CTR without reducing impressions."
	plan, err := json.Marshal(map[string]any{
		"growth_hypothesis": hypothesis, "primary_metric": "ctr", "secondary_metrics": []string{"impressions", "clicks", "position"},
		"target_query": "social publishing api", "target_identity": map[string]any{},
		"baseline_window":     map[string]any{"start": referenceTime.Add(-28 * 24 * time.Hour).Format(time.RFC3339), "end": referenceTime.Add(-time.Hour).Format(time.RFC3339)},
		"baseline_snapshot":   map[string]any{"ctr": 0.04, "impressions": 1200, "clicks": 48, "position": 7.2},
		"baseline_provenance": map[string]any{"source": "gsc", "captured_at": referenceTime.Add(-30 * time.Minute).Format(time.RFC3339)},
		"policy_snapshot":     policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	measurementPolicy, fixType := "measurement_optional", "metadata_rewrite"
	if required {
		measurementPolicy, fixType = "measurement_required", "metadata_ctr_optimization"
	}
	statements := []struct {
		sql  string
		args []any
	}{
		{`insert into projects(id,owner_id,name,slug,config) values($1,'integration','integration',$2,'{}')`, []any{projectID, "task3-quality-" + suffix}},
		{`update product_writer_authority set writer_authority='canonical',write_fenced=false,authority_changed_at=now(),updated_at=now() where project_id=$1 and product='doctor'`, []any{projectID}},
		{`insert into seo_doctor_runs(id,project_id,trigger,status) values($1,$2,'manual','completed')`, []any{runID, projectID}},
		{`insert into seo_doctor_findings(id,project_id,run_id,finding_key,severity,category,issue_type,status,finding_kind) values($1,$2,$3,$4,'P1','metadata','metadata_ctr_optimization','active','optimization')`, []any{findingID, projectID, runID, "quality-" + suffix}},
		{`insert into discovery_shadow_runs(id,project_id,mode,status,candidate_schema_version,signature_version) values($1,$2,'canonical','completed','v1','v1')`, []any{shadowID, projectID}},
		{`insert into discovery_candidates(id,project_id,shadow_run_id,source_kind,source_object_type,source_object_id,target_kind,issue_or_hypothesis_family,change_family,artifact_intent,verification_mode,suggested_owner,candidate_schema_version,status,evidence_fingerprint,exact_signature_hash,signature_payload,conflict_bucket_keys) values($1,$2,$3,'doctor','seo_doctor_finding',$4,'page','metadata_ctr_optimization','metadata.title','repair_existing_surface','immediate','doctor','v1','identity_ready','fixture-evidence',$5,'{}','["fixture-bucket"]')`, []any{candidateID, projectID, shadowID, findingID.String(), "fixture-signature-" + suffix}},
		{`insert into work_conflict_buckets(id,project_id,bucket_key) values($1,$2,'fixture-bucket')`, []any{bucketID, projectID}},
		{`insert into work_signature_registry(id,project_id,candidate_id,shadow_run_id,mode,status,active,exact_signature_hash,signature_payload,conflict_bucket_keys,signature_version,owner,source_object_type,source_object_id,reserved_work_type,reserved_work_id,evidence_fingerprint) values($1,$2,$3,$4,'enforced',$5,$6,$7,'{}','["fixture-bucket"]','v1','doctor','seo_doctor_finding',$8,'site_fix',$9,'fixture-evidence')`, []any{signatureID, projectID, candidateID, shadowID, signatureStatus, fixStatus != "verified", "fixture-signature-" + suffix, findingID.String(), fixID}},
		{`insert into site_fixes(id,project_id,doctor_finding_id,candidate_id,work_signature_id,status,finding_kind,target_urls,evidence_snapshot,proposed_fix,acceptance_tests,fix_type,impact_mode,measurement_policy,classifier_version,decision_origin,decision_confidence,growth_hypothesis,primary_metric,secondary_metrics,measurement_policy_version,measurement_policy_snapshot,measurement_plan_snapshot,approved_at,verified_at) values($1,$2,$3,$4,$5,$6,'optimization','["https://example.com/pricing"]','{}','{}','[{"type":"title_present"}]',$7,$8,$9,'site-fix-classifier-v1','system_rule','high',$10,'ctr','["impressions","clicks","position"]','site-fix-growth-v1',$11,$12,$13,$14)`, []any{fixID, projectID, findingID, candidateID, signatureID, fixStatus, fixType, map[bool]string{true: "conversion_or_ctr", false: "search_visibility"}[required], measurementPolicy, hypothesis, policy, plan, nullableFixtureTime(fixStatus != "proposed", referenceTime), nullableFixtureTime(fixStatus == "verified", referenceTime)}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			_, _ = pool.Exec(ctx, `delete from projects where id=$1`, projectID)
			t.Fatalf("fixture insert failed (%s): %v", fmt.Sprintf("%.72s", statement.sql), err)
		}
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `delete from projects where id=$1`, projectID) })
	return projectID, fixID
}

func nullableFixtureTime(valid bool, value time.Time) any {
	if !valid {
		return nil
	}
	return value
}

func assertFixAndMeasurementCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID, fixID uuid.UUID, wantStatus string, wantMeasurements, wantOutbox int) {
	t.Helper()
	var status string
	var measurements, outbox int
	if err := pool.QueryRow(ctx, `select status from site_fixes where project_id=$1 and id=$2`, projectID, fixID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `select count(*) from site_fix_measurements where project_id=$1 and site_fix_id=$2`, projectID, fixID).Scan(&measurements); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `select count(*) from site_fix_measurement_handoff_outbox where project_id=$1 and site_fix_id=$2`, projectID, fixID).Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || measurements != wantMeasurements || outbox != wantOutbox {
		t.Fatalf("status=%s measurements=%d outbox=%d; want %s/%d/%d", status, measurements, outbox, wantStatus, wantMeasurements, wantOutbox)
	}
}

var _ canonicalSiteFixApprovalMeasurementStore = (*faultAfterApprovalStore)(nil)
var _ canonicalSiteFixMeasurementOptInStore = (*faultAfterMeasurementCreateStore)(nil)
