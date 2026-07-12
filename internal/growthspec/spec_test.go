package growthspec

import (
	"encoding/json"
	"slices"
	"testing"
	"time"
)

func TestBuildDecisionReadyLowCTRSpecification(t *testing.T) {
	result := Build(Input{
		Type:              "gsc_low_ctr_query",
		Query:             "best ai visibility platform",
		TargetURL:         "https://example.com/ai-visibility",
		RecommendedAction: "Rewrite the search snippet",
		ExpectedImpact:    "Capture more existing impressions",
		Evidence: mustJSON(t, map[string]any{
			"source": "gsc_search_analytics", "reason": "low_ctr",
			"clicks_28d": 12.0, "impressions_28d": 1200.0, "ctr_28d": 0.01,
			"position_28d": 4.2, "window_start": "2026-06-01", "window_end": "2026-06-28",
			"why_now": "Search Console shows high visibility but weak click capture.",
		}),
	})
	if result.State != StateDecisionReady || len(result.Missing) != 0 {
		t.Fatalf("result = %#v", result)
	}
	if result.Spec.PrimaryMetric != "gsc_ctr" || result.Spec.Baseline.Value != 0.01 {
		t.Fatalf("unexpected metric/baseline: %#v", result.Spec)
	}
	if result.Spec.ExpectedChange.Direction != "increase" {
		t.Fatalf("direction = %q", result.Spec.ExpectedChange.Direction)
	}
	if result.Spec.MeasurementPolicy.MaxMeasuringDurationDays <= result.Spec.MeasurementPolicy.PrimaryCheckpointOffsetDays {
		t.Fatal("measurement policy must provide a finite follow-up window")
	}
}

func TestBuildRejectsWrongSourceAndInvalidMetricDomain(t *testing.T) {
	wrongSource := Build(Input{
		Type: "gsc_low_ctr_query", Query: "best ai visibility platform", RecommendedAction: "Rewrite the search snippet",
		Now: time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC),
		Evidence: mustJSON(t, map[string]any{
			"source": "ga4", "ctr_28d": 0.01, "impressions_28d": 1200,
			"window_start": "2026-06-01", "window_end": "2026-06-28",
		}),
	})
	if wrongSource.State != StateNeedsEvidence || !slices.Contains(wrongSource.Missing, "baseline_source") {
		t.Fatalf("wrong source result = %#v", wrongSource)
	}

	invalidCTR := Build(Input{
		Type: "gsc_low_ctr_query", Query: "best ai visibility platform", RecommendedAction: "Rewrite the search snippet",
		Now: time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC),
		Evidence: mustJSON(t, map[string]any{
			"source": "gsc_search_analytics", "ctr_28d": 1.2, "impressions_28d": 1200,
			"window_start": "2026-06-01", "window_end": "2026-06-28",
		}),
	})
	if invalidCTR.State != StateNeedsEvidence || !slices.Contains(invalidCTR.Missing, "baseline_value") {
		t.Fatalf("invalid CTR result = %#v", invalidCTR)
	}
}

func TestBuildGA4DecisionReadyAndEvidenceGuards(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	base := map[string]any{
		"source": "ga4", "observation_state": "observed",
		"engagement_rate_28d": 0.31, "sessions_28d": 420,
		"window_start": "2026-06-01", "window_end": "2026-06-28",
	}
	valid := Build(Input{
		Type: "ga4_low_engagement", TargetURL: "https://example.com/pricing",
		RecommendedAction: "Improve the pricing page's proof and CTA hierarchy", Now: now, Evidence: mustJSON(t, base),
	})
	if valid.State != StateDecisionReady || valid.Spec.PrimaryMetric != "ga4_engagement_rate" {
		t.Fatalf("valid GA4 result = %#v", valid)
	}

	for name, test := range map[string]struct {
		mutate func(map[string]any)
		want   string
	}{
		"unavailable": {func(e map[string]any) { e["observation_state"] = "provider_unavailable" }, "observed_provider_result"},
		"low volume":  {func(e map[string]any) { e["sessions_28d"] = 12 }, "baseline_sample_size"},
		"stale":       {func(e map[string]any) { e["window_start"] = "2026-04-01"; e["window_end"] = "2026-04-28" }, "baseline_freshness"},
	} {
		t.Run(name, func(t *testing.T) {
			evidence := cloneEvidence(base)
			test.mutate(evidence)
			result := Build(Input{
				Type: "ga4_low_engagement", TargetURL: "https://example.com/pricing",
				RecommendedAction: "Improve the pricing page's proof and CTA hierarchy", Now: now, Evidence: mustJSON(t, evidence),
			})
			if result.State != StateNeedsEvidence || !slices.Contains(result.Missing, test.want) {
				t.Fatalf("result = %#v, want missing %q", result, test.want)
			}
		})
	}
}

func TestBuildGA4ConversionContract(t *testing.T) {
	result := Build(Input{
		Type: "ga4_conversion_gap", TargetURL: "https://example.com/signup", RecommendedAction: "Reduce signup friction",
		Now: time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC),
		Evidence: mustJSON(t, map[string]any{
			"source": "google_analytics_4", "observation_state": "observed",
			"key_events_28d": 7, "sessions_28d": 300,
			"window_start": "2026-06-01", "window_end": "2026-06-28",
		}),
	})
	if result.State != StateDecisionReady || result.Spec.PrimaryMetric != "ga4_key_events" {
		t.Fatalf("result = %#v", result)
	}
}

func TestBuildDecisionReadyAICitationSpecification(t *testing.T) {
	result := Build(Input{
		Type:              "geo_project_mentioned_without_citation",
		Query:             "best launch content workflow",
		RecommendedAction: "Add an evidence-backed answer block",
		ExpectedImpact:    "Increase answer-engine citations",
		Evidence: mustJSON(t, map[string]any{
			"source": "geo_observations", "observation_state": "observed",
			"project_citation_count": 0, "observed_at": "2026-07-11T23:59:00Z",
			"observation_id": "79bdff20-2df7-4e7d-86ab-0b79fc5d8a92",
		}),
	})
	if result.State != StateDecisionReady {
		t.Fatalf("result = %#v", result)
	}
	if result.Spec.PrimaryMetric != "ai_citation_count" || result.Spec.Baseline.Value != 0 {
		t.Fatalf("unexpected citation baseline: %#v", result.Spec.Baseline)
	}
}

func TestBuildDoesNotTreatProviderUnavailableAsZeroBaseline(t *testing.T) {
	result := Build(Input{
		Type:              "geo_project_mentioned_without_citation",
		Query:             "best launch content workflow",
		RecommendedAction: "Add an evidence-backed answer block",
		Evidence: mustJSON(t, map[string]any{
			"source": "geo_observations", "observation_state": "provider_unavailable",
			"project_citation_count": 0, "observed_at": "2026-07-11T23:59:00Z",
		}),
	})
	if result.State != StateNeedsEvidence {
		t.Fatalf("state = %q, want needs_evidence", result.State)
	}
	if !slices.Contains(result.Missing, "observed_provider_result") {
		t.Fatalf("missing = %#v", result.Missing)
	}
}

func TestBuildHoldsStaleBaselineForRefresh(t *testing.T) {
	result := Build(Input{
		Type: "gsc_low_ctr_query", Query: "best ai visibility platform",
		RecommendedAction: "Rewrite the search snippet", Now: time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC),
		Evidence: mustJSON(t, map[string]any{
			"source": "gsc_search_analytics", "ctr_28d": 0.01, "impressions_28d": 1200,
			"window_start": "2026-04-01", "window_end": "2026-04-28",
		}),
	})
	if result.State != StateNeedsEvidence || !slices.Contains(result.Missing, "baseline_freshness") {
		t.Fatalf("result = %#v", result)
	}
}

func TestBuildHoldsUnknownOrUnidentifiedWork(t *testing.T) {
	unknown := Build(Input{Type: "future_growth_family", Evidence: json.RawMessage(`{}`)})
	if unknown.State != StateNeedsSpecification || !slices.Contains(unknown.Missing, "supported_growth_family") {
		t.Fatalf("unknown = %#v", unknown)
	}

	newAsset := Build(Input{
		Type:  "comparison_page",
		Query: "alpha vs beta",
		Evidence: mustJSON(t, map[string]any{
			"source": "context", "window_start": "2026-07-01", "window_end": "2026-07-11",
			"baseline_value": 10,
		}),
	})
	if newAsset.State != StateNeedsSpecification || !slices.Contains(newAsset.Missing, "intended_slug_or_canonical") {
		t.Fatalf("new asset = %#v", newAsset)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func cloneEvidence(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
