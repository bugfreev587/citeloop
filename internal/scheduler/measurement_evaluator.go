package scheduler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/growthspec"
	"github.com/citeloop/citeloop/internal/measurement"
	"github.com/jackc/pgx/v5/pgtype"
)

type measurementEvidenceQuerier interface {
	GetGrowthMeasurementEvidence(context.Context, db.GetGrowthMeasurementEvidenceParams) (json.RawMessage, error)
}

func evaluateMeasurementCheckpoints(ctx context.Context, q measurementEvidenceQuerier, action db.ContentAction, opportunity db.SeoOpportunity, raw json.RawMessage, now time.Time) (json.RawMessage, int, string, error) {
	var window map[string]any
	if err := json.Unmarshal(raw, &window); err != nil {
		return raw, 0, "", err
	}
	contract := measurementContractForAction(opportunity, raw)
	checkpoints, _ := window["checkpoints"].([]any)
	evaluations := map[int]measurement.EvidenceEvaluation{}
	for _, item := range checkpoints {
		checkpoint, ok := item.(map[string]any)
		if !ok || strings.TrimSpace(stringFromAny(checkpoint["completed_at"])) != now.UTC().Format(time.RFC3339) {
			continue
		}
		if terminalEvaluationReached(evaluations, checkpoints) {
			checkpoint["status"] = "skipped"
			checkpoint["skip_reason"] = "decision_outcome_reached"
			delete(checkpoint, "completed_at")
			delete(checkpoint, "outcome")
			delete(checkpoint, "outcome_label")
			delete(checkpoint, "outcome_reason")
			continue
		}
		day := measurementCheckpointDay(checkpoint["day"])
		baselineStart, baselineEnd, afterStart, afterEnd := measurementEvidenceDates(opportunity, action, day, now)
		targetURL := firstNonEmptyString(valueOrEmpty(action.NormalizedTargetUrl), opportunity.NormalizedPageUrl)
		query := valueOrEmpty(opportunity.Query)
		articleID := action.DraftArticleID
		if !articleID.Valid {
			articleID = action.TargetArticleID
		}
		evidence, err := q.GetGrowthMeasurementEvidence(ctx, db.GetGrowthMeasurementEvidenceParams{
			ProjectID: action.ProjectID, TargetUrl: targetURL, ArticleID: articleID, Query: query,
			GeoEvidenceIds: measurementGeoEvidenceIDs(opportunity),
			BaselineStart:  pgtype.Date{Time: baselineStart, Valid: true}, BaselineEnd: pgtype.Date{Time: baselineEnd, Valid: true},
			AfterStart: pgtype.Date{Time: afterStart, Valid: true}, AfterEnd: pgtype.Date{Time: afterEnd, Valid: true},
		})
		if err != nil {
			return raw, 0, "", err
		}
		evaluation, err := measurement.EvaluateSourceEvidence(contract, evidence, now)
		if err != nil {
			return raw, 0, "", err
		}
		if strings.TrimSpace(stringFromAny(checkpoint["role"])) == string(measurement.RoleBaseline) {
			evaluation.OutcomeLabel = measurement.OutcomeInsufficientData
			evaluation.OutcomeReason = "Baseline evidence was captured; directional attribution starts at the early checkpoint."
			evaluation.AttributionConfidence = "low"
		}
		evaluations[day] = evaluation
	}
	workingRaw := mustJSON(window)
	windowRaw, remaining, terminalReason := applyMeasurementEvaluations(workingRaw, now, evaluations)
	return windowRaw, remaining, terminalReason, nil
}

func measurementContractForAction(opportunity db.SeoOpportunity, window json.RawMessage) measurement.MetricContract {
	var spec growthspec.Spec
	_ = json.Unmarshal(opportunity.GrowthSpec, &spec)
	metric := normalizeMeasurementMetric(spec.PrimaryMetric)
	if metric == "" {
		var windowData map[string]any
		_ = json.Unmarshal(window, &windowData)
		metric = normalizeMeasurementMetric(strings.TrimSpace(stringFromAny(windowData["primary_metric"])))
	}
	if metric == "" {
		metric = "gsc_clicks"
	}
	direction := strings.ToLower(strings.TrimSpace(spec.ExpectedChange.Direction))
	if direction != "decrease" {
		direction = "increase"
	}
	kind := strings.ToLower(strings.TrimSpace(spec.ExpectedChange.DecisionThreshold.Kind))
	if kind != "absolute" {
		kind = "relative"
	}
	threshold := spec.ExpectedChange.DecisionThreshold.Value
	if threshold <= 0 {
		threshold = .10
	}
	contract := measurement.MetricContract{Metric: metric, Direction: direction, ThresholdKind: kind, ThresholdValue: threshold}
	if spec.Baseline.Metric == spec.PrimaryMetric && spec.Baseline.WindowStart != "" && spec.Baseline.WindowEnd != "" {
		baseline := spec.Baseline.Value
		contract.ImmutableBaselineValue = &baseline
		contract.ImmutableBaselineSampleSize = spec.Baseline.SampleSize
		if start, ok := parseMeasurementDate(spec.Baseline.WindowStart); ok {
			if end, ok := parseMeasurementDate(spec.Baseline.WindowEnd); ok && !end.Before(start) {
				contract.ImmutableBaselineWindowDays = int(end.Sub(start).Hours()/24) + 1
			}
		}
	}
	return contract
}

func measurementEvidenceDates(opportunity db.SeoOpportunity, action db.ContentAction, checkpointDay int, now time.Time) (time.Time, time.Time, time.Time, time.Time) {
	start := now.UTC()
	if action.MeasuringStartedAt.Valid {
		start = dateOnly(action.MeasuringStartedAt.Time)
	} else if action.PublishedAt.Valid {
		start = dateOnly(action.PublishedAt.Time)
	}
	comparisonDays := max(1, checkpointDay)
	baselineStart := start.AddDate(0, 0, -comparisonDays)
	baselineEnd := start.AddDate(0, 0, -1)
	var spec growthspec.Spec
	if json.Unmarshal(opportunity.GrowthSpec, &spec) == nil {
		if parsed, ok := parseMeasurementDate(spec.Baseline.WindowStart); ok {
			baselineStart = parsed
		}
		if parsed, ok := parseMeasurementDate(spec.Baseline.WindowEnd); ok && parsed.Before(start) {
			baselineEnd = parsed
		}
	}
	afterOffset := max(0, checkpointDay-1)
	afterEnd := start.AddDate(0, 0, afterOffset)
	if afterEnd.After(dateOnly(now)) {
		afterEnd = dateOnly(now)
	}
	if afterEnd.Before(start) {
		afterEnd = start
	}
	return baselineStart, baselineEnd, start, afterEnd
}

func measurementGeoEvidenceIDs(opportunity db.SeoOpportunity) []string {
	var spec growthspec.Spec
	if json.Unmarshal(opportunity.GrowthSpec, &spec) != nil {
		return []string{}
	}
	return append([]string(nil), spec.Baseline.EvidenceIDs...)
}

func terminalEvaluationReached(evaluations map[int]measurement.EvidenceEvaluation, checkpoints []any) bool {
	for _, item := range checkpoints {
		checkpoint, ok := item.(map[string]any)
		if !ok {
			continue
		}
		day := measurementCheckpointDay(checkpoint["day"])
		evaluation, ok := evaluations[day]
		if !ok || evaluation.OutcomeLabel == measurement.OutcomeInsufficientData {
			continue
		}
		role := strings.TrimSpace(stringFromAny(checkpoint["role"]))
		if role == string(measurement.RolePrimary) || role == string(measurement.RoleFollowUp) {
			return true
		}
	}
	return false
}

func parseMeasurementDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02", time.RFC3339, time.RFC3339Nano} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return dateOnly(parsed), true
		}
	}
	return time.Time{}, false
}

func normalizeMeasurementMetric(metric string) string {
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "clicks", "gsc_clicks":
		return "gsc_clicks"
	case "ctr", "gsc_ctr":
		return "gsc_ctr"
	case "position", "rank", "gsc_position":
		return "gsc_position"
	case "engagement_rate", "ga4_engagement_rate":
		return "ga4_engagement_rate"
	case "conversions", "key_events", "ga4_key_events":
		return "ga4_key_events"
	case "citations", "citation_count", "ai_citation_count":
		return "ai_citation_count"
	default:
		return ""
	}
}

func applyMeasurementEvaluations(raw json.RawMessage, now time.Time, evaluations map[int]measurement.EvidenceEvaluation) (json.RawMessage, int, string) {
	window := map[string]any{}
	if len(raw) > 0 && json.Valid(raw) {
		_ = json.Unmarshal(raw, &window)
	}
	checkpoints, _ := window["checkpoints"].([]any)
	terminalReason := strings.TrimSpace(stringFromAny(window["terminal_reason"]))
	decisionReached := false
	latestDay := -1
	var latest measurement.EvidenceEvaluation
	for _, item := range checkpoints {
		checkpoint, ok := item.(map[string]any)
		if !ok || strings.TrimSpace(stringFromAny(checkpoint["completed_at"])) != now.UTC().Format(time.RFC3339) {
			continue
		}
		day := measurementCheckpointDay(checkpoint["day"])
		evaluation, ok := evaluations[day]
		if !ok {
			continue
		}
		checkpoint["outcome"] = evaluation.OutcomeLabel
		checkpoint["outcome_label"] = evaluation.OutcomeLabel
		checkpoint["outcome_reason"] = evaluation.OutcomeReason
		checkpoint["attribution_confidence"] = evaluation.AttributionConfidence
		checkpoint["data_quality_state"] = evaluation.DataQualityState
		checkpoint["source_freshness"] = jsonObject(evaluation.SourceFreshness)
		checkpoint["seo_metrics"] = jsonObject(evaluation.SEOMetrics)
		checkpoint["ga4_metrics"] = jsonObject(evaluation.GA4Metrics)
		checkpoint["geo_metrics"] = jsonObject(evaluation.GEOMetrics)
		checkpoint["confounders"] = evaluation.Confounders
		checkpoint["guardrail_results"] = evaluation.GuardrailResults
		role := strings.TrimSpace(stringFromAny(checkpoint["role"]))
		if (role == string(measurement.RolePrimary) || role == string(measurement.RoleFollowUp)) && evaluation.OutcomeLabel != measurement.OutcomeInsufficientData {
			decisionReached = true
		}
		if day >= latestDay {
			latestDay = day
			latest = evaluation
		}
	}
	if decisionReached {
		terminalReason = "decision_outcome_reached"
		for _, item := range checkpoints {
			checkpoint, ok := item.(map[string]any)
			if ok && strings.TrimSpace(stringFromAny(checkpoint["status"])) == "scheduled" {
				checkpoint["status"] = "skipped"
				checkpoint["skip_reason"] = terminalReason
			}
		}
	}
	remaining := 0
	for _, item := range checkpoints {
		checkpoint, ok := item.(map[string]any)
		if ok && strings.TrimSpace(stringFromAny(checkpoint["status"])) == "scheduled" {
			remaining++
		}
	}
	if latestDay >= 0 {
		window["latest_outcome"] = latest.OutcomeLabel
		window["outcome_label"] = latest.OutcomeLabel
		window["outcome_reason"] = latest.OutcomeReason
		window["attribution_confidence"] = latest.AttributionConfidence
		window["data_quality_state"] = latest.DataQualityState
		window["confounders"] = latest.Confounders
	}
	if terminalReason != "" {
		window["terminal_reason"] = terminalReason
	}
	if remaining == 0 {
		window["state"] = "completed"
	}
	window["checkpoints"] = checkpoints
	return mustJSON(window), remaining, terminalReason
}

func jsonObject(raw json.RawMessage) any {
	value := map[string]any{}
	if len(raw) > 0 && json.Valid(raw) {
		_ = json.Unmarshal(raw, &value)
	}
	return value
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	text, _ := value.(string)
	return text
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
