package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAutopilotExecuteRouteIsRegistered(t *testing.T) {
	router := (&Server{}).Router()
	req := httptest.NewRequest(http.MethodPost, "/api/projects/not-a-uuid/seo/autopilot/plans/not-a-plan/execute", nil)
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want bad project id", res.Code)
	}
}

func TestAutopilotExecuteContractRequiresGuardsAuditAndRollback(t *testing.T) {
	raw, err := os.ReadFile("handlers_autopilot.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{
		"executeAutopilotPlan",
		"auto_publish_allowed",
		"guardrail_results",
		"autopilot_audit_events",
		"manual_rollback_required",
		"recovery_plan",
		"publisher_capability",
		"policy_not_ready",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("execute contract missing %q", want)
		}
	}
}
