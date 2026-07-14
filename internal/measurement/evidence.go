package measurement

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

type MetricContract struct {
	Metric                      string
	Direction                   string
	ThresholdKind               string
	ThresholdValue              float64
	ImmutableBaselineValue      *float64
	ImmutableBaselineSampleSize float64
	ImmutableBaselineWindowDays int
	ImmutableBaselineRows       int
	ImmutableBaselinePartial    bool
	MinimumAfterRows            int
	MinimumAfterSample          float64
	GuardrailThresholds         map[string]float64
	UseExplicitGuardrails       bool
	UseExplicitMinimumSample    bool
	ImmutableGuardrails         map[string]ImmutableMetricBaseline
}

type ImmutableMetricBaseline struct {
	Value      float64
	SampleSize float64
	Rows       int
	Partial    bool
}

type EvidenceEvaluation struct {
	Evaluation
	BaselineValue   float64         `json:"baseline_value"`
	AfterValue      float64         `json:"after_value"`
	SEOMetrics      json.RawMessage `json:"seo_metrics"`
	GA4Metrics      json.RawMessage `json:"ga4_metrics"`
	GEOMetrics      json.RawMessage `json:"geo_metrics"`
	SourceFreshness json.RawMessage `json:"source_freshness"`
}

type evidenceEnvelope struct {
	GSC struct {
		BaselineClicks      float64 `json:"gsc_baseline_clicks"`
		BaselineImpressions float64 `json:"gsc_baseline_impressions"`
		BaselinePosition    float64 `json:"gsc_baseline_position"`
		BaselineRows        int     `json:"gsc_baseline_rows"`
		BaselinePartial     bool    `json:"gsc_baseline_partial"`
		BaselineDataThrough string  `json:"gsc_baseline_data_through"`
		BaselineUpdatedAt   string  `json:"gsc_baseline_updated_at"`
		AfterClicks         float64 `json:"gsc_after_clicks"`
		AfterImpressions    float64 `json:"gsc_after_impressions"`
		AfterPosition       float64 `json:"gsc_after_position"`
		AfterRows           int     `json:"gsc_after_rows"`
		AfterPartial        bool    `json:"gsc_after_partial"`
		AfterDataThrough    string  `json:"gsc_after_data_through"`
		AfterUpdatedAt      string  `json:"gsc_after_updated_at"`
	} `json:"gsc"`
	GA4 struct {
		BaselineSessions        float64 `json:"ga4_baseline_sessions"`
		BaselineEngagedSessions float64 `json:"ga4_baseline_engaged_sessions"`
		BaselineKeyEvents       float64 `json:"ga4_baseline_key_events"`
		BaselineRows            int     `json:"ga4_baseline_rows"`
		BaselineDataThrough     string  `json:"ga4_baseline_data_through"`
		BaselineUpdatedAt       string  `json:"ga4_baseline_updated_at"`
		AfterSessions           float64 `json:"ga4_after_sessions"`
		AfterEngagedSessions    float64 `json:"ga4_after_engaged_sessions"`
		AfterKeyEvents          float64 `json:"ga4_after_key_events"`
		AfterRows               int     `json:"ga4_after_rows"`
		AfterDataThrough        string  `json:"ga4_after_data_through"`
		AfterUpdatedAt          string  `json:"ga4_after_updated_at"`
	} `json:"ga4"`
	GEO struct {
		BaselineCitations  float64 `json:"geo_baseline_citations"`
		BaselineBrandRate  float64 `json:"geo_baseline_brand_rate"`
		BaselineRows       int     `json:"geo_baseline_rows"`
		BaselineObservedAt string  `json:"geo_baseline_observed_at"`
		AfterCitations     float64 `json:"geo_after_citations"`
		AfterBrandRate     float64 `json:"geo_after_brand_rate"`
		AfterRows          int     `json:"geo_after_rows"`
		AfterObservedAt    string  `json:"geo_after_observed_at"`
	} `json:"geo"`
	Windows   map[string]any    `json:"windows"`
	Providers map[string]string `json:"providers"`
}

func EvaluateSourceEvidence(contract MetricContract, raw json.RawMessage, now time.Time) (EvidenceEvaluation, error) {
	var envelope evidenceEnvelope
	if len(raw) == 0 || !json.Valid(raw) {
		return EvidenceEvaluation{}, fmt.Errorf("measurement evidence must be valid JSON")
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return EvidenceEvaluation{}, err
	}
	baseline, after, guardrails, source, err := metricSamples(contract.Metric, envelope)
	if err != nil {
		return EvidenceEvaluation{}, err
	}
	if contract.ImmutableBaselineValue != nil {
		baseline.Value = *contract.ImmutableBaselineValue
		baseline.Available = true
		if contract.ImmutableBaselineRows > 0 {
			baseline.Rows = contract.ImmutableBaselineRows
		} else {
			baseline.Rows = max(1, baseline.Rows)
		}
		if contract.ImmutableBaselineSampleSize > 0 {
			baseline.SampleSize = contract.ImmutableBaselineSampleSize
		}
		baseline.Partial = contract.ImmutableBaselinePartial
	}
	baselineDays := evidenceWindowDays(envelope.Windows, "baseline_start", "baseline_end")
	afterDays := evidenceWindowDays(envelope.Windows, "after_start", "after_end")
	normalizedDays := baselineDays
	if contract.ImmutableBaselineWindowDays > 0 {
		normalizedDays = contract.ImmutableBaselineWindowDays
	}
	if normalizedDays > 0 && afterDays > 0 {
		scale := float64(normalizedDays) / float64(afterDays)
		if metricIsCumulativeCount(contract.Metric) {
			after.Value *= scale
		}
		for index := range guardrails {
			if guardrails[index].Name == "gsc_impressions" || guardrails[index].Name == "ga4_sessions" {
				guardrails[index].After.Value *= scale
			}
		}
	}
	minimumAfterRows := 1
	minimumAfterSample := float64(1)
	if (source == "gsc" || source == "ga4") && afterDays > 0 {
		minimumAfterRows = int(math.Ceil(float64(afterDays) * .70))
	}
	if source == "ga4" {
		minimumAfterSample = 30
	}
	if contract.UseExplicitMinimumSample {
		minimumAfterRows = max(1, contract.MinimumAfterRows)
		minimumAfterSample = math.Max(1, contract.MinimumAfterSample)
	} else {
		if contract.MinimumAfterRows > 0 {
			minimumAfterRows = contract.MinimumAfterRows
		}
		if contract.MinimumAfterSample > 0 {
			minimumAfterSample = contract.MinimumAfterSample
		}
	}
	if contract.UseExplicitGuardrails {
		filtered := guardrails[:0]
		for _, guardrail := range guardrails {
			if _, ok := contract.GuardrailThresholds[guardrail.Name]; ok {
				filtered = append(filtered, guardrail)
			}
		}
		guardrails = filtered
	}
	for index := range guardrails {
		if frozen, ok := contract.ImmutableGuardrails[guardrails[index].Name]; ok {
			guardrails[index].Baseline.Value = frozen.Value
			guardrails[index].Baseline.Available = true
			if frozen.Rows > 0 {
				guardrails[index].Baseline.Rows = frozen.Rows
			} else {
				guardrails[index].Baseline.Rows = max(1, guardrails[index].Baseline.Rows)
			}
			if frozen.SampleSize > 0 {
				guardrails[index].Baseline.SampleSize = frozen.SampleSize
			}
			guardrails[index].Baseline.Partial = frozen.Partial
		}
		if threshold, ok := contract.GuardrailThresholds[guardrails[index].Name]; ok && threshold > 0 {
			guardrails[index].MaxAdverseRelative = threshold
		}
	}
	freshnessReference := now
	if afterEnd, ok := envelope.Windows["after_end"].(string); ok {
		if parsed := parseMetricTime(afterEnd); !parsed.IsZero() && parsed.Before(now) {
			freshnessReference = parsed.Add(23*time.Hour + 59*time.Minute)
		}
	}
	evaluation := Evaluate(EvaluationInput{
		Metric: contract.Metric, Direction: contract.Direction,
		ThresholdKind: contract.ThresholdKind, ThresholdValue: contract.ThresholdValue,
		Baseline: baseline, After: after, Guardrails: guardrails, Now: freshnessReference,
		FreshnessTolerance: sourceFreshnessTolerance(source), MinimumAfterRows: minimumAfterRows, MinimumAfterSample: minimumAfterSample,
	})
	if evaluation.OutcomeLabel == OutcomeInsufficientData && providerExplicitlyUnavailable(source, envelope.Providers[source]) {
		evaluation.DataQualityState = QualityProviderUnavailable
		evaluation.OutcomeReason = fmt.Sprintf("The %s provider is unavailable, so CiteLoop cannot compute a reliable before/after outcome.", source)
		evaluation.Confounders = uniqueStrings(append(evaluation.Confounders, "Provider availability blocked the checkpoint evaluation."))
	}
	seoMetrics := marshalEvidenceMetrics(envelope.GSC, contract, evaluation, baseline, after, normalizedDays, minimumAfterRows, minimumAfterSample, envelope.Windows)
	ga4Metrics := marshalEvidenceMetrics(envelope.GA4, contract, evaluation, baseline, after, normalizedDays, minimumAfterRows, minimumAfterSample, envelope.Windows)
	geoMetrics := marshalEvidenceMetrics(envelope.GEO, contract, evaluation, baseline, after, normalizedDays, minimumAfterRows, minimumAfterSample, envelope.Windows)
	freshness, _ := json.Marshal(map[string]any{
		"gsc":             map[string]any{"baseline_data_through": envelope.GSC.BaselineDataThrough, "after_data_through": envelope.GSC.AfterDataThrough, "baseline_updated_at": envelope.GSC.BaselineUpdatedAt, "after_updated_at": envelope.GSC.AfterUpdatedAt, "baseline_rows": envelope.GSC.BaselineRows, "after_rows": envelope.GSC.AfterRows, "partial": envelope.GSC.BaselinePartial || envelope.GSC.AfterPartial},
		"ga4":             map[string]any{"baseline_data_through": envelope.GA4.BaselineDataThrough, "after_data_through": envelope.GA4.AfterDataThrough, "baseline_updated_at": envelope.GA4.BaselineUpdatedAt, "after_updated_at": envelope.GA4.AfterUpdatedAt, "baseline_rows": envelope.GA4.BaselineRows, "after_rows": envelope.GA4.AfterRows},
		"geo":             map[string]any{"baseline_observed_at": envelope.GEO.BaselineObservedAt, "after_observed_at": envelope.GEO.AfterObservedAt, "baseline_rows": envelope.GEO.BaselineRows, "after_rows": envelope.GEO.AfterRows},
		"provider_status": envelope.Providers,
	})
	return EvidenceEvaluation{
		Evaluation: evaluation, BaselineValue: baseline.Value, AfterValue: after.Value,
		SEOMetrics: seoMetrics, GA4Metrics: ga4Metrics, GEOMetrics: geoMetrics, SourceFreshness: freshness,
	}, nil
}

func metricSamples(metric string, envelope evidenceEnvelope) (MetricSample, MetricSample, []Guardrail, string, error) {
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "gsc_clicks":
		return gscSample(envelope.GSC.BaselineClicks, envelope.GSC.BaselineImpressions, envelope.GSC.BaselineRows, envelope.GSC.BaselinePartial, firstNonBlank(envelope.GSC.BaselineDataThrough, envelope.GSC.BaselineUpdatedAt)),
			gscSample(envelope.GSC.AfterClicks, envelope.GSC.AfterImpressions, envelope.GSC.AfterRows, envelope.GSC.AfterPartial, firstNonBlank(envelope.GSC.AfterDataThrough, envelope.GSC.AfterUpdatedAt)),
			[]Guardrail{gscImpressionGuardrail(envelope)}, "gsc", nil
	case "gsc_ctr":
		return gscSample(safeRatio(envelope.GSC.BaselineClicks, envelope.GSC.BaselineImpressions), envelope.GSC.BaselineImpressions, envelope.GSC.BaselineRows, envelope.GSC.BaselinePartial, firstNonBlank(envelope.GSC.BaselineDataThrough, envelope.GSC.BaselineUpdatedAt)),
			gscSample(safeRatio(envelope.GSC.AfterClicks, envelope.GSC.AfterImpressions), envelope.GSC.AfterImpressions, envelope.GSC.AfterRows, envelope.GSC.AfterPartial, firstNonBlank(envelope.GSC.AfterDataThrough, envelope.GSC.AfterUpdatedAt)),
			[]Guardrail{gscImpressionGuardrail(envelope)}, "gsc", nil
	case "gsc_impressions":
		return gscSample(envelope.GSC.BaselineImpressions, envelope.GSC.BaselineImpressions, envelope.GSC.BaselineRows, envelope.GSC.BaselinePartial, firstNonBlank(envelope.GSC.BaselineDataThrough, envelope.GSC.BaselineUpdatedAt)),
			gscSample(envelope.GSC.AfterImpressions, envelope.GSC.AfterImpressions, envelope.GSC.AfterRows, envelope.GSC.AfterPartial, firstNonBlank(envelope.GSC.AfterDataThrough, envelope.GSC.AfterUpdatedAt)),
			nil, "gsc", nil
	case "gsc_position":
		return gscSample(envelope.GSC.BaselinePosition, envelope.GSC.BaselineImpressions, envelope.GSC.BaselineRows, envelope.GSC.BaselinePartial, firstNonBlank(envelope.GSC.BaselineDataThrough, envelope.GSC.BaselineUpdatedAt)),
			gscSample(envelope.GSC.AfterPosition, envelope.GSC.AfterImpressions, envelope.GSC.AfterRows, envelope.GSC.AfterPartial, firstNonBlank(envelope.GSC.AfterDataThrough, envelope.GSC.AfterUpdatedAt)),
			[]Guardrail{gscImpressionGuardrail(envelope)}, "gsc", nil
	case "ga4_engagement_rate":
		return plainSample(safeRatio(envelope.GA4.BaselineEngagedSessions, envelope.GA4.BaselineSessions), envelope.GA4.BaselineSessions, envelope.GA4.BaselineRows, firstNonBlank(envelope.GA4.BaselineDataThrough, envelope.GA4.BaselineUpdatedAt)),
			plainSample(safeRatio(envelope.GA4.AfterEngagedSessions, envelope.GA4.AfterSessions), envelope.GA4.AfterSessions, envelope.GA4.AfterRows, firstNonBlank(envelope.GA4.AfterDataThrough, envelope.GA4.AfterUpdatedAt)),
			[]Guardrail{ga4SessionGuardrail(envelope)}, "ga4", nil
	case "ga4_key_events":
		return plainSample(envelope.GA4.BaselineKeyEvents, envelope.GA4.BaselineSessions, envelope.GA4.BaselineRows, firstNonBlank(envelope.GA4.BaselineDataThrough, envelope.GA4.BaselineUpdatedAt)),
			plainSample(envelope.GA4.AfterKeyEvents, envelope.GA4.AfterSessions, envelope.GA4.AfterRows, firstNonBlank(envelope.GA4.AfterDataThrough, envelope.GA4.AfterUpdatedAt)),
			[]Guardrail{ga4SessionGuardrail(envelope)}, "ga4", nil
	case "ga4_conversion_rate":
		return plainSample(safeRatio(envelope.GA4.BaselineKeyEvents, envelope.GA4.BaselineSessions), envelope.GA4.BaselineSessions, envelope.GA4.BaselineRows, firstNonBlank(envelope.GA4.BaselineDataThrough, envelope.GA4.BaselineUpdatedAt)),
			plainSample(safeRatio(envelope.GA4.AfterKeyEvents, envelope.GA4.AfterSessions), envelope.GA4.AfterSessions, envelope.GA4.AfterRows, firstNonBlank(envelope.GA4.AfterDataThrough, envelope.GA4.AfterUpdatedAt)),
			[]Guardrail{ga4SessionGuardrail(envelope)}, "ga4", nil
	case "ga4_sessions":
		return plainSample(envelope.GA4.BaselineSessions, envelope.GA4.BaselineSessions, envelope.GA4.BaselineRows, firstNonBlank(envelope.GA4.BaselineDataThrough, envelope.GA4.BaselineUpdatedAt)),
			plainSample(envelope.GA4.AfterSessions, envelope.GA4.AfterSessions, envelope.GA4.AfterRows, firstNonBlank(envelope.GA4.AfterDataThrough, envelope.GA4.AfterUpdatedAt)), nil, "ga4", nil
	case "ai_citation_count":
		return plainSample(envelope.GEO.BaselineCitations, float64(envelope.GEO.BaselineRows), envelope.GEO.BaselineRows, envelope.GEO.BaselineObservedAt),
			plainSample(envelope.GEO.AfterCitations, float64(envelope.GEO.AfterRows), envelope.GEO.AfterRows, envelope.GEO.AfterObservedAt),
			[]Guardrail{{Name: "ai_brand_mention_rate", Baseline: plainSample(envelope.GEO.BaselineBrandRate, float64(envelope.GEO.BaselineRows), envelope.GEO.BaselineRows, envelope.GEO.BaselineObservedAt), After: plainSample(envelope.GEO.AfterBrandRate, float64(envelope.GEO.AfterRows), envelope.GEO.AfterRows, envelope.GEO.AfterObservedAt), MaxAdverseRelative: .30}}, "geo", nil
	case "ai_brand_mention_rate":
		return plainSample(envelope.GEO.BaselineBrandRate, float64(envelope.GEO.BaselineRows), envelope.GEO.BaselineRows, envelope.GEO.BaselineObservedAt),
			plainSample(envelope.GEO.AfterBrandRate, float64(envelope.GEO.AfterRows), envelope.GEO.AfterRows, envelope.GEO.AfterObservedAt), nil, "geo", nil
	default:
		return MetricSample{}, MetricSample{}, nil, "", fmt.Errorf("unsupported primary metric %q", metric)
	}
}

func gscSample(value, sampleSize float64, rows int, partial bool, observedAt string) MetricSample {
	sample := plainSample(value, sampleSize, rows, observedAt)
	sample.Partial = partial
	return sample
}

func plainSample(value, sampleSize float64, rows int, observedAt string) MetricSample {
	return MetricSample{Value: value, SampleSize: sampleSize, Rows: rows, Available: rows > 0, ObservedThrough: parseMetricTime(observedAt)}
}

func gscImpressionGuardrail(envelope evidenceEnvelope) Guardrail {
	return Guardrail{Name: "gsc_impressions", Baseline: gscSample(envelope.GSC.BaselineImpressions, envelope.GSC.BaselineImpressions, envelope.GSC.BaselineRows, envelope.GSC.BaselinePartial, firstNonBlank(envelope.GSC.BaselineDataThrough, envelope.GSC.BaselineUpdatedAt)), After: gscSample(envelope.GSC.AfterImpressions, envelope.GSC.AfterImpressions, envelope.GSC.AfterRows, envelope.GSC.AfterPartial, firstNonBlank(envelope.GSC.AfterDataThrough, envelope.GSC.AfterUpdatedAt)), MaxAdverseRelative: .30}
}

func ga4SessionGuardrail(envelope evidenceEnvelope) Guardrail {
	return Guardrail{Name: "ga4_sessions", Baseline: plainSample(envelope.GA4.BaselineSessions, envelope.GA4.BaselineSessions, envelope.GA4.BaselineRows, firstNonBlank(envelope.GA4.BaselineDataThrough, envelope.GA4.BaselineUpdatedAt)), After: plainSample(envelope.GA4.AfterSessions, envelope.GA4.AfterSessions, envelope.GA4.AfterRows, firstNonBlank(envelope.GA4.AfterDataThrough, envelope.GA4.AfterUpdatedAt)), MaxAdverseRelative: .30}
}

func safeRatio(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

func parseMetricTime(value string) time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02", time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sourceFreshnessTolerance(source string) time.Duration {
	if source == "geo" {
		return 14 * 24 * time.Hour
	}
	return 5 * 24 * time.Hour
}

func providerExplicitlyUnavailable(source, status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return false
	}
	if source == "geo" {
		return status != "observed" && status != "ok" && status != "connected"
	}
	return status != "connected" && status != "backfilling" && status != "stale"
}

func metricIsCumulativeCount(metric string) bool {
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "gsc_clicks", "gsc_impressions", "ga4_key_events", "ga4_sessions":
		return true
	default:
		return false
	}
}

func evidenceWindowDays(windows map[string]any, startKey, endKey string) int {
	start, _ := windows[startKey].(string)
	end, _ := windows[endKey].(string)
	startTime := parseMetricTime(start)
	endTime := parseMetricTime(end)
	if startTime.IsZero() || endTime.IsZero() || endTime.Before(startTime) {
		return 0
	}
	return int(endTime.Sub(startTime).Hours()/24) + 1
}

func marshalEvidenceMetrics(source any, contract MetricContract, evaluation Evaluation, baseline, after MetricSample, normalizedDays, minimumAfterRows int, minimumAfterSample float64, windows map[string]any) json.RawMessage {
	provenance := "queried_source_window"
	if contract.ImmutableBaselineValue != nil {
		provenance = "bound_growth_spec"
	}
	raw, _ := json.Marshal(map[string]any{
		"source_window": source, "primary_metric": contract.Metric,
		"direction": contract.Direction, "decision_threshold": map[string]any{"kind": contract.ThresholdKind, "value": contract.ThresholdValue},
		"delta_absolute": evaluation.DeltaAbsolute, "delta_relative": evaluation.DeltaRelative,
		"guardrails": evaluation.GuardrailResults, "windows": windows,
		"evaluated_baseline": map[string]any{"value": baseline.Value, "sample_size": baseline.SampleSize, "rows": baseline.Rows, "partial": baseline.Partial, "provenance": provenance, "normalized_window_days": normalizedDays},
		"evaluated_after":    map[string]any{"value": after.Value, "sample_size": after.SampleSize, "rows": after.Rows, "provenance": "queried_after_window", "normalized_window_days": normalizedDays},
		"coverage_required":  map[string]any{"minimum_after_periods": minimumAfterRows, "minimum_after_sample": minimumAfterSample},
	})
	return raw
}
