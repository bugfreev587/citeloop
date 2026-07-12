package scheduler

import (
	"os"
	"strings"
	"testing"
)

func TestSchedulerHasOneOpportunityFindingAuthority(t *testing.T) {
	raw, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, forbidden := range []string{
		"func (s *Scheduler) TickGEO",
		"func (s *Scheduler) geoForProject",
		"func (s *Scheduler) runAIDiscoveryForProject",
		"opportunityfinding.RunAIDiscovery(",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("standalone weekly GEO authority remains: %q", forbidden)
		}
	}
	for _, required := range []string{
		"opportunityfinding.RunCheckpointedWorkflow",
		"opportunityfinding.RefreshAIDiscoveryEvidence",
		"opportunityfinding.MaterializeAIDiscoveryHypotheses",
		"OpportunityFindingStagesForTrigger(trigger)",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("canonical Opportunity Finding authority missing %q", required)
		}
	}
}
