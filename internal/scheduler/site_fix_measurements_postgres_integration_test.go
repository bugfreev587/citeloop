//go:build integration

package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/measurement"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTickSiteFixMeasurementsPostgresEndToEnd(t *testing.T) {
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
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	scheduler := New(pool, nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	scheduler.now = func() time.Time { return now }
	scheduler.alert = func(uuid.UUID, string) {}
	positive := func(context.Context, *db.Queries, db.SiteFixMeasurement, db.SiteFixMeasurementCheckpoint, time.Time) (measurement.EvidenceEvaluation, error) {
		return measurement.EvidenceEvaluation{Evaluation: measurement.Evaluation{OutcomeLabel: measurement.OutcomePositive, OutcomeReason: "frozen threshold crossed", AttributionConfidence: "high", DataQualityState: measurement.QualityComplete, Confounders: []string{"fixture"}, GuardrailResults: map[string]any{}}, SEOMetrics: json.RawMessage(`{"ctr":0.2}`), GA4Metrics: json.RawMessage(`{}`), GEOMetrics: json.RawMessage(`{}`), SourceFreshness: json.RawMessage(`{"gsc":"fresh"}`)}, nil
	}

	t.Run("positive closes future checkpoints and records one learning", func(t *testing.T) {
		projectID, fixID := insertSchedulerSiteFixFixture(t, ctx, pool, now.Add(-2*24*time.Hour), "measurement_optional")
		row := createSchedulerMeasurement(t, ctx, pool, projectID, fixID, now.Add(-2*24*time.Hour), false, "positive")
		scheduler.siteFixEvidenceOverride = positive
		if err := scheduler.TickSiteFixMeasurements(ctx); err != nil {
			t.Fatal(err)
		}
		assertSchedulerTerminal(t, ctx, pool, row, "positive", 1, 0)
		var open int
		if err := pool.QueryRow(ctx, `select count(*) from site_fix_measurement_checkpoints where measurement_id=$1 and computed_at is null`, row.ID).Scan(&open); err != nil || open != 0 {
			t.Fatalf("open=%d err=%v", open, err)
		}
		var skipped int
		if err := pool.QueryRow(ctx, `select count(*) from site_fix_measurement_checkpoints where measurement_id=$1 and outcome_reason='skipped_after_terminal_outcome'`, row.ID).Scan(&skipped); err != nil || skipped == 0 {
			t.Fatalf("skipped=%d err=%v", skipped, err)
		}
		var fixStatus string
		_ = pool.QueryRow(ctx, `select status from site_fixes where id=$1`, fixID).Scan(&fixStatus)
		if fixStatus != "verified" {
			t.Fatalf("fix status=%s", fixStatus)
		}
		var deepLink string
		if err := pool.QueryRow(ctx, `select results_deep_link from site_fix_measurements where id=$1`, row.ID).Scan(&deepLink); err != nil || deepLink != "/projects/"+projectID.String()+"/results?source_type=site_fix&measurement="+row.ID.String() {
			t.Fatalf("deep_link=%q err=%v", deepLink, err)
		}
	})

	t.Run("primary insufficient data waits for bounded follow up", func(t *testing.T) {
		started := now.Add(-2 * 24 * time.Hour)
		projectID, fixID := insertSchedulerSiteFixFixture(t, ctx, pool, started, "measurement_optional")
		row := createSchedulerMeasurement(t, ctx, pool, projectID, fixID, started, false, "bounded-follow-up")
		scheduler.siteFixEvidenceOverride = func(context.Context, *db.Queries, db.SiteFixMeasurement, db.SiteFixMeasurementCheckpoint, time.Time) (measurement.EvidenceEvaluation, error) {
			return schedulerEvidence(measurement.OutcomeInsufficientData), nil
		}
		if err := scheduler.TickSiteFixMeasurements(ctx); err != nil {
			t.Fatal(err)
		}
		var status string
		var primaryComputed bool
		if err := pool.QueryRow(ctx, `select status from site_fix_measurements where id=$1`, row.ID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `select computed_at is not null from site_fix_measurement_checkpoints where measurement_id=$1 and checkpoint_role='primary'`, row.ID).Scan(&primaryComputed); err != nil {
			t.Fatal(err)
		}
		if status != "observing" || !primaryComputed {
			t.Fatalf("after primary status=%s primary_computed=%v", status, primaryComputed)
		}

		now = now.Add(24 * time.Hour)
		scheduler.siteFixEvidenceOverride = func(context.Context, *db.Queries, db.SiteFixMeasurement, db.SiteFixMeasurementCheckpoint, time.Time) (measurement.EvidenceEvaluation, error) {
			return schedulerEvidence(measurement.OutcomeNegative), nil
		}
		if err := scheduler.TickSiteFixMeasurements(ctx); err != nil {
			t.Fatal(err)
		}
		assertSchedulerTerminal(t, ctx, pool, row, measurement.OutcomeNegative, 1, 0)
		var open int
		if err := pool.QueryRow(ctx, `select count(*) from site_fix_measurement_checkpoints where measurement_id=$1 and computed_at is null`, row.ID).Scan(&open); err != nil || open != 0 {
			t.Fatalf("open=%d err=%v", open, err)
		}
		now = now.Add(-24 * time.Hour)
	})

	for _, outcome := range []string{measurement.OutcomeNegative, measurement.OutcomeMixed, measurement.OutcomeInconclusive} {
		outcome := outcome
		t.Run("terminal taxonomy "+outcome, func(t *testing.T) {
			projectID, fixID := insertSchedulerSiteFixFixture(t, ctx, pool, now.Add(-2*24*time.Hour), "measurement_optional")
			row := createSchedulerMeasurement(t, ctx, pool, projectID, fixID, now.Add(-2*24*time.Hour), false, "taxonomy-"+outcome)
			scheduler.siteFixEvidenceOverride = func(context.Context, *db.Queries, db.SiteFixMeasurement, db.SiteFixMeasurementCheckpoint, time.Time) (measurement.EvidenceEvaluation, error) {
				return schedulerEvidence(outcome), nil
			}
			if err := scheduler.TickSiteFixMeasurements(ctx); err != nil {
				t.Fatal(err)
			}
			assertSchedulerTerminal(t, ctx, pool, row, outcome, 1, 0)
		})
	}

	t.Run("prospective remains non directional and quality only", func(t *testing.T) {
		projectID, fixID := insertSchedulerSiteFixFixture(t, ctx, pool, now.Add(-2*24*time.Hour), "measurement_optional")
		row := createSchedulerMeasurement(t, ctx, pool, projectID, fixID, now.Add(-2*24*time.Hour), true, "prospective")
		scheduler.siteFixEvidenceOverride = positive
		if err := scheduler.TickSiteFixMeasurements(ctx); err != nil {
			t.Fatal(err)
		}
		assertSchedulerTerminal(t, ctx, pool, row, "insufficient_data", 0, 1)
		var directional int
		if err := pool.QueryRow(ctx, `select count(*) from site_fix_measurement_checkpoints where measurement_id=$1 and outcome_label in ('positive','negative','mixed','inconclusive')`, row.ID).Scan(&directional); err != nil || directional != 0 {
			t.Fatalf("directional=%d err=%v", directional, err)
		}
	})

	t.Run("poison row defers and next measurement advances", func(t *testing.T) {
		p1, f1 := insertSchedulerSiteFixFixture(t, ctx, pool, now.Add(-2*24*time.Hour), "measurement_optional")
		poison := createSchedulerMeasurement(t, ctx, pool, p1, f1, now.Add(-2*24*time.Hour), false, "a-poison")
		p2, f2 := insertSchedulerSiteFixFixture(t, ctx, pool, now.Add(-2*24*time.Hour), "measurement_optional")
		healthy := createSchedulerMeasurement(t, ctx, pool, p2, f2, now.Add(-2*24*time.Hour), false, "b-healthy")
		scheduler.siteFixEvidenceOverride = func(ctx context.Context, q *db.Queries, row db.SiteFixMeasurement, checkpoint db.SiteFixMeasurementCheckpoint, at time.Time) (measurement.EvidenceEvaluation, error) {
			if row.ID == poison.ID {
				return measurement.EvidenceEvaluation{}, errors.New("secret provider detail")
			}
			return positive(ctx, q, row, checkpoint, at)
		}
		if err := scheduler.TickSiteFixMeasurements(ctx); err != nil {
			t.Fatal(err)
		}
		var attempts int
		var next time.Time
		var reason string
		if err := pool.QueryRow(ctx, `select evaluation_attempt_count,next_attempt_at,failure_reason from site_fix_measurement_checkpoints where measurement_id=$1 and checkpoint_role='early_signal'`, poison.ID).Scan(&attempts, &next, &reason); err != nil {
			t.Fatal(err)
		}
		if attempts != 1 || !next.After(now) || strings.Contains(reason, "secret") {
			t.Fatalf("attempts=%d next=%s reason=%s", attempts, next, reason)
		}
		assertSchedulerTerminal(t, ctx, pool, healthy, "positive", 1, 0)
	})

	t.Run("deadline error terminalizes quality with redacted reason", func(t *testing.T) {
		projectID, fixID := insertSchedulerSiteFixFixture(t, ctx, pool, now.Add(-10*24*time.Hour), "measurement_optional")
		row := createSchedulerMeasurement(t, ctx, pool, projectID, fixID, now.Add(-10*24*time.Hour), false, "deadline")
		scheduler.siteFixEvidenceOverride = func(context.Context, *db.Queries, db.SiteFixMeasurement, db.SiteFixMeasurementCheckpoint, time.Time) (measurement.EvidenceEvaluation, error) {
			return measurement.EvidenceEvaluation{}, errors.New("secret token")
		}
		if err := scheduler.TickSiteFixMeasurements(ctx); err != nil {
			t.Fatal(err)
		}
		assertSchedulerTerminal(t, ctx, pool, row, "insufficient_data", 0, 1)
		var reason string
		_ = pool.QueryRow(ctx, `select outcome_reason from site_fix_measurements where id=$1`, row.ID).Scan(&reason)
		if strings.Contains(reason, "secret") {
			t.Fatalf("unredacted reason=%s", reason)
		}
	})

	t.Run("reconcile activates every generation at immutable occurrence", func(t *testing.T) {
		verified := now.Add(-3 * time.Hour)
		projectID, fixID := insertSchedulerSiteFixFixture(t, ctx, pool, verified, "measurement_optional")
		first := createSchedulerMeasurement(t, ctx, pool, projectID, fixID, now.Add(-time.Hour), false, "multi-a")
		second := createSchedulerMeasurement(t, ctx, pool, projectID, fixID, now.Add(-time.Hour), true, "multi-b")
		scheduler.siteFixEvidenceOverride = positive
		if err := scheduler.TickSiteFixMeasurements(ctx); err != nil {
			t.Fatal(err)
		}
		for _, testCase := range []struct {
			row      db.SiteFixMeasurement
			occurred time.Time
		}{
			{row: first, occurred: verified},
			{row: second, occurred: second.CreatedAt.Time},
		} {
			var occurred, started time.Time
			var status string
			if err := pool.QueryRow(ctx, `select o.occurred_at,m.started_at,o.status from site_fix_measurement_handoff_outbox o join site_fix_measurements m on m.project_id=o.project_id and m.site_fix_id=o.site_fix_id and m.measurement_generation=o.measurement_generation where m.id=$1`, testCase.row.ID).Scan(&occurred, &started, &status); err != nil {
				t.Fatal(err)
			}
			if !occurred.Equal(testCase.occurred) || !started.Equal(testCase.occurred) || status != "completed" {
				t.Fatalf("row=%s occurred=%s started=%s want=%s status=%s", testCase.row.ID, occurred, started, testCase.occurred, status)
			}
		}
		var count int
		_ = pool.QueryRow(ctx, `select count(*) from site_fix_measurement_handoff_outbox where site_fix_id=$1`, fixID).Scan(&count)
		if count != 2 {
			t.Fatalf("handoffs=%d", count)
		}
	})
}

func schedulerEvidence(outcome string) measurement.EvidenceEvaluation {
	return measurement.EvidenceEvaluation{Evaluation: measurement.Evaluation{OutcomeLabel: outcome, OutcomeReason: "scheduler integration outcome " + outcome, AttributionConfidence: "high", DataQualityState: measurement.QualityComplete, Confounders: []string{"fixture"}, GuardrailResults: map[string]any{}}, SEOMetrics: json.RawMessage(`{"ctr":0.2}`), GA4Metrics: json.RawMessage(`{}`), GEOMetrics: json.RawMessage(`{}`), SourceFreshness: json.RawMessage(`{"gsc":"fresh"}`)}
}

func createSchedulerMeasurement(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID, fixID uuid.UUID, created time.Time, prospective bool, key string) db.SiteFixMeasurement {
	t.Helper()
	policy := json.RawMessage(`{"policy_version":"site-fix-growth-v1","early_signal_offset_days":1,"primary_checkpoint_offset_days":2,"follow_up_offsets_days":[3],"max_follow_up_attempts":1,"max_measuring_duration_days":3,"terminalization_grace_period_days":1,"metric_thresholds":{"direction":"increase","kind":"relative","value":0.1},"guardrails":[],"required_data_sources":["gsc"],"minimum_sample":{"minimum_after_periods":1,"minimum_after_sample":1}}`)
	baselineWindow, baselineSnapshot, baselineStatus, confidence := json.RawMessage(`{"start":"2026-06-01T00:00:00Z","end":"2026-06-07T00:00:00Z"}`), json.RawMessage(`{"ctr":{"value":0.1,"sample_size":1000,"rows":7,"partial":false}}`), "ready", "high"
	if prospective {
		baselineWindow, baselineSnapshot, baselineStatus, confidence = json.RawMessage(`{}`), json.RawMessage(`{}`), "unavailable", "low"
	}
	row, err := db.New(pool).CreateSiteFixMeasurement(ctx, db.CreateSiteFixMeasurementParams{ID: uuid.New(), ProjectID: projectID, SiteFixID: fixID, CreationIdempotencyKey: key, TargetUrl: "https://example.com/", NormalizedTargetUrl: "https://example.com/", TargetIdentity: json.RawMessage(`{}`), FixType: "metadata_ctr_optimization", ImpactMode: "conversion_or_ctr", ClassifierVersion: "v1", DecisionOrigin: "system_rule", DecisionConfidence: "high", ProspectiveObservation: prospective, GrowthHypothesis: "CTR improves.", PrimaryMetric: "ctr", SecondaryMetrics: json.RawMessage(`[]`), MeasurementPolicyVersion: "site-fix-growth-v1", MeasurementPolicySnapshot: policy, BaselineWindow: baselineWindow, BaselineSnapshot: baselineSnapshot, BaselineStatus: baselineStatus, Status: "ready", AttributionConfidence: confidence})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `update site_fix_measurements set created_at=$2,updated_at=$2 where id=$1`, row.ID, created); err != nil {
		t.Fatal(err)
	}
	row.CreatedAt = pgutil.TS(created)
	return row
}

func assertSchedulerTerminal(t *testing.T, ctx context.Context, pool *pgxpool.Pool, row db.SiteFixMeasurement, outcome string, learnings, qualities int) {
	t.Helper()
	var status, got string
	var learningCount, qualityCount int
	if err := pool.QueryRow(ctx, `select status,terminal_outcome from site_fix_measurements where id=$1`, row.ID).Scan(&status, &got); err != nil {
		t.Fatal(err)
	}
	_ = pool.QueryRow(ctx, `select count(*) from site_fix_measurement_learnings where measurement_id=$1`, row.ID).Scan(&learningCount)
	_ = pool.QueryRow(ctx, `select count(*) from site_fix_measurement_quality_records where measurement_id=$1`, row.ID).Scan(&qualityCount)
	if status != "terminal" || got != outcome || learningCount != learnings || qualityCount != qualities {
		t.Fatalf("status=%s outcome=%s learning=%d quality=%d", status, got, learningCount, qualityCount)
	}
}

func insertSchedulerSiteFixFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, verifiedAt time.Time, policy string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	projectID, runID, findingID, shadowID, candidateID, bucketID, signatureID, fixID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	suffix := strings.ReplaceAll(projectID.String(), "-", "")
	statements := []struct {
		sql  string
		args []any
	}{
		{`insert into projects(id,owner_id,name,slug,config) values($1,'integration','integration',$2,'{}')`, []any{projectID, "scheduler-" + suffix}},
		{`update product_writer_authority set writer_authority='canonical',authority_changed_at=now(),updated_at=now() where project_id=$1 and product='doctor'`, []any{projectID}},
		{`insert into seo_doctor_runs(id,project_id,trigger,status) values($1,$2,'manual','completed')`, []any{runID, projectID}},
		{`insert into seo_doctor_findings(id,project_id,run_id,finding_key,severity,category,issue_type,status,finding_kind) values($1,$2,$3,$4,'P1','technical','canonical','active','optimization')`, []any{findingID, projectID, runID, "scheduler-" + suffix}},
		{`insert into discovery_shadow_runs(id,project_id,mode,status,candidate_schema_version,signature_version) values($1,$2,'canonical','completed','v1','v1')`, []any{shadowID, projectID}},
		{`insert into discovery_candidates(id,project_id,shadow_run_id,source_kind,source_object_type,source_object_id,target_kind,issue_or_hypothesis_family,change_family,artifact_intent,verification_mode,suggested_owner,candidate_schema_version,status,evidence_fingerprint,exact_signature_hash,signature_payload,conflict_bucket_keys) values($1,$2,$3,'doctor','seo_doctor_finding',$4,'page','metadata','metadata_rewrite','repair_existing_surface','immediate','doctor','v1','identity_ready','fixture',$5,'{}','["fixture"]')`, []any{candidateID, projectID, shadowID, findingID.String(), "scheduler-" + suffix}},
		{`insert into work_conflict_buckets(id,project_id,bucket_key) values($1,$2,'fixture')`, []any{bucketID, projectID}},
		{`insert into work_signature_registry(id,project_id,candidate_id,shadow_run_id,mode,status,active,exact_signature_hash,signature_payload,conflict_bucket_keys,signature_version,owner,source_object_type,source_object_id,reserved_work_type,reserved_work_id,evidence_fingerprint) values($1,$2,$3,$4,'enforced','verified',false,$5,'{}','["fixture"]','v1','doctor','seo_doctor_finding',$6,'site_fix',$7,'fixture')`, []any{signatureID, projectID, candidateID, shadowID, "scheduler-" + suffix, findingID.String(), fixID}},
		{`insert into site_fixes(id,project_id,doctor_finding_id,candidate_id,work_signature_id,status,finding_kind,target_urls,evidence_snapshot,proposed_fix,acceptance_tests,verified_at,measurement_policy) values($1,$2,$3,$4,$5,'verified','optimization','["https://example.com/"]','{}','{}','[]',$6,$7)`, []any{fixID, projectID, findingID, candidateID, signatureID, verifiedAt, policy}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("fixture %.40s: %v", statement.sql, err)
		}
	}
	return projectID, fixID
}
