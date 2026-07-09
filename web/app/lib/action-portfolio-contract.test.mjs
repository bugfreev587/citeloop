import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");

test("SEO action plan exposes normalized portfolio shape", () => {
  const api = read("lib/api.ts");
  for (const snippet of [
    "export type SEOActionPortfolioItem",
    "export type SEOActionPortfolio",
    "portfolio: SEOActionPortfolio",
    "normalizeSEOActionPlan",
    "selected_actions",
    "deferred_actions",
    "rejected_actions",
    "risk_summary",
    "required_approvals",
    "measurement_schedule",
    "action_bucket",
    "review_required",
    "listAutopilotPlans: async",
    "normalizeSEOActionPlan",
  ]) {
    assert.match(api, new RegExp(snippet.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("SEO dashboard renders action portfolio groups", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  for (const snippet of [
    "Action portfolio",
    "Selected actions",
    "Risk summary",
    "Review required",
    "Measurement",
    "latestPortfolioPlan",
    "plan.portfolio.selected_actions",
    "action.action_bucket",
    "action.risk_level",
    "action.review_required",
  ]) {
    assert.match(seo, new RegExp(snippet.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("Analysis distinguishes multi-surface action task types", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  for (const snippet of [
    "Add to Content Plan",
    "Create Page Update",
    "Create Site Fix",
    "Fix Site Issue",
    "Improve Page",
    "Create Content",
    "internal_link_gap",
    "schema_gap",
    "thin_evidence_page",
    "technical_visibility_issue",
    "gsc_query_cannibalization",
    "technical_fix",
    "internal_link_patch",
    "schema_patch",
    "metadata_rewrite",
    "Evidence gap",
    "Cannibalization",
  ]) {
    assert.match(seo, new RegExp(snippet.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("Schema recommendations keep technical CTA ahead of answer-engine copy", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  const match = seo.match(/function opportunityWorkType[\s\S]*?function actionCtaForOpportunity/);
  assert.ok(match, "opportunityWorkType body should be readable");
  const body = match[0];
  const schemaIndex = body.indexOf('type === "schema_gap"');
  const technicalIndex = body.indexOf('type === "technical_visibility_issue"');
  const createContentIndex = body.indexOf('return "Create Content"');
  assert.ok(schemaIndex >= 0 && technicalIndex >= 0 && createContentIndex >= 0, "work type body should include schema, technical, and content fallback branches");
  assert.ok(schemaIndex < createContentIndex, "schema_gap must route to Fix Site Issue before content fallback");
  assert.ok(technicalIndex < createContentIndex, "technical_visibility_issue must route to Fix Site Issue before content fallback");
});

test("Action cards expose why, contribution, output type, and execution result", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  for (const snippet of [
    "actionWhyNowText",
    "actionSEOContributionText",
    "actionOutputTypeLabel",
    "actionPostExecutionText",
    "Why now",
    "SEO/GEO contribution",
    "Output type",
    "After execution",
  ]) {
    assert.match(seo, new RegExp(snippet.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("Merged and closed site fix PRs surface in the after-execution status", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  assert.match(seo, /siteFixPublisherResultStatus\(action\) === "github_pr_merged"/);
  assert.match(seo, /PR merged — verifying in production/);
  assert.match(seo, /siteFixPublisherResultStatus\(action\) === "github_pr_closed"/);
  assert.match(seo, /PR closed without merging/);
});

test("Analysis surfaces approved site fixes as user-visible output", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  for (const snippet of [
    "isDirectAction",
    "directReviewActions",
    "data-site-fixes-queue",
    "Site Fixes",
    "Reviewable output",
    "actionOutputPreviewText",
    "actionOutputTypeLabel(action)",
    "actionSEOContributionText(action)",
    "actionWhyNowText(action)",
  ]) {
    assert.match(seo, new RegExp(snippet.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});
