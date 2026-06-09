import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadDashboardUXLogicModule() {
  const source = await readFile(new URL("./dashboard-ux-logic.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`;
  return import(moduleUrl);
}

test("nextWorkspaceAction prioritizes context before content generation", async () => {
  const { nextWorkspaceAction } = await loadDashboardUXLogicModule();

  const action = nextWorkspaceAction({
    projectId: "project_1",
    hasProfile: false,
    failedPublishCount: 0,
    hasBlockedDrafts: false,
    reviewCount: 0,
    readyCount: 0,
    topicsCount: 0,
  });

  assert.equal(action.title, "Refresh context");
  assert.equal(action.href, "/projects/project_1/context");
  assert.match(action.detail, /before generating/i);
});

test("profilePayloadFromDraft saves structured fields even when advanced JSON is invalid", async () => {
  const { profilePayloadFromDraft } = await loadDashboardUXLogicModule();

  const payload = profilePayloadFromDraft(
    {
      positioning: "Evidence-backed content engine",
      icp: "Growth teams\nFounders",
      value_props: "Turns visibility gaps into content",
      features: "Context extraction",
      differentiators: "Domain-bound evidence",
      competitors: "SuperX",
      key_terms: "GEO\nSEO",
      tone: "Precise",
      banned_claims: "Guaranteed #1 ranking",
      content_rules: "Cite sources",
      advancedJSON: "{not valid json",
    },
    {
      context_confirmed_at: "2026-06-09T00:00:00Z",
      custom_field: "preserve me",
      voice: { legacy: true },
    },
  );

  assert.equal(payload.positioning, "Evidence-backed content engine");
  assert.deepEqual(payload.icp, ["Growth teams", "Founders"]);
  assert.deepEqual(payload.banned_claims, ["Guaranteed #1 ranking"]);
  assert.equal(payload.custom_field, "preserve me");
  assert.equal(payload.context_confirmed_at, "2026-06-09T00:00:00Z");
  assert.deepEqual(payload.voice, { legacy: true, tone: "Precise", rules: ["Cite sources"] });
});

test("profilePayloadFromAdvancedJSON keeps advanced editing explicit", async () => {
  const { profilePayloadFromAdvancedJSON } = await loadDashboardUXLogicModule();

  assert.deepEqual(profilePayloadFromAdvancedJSON('{"custom":true}'), { custom: true });
  assert.throws(() => profilePayloadFromAdvancedJSON("{not valid json"), /Unexpected token|Expected property name/);
});

test("visibilityLifecycleLabel matches real opportunity and content action enums", async () => {
  const { visibilityLifecycleLabel, visibilityLifecycleTone } = await loadDashboardUXLogicModule();

  assert.equal(visibilityLifecycleLabel("open"), "Opportunity detected");
  assert.equal(visibilityLifecycleLabel("accepted"), "Added to Content Plan");
  assert.equal(visibilityLifecycleLabel("converted"), "Added to Content Plan");
  assert.equal(visibilityLifecycleLabel("drafting"), "Draft in progress");
  assert.equal(visibilityLifecycleLabel("ready_for_review"), "Draft waiting for review");
  assert.equal(visibilityLifecycleLabel("approved"), "Approved for publish");
  assert.equal(visibilityLifecycleLabel("measuring"), "Measuring impact");
  assert.equal(visibilityLifecycleLabel("completed"), "Loop closed");
  assert.equal(visibilityLifecycleLabel("done"), "Loop closed");
  assert.equal(visibilityLifecycleLabel("stale"), "Needs re-check");
  assert.equal(visibilityLifecycleTone("failed"), "red");
  assert.equal(visibilityLifecycleTone("completed"), "green");
});
