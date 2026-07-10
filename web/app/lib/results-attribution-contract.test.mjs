import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { test } from "node:test";

const root = join(process.cwd(), "app");
const read = (path) => readFileSync(join(root, path), "utf8");

test("web API exposes Phase 4 results attribution endpoints", () => {
  const api = read("lib/api.ts");
  for (const marker of [
    "export type ActionMeasurement",
    "export type ResultsAction",
    "function normalizeActionMeasurement",
    "function normalizeResultsAction",
    "listResultsActions",
    "getResultsAction",
    "recomputeResults",
    "`/projects/${id}/results/actions`",
    "`/projects/${id}/results/recompute`",
    "outcome_label",
    "outcome_reason",
    "attribution_confidence",
    "confounders",
  ]) {
    assert.match(api, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("Results page renders action-level attribution rows", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  const resultsStart = seo.indexOf('{mode === "results"');
  const resultsBlock = seo.slice(resultsStart);
  for (const marker of [
    "resultsActions",
    "api.listResultsActions(projectId, { limit: 50 })",
    "api.recomputeResults(projectId)",
    "Action-level attribution",
    "Before",
    "After",
    "Outcome reason",
    "Attribution confidence",
    "Confounders",
    "insufficient_data",
  ]) {
    assert.match(resultsBlock, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("Results action attribution opens compact cards into a detail drawer", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  const resultsStart = seo.indexOf('{mode === "results"');
  const resultsBlock = seo.slice(resultsStart);

  for (const marker of [
    "selectedResultActionID",
    "resultDrawerRef",
    "resultReturnFocusRef",
    "data-results-action-card",
    "data-results-drawer",
    "aria-label={`Open attribution details: ${publishedTitle}`}",
    "Measurement details",
    "Manual verify",
    "Verification failed",
  ]) {
    assert.match(resultsBlock, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("Results attribution cards use the shared responsive square-card grid", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  const resultsStart = seo.indexOf('{mode === "results"');
  const resultsBlock = seo.slice(resultsStart);
  const gridStart = resultsBlock.indexOf("data-results-action-grid");
  const gridEnd = resultsBlock.indexOf("</section>", gridStart);
  const gridBlock = resultsBlock.slice(gridStart, gridEnd);

  assert.notEqual(gridStart, -1, "results attribution should expose a responsive card grid");
  assert.equal(gridBlock.includes("md:grid-cols-2"), true, "results attribution should show two cards on medium screens");
  assert.equal(gridBlock.includes("xl:grid-cols-3"), true, "results attribution should show three cards on wide screens");
  assert.equal(gridBlock.includes("min-h-[220px]"), true, "results attribution cards should keep a square-card footprint");
  assert.equal(gridBlock.includes("flex h-full"), true, "results attribution cards should fill the grid cell vertically");
  assert.equal(gridBlock.includes("Open details"), true, "results attribution cards should match Site Fix card footer language");
  assert.equal(gridBlock.includes("md:flex-row"), false, "results attribution cards should not keep the old full-row internal layout");
});

test("Results attribution cards identify the published article behind the action", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  const resultsStart = seo.indexOf('{mode === "results"');
  const resultsBlock = seo.slice(resultsStart);

  for (const marker of [
    "resultPublishedArticleTitle(action)",
    "resultPublishedArticleUrl(action)",
    "data-results-action-card={action.id}",
  ]) {
    assert.match(resultsBlock, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  assert.match(seo, /draft_article_id === requestedResultArticleID/);
});

test("Results separates impact outcomes from measurement queue states", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  const resultsStart = seo.indexOf('{mode === "results"');
  const resultsBlock = seo.slice(resultsStart);

  for (const marker of [
    'type ActionMeasurementKey = "waiting" | "positive" | "negative" | "mixed" | "inconclusive" | "insufficient_data"',
    'if (["mixed"',
    "measurementQueueState(action)",
    'type MeasurementQueueKey = "waiting" | "too_early" | "blocked" | "completed"',
  ]) {
    assert.match(seo, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  for (const marker of [
    '{ label: "Mixed"',
    'label: "Too early"',
    'label: "Blocked"',
    'label: "Completed"',
    'title="Measurement queue"',
    'title="Action-level attribution"',
  ]) {
    assert.match(resultsBlock, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("Results does not treat empty verification snapshots as measurement evidence", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  const resultsStart = seo.indexOf('{mode === "results"');
  const resultsBlock = seo.slice(resultsStart);

  // The snapshot-emptiness helper moved to the shared site-fix module.
  const lib = read("lib/site-fix.ts");
  assert.match(lib, /function hasActionVerificationSnapshot/);

  // The Results UI still consumes it to distinguish measured from unmeasured work.
  for (const marker of [
    "hasActionVerificationSnapshot(action)",
    'action.verified_at ? "Verified" : hasActionVerificationSnapshot(action) ? "Needs check" : "Not started"',
  ]) {
    assert.match(seo, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.doesNotMatch(
    seo,
    /Boolean\(action\.published_at \|\| action\.verified_at \|\| action\.verification_snapshot\)/,
    "empty verification_snapshot objects must not make a ready_for_review action look measured",
  );
  assert.doesNotMatch(
    resultsBlock,
    /action\.verified_at \? "Verified" : action\.verification_snapshot \? "Needs check" : "Not started"/,
    "empty verification_snapshot objects must render as not started",
  );
});

test("Results attribution only shows published or applied actions", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  const resultsStart = seo.indexOf('{mode === "results"');
  const resultsBlock = seo.slice(resultsStart);

  // The results-eligibility helper moved to the shared site-fix module.
  const lib = read("lib/site-fix.ts");
  assert.match(lib, /function hasResultsExecutionEvidence/);

  // The Results UI still gates its attribution list on published/applied evidence.
  for (const marker of [
    "Boolean(action.published_at || action.verified_at)",
    '!["archived", "dismissed"].includes(action.status) && hasResultsExecutionEvidence(action)',
  ]) {
    assert.match(seo, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  for (const marker of [
    "Published / applied",
  ]) {
    assert.match(resultsBlock, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.doesNotMatch(
    resultsBlock,
    /resultsActions\.length \? resultsActions\.filter\(\(action\) => !\["archived", "dismissed"\]\.includes\(action\.status\)\) : resultActions/,
    "Results attribution must not show status-only actions that have no publish or applied timestamp",
  );
  assert.doesNotMatch(
    resultsBlock,
    /<div className="font-semibold text-slate-700">Published<\/div>\s*<div>{formatDate\(action\.published_at \?\? null\)}<\/div>/,
    "date label should make clear direct actions may be applied rather than published",
  );
});
