package scheduler

import (
	"os"
	"strings"
	"testing"
)

func TestSchedulerClosesDueMeasurementWindows(t *testing.T) {
	source, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"case workflow.EventMeasurementWindowDue:",
		"handleMeasurementWindowDue",
		"ListDueMeasuringContentActions",
		"UpdateContentActionOutcomeSummary",
		`"inconclusive"`,
		`"completed"`,
		"outcome_summary",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("scheduler measurement closure missing %q", want)
		}
	}
}
