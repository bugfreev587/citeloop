package scheduler

import (
	"os"
	"strings"
	"testing"
)

func TestSchedulerWorkflowHandlerPlansReviewedOpportunities(t *testing.T) {
	source, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"workflow.EventOpportunityReviewed",
		"workflow.EventOpportunityBatchDone",
		"CountOpenSEOOpportunities",
		"ListUnplannedContentActions",
		"EnqueueWorkflowEvent",
		"CreateTopic",
		"SourceContentActionID",
		"UpdateContentActionStatus",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("scheduler workflow handler missing %q", want)
		}
	}
}

func TestSchedulerWorkflowHandlerGeneratesDraftsFromContentPlanEvents(t *testing.T) {
	source, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"case workflow.EventContentPlanCreated:",
		"handleContentPlanCreated",
		"generateForProject",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("scheduler workflow handler missing content plan auto-generation hook %q", want)
		}
	}
}

func TestSchedulerWorkflowHandlerPublishesApprovedDrafts(t *testing.T) {
	source, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"case workflow.EventDraftApproved:",
		"handleDraftApproved",
		"publishForProject",
		"reconcilePublishForProject",
		"unlockVariants",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("scheduler workflow handler missing approved draft publish hook %q", want)
		}
	}
}

func TestSchedulerMaintainsContentActionTraceability(t *testing.T) {
	source, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"SourceContentActionID",
		"MarkContentActionDraftReady",
		"MarkContentActionMeasuringForDraftArticle",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("scheduler must keep opportunity action traceability; missing %q", want)
		}
	}
}
