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
