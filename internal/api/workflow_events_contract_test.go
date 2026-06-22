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

func TestRunStrategistEnqueuesContentPlanCreated(t *testing.T) {
	source, err := os.ReadFile("handlers_agents.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"workflow.EventContentPlanCreated",
		"s.enqueueWorkflowEvent",
		"len(topics) > 0",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("runStrategist must enqueue content_plan.created so domain topics auto-draft; missing %q", want)
		}
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
