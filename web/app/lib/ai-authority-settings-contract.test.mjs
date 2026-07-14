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
  const automationPanel = settings.slice(settings.indexOf('id="settings-panel-automation"'), settings.indexOf('activeSettingsTab === "activity"'));
  const aiPanel = settings.slice(settings.indexOf('id="settings-panel-ai-assistance"'), settings.indexOf('activeSettingsTab === "crawl"'));

  assert.match(settings, /id: "ai-assistance", title: "AI assistance"/);
  assert.match(settings, /"opportunity-finding": "automation"/);
  assert.match(aiPanel, /AI assistance: Doctor/);
  assert.match(aiPanel, /doctor_ai_enabled/);
  assert.match(aiPanel, /doctor_ai_run_policy/);
  assert.match(aiPanel, /shared provider credential/i);
  assert.match(aiPanel, /token|cost/i);
  assert.match(aiPanel, /saveDoctorAIAuthority/);
  assert.doesNotMatch(aiPanel, /Opportunities AI run policy/);
  assert.match(automationPanel, /id="opportunity-finding"/);
  assert.match(automationPanel, /Opportunities AI run policy/);
  assert.match(automationPanel, /growth_ai_enabled/);
  assert.match(automationPanel, /growth_ai_run_policy/);
  assert.match(automationPanel, /saveGrowthAIAuthority/);
  assert.match(settings, /Scheduled \+ manual/);
  assert.match(automationPanel, /Run finding/);

  assert.doesNotMatch(settings, /opportunity_finding_source_mix/);
  assert.doesNotMatch(settings, /ai_discovery_automation/);
  assert.doesNotMatch(settings, />\s*Signal Scan\s*</);
  assert.doesNotMatch(settings, />\s*AI Discovery\s*</);
  assert.doesNotMatch(settings, /legacy scheduled authority/i);
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
