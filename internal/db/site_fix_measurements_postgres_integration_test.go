//go:build integration

package db

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
		EvaluationAttemptCount: 0, NextAttemptAt: pgutil.TS(now),
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
	checkpointOutcome, checkpointReason := "positive", "threshold crossed"
	completeArgs := CompleteSiteFixMeasurementCheckpointParams{
		DataAvailability: json.RawMessage(`{"gsc":"available"}`), SeoMetrics: json.RawMessage(`{"ctr":0.08}`),
		Ga4Metrics: json.RawMessage(`{}`), GeoMetrics: json.RawMessage(`{}`), ExecutionMetrics: json.RawMessage(`{}`), GuardrailResults: json.RawMessage(`{}`),
		OutcomeLabel: &checkpointOutcome, OutcomeReason: &checkpointReason, AttributionConfidence: "high", ComputedAt: pgutil.TS(now),
		RetryClassification: "not_applicable", ProjectID: projectID, MeasurementID: measurement.ID, CheckpointKey: checkpoint.CheckpointKey, AttemptNumber: checkpoint.AttemptNumber,
	}
	completedCheckpoint, err := q.CompleteSiteFixMeasurementCheckpoint(ctx, completeArgs)
	if err != nil || !completedCheckpoint.ComputedAt.Valid || completedCheckpoint.OutcomeLabel == nil || *completedCheckpoint.OutcomeLabel != "positive" {
		t.Fatalf("checkpoint completion=%+v err=%v", completedCheckpoint, err)
	}
	replayedCompletion, err := q.CompleteSiteFixMeasurementCheckpoint(ctx, completeArgs)
	if err != nil || replayedCompletion.ID != completedCheckpoint.ID || string(replayedCompletion.SeoMetrics) != string(completedCheckpoint.SeoMetrics) {
		t.Fatalf("exact completion replay failed: row=%+v err=%v", replayedCompletion, err)
	}
	completeArgs.SeoMetrics = json.RawMessage(`{"ctr":999}`)
	if _, err := q.CompleteSiteFixMeasurementCheckpoint(ctx, completeArgs); err == nil {
		t.Fatal("conflicting completion replay was accepted")
	}
	if _, err := pool.Exec(ctx, `update site_fix_measurement_checkpoints set outcome_reason='mutated' where id=$1`, checkpoint.ID); err == nil {
		t.Fatal("completed checkpoint accepted result mutation")
	}
	if _, err := pool.Exec(ctx, `delete from site_fix_measurement_checkpoints where id=$1`, checkpoint.ID); err == nil {
		t.Fatal("completed checkpoint accepted delete")
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
	validPlan := json.RawMessage(`{"growth_hypothesis":"A clearer title improves qualified organic CTR.","primary_metric":"gsc_ctr","secondary_metrics":["gsc_impressions"],"target_query":"social publishing","target_identity":{},"baseline_window":{"start":"2026-05-01T00:00:00Z","end":"2026-05-28T00:00:00Z"},"baseline_snapshot":{"gsc_ctr":0.04,"gsc_impressions":1000},"baseline_provenance":{"source":"gsc","captured_at":"2026-05-28T01:00:00Z"},"policy_snapshot":` + string(validPolicy) + `}`)
	if _, err := pool.Exec(ctx, `update site_fixes set status='verified',measurement_policy='measurement_required',fix_type='metadata_ctr_optimization',impact_mode='conversion_or_ctr',classifier_version='site-fix-policy-v1',decision_origin='system_rule',decision_confidence='high',growth_hypothesis='A clearer title improves qualified organic CTR.',primary_metric='gsc_ctr',secondary_metrics='["gsc_impressions"]',measurement_policy_version='site-fix-measurement-v1',measurement_policy_snapshot=$3,measurement_plan_snapshot=$4 where project_id=$1 and id=$2`, projectID, fixID, validPolicy, validPlan); err != nil {
		t.Fatal(err)
	}
	reconciled, err := q.ReconcileVerifiedSiteFixMeasurementHandoffs(ctx, ReconcileVerifiedSiteFixMeasurementHandoffsParams{NowAt: pgutil.TS(now), LimitRows: 10})
	if err != nil || len(reconciled) != 3 {
		t.Fatalf("reconciled=%+v err=%v", reconciled, err)
	}
	if _, err := pool.Exec(ctx, `update site_fix_measurement_handoff_outbox set status='completed',completed_at=now() where project_id=$1 and site_fix_id=$2 and measurement_generation<>$3`, projectID, fixID, qualityMeasurement.MeasurementGeneration); err != nil {
		t.Fatal(err)
	}
	reconciled, err = q.ReconcileVerifiedSiteFixMeasurementHandoffs(ctx, ReconcileVerifiedSiteFixMeasurementHandoffsParams{NowAt: pgutil.TS(now), LimitRows: 10})
	if err != nil || len(reconciled) != 0 {
		t.Fatalf("reconcile replay=%+v err=%v", reconciled, err)
	}

	handoff, err := q.EnqueueSiteFixMeasurementHandoff(ctx, EnqueueSiteFixMeasurementHandoffParams{
		ID: uuid.New(), ProjectID: projectID, SiteFixID: fixID, MeasurementGeneration: qualityMeasurement.MeasurementGeneration,
		IdempotencyKey: "verified:event-1", MaxAttempts: 3, NextAttemptAt: pgutil.TS(now.Add(-time.Minute)), OccurredAt: pgutil.TS(now.Add(-2 * time.Minute)),
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

	exhaustedMeasurement, err := q.CreateSiteFixMeasurement(ctx, createArgs(uuid.New(), "exhausted:event-4"))
	if err != nil {
		t.Fatal(err)
	}
	exhausted, err := q.EnqueueSiteFixMeasurementHandoff(ctx, EnqueueSiteFixMeasurementHandoffParams{
		ID: uuid.New(), ProjectID: projectID, SiteFixID: fixID, MeasurementGeneration: exhaustedMeasurement.MeasurementGeneration,
		IdempotencyKey: "verified:event-2", MaxAttempts: 1, NextAttemptAt: pgutil.TS(now.Add(-time.Minute)), OccurredAt: pgutil.TS(now.Add(-2 * time.Minute)),
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
	if _, err := pool.Exec(ctx, `update site_fix_measurement_handoff_outbox set status='failed_terminal',completed_at=null,last_error_classification='fixture_terminal',last_error='fixture terminal',updated_at=updated_at+interval '1 second' where id=$1`, handoff.ID); err != nil {
		t.Fatal(err)
	}
	alerted := map[uuid.UUID]bool{}
	for range 2 {
		alertToken := uuid.New()
		claimed, err := q.ClaimFailedTerminalSiteFixMeasurementHandoffAlert(ctx, ClaimFailedTerminalSiteFixMeasurementHandoffAlertParams{AlertLockToken: pgtype.UUID{Bytes: alertToken, Valid: true}, AlertLockedUntil: pgutil.TS(now.Add(time.Minute)), NowAt: pgutil.TS(now)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.CompleteFailedTerminalSiteFixMeasurementHandoffAlert(ctx, CompleteFailedTerminalSiteFixMeasurementHandoffAlertParams{AlertNotifiedAt: pgutil.TS(now), ID: claimed.ID, ProjectID: claimed.ProjectID, AlertLockToken: pgtype.UUID{Bytes: uuid.New(), Valid: true}}); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("stale alert owner acknowledged claim: %v", err)
		}
		completedAlert, err := q.CompleteFailedTerminalSiteFixMeasurementHandoffAlert(ctx, CompleteFailedTerminalSiteFixMeasurementHandoffAlertParams{AlertNotifiedAt: pgutil.TS(now), ID: claimed.ID, ProjectID: claimed.ProjectID, AlertLockToken: pgtype.UUID{Bytes: alertToken, Valid: true}})
		if err != nil || !completedAlert.AlertNotifiedAt.Valid || completedAlert.Status != "failed_terminal" {
			t.Fatalf("completed alert=%+v err=%v", completedAlert, err)
		}
		alerted[claimed.ID] = true
	}
	if !alerted[handoff.ID] || !alerted[exhausted.ID] {
		t.Fatalf("alert claims did not progress across terminal rows: %v", alerted)
	}
	if _, err := q.ClaimFailedTerminalSiteFixMeasurementHandoffAlert(ctx, ClaimFailedTerminalSiteFixMeasurementHandoffAlertParams{AlertLockToken: pgtype.UUID{Bytes: uuid.New(), Valid: true}, AlertLockedUntil: pgutil.TS(now.Add(time.Minute)), NowAt: pgutil.TS(now)}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("acknowledged failed_terminal alert was reclaimed: %v", err)
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
	validPlan := json.RawMessage(`{"growth_hypothesis":"A clearer title improves CTR.","primary_metric":"ctr","secondary_metrics":[],"target_query":"social publishing api","target_identity":{},"baseline_window":{"start":"2026-05-01T00:00:00Z","end":"2026-05-28T00:00:00Z"},"baseline_snapshot":{"ctr":0.04},"baseline_provenance":{"source":"gsc","captured_at":"2026-05-28T01:00:00Z"},"policy_snapshot":` + string(validPolicy) + `}`)
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
		if _, err := pool.Exec(ctx, `update site_fixes set measurement_policy='measurement_required',growth_hypothesis='A clearer title improves CTR.',primary_metric='ctr',measurement_policy_version='site-fix-growth-v1',measurement_policy_snapshot=$3,measurement_plan_snapshot=$4 where project_id=$1 and id=$2`, projectID, fixID, validPolicy, validPlan); err != nil {
			t.Fatal(err)
		}
		measurement := createMeasurement(projectID, fixID)
		verifiedAt := time.Now().UTC()
		if err := markVerified(projectID, fixID, appID, verifiedAt); err != nil {
			t.Fatal(err)
		}
		var fixStatus, outboxStatus, idempotencyKey string
		var occurredAt time.Time
		var count int
		if err := pool.QueryRow(ctx, `select status from site_fixes where project_id=$1 and id=$2`, projectID, fixID).Scan(&fixStatus); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `select count(*),min(status),min(idempotency_key),min(occurred_at) from site_fix_measurement_handoff_outbox where project_id=$1 and site_fix_id=$2 and measurement_generation=$3`, projectID, fixID, measurement.MeasurementGeneration).Scan(&count, &outboxStatus, &idempotencyKey, &occurredAt); err != nil {
			t.Fatal(err)
		}
		if fixStatus != "verified" || count != 1 || outboxStatus != "pending" || idempotencyKey != "activate:"+fixID.String()+":1" || !occurredAt.Equal(verifiedAt) {
			t.Fatalf("fix=%s count=%d outbox=%s key=%s occurred_at=%s want=%s", fixStatus, count, outboxStatus, idempotencyKey, occurredAt, verifiedAt)
		}
	})

	t.Run("missing required generation leaves lifecycle untouched", func(t *testing.T) {
		projectID, fixID, appID := insertCanonicalSiteFixFixture(t, ctx, pool, "verifying", "verifying", "verification_pending")
		if _, err := pool.Exec(ctx, `update site_fixes set measurement_policy='measurement_required',growth_hypothesis='A clearer title improves CTR.',primary_metric='ctr',measurement_policy_version='site-fix-growth-v1',measurement_policy_snapshot=$3,measurement_plan_snapshot=$4 where project_id=$1 and id=$2`, projectID, fixID, validPolicy, validPlan); err != nil {
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

func TestResultsFeedAndSiteFixDetailPostgres(t *testing.T) {
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
	q := New(pool)
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	policy := json.RawMessage(`{"policy_version":"site-fix-growth-v1","early_signal_offset_days":1,"primary_checkpoint_offset_days":2,"follow_up_offsets_days":[],"max_follow_up_attempts":0,"max_measuring_duration_days":2,"terminalization_grace_period_days":1,"metric_thresholds":{"direction":"increase","kind":"relative","value":0.1},"guardrails":[],"required_data_sources":["gsc"],"minimum_sample":{"minimum_after_periods":1,"minimum_after_sample":1}}`)
	projectID, fixID, _ := insertCanonicalSiteFixFixture(t, ctx, pool, "approved", "approved", "")
	if _, err := pool.Exec(ctx, `update site_fixes set status='verified',verified_at=$3 where project_id=$1 and id=$2`, projectID, fixID, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	create := func(key string) SiteFixMeasurement {
		row, err := q.CreateSiteFixMeasurement(ctx, CreateSiteFixMeasurementParams{
			ID: uuid.New(), ProjectID: projectID, SiteFixID: fixID, CreationIdempotencyKey: key,
			TargetUrl: "https://example.com/results", NormalizedTargetUrl: "https://example.com/results", TargetIdentity: json.RawMessage(`{"secret":"never-public"}`),
			FixType: "metadata_ctr_optimization", ImpactMode: "conversion_or_ctr", ClassifierVersion: "secret-classifier",
			DecisionOrigin: "system_rule", DecisionConfidence: "high", GrowthHypothesis: "CTR improves.", PrimaryMetric: "ctr", SecondaryMetrics: json.RawMessage(`[]`),
			MeasurementPolicyVersion: "site-fix-growth-v1", MeasurementPolicySnapshot: policy, BaselineWindow: json.RawMessage(`{"start":"2026-06-01","end":"2026-06-07"}`), BaselineSnapshot: json.RawMessage(`{"secret_baseline":true}`), BaselineStatus: "planned", Status: "planned", AttributionConfidence: "none",
		})
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	first, second := create("results-generation-1"), create("results-generation-2")
	verificationOnlyFixID := uuid.New()
	if _, err := pool.Exec(ctx, `insert into site_fixes(id,project_id,doctor_finding_id,candidate_id,work_signature_id,supersedes_site_fix_id,status,finding_kind,target_urls,evidence_snapshot,proposed_fix,acceptance_tests,verified_at,measurement_policy) select $3,project_id,doctor_finding_id,candidate_id,work_signature_id,$2,'verified',finding_kind,target_urls,evidence_snapshot,proposed_fix,acceptance_tests,$4,'verification_only' from site_fixes where project_id=$1 and id=$2`, projectID, fixID, verificationOnlyFixID, now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `update site_fix_measurements set updated_at=case id when $1 then $3::timestamptz else $4::timestamptz end where id in ($1,$2)`, first.ID, second.ID, now, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	opportunityID, actionID := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `insert into seo_opportunities(id,project_id,type,status,page_url,normalized_page_url,query,evidence) values($1,$2,'content_gap','accepted','https://example.com/content','https://example.com/content','query','{}')`, opportunityID, projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `insert into content_actions(id,project_id,opportunity_id,action_type,status,target_url,normalized_target_url,published_at,updated_at) values($1,$2,$3,'publish','completed','https://example.com/content','https://example.com/content',$4,$4)`, actionID, projectID, opportunityID, now); err != nil {
		t.Fatal(err)
	}
	otherProject, otherFix, _ := insertCanonicalSiteFixFixture(t, ctx, pool, "approved", "approved", "")
	if _, err := New(pool).CreateSiteFixMeasurement(ctx, CreateSiteFixMeasurementParams{ID: uuid.New(), ProjectID: otherProject, SiteFixID: otherFix, CreationIdempotencyKey: "other-project", TargetUrl: "https://other.example.com", NormalizedTargetUrl: "https://other.example.com", TargetIdentity: json.RawMessage(`{}`), FixType: "metadata_ctr_optimization", ImpactMode: "conversion_or_ctr", ClassifierVersion: "v1", DecisionOrigin: "system_rule", DecisionConfidence: "high", GrowthHypothesis: "Other.", PrimaryMetric: "ctr", SecondaryMetrics: json.RawMessage(`[]`), MeasurementPolicyVersion: "site-fix-growth-v1", MeasurementPolicySnapshot: policy, BaselineWindow: json.RawMessage(`{}`), BaselineSnapshot: json.RawMessage(`{}`), BaselineStatus: "planned", Status: "planned", AttributionConfidence: "none"}); err != nil {
		t.Fatal(err)
	}

	page1, err := q.ListResultsFeedRows(ctx, ListResultsFeedRowsParams{ProjectID: projectID, LimitRows: 2})
	if err != nil || len(page1) != 2 {
		t.Fatalf("page1=%+v err=%v", page1, err)
	}
	if page1[0].SourceType != "content_action" || page1[0].ID != actionID || page1[1].SourceType != "site_fix" || page1[1].ID != first.ID {
		t.Fatalf("deterministic first page=%+v", page1)
	}
	payload, _ := json.Marshal(page1[0].Payload)
	if !strings.Contains(string(payload), `"action_type":"publish"`) || !strings.Contains(string(payload), `"source_type":"content_action"`) {
		t.Fatalf("legacy content payload changed: %s", payload)
	}
	last := page1[len(page1)-1]
	page2, err := q.ListResultsFeedRows(ctx, ListResultsFeedRowsParams{ProjectID: projectID, LimitRows: 2, CursorActivityAt: last.ActivityAt, CursorSourceType: last.SourceType, CursorID: pgtype.UUID{Bytes: last.ID, Valid: true}})
	if err != nil || len(page2) != 1 || page2[0].ID != second.ID {
		t.Fatalf("page2=%+v err=%v", page2, err)
	}
	for _, row := range append(page1, page2...) {
		if row.ID == verificationOnlyFixID {
			t.Fatal("verification_only Site Fix without a measurement appeared in Results")
		}
	}
	statusRows, err := q.ListResultsFeedRows(ctx, ListResultsFeedRowsParams{ProjectID: projectID, Status: "planned", LimitRows: 10})
	if err != nil || len(statusRows) != 2 || statusRows[0].SourceType != "site_fix" || statusRows[1].SourceType != "site_fix" {
		t.Fatalf("status rows=%+v err=%v", statusRows, err)
	}
	legacyRows, err := q.ListResultsFeedRows(ctx, ListResultsFeedRowsParams{ProjectID: projectID, LegacyCursorAt: pgutil.TS(now), LimitRows: 10})
	if err != nil || len(legacyRows) != 1 || legacyRows[0].ID != second.ID {
		t.Fatalf("legacy cursor rows=%+v err=%v", legacyRows, err)
	}

	detail, err := q.GetSiteFixMeasurementResultsDetail(ctx, GetSiteFixMeasurementResultsDetailParams{ProjectID: projectID, MeasurementID: first.ID})
	if err != nil || detail.SiteFixStatus != "verified" || detail.MeasurementGeneration != 1 {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	if _, err := q.GetSiteFixMeasurementResultsDetail(ctx, GetSiteFixMeasurementResultsDetailParams{ProjectID: otherProject, MeasurementID: first.ID}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-project detail err=%v", err)
	}
	outcome, err := q.GetOrCreateSiteFixMeasurementTerminalOutcome(ctx, GetOrCreateSiteFixMeasurementTerminalOutcomeParams{ID: uuid.New(), ProjectID: projectID, MeasurementID: first.ID, OutcomeLabel: "positive", RecordKind: "directional_learning", TerminalReason: "public terminal reason", MeasurementPolicyVersion: first.MeasurementPolicyVersion, BaselineSnapshot: json.RawMessage(`{"private":true}`), CheckpointSnapshot: json.RawMessage(`{"private":true}`), OutcomeSnapshot: json.RawMessage(`{"private":true}`)})
	if err != nil {
		t.Fatal(err)
	}
	publicOutcome, err := q.GetSiteFixMeasurementTerminalOutcome(ctx, GetSiteFixMeasurementTerminalOutcomeParams{ProjectID: projectID, MeasurementID: first.ID})
	if err != nil || publicOutcome.ID != outcome.ID || publicOutcome.TerminalReason != "public terminal reason" {
		t.Fatalf("public terminal=%+v err=%v", publicOutcome, err)
	}
	for index, role := range []string{"early_signal", "primary"} {
		when := now.Add(time.Duration(index) * time.Hour)
		if _, err := q.GetOrCreateSiteFixMeasurementCheckpoint(ctx, GetOrCreateSiteFixMeasurementCheckpointParams{ID: uuid.New(), ProjectID: projectID, MeasurementID: first.ID, CheckpointKey: role, CheckpointRole: role, ScheduledAt: pgutil.TS(when), WindowStart: pgutil.TS(when.Add(-time.Hour)), WindowEnd: pgutil.TS(when), AttemptNumber: int32(index + 1), RequiredDataSources: json.RawMessage(`[]`), DataAvailability: json.RawMessage(`{}`), MinimumSample: json.RawMessage(`{}`), SeoMetrics: json.RawMessage(`{"provider_secret":true}`), Ga4Metrics: json.RawMessage(`{}`), GeoMetrics: json.RawMessage(`{}`), ExecutionMetrics: json.RawMessage(`{}`), GuardrailResults: json.RawMessage(`{}`), AttributionConfidence: "none", RetryClassification: "not_applicable", EvaluationAttemptCount: 0, NextAttemptAt: pgutil.TS(when)}); err != nil {
			t.Fatal(err)
		}
	}
	checkpoints, err := q.ListSiteFixMeasurementCheckpoints(ctx, ListSiteFixMeasurementCheckpointsParams{ProjectID: projectID, MeasurementID: first.ID})
	if err != nil || len(checkpoints) != 2 || checkpoints[0].CheckpointRole != "early_signal" || checkpoints[1].CheckpointRole != "primary" {
		t.Fatalf("ordered checkpoints=%+v err=%v", checkpoints, err)
	}
	handoff, err := q.EnqueueSiteFixMeasurementHandoff(ctx, EnqueueSiteFixMeasurementHandoffParams{ID: uuid.New(), ProjectID: projectID, SiteFixID: fixID, MeasurementGeneration: first.MeasurementGeneration, IdempotencyKey: "results-failed", MaxAttempts: 1, NextAttemptAt: pgutil.TS(now), OccurredAt: pgutil.TS(now)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `update site_fix_measurement_handoff_outbox set status='failed_terminal',last_error='redacted',updated_at=now() where id=$1`, handoff.ID); err != nil {
		t.Fatal(err)
	}
	latestHandoff, err := q.GetLatestSiteFixMeasurementHandoff(ctx, GetLatestSiteFixMeasurementHandoffParams{ProjectID: projectID, MeasurementID: first.ID})
	if err != nil || latestHandoff.Status != "failed_terminal" {
		t.Fatalf("latest handoff=%+v err=%v", latestHandoff, err)
	}
}

func TestSiteFixMeasurementPlanAlignmentRejectsIncompleteSnapshots(t *testing.T) {
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

	const (
		growthHypothesis = "A clearer title improves CTR."
		primaryMetric    = "ctr"
		policyVersion    = "site-fix-growth-v1"
	)
	validSecondaryMetrics := json.RawMessage(`["impressions"]`)
	validPolicy := json.RawMessage(`{"policy_version":"site-fix-growth-v1"}`)
	validPlan := json.RawMessage(`{"growth_hypothesis":"A clearer title improves CTR.","primary_metric":"ctr","secondary_metrics":["impressions"],"policy_snapshot":{"policy_version":"site-fix-growth-v1"}}`)

	type alignmentCase struct {
		growthHypothesis any
		primaryMetric    any
		secondaryMetrics json.RawMessage
		policyVersion    any
		policySnapshot   json.RawMessage
		planSnapshot     json.RawMessage
	}
	cases := map[string]alignmentCase{
		"missing secondary_metrics": {
			growthHypothesis, primaryMetric, validSecondaryMetrics, policyVersion, validPolicy,
			json.RawMessage(`{"growth_hypothesis":"A clearer title improves CTR.","primary_metric":"ctr","policy_snapshot":{"policy_version":"site-fix-growth-v1"}}`),
		},
		"missing policy_snapshot": {
			growthHypothesis, primaryMetric, validSecondaryMetrics, policyVersion, validPolicy,
			json.RawMessage(`{"growth_hypothesis":"A clearer title improves CTR.","primary_metric":"ctr","secondary_metrics":["impressions"]}`),
		},
		"null denormalized required column": {
			nil, primaryMetric, validSecondaryMetrics, policyVersion, validPolicy, validPlan,
		},
		"wrong secondary_metrics JSON type": {
			growthHypothesis, primaryMetric, validSecondaryMetrics, policyVersion, validPolicy,
			json.RawMessage(`{"growth_hypothesis":"A clearer title improves CTR.","primary_metric":"ctr","secondary_metrics":{},"policy_snapshot":{"policy_version":"site-fix-growth-v1"}}`),
		},
		"wrong policy_snapshot JSON type": {
			growthHypothesis, primaryMetric, validSecondaryMetrics, policyVersion, validPolicy,
			json.RawMessage(`{"growth_hypothesis":"A clearer title improves CTR.","primary_metric":"ctr","secondary_metrics":["impressions"],"policy_snapshot":[]}`),
		},
		"misaligned denormalized values": {
			growthHypothesis, "clicks", validSecondaryMetrics, policyVersion, validPolicy, validPlan,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			projectID, fixID, _ := insertCanonicalSiteFixFixture(t, ctx, pool, "approved", "approved", "")
			_, updateErr := pool.Exec(ctx, `
				update site_fixes
				set measurement_policy = 'measurement_optional',
				    growth_hypothesis = $3,
				    primary_metric = $4,
				    secondary_metrics = $5,
				    measurement_policy_version = $6,
				    measurement_policy_snapshot = $7,
				    measurement_plan_snapshot = $8
				where project_id = $1 and id = $2`,
				projectID, fixID, tc.growthHypothesis, tc.primaryMetric, tc.secondaryMetrics,
				tc.policyVersion, tc.policySnapshot, tc.planSnapshot)
			if updateErr == nil {
				t.Fatal("incomplete or misaligned measurement plan was accepted")
			}
			var pgErr *pgconn.PgError
			if !errors.As(updateErr, &pgErr) || pgErr.ConstraintName != "site_fixes_measurement_plan_alignment_check" {
				t.Fatalf("unexpected rejection: %v", updateErr)
			}
		})
	}

	t.Run("complete aligned snapshot is accepted", func(t *testing.T) {
		projectID, fixID, _ := insertCanonicalSiteFixFixture(t, ctx, pool, "approved", "approved", "")
		if _, err := pool.Exec(ctx, `
			update site_fixes
			set measurement_policy = 'measurement_optional',
			    growth_hypothesis = $3,
			    primary_metric = $4,
			    secondary_metrics = $5,
			    measurement_policy_version = $6,
			    measurement_policy_snapshot = $7,
			    measurement_plan_snapshot = $8
			where project_id = $1 and id = $2`,
			projectID, fixID, growthHypothesis, primaryMetric, validSecondaryMetrics,
			policyVersion, validPolicy, validPlan); err != nil {
			t.Fatalf("complete aligned measurement plan was rejected: %v", err)
		}
	})
}

func TestSiteFixMeasurementPlanSnapshotMigrationUpgradesLegacyRequiredRows(t *testing.T) {
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

	addMigration, err := os.ReadFile("../migrations/0091_site_fix_measurement_plan_snapshot.sql")
	if err != nil {
		t.Fatal(err)
	}
	validateMigration, err := os.ReadFile("../migrations/0092_site_fix_measurement_plan_snapshot_validate.sql")
	if err != nil {
		t.Fatal(err)
	}

	validPolicy := json.RawMessage(`{"policy_version":"site-fix-growth-v1","early_signal_offset_days":7,"primary_checkpoint_offset_days":28,"follow_up_offsets_days":[42],"max_follow_up_attempts":1,"max_measuring_duration_days":56,"terminalization_grace_period_days":2,"metric_thresholds":{"direction":"increase","kind":"relative","value":0.05},"guardrails":[],"required_data_sources":["gsc"],"minimum_sample":{"minimum_after_periods":7,"minimum_after_sample":100}}`)
	overridePlan := json.RawMessage(`{"growth_hypothesis":"Override hypothesis.","primary_metric":"ctr","secondary_metrics":["impressions"],"target_query":"social publishing api","target_identity":{},"baseline_window":{"start":"2026-05-01T00:00:00Z","end":"2026-05-28T00:00:00Z"},"baseline_snapshot":{"ctr":0.04,"impressions":1000},"baseline_provenance":{"source":"gsc","captured_at":"2026-05-28T01:00:00Z"},"policy_snapshot":` + string(validPolicy) + `}`)
	regularPlan := json.RawMessage(`{"growth_hypothesis":"Regular hypothesis.","primary_metric":"clicks","secondary_metrics":["impressions"],"target_query":"social publishing api","target_identity":{},"baseline_window":{"start":"2026-05-01T00:00:00Z","end":"2026-05-28T00:00:00Z"},"baseline_snapshot":{"clicks":40,"impressions":1000},"baseline_provenance":{"source":"gsc","captured_at":"2026-05-28T01:00:00Z"},"policy_snapshot":` + string(validPolicy) + `}`)
	spacedPlan := json.RawMessage(strings.Replace(string(overridePlan), `"Override hypothesis."`, `"  Override hypothesis.  "`, 1))

	overrideProjectID, overrideFixID, _ := insertCanonicalSiteFixFixture(t, ctx, pool, "approved", "approved", "")
	regularProjectID, regularFixID, _ := insertCanonicalSiteFixFixture(t, ctx, pool, "approved", "approved", "")
	invalidProjectID, invalidFixID, _ := insertCanonicalSiteFixFixture(t, ctx, pool, "approved", "approved", "")
	spacedProjectID, spacedFixID, _ := insertCanonicalSiteFixFixture(t, ctx, pool, "approved", "approved", "")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `
		alter table site_fixes
		  drop constraint if exists site_fixes_measurement_plan_alignment_check,
		  drop constraint if exists site_fixes_measurement_plan_snapshot_json_check,
		  drop column if exists measurement_plan_snapshot`); err != nil {
		t.Fatalf("simulate pre-0089 schema: %v", err)
	}

	seedLegacyRequired := func(projectID, fixID uuid.UUID, hypothesis, metric string, evidence json.RawMessage) {
		t.Helper()
		if _, err := tx.Exec(ctx, `
			update site_fixes
			set measurement_policy = 'measurement_required',
			    growth_hypothesis = $3,
			    primary_metric = $4,
			    secondary_metrics = '["impressions"]'::jsonb,
			    measurement_policy_version = 'site-fix-growth-v1',
			    measurement_policy_snapshot = $5,
			    evidence_snapshot = $6
			where project_id = $1 and id = $2`, projectID, fixID, hypothesis, metric, validPolicy, evidence); err != nil {
			t.Fatalf("seed pre-0089 required row: %v", err)
		}
	}
	seedLegacyRequired(overrideProjectID, overrideFixID, "Override hypothesis.", "ctr", json.RawMessage(`{"finding":{"measurement_plan":`+string(regularPlan)+`,"site_fix_policy_override":{"measurement_plan":`+string(overridePlan)+`}}}`))
	seedLegacyRequired(regularProjectID, regularFixID, "Regular hypothesis.", "clicks", json.RawMessage(`{"finding":{"measurement_plan":`+string(regularPlan)+`}}`))
	seedLegacyRequired(invalidProjectID, invalidFixID, "Override hypothesis.", "ctr", json.RawMessage(`{"finding":{"measurement_plan":`+string(regularPlan)+`}}`))
	seedLegacyRequired(spacedProjectID, spacedFixID, "Override hypothesis.", "ctr", json.RawMessage(`{"finding":{"measurement_plan":`+string(spacedPlan)+`}}`))

	if _, err := tx.Exec(ctx, string(addMigration)); err != nil {
		t.Fatalf("apply 0089 to legacy rows: %v", err)
	}
	if _, err := tx.Exec(ctx, string(validateMigration)); err != nil {
		t.Fatalf("apply deploy-safe 0090 to legacy rows: %v", err)
	}

	assertUpgraded := func(projectID, fixID uuid.UUID, wantPlan json.RawMessage) {
		t.Helper()
		var policy string
		var snapshot json.RawMessage
		if err := tx.QueryRow(ctx, `select measurement_policy,measurement_plan_snapshot from site_fixes where project_id=$1 and id=$2`, projectID, fixID).Scan(&policy, &snapshot); err != nil {
			t.Fatal(err)
		}
		if policy != "measurement_required" || !jsonSemanticallyEqualForDBTest(snapshot, wantPlan) {
			t.Fatalf("legacy plan was not upgraded: policy=%s snapshot=%s want=%s", policy, snapshot, wantPlan)
		}
	}
	assertUpgraded(overrideProjectID, overrideFixID, overridePlan)
	assertUpgraded(regularProjectID, regularFixID, regularPlan)
	assertUpgraded(spacedProjectID, spacedFixID, overridePlan)

	var invalidPolicy string
	var invalidSnapshot json.RawMessage
	if err := tx.QueryRow(ctx, `select measurement_policy,measurement_plan_snapshot from site_fixes where project_id=$1 and id=$2`, invalidProjectID, invalidFixID).Scan(&invalidPolicy, &invalidSnapshot); err != nil {
		t.Fatal(err)
	}
	if invalidPolicy != "verification_only" || !jsonSemanticallyEqualForDBTest(invalidSnapshot, json.RawMessage(`{}`)) {
		t.Fatalf("unrecoverable legacy row was not safely downgraded: policy=%s snapshot=%s", invalidPolicy, invalidSnapshot)
	}

	var validated int
	if err := tx.QueryRow(ctx, `select count(*) from pg_constraint where conname in ('site_fixes_measurement_plan_snapshot_json_check','site_fixes_measurement_plan_alignment_check') and convalidated`).Scan(&validated); err != nil || validated != 2 {
		t.Fatalf("plan constraints validated=%d err=%v", validated, err)
	}
}

func jsonSemanticallyEqualForDBTest(left, right json.RawMessage) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	leftCanonical, _ := json.Marshal(leftValue)
	rightCanonical, _ := json.Marshal(rightValue)
	return string(leftCanonical) == string(rightCanonical)
}
