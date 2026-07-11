import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const root = path.dirname(new URL(import.meta.url).pathname);
const read = (relative) => fs.readFileSync(path.join(root, relative), "utf8");

test("project config exposes independent Doctor and Opportunities AI authority", () => {
  const api = read("api.ts");
  for (const field of [
    "growth_signal_enabled",
    "growth_ai_enabled",
    "growth_ai_run_policy",
    "doctor_ai_enabled",
    "doctor_ai_run_policy",
    "capability_policy_version",
  ]) {
    assert.match(api, new RegExp(field));
  }
});

test("settings replaces legacy discovery modes with independent AI consent controls", () => {
  const settings = read("../projects/[id]/settings/settings-client.tsx");
  const panel = settings.slice(settings.indexOf('id="settings-panel-ai-assistance"'));

  assert.match(settings, /id: "ai-assistance", title: "AI assistance"/);
  assert.match(panel, /AI assistance: Doctor/);
  assert.match(panel, /Opportunities/);
  assert.match(panel, /doctor_ai_enabled/);
  assert.match(panel, /doctor_ai_run_policy/);
  assert.match(panel, /growth_ai_enabled/);
  assert.match(panel, /growth_ai_run_policy/);
  assert.match(panel, /shared provider credential/i);
  assert.match(panel, /token|cost/i);
  assert.match(panel, /saveDoctorAIAuthority/);
  assert.match(panel, /saveGrowthAIAuthority/);

  assert.doesNotMatch(panel, /opportunity_finding_source_mix/);
  assert.doesNotMatch(panel, /ai_discovery_automation/);
  assert.doesNotMatch(panel, />\s*Signal Scan\s*</);
  assert.doesNotMatch(panel, />\s*AI Discovery\s*</);
});

test("each AI line saves only its own authority fields", () => {
  const settings = read("../projects/[id]/settings/settings-client.tsx");
  const doctorSave = settings.slice(settings.indexOf("async function saveDoctorAIAuthority"), settings.indexOf("async function saveGrowthAIAuthority"));
  const growthSave = settings.slice(settings.indexOf("async function saveGrowthAIAuthority"), settings.indexOf("async function", settings.indexOf("async function saveGrowthAIAuthority") + 20));

  assert.match(doctorSave, /doctor_ai_enabled/);
  assert.match(doctorSave, /doctor_ai_run_policy/);
  assert.doesNotMatch(doctorSave, /growth_ai_enabled/);
  assert.match(growthSave, /growth_ai_enabled/);
  assert.match(growthSave, /growth_ai_run_policy/);
  assert.doesNotMatch(growthSave, /doctor_ai_enabled/);
});
