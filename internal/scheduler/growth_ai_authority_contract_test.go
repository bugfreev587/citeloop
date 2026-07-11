package scheduler

import (
	"os"
	"strings"
	"testing"
)

func TestScheduledGrowthArbitrationUsesScheduledProjectAuthority(t *testing.T) {
	raw, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{"ComparatorAuthority", "GrowthAITriggerScheduled", "GrowthComparator"} {
		if !strings.Contains(source, required) {
			t.Fatalf("scheduler Growth authority missing %q", required)
		}
	}
	if strings.Contains(source, "comparator = discovery.NewLLMSemanticComparator") {
		t.Fatal("scheduler still turns the shared provider into Growth authority")
	}
}
