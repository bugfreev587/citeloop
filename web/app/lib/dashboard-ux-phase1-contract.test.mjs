import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");
const exists = (relativePath) => fs.existsSync(path.join(appRoot, relativePath));

test("project shell uses user-facing Phase 1 navigation and hides Runs from primary nav", () => {
  const shell = read("components/project-shell.tsx");

  for (const label of ["Context", "Content Plan", "Review", "Publish", "Visibility", "Settings", "Admin"]) {
    assert.match(shell, new RegExp(`label: "${label}"`));
  }

  for (const legacy of [
    'label: "Knowledge"',
    'label: "Topics"',
    'label: "Publishing"',
    'label: "SEO"',
    'label: "Runs"',
  ]) {
    assert.doesNotMatch(shell, new RegExp(legacy.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("project shell hides the sidebar primary action when it repeats the active page", () => {
  const shell = read("components/project-shell.tsx");

  assert.match(shell, /showPrimaryAction/);
  assert.match(shell, /primaryAction !== null && primaryAction\.href !== pathname/);
  assert.match(shell, /\{primaryAction && showPrimaryAction && \(/);
});

test("project shell never renders an Open Home sidebar primary action", () => {
  const shell = read("components/project-shell.tsx");
  const dashboardLogic = read("lib/dashboard-ux-logic.ts");

  assert.match(shell, /WorkspaceAction \| null/);
  assert.match(shell, /return null;/);
  assert.doesNotMatch(shell, /title: "Open Home"/);
  assert.doesNotMatch(dashboardLogic, /title: "Open Home"/);
});

test("project shell groups desktop navigation into SuperX-style sections", () => {
  const shell = read("components/project-shell.tsx");

  assert.match(shell, /const navSections = \[/);
  assert.match(shell, /id: "primary"[\s\S]*label: null[\s\S]*label: "Home"[\s\S]*label: "Context"[\s\S]*label: "Content Plan"/);
  assert.match(shell, /id: "create"[\s\S]*label: "CREATE"[\s\S]*label: "Review"[\s\S]*label: "Publish"/);
  for (const label of ["CREATE", "MEASURE", "SYSTEM"]) {
    assert.match(shell, new RegExp(`label: "${label}"`));
  }
  assert.match(shell, /navSections\s*\n\s*\.map/);
  assert.match(shell, /visibleNavSections\.map/);
  assert.match(shell, /section\.items\.map/);
  assert.match(shell, /tracking-\[0\.18em\]/);
});

test("project shell keeps the fixed-width sidebar primary action to one line when shown", () => {
  const shell = read("components/project-shell.tsx");

  assert.match(shell, /overflow-hidden/);
  assert.match(shell, /truncate whitespace-nowrap/);
  assert.match(shell, /className="shrink-0"/);
});

test("project shell feeds route and opportunity state into the sidebar CTA", () => {
  const shell = read("components/project-shell.tsx");

  assert.match(shell, /api\.listSEOOpportunities\(projectId, \{ status: "open", limit: 10 \}\)/);
  assert.match(shell, /openOpportunityCount/);
  assert.match(shell, /currentPathname: pathname/);
  assert.match(shell, /\[actionSummary, pathname, projectId\]/);
});

test("project shell uses the review-width canvas for every project page", () => {
  const shell = read("components/project-shell.tsx");

  assert.match(shell, /max-w-\[1560px\]/);
  assert.match(shell, /max-w-\[1320px\]/);
  assert.doesNotMatch(shell, /reviewRoute \?/);
  assert.doesNotMatch(shell, /max-w-5xl/);
  assert.doesNotMatch(shell, /max-w-\[960px\]/);
});

test("internal nav entries are hidden when the user cannot access settings, avoiding a 404 dead-door", () => {
  const shell = read("components/project-shell.tsx");
  // Shell must accept and apply a canAccessSettings gate so non-admin users do not see internal entries that 404.
  assert.match(shell, /canAccessSettings/);
  assert.match(shell, /adminOnlyNavLeaves/);
  assert.match(shell, /!adminOnlyNavLeaves\.has\(item\.href\) \|\| canAccessSettings/);
  assert.match(shell, /visibleNav\.map/);

  const layout = read("projects/[id]/layout.tsx");
  assert.match(layout, /canUseInternalTools/);
  assert.match(layout, /canAccessSettings=\{canAccessSettings\}/);
});

test("context and home surface a background-crawl completion signal instead of stranding the user", () => {
  const context = read("projects/[id]/knowledge/knowledge-client.tsx");
  assert.match(context, /backgroundCrawl/);
  assert.match(context, /api\.listInventory\(projectId\)/);
  assert.match(context, /updates automatically when they finish/);
  assert.match(context, /will appear here automatically/);

  const workspace = read("projects/[id]/workspace.tsx");
  // Home polls for profile + inventory so a freshly created project flips to a ready state on its own.
  assert.match(workspace, /contextBuild\.active/);
  assert.match(workspace, /api\.listInventory\(projectId\)/);
  assert.match(workspace, /Your domain context is ready/);
});

test("home shows parallel context-building tracks during onboarding", () => {
  const workspace = read("projects/[id]/workspace.tsx");
  const dashboardLogic = read("lib/dashboard-ux-logic.ts");

  assert.match(workspace, /contextBuildTracks/);
  assert.match(workspace, /onboardingPollCount/);
  assert.match(workspace, /Parallel context build/);
  assert.match(workspace, /Building domain context/);
  assert.match(dashboardLogic, /Product profile/);
  assert.match(dashboardLogic, /Source crawl/);
  assert.match(dashboardLogic, /Evidence snippets/);
  assert.match(workspace, /contextBuild\.tracks\.map/);
  assert.doesNotMatch(workspace, /Estimated progress/);
});

test("visibility defaults to opportunity review and keeps measurement details secondary", () => {
  const visibility = read("projects/[id]/seo/seo-client.tsx");
  assert.match(visibility, /Review opportunities/);
  assert.match(visibility, /need review/);
  assert.match(visibility, /Add to Content Plan/);
  assert.match(visibility, /Measurement and diagnostics/);
  assert.doesNotMatch(visibility, /The numbers below are placeholders/);
  assert.doesNotMatch(visibility, /Showing \{loopRows\.length\} of \{loopTotal\}/);

  const context = read("projects/[id]/knowledge/knowledge-client.tsx");
  // Evidence and source lists stay compact on the page and move full lists into drawers.
  assert.match(context, /Show all \$\{evidenceRows\.length\}/);
  assert.match(context, /Show all \$\{filtered\.length\}/);
  assert.match(context, /evidencePreviewRows/);
  assert.match(context, /sourcePreviewRows/);
  assert.match(context, /activeDrawer/);
  assert.match(context, /DrawerPanel/);
  assert.doesNotMatch(context, /showAllEvidence/);
  assert.doesNotMatch(context, /Show fewer/);
});

test("context profile editors collapse after saving", () => {
  const context = read("projects/[id]/knowledge/knowledge-client.tsx");

  assert.match(context, /profileEditorOpen/);
  assert.match(context, /voiceEditorOpen/);
  assert.match(context, /setProfileEditorOpen\(false\)/);
  assert.match(context, /setVoiceEditorOpen\(false\)/);
  assert.match(context, /Edit Domain profile/);
  assert.match(context, /Edit Voice & rules/);
});

test("settings maps raw errors to user copy, confirms a budget pause, and drops dev jargon", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");
  assert.match(settings, /function friendlyError/);
  assert.match(settings, /detail: friendlyError\(e\.message\)/);
  // Budget -> $0 pauses automation; it must confirm first.
  assert.match(settings, /pauses all automated generation/);
  // The internal "PUT /config replaces the entire config" notice should be gone.
  assert.doesNotMatch(settings, /replaces the entire config/);
});

test("review page is built around automatic recovery, not manual triage", () => {
  const review = read("projects/[id]/review/review-client.tsx");
  const articleDetail = read("projects/[id]/articles/[articleId]/article-detail-client.tsx");
  const previewRouteExists = exists("preview/projects/[id]/articles/[articleId]/page.tsx");
  // Three honest states: only genuine decisions reach the human; everything else
  // is ready or being handled automatically.
  assert.match(review, /Ready to approve/);
  assert.match(review, /Needs your decision/);
  assert.match(review, /CiteLoop is handling/);
  assert.match(review, /No action needed/);
  assert.match(review, /Claim evidence map/);
  assert.match(review, /How this article appears in search/);
  assert.match(review, /Preview/);
  assert.match(review, /reviewQueueSummary/);
  assert.match(review, /selectedArticleId/);
  assert.match(review, /articlePreviewHref/);
  assert.equal(previewRouteExists, true);
  assert.match(review, /Edit draft/);
  assert.match(review, /Reject/);
  // The dead "Resolve" button and the raw QA jargon it exposed are gone.
  assert.doesNotMatch(review, /Fix evidence in Context/);
  assert.doesNotMatch(review, /Fixing evidence/);
  assert.doesNotMatch(review, /Auto repair active/);
  assert.doesNotMatch(review, />\s*Resolve\s*</);
  assert.doesNotMatch(review, /QA evidence map was not returned/);
  assert.doesNotMatch(review, /Web preview/);
  assert.doesNotMatch(review, /qa blocking/);
  assert.match(articleDetail, /Cannot approve:/);
  assert.doesNotMatch(articleDetail, /qa blocking/);
});

test("destructive content-plan and distribution actions confirm before running", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");
  // Archive and schedule-clear are reversible-but-surprising; both must confirm.
  assert.match(topics, /Remove .* from the content plan\?/);
  assert.match(topics, /Clear the scheduled date/);

  // Distribution lives on the Publish page now, not on Home; the confirm stays there.
  const publishing = read("projects/[id]/publishing/publishing-client.tsx");
  assert.match(publishing, /Mark this variant as distributed\?/);
});

test("renamed dashboard routes exist and legacy routes redirect", () => {
  for (const route of [
    "projects/[id]/context/page.tsx",
    "projects/[id]/plan/page.tsx",
    "projects/[id]/publish/page.tsx",
    "projects/[id]/visibility/page.tsx",
    "projects/[id]/settings/activity/page.tsx",
  ]) {
    assert.equal(exists(route), true, `${route} should exist`);
  }

  const redirects = new Map([
    ["projects/[id]/knowledge/page.tsx", "/context"],
    ["projects/[id]/topics/page.tsx", "/plan"],
    ["projects/[id]/publishing/page.tsx", "/publish"],
    ["projects/[id]/seo/page.tsx", "/visibility"],
    ["projects/[id]/runs/page.tsx", "/settings/activity"],
  ]);

  for (const [route, target] of redirects) {
    const source = read(route);
    assert.match(source, /redirect\(/, `${route} should redirect`);
    assert.match(source, new RegExp(target.replace("/", "\\/")), `${route} should redirect to ${target}`);
  }
});

test("home leads with a single next step and does not show run internals by default", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const copy of [
    "Your next step",
    "primaryAction",
    "nextWorkspaceAction",
    "AI citations",
    "Organic traffic",
    "Published pages",
    "Growth loop",
  ]) {
    assert.match(workspace, new RegExp(copy));
  }

  for (const internalCopy of ["Run Insight", "Run Strategist", "Publish tick", "Recent runs", "tokens"]) {
    assert.doesNotMatch(workspace, new RegExp(internalCopy));
  }
});

test("home removes manual planning controls and secondary growth panels", () => {
  const workspace = read("projects/[id]/workspace.tsx");
  const dashboardLogic = read("lib/dashboard-ux-logic.ts");

  for (const removed of [
    "Next growth move",
    "Measurement coverage",
    "Refresh context",
    "Generate content plan",
    "Review drafts to unlock growth",
    "TextInput",
    "Wand2",
    "api.runInsight",
    "api.runStrategist",
    "nextGrowthMove",
    "measurementCoverage",
  ]) {
    assert.doesNotMatch(workspace, new RegExp(removed.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  assert.doesNotMatch(dashboardLogic, /Refresh context when product facts change/);
});

test("home growth metrics use a slim honest strip without a fake trend chart", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const copy of [
    "growthMetricCards",
    "MetricIcon",
    "AI citations",
    "Organic traffic",
    "Published pages",
    "In motion",
  ]) {
    assert.match(workspace, new RegExp(copy));
  }

  // The decorative hardcoded SVG growth curve is removed — no fake data on Home.
  assert.doesNotMatch(workspace, /growthTrendPath/);
  assert.doesNotMatch(workspace, /Growth metric trend/);
  assert.doesNotMatch(workspace, /growthMetricFill/);
});

test("home explains growth status and loop stages from existing product data", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const copy of [
    "Connect Search Console for traffic",
    "Connect for proof",
    "Context",
    "Opportunities",
    "Plan",
    "Drafts",
    "Review",
    "Publish",
    "Measure",
    "Needs you",
    "Activity",
  ]) {
    assert.match(workspace, new RegExp(copy));
  }

  for (const apiCall of [
    "api.getProfile(projectId)",
    "api.listInventory(projectId)",
    "api.getSEOOverview(projectId)",
    'api.listSEOOpportunities(projectId, { status: "open", limit: 50 })',
    "api.listSEOContentActions(projectId, { limit: 50 })",
  ]) {
    assert.match(workspace, new RegExp(apiCall.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.doesNotMatch(workspace, /Results \/ Momentum/);
  assert.doesNotMatch(workspace, /Actionable momentum/);
  assert.doesNotMatch(workspace, /Recent growth signals/);
});

test("home keeps every loop card fresh from page-level state", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  assert.match(workspace, /HOME_REFRESH_MS/);
  assert.match(workspace, /window\.setInterval\(refresh, HOME_REFRESH_MS\)/);
  assert.match(workspace, /window\.addEventListener\("focus", refresh\)/);
  assert.match(workspace, /window\.addEventListener\("pageshow", refreshOnPageShow\)/);
  assert.match(workspace, /document\.addEventListener\("visibilitychange", refreshWhenVisible\)/);
  assert.match(workspace, /seoActions/);
  assert.match(workspace, /planItemCount/);
  assert.match(workspace, /opportunitiesInPlanCount/);
  // Stage statuses are derived live from page state, not hardcoded.
  assert.match(workspace, /Generating \(auto\)/);
  assert.match(workspace, /Drafting \(auto\)/);
  assert.match(workspace, /Needs approval/);
  assert.doesNotMatch(workspace, /Select a planned topic to create the next draft/);
});

test("home renders the loop as a single connected pipeline stepper", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  // One ordered pipeline with all seven stages, each linking to its page.
  for (const copy of [
    "Pipeline",
    "stages",
    "stageDotClass",
    "Context",
    "Opportunities",
    "Plan",
    "Drafts",
    "Review",
    "Publish",
    "Measure",
    "statusLabel",
  ]) {
    assert.match(workspace, new RegExp(copy));
  }
  assert.match(workspace, /href: `\/projects\/\$\{projectId\}\/context`/);
  assert.match(workspace, /href: `\/projects\/\$\{projectId\}\/visibility`/);

  // The decorative circular loop, arrow connectors, and 3x3 grid are gone.
  assert.doesNotMatch(workspace, /loopConnectorLabels/);
  assert.doesNotMatch(workspace, /loopGridClass/);
  assert.doesNotMatch(workspace, /ConnectorIcon/);
  assert.doesNotMatch(workspace, /data-loop-position/);
  assert.doesNotMatch(workspace, /grid-cols-\[2rem_1fr_2rem\]/);
});

test("settings exposes activity log as the secondary home for automation audit details", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");

  assert.match(settings, /Activity Log/);
  assert.match(settings, /\/settings\/activity/);
});

test("context page is a user-reviewable product cognition center, not a raw knowledge JSON page", () => {
  const context = read("projects/[id]/knowledge/knowledge-client.tsx");

  for (const copy of [
    "Context",
    "First-run Context confirmation",
    "Evidence library",
    "Domain profile",
    "Voice & rules",
    "Source pages",
    "Advanced JSON",
    "Positioning",
    "ICP",
    "Competitors",
    "Banned claims",
  ]) {
    assert.match(context, new RegExp(copy));
  }

  for (const legacyCopy of ["Knowledge", "Insight output", "Run Insight", "Profile JSON"]) {
    assert.doesNotMatch(context, new RegExp(legacyCopy));
  }
});

test("visibility page presents opportunity review before measurement diagnostics", () => {
  const visibility = read("projects/[id]/seo/seo-client.tsx");

  for (const copy of [
    "Review opportunities",
    "Find opportunities",
    "What to review",
    "Review result",
    "Measurement and diagnostics",
    "Visibility brief",
    "Add to Content Plan",
    "Dismiss",
    "Public crawl only",
  ]) {
    assert.match(visibility, new RegExp(copy));
  }

  assert.doesNotMatch(visibility, /title="SEO"/);
  assert.doesNotMatch(visibility, /title="Visibility overview"/);
  assert.doesNotMatch(visibility, /title="Opportunities"/);
});

test("activity log defaults to user-facing attention events and hides run internals in details", () => {
  const activity = read("projects/[id]/runs/runs-client.tsx");

  for (const copy of ["Activity Log", "Needs attention", "Recent successful activity", "User impact", "Next action", "Advanced details"]) {
    assert.match(activity, new RegExp(copy));
  }

  for (const defaultHeader of [">Agent<", ">Cost<", ">Model<"]) {
    assert.doesNotMatch(activity, new RegExp(defaultHeader.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.doesNotMatch(activity, /title="Runs"/);
});

test("content plan shows visible feedback while strategist is running", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");

  assert.match(topics, /busy === "strategist"/);
  assert.match(topics, /animate-spin/);
  assert.match(topics, /Generating content plan/);
  assert.doesNotMatch(topics, /Running strategist/);
});

test("content plan treats topic generation as a per-topic background operation", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");

  assert.match(topics, /generatingIds/);
  assert.match(topics, /isGenerating/);
  assert.match(topics, /Starting draft generation/);
  assert.doesNotMatch(topics, /disabled=\{!!busy \|\| topic\.status === "archived"\} size="sm" variant="primary" onClick=\{\(\) => generate\(topic\)\}/);
});

test("content plan backlog excludes drafted topics", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");

  assert.match(topics, /isBacklogStatus/);
  assert.match(topics, /backlogTopics/);
  assert.match(topics, /isBacklogStatus\(topic\.status\)/);
});

test("content plan helps users choose from backlog topics and supports density views", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");

  for (const copy of [
    "Content Plan",
    "Plan health",
    "Ready to draft",
    "Scheduled intent",
    "Needs priority",
    "Recommended next",
    "Why this exists",
    "Pick signal",
  ]) {
    assert.match(topics, new RegExp(copy));
  }

  assert.match(topics, /PlanView/);
  assert.match(topics, /setView\("list"\)/);
  assert.match(topics, /setView\("grid"\)/);
  assert.match(topics, /setView\("compact"\)/);
  assert.match(topics, /lg:grid-cols-2/);
  assert.match(topics, /2xl:grid-cols-3/);
  assert.match(topics, /aria-pressed=\{view === "grid"\}/);
  assert.match(topics, /planHealthForTopics\(topics\)/);
  assert.match(topics, /\{planHealth\.backlog\}/);
});

test("content plan edit form keeps priority inside the card at narrow widths", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");

  assert.match(topics, /lg:grid-cols-\[minmax\(120px,160px\)_minmax\(0,1fr\)_minmax\(96px,120px\)\]/);
  assert.match(topics, /min-w-0/);
  assert.doesNotMatch(topics, /md:grid-cols-\[160px_1fr_120px\]/);
});

test("content plan topic cards separate top chips, body copy, and footer actions", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");

  for (const marker of [
    "data-content-plan-card-top",
    "data-content-plan-card-body",
    "data-content-plan-card-footer",
    "data-content-plan-card-schedule",
    "data-content-plan-card-actions",
  ]) {
    assert.match(topics, new RegExp(marker));
  }

  const topIndex = topics.indexOf("data-content-plan-card-top");
  const bodyIndex = topics.indexOf("data-content-plan-card-body");
  const footerIndex = topics.indexOf("data-content-plan-card-footer");
  const scheduleIndex = topics.indexOf("data-content-plan-card-schedule");
  const actionsIndex = topics.indexOf("data-content-plan-card-actions");

  assert.ok(topIndex < bodyIndex, "status chips should render before the title and content");
  assert.ok(bodyIndex < footerIndex, "title and content should render before footer controls");
  assert.ok(footerIndex < scheduleIndex, "schedule controls should live inside the card footer");
  assert.ok(footerIndex < actionsIndex, "edit, generate, and archive controls should live inside the card footer");
  assert.ok(actionsIndex > scheduleIndex, "actions should sit after the schedule control in the footer row");
});

test("blocking mutations expose button-level progress and keep opportunity review local", () => {
  const ui = read("components/ui.tsx");
  const visibility = read("projects/[id]/seo/seo-client.tsx");
  const topics = read("projects/[id]/topics/topics-client.tsx");
  const review = read("projects/[id]/review/review-client.tsx");
  const publishing = read("projects/[id]/publishing/publishing-client.tsx");
  const settings = read("projects/[id]/settings/settings-client.tsx");
  const context = read("projects/[id]/knowledge/knowledge-client.tsx");
  const admin = read("projects/[id]/admin/admin-client.tsx");
  const workspace = read("projects/[id]/workspace.tsx");

  assert.match(ui, /export function ButtonProgress/);
  assert.match(ui, /Loader2/);
  assert.match(ui, /aria-live=\{busy \? "polite" : undefined\}/);

  assert.match(visibility, /opportunityBusy/);
  assert.match(visibility, /createActionBusy/);
  assert.match(visibility, /dismissBusy/);
  assert.match(visibility, /Adding to plan/);
  assert.match(visibility, /Dismissing/);

  const createActionBlock = visibility.slice(visibility.indexOf("async function createAction"), visibility.indexOf("async function dismiss"));
  assert.match(createActionBlock, /api\.createSEOContentAction/);
  assert.match(createActionBlock, /setOpportunities\(\(current\) => current\.filter/);
  assert.match(createActionBlock, /setActions\(\(current\) => \[action, \.\.\.current\.filter/);
  assert.doesNotMatch(createActionBlock, /await refresh\(\)/);

  const dismissBlock = visibility.slice(visibility.indexOf("async function dismiss"), visibility.indexOf("async function savePolicy"));
  assert.match(dismissBlock, /setOpportunities\(\(current\) => current\.filter/);
  assert.doesNotMatch(dismissBlock, /await refresh\(\)/);

  for (const [source, markers] of [
    [topics, ["Saving topic", "Scheduling", "Archiving"]],
    [review, ["Approving", "Rejecting", "Saving content"]],
    [publishing, ["Reconciling", "Retrying", "Marking distributed"]],
    [settings, ["Saving publisher", "Saving token", "Testing", "Retrying", "Saving settings"]],
    [context, ["Refreshing context", "Confirming context", "Saving source page", "Saving advanced context"]],
    [admin, ["Saving credentials"]],
  ]) {
    assert.match(source, /ButtonProgress/);
    for (const marker of markers) {
      assert.match(source, new RegExp(marker));
    }
  }
});
