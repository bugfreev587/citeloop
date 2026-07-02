package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAutopilotReadinessRouteIsRegistered(t *testing.T) {
	router := (&Server{}).Router()
	req := httptest.NewRequest(http.MethodGet, "/api/projects/not-a-uuid/seo/autopilot/readiness", nil)
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want bad project id", res.Code)
	}
}

func TestAutopilotReadinessContractMentionsPhase5Gates(t *testing.T) {
	raw, err := os.ReadFile("handlers_autopilot.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{
		"type AutopilotReadiness",
		"type AutopilotReadinessGate",
		"ready_for_level_2",
		"publisher_write",
		"notification_write",
		"autopilot_policy_confirmed",
		"monthly_budget_configured",
		"safe_mode_clear",
		"kill_switch_clear",
		"rollback_or_recovery_ready",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("readiness contract missing %q", want)
		}
	}
}

func TestAutopilotReadinessContractUsesPolicyBudgetAndRecoveryAcknowledgement(t *testing.T) {
	raw, err := os.ReadFile("handlers_autopilot.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{
		"policy.MonthlyBudgetLimit",
		"monthly_budget_configured",
		"RecoveryPlanAcknowledgedAt",
		"recovery_plan_acknowledged",
		"rollback_or_recovery_ready",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("readiness contract missing %q", want)
		}
	}
	if strings.Contains(body, "MonthlyBudgetUsd") || strings.Contains(body, "monthly_budget_usd") {
		t.Fatalf("Autopilot readiness must not use project monthly_budget_usd")
	}
}
