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

test("Analysis page renders Phase 5 autopilot controls before advanced diagnostics", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const analysisBranchIndex = source.indexOf('{mode === "analysis" && (');
  const resultsBranchIndex = source.indexOf('{mode === "results" && (');
  const autopilotIndex = source.indexOf("data-analysis-autopilot-visible");
  const diagnosticsIndex = source.indexOf("Advanced diagnostics");

  assert.notEqual(analysisBranchIndex, -1, "seo-client.tsx missing Analysis render branch");
  assert.notEqual(resultsBranchIndex, -1, "seo-client.tsx missing Results render branch");
  assert.notEqual(autopilotIndex, -1, "seo-client.tsx missing visible Autopilot section");
  assert.notEqual(diagnosticsIndex, -1, "seo-client.tsx missing Advanced diagnostics section");
  assert.equal(
    source.match(/data-analysis-autopilot-visible/g)?.length ?? 0,
    1,
    "visible Analysis Autopilot section should render only once",
  );
  assert.ok(
    analysisBranchIndex < autopilotIndex && autopilotIndex < resultsBranchIndex,
    "Autopilot controls must render inside the Analysis branch, before the Results branch starts",
  );
  assert.ok(
    autopilotIndex < diagnosticsIndex,
    "Autopilot controls must render before Advanced diagnostics so they are visible by default",
  );
});

test("Analysis page leads with compact review cards instead of deep data panels", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  for (const expected of [
    "data-analysis-focus-cards",
    "data-analysis-focus-card",
    "What needs review next",
    "Review direct action",
    "Inspect new findings",
    "Automation readiness",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }

  const focusIndex = source.indexOf("data-analysis-focus-cards");
  const directQueueIndex = source.indexOf("data-direct-action-queue");
  const growthIndex = source.indexOf("data-analysis-growth-findings-section");
  const searchIndex = source.indexOf("data-analysis-search-signal");
  const autopilotIndex = source.indexOf("data-analysis-autopilot-visible");

  assert.ok(focusIndex < directQueueIndex, "priority cards should appear before the direct action queue");
  assert.ok(directQueueIndex < growthIndex, "reviewable direct actions should appear before new findings");
  assert.ok(growthIndex < searchIndex, "search metrics should be supporting context after decisions");
  assert.ok(searchIndex < autopilotIndex, "automation readiness should stay after decision context");
});

test("Analysis direct actions open a reusable right drawer for review", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  for (const expected of [
    "selectedDirectActionID",
    "selectedDirectAction",
    "directActionDrawerRef",
    "data-direct-action-card",
    "data-direct-action-drawer",
    "Review action details",
    "Close action details",
    "Mark applied",
    "Needs revision",
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
});
