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

test("articlePreviewHref points drafts to a real standalone preview route", async () => {
  const { articlePreviewHref } = await loadReviewInsightsModule();
  const article = {
    id: "article-1",
    content_md: "# Draft\n\nBody",
    seo_meta: { slug: "draft" },
    canonical_url: null,
    resolved_slug: "draft",
  };

  assert.equal(articlePreviewHref("project-1", article), "/preview/projects/project-1/articles/article-1");
  assert.equal(
    articlePreviewHref("project-1", { ...article, canonical_url: "https://example.com/live-draft" }),
    "https://example.com/live-draft",
  );
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

test("reviewQueueSummary separates ready, auto-repair, and human-decision work", async () => {
  const { reviewQueueSummary } = await loadReviewInsightsModule();
  const ready = {
    id: "ready-article",
    content_md: "# Ready\n\n## Body\n\nText",
    seo_meta: {
      title: "Ready article",
      meta_description: "Ready description",
      slug: "ready-article",
      h1: "Ready article",
      target_keyword: "ready keyword",
    },
    canonical_url: null,
    resolved_slug: null,
    qa_blocking: false,
    repair_attempts: 0,
    repair_status: "idle",
    requires_human_decision: false,
    qa_feedback: {},
  };
  const repairable = {
    ...ready,
    id: "repairable-article",
    qa_blocking: true,
    repair_attempts: 0,
    repair_status: "idle",
  };
  const activelyRepairing = {
    ...ready,
    id: "repairing-article",
    qa_blocking: true,
    repair_attempts: 1,
    repair_status: "repairing",
  };
  const exhausted = {
    ...ready,
    id: "exhausted-article",
    qa_blocking: true,
    repair_attempts: 2,
    repair_status: "exhausted",
  };
  const humanDecision = {
    ...ready,
    id: "human-decision-article",
    qa_blocking: true,
    repair_attempts: 1,
    repair_status: "human_decision",
    requires_human_decision: true,
  };

  const summary = reviewQueueSummary([
    { topic_id: "topic-a", articles: [ready, repairable, activelyRepairing] },
    { topic_id: "topic-b", articles: [exhausted, humanDecision] },
  ]);

  assert.equal(summary.total, 5);
  assert.equal(summary.bundleCount, 2);
  assert.equal(summary.ready, 1);
  assert.equal(summary.autoRepair, 2);
  assert.equal(summary.needsHuman, 2);
  assert.equal(summary.blocked, 4);
});

test("reviewArticleState keeps repair-exhausted drafts in the human bucket", async () => {
  const { reviewArticleState } = await loadReviewInsightsModule();
  const baseArticle = {
    content_md: "# Draft\n\n## Body\n\nText",
    seo_meta: {
      title: "Draft",
      meta_description: "Draft description",
      slug: "draft",
      h1: "Draft",
      target_keyword: "draft keyword",
    },
    canonical_url: null,
    resolved_slug: null,
    qa_blocking: true,
    repair_attempts: 2,
    repair_status: "exhausted",
    requires_human_decision: false,
    qa_feedback: {},
  };

  const state = reviewArticleState(baseArticle);

  assert.equal(state.kind, "needs_human");
  assert.equal(state.approvable, false);
  assert.match(state.detail, /repair budget/i);
});

test("qaClaimRows reads the QA evidence map without inventing evidence labels", async () => {
  const { qaClaimRows } = await loadReviewInsightsModule();
  const article = {
    qa_feedback: {
      claims: [
        { claim: "UniPost supports hosted OAuth.", mapped: true, evidence: "Feature page says UniPost supports hosted OAuth." },
        { claim: "UniPost is SOC 2 ready.", mapped: false, evidence: "" },
      ],
    },
    qa_issues: ["unmapped product claim: UniPost is SOC 2 ready."],
  };

  const rows = qaClaimRows(article);

  assert.equal(rows.length, 2);
  assert.deepEqual(rows.map((row) => row.status), ["mapped", "unmapped"]);
  assert.equal(rows[0].evidence, "Feature page says UniPost supports hosted OAuth.");
  assert.equal(rows[1].claim, "UniPost is SOC 2 ready.");
});

test("searchAppearanceRows only uses current SEO metadata", async () => {
  const { searchAppearanceRows } = await loadReviewInsightsModule();
  const article = {
    content_md: "# OAuth Consent\n\n## Body\n\nText",
    seo_meta: {
      title: "OAuth consent screen best practices",
      meta_description: "Write consent copy users can understand.",
      slug: "oauth-consent-screen-best-practices",
      h1: "OAuth Consent",
      target_keyword: "oauth consent screen",
    },
    canonical_url: null,
    resolved_slug: null,
  };

  const rows = searchAppearanceRows(article);

  assert.deepEqual(
    rows.map((row) => row.label),
    ["Target keyword", "Result title", "Description", "URL slug"],
  );
  assert.equal(rows[0].value, "oauth consent screen");
  assert.equal(rows[3].value, "oauth-consent-screen-best-practices");
  assert.equal(rows.some((row) => /secondary query|snippet promise|search demand/i.test(row.label + row.value)), false);
});
