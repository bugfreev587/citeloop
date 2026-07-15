package scheduler

import (
	"testing"

	"github.com/citeloop/citeloop/internal/opportunityfinding"
)

func TestAIDiscoveryEvidenceSubstepsExposeDurationsAndMergeShortAudits(t *testing.T) {
	result := opportunityfinding.AIDiscoveryResult{Steps: []opportunityfinding.AIDiscoveryStep{
		{Name: "plan_candidates", Status: "ok", Count: 4, DurationMs: 11},
		{Name: "search_evidence", Status: "ok", Count: 745, DurationMs: 2000},
		{Name: "crawler_audit", Status: "ok", Count: 2, DurationMs: 30},
		{Name: "external_surfaces", Status: "ok", Count: 1, DurationMs: 40},
		{Name: "observe_provider", Status: "ok", Count: 13, CostUSD: 0.12, DurationMs: 900},
	}}

	steps := aiDiscoveryEvidenceSubsteps(result)
	if len(steps) != 4 {
		t.Fatalf("substeps = %+v, want 4 with crawler/external merged", steps)
	}
	if steps[0].Key != "plan_candidates" || steps[0].Label == "" || steps[0].DurationMs != 11 {
		t.Fatalf("first substep = %+v, want planned probe duration", steps[0])
	}
	var audit opportunityFindingSubstep
	for _, step := range steps {
		if step.Key == "site_surface_audit" {
			audit = step
		}
	}
	if audit.Key == "" || audit.Count != 3 || audit.DurationMs != 70 {
		t.Fatalf("merged audit substep = %+v, want combined count and duration", audit)
	}
}
