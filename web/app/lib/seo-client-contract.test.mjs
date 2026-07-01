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
