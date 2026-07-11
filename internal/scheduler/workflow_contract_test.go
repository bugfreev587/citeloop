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

func TestSchedulerWorkflowHandlerRunsManualOpportunityFinding(t *testing.T) {
	source, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"case workflow.EventOpportunityFindingRequested:",
		"handleOpportunityFindingRequested",
		"runOpportunityFindingForProject",
		"runOpportunityFindingForProject(ctx, q, project, false)",
		"workflow.Permanent(err)",
		"opportunityfinding.RunAIDiscovery",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("scheduler durable Opportunity Finding handler missing %q", want)
		}
	}
}

func TestOpportunityReviewedPlansAcceptedActionsWithoutWaitingForAllOpenOpportunities(t *testing.T) {
	source, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	start := strings.Index(body, "func (s *Scheduler) handleOpportunityReviewed")
	end := strings.Index(body, "func (s *Scheduler) handleOpportunityBatchCompleted")
	if start == -1 || end == -1 || end <= start {
		t.Fatal("could not locate handleOpportunityReviewed body")
	}
	handler := body[start:end]
	if strings.Contains(handler, "CountOpenSEOOpportunities") || strings.Contains(handler, "open > 0") {
		t.Fatal("accepted content actions must be planned even when other analysis recommendations remain open")
	}
	if !strings.Contains(handler, "ListUnplannedContentActions") || !strings.Contains(handler, "EventOpportunityBatchDone") {
		t.Fatal("opportunity review must enqueue pending content actions for planning")
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

func TestSchedulerWorkflowHandlersGateEventDrivenAutoAdvance(t *testing.T) {
	source, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"handleContentPlanCreated",
		"handleDraftApproved",
		"AutoAdvanceEnabled",
		"workflow auto advance disabled",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("event-driven workflow handlers must honor auto_advance_enabled; missing %q", want)
		}
	}
}

func TestOpportunityBatchPlanningIsolatesFailedActions(t *testing.T) {
	source, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"SAVEPOINT workflow_action_",
		"ROLLBACK TO SAVEPOINT workflow_action_",
		"RELEASE SAVEPOINT workflow_action_",
		`Status:    "failed"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("opportunity batch planning must isolate failed actions with savepoints; missing %q", want)
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
