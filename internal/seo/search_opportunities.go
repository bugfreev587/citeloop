package seo

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/learning"
)

func applySearchLearningScores(ctx context.Context, candidates []searchMetricOpportunityCandidate, scorer learning.CandidateScorer) ([]searchMetricOpportunityCandidate, error) {
	for i := range candidates {
		candidate := &candidates[i]
		result, err := scorer.ScoreCandidate(ctx, learning.CandidateContext(
			candidate.PriorityScore, candidate.Type, candidate.NormalizedPageURL, candidate.Query, candidate.Evidence,
		))
		if err != nil {
			return nil, err
		}
		if len(result.LearningIDs) == 0 {
			continue
		}
		candidate.PriorityScore = result.AdjustedScore
		if candidate.Evidence == nil {
			candidate.Evidence = map[string]any{}
		}
		candidate.Evidence["learning_scoring"] = result.Provenance()
	}
	return candidates, nil
}

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
	ObservedMetadata  map[string]any
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
			priority := clampScore(70 + row.Impressions/120)
			confidence := 78.0
			evidence := searchQueryEvidence(row, "low_ctr")
			enrichLowCTRMetadataEvidence(evidence, row, priority, confidence)
			candidates = append(candidates, searchMetricOpportunityCandidate{
				Type:              "gsc_low_ctr_query",
				Query:             row.Query,
				PageURL:           row.PageURL,
				NormalizedPageURL: row.NormalizedPageURL,
				PriorityScore:     priority,
				Confidence:        confidence,
				RecommendedAction: "Rewrite the title, meta description, and opening promise for the under-clicked query",
				ExpectedImpact:    "Improves capture from impressions CiteLoop can already see in Search Console.",
				Effort:            2,
				RiskLevel:         "low",
				Evidence:          evidence,
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

func enrichLowCTRMetadataEvidence(evidence map[string]any, row searchQueryRollup, priority, confidence float64) {
	if observed := metadataObservedForSearchRow(row); len(observed) > 0 {
		evidence["observed"] = observed
	}
	evidence["opportunity"] = map[string]any{
		"query":          strings.TrimSpace(row.Query),
		"intent":         queryIntentForMetadataRewrite(row.Query, row.ObservedMetadata),
		"problem_detail": "Search Console shows high visibility but weak click-through; rewrite the title and meta description for clearer snippet relevance before changing page scope.",
		"confidence":     round2(confidence / 100),
		"priority":       priorityLabel(priority),
	}
	if proposed := proposedMetadataForSearchRow(row); len(proposed) > 0 {
		evidence["proposed_change"] = proposed
	}
}

func metadataObservedForSearchRow(row searchQueryRollup) map[string]any {
	out := map[string]any{}
	for _, field := range []string{"status", "title", "meta_description", "canonical", "robots", "observed_at"} {
		if value := stringishMetadataValue(row.ObservedMetadata[field]); value != "" {
			out[field] = value
		}
	}
	if value, ok := row.ObservedMetadata["status"].(int); ok {
		out["status"] = value
	}
	if value, ok := row.ObservedMetadata["status"].(int32); ok {
		out["status"] = value
	}
	if value, ok := row.ObservedMetadata["status"].(float64); ok {
		out["status"] = value
	}
	return out
}

func proposedMetadataForSearchRow(row searchQueryRollup) map[string]any {
	query := strings.TrimSpace(row.Query)
	if query == "" {
		return nil
	}
	currentTitle := stringishMetadataValue(row.ObservedMetadata["title"])
	currentDescription := stringishMetadataValue(row.ObservedMetadata["meta_description"])
	title := proposedMetadataTitle(query, currentTitle, row.PageURL)
	description := proposedMetadataDescription(query, currentDescription)
	return map[string]any{
		"title":                    title,
		"meta_description":         description,
		"seo_impact":               "search snippet relevance / CTR",
		"geo_impact":               "entity clarity / product category clarity",
		"content_support_required": false,
		"proposal_source":          "heuristic_from_gsc_low_ctr_and_current_metadata",
		"preserve":                 []string{"canonical", "indexability", "production URL"},
	}
}

func proposedMetadataTitle(query, currentTitle, pageURL string) string {
	queryTitle := titleCaseWords(query)
	brand := brandFromTitle(currentTitle)
	if brand == "" {
		brand = brandFromURL(pageURL)
	}
	if brand != "" && strings.EqualFold(strings.TrimSpace(query), brand) {
		if descriptor := descriptorFromTitle(currentTitle); descriptor != "" {
			return clipWithEllipsis(brand+" | "+descriptor, 60)
		}
		return clipWithEllipsis(brand+" | "+queryTitle+" overview", 60)
	}
	if brand != "" && !strings.EqualFold(brand, queryTitle) {
		return clipWithEllipsis(queryTitle+" | "+brand, 60)
	}
	if strings.TrimSpace(currentTitle) != "" {
		return clipWithEllipsis(strings.TrimSpace(currentTitle), 60)
	}
	return clipWithEllipsis(queryTitle, 60)
}

func proposedMetadataDescription(query, currentDescription string) string {
	query = strings.TrimSpace(query)
	currentDescription = strings.TrimSpace(currentDescription)
	if currentDescription != "" {
		if strings.Contains(strings.ToLower(currentDescription), strings.ToLower(query)) {
			return clipWithEllipsis(currentDescription, 155)
		}
		return clipWithEllipsis("Explore "+query+": "+lowercaseFirst(currentDescription), 155)
	}
	return clipWithEllipsis("Explore "+query+" with clear product details, use cases, and next steps on this page.", 155)
}

func queryIntentForMetadataRewrite(query string, observed map[string]any) string {
	brand := brandFromTitle(stringishMetadataValue(observed["title"]))
	if brand != "" && strings.Contains(strings.ToLower(query), strings.ToLower(brand)) {
		return "branded + product category"
	}
	return "search snippet relevance / low CTR"
}

func priorityLabel(score float64) string {
	switch {
	case score >= 75:
		return "high"
	case score >= 50:
		return "medium"
	default:
		return "low"
	}
}

func brandFromTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	for _, separator := range []string{"|", " - ", ":"} {
		if parts := strings.Split(title, separator); len(parts) > 1 {
			return strings.TrimSpace(parts[0])
		}
	}
	return ""
}

func descriptorFromTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	for _, separator := range []string{"|", " - ", ":"} {
		parts := strings.Split(title, separator)
		if len(parts) > 1 {
			return strings.TrimSpace(strings.Join(parts[1:], separator))
		}
	}
	return ""
}

func brandFromURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimPrefix(strings.TrimPrefix(trimmed, "https://"), "http://")
	host := strings.Split(trimmed, "/")[0]
	host = strings.TrimPrefix(host, "www.")
	if host == "" {
		return ""
	}
	return strings.Split(host, ".")[0]
}

func titleCaseWords(value string) string {
	words := strings.Fields(strings.TrimSpace(value))
	for i, word := range words {
		if word == strings.ToUpper(word) {
			continue
		}
		lower := strings.ToLower(word)
		switch lower {
		case "ai", "api", "seo", "geo", "gsc", "ctr", "llm":
			words[i] = strings.ToUpper(lower)
			continue
		}
		words[i] = strings.ToUpper(lower[:1]) + lower[1:]
	}
	return strings.Join(words, " ")
}

func lowercaseFirst(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToLower(value[:1]) + value[1:]
}

func clipWithEllipsis(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return strings.TrimSpace(value[:limit-3]) + "..."
}

func stringishMetadataValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
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
