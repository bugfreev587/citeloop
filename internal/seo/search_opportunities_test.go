package seo

import (
	"testing"
	"time"
)

func TestSearchMetricOpportunityCandidatesUseGSCSignals(t *testing.T) {
	windowStart := time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	queryRows := []searchQueryRollup{
		{
			PageURL:           "https://example.com/guides/ai-citations",
			NormalizedPageURL: "/guides/ai-citations",
			Query:             "best ai citation tracker",
			Clicks:            12,
			Impressions:       1200,
			CTR:               0.01,
			Position:          4.2,
			WindowStart:       windowStart,
			WindowEnd:         windowEnd,
		},
		{
			PageURL:           "https://example.com/guides/chatgpt-citations",
			NormalizedPageURL: "/guides/chatgpt-citations",
			Query:             "chatgpt citation monitoring",
			Clicks:            18,
			Impressions:       900,
			CTR:               0.02,
			Position:          11.4,
			WindowStart:       windowStart,
			WindowEnd:         windowEnd,
		},
	}
	decayRows := []pageDecayRollup{
		{
			PageURL:             "https://example.com/blog/old-seo-playbook",
			NormalizedPageURL:   "/blog/old-seo-playbook",
			CurrentClicks:       18,
			PreviousClicks:      64,
			CurrentImpressions:  420,
			PreviousImpressions: 950,
			WindowStart:         windowStart,
			WindowEnd:           windowEnd,
		},
	}

	candidates := searchMetricOpportunityCandidates(queryRows, decayRows)

	if len(candidates) != 3 {
		t.Fatalf("candidate count = %d, want 3", len(candidates))
	}
	byType := map[string]searchMetricOpportunityCandidate{}
	for _, candidate := range candidates {
		byType[candidate.Type] = candidate
	}
	for _, typ := range []string{"gsc_low_ctr_query", "gsc_striking_distance_query", "gsc_content_decay"} {
		if _, ok := byType[typ]; !ok {
			t.Fatalf("missing candidate type %q in %#v", typ, candidates)
		}
	}
	lowCTR := byType["gsc_low_ctr_query"]
	if lowCTR.Query != "best ai citation tracker" {
		t.Fatalf("low CTR query = %q", lowCTR.Query)
	}
	if lowCTR.Evidence["source"] != "gsc_search_analytics" {
		t.Fatalf("low CTR source evidence = %v", lowCTR.Evidence["source"])
	}
	if lowCTR.Evidence["ctr_28d"] != 0.01 {
		t.Fatalf("low CTR evidence ctr = %v", lowCTR.Evidence["ctr_28d"])
	}
	if lowCTR.RecommendedAction == "" || lowCTR.ExpectedImpact == "" {
		t.Fatal("candidate should carry user-facing action and impact copy")
	}
	if byType["gsc_content_decay"].Evidence["click_drop_ratio"] != 0.72 {
		t.Fatalf("decay drop ratio = %v", byType["gsc_content_decay"].Evidence["click_drop_ratio"])
	}
}
