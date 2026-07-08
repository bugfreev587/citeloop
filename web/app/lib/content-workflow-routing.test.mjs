import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadContentWorkflowRoutingModule() {
  const source = await readFile(new URL("./content-workflow-routing.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`;
  return import(moduleUrl);
}

test("active content workflow step prefers the section nearest the target line", async () => {
  const { selectActiveContentWorkflowStep } = await loadContentWorkflowRoutingModule();

  assert.equal(
    selectActiveContentWorkflowStep(
      [
        { step: "plan", top: 0 },
        { step: "review", top: 120 },
        { step: "publish", top: 220 },
      ],
      { marker: 260, targetTopOffset: 24 },
    ),
    "plan",
  );

  assert.equal(
    selectActiveContentWorkflowStep(
      [
        { step: "plan", top: -360 },
        { step: "review", top: 24 },
        { step: "publish", top: 140 },
      ],
      { marker: 260, targetTopOffset: 24 },
    ),
    "review",
  );
});

test("active content workflow step stays on plan before any section crosses the marker", async () => {
  const { selectActiveContentWorkflowStep } = await loadContentWorkflowRoutingModule();

  assert.equal(
    selectActiveContentWorkflowStep(
      [
        { step: "plan", top: 320 },
        { step: "review", top: 620 },
        { step: "publish", top: 920 },
      ],
      { marker: 260, targetTopOffset: 24 },
    ),
    "plan",
  );
});
