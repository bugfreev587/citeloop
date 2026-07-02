package seo

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const actionableSEOScoringVersion = "seo_actionable_v1"

type technicalCheckRollup struct {
	PageURL               string
	NormalizedPageURL     string
	HTTPStatus            *int32
	CanonicalStatus       string
	RobotsStatus          string
	TitleStatus           string
	MetaDescriptionStatus string
	H1Status              string
	StructuredDataStatus  string
	SitemapStatus         string
	InternalLinkCount     *int32
	OutboundLinkCount     *int32
	RawDetails            map[string]any
}

type inventoryEvidenceRollup struct {
	URL               string
	NormalizedURL     string
	Title             string
	Summary           string
	EvidenceCount     int
	SummaryWordCount  int
	CapturedEvidence  []string
	PrimarySourceType string
}

type actionableSEOOpportunityCandidate struct {
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

func actionableSEOOpportunityCandidates(
	checks []technicalCheckRollup,
	inventory []inventoryEvidenceRollup,
	queryRows []searchQueryRollup,
) []actionableSEOOpportunityCandidate {
	candidates := make([]actionableSEOOpportunityCandidate, 0)
	for _, check := range checks {
		if normalized := strings.TrimSpace(check.NormalizedPageURL); normalized != "" {
			if check.InternalLinkCount != nil && *check.InternalLinkCount <= 1 {
				candidates = append(candidates, internalLinkGapCandidate(check))
			}
			if strings.EqualFold(strings.TrimSpace(check.StructuredDataStatus), "missing") {
				candidates = append(candidates, schemaGapCandidate(check))
			}
			if issue := technicalVisibilityIssue(check); issue != "" {
				candidates = append(candidates, technicalVisibilityCandidate(check, issue))
			}
		}
	}
	for _, item := range inventory {
		if strings.TrimSpace(item.NormalizedURL) == "" && strings.TrimSpace(item.URL) == "" {
			continue
		}
		if item.EvidenceCount <= 1 && item.SummaryWordCount < 80 {
			candidates = append(candidates, thinEvidenceCandidate(item))
		}
	}
	candidates = append(candidates, cannibalizationCandidates(queryRows)...)
	return dedupeAndSortActionableCandidates(candidates)
}

func internalLinkGapCandidate(check technicalCheckRollup) actionableSEOOpportunityCandidate {
	count := int32(0)
	if check.InternalLinkCount != nil {
		count = *check.InternalLinkCount
	}
	return actionableSEOOpportunityCandidate{
		Type:              "internal_link_gap",
		PageURL:           check.PageURL,
		NormalizedPageURL: check.NormalizedPageURL,
		PriorityScore:     clampScore(72 - float64(count)*4),
		Confidence:        76,
		RecommendedAction: "Add contextual internal links to this existing page",
		ExpectedImpact:    "Improves crawl paths, topical authority flow, and answer-engine context without publishing a new page.",
		Effort:            2,
		RiskLevel:         "low",
		Evidence: actionableEvidence("technical_checks", "internal_link_gap", check.NormalizedPageURL, "",
			"internal_link_gap = latest crawl internal_link_count<=1",
			"low",
			"The latest crawl found too few internal links pointing through this page, so crawlers and answer engines may not understand its cluster role.",
			[]string{"technical_checks", "public_crawl"},
			map[string]any{
				"internal_link_count": count,
				"page_url":            check.PageURL,
			},
		),
	}
}

func schemaGapCandidate(check technicalCheckRollup) actionableSEOOpportunityCandidate {
	return actionableSEOOpportunityCandidate{
		Type:              "schema_gap",
		PageURL:           check.PageURL,
		NormalizedPageURL: check.NormalizedPageURL,
		PriorityScore:     68,
		Confidence:        72,
		RecommendedAction: "Add structured data to the existing page",
		ExpectedImpact:    "Makes page entities, facts, and answers easier for search engines and answer engines to parse.",
		Effort:            2,
		RiskLevel:         "medium",
		Evidence: actionableEvidence("technical_checks", "schema_gap", check.NormalizedPageURL, "",
			"schema_gap = latest crawl structured_data_status=missing",
			"medium",
			"The latest crawl did not find JSON-LD or structured data on a page that can benefit from extractable facts.",
			[]string{"technical_checks", "structured_data_status"},
			map[string]any{
				"structured_data_status": check.StructuredDataStatus,
				"page_url":               check.PageURL,
			},
		),
	}
}

func technicalVisibilityCandidate(check technicalCheckRollup, issue string) actionableSEOOpportunityCandidate {
	return actionableSEOOpportunityCandidate{
		Type:              "technical_visibility_issue",
		PageURL:           check.PageURL,
		NormalizedPageURL: check.NormalizedPageURL,
		PriorityScore:     84,
		Confidence:        82,
		RecommendedAction: "Fix the technical visibility blocker before creating more content",
		ExpectedImpact:    "Restores crawl/index reliability so SEO and GEO improvements can be measured on the affected page.",
		Effort:            2,
		RiskLevel:         "medium",
		Evidence: actionableEvidence("technical_checks", "technical_visibility_issue:"+issue, check.NormalizedPageURL, "",
			"technical_visibility_issue = latest crawl shows http, robots, or canonical blocker",
			"medium",
			"The latest crawl found a technical state that can block indexing or make visibility measurement unreliable.",
			[]string{"technical_checks", "crawl_blocker"},
			map[string]any{
				"issue":            issue,
				"http_status":      check.HTTPStatus,
				"robots_status":    check.RobotsStatus,
				"canonical_status": check.CanonicalStatus,
				"page_url":         check.PageURL,
			},
		),
	}
}

func thinEvidenceCandidate(item inventoryEvidenceRollup) actionableSEOOpportunityCandidate {
	normalized := strings.TrimSpace(item.NormalizedURL)
	if normalized == "" {
		normalized = strings.TrimSpace(item.URL)
	}
	return actionableSEOOpportunityCandidate{
		Type:              "thin_evidence_page",
		PageURL:           item.URL,
		NormalizedPageURL: normalized,
		PriorityScore:     64,
		Confidence:        68,
		RecommendedAction: "Strengthen the evidence block on this existing page",
		ExpectedImpact:    "Adds extractable proof so search engines and answer engines have a source-backed page to cite.",
		Effort:            3,
		RiskLevel:         "medium",
		Evidence: actionableEvidence("content_inventory", "thin_evidence_page", normalized, "",
			"thin_evidence_page = evidence_snippets<=1 + summary_word_count<80",
			"medium",
			"The inventory has too little supporting evidence for the page to be a strong answer-engine source.",
			[]string{"content_inventory", "evidence_snippets"},
			map[string]any{
				"evidence_count":      item.EvidenceCount,
				"summary_word_count":  item.SummaryWordCount,
				"captured_evidence":   item.CapturedEvidence,
				"primary_source_type": item.PrimarySourceType,
			},
		),
	}
}

func cannibalizationCandidates(rows []searchQueryRollup) []actionableSEOOpportunityCandidate {
	byQuery := map[string][]searchQueryRollup{}
	for _, row := range rows {
		query := strings.ToLower(strings.TrimSpace(row.Query))
		if query == "" || strings.TrimSpace(row.NormalizedPageURL) == "" || row.Impressions < 100 {
			continue
		}
		byQuery[query] = append(byQuery[query], row)
	}
	out := make([]actionableSEOOpportunityCandidate, 0)
	for query, group := range byQuery {
		pageSeen := map[string]bool{}
		distinct := make([]searchQueryRollup, 0, len(group))
		for _, row := range group {
			if pageSeen[row.NormalizedPageURL] {
				continue
			}
			pageSeen[row.NormalizedPageURL] = true
			distinct = append(distinct, row)
		}
		if len(distinct) < 2 {
			continue
		}
		sort.SliceStable(distinct, func(i, j int) bool {
			return distinct[i].Impressions > distinct[j].Impressions
		})
		top := distinct[0]
		second := distinct[1]
		if top.Position > 15 || second.Position > 20 || absFloat(top.Position-second.Position) > 5 {
			continue
		}
		totalImpressions := 0.0
		pages := make([]map[string]any, 0, len(distinct))
		for _, row := range distinct {
			totalImpressions += row.Impressions
			pages = append(pages, map[string]any{
				"page_url":            row.PageURL,
				"normalized_page_url": row.NormalizedPageURL,
				"impressions_28d":     round2(row.Impressions),
				"position_28d":        round2(row.Position),
			})
		}
		if totalImpressions < 500 {
			continue
		}
		out = append(out, actionableSEOOpportunityCandidate{
			Type:              "gsc_query_cannibalization",
			Query:             top.Query,
			PageURL:           top.PageURL,
			NormalizedPageURL: top.NormalizedPageURL,
			PriorityScore:     clampScore(70 + totalImpressions/180),
			Confidence:        70,
			RecommendedAction: "Consolidate competing pages for the same query",
			ExpectedImpact:    "Clarifies the preferred page for a query where Search Console shows multiple URLs splitting the same demand.",
			Effort:            3,
			RiskLevel:         "medium",
			Evidence: actionableEvidence("gsc_search_analytics", "gsc_query_cannibalization", top.NormalizedPageURL, query,
				"cannibalization = same query has 2+ URLs with impressions>=100, close positions, total impressions>=500",
				"medium",
				"Search Console shows multiple pages competing for the same query in the same window.",
				[]string{"gsc_search_analytics", "query_data_partial"},
				map[string]any{
					"query":             top.Query,
					"total_impressions": round2(totalImpressions),
					"competing_pages":   pages,
					"window_start":      dateOnly(top.WindowStart),
					"window_end":        dateOnly(top.WindowEnd),
				},
			),
		})
	}
	return out
}

func technicalVisibilityIssue(check technicalCheckRollup) string {
	if check.HTTPStatus != nil && (*check.HTTPStatus < 200 || *check.HTTPStatus >= 300) {
		return "http_status"
	}
	robots := strings.ToLower(strings.TrimSpace(check.RobotsStatus))
	if robots == "noindex" || robots == "disallowed" || robots == "blocked" {
		return "robots_" + robots
	}
	if strings.EqualFold(strings.TrimSpace(check.CanonicalStatus), "missing") {
		return "canonical_missing"
	}
	return ""
}

func actionableEvidence(source, reason, normalizedPageURL, query, method, impactRange, whyNow string, notes []string, extra map[string]any) map[string]any {
	evidence := map[string]any{
		"source":                source,
		"reason":                reason,
		"normalized_page_url":   normalizedPageURL,
		"scoring_method":        method,
		"scoring_version":       actionableSEOScoringVersion,
		"expected_impact_range": impactRange,
		"why_now":               whyNow,
		"data_source_notes":     notes,
		"idempotency_key":       idempotencyKey(source, reason, normalizedPageURL, query),
	}
	if strings.TrimSpace(query) != "" {
		evidence["query"] = query
	}
	for key, value := range extra {
		evidence[key] = value
	}
	return evidence
}

func dedupeAndSortActionableCandidates(candidates []actionableSEOOpportunityCandidate) []actionableSEOOpportunityCandidate {
	seen := map[string]bool{}
	out := make([]actionableSEOOpportunityCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		key := fmt.Sprint(candidate.Evidence["idempotency_key"])
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].PriorityScore > out[j].PriorityScore
	})
	return out
}

func idempotencyKey(source, reason, normalizedPageURL, query string) string {
	return strings.Join([]string{
		strings.TrimSpace(source),
		strings.TrimSpace(reason),
		strings.TrimSpace(normalizedPageURL),
		strings.ToLower(strings.TrimSpace(query)),
	}, "|")
}

func evidenceSnippets(raw json.RawMessage) []string {
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		return values
	}
	return nil
}

func wordCount(value string) int {
	return len(strings.Fields(value))
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
