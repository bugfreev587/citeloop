import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadModule() {
  const source = await readFile(new URL("./growth-radar.ts", import.meta.url), "utf8");
  const output = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ES2020, target: ts.ScriptTarget.ES2020 } }).outputText;
  return import(`data:text/javascript;base64,${Buffer.from(output).toString("base64")}`);
}

const base = { sources: { scheduled: 3, succeeded: 3, skipped: 0, failed: 0 }, evidence: { added: 4, changed: 0, reused: 0, expired: 0 }, terms: { accepted: 2, rejected: 0, held: 0 }, prompts: { active: 60, selected: 10, rotated: 8, targeted: 2 }, candidates: { generated: 3, duplicates: 1, conflicts: 0, watchlist: 1, filtered: 1, created: 0 }, cost_usd: 0.42, status: "ok", reasons: {} };

test("healthy zero explains policy outcomes rather than discovery failure", async () => {
  const { summarizeGrowthRadarRun, explainZeroOpportunities } = await loadModule();
  const summary = summarizeGrowthRadarRun(base);
  assert.equal(summary.health, "healthy_zero");
  assert.equal(summary.created, 0);
  assert.match(explainZeroOpportunities(base).join(" "), /watchlist/i);
  assert.equal(summary.promptRotation, "8 rotated · 2 targeted");
});

test("degraded zero reports source failures and reasons", async () => {
  const { summarizeGrowthRadarRun, explainZeroOpportunities } = await loadModule();
  const run = { ...base, status: "degraded", sources: { scheduled: 4, succeeded: 1, skipped: 1, failed: 2 }, evidence: { added: 0, changed: 0, reused: 0, expired: 0 }, reasons: { no_usable_evidence: 1, brave_budget_exhausted: 1 } };
  assert.equal(summarizeGrowthRadarRun(run).health, "degraded_zero");
  assert.deepEqual(explainZeroOpportunities(run), ["No usable evidence was collected.", "Search evidence budget was exhausted.", "2 evidence sources failed; 1 was skipped."]);
});

test("summary includes rejection, cost, and exact target information", async () => {
  const { summarizeGrowthRadarRun, summarizeExactTargets } = await loadModule();
  const run = { ...base, candidates: { ...base.candidates, created: 2 }, target_platforms: [{ platform: "blog" }, { platform: "dev_to" }, { platform: "reddit", target_key: "r/saas" }] };
  const summary = summarizeGrowthRadarRun(run);
  assert.equal(summary.health, "created");
  assert.equal(summary.cost, "$0.42");
  assert.equal(summary.rejected, 2);
  assert.equal(summarizeExactTargets(run.target_platforms), "Blog + Dev.to + Reddit (r/saas)");
});
