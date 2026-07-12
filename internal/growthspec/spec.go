package growthspec

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	StateLegacy             = "legacy"
	StateNeedsSpecification = "needs_specification"
	StateNeedsEvidence      = "needs_evidence"
	StateDecisionReady      = "decision_ready"
	VersionV1               = "growth-opportunity-v1"
	PolicyVersionV1         = "growth-measurement-v1"
)

type Input struct {
	Type              string
	Query             string
	TargetURL         string
	RecommendedAction string
	ExpectedImpact    string
	Evidence          json.RawMessage
	Now               time.Time
}

type Result struct {
	State   string
	Version string
	Spec    Spec
	Missing []string
}

type Spec struct {
	SchemaVersion        string            `json:"schema_version"`
	Hypothesis           string            `json:"hypothesis"`
	Audience             []string          `json:"audience"`
	Baseline             Baseline          `json:"baseline"`
	PrimaryMetric        string            `json:"primary_metric"`
	ExpectedChange       ExpectedChange    `json:"expected_change"`
	MeasurementPolicy    MeasurementPolicy `json:"measurement_policy"`
	AttributionModel     string            `json:"attribution_model"`
	StopConditions       []string          `json:"stop_conditions"`
	ReconsiderConditions []string          `json:"reconsider_conditions"`
}

type Baseline struct {
	Source      string   `json:"source"`
	Metric      string   `json:"metric"`
	Value       float64  `json:"value"`
	WindowStart string   `json:"window_start"`
	WindowEnd   string   `json:"window_end"`
	SampleSize  float64  `json:"sample_size,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}

type ExpectedChange struct {
	Direction         string            `json:"direction"`
	DecisionThreshold DecisionThreshold `json:"decision_threshold"`
	RangeConfidence   string            `json:"range_confidence"`
}

type DecisionThreshold struct {
	Kind  string  `json:"kind"`
	Value float64 `json:"value"`
}

type MeasurementPolicy struct {
	PolicyVersion                  string `json:"policy_version"`
	EarlySignalOffsetDays          int    `json:"early_signal_offset_days"`
	PrimaryCheckpointOffsetDays    int    `json:"primary_checkpoint_offset_days"`
	FollowUpOffsetsDays            []int  `json:"follow_up_offsets_days"`
	MaxFollowUpAttempts            int    `json:"max_follow_up_attempts"`
	MaxMeasuringDurationDays       int    `json:"max_measuring_duration_days"`
	TerminalizationGracePeriodDays int    `json:"terminalization_grace_period_days"`
}

func (r Result) JSON() (json.RawMessage, error) {
	if r.Spec.SchemaVersion == "" {
		return json.RawMessage(`{}`), nil
	}
	return json.Marshal(r.Spec)
}

type familyContract struct {
	metric             string
	direction          string
	baselineKeys       []string
	sampleKeys         []string
	allowedSources     []string
	requiredStateKey   string
	requiredStateValue string
	thresholdKind      string
	thresholdValue     float64
	primaryDays        int
	attribution        string
	requiresNewAsset   bool
	minValue           float64
	maxValue           *float64
	minSample          float64
	maxBaselineAgeDays int
}

var familyContracts = map[string]familyContract{
	"low_ctr":                                gscContract("gsc_ctr", "increase", "ctr_28d", "impressions_28d", "relative", 0.10, 28),
	"low_ctr_snippet":                        gscContract("gsc_ctr", "increase", "ctr_28d", "impressions_28d", "relative", 0.10, 28),
	"gsc_low_ctr":                            gscContract("gsc_ctr", "increase", "ctr_28d", "impressions_28d", "relative", 0.10, 28),
	"gsc_low_ctr_query":                      gscContract("gsc_ctr", "increase", "ctr_28d", "impressions_28d", "relative", 0.10, 28),
	"gsc_query_gap":                          gscContract("gsc_clicks", "increase", "clicks_28d", "impressions_28d", "relative", 0.10, 56),
	"query_gap":                              gscContract("gsc_clicks", "increase", "clicks_28d", "impressions_28d", "relative", 0.10, 56),
	"striking_distance":                      gscContract("gsc_position", "decrease", "position_28d", "impressions_28d", "absolute", 1, 56),
	"gsc_striking_distance_query":            gscContract("gsc_position", "decrease", "position_28d", "impressions_28d", "absolute", 1, 56),
	"content_decay":                          gscContract("gsc_clicks", "increase", "current_clicks_28d", "current_impressions", "relative", 0.10, 56),
	"content_decay_refresh":                  gscContract("gsc_clicks", "increase", "current_clicks_28d", "current_impressions", "relative", 0.10, 56),
	"gsc_content_decay":                      gscContract("gsc_clicks", "increase", "current_clicks_28d", "current_impressions", "relative", 0.10, 56),
	"geo_project_mentioned_without_citation": geoContract(),
	"geo_competitor_cited_project_absent":    geoContract(),
	"ai_citation_gap":                        geoContract(),
	"weak_citation_surface":                  geoContract(),
	"thin_evidence_page":                     geoContract(),
	"citation_fact_expansion":                geoContract(),
	"gsc_query_cannibalization":              gscContract("gsc_clicks", "increase", "preferred_page_clicks_28d", "total_impressions", "relative", 0.10, 56),
	"ranking_cluster_opportunity":            gscContract("gsc_clicks", "increase", "clicks_28d", "impressions_28d", "relative", 0.10, 56),
	"internal_link_strategy":                 gscContract("gsc_clicks", "increase", "clicks_28d", "impressions_28d", "relative", 0.10, 56),
	"cold_start_context_plan":                newAssetContract(),
	"cold_start_competitive_gap":             newAssetContract(),
	"cold_start_evidence_page":               geoContract(),
	"comparison_page":                        newAssetContract(),
	"alternative_page":                       newAssetContract(),
	"missing_use_case":                       newAssetContract(),
	"ga4_low_engagement":                     ga4EngagementContract(),
	"ga4_high_traffic_low_engagement":        ga4EngagementContract(),
	"ga4_conversion_gap":                     ga4ConversionContract(),
	"ga4_conversion_friction":                ga4ConversionContract(),
}

func gscContract(metric, direction, baselineKey, sampleKey, thresholdKind string, threshold float64, primaryDays int) familyContract {
	contract := familyContract{
		metric: metric, direction: direction, baselineKeys: []string{baselineKey}, sampleKeys: []string{sampleKey},
		allowedSources: []string{"gsc", "gsc_search_analytics", "google_search_console"},
		thresholdKind:  thresholdKind, thresholdValue: threshold, primaryDays: primaryDays,
		attribution: "pre_post_with_query_and_page_guardrails", minValue: 0, minSample: 1, maxBaselineAgeDays: 45,
	}
	if metric == "gsc_ctr" {
		contract.maxValue = floatPointer(1)
	}
	if metric == "gsc_position" {
		contract.minValue = 0.000001
	}
	return contract
}

func geoContract() familyContract {
	return familyContract{
		metric: "ai_citation_count", direction: "increase", baselineKeys: []string{"project_citation_count"},
		allowedSources: []string{"geo_observations"}, requiredStateKey: "observation_state", requiredStateValue: "observed",
		thresholdKind: "absolute", thresholdValue: 1, primaryDays: 42,
		attribution: "pre_post_with_provider_and_prompt_guardrails", minValue: 0, minSample: 1, maxBaselineAgeDays: 30,
	}
}

func newAssetContract() familyContract {
	return familyContract{
		metric: "gsc_clicks", direction: "increase", baselineKeys: []string{"baseline_value"}, sampleKeys: []string{"baseline_sample_size"},
		allowedSources: []string{"context", "context_confirmation", "gsc", "gsc_search_analytics", "google_search_console"},
		thresholdKind:  "absolute", thresholdValue: 1, primaryDays: 56,
		attribution: "pre_post_with_new_asset_ramp", requiresNewAsset: true, minValue: 0, minSample: 1, maxBaselineAgeDays: 45,
	}
}

func ga4EngagementContract() familyContract {
	return familyContract{
		metric: "ga4_engagement_rate", direction: "increase",
		baselineKeys: []string{"engagement_rate_28d", "ga4_engagement_rate_28d"}, sampleKeys: []string{"sessions_28d"},
		allowedSources:   []string{"ga4", "google_analytics", "google_analytics_4"},
		requiredStateKey: "observation_state", requiredStateValue: "observed",
		thresholdKind: "relative", thresholdValue: 0.10, primaryDays: 28,
		attribution: "pre_post_with_channel_and_landing_page_guardrails", minValue: 0, maxValue: floatPointer(1), minSample: 30, maxBaselineAgeDays: 35,
	}
}

func ga4ConversionContract() familyContract {
	return familyContract{
		metric: "ga4_key_events", direction: "increase",
		baselineKeys: []string{"key_events_28d", "conversions_28d", "ga4_conversions_28d"}, sampleKeys: []string{"sessions_28d"},
		allowedSources:   []string{"ga4", "google_analytics", "google_analytics_4"},
		requiredStateKey: "observation_state", requiredStateValue: "observed",
		thresholdKind: "relative", thresholdValue: 0.10, primaryDays: 28,
		attribution: "pre_post_with_channel_and_landing_page_guardrails", minValue: 0, minSample: 30, maxBaselineAgeDays: 35,
	}
}

func Build(input Input) Result {
	family := normalize(input.Type)
	contract, ok := familyContracts[family]
	if !ok {
		return held(StateNeedsSpecification, "supported_growth_family")
	}
	evidence := map[string]any{}
	if len(input.Evidence) > 0 {
		_ = json.Unmarshal(input.Evidence, &evidence)
	}

	specMissing := make([]string, 0, 3)
	if strings.TrimSpace(input.RecommendedAction) == "" {
		specMissing = append(specMissing, "recommended_action")
	}
	if contract.requiresNewAsset && stringValue(evidence["intended_slug_or_canonical"]) == "" {
		specMissing = append(specMissing, "intended_slug_or_canonical")
	}
	if len(specMissing) > 0 {
		return held(StateNeedsSpecification, specMissing...)
	}

	evidenceMissing := make([]string, 0, 5)
	source := stringValue(evidence["source"])
	if source == "" || !containsNormalized(contract.allowedSources, source) {
		evidenceMissing = append(evidenceMissing, "baseline_source")
	}
	if contract.requiredStateKey != "" && normalize(stringValue(evidence[contract.requiredStateKey])) != normalize(contract.requiredStateValue) {
		evidenceMissing = append(evidenceMissing, "observed_provider_result")
	}
	value, hasValue := firstNumber(evidence, contract.baselineKeys)
	if !hasValue || value < contract.minValue || (contract.maxValue != nil && value > *contract.maxValue) {
		evidenceMissing = append(evidenceMissing, "baseline_value")
	}
	windowStart := stringValue(evidence["window_start"])
	windowEnd := stringValue(evidence["window_end"])
	if contract.requiredStateKey != "" {
		observedAt := stringValue(evidence["observed_at"])
		if windowStart == "" {
			windowStart = observedAt
		}
		if windowEnd == "" {
			windowEnd = observedAt
		}
	}
	if !validWindow(windowStart, windowEnd) {
		evidenceMissing = append(evidenceMissing, "baseline_window")
	} else if !freshWindow(windowEnd, input.Now, contract.maxBaselineAgeDays) {
		evidenceMissing = append(evidenceMissing, "baseline_freshness")
	}
	sampleSize := float64(0)
	if len(contract.sampleKeys) > 0 {
		if sample, ok := firstNumber(evidence, contract.sampleKeys); ok && sample >= contract.minSample {
			sampleSize = sample
		} else {
			evidenceMissing = append(evidenceMissing, "baseline_sample_size")
		}
	} else if contract.requiredStateKey != "" {
		sampleSize = 1
	}
	if len(evidenceMissing) > 0 {
		return held(StateNeedsEvidence, evidenceMissing...)
	}

	audience := audienceFor(input, evidence)
	if len(audience) == 0 {
		return held(StateNeedsSpecification, "audience")
	}
	primaryDays := contract.primaryDays
	policy := MeasurementPolicy{
		PolicyVersion: PolicyVersionV1, EarlySignalOffsetDays: max(7, primaryDays/2),
		PrimaryCheckpointOffsetDays: primaryDays,
		FollowUpOffsetsDays:         []int{primaryDays + 14, primaryDays + 28},
		MaxFollowUpAttempts:         2, MaxMeasuringDurationDays: primaryDays + 28,
		TerminalizationGracePeriodDays: 7,
	}
	ids := evidenceIDs(evidence)
	spec := Spec{
		SchemaVersion:        VersionV1,
		Hypothesis:           hypothesisFor(input, contract.metric, contract.direction, audience[0], evidence),
		Audience:             audience,
		Baseline:             Baseline{Source: source, Metric: contract.metric, Value: value, WindowStart: windowStart, WindowEnd: windowEnd, SampleSize: sampleSize, EvidenceIDs: ids},
		PrimaryMetric:        contract.metric,
		ExpectedChange:       ExpectedChange{Direction: contract.direction, DecisionThreshold: DecisionThreshold{Kind: contract.thresholdKind, Value: contract.thresholdValue}, RangeConfidence: "low"},
		MeasurementPolicy:    policy,
		AttributionModel:     contract.attribution,
		StopConditions:       []string{"stop if a guardrail metric materially worsens", "stop if the published or applied artifact is rolled back"},
		ReconsiderConditions: []string{"reconsider after the primary checkpoint if attribution is inconclusive", "reconsider when material source evidence changes"},
	}
	return Result{State: StateDecisionReady, Version: VersionV1, Spec: spec, Missing: []string{}}
}

func firstNumber(evidence map[string]any, keys []string) (float64, bool) {
	for _, key := range keys {
		if value, ok := numberValue(evidence[key]); ok {
			return value, true
		}
	}
	return 0, false
}

func containsNormalized(values []string, candidate string) bool {
	candidate = normalize(candidate)
	for _, value := range values {
		if normalize(value) == candidate {
			return true
		}
	}
	return false
}

func floatPointer(value float64) *float64 { return &value }

func held(state string, missing ...string) Result {
	missing = uniqueSorted(missing)
	return Result{State: state, Version: VersionV1, Spec: Spec{}, Missing: missing}
}

func hypothesisFor(input Input, metric, direction, audience string, evidence map[string]any) string {
	action := strings.TrimSpace(input.RecommendedAction)
	why := strings.TrimSpace(stringValue(evidence["why_now"]))
	if why == "" {
		why = strings.TrimSpace(input.ExpectedImpact)
	}
	if why == "" {
		why = "the observed baseline provides a measurable comparison point"
	}
	return fmt.Sprintf("If CiteLoop %s, then %s will %s for %s because %s", lowerFirst(action), metric, direction, audience, lowerFirst(why))
}

func audienceFor(input Input, evidence map[string]any) []string {
	for _, key := range []string{"audience", "target_audience", "target_personas"} {
		if values := stringSlice(evidence[key]); len(values) > 0 {
			return uniqueSorted(values)
		}
	}
	if query := strings.TrimSpace(input.Query); query != "" {
		return []string{"people searching for " + query}
	}
	if target := strings.TrimSpace(input.TargetURL); target != "" {
		return []string{"organic and answer-engine visitors to " + target}
	}
	return nil
}

func evidenceIDs(evidence map[string]any) []string {
	values := make([]string, 0, 3)
	for _, key := range []string{"observation_id", "run_id", "evidence_id"} {
		if value := stringValue(evidence[key]); value != "" {
			values = append(values, value)
		}
	}
	return uniqueSorted(values)
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func validWindow(start, end string) bool {
	startTime, ok := parseEvidenceTime(start)
	if !ok {
		return false
	}
	endTime, ok := parseEvidenceTime(end)
	return ok && !endTime.Before(startTime)
}

func freshWindow(end string, now time.Time, maxAgeDays int) bool {
	endTime, ok := parseEvidenceTime(end)
	if !ok || maxAgeDays <= 0 {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	endTime = endTime.UTC()
	now = now.UTC()
	return !endTime.After(now.Add(24*time.Hour)) && !endTime.Before(now.AddDate(0, 0, -maxAgeDays))
}

func parseEvidenceTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := stringValue(item); value != "" {
				out = append(out, value)
			}
		}
		return out
	case string:
		if value := strings.TrimSpace(typed); value != "" {
			return []string{value}
		}
	}
	return nil
}

func normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("-", "_", " ", "_", "/", "_").Replace(value)
	return strings.Trim(value, "_")
}

func lowerFirst(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	return strings.ToLower(value[:1]) + value[1:]
}

func uniqueSorted(values []string) []string {
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
	sort.Strings(out)
	return out
}
