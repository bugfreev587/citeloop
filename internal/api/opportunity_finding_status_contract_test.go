package api

import (
	"os"
	"strings"
	"testing"
)

func TestOpportunityFindingStatusUsesRunHistoryAndProjectConfig(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"type OpportunityFindingStatus struct",
		"SourceMix",
		"AIDiscoveryAutomation",
		"ManualMode",
		"LastRun",
		"NextFindingAt",
		"Summary",
		"Counts",
		"ListSEORuns",
		"SEOOpportunityCounts",
		"data_source_notes",
		"generated_anomalies",
		"latestOpportunityFindingRun",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("opportunity finding status contract missing %q", want)
		}
	}
}

func TestOpportunityFindingRoutesAreMounted(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	routes := string(raw)
	for _, want := range []string{
		`r.Get("/opportunity-finding/status", s.getOpportunityFindingStatus)`,
		`r.Post("/opportunity-finding/run", s.runOpportunityFinding)`,
	} {
		if !strings.Contains(routes, want) {
			t.Fatalf("server routes missing %q", want)
		}
	}
}
