import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("SEO page does not expose internal Google Search Console credential fields", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  assert.equal(source.includes("GSC site URL"), false);
  assert.equal(source.includes("Credential ref"), false);
  assert.equal(source.includes("gsc_credential_ref"), false);
});

test("web API exposes Phase 5 autopilot readiness contract", async () => {
  const source = await readFile(new URL("api.ts", import.meta.url), "utf8");

  for (const expected of [
    "AutopilotReadiness",
    "AutopilotReadinessGate",
    "getAutopilotReadiness",
    "ready_for_level_2",
    "rollback_or_recovery_ready",
  ]) {
    assert.equal(source.includes(expected), true, `api.ts missing ${expected}`);
  }
});

test("web API exposes Phase 5 guarded autopilot execution contract", async () => {
  const source = await readFile(new URL("api.ts", import.meta.url), "utf8");

  for (const expected of [
    "AutopilotExecuteResult",
    "executeAutopilotPlan",
    "executed_actions",
    "deferred_actions",
    "guardrail_results",
    "recovery_plans",
  ]) {
    assert.equal(source.includes(expected), true, `api.ts missing ${expected}`);
  }
});

test("SEO autopilot panel exposes Phase 5 guarded execution controls", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  for (const expected of [
    "Readiness",
    "Ready for Level 2",
    "Blocked gates",
    "Execute guarded actions",
    "Recovery plan",
    "Manual rollback required",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }
});

test("Analysis page does not render Automation readiness as a primary module", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const analysisBranchIndex = source.indexOf('{mode === "analysis" && (');
  const resultsBranchIndex = source.indexOf('{mode === "results" && (');

  assert.notEqual(analysisBranchIndex, -1, "seo-client.tsx missing Analysis render branch");
  assert.notEqual(resultsBranchIndex, -1, "seo-client.tsx missing Results render branch");
  assert.equal(source.includes("data-analysis-autopilot-visible"), false);
  assert.equal(source.includes('title="Automation readiness"'), false);
  assert.equal(source.includes("Finish automation setup in Settings"), true);
  assert.ok(
    analysisBranchIndex < source.indexOf("Finish automation setup in Settings") &&
      source.indexOf("Finish automation setup in Settings") < resultsBranchIndex,
    "the lightweight setup bridge should stay inside the Analysis branch",
  );
});

test("Analysis page leads with Opportunity Queue before Site Fixes", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  for (const expected of ["data-analysis-opportunity-finding-status", "data-analysis-growth-findings-section", "Opportunity Queue ·", "data-site-fixes-queue", "Site Fixes", "data-site-fix-card"]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }

  const findingStatusIndex = source.indexOf("data-analysis-opportunity-finding-status");
  const opportunityQueueIndex = source.indexOf("data-analysis-growth-findings-section");
  const siteFixesIndex = source.indexOf("data-site-fixes-queue");
  const loopIndex = source.indexOf("data-analysis-loop-strip");

  assert.ok(findingStatusIndex < opportunityQueueIndex, "Opportunity Finding status should sit above the queue");
  assert.ok(opportunityQueueIndex < siteFixesIndex, "Opportunity Queue should appear before Site Fixes");
  assert.ok(siteFixesIndex < loopIndex, "Site Fixes should stay before lower-priority loop diagnostics");
  assert.equal(source.includes("data-analysis-focus-cards"), false, "Analysis should not render the old top metrics board");
  assert.equal(source.includes("What needs review next"), false, "Analysis should not render the old metrics board headline");
  assert.equal(source.includes("Review direct action"), false, "Analysis should not expose Direct Action as user-facing queue copy");
  assert.equal(source.includes("Direct action queue"), false, "Analysis should rename the old Direct Action queue to Site Fixes");
  assert.equal(source.includes("No direct actions to review"), false, "Analysis empty states should use Site Fixes language");
  assert.equal(source.includes("data-analysis-autopilot-visible"), false);
  assert.equal(source.includes("Automation readiness"), false);
  assert.equal(source.includes("Finish automation setup in Settings"), true);
  assert.equal(source.includes("data-analysis-search-signal"), false, "Analysis should not show search metrics as a first-level panel");
  assert.equal(source.includes("Search performance snapshot"), false, "Home owns the search-performance KPI snapshot");
});

test("Analysis page exposes Opportunity Finding run status", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  for (const expected of [
    "api.getOpportunityFindingStatus(projectId)",
    "api.runOpportunityFinding(projectId)",
    "Last finding",
    "Next finding",
    "Manual mode",
    "Run finding",
    "Signal Scan",
    "AI Discovery",
    "summary.slice(0, 5)",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }
});

test("Analysis Site Fixes open a reusable right drawer for review", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const drawerStart = source.indexOf("data-direct-action-drawer");
  const drawerEnd = source.indexOf('{mode === "analysis" && selectedOpportunity', drawerStart);
  const drawerSource = source.slice(drawerStart, drawerEnd);

  for (const expected of [
    "selectedDirectActionID",
    "selectedDirectAction",
    "directActionDrawerRef",
    "data-site-fix-card",
    "data-direct-action-drawer",
    "Review site fix details",
    "Close action details",
    "Mark applied",
    "Dismiss",
    "dismissSiteFixAction",
    "api.dismissSEOContentAction",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }
  assert.notEqual(drawerStart, -1, "seo-client.tsx missing direct action drawer");
  assert.doesNotMatch(drawerSource, /Needs revision/, "Site Fix review drawer should not expose a non-functional revision action");
  assert.doesNotMatch(drawerSource, /verifyAction\(action, "failed"\)/, "Site Fix review drawer should not mark review feedback as verification failure");
});

test("Analysis Site Fixes expose copyable AI repair JSON", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  for (const expected of [
    "buildSiteFixAIPayload",
    "siteFixImplementationSteps",
    "siteFixLikelySurfaces",
    "siteFixPatchContract",
    "siteFixAIJSON",
    "copySiteFixAIJSON",
    "writeClipboardText",
    "data-site-fix-ai-payload",
    "AI coding fix JSON",
    "Copy fix JSON",
    "site-fix-copy-json-button",
    "whitespace-nowrap",
    "min-w-[9.5rem]",
    "Copy this JSON into Codex or Claude Code",
    "ai_repair",
    "acceptance_tests",
    "proposed_changes",
    "observed_metadata",
    "deduplication_rule",
    "graph_guidance",
    "@graph",
    "#organization",
    "#website",
    "#webpage",
    "do_not",
    "human_review",
    "does not require rich result eligibility",
    "schema_patch",
    "script[type=\\\"application/ld+json\\\"]",
    "Clipboard",
    "Code2",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }
});

test("Analysis Loop in motion makes Site Fixes visible inside the lifecycle", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  const loopStart = source.indexOf("data-analysis-loop-strip");
  const loopEnd = source.indexOf("Finish automation setup in Settings", loopStart);
  const loopBlock = source.slice(loopStart, loopEnd);

  assert.notEqual(loopStart, -1, "seo-client.tsx missing Loop in motion section");
  assert.match(source, /function loopActionDestinationLabel/);
  assert.match(source, /function loopLifecycleSummaryLabel/);
  assert.match(loopBlock, /Published \/ Applied/);
  assert.match(loopBlock, /loopActionDestinationLabel\(action\)/);
  assert.match(loopBlock, /Site Fixes/);
  assert.match(loopBlock, /Content Plan/);
});

test("Analysis opportunity cards expose destination-specific routing and handoff links", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  for (const expected of [
    "opportunityWorkType",
    "opportunityDestination",
    "opportunityPrimaryCTA",
    "assetTypeForWorkType",
    "sentOpportunityLinks",
    "data-opportunity-handoff-card",
    "Recently sent",
    "Sent to Site Fixes",
    "View in Site Fixes",
    "plan?action=${action.id}",
    "focusSiteFixCard",
    "citeloop-linked-card-pulse",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }

  assert.equal(source.includes("Create content task"), false, "Opportunity Queue should not show generic Create content task CTA");
  assert.equal(source.includes("Create technical task"), false, "Opportunity Queue should not show generic technical task CTA");
  const directAssetTypes = source.match(/const directActionAssetTypes = new Set\(\[([^\]]+)\]\)/)?.[1] ?? "";
  assert.equal(directAssetTypes.includes("metadata_rewrite"), false, "Metadata/page-update work should not be routed to Site Fixes");
});

test("Opportunity queue supports snooze, watch, and approval-source provenance", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  for (const expected of [
    "snoozeOpportunity",
    "unsnoozeOpportunity",
    "watchOpportunity",
    "Watch in Results",
    "Watching in Results",
    "Snoozed (",
    "approvalSourceLabel(action.approval_source)",
    "Approved by Autopilot policy",
    "results-watchlist",
    "watchlistStatusLabel",
    "This item moved or was completed",
    "Show all site fixes",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }
});

test("Opportunity review drawer explains work type destination and approval source", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const drawerStart = source.indexOf('data-analysis-drawer');
  const drawerEnd = source.indexOf('data-direct-action-drawer');
  const drawerSource = source.slice(drawerStart, drawerEnd);

  for (const expected of [
    "Approve to send this to",
    "Work type",
    "Destination",
    "Approval source",
    "Human opportunity approval",
    "Create Site Fix",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }

  assert.match(source, /function opportunityWorkType\(opportunity: SEOOpportunity\): OpportunityWorkType/);
  assert.notEqual(drawerStart, -1, "seo-client.tsx missing opportunity drawer");
  assert.notEqual(drawerEnd, -1, "seo-client.tsx missing drawer boundary");
  assert.equal(drawerSource.length >= 0, true, "drawer boundary slice must resolve");

  // PRD §6.2: the drawer must let users correct the route between allowed
  // work types, keep CTA/destination/approval copy in sync, and explain why
  // locked routes cannot change.
  for (const expected of [
    "allowedWorkTypesForOpportunity",
    "routeOverrides",
    "setRouteOverrides",
    'aria-label="Choose work type"',
    "workTypeLockReason",
    "approvalCopyForWorkType",
    "work_type: workTypeKeys[workType]",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }
});

test("Analysis loop progress is a strip and finding dismissal is explicit", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  assert.equal(source.includes("data-analysis-loop-strip"), true, "analysis loop progress should render as a horizontal strip");
  assert.equal(
    source.includes("xl:grid-cols-[minmax(0,1fr)_320px]"),
    false,
    "growth findings should not reserve a persistent right-side loop rail",
  );
  assert.equal(source.includes("Dismiss finding"), true, "dismiss action must make the destructive operation explicit");
  assert.equal(source.includes("Close finding details"), true, "finding drawer needs a separate close affordance");
});

test("Analysis loop metrics reveal selected linked content cards", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const loopStart = source.indexOf("data-analysis-loop-strip");
  const loopEnd = source.indexOf("{readiness &&");
  const loopSource = source.slice(loopStart, loopEnd);

  assert.notEqual(loopStart, -1, "seo-client.tsx missing loop in motion section");
  assert.notEqual(loopEnd, -1, "seo-client.tsx missing loop section boundary");

  for (const expected of [
    "selectedLoopStage",
    "setSelectedLoopStage",
    "selectedLoopActions",
    "loopStageDetailTitle",
    "loopActionCurrentHref",
    "loopActionCurrentLabel",
    "data-loop-stage-card",
    "data-loop-action-card",
    "aria-pressed={selectedLoopStage === item.key}",
    "disabled={item.value === 0}",
    "setSelectedLoopStage((current) => (current === item.key ? null : item.key))",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }

  assert.equal(loopSource.includes("loopPreviewActions"), false, "loop cards should not render an unconditional Recently Sent duplicate");
  assert.equal(loopSource.includes("Open current location"), true, "filtered cards should link to the card's current surface");
  assert.match(source, /\/projects\/\$\{projectId\}\/review\?article=\$\{action\.draft_article_id\}/);
  assert.match(source, /\/projects\/\$\{projectId\}\/results\?action=\$\{action\.id\}/);
});

test("Analysis Site Fix handoff cards use the loop action source for same-page targets", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const loopActionsStart = source.indexOf("const actionsByID = new Map(actions.map((action) => [action.id, action]));");
  const loopActionsEnd = source.indexOf("const measuredActions = loopActions");
  const directStart = source.indexOf("const directReviewActionsAll =");
  const directEnd = source.indexOf("const directReviewActions =");
  const focusStart = source.indexOf("function focusSiteFixCard");
  const focusEnd = source.indexOf("useEffect(() => {\n    if (!pendingSiteFixFocusID)");
  const loopActionsSource = source.slice(loopActionsStart, loopActionsEnd);
  const directSource = source.slice(directStart, directEnd);
  const focusSource = source.slice(focusStart, focusEnd);

  assert.notEqual(loopActionsStart, -1, "loop actions should index full content actions by id");
  assert.notEqual(loopActionsEnd, -1, "loop action source boundary should resolve");
  assert.notEqual(directStart, -1, "seo-client.tsx missing Site Fixes action source");
  assert.notEqual(directEnd, -1, "Site Fixes action source boundary should resolve");
  assert.notEqual(focusStart, -1, "seo-client.tsx missing Site Fix focus handler");
  assert.notEqual(focusEnd, -1, "Site Fix focus handler boundary should resolve");

  for (const expected of [
    "const summaryLoopActions = (visibilitySummary?.actions_in_loop ?? []).map",
    "const matchingAction = actionsByID.get(summaryAction.id);",
    "return matchingAction ? { ...summaryAction, ...matchingAction } : summaryAction;",
  ]) {
    assert.equal(loopActionsSource.includes(expected), true, `loop action source should include ${expected}`);
  }

  assert.match(directSource, /const directReviewActionsAll = loopActions\s+\.filter\(\(action\) => isDirectAction\(action\)\)/);
  assert.equal(directSource.includes("const directReviewActionsAll = actions"), false, "Site Fixes should not depend on the separately paged content action list");
  assert.equal(focusSource.includes("directReviewActionsAll.some((action) => action.id === actionID)"), true, "Loop Site Fix cards should expand the same source they render from");
});

test("Opportunity queue lays finding cards out as responsive rectangles with three per row at most", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const queueStart = source.indexOf("data-analysis-growth-findings-section");
  const queueEnd = source.indexOf("data-analysis-loop-strip");
  const queueSource = source.slice(queueStart, queueEnd);

  assert.notEqual(queueStart, -1, "seo-client.tsx missing opportunity queue section");
  assert.notEqual(queueEnd, -1, "seo-client.tsx missing loop strip section after opportunity queue");
  assert.equal(queueSource.includes("data-analysis-finding-grid"), true, "opportunity queue should expose its responsive card grid");
  assert.equal(queueSource.includes("md:grid-cols-2"), true, "opportunity queue should place cards horizontally on medium screens");
  assert.equal(queueSource.includes("xl:grid-cols-3"), true, "opportunity queue should cap wide layouts at three cards per row");
  assert.equal(queueSource.includes("min-h-[220px]"), true, "opportunity cards should keep a rectangular card footprint");
  assert.equal(queueSource.includes("lg:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)_auto]"), false, "finding cards should not keep the old full-row internal layout");
  assert.equal(queueSource.includes("risk_level"), false, "risk level is an internal judgment and belongs in the drawer");
  assert.equal(queueSource.includes("sourceModeForOpportunity"), false, "source mode is diagnostic context and belongs in the drawer");
  assert.equal(queueSource.includes("priority_score"), false, "raw priority scores should not appear on first-level cards");
  assert.equal(queueSource.includes("Signal"), false, "first-level cards should avoid backend signal labels");
});
