package seo

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/jackc/pgx/v5/pgtype"
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
			ObservedMetadata: map[string]any{
				"status":           200,
				"title":            "CiteLoop | AI SEO Control Center",
				"meta_description": "Run AI SEO workflows from one control center.",
				"canonical":        "https://example.com/guides/ai-citations",
				"robots":           "indexable",
				"observed_at":      "2026-07-08",
			},
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
		{
			PageURL:           "https://example.com/guides/source-backed-seo",
			NormalizedPageURL: "/guides/source-backed-seo",
			Query:             "source backed seo workflow",
			Clicks:            28,
			Impressions:       700,
			CTR:               0.04,
			Position:          9.8,
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

	if len(candidates) != 4 {
		t.Fatalf("candidate count = %d, want 4", len(candidates))
	}
	byType := map[string]searchMetricOpportunityCandidate{}
	for _, candidate := range candidates {
		byType[candidate.Type] = candidate
	}
	for _, typ := range []string{"gsc_low_ctr_query", "gsc_striking_distance_query", "gsc_query_gap", "gsc_content_decay"} {
		if _, ok := byType[typ]; !ok {
			t.Fatalf("missing candidate type %q in %#v", typ, candidates)
		}
	}
	for _, candidate := range candidates {
		for _, key := range []string{"scoring_method", "scoring_version", "expected_impact_range", "why_now"} {
			if candidate.Evidence[key] == nil || candidate.Evidence[key] == "" {
				t.Fatalf("%s evidence missing %q in %#v", candidate.Type, key, candidate.Evidence)
			}
		}
		if notes, ok := candidate.Evidence["data_source_notes"].([]string); !ok || len(notes) == 0 {
			t.Fatalf("%s evidence missing data source notes in %#v", candidate.Type, candidate.Evidence)
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
	observed, ok := lowCTR.Evidence["observed"].(map[string]any)
	if !ok {
		t.Fatalf("low CTR metadata rewrite should carry observed metadata, got %#v", lowCTR.Evidence)
	}
	if observed["title"] != "CiteLoop | AI SEO Control Center" || observed["meta_description"] != "Run AI SEO workflows from one control center." || observed["canonical"] != "https://example.com/guides/ai-citations" {
		t.Fatalf("low CTR observed metadata not preserved: %#v", observed)
	}
	opportunity, ok := lowCTR.Evidence["opportunity"].(map[string]any)
	if !ok || opportunity["intent"] == "" || opportunity["problem_detail"] == "" || opportunity["confidence"] == nil || opportunity["priority"] == nil {
		t.Fatalf("low CTR metadata rewrite should carry intent/problem/confidence/priority, got %#v", lowCTR.Evidence["opportunity"])
	}
	proposed, ok := lowCTR.Evidence["proposed_change"].(map[string]any)
	if !ok {
		t.Fatalf("low CTR metadata rewrite should carry proposed title/meta, got %#v", lowCTR.Evidence)
	}
	if proposed["title"] == "" || proposed["meta_description"] == "" {
		t.Fatalf("low CTR proposed metadata should include concrete title and meta description, got %#v", proposed)
	}
	if proposed["seo_impact"] == "" || proposed["geo_impact"] == "" || proposed["content_support_required"] != false {
		t.Fatalf("low CTR proposed metadata should distinguish SEO/GEO impact and content support, got %#v", proposed)
	}
	if lowCTR.RecommendedAction == "" || lowCTR.ExpectedImpact == "" {
		t.Fatal("candidate should carry user-facing action and impact copy")
	}
	if byType["gsc_content_decay"].Evidence["click_drop_ratio"] != 0.72 {
		t.Fatalf("decay drop ratio = %v", byType["gsc_content_decay"].Evidence["click_drop_ratio"])
	}
}

func TestSearchQueryRollupsCarryObservedMetadataFromTechnicalChecks(t *testing.T) {
	status := int32(200)
	robots := "indexable"
	checkedAt := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	windowStart := time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)

	observedByURL := observedMetadataByNormalizedURL([]db.TechnicalCheck{
		{
			PageUrl:           "https://example.com/",
			NormalizedPageUrl: "/",
			HttpStatus:        &status,
			RobotsStatus:      &robots,
			RawDetails: json.RawMessage(`{
				"page_title": "Example | Product API",
				"meta_description": "Ship product workflows through one API.",
				"canonical_url": "https://example.com/"
			}`),
			CheckedAt: pgtype.Timestamptz{Time: checkedAt, Valid: true},
		},
	})

	rollups := toSearchQueryRollups([]db.ListSearchQueryOpportunityRollupsRow{
		{
			PageUrl:           "https://example.com/",
			NormalizedPageUrl: "/",
			Query:             "example api",
			Clicks28d:         pgutil.Numeric(3),
			Impressions28d:    pgutil.Numeric(500),
			Ctr28d:            pgutil.Numeric(0.006),
			Position28d:       pgutil.Numeric(3.8),
			WindowStart:       pgtype.Date{Time: windowStart, Valid: true},
			WindowEnd:         pgtype.Date{Time: windowEnd, Valid: true},
		},
	}, observedByURL)

	if len(rollups) != 1 {
		t.Fatalf("rollup count = %d, want 1", len(rollups))
	}
	observed := rollups[0].ObservedMetadata
	if observed["status"] != 200 || observed["title"] != "Example | Product API" || observed["meta_description"] != "Ship product workflows through one API." {
		t.Fatalf("observed metadata did not carry from technical checks: %#v", observed)
	}
	if observed["canonical"] != "https://example.com/" || observed["robots"] != "indexable" || observed["observed_at"] != "2026-07-08" {
		t.Fatalf("observed metadata should preserve canonical, indexability, and timestamp: %#v", observed)
	}

	observed["title"] = "mutated"
	if observedByURL["/"]["title"] == "mutated" {
		t.Fatal("rollups should copy observed metadata instead of sharing the source map")
	}
}

func TestProposedMetadataTitleUsesQueryAndExistingBrandContext(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		currentTitle string
		pageURL      string
		want         string
	}{
		{
			name:         "branded query keeps brand and reuses title descriptor",
			query:        "unipost",
			currentTitle: "UniPost - Social publishing for teams",
			pageURL:      "https://unipost.dev/",
			want:         "UniPost | Social publishing for teams",
		},
		{
			name:         "category query leads with query and preserves brand",
			query:        "best ai citation tracker",
			currentTitle: "CiteLoop | AI SEO Control Center",
			pageURL:      "https://citeloop.ai/guides/ai-citations",
			want:         "Best AI Citation Tracker | CiteLoop",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := proposedMetadataTitle(tt.query, tt.currentTitle, tt.pageURL); got != tt.want {
				t.Fatalf("proposed metadata title = %q, want %q", got, tt.want)
			}
		})
	}
}
