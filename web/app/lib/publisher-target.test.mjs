import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadPublisherTargetModule() {
  const source = await readFile(new URL("./publisher-target.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`;
  return import(moduleUrl);
}

test("deriveGitHubBranch maps UniPost project domains to deployment branches", async () => {
  const { deriveGitHubBranch } = await loadPublisherTargetModule();

  assert.equal(deriveGitHubBranch("https://dev.unipost.dev/"), "dev");
  assert.equal(deriveGitHubBranch("staging.unipost.dev"), "staging");
  assert.equal(deriveGitHubBranch("https://unipost.dev/blog"), "main");
  assert.equal(deriveGitHubBranch("https://customer.example"), "");
});

test("derivePublishTarget uses the project domain and content leaf", async () => {
  const { derivePublishTarget } = await loadPublisherTargetModule();

  assert.deepEqual(derivePublishTarget("https://staging.unipost.dev/", "content/citeloop/blog"), {
    baseURL: "https://staging.unipost.dev/blog",
    branch: "staging",
  });
  assert.deepEqual(derivePublishTarget("customer.example", "content/articles"), {
    baseURL: "https://customer.example/articles",
    branch: "",
  });
  assert.deepEqual(derivePublishTarget("https://unipost.dev/blog", "content/citeloop/blog"), {
    baseURL: "https://unipost.dev/blog",
    branch: "main",
  });
});
