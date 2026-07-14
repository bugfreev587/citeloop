//go:build integration

package db

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSiteFixMeasurementPostgresIdempotencyAndLeaseRecovery(t *testing.T) {
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

	for _, object := range []string{
		"site_fix_measurements",
		"site_fix_measurement_checkpoints",
		"site_fix_measurement_handoff_outbox",
	} {
		var exists bool
		if err := pool.QueryRow(ctx, `select to_regclass('public.' || $1) is not null`, object).Scan(&exists); err != nil || !exists {
			t.Fatalf("run migrations before Site Fix measurement integration tests: %s exists=%v err=%v", object, exists, err)
		}
	}

	validPolicy := json.RawMessage(`{
		"policy_version":"site-fix-measurement-v1",
		"early_signal_offset_days":7,
		"primary_checkpoint_offset_days":14,
		"follow_up_offsets_days":[28],
		"max_follow_up_attempts":1,
		"max_measuring_duration_days":28,
		"terminalization_grace_period_days":7,
		"metric_thresholds":{"direction":"increase","kind":"relative","value":0.1},
		"guardrails":[{"metric":"gsc_impressions","max_adverse_relative":0.3}],
		"required_data_sources":["gsc"],
		"minimum_sample":{"minimum_after_periods":7,"minimum_after_sample":30}
	}`)
	for name, policy := range map[string]json.RawMessage{
		"valid":                   validPolicy,
		"empty_source":            json.RawMessage(`{"policy_version":"v1","early_signal_offset_days":7,"primary_checkpoint_offset_days":14,"follow_up_offsets_days":[],"max_follow_up_attempts":0,"max_measuring_duration_days":28,"terminalization_grace_period_days":7,"metric_thresholds":{"direction":"increase","kind":"relative","value":0.1},"guardrails":[],"required_data_sources":[""],"minimum_sample":{"minimum_after_periods":7}}`),
		"unknown_source_adapter":  json.RawMessage(`{"policy_version":"v1","early_signal_offset_days":7,"primary_checkpoint_offset_days":14,"follow_up_offsets_days":[],"max_follow_up_attempts":0,"max_measuring_duration_days":28,"terminalization_grace_period_days":7,"metric_thresholds":{"direction":"increase","kind":"relative","value":0.1},"guardrails":[],"required_data_sources":["adobe_analytics"],"minimum_sample":{"minimum_after_periods":7}}`),
		"negative_threshold":      json.RawMessage(`{"policy_version":"v1","early_signal_offset_days":7,"primary_checkpoint_offset_days":14,"follow_up_offsets_days":[],"max_follow_up_attempts":0,"max_measuring_duration_days":28,"terminalization_grace_period_days":7,"metric_thresholds":{"direction":"increase","kind":"relative","value":-0.1},"guardrails":[],"required_data_sources":["gsc"],"minimum_sample":{"minimum_after_periods":7}}`),
		"unusable_minimum_sample": json.RawMessage(`{"policy_version":"v1","early_signal_offset_days":7,"primary_checkpoint_offset_days":14,"follow_up_offsets_days":[],"max_follow_up_attempts":0,"max_measuring_duration_days":28,"terminalization_grace_period_days":7,"metric_thresholds":{"direction":"increase","kind":"relative","value":0.1},"guardrails":[],"required_data_sources":["gsc"],"minimum_sample":{}}`),
	} {
		var accepted bool
		if err := pool.QueryRow(ctx, `select site_fix_measurement_policy_is_finite($1::jsonb)`, policy).Scan(&accepted); err != nil {
			t.Fatalf("validate %s policy: %v", name, err)
		}
		if accepted != (name == "valid") {
			t.Fatalf("policy %s accepted=%v", name, accepted)
		}
	}

	projectID, fixID, _ := insertCanonicalSiteFixFixture(t, ctx, pool, "approved", "approved", "")
	createArgs := func(id uuid.UUID, key string) CreateSiteFixMeasurementParams {
		return CreateSiteFixMeasurementParams{
			ID: id, ProjectID: projectID, SiteFixID: fixID, CreationIdempotencyKey: key,
			TargetUrl: "https://example.com/", NormalizedTargetUrl: "https://example.com/", TargetIdentity: json.RawMessage(`{"url":"https://example.com/"}`),
			FixType: "metadata_ctr_optimization", ImpactMode: "conversion_or_ctr", ClassifierVersion: "site-fix-policy-v1",
			DecisionOrigin: "system_rule", DecisionConfidence: "high", GrowthHypothesis: "A clearer title improves qualified organic CTR.",
			PrimaryMetric: "gsc_ctr", SecondaryMetrics: json.RawMessage(`["gsc_impressions"]`),
			MeasurementPolicyVersion: "site-fix-measurement-v1", MeasurementPolicySnapshot: validPolicy,
			BaselineWindow: json.RawMessage(`{"window_days":28}`), BaselineSnapshot: json.RawMessage(`{}`), BaselineStatus: "planned",
			Status: "planned", AttributionConfidence: "none",
		}
	}

	type createResult struct {
		measurement SiteFixMeasurement
		err         error
	}
	start := make(chan struct{})
	results := make(chan createResult, 2)
	for _, id := range []uuid.UUID{uuid.New(), uuid.New()} {
		go func(id uuid.UUID) {
			<-start
			measurement, createErr := New(pool).CreateSiteFixMeasurement(ctx, createArgs(id, "approved:event-1"))
			results <- createResult{measurement: measurement, err: createErr}
		}(id)
	}
	close(start)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("concurrent replay errors: first=%v second=%v", first.err, second.err)
	}
	if first.measurement.ID != second.measurement.ID || first.measurement.MeasurementGeneration != 1 || second.measurement.MeasurementGeneration != 1 {
		t.Fatalf("same Approved event allocated multiple measurements: first=%+v second=%+v", first.measurement, second.measurement)
	}
	measurement := first.measurement
	secondGeneration, err := New(pool).CreateSiteFixMeasurement(ctx, createArgs(uuid.New(), "manual-opt-in:event-2"))
	if err != nil || secondGeneration.MeasurementGeneration != 2 || secondGeneration.ID == measurement.ID {
		t.Fatalf("new creation identity did not allocate next generation: measurement=%+v err=%v", secondGeneration, err)
	}
	var counter int32
	if err := pool.QueryRow(ctx, `select last_generation from site_fix_measurement_generation_counters where project_id=$1 and site_fix_id=$2`, projectID, fixID).Scan(&counter); err != nil || counter != 2 {
		t.Fatalf("generation counter=%d err=%v", counter, err)
	}

	q := New(pool)
	now := time.Now().UTC()
	checkpointArgs := GetOrCreateSiteFixMeasurementCheckpointParams{
		ID: uuid.New(), ProjectID: projectID, MeasurementID: measurement.ID,
		CheckpointKey: "primary:14", CheckpointRole: "primary",
		ScheduledAt: pgutil.TS(now), WindowStart: pgutil.TS(now.AddDate(0, 0, -28)), WindowEnd: pgutil.TS(now), AttemptNumber: 1,
		RequiredDataSources: json.RawMessage(`["gsc"]`), DataAvailability: json.RawMessage(`{"gsc":"available"}`), MinimumSample: json.RawMessage(`{"minimum_after_periods":7}`),
		SeoMetrics: json.RawMessage(`{}`), Ga4Metrics: json.RawMessage(`{}`), GeoMetrics: json.RawMessage(`{}`), ExecutionMetrics: json.RawMessage(`{}`), GuardrailResults: json.RawMessage(`{}`),
		AttributionConfidence: "none", RetryClassification: "not_applicable",
	}
	checkpoint, err := q.GetOrCreateSiteFixMeasurementCheckpoint(ctx, checkpointArgs)
	if err != nil {
		t.Fatal(err)
	}
	checkpointArgs.ID = uuid.New()
	replayedCheckpoint, err := q.GetOrCreateSiteFixMeasurementCheckpoint(ctx, checkpointArgs)
	if err != nil || replayedCheckpoint.ID != checkpoint.ID {
		t.Fatalf("checkpoint replay row=%+v err=%v", replayedCheckpoint, err)
	}

	outcomeArgs := GetOrCreateSiteFixMeasurementTerminalOutcomeParams{
		ID: uuid.New(), ProjectID: projectID, MeasurementID: measurement.ID, OutcomeLabel: "positive", RecordKind: "directional_learning",
		TerminalReason: "primary checkpoint crossed threshold", MeasurementPolicyVersion: measurement.MeasurementPolicyVersion,
		BaselineSnapshot: json.RawMessage(`{}`), CheckpointSnapshot: json.RawMessage(`{}`), OutcomeSnapshot: json.RawMessage(`{"label":"positive"}`),
	}
	outcome, err := q.GetOrCreateSiteFixMeasurementTerminalOutcome(ctx, outcomeArgs)
	if err != nil {
		t.Fatal(err)
	}
	outcomeArgs.ID = uuid.New()
	replayedOutcome, err := q.GetOrCreateSiteFixMeasurementTerminalOutcome(ctx, outcomeArgs)
	if err != nil || replayedOutcome.ID != outcome.ID {
		t.Fatalf("outcome replay row=%+v err=%v", replayedOutcome, err)
	}
	learningArgs := GetOrCreateSiteFixMeasurementLearningParams{
		ID: uuid.New(), ProjectID: projectID, TerminalOutcomeID: outcome.ID, MeasurementID: measurement.ID,
		LearningSummary: "Title clarity correlated with CTR improvement.", Applicability: json.RawMessage(`{"fix_type":"metadata_ctr_optimization"}`), LearningVersion: "site-fix-learning-v1",
	}
	learning, err := q.GetOrCreateSiteFixMeasurementLearning(ctx, learningArgs)
	if err != nil {
		t.Fatal(err)
	}
	learningArgs.ID = uuid.New()
	replayedLearning, err := q.GetOrCreateSiteFixMeasurementLearning(ctx, learningArgs)
	if err != nil || replayedLearning.ID != learning.ID {
		t.Fatalf("learning replay row=%+v err=%v", replayedLearning, err)
	}

	qualityMeasurement, err := q.CreateSiteFixMeasurement(ctx, createArgs(uuid.New(), "quality:event-3"))
	if err != nil || qualityMeasurement.MeasurementGeneration != 3 {
		t.Fatalf("quality generation=%+v err=%v", qualityMeasurement, err)
	}
	qualityOutcome, err := q.GetOrCreateSiteFixMeasurementTerminalOutcome(ctx, GetOrCreateSiteFixMeasurementTerminalOutcomeParams{
		ID: uuid.New(), ProjectID: projectID, MeasurementID: qualityMeasurement.ID, OutcomeLabel: "insufficient_data", RecordKind: "measurement_quality",
		TerminalReason: "provider unavailable", MeasurementPolicyVersion: qualityMeasurement.MeasurementPolicyVersion,
		BaselineSnapshot: json.RawMessage(`{}`), CheckpointSnapshot: json.RawMessage(`{}`), OutcomeSnapshot: json.RawMessage(`{"label":"insufficient_data"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	qualityArgs := GetOrCreateSiteFixMeasurementQualityRecordParams{
		ID: uuid.New(), ProjectID: projectID, TerminalOutcomeID: qualityOutcome.ID, MeasurementID: qualityMeasurement.ID,
		DataQualityState: "provider_unavailable", QualityGaps: json.RawMessage(`["gsc"]`), Recommendation: "Reconnect GSC.", QualityVersion: "site-fix-measurement-quality-v1",
	}
	quality, err := q.GetOrCreateSiteFixMeasurementQualityRecord(ctx, qualityArgs)
	if err != nil {
		t.Fatal(err)
	}
	qualityArgs.ID = uuid.New()
	replayedQuality, err := q.GetOrCreateSiteFixMeasurementQualityRecord(ctx, qualityArgs)
	if err != nil || replayedQuality.ID != quality.ID {
		t.Fatalf("quality replay row=%+v err=%v", replayedQuality, err)
	}

	page, err := q.ListSiteFixMeasurementsForResults(ctx, ListSiteFixMeasurementsForResultsParams{ProjectID: projectID, PageLimit: 1, PageOffset: 1})
	if err != nil || len(page) != 1 {
		t.Fatalf("paginated Results rows=%d err=%v", len(page), err)
	}

	handoff, err := q.EnqueueSiteFixMeasurementHandoff(ctx, EnqueueSiteFixMeasurementHandoffParams{
		ID: uuid.New(), ProjectID: projectID, SiteFixID: fixID, MeasurementGeneration: measurement.MeasurementGeneration,
		IdempotencyKey: "verified:event-1", MaxAttempts: 3, NextAttemptAt: pgutil.TS(now.Add(-time.Minute)),
	})
	if err != nil {
		t.Fatal(err)
	}
	staleToken, recoveryToken := uuid.New(), uuid.New()
	staleClaim, err := q.ClaimSiteFixMeasurementHandoff(ctx, ClaimSiteFixMeasurementHandoffParams{
		LockToken: pgtype.UUID{Bytes: staleToken, Valid: true}, LockedUntil: pgutil.TS(now.Add(-time.Second)), NowAt: pgutil.TS(now),
	})
	if err != nil || staleClaim.ID != handoff.ID || staleClaim.AttemptCount != 1 {
		t.Fatalf("stale claim=%+v err=%v", staleClaim, err)
	}
	recovered, err := q.ClaimSiteFixMeasurementHandoff(ctx, ClaimSiteFixMeasurementHandoffParams{
		LockToken: pgtype.UUID{Bytes: recoveryToken, Valid: true}, LockedUntil: pgutil.TS(now.Add(time.Minute)), NowAt: pgutil.TS(now),
	})
	if err != nil || recovered.ID != handoff.ID || recovered.AttemptCount != 2 || !recovered.LockToken.Valid || recovered.LockToken.Bytes != recoveryToken {
		t.Fatalf("recovered claim=%+v err=%v", recovered, err)
	}
	if _, err := q.CompleteSiteFixMeasurementHandoff(ctx, CompleteSiteFixMeasurementHandoffParams{
		ID: handoff.ID, ProjectID: projectID, LockToken: pgtype.UUID{Bytes: staleToken, Valid: true},
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stale lease completed recovered handoff: %v", err)
	}
	completed, err := q.CompleteSiteFixMeasurementHandoff(ctx, CompleteSiteFixMeasurementHandoffParams{
		ID: handoff.ID, ProjectID: projectID, LockToken: pgtype.UUID{Bytes: recoveryToken, Valid: true},
	})
	if err != nil || completed.Status != "completed" {
		t.Fatalf("recovered owner completion=%+v err=%v", completed, err)
	}

	exhausted, err := q.EnqueueSiteFixMeasurementHandoff(ctx, EnqueueSiteFixMeasurementHandoffParams{
		ID: uuid.New(), ProjectID: projectID, SiteFixID: fixID, MeasurementGeneration: secondGeneration.MeasurementGeneration,
		IdempotencyKey: "verified:event-2", MaxAttempts: 1, NextAttemptAt: pgutil.TS(now.Add(-time.Minute)),
	})
	if err != nil {
		t.Fatal(err)
	}
	exhaustedToken := uuid.New()
	claimedExhausted, err := q.ClaimSiteFixMeasurementHandoff(ctx, ClaimSiteFixMeasurementHandoffParams{
		LockToken: pgtype.UUID{Bytes: exhaustedToken, Valid: true}, LockedUntil: pgutil.TS(now.Add(-time.Second)), NowAt: pgutil.TS(now),
	})
	if err != nil || claimedExhausted.ID != exhausted.ID || claimedExhausted.AttemptCount != 1 {
		t.Fatalf("attempt-limit claim=%+v err=%v", claimedExhausted, err)
	}
	terminalized, err := q.TerminalizeExpiredSiteFixMeasurementHandoffs(ctx, pgutil.TS(now))
	if err != nil || len(terminalized) != 1 || terminalized[0].ID != exhausted.ID || terminalized[0].Status != "failed_terminal" {
		t.Fatalf("expired attempt-limit terminalization=%+v err=%v", terminalized, err)
	}
	if _, err := q.ClaimSiteFixMeasurementHandoff(ctx, ClaimSiteFixMeasurementHandoffParams{
		LockToken: pgtype.UUID{Bytes: uuid.New(), Valid: true}, LockedUntil: pgutil.TS(now.Add(time.Minute)), NowAt: pgutil.TS(now),
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("terminal handoff was reclaimable: %v", err)
	}
}

func TestCanonicalSiteFixVerificationHandoffIsAtomic(t *testing.T) {
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

	validPolicy := json.RawMessage(`{"policy_version":"site-fix-growth-v1","early_signal_offset_days":7,"primary_checkpoint_offset_days":28,"follow_up_offsets_days":[42],"max_follow_up_attempts":1,"max_measuring_duration_days":56,"terminalization_grace_period_days":2,"metric_thresholds":{"direction":"increase","kind":"relative","value":0.05},"guardrails":[],"required_data_sources":["gsc"],"minimum_sample":{"minimum_after_periods":7,"minimum_after_sample":100}}`)
	createMeasurement := func(projectID, fixID uuid.UUID) SiteFixMeasurement {
		measurement, createErr := New(pool).CreateSiteFixMeasurement(ctx, CreateSiteFixMeasurementParams{
			ID: uuid.New(), ProjectID: projectID, SiteFixID: fixID, CreationIdempotencyKey: "approval-required-v1:" + fixID.String(),
			TargetUrl: "https://example.com/", NormalizedTargetUrl: "https://example.com/", TargetIdentity: json.RawMessage(`{"target_url":"https://example.com/"}`),
			FixType: "metadata_ctr_optimization", ImpactMode: "conversion_or_ctr", ClassifierVersion: "site-fix-classifier-v1",
			DecisionOrigin: "system_rule", DecisionConfidence: "high", GrowthHypothesis: "A clearer title improves CTR.", PrimaryMetric: "ctr",
			SecondaryMetrics: json.RawMessage(`[]`), MeasurementPolicyVersion: "site-fix-growth-v1", MeasurementPolicySnapshot: validPolicy,
			BaselineWindow:   json.RawMessage(`{"start":"2026-05-01T00:00:00Z","end":"2026-05-28T00:00:00Z"}`),
			BaselineSnapshot: json.RawMessage(`{"ctr":0.04,"provenance":{"source":"gsc"}}`), BaselineStatus: "ready", Status: "ready", AttributionConfidence: "high",
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return measurement
	}
	markVerified := func(projectID, fixID, appID uuid.UUID, verifiedAt time.Time) error {
		_, markErr := New(pool).MarkCanonicalSiteFixVerified(ctx, MarkCanonicalSiteFixVerifiedParams{
			SiteFixID: fixID, ProjectID: projectID, ApplicationID: appID,
			DeploymentSnapshot: json.RawMessage(`{"source":"integration"}`), VerificationSnapshot: json.RawMessage(`{"result":"passed"}`),
			VerifiedAt: pgutil.TS(verifiedAt),
		})
		return markErr
	}

	t.Run("required creates exactly one handoff", func(t *testing.T) {
		projectID, fixID, appID := insertCanonicalSiteFixFixture(t, ctx, pool, "verifying", "verifying", "verification_pending")
		if _, err := pool.Exec(ctx, `update site_fixes set measurement_policy='measurement_required',growth_hypothesis='A clearer title improves CTR.',primary_metric='ctr',measurement_policy_version='site-fix-growth-v1',measurement_policy_snapshot=$3 where project_id=$1 and id=$2`, projectID, fixID, validPolicy); err != nil {
			t.Fatal(err)
		}
		measurement := createMeasurement(projectID, fixID)
		verifiedAt := time.Now().UTC()
		if err := markVerified(projectID, fixID, appID, verifiedAt); err != nil {
			t.Fatal(err)
		}
		var fixStatus, outboxStatus, idempotencyKey string
		var count int
		if err := pool.QueryRow(ctx, `select status from site_fixes where project_id=$1 and id=$2`, projectID, fixID).Scan(&fixStatus); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `select count(*),min(status),min(idempotency_key) from site_fix_measurement_handoff_outbox where project_id=$1 and site_fix_id=$2 and measurement_generation=$3`, projectID, fixID, measurement.MeasurementGeneration).Scan(&count, &outboxStatus, &idempotencyKey); err != nil {
			t.Fatal(err)
		}
		if fixStatus != "verified" || count != 1 || outboxStatus != "pending" || idempotencyKey != "activate:"+fixID.String()+":1" {
			t.Fatalf("fix=%s count=%d outbox=%s key=%s", fixStatus, count, outboxStatus, idempotencyKey)
		}
	})

	t.Run("missing required generation leaves lifecycle untouched", func(t *testing.T) {
		projectID, fixID, appID := insertCanonicalSiteFixFixture(t, ctx, pool, "verifying", "verifying", "verification_pending")
		if _, err := pool.Exec(ctx, `update site_fixes set measurement_policy='measurement_required',growth_hypothesis='A clearer title improves CTR.',primary_metric='ctr',measurement_policy_version='site-fix-growth-v1',measurement_policy_snapshot=$3 where project_id=$1 and id=$2`, projectID, fixID, validPolicy); err != nil {
			t.Fatal(err)
		}
		if err := markVerified(projectID, fixID, appID, time.Now().UTC()); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("missing required measurement error=%v", err)
		}
		var fixStatus, appStatus string
		if err := pool.QueryRow(ctx, `select sf.status,a.status from site_fixes sf join site_change_applications a on a.site_fix_id=sf.id and a.project_id=sf.project_id where sf.project_id=$1 and sf.id=$2`, projectID, fixID).Scan(&fixStatus, &appStatus); err != nil {
			t.Fatal(err)
		}
		if fixStatus != "verifying" || appStatus != "verification_pending" {
			t.Fatalf("failed invariant partially committed fix=%s app=%s", fixStatus, appStatus)
		}
	})

	t.Run("verification only enqueues zero", func(t *testing.T) {
		projectID, fixID, appID := insertCanonicalSiteFixFixture(t, ctx, pool, "verifying", "verifying", "verification_pending")
		if err := markVerified(projectID, fixID, appID, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := pool.QueryRow(ctx, `select count(*) from site_fix_measurement_handoff_outbox where project_id=$1 and site_fix_id=$2`, projectID, fixID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("verification-only handoffs=%d err=%v", count, err)
		}
	})
}
