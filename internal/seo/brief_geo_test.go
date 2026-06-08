package seo

import (
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

func TestBriefGEOSectionsSeparatesBlockersAndTopOpportunities(t *testing.T) {
	blockedURL := "https://example.com/blocked"
	query := "best social scheduling tools"
	opps := []db.SeoOpportunity{
		{
			ID:        uuid.New(),
			Type:      "geo_crawler_access_blocked",
			PageUrl:   &blockedURL,
			RiskLevel: "high",
		},
		{
			ID:        uuid.New(),
			Type:      "geo_competitor_cited_project_absent",
			Query:     &query,
			RiskLevel: "medium",
		},
		{
			ID:        uuid.New(),
			Type:      "indexing_anomaly",
			RiskLevel: "low",
		},
	}

	blockers, geoOpps := briefGEOSections(opps)

	if len(blockers) != 1 || !strings.Contains(blockers[0], blockedURL) {
		t.Fatalf("blockers = %+v, want blocked URL", blockers)
	}
	if len(geoOpps) != 1 || geoOpps[0].Type != "geo_competitor_cited_project_absent" {
		t.Fatalf("geo opportunities = %+v, want competitor cited gap", geoOpps)
	}
}
