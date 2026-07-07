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

  for (const marker of [
    "function hasActionVerificationSnapshot",
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

  for (const marker of [
    "function hasResultsExecutionEvidence",
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
