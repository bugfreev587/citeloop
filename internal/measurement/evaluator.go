package measurement

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	OutcomePositive         = "positive"
	OutcomeNegative         = "negative"
	OutcomeMixed            = "mixed"
	OutcomeInconclusive     = "inconclusive"
	OutcomeInsufficientData = "insufficient_data"

	QualityComplete            = "complete"
	QualityPartial             = "partial"
	QualityInsufficient        = "insufficient"
	QualityProviderUnavailable = "provider_unavailable"
	QualityStale               = "stale"
)

type MetricSample struct {
	Value           float64   `json:"value"`
	SampleSize      float64   `json:"sample_size"`
	Rows            int       `json:"rows"`
	Available       bool      `json:"available"`
	Partial         bool      `json:"partial"`
	ObservedThrough time.Time `json:"observed_through"`
}

type Guardrail struct {
	Name               string       `json:"name"`
	Baseline           MetricSample `json:"baseline"`
	After              MetricSample `json:"after"`
	MaxAdverseRelative float64      `json:"max_adverse_relative"`
}

type EvaluationInput struct {
	Metric             string
	Direction          string
	ThresholdKind      string
	ThresholdValue     float64
	Baseline           MetricSample
	After              MetricSample
	Guardrails         []Guardrail
	Now                time.Time
	FreshnessTolerance time.Duration
	MinimumAfterRows   int
	MinimumAfterSample float64
}

type Evaluation struct {
	OutcomeLabel          string         `json:"outcome_label"`
	OutcomeReason         string         `json:"outcome_reason"`
	AttributionConfidence string         `json:"attribution_confidence"`
	DataQualityState      string         `json:"data_quality_state"`
	DeltaAbsolute         *float64       `json:"delta_absolute,omitempty"`
	DeltaRelative         *float64       `json:"delta_relative,omitempty"`
	Confounders           []string       `json:"confounders"`
	GuardrailResults      map[string]any `json:"guardrail_results"`
}

func Evaluate(input EvaluationInput) Evaluation {
	quality, confounders := evaluationQuality(input)
	if quality == QualityInsufficient || quality == QualityStale {
		return Evaluation{
			OutcomeLabel: OutcomeInsufficientData, OutcomeReason: insufficientReason(quality, input.Metric),
			AttributionConfidence: "low", DataQualityState: quality,
			Confounders: confounders, GuardrailResults: map[string]any{},
		}
	}
	confounders = append(confounders, "Seasonality, competitor changes, concurrent releases, and crawl or analytics latency may influence the observed movement.")

	delta := input.After.Value - input.Baseline.Value
	oriented := delta
	if strings.EqualFold(strings.TrimSpace(input.Direction), "decrease") {
		oriented = -delta
	}
	relative, hasRelative := relativeDelta(oriented, input.Baseline.Value)
	label := classifyDelta(oriented, relative, hasRelative, input.ThresholdKind, input.ThresholdValue)
	guardrailResults := map[string]any{}
	guardrailSplit := false
	for _, guardrail := range input.Guardrails {
		if !guardrail.Baseline.Available || !guardrail.After.Available || guardrail.Baseline.Value == 0 {
			continue
		}
		change := (guardrail.After.Value - guardrail.Baseline.Value) / math.Abs(guardrail.Baseline.Value)
		limit := guardrail.MaxAdverseRelative
		if limit <= 0 {
			limit = .30
		}
		adverse := change <= -limit
		guardrailResults[guardrail.Name] = map[string]any{
			"baseline": guardrail.Baseline.Value, "after": guardrail.After.Value,
			"relative_change": change, "adverse": adverse,
		}
		if adverse {
			guardrailSplit = true
			confounders = append(confounders, fmt.Sprintf("%s fell %.1f%% and crossed its %.1f%% guardrail", guardrail.Name, math.Abs(change)*100, limit*100))
		}
	}
	if guardrailSplit && label == OutcomePositive {
		label = OutcomeMixed
	}

	confidence := "medium"
	if quality == QualityPartial {
		confidence = "low"
	} else if input.Baseline.Rows >= 7 && input.After.Rows >= 7 && input.Baseline.SampleSize >= 30 && input.After.SampleSize >= 30 {
		confidence = "high"
	}
	abs := delta
	result := Evaluation{
		OutcomeLabel: label, OutcomeReason: outcomeReason(label, input.Metric, input.Baseline.Value, input.After.Value),
		AttributionConfidence: confidence, DataQualityState: quality, DeltaAbsolute: &abs,
		Confounders: uniqueStrings(confounders), GuardrailResults: guardrailResults,
	}
	if hasRelative {
		rel := relative
		if strings.EqualFold(strings.TrimSpace(input.Direction), "decrease") {
			rel = -relative
		}
		result.DeltaRelative = &rel
	}
	return result
}

func evaluationQuality(input EvaluationInput) (string, []string) {
	confounders := []string{}
	if !input.Baseline.Available || !input.After.Available || input.Baseline.Rows <= 0 || input.After.Rows <= 0 || input.Baseline.SampleSize <= 0 || input.After.SampleSize <= 0 {
		return QualityInsufficient, []string{"The baseline or after window has no comparable observations."}
	}
	if input.MinimumAfterRows > 0 && input.After.Rows < input.MinimumAfterRows {
		return QualityInsufficient, []string{fmt.Sprintf("The after window covers %d source periods; at least %d are required.", input.After.Rows, input.MinimumAfterRows)}
	}
	if input.MinimumAfterSample > 0 && input.After.SampleSize < input.MinimumAfterSample {
		return QualityInsufficient, []string{fmt.Sprintf("The after window sample is %.4g; at least %.4g is required.", input.After.SampleSize, input.MinimumAfterSample)}
	}
	tolerance := input.FreshnessTolerance
	if tolerance <= 0 {
		tolerance = 5 * 24 * time.Hour
	}
	now := input.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if input.After.ObservedThrough.IsZero() || input.After.ObservedThrough.UTC().Before(now.Add(-tolerance)) {
		return QualityStale, []string{"The newest source observation is outside the allowed freshness window."}
	}
	if input.Baseline.Partial || input.After.Partial {
		confounders = append(confounders, "The provider reports partial dimension coverage for at least one comparison window.")
		return QualityPartial, confounders
	}
	return QualityComplete, confounders
}

func classifyDelta(oriented, relative float64, hasRelative bool, kind string, threshold float64) string {
	if threshold < 0 {
		threshold = math.Abs(threshold)
	}
	value := oriented
	if strings.EqualFold(strings.TrimSpace(kind), "relative") {
		if !hasRelative {
			return OutcomeInconclusive
		}
		value = relative
	}
	if value >= threshold {
		return OutcomePositive
	}
	if value <= -threshold {
		return OutcomeNegative
	}
	return OutcomeInconclusive
}

func relativeDelta(oriented, baseline float64) (float64, bool) {
	if baseline == 0 {
		return 0, false
	}
	return oriented / math.Abs(baseline), true
}

func insufficientReason(quality, metric string) string {
	if quality == QualityStale {
		return fmt.Sprintf("The %s observations are stale, so CiteLoop cannot make a reliable before/after attribution.", metric)
	}
	return fmt.Sprintf("The %s baseline or after window lacks enough comparable observations for attribution.", metric)
}

func outcomeReason(label, metric string, baseline, after float64) string {
	switch label {
	case OutcomePositive:
		return fmt.Sprintf("%s improved beyond the decision threshold (%.4g → %.4g).", metric, baseline, after)
	case OutcomeNegative:
		return fmt.Sprintf("%s moved adversely beyond the decision threshold (%.4g → %.4g).", metric, baseline, after)
	case OutcomeMixed:
		return fmt.Sprintf("%s improved, but a guardrail moved adversely (%.4g → %.4g).", metric, baseline, after)
	default:
		return fmt.Sprintf("%s changed from %.4g to %.4g without crossing the decision threshold.", metric, baseline, after)
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
