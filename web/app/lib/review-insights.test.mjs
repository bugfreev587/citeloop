import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadReviewInsightsModule() {
  const source = await readFile(new URL("./review-insights.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`;
  return import(moduleUrl);
}

test("explainQAIssue translates missing claims into reviewer language", async () => {
  const { explainQAIssue } = await loadReviewInsightsModule();

  const issue = explainQAIssue("qa step failed: parse qa: missing claims");

  assert.equal(issue.title, "QA evidence map was not returned");
  assert.match(issue.detail, /claims array/);
  assert.match(issue.action, /rerun QA/);
});

test("buildSEOContributions summarizes search contribution signals", async () => {
  const { buildSEOContributions, previewPath } = await loadReviewInsightsModule();
  const article = {
    content_md: "# H1\n\n## Why it matters\n\nSee [UniPost](/docs).",
    seo_meta: {
      title: "OAuth Flows Explained",
      meta_description: "Securely connect user social accounts.",
      slug: "oauth-flows-explained",
      h1: "OAuth Flows Explained",
      target_keyword: "oauth social accounts",
    },
    canonical_url: null,
    resolved_slug: null,
  };

  const rows = buildSEOContributions(article);

  assert.equal(previewPath(article), "/blog/oauth-flows-explained");
  assert.equal(rows[0].label, "Search intent");
  assert.equal(rows[0].status, "ready");
  assert.equal(rows[3].value, "oauth-flows-explained");
  assert.equal(rows[5].value, "1 markdown links");
});
