package measurement

import (
	"testing"
	"time"
)

func TestEvaluateClassifiesDirectionalOutcomes(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-48 * time.Hour)
	tests := []struct {
		name      string
		input     EvaluationInput
		wantLabel string
	}{
		{
			name: "relative increase is positive",
			input: EvaluationInput{Metric: "gsc_ctr", Direction: "increase", ThresholdKind: "relative", ThresholdValue: .10,
				Baseline: sample(.10, 100, 28, fresh), After: sample(.13, 120, 28, fresh), Now: now},
			wantLabel: OutcomePositive,
		},
		{
			name: "absolute decrease is positive for rank position",
			input: EvaluationInput{Metric: "gsc_position", Direction: "decrease", ThresholdKind: "absolute", ThresholdValue: 1,
				Baseline: sample(10, 500, 28, fresh), After: sample(8.5, 600, 28, fresh), Now: now},
			wantLabel: OutcomePositive,
		},
		{
			name: "opposite threshold is negative",
			input: EvaluationInput{Metric: "ga4_engagement_rate", Direction: "increase", ThresholdKind: "relative", ThresholdValue: .10,
				Baseline: sample(.60, 300, 28, fresh), After: sample(.45, 280, 28, fresh), Now: now},
			wantLabel: OutcomeNegative,
		},
		{
			name: "complete but small movement is inconclusive",
			input: EvaluationInput{Metric: "gsc_clicks", Direction: "increase", ThresholdKind: "relative", ThresholdValue: .10,
				Baseline: sample(100, 1000, 28, fresh), After: sample(105, 1100, 28, fresh), Now: now},
			wantLabel: OutcomeInconclusive,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.input)
			if got.OutcomeLabel != tt.wantLabel {
				t.Fatalf("outcome=%s want=%s result=%+v", got.OutcomeLabel, tt.wantLabel, got)
			}
			if got.DataQualityState != QualityComplete || got.DeltaAbsolute == nil || got.AttributionConfidence == "" {
				t.Fatalf("complete evaluation contract missing: %+v", got)
			}
		})
	}
}

func TestEvaluateMarksSplitPrimaryAndGuardrailMixed(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-24 * time.Hour)
	got := Evaluate(EvaluationInput{
		Metric: "gsc_ctr", Direction: "increase", ThresholdKind: "relative", ThresholdValue: .10,
		Baseline: sample(.10, 1000, 28, fresh), After: sample(.13, 500, 28, fresh), Now: now,
		Guardrails: []Guardrail{{Name: "gsc_impressions", Baseline: sample(1000, 1000, 28, fresh), After: sample(500, 500, 28, fresh), MaxAdverseRelative: .30}},
	})
	if got.OutcomeLabel != OutcomeMixed || len(got.Confounders) == 0 {
		t.Fatalf("expected mixed with guardrail confounder: %+v", got)
	}
}

func TestEvaluateRefusesMissingStaleAndZeroSampleEvidence(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		after MetricSample
		want  string
	}{
		{name: "missing", after: MetricSample{}, want: QualityInsufficient},
		{name: "zero sample", after: sample(10, 0, 5, now), want: QualityInsufficient},
		{name: "stale", after: sample(10, 100, 5, now.Add(-10*24*time.Hour)), want: QualityStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(EvaluationInput{Metric: "gsc_clicks", Direction: "increase", ThresholdKind: "relative", ThresholdValue: .10,
				Baseline: sample(8, 100, 5, now.Add(-24*time.Hour)), After: tt.after, Now: now, FreshnessTolerance: 5 * 24 * time.Hour})
			if got.OutcomeLabel != OutcomeInsufficientData || got.DataQualityState != tt.want {
				t.Fatalf("result=%+v want quality=%s", got, tt.want)
			}
		})
	}
}

func TestEvaluateKeepsPartialEvidenceButLowersConfidence(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	baseline := sample(10, 100, 7, now.Add(-24*time.Hour))
	after := sample(12, 100, 7, now.Add(-24*time.Hour))
	after.Partial = true
	got := Evaluate(EvaluationInput{Metric: "gsc_clicks", Direction: "increase", ThresholdKind: "relative", ThresholdValue: .10,
		Baseline: baseline, After: after, Now: now})
	if got.OutcomeLabel != OutcomePositive || got.DataQualityState != QualityPartial || got.AttributionConfidence != "low" {
		t.Fatalf("partial evidence should classify transparently with low confidence: %+v", got)
	}
}

func sample(value, sampleSize float64, rows int, observedThrough time.Time) MetricSample {
	return MetricSample{Value: value, SampleSize: sampleSize, Rows: rows, Available: true, ObservedThrough: observedThrough}
}
