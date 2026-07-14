package measurement

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEvaluateSourceEvidenceMapsGSCGA4AndGEO(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	raw := json.RawMessage(`{
      "gsc":{"gsc_baseline_clicks":10,"gsc_baseline_impressions":100,"gsc_baseline_position":12,"gsc_baseline_rows":7,"gsc_baseline_partial":false,"gsc_baseline_updated_at":"2026-07-11T00:00:00Z","gsc_after_clicks":20,"gsc_after_impressions":120,"gsc_after_position":9,"gsc_after_rows":7,"gsc_after_partial":false,"gsc_after_updated_at":"2026-07-11T00:00:00Z"},
      "ga4":{"ga4_baseline_sessions":100,"ga4_baseline_engaged_sessions":50,"ga4_baseline_key_events":2,"ga4_baseline_rows":7,"ga4_baseline_updated_at":"2026-07-11T00:00:00Z","ga4_after_sessions":110,"ga4_after_engaged_sessions":66,"ga4_after_key_events":4,"ga4_after_rows":7,"ga4_after_updated_at":"2026-07-11T00:00:00Z"},
      "geo":{"geo_baseline_citations":0.1,"geo_baseline_brand_rate":0.3,"geo_baseline_rows":10,"geo_baseline_observed_at":"2026-07-11T00:00:00Z","geo_after_citations":0.4,"geo_after_brand_rate":0.5,"geo_after_rows":10,"geo_after_observed_at":"2026-07-11T00:00:00Z"},
      "windows":{"baseline_start":"2026-07-01","baseline_end":"2026-07-07","after_start":"2026-07-08","after_end":"2026-07-12"}
    }`)

	tests := []struct {
		metric    string
		direction string
		kind      string
		threshold float64
		want      string
	}{
		{metric: "gsc_clicks", direction: "increase", kind: "relative", threshold: .10, want: OutcomePositive},
		{metric: "ga4_engagement_rate", direction: "increase", kind: "relative", threshold: .10, want: OutcomePositive},
		{metric: "ga4_key_events", direction: "increase", kind: "relative", threshold: .10, want: OutcomePositive},
		{metric: "ai_citation_count", direction: "increase", kind: "absolute", threshold: .1, want: OutcomePositive},
	}
	for _, tt := range tests {
		t.Run(tt.metric, func(t *testing.T) {
			got, err := EvaluateSourceEvidence(MetricContract{Metric: tt.metric, Direction: tt.direction, ThresholdKind: tt.kind, ThresholdValue: tt.threshold}, raw, now)
			if err != nil {
				t.Fatal(err)
			}
			if got.OutcomeLabel != tt.want || len(got.SEOMetrics) == 0 || len(got.GA4Metrics) == 0 || len(got.GEOMetrics) == 0 || len(got.SourceFreshness) == 0 {
				t.Fatalf("evaluation contract missing for %s: %+v", tt.metric, got)
			}
		})
	}
}

func TestEvaluateSourceEvidenceUsesImmutableSpecBaseline(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	baseline := 100.0
	raw := json.RawMessage(`{"gsc":{"gsc_baseline_clicks":50,"gsc_baseline_impressions":100,"gsc_baseline_rows":7,"gsc_baseline_updated_at":"2026-07-11T00:00:00Z","gsc_after_clicks":110,"gsc_after_impressions":100,"gsc_after_rows":7,"gsc_after_updated_at":"2026-07-11T00:00:00Z"},"ga4":{},"geo":{},"windows":{}}`)
	got, err := EvaluateSourceEvidence(MetricContract{Metric: "gsc_clicks", Direction: "increase", ThresholdKind: "relative", ThresholdValue: .10, ImmutableBaselineValue: &baseline, ImmutableBaselineSampleSize: 100}, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.OutcomeLabel != OutcomePositive || got.BaselineValue != 100 || got.AfterValue != 110 {
		t.Fatalf("immutable baseline was not used: %+v", got)
	}
}

func TestEvaluateSourceEvidenceNormalizesCountWindowsAndPersistsUsedBaseline(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	baseline := 100.0
	raw := json.RawMessage(`{
      "gsc":{"gsc_baseline_clicks":50,"gsc_baseline_impressions":280,"gsc_baseline_rows":28,"gsc_baseline_data_through":"2026-06-28","gsc_after_clicks":60,"gsc_after_impressions":140,"gsc_after_rows":14,"gsc_after_data_through":"2026-07-14"},
      "ga4":{},"geo":{},"windows":{"baseline_start":"2026-06-01","baseline_end":"2026-06-28","after_start":"2026-07-01","after_end":"2026-07-14"}
    }`)
	got, err := EvaluateSourceEvidence(MetricContract{Metric: "gsc_clicks", Direction: "increase", ThresholdKind: "relative", ThresholdValue: .10, ImmutableBaselineValue: &baseline, ImmutableBaselineSampleSize: 280, ImmutableBaselineWindowDays: 28}, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.OutcomeLabel != OutcomePositive || got.BaselineValue != 100 || got.AfterValue != 120 {
		t.Fatalf("duration-normalized result is wrong: %+v", got)
	}
	if !strings.Contains(string(got.SEOMetrics), `"provenance":"bound_growth_spec"`) || !strings.Contains(string(got.SEOMetrics), `"value":100`) || !strings.Contains(string(got.SEOMetrics), `"normalized_window_days":28`) {
		t.Fatalf("ledger cannot reconstruct immutable-baseline evaluation: %s", got.SEOMetrics)
	}
}

func TestEvaluateSourceEvidenceRejectsUnsupportedMetric(t *testing.T) {
	if _, err := EvaluateSourceEvidence(MetricContract{Metric: "made_up"}, json.RawMessage(`{}`), time.Now()); err == nil {
		t.Fatal("unsupported metric must fail closed")
	}
}

func TestEvaluateSourceEvidenceReportsProviderUnavailable(t *testing.T) {
	raw := json.RawMessage(`{"gsc":{"gsc_baseline_clicks":10,"gsc_baseline_impressions":100,"gsc_baseline_rows":7,"gsc_baseline_updated_at":"2026-07-11T00:00:00Z"},"ga4":{},"geo":{},"providers":{"gsc":"expired"},"windows":{}}`)
	got, err := EvaluateSourceEvidence(MetricContract{Metric: "gsc_clicks", Direction: "increase", ThresholdKind: "relative", ThresholdValue: .1}, raw, time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got.OutcomeLabel != OutcomeInsufficientData || got.DataQualityState != QualityProviderUnavailable {
		t.Fatalf("provider availability not preserved: %+v", got)
	}
}

func TestEvaluateSourceEvidenceRequiresSubstantialWindowCoverage(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	baseline := 100.0
	raw := json.RawMessage(`{"gsc":{"gsc_baseline_clicks":100,"gsc_baseline_impressions":280,"gsc_baseline_rows":28,"gsc_baseline_data_through":"2026-06-30","gsc_after_clicks":20,"gsc_after_impressions":100,"gsc_after_rows":1,"gsc_after_data_through":"2026-07-28"},"ga4":{},"geo":{},"windows":{"baseline_start":"2026-06-03","baseline_end":"2026-06-30","after_start":"2026-07-01","after_end":"2026-07-28"}}`)
	got, err := EvaluateSourceEvidence(MetricContract{Metric: "gsc_clicks", Direction: "increase", ThresholdKind: "relative", ThresholdValue: .1, ImmutableBaselineValue: &baseline, ImmutableBaselineSampleSize: 280, ImmutableBaselineWindowDays: 28}, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.OutcomeLabel != OutcomeInsufficientData || got.DataQualityState != QualityInsufficient {
		t.Fatalf("one fresh day must not terminalize a 28-day checkpoint: %+v", got)
	}
}

func TestEvaluateSourceEvidenceKeepsSmallGA4SampleInsufficient(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	raw := json.RawMessage(`{"gsc":{},"ga4":{"ga4_baseline_sessions":100,"ga4_baseline_engaged_sessions":50,"ga4_baseline_rows":28,"ga4_baseline_data_through":"2026-06-30","ga4_after_sessions":10,"ga4_after_engaged_sessions":9,"ga4_after_rows":28,"ga4_after_data_through":"2026-07-28"},"geo":{},"windows":{"baseline_start":"2026-06-03","baseline_end":"2026-06-30","after_start":"2026-07-01","after_end":"2026-07-28"}}`)
	got, err := EvaluateSourceEvidence(MetricContract{Metric: "ga4_engagement_rate", Direction: "increase", ThresholdKind: "relative", ThresholdValue: .1}, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.OutcomeLabel != OutcomeInsufficientData {
		t.Fatalf("small GA4 sample must remain insufficient: %+v", got)
	}
}

func TestEvaluateSourceEvidenceHonorsFrozenCoverageAndGuardrails(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	raw := json.RawMessage(`{"gsc":{"gsc_baseline_clicks":10,"gsc_baseline_impressions":100,"gsc_baseline_rows":7,"gsc_baseline_data_through":"2026-06-30","gsc_after_clicks":20,"gsc_after_impressions":79,"gsc_after_rows":7,"gsc_after_data_through":"2026-07-14"},"ga4":{},"geo":{},"windows":{"baseline_start":"2026-06-24","baseline_end":"2026-06-30","after_start":"2026-07-08","after_end":"2026-07-14"}}`)
	got, err := EvaluateSourceEvidence(MetricContract{Metric: "gsc_ctr", Direction: "increase", ThresholdKind: "relative", ThresholdValue: .1, MinimumAfterRows: 8, MinimumAfterSample: 50, GuardrailThresholds: map[string]float64{"gsc_impressions": .2}, UseExplicitMinimumSample: true, UseExplicitGuardrails: true}, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.OutcomeLabel != OutcomeInsufficientData {
		t.Fatalf("frozen minimum ignored: %+v", got)
	}
	got, err = EvaluateSourceEvidence(MetricContract{Metric: "gsc_ctr", Direction: "increase", ThresholdKind: "relative", ThresholdValue: .1, MinimumAfterRows: 7, MinimumAfterSample: 50, GuardrailThresholds: map[string]float64{"gsc_impressions": .2}, UseExplicitMinimumSample: true, UseExplicitGuardrails: true}, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.OutcomeLabel != OutcomeMixed {
		t.Fatalf("frozen guardrail ignored: %+v", got)
	}
	got, err = EvaluateSourceEvidence(MetricContract{Metric: "gsc_ctr", Direction: "increase", ThresholdKind: "relative", ThresholdValue: .1, MinimumAfterRows: 7, MinimumAfterSample: 50, GuardrailThresholds: map[string]float64{}, UseExplicitMinimumSample: true, UseExplicitGuardrails: true}, raw, now)
	if err != nil || got.OutcomeLabel != OutcomePositive {
		t.Fatalf("undeclared guardrail affected outcome: %+v err=%v", got, err)
	}
}

func TestEvaluateSourceEvidenceUsesImmutableGuardrailBaseline(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	raw := json.RawMessage(`{"gsc":{"gsc_baseline_clicks":10,"gsc_baseline_impressions":10000,"gsc_baseline_rows":7,"gsc_baseline_data_through":"2026-06-30","gsc_after_clicks":160,"gsc_after_impressions":800,"gsc_after_rows":7,"gsc_after_data_through":"2026-07-14"},"ga4":{},"geo":{},"windows":{"baseline_start":"2026-06-24","baseline_end":"2026-06-30","after_start":"2026-07-08","after_end":"2026-07-14"}}`)
	baseline := .10
	got, err := EvaluateSourceEvidence(MetricContract{Metric: "gsc_ctr", Direction: "increase", ThresholdKind: "relative", ThresholdValue: .1, ImmutableBaselineValue: &baseline, ImmutableBaselineSampleSize: 1000, ImmutableBaselineRows: 7, ImmutableGuardrails: map[string]ImmutableMetricBaseline{"gsc_impressions": {Value: 1000, SampleSize: 1000, Rows: 7}}, GuardrailThresholds: map[string]float64{"gsc_impressions": .15}, UseExplicitGuardrails: true, UseExplicitMinimumSample: true, MinimumAfterRows: 1, MinimumAfterSample: 1}, raw, now)
	if err != nil || got.OutcomeLabel != OutcomeMixed {
		t.Fatalf("frozen guardrail baseline changed outcome: %+v err=%v", got, err)
	}
	if !strings.Contains(string(got.SEOMetrics), `"sample_size":1000`) || strings.Contains(string(got.SEOMetrics), `"sample_size":10000`) {
		t.Fatalf("live baseline sample leaked into ledger: %s", got.SEOMetrics)
	}
}
