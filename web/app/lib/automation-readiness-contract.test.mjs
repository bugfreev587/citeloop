import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("automation readiness mapper owns every gate target and priority", async () => {
  const source = await readFile(new URL("automation-readiness.ts", import.meta.url), "utf8");

  for (const expected of [
    "READINESS_GATE_ACTIONS",
    "search_read",
    "publisher_write",
    "notification_write",
    "autopilot_policy_confirmed",
    "automation_pause_clear",
    "monthly_budget_configured",
    "safe_mode_clear",
    "kill_switch_clear",
    "rollback_or_recovery_ready",
    "settings#search-console",
    "settings#publisher",
    "settings#notifications",
    "settings#automation-policy",
    "settings#automation-status",
    "settings#automation",
    "settings#recovery-plan",
    "Set Autopilot budget",
    "SEOPolicy.monthly_budget_limit",
    "priorityWhenLevel2: \"P0\"",
    "priorityBeforeLevel2: \"P1\"",
    "priorityBeforeLevel2: \"P2\"",
  ]) {
    assert.equal(source.includes(expected), true, `automation-readiness.ts missing ${expected}`);
  }

  assert.equal(source.includes("settings#project"), false, "Autopilot budget must not link to project budget");
  assert.equal(source.includes("monthly_budget_usd"), false, "Autopilot budget must not use project monthly_budget_usd");
});

test("automation readiness mapper exposes a per-gate fix destination", async () => {
  const source = await readFile(new URL("automation-readiness.ts", import.meta.url), "utf8");

  assert.equal(source.includes("export function readinessGateActionFor"), true, "readinessGateActionFor must be exported for Settings fix links");
  // safe mode / kill switch fixes must point at the policy controls where the toggles live
  assert.equal(source.includes("settings#automation-policy"), true);
});

test("automation readiness mapper dedupes setup sources behind canonical gates", async () => {
  const source = await readFile(new URL("automation-readiness.ts", import.meta.url), "utf8");

  for (const expected of [
    "SETUP_DEDUPE_KEYS",
    "search_data",
    "publisher_write",
    "notification_write",
    "AutopilotReadiness.gates",
    "SEOOverview.setup_checklist",
  ]) {
    assert.equal(source.includes(expected), true, `dedupe contract missing ${expected}`);
  }
});
