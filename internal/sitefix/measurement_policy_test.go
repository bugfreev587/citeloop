package sitefix

import (
	"encoding/json"
	"testing"
	"time"
)

func TestClassifySiteFixMeasurementDeterministicPrecedence(t *testing.T) {
	completePlan := completeCTRMeasurementPlanJSON()
	tests := []struct {
		name       string
		input      MeasurementClassificationInput
		wantType   string
		wantImpact string
		wantPolicy string
		wantOrigin string
		wantConf   string
	}{
		{
			name: "valid explicit override wins",
			input: MeasurementClassificationInput{
				ReferenceTime:    baselineCutoffForTest(),
				TargetURLs:       json.RawMessage(`["https://example.com/pricing"]`),
				FindingIssueType: "canonical_missing",
				FindingEvidence: json.RawMessage(`{
					"site_fix_policy_override": {
						"fix_type": "metadata_ctr_optimization",
						"impact_mode": "conversion_or_ctr",
						"measurement_policy": "measurement_required",
						"measurement_plan": ` + string(completePlan) + `
					}
				}`),
				ProposedFix: json.RawMessage(`{"fix_type":"schema_entity_optimization","mutations":[{"operation":"add","field":"schema_entity"}]}`),
			},
			wantType: "metadata_ctr_optimization", wantImpact: "conversion_or_ctr",
			wantPolicy: "measurement_required", wantOrigin: "user_override", wantConf: "high",
		},
		{
			name: "invalid explicit override fails closed instead of falling through",
			input: MeasurementClassificationInput{
				TargetURLs:       json.RawMessage(`["https://example.com/pricing"]`),
				FindingIssueType: "canonical_missing",
				FindingEvidence:  json.RawMessage(`{"site_fix_policy_override":{"fix_type":"not-a-fix-type","measurement_policy":"measurement_required"}}`),
				ProposedFix:      json.RawMessage(`{"fix_type":"schema_validity_repair","mutations":[{"operation":"add","field":"schema_entity"}]}`),
			},
			wantType: "unknown", wantImpact: "unclassified",
			wantPolicy: "verification_only", wantOrigin: "user_override", wantConf: "low",
		},
		{
			name: "structured proposed fix wins over issue and mutation",
			input: MeasurementClassificationInput{
				TargetURLs:       json.RawMessage(`["https://example.com/pricing"]`),
				FindingIssueType: "canonical_missing",
				ProposedFix:      json.RawMessage(`{"fix_type":"schema_entity_optimization","mutations":[{"operation":"update","field":"canonical"}]}`),
			},
			wantType: "schema_entity_optimization", wantImpact: "geo_visibility",
			wantPolicy: "verification_only", wantOrigin: "system_rule", wantConf: "high",
		},
		{
			name: "exact issue wins over mutation",
			input: MeasurementClassificationInput{
				TargetURLs:       json.RawMessage(`["https://example.com/pricing"]`),
				FindingIssueType: "canonical_missing",
				ProposedFix:      json.RawMessage(`{"mutations":[{"operation":"update","field":"title"}]}`),
			},
			wantType: "canonical_repair", wantImpact: "technical_reliability",
			wantPolicy: "verification_only", wantOrigin: "system_rule", wantConf: "high",
		},
		{
			name: "exact mutation fallback",
			input: MeasurementClassificationInput{
				TargetURLs:       json.RawMessage(`["https://example.com/pricing"]`),
				FindingIssueType: "new_issue_type",
				ProposedFix:      json.RawMessage(`{"mutations":[{"operation":"update","field":"title"}]}`),
			},
			wantType: "title_readability", wantImpact: "presentation_only",
			wantPolicy: "verification_only", wantOrigin: "system_rule", wantConf: "medium",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifySiteFixMeasurement(tt.input)
			if got.FixType != tt.wantType || got.ImpactMode != tt.wantImpact || got.MeasurementPolicy != tt.wantPolicy ||
				got.DecisionOrigin != tt.wantOrigin || got.DecisionConfidence != tt.wantConf {
				t.Fatalf("classification = %+v", got)
			}
			if got.ClassifierVersion != SiteFixClassifierVersionV1 {
				t.Fatalf("classifier version = %q", got.ClassifierVersion)
			}
			if tt.name == "invalid explicit override fails closed instead of falling through" && got.ValidationError == "" {
				t.Fatal("invalid explicit override did not expose a validation error")
			}
		})
	}
}

func TestClassifySiteFixMeasurementNullOverrideIsAbsentButMalformedOverrideFails(t *testing.T) {
	nullOverride := ClassifySiteFixMeasurement(MeasurementClassificationInput{
		FindingIssueType: "canonical_missing",
		FindingEvidence:  json.RawMessage(`{"site_fix_policy_override":null}`),
		ProposedFix:      json.RawMessage(`{"mutations":[{"operation":"update","field":"title"}]}`),
	})
	if nullOverride.FixType != "canonical_repair" || nullOverride.ValidationError != "" {
		t.Fatalf("null override should be absent: %+v", nullOverride)
	}
	malformed := ClassifySiteFixMeasurement(MeasurementClassificationInput{
		FindingIssueType: "canonical_missing",
		FindingEvidence:  json.RawMessage(`{"site_fix_policy_override":"measurement_required"}`),
	})
	if malformed.FixType != "unknown" || malformed.MeasurementPolicy != "verification_only" || malformed.ValidationError == "" {
		t.Fatalf("malformed non-null override should fail closed: %+v", malformed)
	}
}

func TestClassifySiteFixMeasurementCanonicalizesSecurityAlias(t *testing.T) {
	got := ClassifySiteFixMeasurement(MeasurementClassificationInput{
		FindingIssueType: "novel_issue",
		ProposedFix:      json.RawMessage(`{"fix_type":"security_or_config_repair"}`),
	})
	if got.FixType != "technical_fix" || got.ImpactMode != "technical_reliability" || got.MeasurementPolicy != "verification_only" {
		t.Fatalf("security alias = %+v", got)
	}
}

func TestClassifySiteFixMeasurementPresentationAndUnknownFailClosed(t *testing.T) {
	tests := []struct {
		name       string
		input      MeasurementClassificationInput
		wantType   string
		wantImpact string
		wantConf   string
	}{
		{
			name: "metadata readability remains presentation only despite CTR prose",
			input: MeasurementClassificationInput{
				TargetURLs:       json.RawMessage(`["https://example.com/blog/post"]`),
				FindingIssueType: "metadata_readability",
				FindingEvidence:  json.RawMessage(`{"why":"improve CTR clicks impressions and ranking"}`),
				ProposedFix:      json.RawMessage(`{"fix_intent":"Rewrite metadata to increase CTR and clicks","mutations":[{"operation":"update","field":"title"}]}`),
			},
			wantType: "title_readability", wantImpact: "presentation_only", wantConf: "high",
		},
		{
			name: "unknown does not become technical or optional",
			input: MeasurementClassificationInput{
				TargetURLs:       json.RawMessage(`["https://example.com/blog/post"]`),
				FindingIssueType: "novel_unknown_issue",
				FindingEvidence:  json.RawMessage(`{"description":"CTR clicks citations conversion schema canonical"}`),
				ProposedFix:      json.RawMessage(`{"developer_instructions":"Improve organic CTR","mutations":[{"operation":"update","field":"novel_field"}]}`),
			},
			wantType: "unknown", wantImpact: "unclassified", wantConf: "low",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifySiteFixMeasurement(tt.input)
			if got.FixType != tt.wantType || got.ImpactMode != tt.wantImpact || got.MeasurementPolicy != "verification_only" || got.DecisionConfidence != tt.wantConf {
				t.Fatalf("classification = %+v", got)
			}
			if got.GrowthHypothesis != nil || got.PrimaryMetric != nil || len(got.MeasurementPolicySnapshot) != 0 {
				t.Fatalf("fail-closed classification retained measurement plan: %+v", got)
			}
		})
	}
}

func TestClassifySiteFixMeasurementSchemaValidityVersusEntityOptimization(t *testing.T) {
	tests := []struct {
		name       string
		input      MeasurementClassificationInput
		wantType   string
		wantImpact string
		wantPolicy string
	}{
		{
			name: "missing schema is an optional patch rather than a validity repair",
			input: MeasurementClassificationInput{
				TargetURLs:       json.RawMessage(`["https://example.com/product"]`),
				FindingIssueType: "structured_data_missing",
				ProposedFix:      json.RawMessage(`{"mutations":[{"operation":"add","field":"jsonld"}]}`),
			},
			wantType: "schema_patch", wantImpact: "search_visibility", wantPolicy: "measurement_optional",
		},
		{
			name: "schema parser repair is technical validity",
			input: MeasurementClassificationInput{
				TargetURLs:       json.RawMessage(`["https://example.com/product"]`),
				FindingIssueType: "structured_data_invalid",
				ProposedFix:      json.RawMessage(`{"mutations":[{"operation":"update","field":"jsonld"}]}`),
				AcceptanceTests:  json.RawMessage(`[{"type":"schema_valid"}]`),
			},
			wantType: "schema_validity_repair", wantImpact: "technical_reliability", wantPolicy: "verification_only",
		},
		{
			name: "typed entity optimization requires readiness",
			input: MeasurementClassificationInput{
				TargetURLs:       json.RawMessage(`["https://example.com/product"]`),
				FindingIssueType: "structured_data_invalid",
				ProposedFix:      json.RawMessage(`{"fix_type":"schema_entity_optimization","mutations":[{"operation":"update","field":"jsonld"}]}`),
			},
			wantType: "schema_entity_optimization", wantImpact: "geo_visibility", wantPolicy: "verification_only",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifySiteFixMeasurement(tt.input)
			if got.FixType != tt.wantType || got.ImpactMode != tt.wantImpact || got.MeasurementPolicy != tt.wantPolicy {
				t.Fatalf("classification = %+v", got)
			}
		})
	}
}

func TestClassifySiteFixMeasurementRequiredReadiness(t *testing.T) {
	valid := MeasurementClassificationInput{
		ReferenceTime:    baselineCutoffForTest(),
		TargetURLs:       json.RawMessage(`["https://example.com/pricing"]`),
		FindingIssueType: "metadata_ctr_optimization",
		FindingEvidence:  json.RawMessage(`{"measurement_plan":` + string(completeCTRMeasurementPlanJSON()) + `}`),
		ProposedFix:      json.RawMessage(`{"mutations":[{"operation":"update","field":"title"}]}`),
	}
	got := ClassifySiteFixMeasurement(valid)
	if got.MeasurementPolicy != "measurement_required" || got.GrowthHypothesis == nil || *got.GrowthHypothesis == "" ||
		got.PrimaryMetric == nil || *got.PrimaryMetric != "ctr" || got.MeasurementPolicyVersion == nil ||
		*got.MeasurementPolicyVersion != "site-fix-growth-v1" || len(got.MeasurementPolicySnapshot) == 0 || got.Plan == nil {
		t.Fatalf("ready required classification = %+v", got)
	}

	tests := []struct {
		name   string
		mutate func(map[string]any)
		target json.RawMessage
	}{
		{name: "multiple target URLs", target: json.RawMessage(`["https://example.com/pricing","https://example.com/plans"]`)},
		{name: "non-normalized target URL", target: json.RawMessage(`["HTTPS://EXAMPLE.COM/pricing#top"]`)},
		{name: "blank hypothesis", mutate: func(plan map[string]any) { plan["growth_hypothesis"] = " " }},
		{name: "unsupported metric source pair", mutate: func(plan map[string]any) { plan["primary_metric"] = "conversion_rate" }},
		{name: "unsupported same-source guardrail", mutate: func(plan map[string]any) {
			policy := plan["policy_snapshot"].(map[string]any)
			policy["guardrails"] = []any{map[string]any{"metric": "clicks", "max_adverse_relative": 0.2}}
		}},
		{name: "missing target query", mutate: func(plan map[string]any) { delete(plan, "target_query") }},
		{name: "missing baseline snapshot", mutate: func(plan map[string]any) { plan["baseline_snapshot"] = map[string]any{} }},
		{name: "baseline metric missing sample size", mutate: func(plan map[string]any) {
			delete(plan["baseline_snapshot"].(map[string]any)["ctr"].(map[string]any), "sample_size")
		}},
		{name: "baseline metric missing rows", mutate: func(plan map[string]any) {
			delete(plan["baseline_snapshot"].(map[string]any)["ctr"].(map[string]any), "rows")
		}},
		{name: "baseline metric missing partial flag", mutate: func(plan map[string]any) {
			delete(plan["baseline_snapshot"].(map[string]any)["ctr"].(map[string]any), "partial")
		}},
		{name: "missing baseline provenance", mutate: func(plan map[string]any) { delete(plan, "baseline_provenance") }},
		{name: "unbounded policy", mutate: func(plan map[string]any) {
			policy := plan["policy_snapshot"].(map[string]any)
			policy["max_measuring_duration_days"] = 0
		}},
		{name: "future baseline", mutate: func(plan map[string]any) {
			window := plan["baseline_window"].(map[string]any)
			window["end"] = baselineCutoffForTest().Add(time.Hour).Format(time.RFC3339)
		}},
		{name: "captured before baseline ended", mutate: func(plan map[string]any) {
			window := plan["baseline_window"].(map[string]any)
			provenance := plan["baseline_provenance"].(map[string]any)
			provenance["captured_at"] = mustTimeForTest(window["end"].(string)).Add(-time.Minute).Format(time.RFC3339)
		}},
		{name: "stale baseline capture", mutate: func(plan map[string]any) {
			provenance := plan["baseline_provenance"].(map[string]any)
			provenance["captured_at"] = baselineCutoffForTest().Add(-(7*24*time.Hour + time.Minute)).Format(time.RFC3339)
		}},
		{name: "stale baseline window end", mutate: func(plan map[string]any) {
			window := plan["baseline_window"].(map[string]any)
			window["start"] = baselineCutoffForTest().Add(-60 * 24 * time.Hour).Format(time.RFC3339)
			window["end"] = baselineCutoffForTest().Add(-(10*24*time.Hour + time.Hour)).Format(time.RFC3339)
			provenance := plan["baseline_provenance"].(map[string]any)
			provenance["captured_at"] = baselineCutoffForTest().Add(-time.Hour).Format(time.RFC3339)
		}},
		{name: "secondary metric absent from baseline", mutate: func(plan map[string]any) {
			delete(plan["baseline_snapshot"].(map[string]any), "position")
		}},
		{name: "guardrail metric absent from baseline", mutate: func(plan map[string]any) {
			delete(plan["baseline_snapshot"].(map[string]any), "impressions")
		}},
		{name: "secondary metric from another source", mutate: func(plan map[string]any) {
			plan["secondary_metrics"] = []any{"conversion_rate"}
			plan["baseline_snapshot"].(map[string]any)["conversion_rate"] = 0.1
		}},
		{name: "guardrail metric from another source", mutate: func(plan map[string]any) {
			policy := plan["policy_snapshot"].(map[string]any)
			policy["guardrails"] = []any{map[string]any{"metric": "conversion_rate", "max_adverse_relative": 0.1}}
			plan["baseline_snapshot"].(map[string]any)["conversion_rate"] = 0.1
		}},
		{name: "multiple required sources are outside v1", mutate: func(plan map[string]any) {
			policy := plan["policy_snapshot"].(map[string]any)
			policy["required_data_sources"] = []any{"gsc", "ga4"}
		}},
		{name: "non canonical policy version", mutate: func(plan map[string]any) {
			policy := plan["policy_snapshot"].(map[string]any)
			policy["policy_version"] = " SITE-FIX-GROWTH-V1 "
		}},
		{name: "non canonical source token", mutate: func(plan map[string]any) {
			policy := plan["policy_snapshot"].(map[string]any)
			policy["required_data_sources"] = []any{" GSC "}
			plan["baseline_provenance"].(map[string]any)["source"] = " GSC "
		}},
		{name: "non canonical primary metric", mutate: func(plan map[string]any) {
			plan["primary_metric"] = " CTR "
		}},
		{name: "non canonical secondary metric", mutate: func(plan map[string]any) {
			plan["secondary_metrics"] = []any{" Impressions ", "clicks", "position"}
		}},
		{name: "non canonical guardrail alias", mutate: func(plan map[string]any) {
			policy := plan["policy_snapshot"].(map[string]any)
			policy["guardrails"] = []any{map[string]any{"metric": "organic_impressions", "max_adverse_relative": 0.1}}
			plan["baseline_snapshot"].(map[string]any)["organic_impressions"] = 1200
		}},
		{name: "non-array guardrails", mutate: func(plan map[string]any) {
			policy := plan["policy_snapshot"].(map[string]any)
			policy["guardrails"] = nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var plan map[string]any
			if err := json.Unmarshal(completeCTRMeasurementPlanJSON(), &plan); err != nil {
				t.Fatal(err)
			}
			if tt.mutate != nil {
				tt.mutate(plan)
			}
			raw, _ := json.Marshal(map[string]any{"measurement_plan": plan})
			input := valid
			input.FindingEvidence = raw
			if tt.target != nil {
				input.TargetURLs = tt.target
			}
			got := ClassifySiteFixMeasurement(input)
			if got.FixType != "metadata_ctr_optimization" || got.ImpactMode != "conversion_or_ctr" || got.MeasurementPolicy != "verification_only" {
				t.Fatalf("not-ready required candidate did not fail closed: %+v", got)
			}
			if got.Plan != nil || got.GrowthHypothesis != nil || got.PrimaryMetric != nil || len(got.MeasurementPolicySnapshot) != 0 {
				t.Fatalf("not-ready candidate retained plan: %+v", got)
			}
		})
	}
}

func TestClassifySiteFixMeasurementBaselineFreshnessBoundaries(t *testing.T) {
	if SiteFixBaselineCaptureMaxAge != 7*24*time.Hour || SiteFixBaselineEndMaxAge != 10*24*time.Hour || SiteFixBaselineWindowMaxAge != 90*24*time.Hour {
		t.Fatalf("unexpected v1 freshness bounds: capture=%s end=%s window=%s", SiteFixBaselineCaptureMaxAge, SiteFixBaselineEndMaxAge, SiteFixBaselineWindowMaxAge)
	}
	cutoff := baselineCutoffForTest()
	var plan map[string]any
	if err := json.Unmarshal(completeCTRMeasurementPlanJSON(), &plan); err != nil {
		t.Fatal(err)
	}
	plan["baseline_window"] = map[string]any{
		"start": cutoff.Add(-38 * 24 * time.Hour).Format(time.RFC3339),
		"end":   cutoff.Add(-SiteFixBaselineEndMaxAge).Format(time.RFC3339),
	}
	plan["baseline_provenance"] = map[string]any{
		"source": "gsc", "captured_at": cutoff.Add(-SiteFixBaselineCaptureMaxAge).Format(time.RFC3339),
	}
	evidence, _ := json.Marshal(map[string]any{"measurement_plan": plan})
	got := ClassifySiteFixMeasurement(MeasurementClassificationInput{
		ReferenceTime: cutoff, TargetURLs: json.RawMessage(`["https://example.com/pricing"]`),
		FindingIssueType: "metadata_ctr_optimization", FindingEvidence: evidence,
	})
	if got.MeasurementPolicy != "measurement_required" {
		t.Fatalf("exact freshness boundaries must remain eligible: %+v", got)
	}
}

func TestClassifySiteFixMeasurementProductionIssueTaxonomy(t *testing.T) {
	tests := []struct {
		issue, wantType, wantImpact, wantPolicy string
	}{
		{"structured_data_missing", "schema_patch", "search_visibility", "measurement_optional"},
		{"schema_gap", "schema_patch", "search_visibility", "measurement_optional"},
		{"json_ld_missing", "schema_patch", "search_visibility", "measurement_optional"},
		{"schema_missing", "schema_patch", "search_visibility", "measurement_optional"},
		{"title_missing", "metadata_format", "presentation_only", "verification_only"},
		{"missing_title", "metadata_format", "presentation_only", "verification_only"},
		{"metadata_title", "title_readability", "presentation_only", "verification_only"},
		{"title_duplicate", "title_readability", "presentation_only", "verification_only"},
		{"duplicate_title", "title_readability", "presentation_only", "verification_only"},
		{"title_too_long", "title_readability", "presentation_only", "verification_only"},
		{"title_invalid", "title_readability", "presentation_only", "verification_only"},
		{"meta_description_missing", "metadata_format", "presentation_only", "verification_only"},
		{"metadata_description", "metadata_format", "presentation_only", "verification_only"},
		{"h1_missing", "content_typo_or_clarity", "presentation_only", "verification_only"},
		{"canonical_missing", "canonical_repair", "technical_reliability", "verification_only"},
		{"canonical_mismatch", "canonical_repair", "technical_reliability", "verification_only"},
		{"canonical_invalid", "canonical_repair", "technical_reliability", "verification_only"},
		{"canonical_multiple", "canonical_repair", "technical_reliability", "verification_only"},
		{"robots_blocked", "robots_repair", "technical_reliability", "verification_only"},
		{"robots_conflict", "robots_repair", "technical_reliability", "verification_only"},
		{"noindex", "robots_repair", "technical_reliability", "verification_only"},
		{"noindex_conflict", "robots_repair", "technical_reliability", "verification_only"},
		{"broken_url", "redirect_or_http_repair", "technical_reliability", "verification_only"},
		{"soft_404", "redirect_or_http_repair", "technical_reliability", "verification_only"},
		{"redirect_loop", "redirect_or_http_repair", "technical_reliability", "verification_only"},
		{"redirect_chain", "redirect_or_http_repair", "technical_reliability", "verification_only"},
		{"internal_link_gap", "internal_link_patch", "search_visibility", "measurement_optional"},
		{"zero_internal_links", "internal_link_patch", "search_visibility", "measurement_optional"},
		{"broken_internal_link", "internal_link_patch", "search_visibility", "measurement_optional"},
		{"orphan_page", "internal_link_patch", "search_visibility", "measurement_optional"},
		{"sitemap_update", "sitemap_repair", "technical_reliability", "verification_only"},
		{"important_page_missing_from_sitemap", "sitemap_repair", "technical_reliability", "verification_only"},
		{"geo_crawler_access_blocked", "robots_repair", "technical_reliability", "verification_only"},
		{"unsafe_mdx_detected", "schema_validity_repair", "technical_reliability", "verification_only"},
		{"metadata_readability", "title_readability", "presentation_only", "verification_only"},
		{"duplicate_metadata_template", "metadata_format", "presentation_only", "verification_only"},
		{"supported_fact_extractability", "geo_content_clarity", "geo_visibility", "measurement_optional"},
		{"citation_readiness_structure", "geo_content_clarity", "geo_visibility", "measurement_optional"},
		{"source_association", "geo_content_clarity", "geo_visibility", "measurement_optional"},
		{"entity_naming_consistency", "geo_content_clarity", "geo_visibility", "measurement_optional"},
		{"ga4_missing", "technical_fix", "technical_reliability", "verification_only"},
		{"tracking_missing", "technical_fix", "technical_reliability", "verification_only"},
		{"measurement_readiness", "technical_fix", "technical_reliability", "verification_only"},
		{"security_or_config_repair", "technical_fix", "technical_reliability", "verification_only"},
	}
	for _, tt := range tests {
		t.Run(tt.issue, func(t *testing.T) {
			got := ClassifySiteFixMeasurement(MeasurementClassificationInput{FindingIssueType: tt.issue})
			if got.FixType != tt.wantType || got.ImpactMode != tt.wantImpact || got.MeasurementPolicy != tt.wantPolicy || got.DecisionConfidence != "high" {
				t.Fatalf("classification = %+v", got)
			}
		})
	}
}

func TestClassifySiteFixMeasurementStructuredFallbackTieBreak(t *testing.T) {
	tests := []struct {
		name     string
		input    MeasurementClassificationInput
		wantType string
	}{
		{
			name: "acceptance beats mutation and surface",
			input: MeasurementClassificationInput{
				FindingIssueType: "novel_issue", TargetSurface: "content.entity",
				AcceptanceTests: json.RawMessage(`[{"type":"schema_valid"}]`),
				ProposedFix:     json.RawMessage(`{"mutations":[{"operation":"update","field":"schema_entity"}]}`),
			},
			wantType: "schema_validity_repair",
		},
		{
			name: "mutation beats surface",
			input: MeasurementClassificationInput{
				FindingIssueType: "novel_issue", TargetSurface: "content.entity",
				ProposedFix: json.RawMessage(`{"mutations":[{"operation":"update","field":"canonical"}]}`),
			},
			wantType: "canonical_repair",
		},
		{
			name: "exact structured surface is final fallback",
			input: MeasurementClassificationInput{
				FindingIssueType: "novel_issue", TargetSurface: "links.internal",
				ProposedFix: json.RawMessage(`{"mutations":[{"operation":"update","field":"novel_field"}]}`),
			},
			wantType: "internal_link_patch",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifySiteFixMeasurement(tt.input)
			if got.FixType != tt.wantType {
				t.Fatalf("classification = %+v", got)
			}
		})
	}
}

func completeCTRMeasurementPlanJSON() json.RawMessage {
	return completeCTRMeasurementPlanJSONAt(baselineCutoffForTest())
}

func completeCTRMeasurementPlanJSONAt(cutoff time.Time) json.RawMessage {
	var plan map[string]any
	if err := json.Unmarshal(json.RawMessage(`{
		"growth_hypothesis":"A clearer title will improve qualified organic CTR without reducing impressions.",
		"primary_metric":"ctr",
		"secondary_metrics":["impressions","clicks","position"],
		"target_query":"social publishing api",
		"baseline_window":{"start":"2026-05-01T00:00:00Z","end":"2026-05-28T00:00:00Z"},
		"baseline_snapshot":{
			"ctr":{"value":0.04,"sample_size":1200,"rows":28,"partial":false},
			"impressions":{"value":1200,"sample_size":1200,"rows":28,"partial":false},
			"clicks":{"value":48,"sample_size":1200,"rows":28,"partial":false},
			"position":{"value":7.2,"sample_size":1200,"rows":28,"partial":false}
		},
		"baseline_provenance":{"source":"gsc","captured_at":"2026-05-29T00:00:00Z"},
		"policy_snapshot":{
			"policy_version":"site-fix-growth-v1",
			"early_signal_offset_days":7,
			"primary_checkpoint_offset_days":28,
			"follow_up_offsets_days":[42],
			"max_follow_up_attempts":1,
			"max_measuring_duration_days":56,
			"minimum_sample":{"minimum_after_periods":7,"minimum_after_sample":100},
			"metric_thresholds":{"direction":"increase","kind":"relative","value":0.05},
			"guardrails":[{"metric":"impressions","max_adverse_relative":0.15}],
			"required_data_sources":["gsc"],
			"terminalization_grace_period_days":2
		}
	}`), &plan); err != nil {
		panic(err)
	}
	plan["baseline_window"] = map[string]any{
		"start": cutoff.Add(-28 * 24 * time.Hour).Format(time.RFC3339),
		"end":   cutoff.Add(-time.Hour).Format(time.RFC3339),
	}
	plan["baseline_provenance"] = map[string]any{
		"source": "gsc", "captured_at": cutoff.Add(-30 * time.Minute).Format(time.RFC3339),
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		panic(err)
	}
	return raw
}

func baselineCutoffForTest() time.Time {
	return mustTimeForTest("2026-05-29T01:00:00Z")
}

func TestRecoverApprovedSiteFixMeasurementPlanFreezesPersistedStructuredPlan(t *testing.T) {
	cutoff := baselineCutoffForTest()
	hypothesis := "A clearer title will improve qualified organic CTR without reducing impressions."
	metric := "ctr"
	version := "site-fix-growth-v1"
	plan, err := RecoverApprovedSiteFixMeasurementPlan(StoredSiteFixMeasurementInput{
		TargetURLs:                json.RawMessage(`["https://example.com/pricing"]`),
		ProposedFix:               json.RawMessage(`{"fix_type":"metadata_ctr_optimization","measurement_plan":` + string(completeCTRMeasurementPlanJSONAt(cutoff)) + `}`),
		FixType:                   "metadata_ctr_optimization",
		ImpactMode:                "conversion_or_ctr",
		MeasurementPolicy:         "measurement_required",
		ClassifierVersion:         SiteFixClassifierVersionV1,
		DecisionOrigin:            "system_rule",
		DecisionConfidence:        "high",
		GrowthHypothesis:          &hypothesis,
		PrimaryMetric:             &metric,
		SecondaryMetrics:          json.RawMessage(`["impressions","clicks","position"]`),
		MeasurementPolicyVersion:  &version,
		MeasurementPolicySnapshot: mustPolicySnapshotForTest(t, completeCTRMeasurementPlanJSONAt(cutoff)),
	}, cutoff)
	if err != nil {
		t.Fatalf("recover approved plan: %v", err)
	}
	if plan.TargetURL != "https://example.com/pricing" || plan.TargetQuery == nil || *plan.TargetQuery != "social publishing api" {
		t.Fatalf("target was not frozen from structured plan: %+v", plan)
	}
	if plan.BaselineStatus != "ready" || plan.Status != "ready" || plan.AttributionConfidence != "high" || plan.ProspectiveObservation {
		t.Fatalf("approved measurement lifecycle = %+v", plan)
	}
	var baseline map[string]any
	if json.Unmarshal(plan.BaselineSnapshot, &baseline) != nil || baseline["provenance"] == nil || baseline["frozen_at"] != cutoff.Format(time.RFC3339) {
		t.Fatalf("baseline provenance was not frozen: %s", plan.BaselineSnapshot)
	}
	var identity map[string]any
	if json.Unmarshal(plan.TargetIdentity, &identity) != nil || identity["target_url"] != "https://example.com/pricing" || identity["target_query"] != "social publishing api" {
		t.Fatalf("target identity was not frozen: %s", plan.TargetIdentity)
	}
	if string(plan.MeasurementPolicySnapshot) != string(mustPolicySnapshotForTest(t, completeCTRMeasurementPlanJSONAt(cutoff))) {
		t.Fatal("recovery mutated the immutable policy snapshot")
	}
}

func TestRecoverStructuredMeasurementPlanSnapshotPrefersFindingOverride(t *testing.T) {
	overridePlan := json.RawMessage(`{"growth_hypothesis":"override plan"}`)
	recovered := recoverStructuredMeasurementPlanSnapshot(StoredSiteFixMeasurementInput{
		ProposedFix: json.RawMessage(`{"measurement_plan":{"growth_hypothesis":"proposed plan"}}`),
		EvidenceSnapshot: json.RawMessage(`{
			"finding": {
				"measurement_plan": {"growth_hypothesis":"regular plan"},
				"site_fix_policy_override": {
					"measurement_plan": {"growth_hypothesis":"override plan"}
				}
			}
		}`),
	})
	if !jsonSemanticallyEqual(recovered, overridePlan) {
		t.Fatalf("recovered plan ignored explicit override: %s", recovered)
	}
}

func TestRecoverApprovedSiteFixMeasurementPlanRevalidatesBaselineAtApprovalCutoff(t *testing.T) {
	classifiedAt := baselineCutoffForTest()
	hypothesis, metric, version := "A clearer title will improve qualified organic CTR without reducing impressions.", "ctr", "site-fix-growth-v1"
	_, err := RecoverApprovedSiteFixMeasurementPlan(StoredSiteFixMeasurementInput{
		TargetURLs:  json.RawMessage(`["https://example.com/pricing"]`),
		ProposedFix: json.RawMessage(`{"measurement_plan":` + string(completeCTRMeasurementPlanJSONAt(classifiedAt)) + `}`),
		FixType:     "metadata_ctr_optimization", ImpactMode: "conversion_or_ctr", MeasurementPolicy: "measurement_required",
		ClassifierVersion: SiteFixClassifierVersionV1, DecisionOrigin: "system_rule", DecisionConfidence: "high",
		GrowthHypothesis: &hypothesis, PrimaryMetric: &metric, SecondaryMetrics: json.RawMessage(`["impressions","clicks","position"]`),
		MeasurementPolicyVersion: &version, MeasurementPolicySnapshot: mustPolicySnapshotForTest(t, completeCTRMeasurementPlanJSONAt(classifiedAt)),
	}, classifiedAt.Add(8*24*time.Hour))
	if err == nil {
		t.Fatal("stale baseline was accepted at approval")
	}
}

func TestRecoverProspectiveSiteFixMeasurementPlanDoesNotFabricateBaseline(t *testing.T) {
	classifiedAt := baselineCutoffForTest()
	optedInAt := classifiedAt.Add(30 * 24 * time.Hour)
	hypothesis, metric, version := "A clearer title will improve qualified organic CTR without reducing impressions.", "ctr", "site-fix-growth-v1"
	plan, err := RecoverProspectiveSiteFixMeasurementPlan(StoredSiteFixMeasurementInput{
		TargetURLs:  json.RawMessage(`["https://example.com/pricing"]`),
		ProposedFix: json.RawMessage(`{"measurement_plan":` + string(completeCTRMeasurementPlanJSONAt(classifiedAt)) + `}`),
		FixType:     "metadata_rewrite", ImpactMode: "search_visibility", MeasurementPolicy: "measurement_optional",
		ClassifierVersion: SiteFixClassifierVersionV1, DecisionOrigin: "system_rule", DecisionConfidence: "high",
		GrowthHypothesis: &hypothesis, PrimaryMetric: &metric, SecondaryMetrics: json.RawMessage(`["impressions","clicks","position"]`),
		MeasurementPolicyVersion: &version, MeasurementPolicySnapshot: mustPolicySnapshotForTest(t, completeCTRMeasurementPlanJSONAt(classifiedAt)),
	}, optedInAt)
	if err != nil {
		t.Fatalf("recover prospective plan: %v", err)
	}
	if !plan.ProspectiveObservation || plan.BaselineStatus != "unavailable" || plan.Status != "ready" || plan.AttributionConfidence != "low" {
		t.Fatalf("prospective measurement lifecycle = %+v", plan)
	}
	var baseline map[string]any
	if json.Unmarshal(plan.BaselineSnapshot, &baseline) != nil || baseline["reason"] != "no_prechange_baseline" || baseline["opted_in_at"] != optedInAt.Format(time.RFC3339) {
		t.Fatalf("prospective baseline provenance = %s", plan.BaselineSnapshot)
	}
	if _, exists := baseline["ctr"]; exists {
		t.Fatal("prospective measurement fabricated a historical metric baseline")
	}
}

func mustPolicySnapshotForTest(t *testing.T, raw json.RawMessage) json.RawMessage {
	t.Helper()
	var document struct {
		Policy json.RawMessage `json:"policy_snapshot"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	return document.Policy
}

func mustTimeForTest(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}
