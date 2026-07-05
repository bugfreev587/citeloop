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

  for (const expected of ["data-analysis-growth-findings-section", "Opportunity Queue ·", "data-site-fixes-queue", "Site Fixes", "data-site-fix-card"]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }

  const opportunityQueueIndex = source.indexOf("data-analysis-growth-findings-section");
  const siteFixesIndex = source.indexOf("data-site-fixes-queue");
  const loopIndex = source.indexOf("data-analysis-loop-strip");

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

test("Analysis Site Fixes open a reusable right drawer for review", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  for (const expected of [
    "selectedDirectActionID",
    "selectedDirectAction",
    "directActionDrawerRef",
    "data-site-fix-card",
    "data-direct-action-drawer",
    "Review site fix details",
    "Close action details",
    "Mark applied",
    "Needs revision",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }
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
    "focusSiteFixCard",
    "citeloop-linked-card-pulse",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }

  assert.equal(source.includes("Create content task"), false, "Opportunity Queue should not show generic Create content task CTA");
  assert.equal(source.includes("Create technical task"), false, "Opportunity Queue should not show generic technical task CTA");
  assert.equal(source.includes("opportunityWorkTypeOptions"), false, "Opportunity work type should be fixed by the opportunity, not chosen in the UI");
  assert.equal(source.includes("workTypeOverrides"), false, "Opportunity approval must not override the system-selected work type");
  const directAssetTypes = source.match(/const directActionAssetTypes = new Set\(\[([^\]]+)\]\)/)?.[1] ?? "";
  assert.equal(directAssetTypes.includes("metadata_rewrite"), false, "Metadata/page-update work should not be routed to Site Fixes");
});

test("Opportunity review drawer explains work type destination and approval source", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

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
  assert.equal(source.includes('role="group" aria-label="Work type"'), false, "Work type should render as read-only information, not a toggle group");
  assert.equal(source.includes("aria-pressed"), false, "Opportunity drawer should not expose work type selection buttons");
  assert.equal(source.includes("setWorkTypeOverrides"), false, "Opportunity drawer should not let users change the system-selected work type");
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
