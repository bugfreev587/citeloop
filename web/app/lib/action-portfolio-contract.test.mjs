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
    "technical_fix",
    "internal_link_patch",
    "schema_patch",
    "metadata_rewrite",
  ]) {
    assert.match(seo, new RegExp(snippet.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
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
