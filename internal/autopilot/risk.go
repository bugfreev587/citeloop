// Package autopilot implements guarded autonomy primitives for SEO operations.
package autopilot

import "strings"

const DefaultRiskClassifierVersion = "v1"

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type RiskPolicy struct {
	LowTrafficClicks28DThreshold      int
	LowTrafficImpressions28DThreshold int
	MinConfidenceForAutoPublish       float64
	ClassifierVersion                 string
}

type RiskInput struct {
	ActionType           string
	PageType             string
	DiffScope            string
	AssetType            string
	PublicationSurface   string
	DistributionPlatform string
	ExternalOwnerType    string
	Clicks28D            int
	Impressions28D       int
	TrafficPercentile    float64
	Confidence           float64
	TouchesProductClaim  bool
	TouchesCanonical     bool
	TouchesRobots        bool
	TouchesRedirect      bool
	MergeNoindexDelete   bool
	SchemaChange         bool
}

type RiskResult struct {
	Level             RiskLevel `json:"risk_level"`
	Reasons           []string  `json:"risk_reasons"`
	ClassifierVersion string    `json:"classifier_version"`
	LowTraffic        bool      `json:"low_traffic"`
}

func ClassifyRisk(input RiskInput, policy RiskPolicy) RiskResult {
	policy = normalizePolicy(policy)
	result := RiskResult{
		Level:             RiskLow,
		ClassifierVersion: policy.ClassifierVersion,
		LowTraffic:        isLowTraffic(input, policy),
	}
	pageType := strings.ToLower(strings.TrimSpace(input.PageType))
	action := strings.ToLower(strings.TrimSpace(input.ActionType))
	diff := strings.ToLower(strings.TrimSpace(input.DiffScope))
	asset := strings.ToLower(strings.TrimSpace(input.AssetType))
	surface := strings.ToLower(strings.TrimSpace(input.PublicationSurface))
	platform := strings.ToLower(strings.TrimSpace(input.DistributionPlatform))
	owner := strings.ToLower(strings.TrimSpace(input.ExternalOwnerType))

	if criticalPage(pageType) {
		return result.high("critical_page")
	}
	if input.MergeNoindexDelete || strings.Contains(action, "merge") || strings.Contains(action, "noindex") || strings.Contains(action, "delete") {
		return result.high("merge_noindex_or_delete")
	}
	if input.TouchesCanonical || input.TouchesRobots || input.TouchesRedirect {
		return result.high("canonical_robots_or_redirect_change")
	}
	if diff == "major rewrite" || strings.Contains(action, "major") {
		return result.high("major_rewrite")
	}
	if input.TrafficPercentile >= 80 {
		return result.high("high_traffic_page")
	}
	if surface == "external" && communityPlatform(platform) {
		return result.high("community_distribution_requires_manual_review")
	}

	if strings.Contains(action, "create supporting article") || strings.Contains(action, "refresh") || diff == "paragraph" || diff == "section" {
		result.medium("content_body_change")
	}
	if asset == "comparison_page" || asset == "alternative_page" {
		result.medium("buyer_intent_asset")
	}
	if asset == "schema_patch" || input.SchemaChange {
		result.medium("structured_data_change")
	}
	if surface == "external" || owner == "third_party" {
		result.medium("external_surface_change")
	}
	if strings.Contains(action, "internal link") && !result.LowTraffic {
		result.medium("internal_link_on_non_low_traffic_page")
	}
	if input.TouchesProductClaim {
		result.medium("product_claim_diff")
	}
	if input.Confidence > 0 && input.Confidence < policy.MinConfidenceForAutoPublish {
		result.medium("confidence_below_auto_publish_threshold")
	}
	return result
}

func communityPlatform(platform string) bool {
	switch platform {
	case "reddit", "hacker_news", "hn":
		return true
	default:
		return false
	}
}

func normalizePolicy(policy RiskPolicy) RiskPolicy {
	if policy.LowTrafficClicks28DThreshold == 0 {
		policy.LowTrafficClicks28DThreshold = 10
	}
	if policy.LowTrafficImpressions28DThreshold == 0 {
		policy.LowTrafficImpressions28DThreshold = 500
	}
	if policy.MinConfidenceForAutoPublish == 0 {
		policy.MinConfidenceForAutoPublish = 80
	}
	if policy.ClassifierVersion == "" {
		policy.ClassifierVersion = DefaultRiskClassifierVersion
	}
	return policy
}

func isLowTraffic(input RiskInput, policy RiskPolicy) bool {
	return input.Clicks28D < policy.LowTrafficClicks28DThreshold &&
		input.Impressions28D < policy.LowTrafficImpressions28DThreshold &&
		input.TrafficPercentile < 60 &&
		!criticalPage(strings.ToLower(strings.TrimSpace(input.PageType)))
}

func criticalPage(pageType string) bool {
	switch pageType {
	case "homepage", "pricing", "docs", "legal":
		return true
	default:
		return false
	}
}

func (r RiskResult) high(reason string) RiskResult {
	r.Level = RiskHigh
	r.Reasons = append(r.Reasons, reason)
	return r
}

func (r *RiskResult) medium(reason string) {
	if r.Level != RiskHigh {
		r.Level = RiskMedium
	}
	r.Reasons = append(r.Reasons, reason)
}
