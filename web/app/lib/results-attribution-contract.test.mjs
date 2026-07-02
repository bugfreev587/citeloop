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
    "aria-label={`Open attribution details: ${action.action_type}`}",
    "Measurement details",
    "Manual verify",
    "Verification failed",
  ]) {
    assert.match(resultsBlock, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
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
