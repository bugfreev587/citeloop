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

func TestRunOpportunityFindingIncludesAIDiscoveryStage(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(raw)
	body := functionBody(t, source, "func (s *Server) runOpportunityFinding")
	for _, want := range []string{
		"OpportunityFindingStages(false)",
		"opportunityfinding.RunAIDiscovery",
		`"ai_discovery"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("runOpportunityFinding must include AI Discovery stage; missing %q", want)
		}
	}
}

func functionBody(t *testing.T, source, marker string) string {
	t.Helper()
	start := strings.Index(source, marker)
	if start == -1 {
		t.Fatalf("missing %s", marker)
	}
	open := strings.Index(source[start:], "{")
	if open == -1 {
		t.Fatalf("missing opening brace for %s", marker)
	}
	pos := start + open
	depth := 0
	for i := pos; i < len(source); i++ {
		switch source[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[pos+1 : i]
			}
		}
	}
	t.Fatalf("missing closing brace for %s", marker)
	return ""
}
