package sitefix

import (
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"time"
)

const SiteFixClassifierVersionV1 = "site-fix-classifier-v1"

const (
	SiteFixBaselineCaptureMaxAge = 7 * 24 * time.Hour
	SiteFixBaselineEndMaxAge     = 10 * 24 * time.Hour
	SiteFixBaselineWindowMaxAge  = 90 * 24 * time.Hour
)

type MeasurementClassificationInput struct {
	ReferenceTime    time.Time
	TargetURLs       json.RawMessage
	FindingIssueType string
	TargetSurface    string
	FindingEvidence  json.RawMessage
	ProposedFix      json.RawMessage
	AcceptanceTests  json.RawMessage
}

type SiteFixMeasurementPlan struct {
	TargetURL           string
	NormalizedTargetURL string
	TargetQuery         *string
	TargetIdentity      json.RawMessage
	BaselineWindow      json.RawMessage
	BaselineSnapshot    json.RawMessage
	BaselineProvenance  json.RawMessage
	PolicySnapshot      json.RawMessage
}

type SiteFixMeasurementClassification struct {
	FixType                   string
	ImpactMode                string
	MeasurementPolicy         string
	ClassifierVersion         string
	DecisionOrigin            string
	DecisionConfidence        string
	GrowthHypothesis          *string
	PrimaryMetric             *string
	SecondaryMetrics          json.RawMessage
	MeasurementPolicyVersion  *string
	MeasurementPolicySnapshot json.RawMessage
	Plan                      *SiteFixMeasurementPlan
	ValidationError           string
}

type measurementPlanDocument struct {
	GrowthHypothesis   string          `json:"growth_hypothesis"`
	PrimaryMetric      string          `json:"primary_metric"`
	SecondaryMetrics   []string        `json:"secondary_metrics"`
	TargetQuery        string          `json:"target_query"`
	TargetIdentity     json.RawMessage `json:"target_identity"`
	BaselineWindow     json.RawMessage `json:"baseline_window"`
	BaselineSnapshot   json.RawMessage `json:"baseline_snapshot"`
	BaselineProvenance json.RawMessage `json:"baseline_provenance"`
	PolicySnapshot     json.RawMessage `json:"policy_snapshot"`
}

type finiteMeasurementPolicy struct {
	PolicyVersion                  string                `json:"policy_version"`
	EarlySignalOffsetDays          int                   `json:"early_signal_offset_days"`
	PrimaryCheckpointOffsetDays    int                   `json:"primary_checkpoint_offset_days"`
	FollowUpOffsetsDays            []int                 `json:"follow_up_offsets_days"`
	MaxFollowUpAttempts            int                   `json:"max_follow_up_attempts"`
	MaxMeasuringDurationDays       int                   `json:"max_measuring_duration_days"`
	MinimumSample                  finiteMinimumSample   `json:"minimum_sample"`
	MetricThresholds               finiteMetricThreshold `json:"metric_thresholds"`
	Guardrails                     []finiteGuardrail     `json:"guardrails"`
	RequiredDataSources            []string              `json:"required_data_sources"`
	TerminalizationGracePeriodDays int                   `json:"terminalization_grace_period_days"`
}

type finiteMinimumSample struct {
	MinimumAfterPeriods *int     `json:"minimum_after_periods"`
	MinimumAfterSample  *float64 `json:"minimum_after_sample"`
}

type finiteMetricThreshold struct {
	Direction string  `json:"direction"`
	Kind      string  `json:"kind"`
	Value     float64 `json:"value"`
}

type finiteGuardrail struct {
	Metric             string  `json:"metric"`
	MaxAdverseRelative float64 `json:"max_adverse_relative"`
}

type baselineWindowDocument struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type baselineProvenanceDocument struct {
	Source     string `json:"source"`
	CapturedAt string `json:"captured_at"`
}

type classificationRule struct {
	ImpactMode string
	Policy     string
}

var siteFixClassificationRules = map[string]classificationRule{
	"title_readability":                    {ImpactMode: "presentation_only", Policy: "verification_only"},
	"metadata_format":                      {ImpactMode: "presentation_only", Policy: "verification_only"},
	"canonical_repair":                     {ImpactMode: "technical_reliability", Policy: "verification_only"},
	"robots_repair":                        {ImpactMode: "technical_reliability", Policy: "verification_only"},
	"sitemap_repair":                       {ImpactMode: "technical_reliability", Policy: "verification_only"},
	"redirect_or_http_repair":              {ImpactMode: "technical_reliability", Policy: "verification_only"},
	"schema_validity_repair":               {ImpactMode: "technical_reliability", Policy: "verification_only"},
	"content_typo_or_clarity":              {ImpactMode: "presentation_only", Policy: "verification_only"},
	"technical_fix":                        {ImpactMode: "technical_reliability", Policy: "verification_only"},
	"metadata_ctr_optimization":            {ImpactMode: "conversion_or_ctr", Policy: "measurement_required"},
	"search_title_keyword_optimization":    {ImpactMode: "search_visibility", Policy: "measurement_required"},
	"internal_link_authority_optimization": {ImpactMode: "search_visibility", Policy: "measurement_required"},
	"schema_entity_optimization":           {ImpactMode: "geo_visibility", Policy: "measurement_required"},
	"geo_entity_clarity":                   {ImpactMode: "geo_visibility", Policy: "measurement_required"},
	"geo_citation_optimization":            {ImpactMode: "geo_visibility", Policy: "measurement_required"},
	"content_rewrite_for_search":           {ImpactMode: "content_demand", Policy: "measurement_required"},
	"content_demand_expansion":             {ImpactMode: "content_demand", Policy: "measurement_required"},
	"external_distribution":                {ImpactMode: "content_demand", Policy: "measurement_required"},
	"conversion_or_cta_optimization":       {ImpactMode: "conversion_or_ctr", Policy: "measurement_required"},
	"internal_link_patch":                  {ImpactMode: "search_visibility", Policy: "measurement_optional"},
	"schema_patch":                         {ImpactMode: "search_visibility", Policy: "measurement_optional"},
	"metadata_rewrite":                     {ImpactMode: "search_visibility", Policy: "measurement_optional"},
	"geo_content_clarity":                  {ImpactMode: "geo_visibility", Policy: "measurement_optional"},
	"unknown":                              {ImpactMode: "unclassified", Policy: "verification_only"},
}

var issueTypeRules = map[string]string{
	"schema_gap":                           "schema_patch",
	"json_ld_missing":                      "schema_patch",
	"schema_missing":                       "schema_patch",
	"metadata_readability":                 "title_readability",
	"title_missing":                        "metadata_format",
	"missing_title":                        "metadata_format",
	"title_duplicate":                      "title_readability",
	"duplicate_title":                      "title_readability",
	"title_too_long":                       "title_readability",
	"title_invalid":                        "title_readability",
	"metadata_title":                       "title_readability",
	"meta_description_missing":             "metadata_format",
	"metadata_description":                 "metadata_format",
	"duplicate_metadata_template":          "metadata_format",
	"canonical_missing":                    "canonical_repair",
	"canonical_mismatch":                   "canonical_repair",
	"canonical_invalid":                    "canonical_repair",
	"canonical_multiple":                   "canonical_repair",
	"robots_blocked":                       "robots_repair",
	"robots_conflict":                      "robots_repair",
	"noindex":                              "robots_repair",
	"noindex_conflict":                     "robots_repair",
	"geo_crawler_access_blocked":           "robots_repair",
	"important_page_missing_from_sitemap":  "sitemap_repair",
	"sitemap_missing":                      "sitemap_repair",
	"redirect_chain":                       "redirect_or_http_repair",
	"redirect_loop":                        "redirect_or_http_repair",
	"broken_url":                           "redirect_or_http_repair",
	"soft_404":                             "redirect_or_http_repair",
	"structured_data_missing":              "schema_patch",
	"structured_data_invalid":              "schema_validity_repair",
	"unsafe_mdx_detected":                  "schema_validity_repair",
	"internal_link_gap":                    "internal_link_patch",
	"zero_internal_links":                  "internal_link_patch",
	"broken_internal_link":                 "internal_link_patch",
	"orphan_page":                          "internal_link_patch",
	"sitemap_update":                       "sitemap_repair",
	"h1_missing":                           "content_typo_or_clarity",
	"supported_fact_extractability":        "geo_content_clarity",
	"citation_readiness_structure":         "geo_content_clarity",
	"source_association":                   "geo_content_clarity",
	"entity_naming_consistency":            "geo_content_clarity",
	"ga4_missing":                          "technical_fix",
	"tracking_missing":                     "technical_fix",
	"measurement_readiness":                "technical_fix",
	"security_or_config_repair":            "technical_fix",
	"metadata_ctr_optimization":            "metadata_ctr_optimization",
	"search_title_keyword_optimization":    "search_title_keyword_optimization",
	"internal_link_authority_optimization": "internal_link_authority_optimization",
	"schema_entity_optimization":           "schema_entity_optimization",
	"geo_entity_clarity":                   "geo_entity_clarity",
	"geo_citation_optimization":            "geo_citation_optimization",
	"content_rewrite_for_search":           "content_rewrite_for_search",
	"content_demand_expansion":             "content_demand_expansion",
	"external_distribution":                "external_distribution",
	"conversion_or_cta_optimization":       "conversion_or_cta_optimization",
}

var mutationFieldRules = map[string]string{
	"title":                   "title_readability",
	"metadata_title":          "title_readability",
	"meta_description":        "metadata_format",
	"metadata_description":    "metadata_format",
	"canonical":               "canonical_repair",
	"robots":                  "robots_repair",
	"sitemap":                 "sitemap_repair",
	"sitemap_entry":           "sitemap_repair",
	"redirect":                "redirect_or_http_repair",
	"http_status":             "redirect_or_http_repair",
	"http_response":           "redirect_or_http_repair",
	"schema":                  "schema_patch",
	"jsonld":                  "schema_patch",
	"schema_entity":           "schema_entity_optimization",
	"internal_link":           "internal_link_patch",
	"internal_link_authority": "internal_link_authority_optimization",
	"content_clarity":         "content_typo_or_clarity",
	"h1":                      "content_typo_or_clarity",
	"unsafe_output":           "technical_fix",
	"answer_block":            "geo_content_clarity",
	"source_association":      "geo_content_clarity",
	"entity_name":             "geo_content_clarity",
	"tracking":                "technical_fix",
}

var targetSurfaceRules = map[string]string{
	"schema.jsonld":               "schema_patch",
	"metadata.title":              "title_readability",
	"metadata.description":        "metadata_format",
	"content.heading":             "content_typo_or_clarity",
	"url.canonical":               "canonical_repair",
	"indexability.robots":         "robots_repair",
	"indexability.ai_crawler":     "robots_repair",
	"availability.http":           "redirect_or_http_repair",
	"links.internal":              "internal_link_patch",
	"discovery.sitemap":           "sitemap_repair",
	"rendering.template":          "technical_fix",
	"content.evidence":            "geo_content_clarity",
	"content.entity":              "geo_content_clarity",
	"measurement.instrumentation": "technical_fix",
}

func ClassifySiteFixMeasurement(input MeasurementClassificationInput) SiteFixMeasurementClassification {
	base := func(fixType, origin, confidence string) SiteFixMeasurementClassification {
		rule := siteFixClassificationRules[fixType]
		return SiteFixMeasurementClassification{
			FixType: fixType, ImpactMode: rule.ImpactMode, MeasurementPolicy: rule.Policy,
			ClassifierVersion: SiteFixClassifierVersionV1, DecisionOrigin: origin,
			DecisionConfidence: confidence, SecondaryMetrics: json.RawMessage(`[]`),
		}
	}

	if override, present := rawField(input.FindingEvidence, "site_fix_policy_override"); present && !isJSONNull(override) {
		if nonEmptyJSONObject(override) {
			if classification, valid := classifyExplicitOverride(input, override); valid {
				return classification
			}
		}
		invalid := base("unknown", "user_override", "low")
		invalid.ValidationError = "site_fix_policy_override failed policy validation"
		return invalid
	}

	planRaw, _ := objectField(input.ProposedFix, "measurement_plan")
	if len(planRaw) == 0 {
		planRaw, _ = objectField(input.FindingEvidence, "measurement_plan")
	}

	if fixType, ok := stringField(input.ProposedFix, "fix_type"); ok && isKnownFixType(fixType) {
		fixType = canonicalFixType(fixType)
		return attachReadyPlan(base(fixType, "system_rule", "high"), input, planRaw)
	}
	if fixType, ok := issueTypeRules[normalizeClassifierToken(input.FindingIssueType)]; ok {
		return attachReadyPlan(base(fixType, "system_rule", "high"), input, planRaw)
	}
	// Structured fallback tie-break is deliberately safety-first and stable:
	// acceptance contract, then mutation field, then candidate change surface.
	if fixType, ok := fixTypeFromAcceptanceTests(input.AcceptanceTests); ok {
		return attachReadyPlan(base(fixType, "system_rule", "medium"), input, planRaw)
	}
	if fixType, ok := fixTypeFromMutations(input.ProposedFix); ok {
		return attachReadyPlan(base(fixType, "system_rule", "medium"), input, planRaw)
	}
	if fixType, ok := targetSurfaceRules[normalizeClassifierToken(input.TargetSurface)]; ok {
		return attachReadyPlan(base(fixType, "system_rule", "medium"), input, planRaw)
	}
	return base("unknown", "system_rule", "low")
}

func classifyExplicitOverride(input MeasurementClassificationInput, raw json.RawMessage) (SiteFixMeasurementClassification, bool) {
	fixType, ok := stringField(raw, "fix_type")
	if !ok || !isKnownFixType(fixType) || fixType == "unknown" {
		return SiteFixMeasurementClassification{}, false
	}
	fixType = canonicalFixType(fixType)
	rule := siteFixClassificationRules[fixType]
	impact, impactOK := stringField(raw, "impact_mode")
	policy, policyOK := stringField(raw, "measurement_policy")
	if !impactOK || impact != rule.ImpactMode || !policyOK || !validOverridePolicy(rule.Policy, policy) {
		return SiteFixMeasurementClassification{}, false
	}
	classification := SiteFixMeasurementClassification{
		FixType: fixType, ImpactMode: impact, MeasurementPolicy: policy,
		ClassifierVersion: SiteFixClassifierVersionV1, DecisionOrigin: "user_override",
		DecisionConfidence: "high", SecondaryMetrics: json.RawMessage(`[]`),
	}
	planRaw, _ := objectField(raw, "measurement_plan")
	if policy == "measurement_required" {
		classification = attachReadyPlan(classification, input, planRaw)
		if classification.MeasurementPolicy != "measurement_required" {
			return SiteFixMeasurementClassification{}, false
		}
		return classification, true
	}
	if len(planRaw) > 0 {
		classification = attachReadyPlan(classification, input, planRaw)
		classification.MeasurementPolicy = policy
	}
	return classification, true
}

func validOverridePolicy(defaultPolicy, requested string) bool {
	switch defaultPolicy {
	case "verification_only":
		return requested == "verification_only"
	case "measurement_optional":
		return requested == "verification_only" || requested == "measurement_optional" || requested == "measurement_required"
	case "measurement_required":
		return requested == "verification_only" || requested == "measurement_required"
	default:
		return false
	}
}

func attachReadyPlan(classification SiteFixMeasurementClassification, input MeasurementClassificationInput, raw json.RawMessage) SiteFixMeasurementClassification {
	desiredPolicy := classification.MeasurementPolicy
	if desiredPolicy == "verification_only" || len(raw) == 0 {
		if desiredPolicy == "measurement_required" {
			classification.MeasurementPolicy = "verification_only"
		}
		return classification
	}
	document, plan, policy, ok := validateMeasurementPlan(input.ReferenceTime, input.TargetURLs, classification.ImpactMode, raw)
	if !ok {
		if desiredPolicy == "measurement_required" {
			classification.MeasurementPolicy = "verification_only"
		}
		return classification
	}
	hypothesis := strings.TrimSpace(document.GrowthHypothesis)
	metric := document.PrimaryMetric
	version := policy.PolicyVersion
	secondary, _ := json.Marshal(document.SecondaryMetrics)
	classification.GrowthHypothesis = &hypothesis
	classification.PrimaryMetric = &metric
	classification.SecondaryMetrics = secondary
	classification.MeasurementPolicyVersion = &version
	classification.MeasurementPolicySnapshot = append(json.RawMessage(nil), document.PolicySnapshot...)
	classification.Plan = plan
	return classification
}

func validateMeasurementPlan(referenceTime time.Time, targetsRaw json.RawMessage, impactMode string, raw json.RawMessage) (measurementPlanDocument, *SiteFixMeasurementPlan, finiteMeasurementPolicy, bool) {
	var document measurementPlanDocument
	if json.Unmarshal(raw, &document) != nil || strings.TrimSpace(document.GrowthHypothesis) == "" {
		return document, nil, finiteMeasurementPolicy{}, false
	}
	metric := document.PrimaryMetric
	if metric == "" || metric != normalizeClassifierToken(metric) || !supportedMetric(metric) || referenceTime.IsZero() {
		return document, nil, finiteMeasurementPolicy{}, false
	}
	var targets []string
	if json.Unmarshal(targetsRaw, &targets) != nil || len(targets) != 1 {
		return document, nil, finiteMeasurementPolicy{}, false
	}
	targetURL, ok := normalizedMeasurementTargetURL(targets[0])
	if !ok {
		return document, nil, finiteMeasurementPolicy{}, false
	}
	var policy finiteMeasurementPolicy
	if json.Unmarshal(document.PolicySnapshot, &policy) != nil || !finitePolicyShape(document.PolicySnapshot) || !finitePolicyValid(policy) || !metricSourcePairSupported(metric, policy.RequiredDataSources) {
		return document, nil, finiteMeasurementPolicy{}, false
	}
	if metricRequiresQuery(metric) && strings.TrimSpace(document.TargetQuery) == "" {
		return document, nil, finiteMeasurementPolicy{}, false
	}
	if (impactMode == "geo_visibility" || metricRequiresEntity(metric)) && !nonEmptyJSONObject(document.TargetIdentity) {
		return document, nil, finiteMeasurementPolicy{}, false
	}
	metrics := []string{metric}
	seenMetrics := map[string]bool{metric: true}
	for _, secondary := range document.SecondaryMetrics {
		if secondary == "" || secondary != normalizeClassifierToken(secondary) || !supportedMetric(secondary) || seenMetrics[secondary] || !metricSourcePairSupported(secondary, policy.RequiredDataSources) {
			return document, nil, finiteMeasurementPolicy{}, false
		}
		seenMetrics[secondary] = true
		metrics = append(metrics, secondary)
	}
	for _, guardrail := range policy.Guardrails {
		if guardrail.Metric == "" || guardrail.Metric != normalizeClassifierToken(guardrail.Metric) || !supportedMetric(guardrail.Metric) || !metricSourcePairSupported(guardrail.Metric, policy.RequiredDataSources) {
			return document, nil, finiteMeasurementPolicy{}, false
		}
		if !seenMetrics[guardrail.Metric] {
			seenMetrics[guardrail.Metric] = true
			metrics = append(metrics, guardrail.Metric)
		}
	}
	if !baselineReady(document, referenceTime, metrics, policy.RequiredDataSources) {
		return document, nil, finiteMeasurementPolicy{}, false
	}
	var query *string
	if value := strings.TrimSpace(document.TargetQuery); value != "" {
		query = &value
	}
	identity := document.TargetIdentity
	if len(identity) == 0 {
		identity = json.RawMessage(`{}`)
	}
	return document, &SiteFixMeasurementPlan{
		TargetURL: targetURL, NormalizedTargetURL: targetURL, TargetQuery: query,
		TargetIdentity: identity, BaselineWindow: document.BaselineWindow,
		BaselineSnapshot: document.BaselineSnapshot, BaselineProvenance: document.BaselineProvenance,
		PolicySnapshot: document.PolicySnapshot,
	}, policy, true
}

func finitePolicyValid(policy finiteMeasurementPolicy) bool {
	if policy.PolicyVersion != "site-fix-growth-v1" || policy.EarlySignalOffsetDays < 1 || policy.EarlySignalOffsetDays > 365 ||
		policy.PrimaryCheckpointOffsetDays <= policy.EarlySignalOffsetDays || policy.PrimaryCheckpointOffsetDays > 365 ||
		policy.MaxMeasuringDurationDays < policy.PrimaryCheckpointOffsetDays || policy.MaxMeasuringDurationDays > 365 ||
		policy.MaxFollowUpAttempts < 0 || policy.MaxFollowUpAttempts > 4 || len(policy.FollowUpOffsetsDays) > policy.MaxFollowUpAttempts ||
		policy.TerminalizationGracePeriodDays < 0 || policy.TerminalizationGracePeriodDays > 30 ||
		len(policy.RequiredDataSources) != 1 || !thresholdValid(policy.MetricThresholds) {
		return false
	}
	if policy.MinimumSample.MinimumAfterPeriods == nil && policy.MinimumSample.MinimumAfterSample == nil {
		return false
	}
	if policy.MinimumSample.MinimumAfterPeriods != nil && (*policy.MinimumSample.MinimumAfterPeriods < 1 || *policy.MinimumSample.MinimumAfterPeriods > 365) {
		return false
	}
	if policy.MinimumSample.MinimumAfterSample != nil && (*policy.MinimumSample.MinimumAfterSample <= 0 || *policy.MinimumSample.MinimumAfterSample > 1_000_000_000_000) {
		return false
	}
	previous := policy.PrimaryCheckpointOffsetDays
	for _, offset := range policy.FollowUpOffsetsDays {
		if offset <= previous || offset > policy.MaxMeasuringDurationDays {
			return false
		}
		previous = offset
	}
	sources := map[string]bool{}
	for _, source := range policy.RequiredDataSources {
		if source == "" || source != normalizeClassifierToken(source) || !supportedDataSource(source) || sources[source] {
			return false
		}
		sources[source] = true
	}
	guardrails := map[string]bool{}
	for _, guardrail := range policy.Guardrails {
		metric := guardrail.Metric
		if metric == "" || metric != normalizeClassifierToken(metric) || !supportedMetric(metric) || guardrails[metric] || guardrail.MaxAdverseRelative <= 0 || guardrail.MaxAdverseRelative > 1 {
			return false
		}
		guardrails[metric] = true
	}
	return true
}

func thresholdValid(threshold finiteMetricThreshold) bool {
	return (threshold.Direction == "increase" || threshold.Direction == "decrease") &&
		(threshold.Kind == "absolute" || threshold.Kind == "relative") && threshold.Value >= 0 && threshold.Value <= 1_000_000_000
}

func baselineReady(document measurementPlanDocument, cutoff time.Time, metrics, sources []string) bool {
	var window baselineWindowDocument
	if json.Unmarshal(document.BaselineWindow, &window) != nil {
		return false
	}
	start, startErr := time.Parse(time.RFC3339, strings.TrimSpace(window.Start))
	end, endErr := time.Parse(time.RFC3339, strings.TrimSpace(window.End))
	if startErr != nil || endErr != nil || !end.After(start) || end.Sub(start) > SiteFixBaselineWindowMaxAge {
		return false
	}
	var snapshot map[string]any
	if json.Unmarshal(document.BaselineSnapshot, &snapshot) != nil || len(snapshot) == 0 {
		return false
	}
	for _, metric := range metrics {
		value, ok := snapshot[metric].(float64)
		if !ok || value < 0 {
			return false
		}
	}
	var provenance baselineProvenanceDocument
	if len(sources) != 1 || json.Unmarshal(document.BaselineProvenance, &provenance) != nil || provenance.Source != normalizeClassifierToken(provenance.Source) || !supportedDataSource(provenance.Source) {
		return false
	}
	capturedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(provenance.CapturedAt))
	if err != nil || end.After(capturedAt) || capturedAt.After(cutoff) || cutoff.Sub(capturedAt) > SiteFixBaselineCaptureMaxAge || cutoff.Sub(end) > SiteFixBaselineEndMaxAge {
		return false
	}
	return sources[0] == provenance.Source
}

func metricSourcePairSupported(metric string, sources []string) bool {
	want := ""
	switch metric {
	case "ctr", "impressions", "clicks", "position":
		want = "gsc"
	case "conversion_rate", "qualified_actions", "referral_sessions":
		want = "ga4"
	case "citations", "brand_mentions":
		want = "geo"
	default:
		return false
	}
	return containsNormalizedToken(sources, want)
}

func supportedMetric(metric string) bool {
	return metricSourcePairSupported(metric, []string{"gsc", "ga4", "geo"})
}

func metricRequiresQuery(metric string) bool {
	switch metric {
	case "ctr", "impressions", "clicks", "position":
		return true
	default:
		return false
	}
}

func metricRequiresEntity(metric string) bool {
	return metric == "citations" || metric == "brand_mentions"
}

func supportedDataSource(source string) bool {
	return source == "gsc" || source == "ga4" || source == "geo"
}

func normalizedMeasurementTargetURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" || u.Hostname() == "" {
		return "", false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host += ":" + port
	}
	u.Host = host
	u.RawQuery = u.Query().Encode()
	normalized := u.String()
	return normalized, normalized == raw
}

func fixTypeFromAcceptanceTests(raw json.RawMessage) (string, bool) {
	var tests []struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &tests) != nil {
		return "", false
	}
	fixTypes := make([]string, 0, len(tests))
	for _, test := range tests {
		switch normalizeClassifierToken(test.Type) {
		case "schema_valid", "jsonld_valid", "structured_data_valid":
			fixTypes = append(fixTypes, "schema_validity_repair")
		case "entity_identity_present", "schema_entity_present":
			fixTypes = append(fixTypes, "schema_entity_optimization")
		}
	}
	if len(fixTypes) == 0 {
		return "", false
	}
	sort.Strings(fixTypes)
	return fixTypes[0], true
}

func fixTypeFromMutations(raw json.RawMessage) (string, bool) {
	var document struct {
		Mutations []struct {
			Field string `json:"field"`
		} `json:"mutations"`
	}
	if json.Unmarshal(raw, &document) != nil {
		return "", false
	}
	fixTypes := make([]string, 0, len(document.Mutations))
	for _, mutation := range document.Mutations {
		if fixType, ok := mutationFieldRules[normalizeClassifierToken(mutation.Field)]; ok {
			fixTypes = append(fixTypes, fixType)
		}
	}
	if len(fixTypes) == 0 {
		return "", false
	}
	sort.Strings(fixTypes)
	return fixTypes[0], true
}

func finitePolicyShape(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return false
	}
	for _, field := range []string{
		"policy_version", "early_signal_offset_days", "primary_checkpoint_offset_days",
		"follow_up_offsets_days", "max_follow_up_attempts", "max_measuring_duration_days",
		"minimum_sample", "metric_thresholds", "guardrails", "required_data_sources",
		"terminalization_grace_period_days",
	} {
		if _, ok := object[field]; !ok {
			return false
		}
	}
	var followUps, guardrails, sources []any
	return json.Unmarshal(object["follow_up_offsets_days"], &followUps) == nil && followUps != nil &&
		json.Unmarshal(object["guardrails"], &guardrails) == nil && guardrails != nil &&
		json.Unmarshal(object["required_data_sources"], &sources) == nil && sources != nil
}

func objectField(raw json.RawMessage, field string) (json.RawMessage, bool) {
	value, ok := rawField(raw, field)
	if !ok || len(value) == 0 || string(value) == "null" || !nonEmptyJSONObject(value) {
		return nil, false
	}
	return value, true
}

func rawField(raw json.RawMessage, field string) (json.RawMessage, bool) {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return nil, false
	}
	value, ok := object[field]
	return value, ok
}

func stringField(raw json.RawMessage, field string) (string, bool) {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return "", false
	}
	var value string
	if json.Unmarshal(object[field], &value) != nil {
		return "", false
	}
	value = normalizeClassifierToken(value)
	return value, value != ""
}

func nonEmptyJSONObject(raw json.RawMessage) bool {
	var object map[string]any
	return json.Unmarshal(raw, &object) == nil && len(object) > 0
}

func isKnownFixType(fixType string) bool {
	_, ok := siteFixClassificationRules[canonicalFixType(fixType)]
	return ok
}

func canonicalFixType(fixType string) string {
	fixType = normalizeClassifierToken(fixType)
	if fixType == "security_or_config_repair" {
		return "technical_fix"
	}
	return fixType
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

func normalizeClassifierToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func containsNormalizedToken(values []string, want string) bool {
	want = normalizeClassifierToken(want)
	for _, value := range values {
		if normalizeClassifierToken(value) == want {
			return true
		}
	}
	return false
}
