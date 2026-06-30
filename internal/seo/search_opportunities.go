package seo

import (
	"math"
	"sort"
	"time"
)

const searchOpportunityScoringVersion = "gsc_metric_v2"

type searchQueryRollup struct {
	PageURL           string
	NormalizedPageURL string
	Query             string
	Clicks            float64
	Impressions       float64
	CTR               float64
	Position          float64
	WindowStart       time.Time
	WindowEnd         time.Time
}

type pageDecayRollup struct {
	PageURL             string
	NormalizedPageURL   string
	CurrentClicks       float64
	PreviousClicks      float64
	CurrentImpressions  float64
	PreviousImpressions float64
	WindowStart         time.Time
	WindowEnd           time.Time
}

type searchMetricOpportunityCandidate struct {
	Type              string
	Query             string
	PageURL           string
	NormalizedPageURL string
	PriorityScore     float64
	Confidence        float64
	RecommendedAction string
	ExpectedImpact    string
	Effort            int32
	RiskLevel         string
	Evidence          map[string]any
}

func searchMetricOpportunityCandidates(queryRows []searchQueryRollup, decayRows []pageDecayRollup) []searchMetricOpportunityCandidate {
	candidates := []searchMetricOpportunityCandidate{}
	for _, row := range queryRows {
		if row.Impressions >= 100 && row.Position <= 8 && row.CTR <= 0.02 {
			candidates = append(candidates, searchMetricOpportunityCandidate{
				Type:              "gsc_low_ctr_query",
				Query:             row.Query,
				PageURL:           row.PageURL,
				NormalizedPageURL: row.NormalizedPageURL,
				PriorityScore:     clampScore(70 + row.Impressions/120),
				Confidence:        78,
				RecommendedAction: "Rewrite the title, meta description, and opening promise for the under-clicked query",
				ExpectedImpact:    "Improves capture from impressions CiteLoop can already see in Search Console.",
				Effort:            2,
				RiskLevel:         "low",
				Evidence:          searchQueryEvidence(row, "low_ctr"),
			})
		}
		if row.Impressions >= 250 && row.Position > 3 && row.Position <= 15 && row.CTR > 0.025 {
			candidates = append(candidates, searchMetricOpportunityCandidate{
				Type:              "gsc_query_gap",
				Query:             row.Query,
				PageURL:           row.PageURL,
				NormalizedPageURL: row.NormalizedPageURL,
				PriorityScore:     clampScore(69 + row.Impressions/140),
				Confidence:        74,
				RecommendedAction: "Expand the existing page or create a supporting section for the query intent",
				ExpectedImpact:    "Captures demand where Search Console shows relevance but the page is not yet the strongest answer.",
				Effort:            3,
				RiskLevel:         "medium",
				Evidence: withSearchOpportunityMetadata(
					searchQueryEvidence(row, "query_gap"),
					"query_gap = impressions>=250 + position 3-15 + CTR>2.5%",
					"medium",
					"Search Console shows this page already earns clicks for the query, but its average position suggests the answer is not strong enough yet.",
					[]string{"gsc_search_analytics", "query_data_partial"},
				),
			})
			continue
		}
		if row.Impressions >= 100 && row.Position > 8 && row.Position <= 20 {
			candidates = append(candidates, searchMetricOpportunityCandidate{
				Type:              "gsc_striking_distance_query",
				Query:             row.Query,
				PageURL:           row.PageURL,
				NormalizedPageURL: row.NormalizedPageURL,
				PriorityScore:     clampScore(68 + row.Impressions/150),
				Confidence:        72,
				RecommendedAction: "Strengthen the existing page for a query sitting within striking distance",
				ExpectedImpact:    "A focused refresh can move an already-visible query closer to page-one traffic.",
				Effort:            3,
				RiskLevel:         "medium",
				Evidence: withSearchOpportunityMetadata(
					searchQueryEvidence(row, "striking_distance"),
					"striking_distance = impressions>=100 + position 8-20",
					"medium",
					"Search Console shows meaningful impressions for a query ranking just outside high-click positions.",
					[]string{"gsc_search_analytics", "query_data_partial"},
				),
			})
		}
	}
	for _, row := range decayRows {
		if row.PreviousClicks < 10 || row.CurrentClicks >= row.PreviousClicks*0.7 {
			continue
		}
		dropRatio := round2((row.PreviousClicks - row.CurrentClicks) / row.PreviousClicks)
		candidates = append(candidates, searchMetricOpportunityCandidate{
			Type:              "gsc_content_decay",
			PageURL:           row.PageURL,
			NormalizedPageURL: row.NormalizedPageURL,
			PriorityScore:     clampScore(66 + dropRatio*30),
			Confidence:        70,
			RecommendedAction: "Refresh the decaying page before creating new content for the same demand",
			ExpectedImpact:    "Recovers existing search demand where CiteLoop can see a meaningful click drop.",
			Effort:            3,
			RiskLevel:         "medium",
			Evidence: withSearchOpportunityMetadata(map[string]any{
				"source":               "gsc_search_analytics",
				"reason":               "content_decay",
				"click_drop_ratio":     dropRatio,
				"current_clicks_28d":   round2(row.CurrentClicks),
				"previous_clicks_28d":  round2(row.PreviousClicks),
				"current_impressions":  round2(row.CurrentImpressions),
				"previous_impressions": round2(row.PreviousImpressions),
				"window_start":         dateOnly(row.WindowStart),
				"window_end":           dateOnly(row.WindowEnd),
			},
				"content_decay = previous_clicks>=10 + current_clicks<70% previous_clicks",
				"medium",
				"Search Console page totals show a meaningful click decline compared with the previous 28-day window.",
				[]string{"gsc_search_analytics", "page_total"},
			),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].PriorityScore > candidates[j].PriorityScore
	})
	return candidates
}

func searchQueryEvidence(row searchQueryRollup, reason string) map[string]any {
	evidence := map[string]any{
		"source":          "gsc_search_analytics",
		"reason":          reason,
		"clicks_28d":      round2(row.Clicks),
		"impressions_28d": round2(row.Impressions),
		"ctr_28d":         round4(row.CTR),
		"position_28d":    round2(row.Position),
		"window_start":    dateOnly(row.WindowStart),
		"window_end":      dateOnly(row.WindowEnd),
	}
	if reason == "low_ctr" {
		return withSearchOpportunityMetadata(
			evidence,
			"low_ctr = impressions>=100 + position<=8 + CTR<=2%",
			"low",
			"Search Console shows high visibility but weak click capture for this query.",
			[]string{"gsc_search_analytics", "query_data_partial"},
		)
	}
	return evidence
}

func withSearchOpportunityMetadata(evidence map[string]any, method, impactRange, whyNow string, notes []string) map[string]any {
	evidence["scoring_method"] = method
	evidence["scoring_version"] = searchOpportunityScoringVersion
	evidence["expected_impact_range"] = impactRange
	evidence["why_now"] = whyNow
	evidence["data_source_notes"] = notes
	return evidence
}

func clampScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 95 {
		return 95
	}
	return round2(score)
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func dateOnly(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}
