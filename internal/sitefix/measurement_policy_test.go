package sitefix

import (
	"encoding/json"
	"testing"
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
		{name: "missing target query", mutate: func(plan map[string]any) { delete(plan, "target_query") }},
		{name: "missing baseline snapshot", mutate: func(plan map[string]any) { plan["baseline_snapshot"] = map[string]any{} }},
		{name: "missing baseline provenance", mutate: func(plan map[string]any) { delete(plan, "baseline_provenance") }},
		{name: "unbounded policy", mutate: func(plan map[string]any) {
			policy := plan["policy_snapshot"].(map[string]any)
			policy["max_measuring_duration_days"] = 0
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

func completeCTRMeasurementPlanJSON() json.RawMessage {
	return json.RawMessage(`{
		"growth_hypothesis":"A clearer title will improve qualified organic CTR without reducing impressions.",
		"primary_metric":"ctr",
		"secondary_metrics":["impressions","clicks","position"],
		"target_query":"social publishing api",
		"baseline_window":{"start":"2026-05-01T00:00:00Z","end":"2026-05-28T00:00:00Z"},
		"baseline_snapshot":{"ctr":0.04,"impressions":1200,"clicks":48},
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
	}`)
}
