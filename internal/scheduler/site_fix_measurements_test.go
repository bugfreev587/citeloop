package scheduler

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/measurement"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
)

func TestSiteFixCheckpointScheduleIsDeterministic(t *testing.T) {
	started := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	policy, err := measurement.Parse(json.RawMessage(`{"policy_version":"v1","early_signal_offset_days":7,"primary_checkpoint_offset_days":14,"follow_up_offsets_days":[21],"max_follow_up_attempts":1,"max_measuring_duration_days":21,"terminalization_grace_period_days":2}`))
	if err != nil {
		t.Fatal(err)
	}
	measurementRow := db.SiteFixMeasurement{ID: uuid.New(), ProjectID: uuid.New(), StartedAt: pgutil.TS(started), BaselineWindow: json.RawMessage(`{"start":"2026-06-01T00:00:00Z","end":"2026-06-28T00:00:00Z"}`)}
	got, err := siteFixCheckpointSchedule(measurementRow, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[0].Role != "baseline" || got[1].Role != "early_signal" || !got[2].ScheduledAt.Equal(started.AddDate(0, 0, 14)) || got[3].Attempt != 1 {
		t.Fatalf("schedule=%+v", got)
	}
}

func TestSiteFixMetricContractUsesFrozenPolicy(t *testing.T) {
	row := db.SiteFixMeasurement{
		PrimaryMetric:             "ctr",
		BaselineSnapshot:          json.RawMessage(`{"ctr":{"value":0.04,"sample_size":900,"rows":28,"partial":true},"gsc_impressions":{"value":1000,"sample_size":1000,"rows":28,"partial":false}}`),
		MeasurementPolicySnapshot: json.RawMessage(`{"metric_thresholds":{"direction":"increase","kind":"relative","value":0.12},"minimum_sample":{"minimum_after_periods":7,"minimum_after_sample":100},"guardrails":[{"metric":"gsc_impressions","max_adverse_relative":0.2}]}`),
	}
	contract, err := siteFixMetricContract(row)
	if err != nil {
		t.Fatal(err)
	}
	if contract.Metric != "gsc_ctr" || contract.ThresholdValue != .12 || contract.MinimumAfterRows != 7 || contract.MinimumAfterSample != 100 || contract.GuardrailThresholds["gsc_impressions"] != .2 || contract.ImmutableBaselineValue == nil || *contract.ImmutableBaselineValue != .04 || contract.ImmutableBaselineSampleSize != 900 || contract.ImmutableBaselineRows != 28 || !contract.ImmutableBaselinePartial {
		t.Fatalf("contract=%+v", contract)
	}
}

func TestSiteFixMetricContractRejectsLegacyBaselineWithoutFrozenMetadata(t *testing.T) {
	row := db.SiteFixMeasurement{PrimaryMetric: "ctr", BaselineSnapshot: json.RawMessage(`{"ctr":0.04}`), MeasurementPolicySnapshot: json.RawMessage(`{"metric_thresholds":{"direction":"increase","kind":"relative","value":0.12},"minimum_sample":{"minimum_after_periods":7,"minimum_after_sample":100},"guardrails":[]}`)}
	if _, err := siteFixMetricContract(row); err == nil {
		t.Fatal("legacy baseline without frozen sample, rows, and partial metadata was accepted")
	}
}

func TestSiteFixOutcomeRules(t *testing.T) {
	for _, tc := range []struct {
		role, outcome         string
		prospective, deadline bool
		wantTerminal          bool
		want                  string
	}{
		{"baseline", measurement.OutcomePositive, false, false, false, ""},
		{"early_signal", measurement.OutcomePositive, false, false, false, ""},
		{"primary", measurement.OutcomePositive, false, false, true, measurement.OutcomePositive},
		{"primary", measurement.OutcomeNegative, false, false, true, measurement.OutcomeNegative},
		{"primary", measurement.OutcomeInconclusive, false, false, true, measurement.OutcomeInconclusive},
		{"primary", measurement.OutcomeInsufficientData, false, false, false, ""},
		{"follow_up", measurement.OutcomeMixed, false, false, true, measurement.OutcomeMixed},
		{"follow_up", measurement.OutcomeInsufficientData, false, true, true, measurement.OutcomeInsufficientData},
		{"primary", measurement.OutcomePositive, true, false, true, measurement.OutcomeInsufficientData},
	} {
		terminal, outcome := siteFixTerminalDecision(tc.role, tc.outcome, tc.prospective, tc.deadline)
		if terminal != tc.wantTerminal || outcome != tc.want {
			t.Fatalf("%+v got terminal=%v outcome=%s", tc, terminal, outcome)
		}
	}
}

func TestSiteFixHandoffBackoffIsFinite(t *testing.T) {
	base := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	if got := siteFixHandoffRetryAt(base, 1); !got.Equal(base.Add(time.Minute)) {
		t.Fatal(got)
	}
	if got := siteFixHandoffRetryAt(base, 20); !got.Equal(base.Add(time.Hour)) {
		t.Fatal(got)
	}
}

func TestSiteFixMetricVocabularyHasRealAdapters(t *testing.T) {
	for input, want := range map[string]string{"ctr": "gsc_ctr", "clicks": "gsc_clicks", "impressions": "gsc_impressions", "position": "gsc_position", "citations": "ai_citation_count", "brand_mentions": "ai_brand_mention_rate", "conversion_rate": "ga4_conversion_rate", "qualified_actions": "ga4_key_events", "referral_sessions": "ga4_sessions"} {
		got, err := siteFixMetricName(input)
		if err != nil || got != want {
			t.Fatalf("%s => %s, %v", input, got, err)
		}
	}
	if _, err := siteFixMetricName("silent_default"); err == nil {
		t.Fatal("unsupported metrics must fail closed")
	}
}

func TestSiteFixGEOEvidenceIDsAreValidatedAndDeduplicated(t *testing.T) {
	id := uuid.New()
	got, err := siteFixGEOEvidenceIDs(json.RawMessage(`{"geo_evidence_ids":["` + id.String() + `","` + id.String() + `"]}`))
	if err != nil || len(got) != 1 || got[0] != id.String() {
		t.Fatalf("ids=%v err=%v", got, err)
	}
	if _, err := siteFixGEOEvidenceIDs(json.RawMessage(`{"evidence_ids":["not-a-uuid"]}`)); err == nil {
		t.Fatal("invalid evidence id accepted")
	}
}

func TestSiteFixEvidenceFailureOnlyTerminalizesAtFiniteDeadline(t *testing.T) {
	errEvidence := errors.New("provider timeout")
	if terminal, _ := siteFixDeadlineEvidenceFailure(false, errEvidence); terminal {
		t.Fatal("transient pre-deadline error must roll back for retry")
	}
	terminal, reason := siteFixDeadlineEvidenceFailure(true, errEvidence)
	if !terminal || reason == "" {
		t.Fatalf("deadline error terminal=%v reason=%q", terminal, reason)
	}
}

func TestSiteFixActivationUsesPersistedHandoffTime(t *testing.T) {
	created := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	verified := created.Add(-time.Minute)
	event := db.SiteFixMeasurementHandoffOutbox{CreatedAt: pgutil.TS(created), NextAttemptAt: pgutil.TS(created.Add(time.Hour)), OccurredAt: pgutil.TS(verified)}
	if got := siteFixHandoffStartedAt(event); !got.Equal(verified) {
		t.Fatalf("started_at=%s", got)
	}
	// Retry scheduling never changes the immutable occurrence.
	event.NextAttemptAt = pgutil.TS(created.Add(time.Hour))
	if got := siteFixHandoffStartedAt(event); !got.Equal(verified) {
		t.Fatalf("retried started_at=%s", got)
	}
}
