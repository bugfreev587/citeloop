import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");
const exists = (relativePath) => fs.existsSync(path.join(appRoot, relativePath));

test("project shell uses user-facing Phase 1 navigation and hides Runs from primary nav", () => {
  const shell = read("components/project-shell.tsx");

  for (const label of ["Context", "Content Plan", "Review", "Publish", "Visibility", "Settings"]) {
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

test("project shell keeps the fixed-width sidebar primary action to one line", () => {
  const shell = read("components/project-shell.tsx");

  assert.match(shell, /overflow-hidden/);
  assert.match(shell, /truncate whitespace-nowrap/);
  assert.match(shell, /className="shrink-0"/);
});

test("project shell uses the review-width canvas for every project page", () => {
  const shell = read("components/project-shell.tsx");

  assert.match(shell, /max-w-\[1560px\]/);
  assert.match(shell, /max-w-\[1320px\]/);
  assert.doesNotMatch(shell, /reviewRoute \?/);
  assert.doesNotMatch(shell, /max-w-5xl/);
  assert.doesNotMatch(shell, /max-w-\[960px\]/);
});

test("settings nav entry is hidden when the user cannot access settings, avoiding a 404 dead-door", () => {
  const shell = read("components/project-shell.tsx");
  // Shell must accept and apply a canAccessSettings gate so non-admin users do not see a Settings entry that 404s.
  assert.match(shell, /canAccessSettings/);
  assert.match(shell, /item\.href !== "settings" \|\| canAccessSettings/);
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
  // Home polls for the onboarding profile so a freshly created project flips to a ready state on its own.
  assert.match(workspace, /if \(profile\) return;/);
  assert.match(workspace, /Your domain context is ready/);
});

test("visibility degrades search metrics and signals truncated loop, context paginates evidence", () => {
  const visibility = read("projects/[id]/seo/seo-client.tsx");
  // Placeholder search metrics are visually dimmed + labelled when GSC is not connected.
  assert.match(visibility, /The numbers below are placeholders/);
  assert.match(visibility, /opacity-60/);
  // Loop closure tells the user when it is truncated instead of silently capping at 5.
  assert.match(visibility, /Showing \{loopRows\.length\} of \{loopTotal\}/);

  const context = read("projects/[id]/knowledge/knowledge-client.tsx");
  // Evidence library no longer silently hides items behind a fixed slice of 8.
  assert.match(context, /Show all \$\{evidenceRows\.length\}/);
  assert.match(context, /showAllEvidence/);
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

test("review surfaces honest repair state and a deep link to fix evidence in context", () => {
  const review = read("projects/[id]/review/review-client.tsx");
  const articleDetail = read("projects/[id]/articles/[articleId]/article-detail-client.tsx");
  assert.match(review, /Fix evidence in Context/);
  assert.match(review, /Automatic repair is exhausted/);
  assert.match(review, /repairExhausted/);
  assert.match(review, /Cannot approve:/);
  assert.doesNotMatch(review, /qa blocking/);
  assert.match(articleDetail, /Cannot approve:/);
  assert.doesNotMatch(articleDetail, /qa blocking/);
});

test("destructive content-plan and distribution actions confirm before running", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");
  // Archive and schedule-clear are reversible-but-surprising; both must confirm.
  assert.match(topics, /Remove .* from the content plan\?/);
  assert.match(topics, /Clear the scheduled date/);

  const workspace = read("projects/[id]/workspace.tsx");
  assert.match(workspace, /Mark this variant as distributed\?/);
  assert.match(workspace, /Copied to clipboard/);
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

test("home leads with growth outcomes and does not show run internals by default", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const copy of [
    "Growth Overview",
    "Growth impact",
    "AI citations",
    "Organic traffic",
    "Published pages",
    "Next growth move",
    "Growth loop",
    "Measurement coverage",
    "Refresh context",
    "Generate content plan",
  ]) {
    assert.match(workspace, new RegExp(copy));
  }

  for (const internalCopy of ["Run Insight", "Run Strategist", "Publish tick", "Recent runs", "tokens"]) {
    assert.doesNotMatch(workspace, new RegExp(internalCopy));
  }
});

test("home growth metrics use unwrapped SuperX-style cards", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const copy of [
    "growthMetricCards",
    "growthTrendPath",
    "MetricIcon",
    "Growth metric trend",
    "AI citations",
    "Organic traffic",
    "Published pages",
    "Opportunities in motion",
  ]) {
    assert.match(workspace, new RegExp(copy));
  }

  assert.match(workspace, /xl:grid-cols-\[minmax\(0,1\.6fr\)_minmax\(360px,1fr\)\]/);
  assert.match(workspace, /rounded-\[18px\] border border-slate-200 bg-white/);
  assert.doesNotMatch(workspace, /<section className="rounded-2xl border border-slate-200 bg-white p-5/);
});

test("home explains growth limits and loop status from existing product data", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const copy of [
    "Growth measurement is limited",
    "Search Console is not connected yet",
    "Review drafts to unlock growth",
    "Find opportunities",
    "Plan content",
    "Create drafts",
    "Review",
    "Publish",
    "Measure results",
    "Recent growth signals",
    "CiteLoop knowledge",
    "More waiting",
    "Cannot approve:",
  ]) {
    assert.match(workspace, new RegExp(copy));
  }

  for (const apiCall of [
    "api.getProfile(projectId)",
    "api.listInventory(projectId)",
    "api.getSEOOverview(projectId)",
    "api.listSEOOpportunities(projectId",
  ]) {
    assert.match(workspace, new RegExp(apiCall.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.doesNotMatch(workspace, /Results \/ Momentum/);
  assert.doesNotMatch(workspace, /Loop progress/);
  assert.doesNotMatch(workspace, /Actionable momentum/);
  assert.doesNotMatch(workspace, /Active loop items/);
  assert.doesNotMatch(workspace, /No failed or degraded activity needs attention right now/);
  assert.doesNotMatch(workspace, /label: "Needs evidence"/);
  assert.doesNotMatch(workspace, /Automation healthy/);
});

test("home growth loop renders linked status cards with arrows between cards", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const copy of [
    "Context",
    "Context feeds Find opportunities",
    "Find opportunities connects to Plan content",
    "Plan content connects to Create drafts",
    "Create drafts connects to Review",
    "Review connects to Publish",
    "Publish connects to Measure results",
    "Measure results connects back to Find opportunities",
    "waiting for analytics signal",
    "source pages",
    "evidence snippets",
    "topics in the content backlog",
    "drafts created or approved",
  ]) {
    assert.match(workspace, new RegExp(copy));
  }

  assert.match(workspace, /statusLines/);
  assert.match(workspace, /grid-cols-\[2rem_1fr_2rem\]/);
  assert.match(workspace, /className="text-center text-base font-bold leading-5 text-slate-950"/);
  assert.match(workspace, /href=\{card\.href\}/);
  assert.match(workspace, /data-loop-position=\{card\.position\}/);
  assert.doesNotMatch(workspace, /Open \{card\.label\}/);
  assert.doesNotMatch(workspace, /<ArrowRight size=\{15\} \/>/);
  for (const position of ["0", "1", "2", "3", "4", "5", "6"]) {
    assert.match(workspace, new RegExp(`position: ${position}`));
  }

  assert.match(workspace, /lg:gap-x-14 lg:gap-y-14/);
  assert.match(workspace, /h-10 w-10/);
  assert.match(workspace, /ConnectorIcon size=\{20\}/);
  assert.match(workspace, /text-xl/);
  assert.match(workspace, /grid-cols-\[1fr_1fr_1fr\]/);
  assert.match(workspace, /loopConnectorLabels/);
  assert.doesNotMatch(workspace, />Overview</);
  assert.doesNotMatch(workspace, /min-h-\[172px\]/);
  assert.doesNotMatch(workspace, /metricValue/);
  assert.doesNotMatch(workspace, /-right-6/);
  assert.doesNotMatch(workspace, /-bottom-6/);
  assert.doesNotMatch(workspace, /xl:grid-cols-6/);
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

test("visibility page presents SEO and AI-answer outcomes before advanced diagnostics", () => {
  const visibility = read("projects/[id]/seo/seo-client.tsx");

  for (const copy of [
    "Visibility",
    "SEO and AI-answer visibility for your domain",
    "Visibility overview",
    "Search visibility",
    "AI visibility",
    "Loop closure",
    "Opportunity detected",
    "Added to Content Plan",
    "Measuring impact",
    "Add to Content Plan",
    "Advanced diagnostics",
    "Public crawl only",
  ]) {
    assert.match(visibility, new RegExp(copy));
  }

  assert.doesNotMatch(visibility, /title="SEO"/);
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

  assert.match(topics, /function isBacklogStatus/);
  assert.match(topics, /backlogTopics/);
  assert.match(topics, /isBacklogStatus\(topic\.status\)/);
});
