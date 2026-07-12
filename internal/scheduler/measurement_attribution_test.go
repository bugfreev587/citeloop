package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/measurement"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestLegacyMeasuringPolicyBindsBeforeCheckpointEvaluation(t *testing.T) {
	raw, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	bind := strings.Index(source, "q.BindLegacyMeasuringContentActionPolicy")
	evaluate := strings.Index(source, "completeActionMeasurementCheckpoints(action, now)")
	if bind < 0 || evaluate < 0 || bind > evaluate {
		t.Fatal("legacy measuring action must bind its policy before checkpoint evaluation")
	}
}

func TestTerminalLearningRecordsInsideMeasurementTransaction(t *testing.T) {
	raw, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	update := strings.Index(source, "q.UpdateContentActionOutcomeSummary")
	record := strings.Index(source, "learning.RecordTerminalOutcome")
	commit := strings.Index(source, "return tx.Commit(ctx)")
	if update < 0 || record < update || commit < record {
		t.Fatal("terminal action update and learning/quality record must commit in one transaction")
	}
}

func TestMeasurementOutcomeSummaryExplainsInsufficientData(t *testing.T) {
	action := db.ContentAction{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
		Status:    "measuring",
	}
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	window := json.RawMessage(`{"primary_metric":"clicks","checkpoints":[{"day":7,"status":"completed","outcome_label":"insufficient_data"}]}`)

	raw := measurementOutcomeSummary(action, "measuring", 1, 1, now, window)

	var summary map[string]any
	if err := json.Unmarshal(raw, &summary); err != nil {
		t.Fatal(err)
	}
	if summary["outcome_label"] != "insufficient_data" {
		t.Fatalf("outcome_label = %#v, want insufficient_data; summary=%s", summary["outcome_label"], string(raw))
	}
	if summary["outcome_reason"] == "" {
		t.Fatalf("outcome_reason should explain why attribution is unavailable: %s", string(raw))
	}
	if summary["attribution_confidence"] != "low" {
		t.Fatalf("attribution_confidence = %#v, want low", summary["attribution_confidence"])
	}
	confounders, ok := summary["confounders"].([]any)
	if !ok || len(confounders) == 0 {
		t.Fatalf("confounders = %#v, want at least one explanatory note", summary["confounders"])
	}
}

func TestDueCheckpointCarriesOutcomeLabelAndReason(t *testing.T) {
	publishedAt := pgtype.Timestamptz{Time: time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC), Valid: true}
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	rawWindow := json.RawMessage(`{"primary_metric":"clicks","checkpoints":[{"day":7,"status":"scheduled"}]}`)

	window, completed, remaining := completeDueMeasurementCheckpoints(rawWindow, publishedAt, now)
	if completed != 1 || remaining != 4 {
		t.Fatalf("completed=%d remaining=%d, want 1/4 finite legacy schedule", completed, remaining)
	}
	var data struct {
		Checkpoints []struct {
			Role          string `json:"role"`
			Status        string `json:"status"`
			OutcomeLabel  string `json:"outcome_label"`
			OutcomeReason string `json:"outcome_reason"`
		} `json:"checkpoints"`
	}
	if err := json.Unmarshal(window, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Checkpoints) != 5 {
		t.Fatalf("checkpoints=%d, want 5", len(data.Checkpoints))
	}
	if data.Checkpoints[0].Role != "baseline" || data.Checkpoints[0].OutcomeLabel != "insufficient_data" {
		t.Fatalf("checkpoint outcome_label=%q, want insufficient_data; window=%s", data.Checkpoints[0].OutcomeLabel, string(window))
	}
	if data.Checkpoints[0].OutcomeReason == "" {
		t.Fatalf("checkpoint outcome_reason should be present; window=%s", string(window))
	}
}

func TestAbsoluteDeadlineTerminalizesRemainingCheckpoints(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	deadline := start.AddDate(0, 0, 91)
	action := db.ContentAction{
		ID: uuid.New(), ProjectID: uuid.New(), Status: "measuring",
		MeasurementPolicyVersion: "growth-measurement-v1",
		MeasurementPolicy: json.RawMessage(`{
          "policy_version":"growth-measurement-v1","early_signal_offset_days":28,
          "primary_checkpoint_offset_days":56,"follow_up_offsets_days":[70,84],
          "max_follow_up_attempts":2,"max_measuring_duration_days":84,
          "terminalization_grace_period_days":7
        }`),
		MeasuringStartedAt: pgtype.Timestamptz{Time: start, Valid: true},
		AbsoluteTerminalAt: pgtype.Timestamptz{Time: deadline, Valid: true},
	}
	window, _, remaining, terminalReason := completeActionMeasurementCheckpoints(action, deadline.Add(time.Minute))
	if remaining != 0 || terminalReason == nil || *terminalReason != "absolute_deadline_reached" {
		t.Fatalf("remaining=%d terminalReason=%v window=%s", remaining, terminalReason, window)
	}
	measurements := actionMeasurementsFromWindow(action, window, deadline.Add(time.Minute))
	if len(measurements) != 5 {
		t.Fatalf("measurements=%d, want all terminal checkpoints", len(measurements))
	}
	for _, item := range measurements {
		if item.MeasurementPolicyVersion != "growth-measurement-v1" || item.CheckpointRole == "" || item.DataQualityState != "insufficient" {
			t.Fatalf("measurement contract missing: %#v", item)
		}
	}
}

func TestMeasurementContractUsesDecisionReadyGrowthSpec(t *testing.T) {
	baseline := 10.0
	opportunity := db.SeoOpportunity{GrowthSpec: json.RawMessage(`{
      "primary_metric":"gsc_clicks",
		"baseline":{"metric":"gsc_clicks","value":10,"sample_size":100,"window_start":"2026-06-01","window_end":"2026-06-28","evidence_ids":["observation-1"]},
      "expected_change":{"direction":"increase","decision_threshold":{"kind":"relative","value":0.1}}
    }`)}
	contract := measurementContractForAction(opportunity, json.RawMessage(`{"primary_metric":"clicks"}`))
	if contract.Metric != "gsc_clicks" || contract.Direction != "increase" || contract.ThresholdKind != "relative" || contract.ThresholdValue != .1 {
		t.Fatalf("decision-ready contract not used: %+v", contract)
	}
	if contract.ImmutableBaselineValue == nil || *contract.ImmutableBaselineValue != baseline || contract.ImmutableBaselineSampleSize != 100 {
		t.Fatalf("immutable baseline missing: %+v", contract)
	}
	if contract.ImmutableBaselineWindowDays != 28 || len(measurementGeoEvidenceIDs(opportunity)) != 1 {
		t.Fatalf("baseline window or evidence identity missing: %+v ids=%v", contract, measurementGeoEvidenceIDs(opportunity))
	}
}

func TestPrimaryRealOutcomeTerminalizesBoundedFollowUps(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	raw := json.RawMessage(`{"checkpoints":[
      {"day":0,"role":"baseline","status":"completed","completed_at":"2026-06-01T00:00:00Z"},
      {"day":14,"role":"early","status":"completed","completed_at":"2026-06-15T00:00:00Z"},
      {"day":28,"role":"primary","status":"completed","completed_at":"2026-07-12T12:00:00Z"},
      {"day":56,"role":"follow_up","status":"scheduled"},
      {"day":90,"role":"follow_up","status":"scheduled"}
    ]}`)
	evaluation := measurement.EvidenceEvaluation{Evaluation: measurement.Evaluation{
		OutcomeLabel: measurement.OutcomePositive, OutcomeReason: "clicks improved", AttributionConfidence: "high",
		DataQualityState: measurement.QualityComplete, Confounders: []string{}, GuardrailResults: map[string]any{},
	}, SEOMetrics: json.RawMessage(`{"after":{"clicks":20}}`), GA4Metrics: json.RawMessage(`{}`), GEOMetrics: json.RawMessage(`{}`), SourceFreshness: json.RawMessage(`{"gsc":{"after_updated_at":"2026-07-11T00:00:00Z"}}`)}

	window, remaining, reason := applyMeasurementEvaluations(raw, now, map[int]measurement.EvidenceEvaluation{28: evaluation})
	if remaining != 0 || reason != "decision_outcome_reached" {
		t.Fatalf("remaining=%d reason=%q window=%s", remaining, reason, window)
	}
	var data map[string]any
	if err := json.Unmarshal(window, &data); err != nil {
		t.Fatal(err)
	}
	checkpoints := data["checkpoints"].([]any)
	primary := checkpoints[2].(map[string]any)
	if primary["outcome_label"] != "positive" || primary["seo_metrics"] == nil || primary["source_freshness"] == nil {
		t.Fatalf("real primary evidence missing: %#v", primary)
	}
	for _, index := range []int{3, 4} {
		if checkpoints[index].(map[string]any)["status"] != "skipped" {
			t.Fatalf("follow-up %d not terminalized: %#v", index, checkpoints[index])
		}
	}
	measurements := actionMeasurementsFromWindow(db.ContentAction{ID: uuid.New(), ProjectID: uuid.New(), MeasurementPolicyVersion: "growth-measurement-v1"}, window, now)
	if len(measurements) != 1 || string(measurements[0].SeoMetrics) == "{}" || len(measurements[0].SourceFreshness) == 0 {
		t.Fatalf("checkpoint ledger did not persist real metrics: %#v", measurements)
	}
}

func TestEarlyOutcomeDoesNotSkipPrimaryCheckpoint(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	raw := json.RawMessage(`{"checkpoints":[{"day":14,"role":"early","status":"completed","completed_at":"2026-07-12T12:00:00Z"},{"day":28,"role":"primary","status":"scheduled"}]}`)
	evaluation := measurement.EvidenceEvaluation{Evaluation: measurement.Evaluation{OutcomeLabel: measurement.OutcomePositive, OutcomeReason: "early signal", AttributionConfidence: "medium", DataQualityState: measurement.QualityComplete, GuardrailResults: map[string]any{}}, SEOMetrics: json.RawMessage(`{}`), GA4Metrics: json.RawMessage(`{}`), GEOMetrics: json.RawMessage(`{}`), SourceFreshness: json.RawMessage(`{}`)}
	_, remaining, reason := applyMeasurementEvaluations(raw, now, map[int]measurement.EvidenceEvaluation{14: evaluation})
	if remaining != 1 || reason != "" {
		t.Fatalf("early checkpoint must keep primary scheduled; remaining=%d reason=%q", remaining, reason)
	}
}

type measurementEvidenceStub struct {
	params db.GetGrowthMeasurementEvidenceParams
	raw    json.RawMessage
	raws   []json.RawMessage
	calls  int
}

func (stub *measurementEvidenceStub) GetGrowthMeasurementEvidence(_ context.Context, params db.GetGrowthMeasurementEvidenceParams) (json.RawMessage, error) {
	stub.params = params
	if stub.calls < len(stub.raws) {
		raw := stub.raws[stub.calls]
		stub.calls++
		return raw, nil
	}
	stub.calls++
	return stub.raw, nil
}

func TestEvaluateMeasurementCheckpointsUsesTargetQueryAndRealWindows(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	started := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	target := "https://example.com/page"
	query := "example query"
	stub := &measurementEvidenceStub{raw: json.RawMessage(`{
      "gsc":{"gsc_baseline_clicks":10,"gsc_baseline_impressions":100,"gsc_baseline_rows":28,"gsc_baseline_updated_at":"2026-07-14T00:00:00Z","gsc_after_clicks":20,"gsc_after_impressions":110,"gsc_after_rows":14,"gsc_after_updated_at":"2026-07-14T00:00:00Z"},
      "ga4":{},"geo":{},"windows":{}
    }`)}
	action := db.ContentAction{ID: uuid.New(), ProjectID: uuid.New(), OpportunityID: uuid.New(), NormalizedTargetUrl: &target, MeasuringStartedAt: pgtype.Timestamptz{Time: started, Valid: true}}
	opportunity := db.SeoOpportunity{ID: action.OpportunityID, ProjectID: action.ProjectID, Query: &query, GrowthSpec: json.RawMessage(`{"primary_metric":"gsc_clicks","expected_change":{"direction":"increase","decision_threshold":{"kind":"relative","value":0.1}},"baseline":{"metric":"gsc_clicks","value":10,"sample_size":100,"window_start":"2026-06-01","window_end":"2026-06-28"}}`)}
	window := json.RawMessage(`{"primary_metric":"clicks","checkpoints":[{"day":14,"role":"early","status":"completed","completed_at":"2026-07-15T12:00:00Z"},{"day":28,"role":"primary","status":"scheduled"}]}`)

	updated, remaining, reason, err := evaluateMeasurementCheckpoints(context.Background(), stub, action, opportunity, window, now)
	if err != nil {
		t.Fatal(err)
	}
	if remaining != 1 || reason != "" || stub.params.TargetUrl != target || stub.params.Query != query {
		t.Fatalf("unexpected orchestration: remaining=%d reason=%q params=%+v", remaining, reason, stub.params)
	}
	if got := stub.params.BaselineStart.Time.Format("2006-01-02"); got != "2026-06-01" {
		t.Fatalf("baseline start=%s", got)
	}
	if got := stub.params.AfterEnd.Time.Format("2006-01-02"); got != "2026-07-14" {
		t.Fatalf("after end=%s", got)
	}
	if !strings.Contains(string(updated), `"outcome_label":"positive"`) || !strings.Contains(string(updated), `"seo_metrics"`) {
		t.Fatalf("real evaluation missing from window: %s", updated)
	}
}

func TestDelayedReconcileStopsAtFirstTerminalCheckpoint(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	started := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	target := "https://example.com/page"
	metricJSON := func(after float64) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"gsc":{"gsc_baseline_clicks":100,"gsc_baseline_impressions":280,"gsc_baseline_rows":28,"gsc_baseline_data_through":"2026-04-30","gsc_after_clicks":%g,"gsc_after_impressions":280,"gsc_after_rows":28,"gsc_after_data_through":"2026-07-14"},"ga4":{},"geo":{},"windows":{"baseline_start":"2026-04-03","baseline_end":"2026-04-30","after_start":"2026-05-01","after_end":"2026-05-28"}}`, after))
	}
	stub := &measurementEvidenceStub{raws: []json.RawMessage{metricJSON(80), metricJSON(120)}}
	action := db.ContentAction{ID: uuid.New(), ProjectID: uuid.New(), OpportunityID: uuid.New(), NormalizedTargetUrl: &target, MeasuringStartedAt: pgtype.Timestamptz{Time: started, Valid: true}}
	opportunity := db.SeoOpportunity{ID: action.OpportunityID, ProjectID: action.ProjectID, GrowthSpec: json.RawMessage(`{"primary_metric":"gsc_clicks","expected_change":{"direction":"increase","decision_threshold":{"kind":"relative","value":0.1}},"baseline":{"metric":"gsc_clicks","value":100,"sample_size":280,"window_start":"2026-04-03","window_end":"2026-04-30"}}`)}
	window := json.RawMessage(`{"checkpoints":[{"day":28,"role":"primary","status":"completed","completed_at":"2026-07-15T12:00:00Z"},{"day":56,"role":"follow_up","status":"completed","completed_at":"2026-07-15T12:00:00Z"},{"day":90,"role":"follow_up","status":"scheduled"}]}`)

	updated, remaining, reason, err := evaluateMeasurementCheckpoints(context.Background(), stub, action, opportunity, window, now)
	if err != nil {
		t.Fatal(err)
	}
	if stub.calls != 1 || remaining != 0 || reason != "decision_outcome_reached" {
		t.Fatalf("delayed reconcile did not stop at primary: calls=%d remaining=%d reason=%q window=%s", stub.calls, remaining, reason, updated)
	}
	var data map[string]any
	_ = json.Unmarshal(updated, &data)
	checkpoints := data["checkpoints"].([]any)
	if checkpoints[0].(map[string]any)["outcome_label"] != "negative" || checkpoints[1].(map[string]any)["status"] != "skipped" {
		t.Fatalf("primary outcome was overwritten or follow-up persisted: %s", updated)
	}
	measurements := actionMeasurementsFromWindow(action, updated, now)
	if len(measurements) != 1 || measurements[0].CheckpointDay != 28 || measurements[0].OutcomeLabel != "negative" {
		t.Fatalf("immutable ledger should contain only terminal primary: %#v", measurements)
	}
}

func TestLegacyEvidenceWindowsAreDurationMatched(t *testing.T) {
	started := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	action := db.ContentAction{MeasuringStartedAt: pgtype.Timestamptz{Time: started, Valid: true}}
	baselineStart, baselineEnd, afterStart, afterEnd := measurementEvidenceDates(db.SeoOpportunity{}, action, 14, started.AddDate(0, 0, 14))
	if baselineStart.Format("2006-01-02") != "2026-06-17" || baselineEnd.Format("2006-01-02") != "2026-06-30" || afterStart.Format("2006-01-02") != "2026-07-01" || afterEnd.Format("2006-01-02") != "2026-07-14" {
		t.Fatalf("windows are not matched 14-day periods: %s %s %s %s", baselineStart, baselineEnd, afterStart, afterEnd)
	}
}
