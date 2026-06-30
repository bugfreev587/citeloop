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
