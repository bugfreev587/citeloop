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
