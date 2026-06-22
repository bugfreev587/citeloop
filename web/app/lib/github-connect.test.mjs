import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadGithubConnectModule() {
  const source = await readFile(new URL("./github-connect.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`;
  return import(moduleUrl);
}

function memoryStorage() {
  const values = new Map();
  return {
    getItem(key) {
      return values.has(key) ? values.get(key) : null;
    },
    setItem(key, value) {
      values.set(key, String(value));
    },
    removeItem(key) {
      values.delete(key);
    },
  };
}

test("GitHub callback prefers state but can recover the project from the connect handoff", async () => {
  const originalWindow = globalThis.window;
  const storage = memoryStorage();
  globalThis.window = { localStorage: storage };
  try {
    const { rememberGithubConnectProject, resolveGithubCallbackProjectID, forgetGithubConnectProject } = await loadGithubConnectModule();

    rememberGithubConnectProject("project-from-drawer");

    assert.equal(resolveGithubCallbackProjectID("project-from-state"), "project-from-state");
    assert.equal(resolveGithubCallbackProjectID(""), "project-from-drawer");

    forgetGithubConnectProject();
    assert.equal(resolveGithubCallbackProjectID(""), "");
  } finally {
    if (originalWindow === undefined) {
      delete globalThis.window;
    } else {
      globalThis.window = originalWindow;
    }
  }
});
