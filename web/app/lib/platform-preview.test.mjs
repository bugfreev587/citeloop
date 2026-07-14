import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadModule() {
  const source = await readFile(new URL("./platform-preview.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ES2020, target: ts.ScriptTarget.ES2020 } }).outputText;
  return import(`data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`);
}

test("platform preview exposes pinned contract and native fields", async () => {
  const { platformPreview } = await loadModule();
  const preview = platformPreview({
    platform: "reddit", output_type: "community_post", platform_contract_version: "v1",
    platform_metadata: { title: "A workflow", subreddit: "r/saas", post_type: "community_post", flair: "Discussion" },
    contract_validation: { passed: true, failures: [], warnings: [] }, content_md: "Body",
  });
  assert.equal(preview.title, "A workflow");
  assert.match(preview.detailLines.join(" "), /r\/saas/);
  assert.equal(preview.validationPassed, true);
});

test("Hacker News preview is a link package, not an article body", async () => {
  const { platformPreview } = await loadModule();
  const preview = platformPreview({ platform: "hacker_news", output_type: "link_submission", platform_metadata: { title: "How contracts work", url: "{{CANONICAL_URL}}" }, content_md: "" });
  assert.equal(preview.bodyLabel, "No generated comment or article body");
  assert.match(preview.detailLines.join(" "), /CANONICAL_URL/);
  assert.equal(preview.validationPassed, false);
  assert.match(preview.validationMessages.join(" "), /not been validated/i);
});
