package db

import (
	"os"
	"strings"
	"testing"
)

func TestWorkflowEventSchemaContract(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0015_workflow_events.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)

	for _, want := range []string{
		"create table if not exists workflow_events",
		"status text not null default 'pending'",
		"check (status in ('pending','running','succeeded','failed','dead'))",
		"dedupe_key text not null unique",
		"run_after timestamptz not null default now()",
		"locked_at timestamptz",
		"idx_workflow_events_pending",
		"where status = 'pending'",
		"idx_workflow_events_reclaim",
		"where status = 'running'",
		"add column if not exists source_content_action_id",
		"uniq_topic_source_content_action",
		"where source_content_action_id is not null",
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("workflow migration missing %q", want)
		}
	}
}

func TestWorkflowEventQueriesExposeDurableWorkerSemantics(t *testing.T) {
	required := []string{
		enqueueWorkflowEvent,
		claimPendingWorkflowEvents,
		reclaimStuckWorkflowEvents,
		markWorkflowEventSucceeded,
		markWorkflowEventFailed,
		retryWorkflowEvent,
		listDeadWorkflowEventsForProject,
		countOpenSEOOpportunities,
		listUnplannedContentActions,
	}
	for i, query := range required {
		if query == "" {
			t.Fatalf("workflow query %d is empty", i)
		}
	}
	if !strings.Contains(enqueueWorkflowEvent, "on conflict (dedupe_key) do update") {
		t.Fatal("EnqueueWorkflowEvent must be idempotent by dedupe_key")
	}
	if !strings.Contains(claimPendingWorkflowEvents, "for update skip locked") {
		t.Fatal("ClaimPendingWorkflowEvents must lock claimed rows")
	}
	if !strings.Contains(claimPendingWorkflowEvents, "status = 'running'") || !strings.Contains(claimPendingWorkflowEvents, "locked_at = now()") {
		t.Fatal("ClaimPendingWorkflowEvents must mark claimed events running with locked_at")
	}
	if !strings.Contains(reclaimStuckWorkflowEvents, "status = 'running'") || !strings.Contains(reclaimStuckWorkflowEvents, "interval '30 minutes'") {
		t.Fatal("ReclaimStuckWorkflowEvents must avoid reclaiming long LLM generation work before thirty minutes")
	}
	if !strings.Contains(markWorkflowEventFailed, "status = case when attempts >= 4 then 'dead' else 'pending' end") {
		t.Fatal("MarkWorkflowEventFailed must dead-letter events after the fourth attempt")
	}
	if !strings.Contains(retryWorkflowEvent, "status = 'pending'") || !strings.Contains(retryWorkflowEvent, "run_after = now()") {
		t.Fatal("RetryWorkflowEvent must make dead/failed events immediately pending")
	}
}

func TestOpportunityPlanningQueriesExposeBatchInputs(t *testing.T) {
	if !strings.Contains(countOpenSEOOpportunities, "status = 'open'") {
		t.Fatal("CountOpenSEOOpportunities must count only open opportunities")
	}
	if !strings.Contains(listUnplannedContentActions, "left join topics") {
		t.Fatal("ListUnplannedContentActions must detect actions without source topics")
	}
	if !strings.Contains(listUnplannedContentActions, "t.source_content_action_id = ca.id") {
		t.Fatal("ListUnplannedContentActions must use topics.source_content_action_id")
	}
	if !strings.Contains(listUnplannedContentActions, "ca.status = 'ready_for_review'") {
		t.Fatal("ListUnplannedContentActions must select accepted ready_for_review actions for Phase 1")
	}
	if !strings.Contains(listUnplannedContentActions, "for update") || !strings.Contains(listUnplannedContentActions, "skip locked") {
		t.Fatal("ListUnplannedContentActions must use skip locked inside the workflow transaction")
	}
	if !strings.Contains(listUnplannedContentActions, "coalesce(ca.work_type, '') <> 'fix_site_issue'") {
		t.Fatal("ListUnplannedContentActions must exclude site-fix work type before topic planning")
	}
	for _, direct := range []string{"internal_link_patch", "schema_patch", "sitemap_update", "technical_fix"} {
		if !strings.Contains(listUnplannedContentActions, direct) {
			t.Fatalf("ListUnplannedContentActions must exclude direct action asset %q from topic planning", direct)
		}
	}
	if strings.Contains(listUnplannedContentActions, "%metadata_rewrite%") {
		t.Fatal("ListUnplannedContentActions must not exclude all metadata rewrite work; routing is split by work_type")
	}
}

func TestUnplannedContentActionsAreRequeuedByMigration(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0029_requeue_unplanned_content_actions.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{
		"insert into workflow_events",
		"opportunity.batch_completed",
		"content_actions ca",
		"left join topics t",
		"t.source_content_action_id = ca.id",
		"ca.status = 'ready_for_review'",
		"t.id is null",
		"on conflict (dedupe_key) do nothing",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("unplanned content action requeue migration missing %q", want)
		}
	}
}

func TestFailedDirectContentActionsAreRecoveredByMigration(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0032_recover_failed_direct_content_actions.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{
		"update content_actions",
		"status = 'ready_for_review'",
		"status = 'failed'",
		"approved_at is null",
		"published_at is null",
		"verified_at is null",
		"metadata_rewrite",
		"direct_patch",
		"title",
		"meta description",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("failed direct action recovery migration missing %q", want)
		}
	}
}

func TestTopicSourceContentActionContract(t *testing.T) {
	if !strings.Contains(createTopic, "source_content_action_id") {
		t.Fatal("CreateTopic must persist source_content_action_id")
	}
	if !strings.Contains(updateTopic, "source_content_action_id") {
		t.Fatal("UpdateTopic must preserve source_content_action_id")
	}
	if !strings.Contains(selectGenerationCandidates, "source_content_action_id is not null") {
		t.Fatal("SelectGenerationCandidates should prioritize action-sourced topics first")
	}
}

func TestContentActionTraceabilityQueries(t *testing.T) {
	if !strings.Contains(markContentActionDraftReady, "draft_article_id") || !strings.Contains(markContentActionDraftReady, "status = 'ready_for_review'") {
		t.Fatal("MarkContentActionDraftReady must link the generated draft article back to the content action")
	}
	if !strings.Contains(markContentActionMeasuringForDraftArticle, "published_at = now()") || !strings.Contains(markContentActionMeasuringForDraftArticle, "status = 'measuring'") {
		t.Fatal("MarkContentActionMeasuringForDraftArticle must move published action work into measurement")
	}
}
