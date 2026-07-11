package api

import (
	"os"
	"strings"
	"testing"
)

func TestAllContentActionCreationPathsUseGrowthExecutionGuard(t *testing.T) {
	seoRaw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatal(err)
	}
	autopilotRaw, err := os.ReadFile("handlers_autopilot.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(seoRaw), "requireGrowthOpportunityExecutable(ctx, projectID, opp.ID)") {
		t.Fatal("reviewed Growth action creation bypasses canonical signature/blocker guard")
	}
	if !strings.Contains(string(autopilotRaw), "requireGrowthOpportunityExecutable(r.Context(), projectID, opp.ID)") {
		t.Fatal("autopilot Growth action creation bypasses canonical signature/blocker guard")
	}
}
