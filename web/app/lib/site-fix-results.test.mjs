import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadModule() {
  let source = "export {};";
  try {
    source = await readFile(new URL("./site-fix-results.ts", import.meta.url), "utf8");
  } catch {
    // The first TDD run deliberately exercises the missing helper module.
  }
  const transpiled = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ES2020, target: ts.ScriptTarget.ES2020 },
  }).outputText;
  return import(`data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`);
}

test("Site Fix outcomes contribute through a dedicated taxonomy", async () => {
  const results = await loadModule();
  assert.equal(typeof results.siteFixMeasurementOutcomeState, "function");
  assert.deepEqual(results.siteFixMeasurementOutcomeState({ status: "terminal", terminal_outcome: "positive" }), {
    key: "positive",
    attention: false,
  });
  assert.deepEqual(results.siteFixMeasurementOutcomeState({ status: "terminal", terminal_outcome: "negative" }), {
    key: "negative",
    attention: true,
  });
  assert.deepEqual(results.siteFixMeasurementOutcomeState({ status: "observing" }), {
    key: "waiting",
    attention: false,
  });
});

test("Site Fix queue states remain independent from ContentAction helpers", async () => {
  const results = await loadModule();
  assert.equal(typeof results.siteFixMeasurementQueueState, "function");
  assert.equal(results.siteFixMeasurementQueueState({ status: "observing" }), "waiting");
  assert.equal(results.siteFixMeasurementQueueState({ status: "ready" }), "too_early");
  assert.equal(results.siteFixMeasurementQueueState({ status: "baseline_blocked" }), "blocked");
  assert.equal(results.siteFixMeasurementQueueState({ status: "failed_terminal" }), "blocked");
  assert.equal(results.siteFixMeasurementQueueState({ status: "terminal" }), "completed");
});

const siteFixSummary = (id, title) => ({
  source_type: "site_fix",
  id,
  site_fix_id: `fix-${id}`,
  title,
  status: "ready",
});

test("Site Fix deep-link pin survives list-before-detail completion and stays in the first 12 cards", async () => {
  const results = await loadModule();
  assert.equal(typeof results.createSiteFixDeepLinkState, "function");
  assert.equal(typeof results.reduceSiteFixDeepLinkState, "function");
  assert.equal(typeof results.pinSiteFixResultsSummary, "function");

  const target = siteFixSummary("measurement-target", "Target from list");
  const ordinaryRows = Array.from({ length: 15 }, (_, index) => ({
    source_type: "content_action",
    id: `action-${index}`,
    action_type: "publish",
  }));
  let handoff = results.createSiteFixDeepLinkState(target.id);

  handoff = results.reduceSiteFixDeepLinkState(handoff, { type: "feed_settled", items: ordinaryRows });
  assert.equal(handoff.status, "pending");

  handoff = results.reduceSiteFixDeepLinkState(handoff, {
    type: "detail_succeeded",
    detail: { measurement: target },
  });
  assert.equal(handoff.status, "resolved");
  assert.equal(handoff.pinnedSummary.id, target.id);
  const visible = results.pinSiteFixResultsSummary(ordinaryRows, handoff.pinnedSummary);
  assert.equal(visible[0].id, target.id);
  assert.equal(visible.slice(0, 12).some((item) => item.id === target.id), true);
  assert.equal(visible.filter((item) => item.id === target.id).length, 1);
});

test("Site Fix deep-link pin survives detail-before-list completion and a later feed refresh", async () => {
  const results = await loadModule();
  const target = siteFixSummary("measurement-target", "Target from detail");
  const refreshedTarget = siteFixSummary(target.id, "Stale list copy");
  const refreshedRows = [
    ...Array.from({ length: 15 }, (_, index) => ({
      source_type: "content_action",
      id: `action-${index}`,
      action_type: "publish",
    })),
    refreshedTarget,
  ];
  let handoff = results.createSiteFixDeepLinkState(target.id);

  handoff = results.reduceSiteFixDeepLinkState(handoff, {
    type: "detail_succeeded",
    detail: { measurement: target },
  });
  assert.equal(handoff.status, "resolved");
  assert.equal(handoff.pinnedSummary.title, "Target from detail");

  handoff = results.reduceSiteFixDeepLinkState(handoff, { type: "feed_settled", items: refreshedRows });
  const visible = results.pinSiteFixResultsSummary(refreshedRows, handoff.pinnedSummary);
  assert.equal(visible[0].title, "Target from detail");
  assert.equal(visible.slice(0, 12).some((item) => item.id === target.id), true);
  assert.equal(visible.filter((item) => item.id === target.id).length, 1);
});

test("Site Fix deep-link failure is terminal only after both detail and feed miss", async () => {
  const results = await loadModule();
  let handoff = results.createSiteFixDeepLinkState("missing-measurement");

  handoff = results.reduceSiteFixDeepLinkState(handoff, { type: "detail_failed" });
  assert.equal(handoff.status, "pending");
  handoff = results.reduceSiteFixDeepLinkState(handoff, { type: "feed_settled", items: [] });
  assert.equal(handoff.status, "failed");
});

test("Site Fix deep-link consumes a settled feed only once while detail is pending", async () => {
  const results = await loadModule();
  const initial = results.createSiteFixDeepLinkState("off-page-measurement");
  const settled = results.reduceSiteFixDeepLinkState(initial, { type: "feed_settled", items: [] });
  const duplicate = results.reduceSiteFixDeepLinkState(settled, { type: "feed_settled", items: [] });

  assert.equal(settled.status, "pending");
  assert.equal(settled.feedSettled, true);
  assert.equal(duplicate, settled);
});
