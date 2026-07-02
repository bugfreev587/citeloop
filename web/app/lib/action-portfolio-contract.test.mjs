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
    "Create content task",
    "Create refresh task",
    "Create technical task",
    "Create internal-link task",
    "Create GEO asset task",
    "Create evidence refresh task",
    "Create consolidation task",
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
  const match = seo.match(/function actionCtaForOpportunity[\s\S]*?function sourceModeForOpportunity/);
  assert.ok(match, "actionCtaForOpportunity body should be readable");
  const body = match[0];
  const schemaIndex = body.indexOf('type === "schema_gap"');
  const technicalIndex = body.indexOf('type === "technical_visibility_issue"');
  const geoIndex = body.indexOf("Create GEO asset task");
  assert.ok(schemaIndex >= 0 && technicalIndex >= 0 && geoIndex >= 0, "CTA body should include schema, technical, and GEO branches");
  assert.ok(schemaIndex < geoIndex, "schema_gap must route to technical task before generic answer-engine GEO copy");
  assert.ok(technicalIndex < geoIndex, "technical_visibility_issue must route to technical task before generic answer-engine GEO copy");
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

test("Analysis surfaces reviewable direct actions as user-visible output", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  for (const snippet of [
    "isDirectAction",
    "directReviewActions",
    "data-direct-action-queue",
    "Direct action queue",
    "Reviewable output",
    "actionOutputPreviewText",
    "actionOutputTypeLabel(action)",
    "actionSEOContributionText(action)",
    "actionWhyNowText(action)",
  ]) {
    assert.match(seo, new RegExp(snippet.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});
