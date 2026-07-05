import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { join } from "node:path";

const root = join(process.cwd(), "app");
const read = (path) => readFileSync(join(root, path), "utf8");

test("web API exposes visibility summary and lifecycle contract", () => {
  const api = read("lib/api.ts");

  for (const marker of [
    "export type VisibilityLifecycleStage",
    "export type VisibilityLifecycleCounts",
    "export type VisibilityActionInLoop",
    "export type VisibilitySummary",
    "getVisibilitySummary",
    "`/projects/${id}/seo/visibility/summary`",
    "lifecycle_counts",
    "actions_in_loop",
    "top_measurement_updates",
    "diagnostics_health",
  ]) {
    assert.match(api, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("shared visibility lifecycle helper derives PRD presentation stages", () => {
  const lifecycle = read("lib/visibility-lifecycle.ts");

  for (const marker of [
    "export function deriveVisibilityLifecycleStage",
    "export function visibilityLifecycleCounts",
    "detected",
    "added_to_plan",
    "planned",
    "drafting",
    "ready_for_review",
    "approved",
    "published_or_applied",
    "measuring",
    "learned",
    "blocked",
    "topic_status",
    "draft_article_status",
    "outcome_summary",
  ]) {
    assert.match(lifecycle, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("analysis page renders loop in motion from lifecycle summary", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");

  for (const marker of [
    "getVisibilitySummary",
    "visibilitySummary",
    "Loop in motion",
    "lifecycle_counts",
    "added_to_plan",
    "ready_for_review",
    "published_or_applied",
    "View results",
    "deriveVisibilityLifecycleStage",
  ]) {
    assert.match(seo, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.doesNotMatch(seo, /active tasks<\/Badge>/);
});

test("results measurement queue consumes visibility summary loop actions", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");

  assert.match(seo, /const measuredActions = loopActions\.filter/);
  assert.match(seo, /Measurement queue/);
  assert.doesNotMatch(seo, /const measuredActions = actions\.filter/);
});

test("home consumes visibility summary for opportunity and loop counts", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const marker of [
    "VisibilitySummary",
    "getVisibilitySummary(projectId)",
    "visibilitySummary",
    "setVisibilitySummary",
    "visibilityOpenOpportunityCount",
    "visibilityActionsInLoopCount",
    "visibilityCitationSignalCount",
    "openOpportunityCount: visibilityOpenOpportunityCount",
    "analysisActionCount: visibilityActionsInLoopCount",
  ]) {
    assert.match(workspace, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.doesNotMatch(workspace, /api\.listSEOOpportunities\(projectId, \{ status: "open", limit: 50 \}\)/);
  assert.doesNotMatch(workspace, /api\.listSEOContentActions\(projectId, \{ limit: 50 \}\)/);
  assert.doesNotMatch(workspace, /openOpportunityCount: seoOpportunities\.length/);
});

test("content plan consumes visibility summary for analysis handoff state", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");

  for (const marker of [
    "VisibilitySummary",
    "getVisibilitySummary(projectId)",
    "visibilitySummary",
    "setVisibilitySummary",
    "summaryOpenOpportunityCount",
    "summaryPendingPlanActions",
    "acceptedPlanActions",
    "data-content-plan-handoff-section",
    "data-content-plan-action-card",
    "highlightContentPlanAction",
    "useSearchParams",
    "searchParams.get(\"action\")",
    "citeloop-linked-card-pulse",
    "[\"added_to_plan\", \"planned\", \"drafting\", \"ready_for_review\"].includes(action.lifecycle_stage)",
  ]) {
    assert.match(topics, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.doesNotMatch(topics, /api\.listSEOOpportunities\(projectId, \{ status: "open", limit: 50 \}\)/);
  assert.doesNotMatch(topics, /api\.listSEOContentActions\(projectId, \{ limit: 50 \}\)/);
  assert.doesNotMatch(topics, /setPendingContentActions\(actions\.filter/);
});
