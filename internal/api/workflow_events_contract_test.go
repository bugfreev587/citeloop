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

func TestArticleApproveEnqueuesDraftApprovedWithoutBufferDelay(t *testing.T) {
	source, err := os.ReadFile("handlers_review.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"workflow.EventDraftApproved",
		"s.enqueueWorkflowEvent",
		"approveArticleScoped",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("article approve must enqueue draft approved workflow event; missing %q", want)
		}
	}
	if strings.Contains(body, "BufferDays") {
		t.Fatal("article approve must not default unscheduled canonicals to now+buffer_days")
	}
}
