package seo

import (
	"testing"
	"time"
)

func TestActionableSEOOpportunityCandidatesUseTechnicalInventoryAndQuerySignals(t *testing.T) {
	windowStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
	checks := []technicalCheckRollup{
		{
			PageURL:              "https://example.com/product",
			NormalizedPageURL:    "/product",
			HTTPStatus:           int32PtrSEO(200),
			StructuredDataStatus: "missing",
			InternalLinkCount:    int32PtrSEO(0),
			RawDetails:           map[string]any{"body_bytes": 12000},
		},
		{
			PageURL:           "https://example.com/docs/api",
			NormalizedPageURL: "/docs/api",
			HTTPStatus:        int32PtrSEO(200),
			RobotsStatus:      "noindex",
			InternalLinkCount: int32PtrSEO(4),
			RawDetails:        map[string]any{"body_bytes": 32000},
		},
	}
	inventory := []inventoryEvidenceRollup{
		{
			URL:               "https://example.com/product",
			NormalizedURL:     "/product",
			Title:             "Product",
			Summary:           "Short landing page",
			EvidenceCount:     0,
			SummaryWordCount:  3,
			CapturedEvidence:  []string{},
			PrimarySourceType: "existing",
		},
	}
	queryRows := []searchQueryRollup{
		{
			PageURL:           "https://example.com/blog/ai-seo",
			NormalizedPageURL: "/blog/ai-seo",
			Query:             "ai seo workflow",
			Clicks:            24,
			Impressions:       700,
			CTR:               0.034,
			Position:          8.2,
			WindowStart:       windowStart,
			WindowEnd:         windowEnd,
		},
		{
			PageURL:           "https://example.com/guides/ai-seo",
			NormalizedPageURL: "/guides/ai-seo",
			Query:             "ai seo workflow",
			Clicks:            18,
			Impressions:       560,
			CTR:               0.032,
			Position:          9.1,
			WindowStart:       windowStart,
			WindowEnd:         windowEnd,
		},
	}

	candidates := actionableSEOOpportunityCandidates(checks, inventory, queryRows)

	requireCandidateTypes(t, candidates,
		"internal_link_gap",
		"schema_gap",
		"thin_evidence_page",
		"technical_visibility_issue",
		"gsc_query_cannibalization",
	)
	for _, candidate := range candidates {
		if candidate.RecommendedAction == "" || candidate.ExpectedImpact == "" {
			t.Fatalf("%s should carry user-facing action and impact copy: %#v", candidate.Type, candidate)
		}
		if candidate.Confidence <= 0 || candidate.Effort <= 0 || candidate.RiskLevel == "" {
			t.Fatalf("%s should carry confidence, effort, and risk: %#v", candidate.Type, candidate)
		}
		for _, key := range []string{"source", "why_now", "scoring_method", "scoring_version", "expected_impact_range", "idempotency_key", "reason"} {
			if candidate.Evidence[key] == nil || candidate.Evidence[key] == "" {
				t.Fatalf("%s evidence missing %q in %#v", candidate.Type, key, candidate.Evidence)
			}
		}
	}
	if byType := candidatesByType(candidates); byType["internal_link_gap"].NormalizedPageURL != "/product" {
		t.Fatalf("internal link gap page = %q, want /product", byType["internal_link_gap"].NormalizedPageURL)
	}
}

func TestActionableSEOOpportunityCandidatesStaySilentWithoutRequiredSignals(t *testing.T) {
	checks := []technicalCheckRollup{
		{
			PageURL:              "https://example.com/product",
			NormalizedPageURL:    "/product",
			HTTPStatus:           int32PtrSEO(200),
			RobotsStatus:         "present",
			CanonicalStatus:      "present",
			StructuredDataStatus: "present",
			InternalLinkCount:    int32PtrSEO(5),
			RawDetails:           map[string]any{"body_bytes": 64000},
		},
	}
	inventory := []inventoryEvidenceRollup{
		{
			URL:               "https://example.com/product",
			NormalizedURL:     "/product",
			Title:             "Product",
			Summary:           "A page with enough supporting evidence for answer engines.",
			EvidenceCount:     4,
			SummaryWordCount:  9,
			CapturedEvidence:  []string{"fact one", "fact two", "fact three", "fact four"},
			PrimarySourceType: "existing",
		},
	}
	queryRows := []searchQueryRollup{
		{
			PageURL:           "https://example.com/product",
			NormalizedPageURL: "/product",
			Query:             "ai seo workflow",
			Impressions:       420,
			Position:          7.8,
		},
	}

	candidates := actionableSEOOpportunityCandidates(checks, inventory, queryRows)

	if len(candidates) != 0 {
		t.Fatalf("candidate count = %d, want 0: %#v", len(candidates), candidates)
	}
}

func TestActionableSEOOpportunityAnalyzersRequireTheirSupportingData(t *testing.T) {
	t.Run("internal link gap requires low internal link count", func(t *testing.T) {
		candidates := actionableSEOOpportunityCandidates([]technicalCheckRollup{{
			PageURL:           "https://example.com/product",
			NormalizedPageURL: "/product",
			InternalLinkCount: int32PtrSEO(3),
		}}, nil, nil)
		if hasCandidateType(candidates, "internal_link_gap") {
			t.Fatalf("internal link gap should not appear without low internal link count: %#v", candidates)
		}
	})
	t.Run("schema gap requires missing structured data", func(t *testing.T) {
		candidates := actionableSEOOpportunityCandidates([]technicalCheckRollup{{
			PageURL:              "https://example.com/product",
			NormalizedPageURL:    "/product",
			StructuredDataStatus: "present",
		}}, nil, nil)
		if hasCandidateType(candidates, "schema_gap") {
			t.Fatalf("schema gap should not appear when structured data is present: %#v", candidates)
		}
	})
	t.Run("thin evidence page requires weak inventory evidence", func(t *testing.T) {
		candidates := actionableSEOOpportunityCandidates(nil, []inventoryEvidenceRollup{{
			URL:              "https://example.com/product",
			NormalizedURL:    "/product",
			EvidenceCount:    3,
			SummaryWordCount: 120,
		}}, nil)
		if hasCandidateType(candidates, "thin_evidence_page") {
			t.Fatalf("thin evidence page should not appear with enough evidence: %#v", candidates)
		}
	})
	t.Run("technical visibility issue requires a blocker", func(t *testing.T) {
		candidates := actionableSEOOpportunityCandidates([]technicalCheckRollup{{
			PageURL:           "https://example.com/product",
			NormalizedPageURL: "/product",
			HTTPStatus:        int32PtrSEO(200),
			RobotsStatus:      "present",
			CanonicalStatus:   "present",
		}}, nil, nil)
		if hasCandidateType(candidates, "technical_visibility_issue") {
			t.Fatalf("technical visibility issue should not appear without a blocker: %#v", candidates)
		}
	})
	t.Run("cannibalization requires multiple competing pages", func(t *testing.T) {
		candidates := actionableSEOOpportunityCandidates(nil, nil, []searchQueryRollup{{
			PageURL:           "https://example.com/product",
			NormalizedPageURL: "/product",
			Query:             "ai seo workflow",
			Impressions:       700,
			Position:          8.2,
		}})
		if hasCandidateType(candidates, "gsc_query_cannibalization") {
			t.Fatalf("cannibalization should not appear with one page for the query: %#v", candidates)
		}
	})
}

func candidatesByType(candidates []actionableSEOOpportunityCandidate) map[string]actionableSEOOpportunityCandidate {
	out := map[string]actionableSEOOpportunityCandidate{}
	for _, candidate := range candidates {
		out[candidate.Type] = candidate
	}
	return out
}

func requireCandidateTypes(t *testing.T, candidates []actionableSEOOpportunityCandidate, types ...string) {
	t.Helper()
	byType := candidatesByType(candidates)
	for _, typ := range types {
		if _, ok := byType[typ]; !ok {
			t.Fatalf("missing candidate type %q in %#v", typ, candidates)
		}
	}
}

func hasCandidateType(candidates []actionableSEOOpportunityCandidate, typ string) bool {
	_, ok := candidatesByType(candidates)[typ]
	return ok
}

func int32PtrSEO(value int32) *int32 {
	return &value
}
