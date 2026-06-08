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

test("articlePreviewBlocks keeps the full article body", async () => {
  const { articlePreviewBlocks } = await loadReviewInsightsModule();
  const content = Array.from({ length: 14 }, (_, index) => `## Section ${index + 1}\n\nBody ${index + 1}`).join("\n\n");

  const blocks = articlePreviewBlocks(content, "Full preview article");

  assert.equal(blocks.length, 29);
  assert.equal(blocks.at(-1), "Body 14");
});

test("qa issue guidance does not ask the reviewer to trigger AI repair manually", async () => {
  const { explainQAIssue } = await loadReviewInsightsModule();

  const issue = explainQAIssue("qa step failed: parse qa: missing claims");

  assert.doesNotMatch(issue.action, /use ai fix/i);
  assert.match(issue.action, /automatic/i);
});

test("shouldAutoRepairArticle catches QA and SEO repairable drafts", async () => {
  const { shouldAutoRepairArticle } = await loadReviewInsightsModule();
  const baseArticle = {
    content_md: "# H1\n\n## Why it matters\n\nBody",
    seo_meta: {
      title: "OAuth Flows Explained",
      meta_description: "Securely connect user social accounts.",
      slug: "oauth-flows-explained",
      h1: "OAuth Flows Explained",
      target_keyword: "oauth social accounts",
    },
    canonical_url: null,
    resolved_slug: null,
    qa_blocking: false,
    repair_attempts: 0,
    requires_human_decision: false,
  };

  assert.equal(shouldAutoRepairArticle(baseArticle), false);
  assert.equal(shouldAutoRepairArticle({ ...baseArticle, qa_blocking: true }), true);
  assert.equal(shouldAutoRepairArticle({ ...baseArticle, seo_meta: { ...baseArticle.seo_meta, target_keyword: "" } }), true);
  assert.equal(shouldAutoRepairArticle({ ...baseArticle, qa_blocking: true, repair_attempts: 2 }), false);
  assert.equal(shouldAutoRepairArticle({ ...baseArticle, qa_blocking: true, requires_human_decision: true }), false);
});
