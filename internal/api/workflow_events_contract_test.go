package api

import (
	"os"
	"strings"
	"testing"
)

func TestSEOReviewHandlersEnqueueWorkflowEvents(t *testing.T) {
	source, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"workflow.EventOpportunityReviewed",
		"s.enqueueWorkflowEvent",
		"updateSEOOpportunityStatus",
		"createSEOContentAction",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("SEO handlers must enqueue opportunity review workflow events; missing %q", want)
		}
	}
}

func TestLegacyStrategistHandlerIsNotExposed(t *testing.T) {
	routes, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(routes), `r.Post("/strategist", s.runStrategist)`) {
		t.Fatal("legacy Strategist route must not be registered")
	}

	handlers, err := os.ReadFile("handlers_agents.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(handlers), "func (s *Server) runStrategist") {
		t.Fatal("legacy runStrategist handler must not be exposed")
	}
}

func TestArticleApproveEnqueuesDraftApprovedWithProjectSchedulePolicy(t *testing.T) {
	source, err := os.ReadFile("handlers_review.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"workflow.EventDraftApproved",
		"s.enqueueWorkflowEvent",
		"approveArticleScoped",
		"canonicalApprovalScheduleAt",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("article approve must enqueue draft approved workflow event; missing %q", want)
		}
	}
}

func TestApplyFixAutoApprovesAndEnqueuesDraftApproved(t *testing.T) {
	source, err := os.ReadFile("handlers_review.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"applyFixProjectArticle",
		"RepairArticleWithInstruction",
		"approveArticleRecord",
		"autoFixReviewer",
		"workflow.EventDraftApproved",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("apply fix must become a terminal QA approval flow; missing %q", want)
		}
	}
}

func TestUpdateConfigRequeuesWorkflowWhenAutoAdvanceTurnsOn(t *testing.T) {
	source, err := os.ReadFile("handlers_projects.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	start := strings.Index(body, "func (s *Server) updateConfig")
	end := strings.Index(body, "func (s *Server) getProfile")
	if start < 0 || end < 0 || end <= start {
		t.Fatal("could not locate updateConfig body")
	}
	handler := body[start:end]
	for _, want := range []string{
		"previousCfg.AutoAdvanceEnabled",
		"cfg.AutoAdvanceEnabled",
		"workflow.EventOpportunityReviewed",
		"workflow.EventContentPlanCreated",
		"s.enqueueWorkflowEvent",
		`"source": "auto_toggle"`,
	} {
		if !strings.Contains(handler, want) {
			t.Fatalf("updateConfig must wake workflow when Auto is turned on; missing %q", want)
		}
	}
}
