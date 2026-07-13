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

test("Canonical site fix helpers describe the closed-loop lifecycle", () => {
  const lib = read("lib/site-fix.ts");
  const progress = read("lib/site-fix-pr-progress.ts");
  for (const snippet of [
    "canonicalSiteFixStatusLabel",
    "canonicalSiteFixNextAction",
    "Awaiting deploy",
    "Create the approved repair pull request",
    "Retry pull request creation",
    "Retry verification",
    "Verified",
  ]) {
    assert.match(`${lib}\n${progress}`, new RegExp(snippet));
  }
});

test("Canonical Site Fixes visibly expose provenance, application, and verification", () => {
  const siteFixes = read("projects/[id]/site-fixes/site-fixes-client.tsx");
  const progress = read("lib/site-fix-pr-progress.ts");
  const contract = `${siteFixes}\n${progress}`;
  for (const snippet of [
    "Finding",
    "Approved",
    "Applied / deploy",
    "Verified",
    "Evidence",
    "Proposed fix",
    "Acceptance checks",
    "Verification",
    "failure_reason",
    "retry_count",
    "application",
  ]) {
    assert.match(contract, new RegExp(snippet));
  }
  assert.doesNotMatch(siteFixes, /I applied this manually/);
  assert.doesNotMatch(siteFixes, /manual_apply_required/);
  assert.match(siteFixes, /href=\{`\/projects\/\$\{projectId\}\/doctor\?finding=\$\{selected\.doctor_finding_id\}`\}/);
  assert.match(siteFixes, /legacy_opportunity_id/);
  assert.match(siteFixes, /legacy_content_action_id/);
  assert.match(siteFixes, /fix\.legacy_aliases\?\.some\(\(alias\) => alias\.object_id === requestedID\)/);
  assert.match(siteFixes, /Legacy provenance/);
  assert.match(siteFixes, /verifications: updated\.verifications \?\? existing\.verifications/);
  assert.match(progress, /case "approved":[\s\S]*case "ready_to_apply":[\s\S]*label: "Create PR"/);
  assert.match(progress, /case "preparing":[\s\S]*failure_reason[\s\S]*label: "Retry PR creation"/);
  assert.match(progress, /busyLabel: "Approving & creating PR\.\.\."/);
  assert.match(progress, /busyLabel: "Creating PR\.\.\."/);
  assert.match(siteFixes, /siteFixPullRequestMutationAction\(selected\)/);
});

test("Site Fix lifecycle never presents deploy or verification-in-progress as verified", () => {
  const siteFixes = read("projects/[id]/site-fixes/site-fixes-client.tsx");
  const progress = read("lib/site-fix-pr-progress.ts");
  const lifecycle = progress.slice(progress.indexOf("export function canonicalSiteFixMilestones"), progress.indexOf("export function shouldPollSiteFixLifecycle"));
  assert.match(lifecycle, /deploymentProvingStatus/);
  assert.match(lifecycle, /status === "verified"/);
  assert.match(lifecycle, /firstIncomplete/);
  assert.doesNotMatch(lifecycle, /applied_at|github_pr_url|github_pr_state|merged_at/);
  assert.match(siteFixes, /const milestones = canonicalSiteFixMilestones\(fix\)/);
  assert.match(siteFixes, /milestone\.complete/);
  assert.match(siteFixes, /milestone\.current/);
});

test("Site fix PR awaiting-merge nag is a subscribable notification event", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");
  assert.match(settings, /"sitefix\.pr\.awaiting_merge": "Site fix PR awaiting merge"/);
});

test("Analysis surfaces approved site fixes as user-visible output", () => {
  // Approved site fixes are now surfaced on the dedicated Site Fixes page.
  const siteFixes = read("projects/[id]/site-fixes/site-fixes-client.tsx");
  for (const snippet of [
    "SiteFix",
    "siteFixes",
    "data-site-fixes-grid",
    "Site Fixes",
    "Proposed fix",
    "canonicalSiteFixNextAction",
    "canonicalSiteFixStatusLabel",
  ]) {
    assert.match(siteFixes, new RegExp(snippet.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  for (const forbidden of ["SEOContentAction", "ResultsAction", "opportunity_status", "measuring", "Growth", "createSiteFixGitHubPR"]) {
    assert.doesNotMatch(siteFixes, new RegExp(forbidden));
  }
});
