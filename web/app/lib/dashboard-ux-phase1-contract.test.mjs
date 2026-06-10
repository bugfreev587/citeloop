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

test("review surfaces honest repair state and a deep link to fix evidence in context", () => {
  const review = read("projects/[id]/review/review-client.tsx");
  assert.match(review, /Fix evidence in Context/);
  assert.match(review, /Automatic repair is exhausted/);
  assert.match(review, /repairExhausted/);
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

test("home exposes a user-facing next action and does not show run internals by default", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const copy of ["Next action", "Why this", "Also waiting", "Refresh context", "Generate content plan"]) {
    assert.match(workspace, new RegExp(copy));
  }

  for (const internalCopy of ["Run Insight", "Run Strategist", "Publish tick", "Recent runs", "tokens"]) {
    assert.doesNotMatch(workspace, new RegExp(internalCopy));
  }
});

test("home phase 2 exposes momentum, loop progress, and context health from existing product data", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const copy of [
    "Results / Momentum",
    "Loop progress",
    "Context health",
    "Activity warning summary",
    "Published this month",
    "Opportunities converted",
    "Evidence coverage",
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

  assert.doesNotMatch(workspace, /Automation healthy/);
  assert.doesNotMatch(workspace, /No failed or degraded activity needs attention right now/);
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
  assert.match(topics, /Running strategist/);
});

test("content plan treats topic generation as a per-topic background operation", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");

  assert.match(topics, /generatingIds/);
  assert.match(topics, /isGenerating/);
  assert.match(topics, /Starting draft generation/);
  assert.doesNotMatch(topics, /disabled=\{!!busy \|\| topic\.status === "archived"\} size="sm" variant="primary" onClick=\{\(\) => generate\(topic\)\}/);
});
