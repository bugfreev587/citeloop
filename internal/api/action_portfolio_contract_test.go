package api

import (
	"os"
	"strings"
	"testing"
)

func TestGenerateAutopilotPlanWritesActionPortfolioDocument(t *testing.T) {
	raw, err := os.ReadFile("handlers_autopilot.go")
	if err != nil {
		t.Fatalf("read handlers_autopilot.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"actionPortfolioDocument",
		"selected_actions",
		"deferred_actions",
		"rejected_actions",
		"reason_codes",
		"policy_snapshot",
		"budget_snapshot",
		"risk_summary",
		"required_approvals",
		"measurement_schedule",
		"action_bucket",
		"review_required",
		"measurementScheduleForAction",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("autopilot plan missing %q", want)
		}
	}
}
