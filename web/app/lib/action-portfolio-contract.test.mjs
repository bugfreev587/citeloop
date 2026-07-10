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
  // The after-execution status copy now lives in the shared site-fix helper
  // (actionPostExecutionText), consumed by both the analysis and Site Fixes UIs.
  const lib = read("lib/site-fix.ts");
  assert.match(lib, /siteFixPublisherResultStatus\(action\) === "github_pr_merged"/);
  assert.match(lib, /PR merged — verifying in production/);
  assert.match(lib, /siteFixPublisherResultStatus\(action\) === "github_pr_closed"/);
  assert.match(lib, /PR closed without merging/);
  assert.match(lib, /siteFixPublisherResultStatus\(action\) === "needs_follow_up"/);
  assert.match(lib, /siteFixFollowUpReason\(action\) \|\| "Needs follow-up/);
});

test("Verified site fixes expose automatic verification and accurate PR actions", () => {
  const lib = read("lib/site-fix.ts");
  const siteFixes = read("projects/[id]/site-fixes/site-fixes-client.tsx");
  for (const snippet of ["siteFixVerificationLabel", "Verified automatically", "siteFixPRLinkLabel", "View merged PR"]) {
    assert.match(lib, new RegExp(snippet));
  }
  assert.match(siteFixes, /siteFixVerificationLabel/);
  assert.match(siteFixes, /siteFixPRLinkLabel/);
  assert.match(siteFixes, /!drawerAction\.verified_at/);

  const linkLabel = lib.match(/export function siteFixPRLinkLabel[\s\S]*?\n}/)?.[0] ?? "";
  const closedIndex = linkLabel.indexOf('state === "closed"');
  const openIndex = linkLabel.indexOf('state === "open"');
  const mergedIndex = linkLabel.indexOf('state === "merged"');
  const verifiedFallbackIndex = linkLabel.indexOf("action.verified_at");
  assert.ok(closedIndex >= 0, "closed PR state should have an explicit label");
  assert.ok(openIndex > closedIndex, "open PR state should be checked after closed state");
  assert.ok(mergedIndex > openIndex, "merged PR state should be checked after open state");
  assert.ok(verifiedFallbackIndex > mergedIndex, "verified_at should only be a fallback after explicit PR states");
});

test("Site fix PR awaiting-merge nag is a subscribable notification event", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");
  assert.match(settings, /"sitefix\.pr\.awaiting_merge": "Site fix PR awaiting merge"/);
});

test("Analysis surfaces approved site fixes as user-visible output", () => {
  // Approved site fixes are now surfaced on the dedicated Site Fixes page.
  const siteFixes = read("projects/[id]/site-fixes/site-fixes-client.tsx");
  for (const snippet of [
    "isDirectAction",
    "visibleSiteFixes",
    "data-site-fixes-grid",
    "Site Fixes",
    "Reviewable output",
    "actionOutputPreviewText",
    "actionOutputTypeLabel(drawerAction)",
    "actionSEOContributionText(drawerAction)",
    "actionWhyNowText(action)",
  ]) {
    assert.match(siteFixes, new RegExp(snippet.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});
