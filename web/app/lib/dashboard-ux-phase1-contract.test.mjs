import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");
const exists = (relativePath) => fs.existsSync(path.join(appRoot, relativePath));

test("project shell uses user-facing Phase 1 navigation and hides Runs from primary nav", () => {
  const shell = read("components/project-shell.tsx");

  for (const label of ["Home", "Opportunities", "Content Plan", "Review", "Publish", "Results"]) {
    assert.match(shell, new RegExp(`label: "${label}"`));
  }
  assert.match(shell, /href=\{`\/projects\/\$\{projectId\}\/context`\}/);
  assert.match(shell, />\s*Context\s*</);

  for (const legacy of [
    'label: "Knowledge"',
    'label: "Topics"',
    'label: "Publishing"',
    'label: "SEO"',
    'label: "Runs"',
    'label: "Visibility"',
    'label: "SYSTEM"',
    'label: "Intelligence"',
    'label: "Execution"',
    'label: "Outcomes"',
  ]) {
    assert.doesNotMatch(shell, new RegExp(legacy.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.match(shell, /Docs[\s\S]*Context[\s\S]*Settings/);
  assert.match(shell, /isPlatformAdmin[\s\S]*Admin/);
  assert.doesNotMatch(shell, /\/admin\?from=/);
  assert.match(shell, /\/projects\/\$\{projectId\}\/admin/);
});

test("project shell does not render a sidebar primary action slot", () => {
  const shell = read("components/project-shell.tsx");

  assert.doesNotMatch(shell, /primaryAction/);
  assert.doesNotMatch(shell, /showPrimaryAction/);
  assert.doesNotMatch(shell, /bg-gradient-to-r/);
});

test("project shell never renders global sidebar CTA logic", () => {
  const shell = read("components/project-shell.tsx");
  const dashboardLogic = read("lib/dashboard-ux-logic.ts");

  assert.doesNotMatch(shell, /sidebarPrimaryAction/);
  assert.doesNotMatch(dashboardLogic, /sidebarPrimaryAction/);
  assert.doesNotMatch(shell, /title: "Open Home"/);
  assert.doesNotMatch(dashboardLogic, /title: "Open Home"/);
});

test("project shell groups desktop navigation into SuperX-style sections", () => {
  const shell = read("components/project-shell.tsx");

  assert.match(shell, /const navSections = \[/);
  assert.match(shell, /id: "primary"[\s\S]*label: null[\s\S]*label: "Home"/);
  assert.doesNotMatch(shell, /id: "primary"[\s\S]*label: "Context"[\s\S]*id: "analysis"/);
  assert.match(shell, /id: "analysis"[\s\S]*label: "Analysis"[\s\S]*label: "Opportunities"/);
  assert.match(shell, /id: "content"[\s\S]*label: "Content"[\s\S]*label: "Content Plan"[\s\S]*label: "Review"[\s\S]*label: "Publish"/);
  assert.match(shell, /id: "results"[\s\S]*label: "Results"[\s\S]*label: "Results"/);
  for (const label of ["Analysis", "Content", "Results"]) {
    assert.match(shell, new RegExp(`label: "${label}"`));
  }
  for (const legacyGroup of ["Intelligence", "Execution", "Outcomes", "ANALYZE", "CREATE", "DELIVER", "MEASURE"]) {
    assert.doesNotMatch(shell, new RegExp(`label: "${legacyGroup}"`));
  }
  assert.doesNotMatch(shell, /id: "system"/);
  assert.match(shell, /navSections\s*\n\s*\.map/);
  assert.match(shell, /visibleNavSections\.map/);
  assert.match(shell, /section\.items\.map/);
  assert.match(shell, /tracking-\[0\.18em\]/);
});

test("project shell keeps primary navigation labels stable without a CTA above Home", () => {
  const shell = read("components/project-shell.tsx");

  assert.match(shell, /label: "Home"/);
  assert.match(shell, /Docs[\s\S]*Context[\s\S]*Settings/);
  assert.match(shell, /label: "Content Plan"/);
  assert.doesNotMatch(shell, /truncate whitespace-nowrap/);
});

test("project shell sidebar stays usable in short landscape mobile viewports", () => {
  const shell = read("components/project-shell.tsx");
  const asideClass = shell.match(/<aside className="([^"]+)"/)?.[1] ?? "";

  assert.match(asideClass, /h-\[100dvh\]/);
  assert.match(asideClass, /overflow-y-auto/);
  assert.match(asideClass, /overscroll-contain/);
  assert.match(asideClass, /pb-\[calc\(1rem\+env\(safe-area-inset-bottom\)\)\]/);
});

test("project shell does not fetch route or opportunity state for a sidebar CTA", () => {
  const shell = read("components/project-shell.tsx");

  assert.doesNotMatch(shell, /api\.listSEOOpportunities\(projectId, \{ status: "open", limit: 10 \}\)/);
  assert.doesNotMatch(shell, /openOpportunityCount/);
  assert.doesNotMatch(shell, /currentPathname: pathname/);
  assert.doesNotMatch(shell, /actionSummary/);
});

test("project shell uses the review-width canvas for every project page", () => {
  const shell = read("components/project-shell.tsx");

  assert.match(shell, /max-w-\[1560px\]/);
  assert.match(shell, /max-w-\[1320px\]/);
  assert.doesNotMatch(shell, /reviewRoute \?/);
  assert.doesNotMatch(shell, /max-w-5xl/);
  assert.doesNotMatch(shell, /max-w-\[960px\]/);
});

test("settings is visible to project users while admin remains separate", () => {
  const shell = read("components/project-shell.tsx");
  assert.doesNotMatch(shell, /canAccessSettings/);
  assert.match(shell, /href=\{`\/projects\/\$\{projectId\}\/settings`\}/);
  assert.match(shell, /isPlatformAdmin && \(/);
  assert.match(shell, /visibleNav\.map/);

  const layout = read("projects/[id]/layout.tsx");
  assert.doesNotMatch(layout, /canUseInternalTools/);
  assert.doesNotMatch(layout, /canAccessSettings=/);

  const settingsPage = read("projects/[id]/settings/page.tsx");
  assert.doesNotMatch(settingsPage, /canUseInternalTools/);
  assert.doesNotMatch(settingsPage, /notFound\(\)/);
});

test("missing project routes show an onboarding warning instead of rendering child pages", () => {
  const layout = read("projects/[id]/layout.tsx");
  const shell = read("components/project-shell.tsx");
  const accountMenu = read("components/project-account-menu.tsx");

  assert.match(layout, /isProjectMissingError\(error\)/);
  assert.match(layout, /shouldRenderProjectChildren \? children : null/);
  assert.doesNotMatch(layout, /project \? children : null/);
  assert.match(shell, /No project found/);
  assert.match(shell, /Connect your domain to create your first project\./);
  assert.match(shell, /Connect project/);
  assert.match(shell, /href="\/projects"/);
  assert.match(shell, /project && <ProjectVisitRecorder projectId=\{projectId\} \/>/);
  assert.match(accountMenu, /project \? uniqueProjects\(projects, currentProject\) : projects/);
  assert.match(accountMenu, /No project found/);
  assert.doesNotMatch(accountMenu, /project \?\? \{ id: projectId/);
});

test("project layout keeps child pages available when the server project prefetch times out", () => {
  const layout = read("projects/[id]/layout.tsx");

  assert.match(layout, /function isRecoverableProjectLoadError/);
  assert.match(layout, /CiteLoop API request timed out/);
  assert.match(layout, /recoverableProjectLoadError = isRecoverableProjectLoadError\(error\)/);
  assert.match(layout, /const shouldRenderProjectChildren = Boolean\(project\) \|\| recoverableProjectLoadError/);
  assert.match(layout, /shouldRenderProjectChildren \? children : null/);
});

test("home does not show context-build progress or raw API payloads before a project exists", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  assert.match(workspace, /const showContextBuild = projectLoaded && contextBuild\.active/);
  assert.match(workspace, /showContextBuild && \(/);
  assert.doesNotMatch(workspace, /contextBuild\.active && \(/);
  assert.doesNotMatch(workspace, /title="API server unavailable"/);
  assert.doesNotMatch(workspace, /Dashboard data could not be loaded/);
  assert.doesNotMatch(workspace, /detail=\{`Dashboard data could not be loaded \(\$\{apiError\}\)\.`\}/);
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

test("analysis owns decisions while results owns impact reports and learning", () => {
  assert.equal(exists("projects/[id]/analysis/page.tsx"), true, "analysis route should exist");
  assert.equal(exists("projects/[id]/results/page.tsx"), true, "results route should exist");

  const analysisPage = read("projects/[id]/analysis/page.tsx");
  const resultsPage = read("projects/[id]/results/page.tsx");
  const seo = read("projects/[id]/seo/seo-client.tsx");

  assert.match(analysisPage, /AnalysisClient/);
  assert.match(resultsPage, /ResultsClient/);
  assert.match(seo, /export function AnalysisClient/);
  assert.match(seo, /export function ResultsClient/);
  assert.match(seo, /mode="analysis"/);
  assert.match(seo, /mode="results"/);

  for (const copy of [
    "Opportunities",
    "Analysis",
    "Opportunity Queue",
    "Site Fixes",
    "Loop in motion",
    "View results",
    "Add to Content Plan",
    "Create Page Update",
    "Create Site Fix",
  ]) {
    assert.match(seo, new RegExp(copy));
  }
  for (const copy of ["Impact Reports", "Results and learning", "GEO visibility"]) {
    assert.match(seo, new RegExp(copy));
  }
  assert.doesNotMatch(analysisPage, /ResultsClient/);
  assert.doesNotMatch(resultsPage, /AnalysisClient/);
  assert.doesNotMatch(seo, /The numbers below are placeholders/);
  assert.doesNotMatch(seo, /Showing \{loopRows\.length\} of \{loopTotal\}/);

  const context = read("projects/[id]/knowledge/knowledge-client.tsx");
  // Evidence stays compact on the page; source pages move behind a secondary coverage drawer.
  assert.match(context, /Show all \$\{evidenceRows\.length\}/);
  assert.match(context, /evidencePreviewRows/);
  assert.match(context, /SourceCoveragePanel/);
  assert.match(context, /View source pages/);
  assert.match(context, /setActiveDrawer\("sources"\)/);
  assert.match(context, /activeDrawer/);
  assert.match(context, /DrawerPanel/);
  assert.doesNotMatch(context, /sourcePreviewRows/);
  assert.doesNotMatch(context, /showAllEvidence/);
  assert.doesNotMatch(context, /Show fewer/);
});

test("analysis surface uses a compact GSC status control and keeps decisions out of large cards", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");

  for (const copy of [
    "Opportunity Queue",
    "Site Fixes",
    "Loop in motion",
    "View results",
    "GSC Connected",
    "GSC Not connected",
    "Search Console details",
    "Manage in Settings",
    "Connect Search Console",
    "Add to Content Plan",
    "Create Page Update",
    "Create Site Fix",
    "Recently sent",
    "Open details",
    "Opportunity details",
    "Evidence",
    "Confidence",
    "High priority",
    "Medium priority",
    "Low priority",
    "No opportunities to review",
  ]) {
    assert.match(seo, new RegExp(copy));
  }

  assert.match(seo, /function GSCStatusMenu/);
  assert.match(seo, /const \[gscMenuOpen, setGSCMenuOpen\] = useState\(false\)/);
  assert.match(seo, /const gscMenuRef = useRef<HTMLDivElement \| null>\(null\)/);
  assert.match(seo, /document\.addEventListener\("pointerdown", onPointerDown\)/);
  assert.match(seo, /document\.removeEventListener\("pointerdown", onPointerDown\)/);
  assert.match(seo, /gscMenuRef\.current\?\.contains\(target\)/);
  assert.match(seo, /setGSCMenuOpen\(false\)/);
  assert.match(seo, /function opportunityPrimaryCTA/);
  assert.match(seo, /function opportunityPriorityLabel/);
  assert.match(seo, /function toneForOpportunityPriority/);
  assert.match(seo, /\/projects\/\$\{projectId\}\/settings#search-console/);
  assert.match(seo, /const \[selectedOpportunityID, setSelectedOpportunityID\] = useState<string \| null>\(null\)/);
  assert.match(seo, /const selectedOpportunity = useMemo/);
  assert.match(seo, /setSelectedOpportunityID\(opp\.id\)/);
  assert.match(seo, /aria-label=\{`Open opportunity details: \$\{opportunityTitle\(opp\)\}`\}/);
  assert.match(seo, /role="dialog"/);
  assert.match(seo, /aria-modal="true"/);
  assert.match(seo, /Opportunity details/);
  assert.match(seo, /Drawer actions/);
  assert.match(seo, /data-analysis-growth-findings-section/);
  assert.match(seo, /data-analysis-finding-card/);
  assert.match(seo, /data-analysis-drawer/);
  const opportunityCardBlock = seo.slice(seo.indexOf("data-analysis-finding-card"), seo.indexOf("data-analysis-loop-strip"));
  assert.doesNotMatch(opportunityCardBlock, /risk_level/);
  assert.doesNotMatch(opportunityCardBlock, /sourceModeForOpportunity/);
  assert.doesNotMatch(opportunityCardBlock, /Score \{metric\(opp\.priority_score\)\}/);
  assert.doesNotMatch(opportunityCardBlock, />Signal</);
  assert.match(seo, /motion-safe:animate-\[citeloop-drawer-panel-in_220ms_cubic-bezier\(0\.16,1,0\.3,1\)\]/);
  assert.match(seo, /motion-safe:animate-\[citeloop-drawer-scrim-in_180ms_ease-out\]/);
  assert.match(seo, /const analysisSurfaceRef = useRef<HTMLDivElement \| null>\(null\)/);
  assert.match(seo, /const analysisDrawerRef = useRef<HTMLElement \| null>\(null\)/);
  assert.match(seo, /const analysisReturnFocusRef = useRef<HTMLElement \| null>\(null\)/);
  assert.match(seo, /<div ref=\{mode === "analysis" \? analysisSurfaceRef : mode === "results" \? resultsSurfaceRef : undefined\} className="space-y-7">/);
  assert.match(seo, /analysisSurfaceRef\.current\.setAttribute\("aria-hidden", "true"\)/);
  assert.match(seo, /analysisReturnFocusRef\.current\?\.focus\(\)/);
  assert.match(seo, /\}, \[selectedOpportunity\?\.id\]\)/);
  assert.match(seo, /event\.key === "Tab"/);
  assert.match(seo, /max-w-2xl/);
  assert.match(seo, /document\.body\.style\.overflow = "hidden"/);
  assert.match(seo, /document\.body\.style\.overflow = previousBodyOverflow/);
  assert.match(seo, /className="absolute right-0 top-0 flex h-\[100dvh\] max-h-\[100dvh\] w-full max-w-2xl/);
  assert.match(seo, /className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-5"/);
  assert.match(seo, /aria-label="Drawer actions"[\s\S]*className="shrink-0 flex flex-col gap-2 border-t/);
  assert.match(seo, /pb-\[calc\(1\.5rem\+env\(safe-area-inset-bottom\)\)\]/);
  assert.doesNotMatch(seo, /<div className="min-w-0 rounded-xl border border-slate-200 bg-white">\s*<div className="flex flex-col gap-3 border-b border-slate-100 p-4/);
  assert.doesNotMatch(seo, /<details className="relative">/);
  assert.doesNotMatch(seo, /<details[\s\S]*View evidence/);
  assert.match(seo, /api\.listSEOOpportunities\(projectId, \{ status: "open", limit: 50 \}\)/);
  assert.doesNotMatch(seo, /Decision queue/);
  assert.doesNotMatch(seo, /What needs approval now/);
  assert.doesNotMatch(seo, /Search data status/);
  assert.doesNotMatch(seo, /Search performance snapshot/);
  assert.doesNotMatch(seo, /data-analysis-search-signal/);
  assert.doesNotMatch(seo, /Recommendation \{index \+ 1\}/);
  assert.doesNotMatch(seo, /\{mode === "analysis" && actions\.length > 0 && \(/);
  assert.doesNotMatch(seo, /<SectionHeader title="Content actions"/);
  assert.doesNotMatch(seo, /Decide which recommendations deserve content work/);
  assert.doesNotMatch(seo, /Raw GSC rows/);
  assert.doesNotMatch(seo, /Full signal table/);
});

test("analysis handoff cards mirror Content Plan actions from visibility summary", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");

  assert.match(seo, /const activeHandoffStages = new Set/);
  assert.match(seo, /activeHandoffStages\.has\(deriveVisibilityLifecycleStage\(action\)\)/);
  assert.match(seo, /const sentOpportunityLinks = loopActions\s*\.filter\(isRecentlySentAction\)/);
  assert.doesNotMatch(seo, /const sentOpportunityLinks = actions\s*\.filter\(isRecentlySentAction\)/);
  assert.match(seo, /actionHandoffHref\(projectId, action\)/);
  assert.match(seo, /\/projects\/\$\{projectId\}\/plan\?action=\$\{action\.id\}/);
});

test("results surface defaults to published outcomes with card-triggered attribution details", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  const resultsStart = seo.indexOf('{mode === "results"');
  const nextAnalysisStart = seo.indexOf('{mode === "analysis" && (', resultsStart + 1);
  const resultsBlock = seo.slice(resultsStart, nextAnalysisStart);
  const resultDrawerStart = seo.indexOf('{mode === "results" && selectedResultAction', resultsStart);
  const resultDrawerEnd = seo.indexOf('{mode === "analysis" && selectedOpportunity', resultDrawerStart + 1);
  const resultDrawerBlock = seo.slice(resultDrawerStart, resultDrawerEnd);

  for (const copy of [
    "Outcome summary",
    "Published work",
    "Measurement queue",
    "Waiting",
    "Positive",
    "Negative",
    "Inconclusive",
    "Measurement window",
    "AI citation signals",
    "Published / applied",
    "No published or applied actions are ready for attribution yet",
    "Advanced diagnostics",
  ]) {
    assert.match(resultsBlock, new RegExp(copy));
  }

  assert.match(seo, /function actionMeasurementState/);
  assert.match(seo, /const measuredActions = loopActions\.filter/);
  assert.match(seo, /const resultActions = loopActions\.filter/);
  assert.match(resultsBlock, /resultActions\.slice\(0, 12\)\.map/);
  assert.match(resultsBlock, /data-results-action-card/);
  assert.match(resultDrawerBlock, /data-results-drawer/);
  assert.match(resultDrawerBlock, /Manual verify/);
  assert.match(resultDrawerBlock, /Verification failed/);
  assert.match(resultDrawerBlock, /Measurement details/);
  assert.match(resultDrawerBlock, /verifyAction\(action, "verified"\)/);
  assert.match(resultDrawerBlock, /verifyAction\(action, "failed"\)/);
  assert.match(resultsBlock, /<details[\s\S]*Advanced diagnostics/);
  assert.doesNotMatch(resultsBlock, /Add to Content Plan/);
  assert.doesNotMatch(resultsBlock, /Dismiss/);
  assert.doesNotMatch(resultsBlock, /Opportunity queue/);
});

test("Phase 5 pages separate growth operating outputs", () => {
  const workspace = read("projects/[id]/workspace.tsx");
  const seo = read("projects/[id]/seo/seo-client.tsx");
  const topics = read("projects/[id]/topics/topics-client.tsx");
  const workflow = read("projects/[id]/content-workflow-client.tsx");
  const activity = read("projects/[id]/runs/runs-client.tsx");
  const resultsStart = seo.indexOf('{mode === "results"');
  const nextAnalysisStart = seo.indexOf('{mode === "analysis" && (', resultsStart + 1);
  const resultsBlock = seo.slice(resultsStart, nextAnalysisStart);

  for (const copy of ["Growth Control Center", "Opportunities", "Action Portfolio", "Impact Reports", "Operations health", "Learning signal"]) {
    assert.match(`${workspace}\n${seo}\n${topics}\n${activity}`, new RegExp(copy));
  }
  assert.match(workspace, /highestPriorityOpportunity/);
  assert.match(workspace, /measurementResultNeedsAttention/);
  assert.match(resultsBlock, /title="Impact Reports"/);
  assert.match(resultsBlock, /title="Learning signal"/);
  assert.match(resultsBlock, /Conservative learning/);
  assert.match(activity, /Operations health/);
  assert.match(activity, /Operational blockers/);
  assert.match(activity, /Diagnostics/);
  assert.match(workflow, /planned topics/i);
  assert.match(workflow, /action handoff/i);
  assert.doesNotMatch(`${workspace}\n${seo}\n${topics}\n${activity}`, /content pipeline/i);
  assert.doesNotMatch(resultsBlock, /Measurement and diagnostics/);
});

test("home pipeline keeps workflow counts separate from search performance metrics", () => {
  const workspace = read("projects/[id]/workspace.tsx");
  const stagesBlock = workspace.slice(workspace.indexOf("const stages:"), workspace.indexOf("const nextScheduledRow"));

  assert.match(stagesBlock, /label: "Results"/);
  assert.match(stagesBlock, /metricValue: measuringActions/);
  assert.match(stagesBlock, /Measuring impact/);
  assert.match(workspace, /label: "Organic traffic"[\s\S]*value: searchDataConnected \? metric\(clicks28d\) : "Limited"/);
  assert.doesNotMatch(stagesBlock, /label: "Measurement"/);
  assert.doesNotMatch(workspace, /metricValue: searchDataConnected \? metric\(clicks28d\) : "-"/);
});

test("home turns Needs you into the main human action queue", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const copy of [
    "Manual gates and setup",
    "Blocking now",
    "Needs review",
    "Improves results",
    "Confirm Context",
    "Review opportunities",
    "Connect Search Console",
    "View all open actions",
  ]) {
    assert.match(workspace, new RegExp(copy));
  }

  assert.match(workspace, /humanActionItems/);
  assert.match(workspace, /visibleHumanActionItems/);
  assert.match(workspace, /primaryAction = humanActionItems\[0\]/);
  assert.doesNotMatch(workspace, /Automation warnings/);
  assert.doesNotMatch(workspace, /Variants waiting on canonical/);
});

test("home global action queue fetches readiness and uses explicit P0 P1 P2 priorities", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const expected of [
    'type HumanActionPriority = "P0" | "P1" | "P2"',
    "rank: number",
    "tone: StageTone",
    "api.getAutopilotReadiness(projectId).catch(() => null)",
    "buildReadinessHumanActions",
    "priorityOrder",
    "P0",
    "P1",
    "P2",
    "border-l-[#f87171]",
    "border-l-[#f59e0b]",
    "border-l-[#7dd3fc]",
  ]) {
    assert.equal(workspace.includes(expected), true, `workspace.tsx missing ${expected}`);
  }

  assert.doesNotMatch(workspace, /sort\(\(a, b\) => a\.priority - b\.priority\)/);
});

test("home presents Needs you as compact action tiles above the pipeline", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  const needsYouIndex = workspace.indexOf('title="Needs you"');
  const pipelineIndex = workspace.indexOf('title="Pipeline"');
  assert.ok(needsYouIndex > -1, "Needs you section should render");
  assert.ok(pipelineIndex > -1, "Pipeline section should render");
  assert.ok(needsYouIndex < pipelineIndex, "Needs you should appear before Pipeline on Home");

  for (const contract of [
    "humanPriorityStyles",
    "humanActionIcon",
    "Action spotlight",
    "sm:grid-cols-2 xl:grid-cols-4",
    "border-l-4",
    "compactActionTileClass",
    "Where this project is in the loop",
  ]) {
    assert.match(workspace, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("home keeps Pipeline in the first desktop fold with compact metrics and actions", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const contract of [
    "compactMetricCardClass",
    "compactActionTileClass",
    "lg:grid-cols-4",
    "lg:min-h-[148px]",
    "lg:min-h-[118px]",
    "text-3xl",
    "line-clamp-2",
    "First-fold pipeline",
  ]) {
    assert.match(workspace, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.doesNotMatch(workspace, /lg:row-span-2/);
  assert.doesNotMatch(workspace, /md:text-6xl/);
  assert.doesNotMatch(workspace, /sm:aspect-\[1\.05\/1\]/);
});

test("gsc oauth entry points are self-serve and action-first", () => {
  assert.equal(exists("projects/[id]/settings/gsc/callback/page.tsx"), true, "GSC callback route should exist");
  assert.equal(
    exists("integrations/google/search-console/callback/page.tsx"),
    true,
    "Google OAuth should use one static callback URI for Google Console registration",
  );
  assert.equal(
    exists("projects/[id]/settings/gsc/callback/gsc-callback-client.tsx"),
    true,
    "GSC callback client should exist",
  );
  assert.equal(
    exists("integrations/google/search-console/callback/gsc-callback-client.tsx"),
    true,
    "Static GSC callback client should exist",
  );

  for (const [file, copies] of [
    ["projects/[id]/workspace.tsx", ["Connect Search Console for traffic", "Search Console connected"]],
    ["projects/[id]/seo/seo-client.tsx", ["Connect Search Console", "Search Console property", "Select property", "Backfilling Search Console", "Search data is stale", "Property mismatch"]],
    [
      "projects/[id]/settings/settings-client.tsx",
      [
        "Search Console connection",
        "Connect Search Console",
        "Authorized properties",
        "Set up Search Console property",
        "Open Search Console",
        "Verify DNS ownership",
        "Connect after verification",
        "backfilling",
        "stale",
        "mismatch",
      ],
    ],
    ["integrations/google/search-console/callback/gsc-callback-client.tsx", ["Finishing Search Console connection", "Return to Settings", "project_id"]],
    ["projects/[id]/settings/gsc/callback/gsc-callback-client.tsx", ["Finishing Search Console connection", "Return to Settings"]],
  ]) {
    const source = read(file);
    for (const copy of copies) {
      assert.match(source, new RegExp(copy));
    }
  }
});

test("settings hides the connect CTA after Search Console authorization returns properties", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");

  assert.match(settings, /const gscHasAuthorizedProperties = Boolean/);
  assert.match(settings, /const canStartGSCOAuth =/);
  assert.match(settings, /!gscHasAuthorizedProperties/);
  assert.match(settings, /canStartGSCOAuth && \(/);
  assert.doesNotMatch(settings, /gscConnection\?\.status === "connected" \? "Reconnect Search Console" : "Connect Search Console"/);
});

test("publisher settings show CMS connector next steps without pretending connectors are live", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");

  for (const copy of ["WordPress", "CMS connector roadmap", "Draft-only until OAuth connector is ready", "GitHub/Next.js"]) {
    assert.match(settings, new RegExp(copy));
  }
});

test("publisher settings restore GitHub OAuth App connection as the primary path", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");

  assert.match(settings, /GithubIntegrationStatus/);
  assert.match(settings, /rememberGithubConnectProject/);
  assert.match(settings, /const \[githubIntegration, setGithubIntegration\]/);
  assert.match(settings, /api\.getGithubIntegration\(projectId\)/);
  assert.match(settings, /function connectGithub\(\)/);
  assert.match(settings, /rememberGithubConnectProject\(projectId,\s*`\/projects\/\$\{projectId\}\/settings\?github=connected#publisher`/);
  assert.match(settings, /githubIntegration\?\.install_url/);
  assert.match(settings, /window\.location\.href = githubIntegration\.install_url/);
  assert.match(settings, /function reuseGithub\(\)/);
  assert.match(settings, /reusable_installation_id/);
  assert.match(settings, /\/integrations\/github\/callback\?installation_id=/);
  assert.match(settings, /Connect GitHub/);
  assert.match(settings, /Connected via GitHub App/);
  assert.match(settings, /Advanced: connect with a personal access token/);
});

test("context profile editors collapse after saving", () => {
  const context = read("projects/[id]/knowledge/knowledge-client.tsx");

  assert.match(context, /profileEditorOpen/);
  assert.match(context, /voiceEditorOpen/);
  assert.match(context, /setProfileEditorOpen\(false\)/);
  assert.match(context, /setVoiceEditorOpen\(false\)/);
  assert.match(context, /Edit domain profile/);
  assert.match(context, /Edit voice & rules/);
});

test("context edit buttons stay single-line in section headers", () => {
  const context = read("projects/[id]/knowledge/knowledge-client.tsx");

  assert.match(context, /const contextEditButtonClass = "([^"]*whitespace-nowrap[^"]*)"/);
  assert.match(context, /const contextEditButtonClass = "([^"]*shrink-0[^"]*)"/);
  assert.match(context, /const contextEditButtonClass = "([^"]*min-w-\[[^\]]+\][^"]*)"/);
  assert.match(context, /className=\{contextEditButtonClass\}[\s\S]*Edit domain profile/);
  assert.match(context, /className=\{contextEditButtonClass\}[\s\S]*Edit voice & rules/);
});

test("settings maps raw errors to user copy, confirms a budget pause, and drops dev jargon", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");
  assert.match(settings, /function friendlyError/);
  assert.match(settings, /detail: friendlyError\(e\.message\)/);
  assert.match(settings, /function isProjectScopedMissing/);
  assert.match(settings, /if \(isProjectScopedMissing\(e\.message\)\)/);
  assert.doesNotMatch(settings, /Publisher connections unavailable", detail: e\.message/);
  assert.doesNotMatch(settings, /Search Console connection unavailable", detail: e\.message/);
  assert.doesNotMatch(settings, /Notifications unavailable", detail: e\.message/);
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
  for (const copy of [
    "Overall Metrics",
    "Total in review",
    "data-review-overall-metrics",
    "data-review-metric-card",
    "data-review-decision-section",
    "data-review-card",
    "data-review-drawer",
    "Review drawer actions",
  ]) {
    assert.match(review, new RegExp(copy));
  }
  assert.match(review, /motion-safe:animate-\[citeloop-drawer-panel-in_220ms_cubic-bezier\(0\.16,1,0\.3,1\)\]/);
  assert.match(review, /motion-safe:animate-\[citeloop-drawer-scrim-in_180ms_ease-out\]/);
  assert.match(review, /const reviewSurfaceRef = useRef<HTMLDivElement \| null>\(null\)/);
  assert.match(review, /const reviewDrawerRef = useRef<HTMLElement \| null>\(null\)/);
  assert.match(review, /const reviewReturnFocusRef = useRef<HTMLElement \| null>\(null\)/);
  assert.match(review, /reviewSurfaceRef\.current\.setAttribute\("aria-hidden", "true"\)/);
  assert.match(review, /reviewReturnFocusRef\.current\?\.focus\(\)/);
  assert.match(review, /\}, \[selectedArticle\?\.id\]\)/);
  assert.match(review, /event\.key === "Tab"/);
  assert.match(review, /aria-describedby=\{descriptionId\}/);
  assert.match(review, /data-review-card-description/);
  assert.match(review, /max-w-2xl/);
  assert.match(review, /No action needed/);
  assert.match(review, /Claim evidence map/);
  assert.match(review, /Asset type/);
  assert.match(review, /Source evidence/);
  assert.match(review, /assetMetadata/);
  assert.match(review, /How this article appears in search/);
  assert.match(review, /Preview/);
  assert.match(review, /applied the fix and approved the draft/);
  assert.match(review, /Apply QA fix/);
  assert.match(review, /reviewQueueSummary/);
  assert.match(review, /selectedArticleId/);
  assert.match(review, /queueArticles\.length === 0/);
  assert.doesNotMatch(review, /setSelectedArticleId\(queueArticles\[0\]\.article\.id\)/);
  assert.doesNotMatch(review, /Loading the first draft/);
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
  assert.doesNotMatch(review, /Applying & re-checking/);
  assert.doesNotMatch(review, /Select a draft to see the details\./);
  assert.doesNotMatch(review, /xl:grid-cols-\[minmax\(0,1fr\)_minmax\(420px,0\.9fr\)\]/);
  assert.match(articleDetail, /Cannot approve:/);
  assert.doesNotMatch(articleDetail, /qa blocking/);
});

test("review page opens linked Content Plan drafts from the article query param", () => {
  const review = read("projects/[id]/review/review-client.tsx");

  assert.match(review, /useSearchParams/);
  assert.match(review, /searchParams\.get\("article"\)/);
  assert.match(review, /requestedArticleId/);
  assert.match(review, /setSelectedArticleId\(requestedArticleId\)/);
  assert.match(review, /reviewCardRefs/);
  assert.match(review, /data-linked-review-card/);
  assert.match(review, /scrollIntoView\(\{ block: "center", behavior: "smooth" \}\)/);
  assert.match(review, /citeloop-linked-card-pulse/);
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

test("publishing first viewport is content-first Manual publish instead of platform-first C2", () => {
  const publishing = read("projects/[id]/publishing/publishing-client.tsx");
  const workflow = read("projects/[id]/content-workflow-client.tsx");

  assert.match(publishing, /buildPublishDestinations/);
  assert.match(publishing, /buildPublishHeaderCta/);
  assert.match(publishing, /buildReadyNow/);
  assert.match(publishing, /data-publish-c2-first-viewport/);
  assert.match(publishing, /data-publish-ready-to-post/);
  assert.match(publishing, /data-publish-c2-destinations/);
  assert.match(publishing, /title="Ready to post"/);
  assert.match(publishing, /title="Publish destinations"/);
  assert.match(workflow, /Ready content first\. Choose where and when per post\./);
  assert.match(publishing, /onDestination=\{\(\) => setDrawer\("github"\)\}/);
  assert.match(publishing, /onTiming=\{\(\) => setDrawer\("schedule"\)\}/);
  assert.match(publishing, />\s*Schedule\s*</);
  assert.match(publishing, />\s*View all\s*</);
  assert.ok(
    publishing.indexOf("data-publish-ready-to-post") < publishing.indexOf("data-publish-c2-destinations"),
    "Ready content should appear before destinations in the first viewport source order",
  );

  for (const oldLane of [
    'title="Ready to publish"',
    'title="Scheduled to publish"',
    'title="Publishing failed"',
    '<SectionHeader title="Syndication"',
  ]) {
    assert.ok(!publishing.includes(oldLane), `${oldLane} should not be a first-viewport lane`);
  }
});

test("publishing destination tiles replace the platform popover and keep settings as the connection boundary", () => {
  const publishing = read("projects/[id]/publishing/publishing-client.tsx");
  const logic = read("lib/publish-destinations-logic.ts");
  const settings = read("projects/[id]/settings/settings-client.tsx");

  for (const contract of [
    "function DestinationTile",
    "data-publish-destination-tile",
    "GitHub/Next.js",
    "CMS roadmap",
    "Manage in Settings",
    "Retry test",
    "Check status",
    "Checking status",
  ]) {
    assert.match(publishing, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  for (const contract of ["dev_to", "hashnode", "reddit", "medium", "linkedin", "hacker_news", "WordPress", "Webflow", "Shopify", "Custom CMS"]) {
    assert.match(logic, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.doesNotMatch(publishing, />\s*Reconcile\s*</);
  assert.doesNotMatch(publishing, /busyLabel="Reconciling"/);
  assert.doesNotMatch(publishing, /platformsOpen/);
  assert.doesNotMatch(publishing, /platformsMenuRef/);
  assert.doesNotMatch(publishing, /drawer === "platforms"/);
  assert.doesNotMatch(publishing, /async function savePublisherConnection/);
  assert.doesNotMatch(publishing, /async function disconnectConnection/);
  assert.doesNotMatch(publishing, />\s*Publisher account\s*</i);
  assert.doesNotMatch(publishing, /<select[\s\S]*No enabled connections[\s\S]*<\/select>/);
  assert.ok(publishing.includes('href={`/projects/${projectId}/settings#publisher`}'));

  assert.match(settings, /setPublisherConnectionEnabled/);
  assert.match(settings, />\s*Enable\s*</);
  assert.match(settings, />\s*Disable\s*</);
});

test("publishing Ready now strip uses publish retry and preview actions", () => {
  const publishing = read("projects/[id]/publishing/publishing-client.tsx");
  const logic = read("lib/publish-destinations-logic.ts");

  const readyNowBlock = publishing.slice(
    publishing.indexOf("function ReadyNowStrip"),
    publishing.indexOf("function ManualPlatformRows"),
  );

  assert.match(readyNowBlock, /title="Ready to post"/);
  assert.match(readyNowBlock, /articlePreviewHref\(projectId, item\.article\)/);
  assert.match(readyNowBlock, /target="_blank"/);
  assert.match(readyNowBlock, /rel="noopener noreferrer"/);
  assert.match(readyNowBlock, /item\.secondaryActionLabel/);
  assert.match(readyNowBlock, /SEO Details/);
  assert.match(readyNowBlock, /onSeoDetails\(item\.article\)/);
  assert.match(readyNowBlock, /item\.destinationActionLabel/);
  assert.match(readyNowBlock, /item\.timingActionLabel/);
  assert.match(readyNowBlock, /publishTimeLabel\(item\.article\)/);
  assert.match(readyNowBlock, /item\.actionLabel/);
  assert.match(readyNowBlock, /item\.action === "retry" \? "Retrying" : "Queuing"/);
  assert.match(readyNowBlock, /line-clamp-2 break-words/);
  assert.match(readyNowBlock, /readyNow\.emptyState\.title/);
  assert.match(logic, /No approved posts ready/);
  assert.doesNotMatch(readyNowBlock, />\s*Review\s*</);
  assert.ok(
    !readyNowBlock.includes('href={`/projects/${projectId}/articles/${item.articleId}`}'),
    "Preview should not route to the internal article detail page",
  );
  assert.match(publishing, /drawer === "seo_details"/);
  assert.match(publishing, /dataAttribute="publish-seo-details-drawer"/);
  assert.match(publishing, /function SEODetailsDrawerContent/);
});

test("publishing manual drafts and View all drawer preserve non-first-viewport states", () => {
  const publishing = read("projects/[id]/publishing/publishing-client.tsx");
  const logic = read("lib/publish-destinations-logic.ts");

  assert.match(publishing, /data-manual-syndication-chips/);
  assert.match(publishing, /function ManualPlatformRows/);
  assert.match(publishing, /Copy/);
  assert.match(publishing, /Open/);
  assert.match(publishing, /Mark distributed/);
  assert.match(publishing, /Unlocks after the canonical URL is published and verified\./);

  const waitingRowsBlock = publishing.slice(
    publishing.indexOf("platform.waitingRows.length > 0"),
    publishing.indexOf("platform.readyRows.length === 0"),
  );
  assert.doesNotMatch(waitingRowsBlock, /onCopy/);
  assert.doesNotMatch(waitingRowsBlock, /onMarkDistributed/);

  assert.match(publishing, /data-publish-view-all-drawer/);
  assert.match(publishing, /data-publish-operational-group/);
  assert.match(publishing, /buildPublishingOperationalGroups/);
  for (const group of ["Ready", "Scheduled", "Published", "Failed", "Waiting on canonical", "Ready to distribute"]) {
    assert.match(logic, new RegExp(group.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("renamed dashboard routes exist and legacy routes redirect", () => {
  for (const route of [
    "projects/[id]/context/page.tsx",
    "projects/[id]/analysis/page.tsx",
    "projects/[id]/plan/page.tsx",
    "projects/[id]/publish/page.tsx",
    "projects/[id]/results/page.tsx",
    "projects/[id]/settings/activity/page.tsx",
  ]) {
    assert.equal(exists(route), true, `${route} should exist`);
  }

  const redirects = new Map([
    ["projects/[id]/knowledge/page.tsx", "/context"],
    ["projects/[id]/topics/page.tsx", "/plan"],
    ["projects/[id]/publishing/page.tsx", "/publish"],
    ["projects/[id]/opportunities/page.tsx", "/analysis"],
    ["projects/[id]/visibility/page.tsx", "/results"],
    ["projects/[id]/seo/page.tsx", "/results"],
    ["projects/[id]/runs/page.tsx", "/settings#activity"],
  ]);

  for (const [route, target] of redirects) {
    const source = read(route);
    assert.match(source, /redirect\(/, `${route} should redirect`);
    assert.match(source, new RegExp(target.replace("/", "\\/")), `${route} should redirect to ${target}`);
  }
});

test("content plan review and publish form one continuous workflow surface", () => {
  assert.equal(exists("projects/[id]/content-workflow-client.tsx"), true, "content workflow wrapper should exist");

  const workflow = read("projects/[id]/content-workflow-client.tsx");
  const plan = read("projects/[id]/plan/page.tsx");
  const reviewPage = read("projects/[id]/review/page.tsx");
  const publishPage = read("projects/[id]/publish/page.tsx");

  for (const route of [
    [plan, 'initialStep="plan"'],
    [reviewPage, 'initialStep="review"'],
    [publishPage, 'initialStep="publish"'],
  ]) {
    assert.match(route[0], /ContentWorkflowClient/);
    assert.match(route[0], new RegExp(route[1]));
  }

  for (const contract of [
    "ContentWorkflowClient",
    "TopicsClient",
    "ReviewClient",
    "PublishingClient",
    "content-workflow-plan",
    "content-workflow-review",
    "content-workflow-publish",
    "data-content-workflow-section",
    "scrollToStep(initialStep",
    "window.history.replaceState",
    "window.addEventListener(\"scroll\"",
  ]) {
    assert.match(workflow, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.match(workflow, /syncPathToStep\(nextStep\)/);
  assert.match(workflow, /window\.location\.pathname !== nextHref/);
  assert.doesNotMatch(workflow, /scroll-snap|snap-y|snap-mandatory/);
});

test("content workflow PRD defines stage identity and click acceptance criteria", () => {
  assert.equal(
    exists("../../docs/PRD-CiteLoop-Content-Workflow-Section-Identity.md"),
    true,
    "focused PRD should document the Content workflow stage identity work",
  );

  const prd = read("../../docs/PRD-CiteLoop-Content-Workflow-Section-Identity.md");

  for (const copy of [
    "Content Plan is Step 1 of 3",
    "Review is Step 2 of 3",
    "Publish is Step 3 of 3",
    "clicking Review left the Review section lower than the top of the work area",
    "clicking Publish could still show Content Plan selected and visible",
    "Clicking the left navigation `Review` entry lands the Review stage at the",
    "Clicking the left navigation `Publish` entry lands the Publish stage at",
    "No `scroll-snap` or mandatory snapping styles are introduced",
  ]) {
    assert.match(prd, new RegExp(copy.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("content workflow stages put stage identity before step metadata", () => {
  const workflow = read("projects/[id]/content-workflow-client.tsx");
  const stageActions = read("projects/[id]/content-workflow-stage-actions.tsx");
  const topics = read("projects/[id]/topics/topics-client.tsx");
  const review = read("projects/[id]/review/review-client.tsx");
  const publishing = read("projects/[id]/publishing/publishing-client.tsx");

  for (const contract of [
    "STAGE_META",
    "Step 1 of 3",
    "Step 2 of 3",
    "Step 3 of 3",
    "data-content-workflow-stage-shell",
    "data-content-workflow-stage-accent",
    "data-content-workflow-stage-title",
    "data-content-workflow-stage-header-action",
    "data-content-workflow-stage-step",
    "data-content-workflow-stage-body",
  ]) {
    assert.match(workflow, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.match(workflow, /title: "Content Plan"[\s\S]*title: "Review"[\s\S]*title: "Publish"/);
  assert.match(workflow, /eyebrow: "Planned topics and action handoff"/);
  assert.doesNotMatch(workflow, /eyebrow: "Topic backlog and action handoff"/);
  assert.match(workflow, /toneClass: "border-sky-200 bg-sky-100\/70"/);
  assert.match(workflow, /toneClass: "border-amber-200 bg-amber-100\/70"/);
  assert.match(workflow, /toneClass: "border-emerald-200 bg-emerald-100\/70"/);
  assert.match(workflow, /<h1[\s\S]*data-content-workflow-stage-title[\s\S]*\{meta\.title\}[\s\S]*<\/h1>/);
  assert.ok(
    workflow.indexOf("data-content-workflow-stage-title") < workflow.indexOf("data-content-workflow-stage-step"),
    "stage title should appear before the step metadata",
  );
  assert.ok(
    workflow.indexOf("data-content-workflow-stage-title") < workflow.indexOf("data-content-workflow-stage-header-action"),
    "stage actions should share the title row instead of sitting below the divider",
  );
  assert.ok(
    workflow.indexOf("data-content-workflow-stage-header-action") < workflow.indexOf("data-content-workflow-stage-step"),
    "stage actions should appear above the step metadata",
  );
  assert.match(stageActions, /ContentWorkflowStageHeaderActionContext/);
  assert.match(stageActions, /function ContentWorkflowStageHeaderAction/);
  assert.match(stageActions, /createPortal/);
  assert.match(workflow, /ContentWorkflowStageHeaderActionContext\.Provider/);
  assert.match(topics, /<ContentWorkflowStageHeaderAction>\s*\{autoSwitch\}\s*<\/ContentWorkflowStageHeaderAction>/);
  assert.match(review, /<ContentWorkflowStageHeaderAction>\s*\{reviewHeaderAction\}\s*<\/ContentWorkflowStageHeaderAction>/);
  assert.match(publishing, /<ContentWorkflowStageHeaderAction>\s*\{publishingHeaderAction\}\s*<\/ContentWorkflowStageHeaderAction>/);
  assert.doesNotMatch(workflow, /rounded-full border border-white\/80 bg-white\/70/);
  assert.doesNotMatch(topics, /<SectionHeader title="Content Plan"/);
  assert.doesNotMatch(review, /<SectionHeader[\s\S]{0,120}title="Review"[\s\S]{0,120}level="page"/);
  assert.doesNotMatch(publishing, /<SectionHeader[\s\S]{0,120}title="Publish"[\s\S]{0,120}level="page"/);
  assert.doesNotMatch(topics, /<section className="flex min-h-8 items-center justify-end">/);
  assert.doesNotMatch(review, /<div className="flex min-h-8 items-center justify-end">/);
  assert.doesNotMatch(publishing, /<div className="flex min-h-8 items-center justify-end">/);
  assert.doesNotMatch(publishing, /title="Publishing"/);
  assert.match(workflow, /data-content-workflow-stage-step className="text-sm font-bold uppercase/);
});

test("content workflow route clicks retry target alignment while content settles", () => {
  const workflow = read("projects/[id]/content-workflow-client.tsx");

  for (const contract of [
    "TARGET_TOP_OFFSET",
    "TARGET_ALIGNMENT_TOLERANCE",
    "TARGET_SETTLE_TIMEOUT_MS",
    "pendingTargetRef",
    "pendingStartedAtRef",
    "isStepAligned",
    "window.requestAnimationFrame(settleTarget)",
    "pendingTargetRef.current = initialStep",
  ]) {
    assert.match(workflow, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.match(workflow, /if \(pendingTargetRef\.current[\s\S]*return/);
  assert.match(workflow, /scrollToStep\(initialStep, "auto"\)/);
  assert.match(workflow, /window\.dispatchEvent\(new CustomEvent\(CONTENT_WORKFLOW_PATH_CHANGE_EVENT/);
});

test("content workflow scroll keeps project shell navigation active state in sync", () => {
  const shell = read("components/project-shell.tsx");
  const workflow = read("projects/[id]/content-workflow-client.tsx");

  assert.match(workflow, /ACTIVE_STEP_MARKER_MAX_OFFSET/);
  assert.match(workflow, /Math\.min\(window\.innerHeight \* 0\.35, TARGET_TOP_OFFSET \+ ACTIVE_STEP_MARKER_MAX_OFFSET\)/);
  assert.match(workflow, /CONTENT_WORKFLOW_PATH_CHANGE_EVENT/);
  assert.match(workflow, /window\.dispatchEvent/);
  assert.match(workflow, /detail: \{ pathname: nextHref \}/);

  assert.match(shell, /CONTENT_WORKFLOW_PATH_CHANGE_EVENT/);
  assert.match(shell, /const \[activePathname, setActivePathname\] = useState\(pathname\)/);
  assert.match(shell, /setActivePathname\(pathname\)/);
  assert.match(shell, /window\.addEventListener\(CONTENT_WORKFLOW_PATH_CHANGE_EVENT, onContentWorkflowPathChange/);
  assert.match(shell, /window\.removeEventListener\(CONTENT_WORKFLOW_PATH_CHANGE_EVENT, onContentWorkflowPathChange/);
  assert.match(shell, /isActive\(activePathname, projectId, item\.href\)/);
  assert.doesNotMatch(shell, /isActive\(pathname, projectId, item\.href\)/);
});

test("home leads with linked metrics instead of hero or refresh-context prompts", () => {
  const workspace = read("projects/[id]/workspace.tsx");
  const dashboardLogic = read("lib/dashboard-ux-logic.ts");

  for (const copy of [
    "metricGridCards",
    "metricChangeLabel",
    "accountProjects",
    "otherProjects",
    "primaryAction",
    "nextWorkspaceAction",
    "homeAICitationMetric",
    "homeInMotionMetric",
    "Organic traffic",
    "Published pages",
  ]) {
    assert.match(workspace, new RegExp(copy));
  }

  for (const copy of ["AI citation gaps", "Review opportunities", "View opportunities", "In motion"]) {
    assert.match(dashboardLogic, new RegExp(copy));
  }

  for (const route of [
    "href: `/projects/${projectId}/analysis`",
    "href: `/projects/${projectId}/results`",
    "href: `/projects/${projectId}/publish`",
  ]) {
    assert.match(workspace, new RegExp(route.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  for (const removed of [
    "growthHeadline",
    "growthDetail",
    "Your next step",
    "Growth loop",
    "CiteLoop is measuring growth from published work",
    "Connect your product to start the growth loop",
    "<RefreshCw",
  ]) {
    assert.doesNotMatch(workspace, new RegExp(removed.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
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

test("home growth metrics are first-viewport linked cards with honest change labels", () => {
  const workspace = read("projects/[id]/workspace.tsx");
  const dashboardLogic = read("lib/dashboard-ux-logic.ts");

  for (const copy of [
    "metricGridCards",
    "metricChangeLabel",
    "metricChangeTone",
    "featured",
    "MetricIcon",
    "homeAICitationMetric",
    "homeInMotionMetric",
    "Organic traffic",
    "Published pages",
    "Search Console connected",
    "this month",
    "View",
  ]) {
    assert.match(workspace, new RegExp(copy));
  }
  for (const copy of ["AI citation gaps", "In motion", "already in execution", "0 active now"]) {
    assert.match(dashboardLogic, new RegExp(copy));
  }

  // The decorative hardcoded SVG growth curve is removed — no fake data on Home.
  assert.doesNotMatch(workspace, /growthTrendPath/);
  assert.doesNotMatch(workspace, /Growth metric trend/);
  assert.doesNotMatch(workspace, /growthMetricFill/);
});

test("home shows other account projects only when more than one project is available", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const copy of [
    "api.listProjects().catch(() => [])",
    "setAccountProjects(projectRows)",
    "const otherProjects = accountProjects.filter",
    "otherProjects.length > 0",
    "Other projects",
    "Switch",
  ]) {
    assert.match(workspace, new RegExp(copy.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.match(workspace, /href=\{`\/projects\/\$\{candidate\.id\}`\}/);
});

test("home explains growth status and loop stages from existing product data", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const copy of [
    "Connect Search Console for traffic",
    "Connect for proof",
    "Opportunities",
    "Content Plan",
    "Review",
    "Publish",
    "Results",
    "Needs you",
    "Activity",
  ]) {
    assert.match(workspace, new RegExp(copy));
  }

  for (const apiCall of [
    "api.getProfile(projectId)",
    "api.listInventory(projectId)",
    "api.getSEOOverview(projectId)",
    "api.getVisibilitySummary(projectId)",
  ]) {
    assert.match(workspace, new RegExp(apiCall.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.doesNotMatch(workspace, /api\.listSEOOpportunities\(projectId, \{ status: "open", limit: 50 \}\)/);
  assert.doesNotMatch(workspace, /api\.listSEOContentActions\(projectId, \{ limit: 50 \}\)/);

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
  assert.match(workspace, /visibilityActionsInLoopCount/);
  assert.match(workspace, /visibilityOpenOpportunityCount/);
  assert.match(workspace, /planItemCount/);
  assert.match(workspace, /opportunitiesInPlanCount/);
  // Stage statuses are derived live from page state, not hardcoded.
  assert.match(workspace, /Generating \(auto\)/);
  assert.match(workspace, /Plan ready/);
  assert.match(workspace, /Needs approval/);
  assert.doesNotMatch(workspace, /Select a planned topic to create the next draft/);
});

test("home renders the loop as a single connected pipeline stepper", () => {
  const workspace = read("projects/[id]/workspace.tsx");
  const stagesBlock = workspace.slice(workspace.indexOf("const stages:"), workspace.indexOf("const nextScheduledRow"));

  // One ordered daily pipeline; Context stays in setup/utility surfaces.
  for (const copy of ["Pipeline", "stageDotClass"]) {
    assert.match(workspace, new RegExp(copy));
  }
  for (const copy of [
    "stages",
    "Opportunities",
    "Content Plan",
    "Review",
    "Publish",
    "Results",
    "statusLabel",
  ]) {
    assert.match(stagesBlock, new RegExp(copy));
  }
  for (const removedDailyStage of ['label: "Context"', 'label: "Analysis"', 'label: "Plan"', 'label: "Drafts"', 'label: "Measurement"']) {
    assert.doesNotMatch(stagesBlock, new RegExp(removedDailyStage.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  assert.match(stagesBlock, /href: `\/projects\/\$\{projectId\}\/analysis`/);
  assert.match(stagesBlock, /href: `\/projects\/\$\{projectId\}\/plan`/);
  assert.match(stagesBlock, /href: `\/projects\/\$\{projectId\}\/review`/);
  assert.match(stagesBlock, /href: `\/projects\/\$\{projectId\}\/publish`/);
  assert.match(stagesBlock, /href: `\/projects\/\$\{projectId\}\/results`/);

  // The decorative circular loop, arrow connectors, and 3x3 grid are gone.
  assert.doesNotMatch(workspace, /loopConnectorLabels/);
  assert.doesNotMatch(workspace, /loopGridClass/);
  assert.doesNotMatch(workspace, /ConnectorIcon/);
  assert.doesNotMatch(workspace, /data-loop-position/);
  assert.doesNotMatch(workspace, /grid-cols-\[2rem_1fr_2rem\]/);
});

test("settings exposes activity log as the secondary home for automation audit details", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");
  const activityPage = read("projects/[id]/settings/activity/page.tsx");

  assert.match(settings, /Activity Log/);
  assert.match(settings, /import \{ RunsClient \} from "\.\.\/runs\/runs-client"/);
  assert.match(settings, /<RunsClient projectId=\{projectId\} embeddedInSettings/);
  assert.doesNotMatch(settings, /Open Activity Log/);
  assert.doesNotMatch(settings, /\/settings\/activity/);

  assert.match(activityPage, /redirect\(`\/projects\/\$\{id\}\/settings#activity`\)/);
  assert.doesNotMatch(activityPage, /<RunsClient/);
});

test("settings groups every top-level section behind a tab", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");
  const expectedTabs = [
    "Project config",
    "Activity Log",
    "Search Console connection",
    "Publisher connection",
    "Crawl config",
    "Notifications",
  ];

  assert.match(settings, /type SettingsTabId =/);
  assert.match(settings, /const settingsTabs:/);
  assert.match(settings, /role="tablist"/);
  assert.match(settings, /role="tab"/);
  assert.match(settings, /aria-selected=\{activeSettingsTab === tab\.id\}/);
  assert.match(settings, /role="tabpanel"/);
  assert.match(settings, /activeSettingsTab === "project" && \(/);
  assert.match(settings, /activeSettingsTab === "activity" && \(/);
  assert.match(settings, /activeSettingsTab === "search-console" && \(/);
  assert.match(settings, /activeSettingsTab === "publisher" && \(/);
  assert.match(settings, /activeSettingsTab === "crawl" && \(/);
  assert.match(settings, /activeSettingsTab === "notifications" && \(/);
  assert.doesNotMatch(settings, /activeSettingsTab === "subscriptions" && \(/);
  assert.doesNotMatch(settings, /activeSettingsTab === "deliveries" && \(/);
  assert.doesNotMatch(settings, /\| "subscriptions"/);
  assert.doesNotMatch(settings, /\| "deliveries"/);
  const notificationsPanel = settings.slice(settings.indexOf('id="settings-panel-notifications"'));
  assert.match(notificationsPanel, /title="Subscriptions"[\s\S]*Channels[\s\S]*Events[\s\S]*title="Deliveries"/);
  assert.doesNotMatch(notificationsPanel, /title="Notifications"/);
  assert.doesNotMatch(notificationsPanel, /lg:grid-cols-\[220px_1fr\]/);
  assert.match(settings, /activeEventsChannel/);
  assert.match(settings, /openChannelEvents/);
  assert.match(settings, /saveChannelEvents/);
  assert.match(settings, /busyLabel="Saving events"/);
  assert.match(settings, />\s*Save\s*</);
  assert.match(settings, />\s*Cancel\s*</);
  assert.doesNotMatch(settings, /Notification subscriptions/);
  assert.doesNotMatch(settings, /Notification deliveries/);

  const tabModel = settings.slice(settings.indexOf("const settingsTabs:"), settings.indexOf("export function SettingsClient"));
  assert.doesNotMatch(tabModel, /Notification subscriptions/);
  assert.doesNotMatch(tabModel, /Notification deliveries/);

  for (const tab of expectedTabs) {
    const escapedTab = tab.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    assert.match(settings, new RegExp(`title: "${escapedTab}"`));
  }
});

test("settings deep links open the matching configuration tab", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");

  assert.match(settings, /function settingsTabFromHash/);
  assert.match(settings, /window\.location\.hash/);
  assert.match(settings, /const \[activeSettingsTab, setActiveSettingsTab\] = useState<SettingsTabId>\("project"\)/);
  assert.doesNotMatch(settings, /useState<SettingsTabId>\(\(\) => \{[\s\S]*window\.location\.hash[\s\S]*\}\)/);
  assert.match(settings, /setActiveSettingsTab\(settingsTabFromHash\(window\.location\.hash\)\)/);
  assert.match(settings, /function openSettingsAnchor/);
  assert.match(settings, /setActiveSettingsTab\(settingsTabFromHash\(nextHash\)\)/);
  assert.match(settings, /window\.history\.replaceState\(null, "", nextHash\)/);
  assert.match(settings, /function activateSettingsTab/);
  assert.match(settings, /window\.history\.replaceState/);
  assert.match(settings, /#\$\{tabId\}/);
  assert.match(settings, /onClick=\{\(\) => activateSettingsTab\(tab\.id\)\}/);
});

test("settings exposes Automation as the system setup tab", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");

  for (const expected of [
    '| "automation"',
    'id: "automation", title: "Automation"',
    'activeSettingsTab === "automation" && (',
    'id="settings-panel-automation"',
    'id="automation"',
    'id="automation-policy"',
    'id="recovery-plan"',
    "settingsAnchorToTab",
    '"automation-policy": "automation"',
    '"recovery-plan": "automation"',
    "monthly_budget_limit",
    "Autopilot budget",
    "Recovery plan",
  ]) {
    assert.equal(settings.includes(expected), true, `settings-client.tsx missing ${expected}`);
  }

  assert.doesNotMatch(settings, /id="general"/);
  assert.doesNotMatch(settings, /#general/);
});

test("automation readiness splits blocked from ready and gives every blocked gate a fix link", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");

  for (const expected of [
    "readinessGateActionFor",
    "automationReadinessCards",
    "selectedAutomationCheck",
    "openAutomationCheck",
    "const blockedGates",
    "const readyGates",
    "Readiness checks",
    "checks ready",
    "aspect-square",
    "border-emerald-200 bg-emerald-50",
    "border-red-200 bg-red-50",
    // blocked gates open a focused details modal, not just instruction text
    "Automation check details",
    "role=\"dialog\"",
    "aria-modal=\"true\"",
    "selectedAutomationCard.action?.cta",
    // plain-language explainer for the jargon
    "guarded automation (Level",
    "Emergency stop",
  ]) {
    assert.equal(settings.includes(expected), true, `settings-client.tsx missing ${expected}`);
  }

  // the old undifferentiated "Blocked gates / Plans / Open safe mode" stat grid is gone
  assert.doesNotMatch(settings, /Latest plan status/);
});

test("automation policy is edited from a confirmable modal instead of auto-saving field blur", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");
  const api = read("lib/api.ts");

  for (const expected of [
    "Automation Policy",
    "Automation status",
    "automation_paused",
    "Pause automation",
    "Resume automation",
    "Automation is paused. CiteLoop will not run scheduled automation or execute changes automatically.",
    "Automation is active. CiteLoop may run scheduled automation according to the autonomy level below.",
    "Autonomy level",
    "Level 0 Observe only",
    "Level 1 Draft only",
    "Level 2 Guarded execution",
    "Level 3 Future",
    "Level 4 Future",
    "policyDraft",
    "setPolicyDraft",
    "savePolicyDraft",
    "Save policy",
    "Review and save policy changes",
    "Emergency stop is on",
    "Review emergency stop",
    "openPolicyCheck",
    'setSelectedAutomationCheck("autopilot_policy_confirmed")',
    "Promise<boolean>",
    "return true",
    "return false",
    "exitOpenSafeModeEvents",
    "api.exitSafeMode",
    "Open safe mode events",
    "Safe mode policy switch is on",
    "open safe mode event",
  ]) {
    assert.equal(settings.includes(expected), true, `settings-client.tsx missing ${expected}`);
  }

  assert.match(api, /exitSafeMode: async/);
  assert.match(api, /\/projects\/\$\{id\}\/seo\/autopilot\/safe-mode\/\$\{safeModeID\}\/exit/);

  assert.doesNotMatch(settings, /onBlur=\{\(event\) => saveAutomationPolicy/);
  assert.doesNotMatch(settings, /onChange=\{\(event\) => saveAutomationPolicy\(\{ kill_switch_enabled/);
  assert.doesNotMatch(settings, /onChange=\{\(event\) => saveAutomationPolicy\(\{ safe_mode_enabled/);
  assert.doesNotMatch(settings, /saveAutomationPolicy\(\{ kill_switch_enabled: false/);
  assert.doesNotMatch(settings, /api\.updateSEOPolicy\(projectId, \{ \.\.\.policy, safe_mode_enabled: false \}\)/);
  assert.doesNotMatch(settings, />\s*Turn off emergency stop\s*</);
});

test("recovery plan check requires review before acknowledgement", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");

  for (const expected of [
    "reviewedRecoveryPlan",
    "Review recovery plan",
    "Manual recovery plan",
    "Return to recovery check",
    "Settings - automation - recovery plan",
    "recovery-plan-review",
    "recovery-plan-return",
    "recovery_plan_acknowledged",
  ]) {
    assert.equal(settings.includes(expected), true, `settings-client.tsx missing ${expected}`);
  }

  assert.match(settings, /selectedAutomationCard\.id === "rollback_or_recovery_ready"[\s\S]*reviewedRecoveryPlan/);
  assert.match(settings, /Confirm recovery plan/);
  assert.doesNotMatch(settings, /selectedAutomationCard\.id === "rollback_or_recovery_ready"\s*\?\s*"Confirm recovery plan"/);
});

test("notifications setup has an empty state that explains the automation gate", () => {
  const settings = read("projects/[id]/settings/settings-client.tsx");
  const notificationsPanel = settings.slice(settings.indexOf('id="settings-panel-notifications"'));

  for (const expected of [
    "Automation needs a notification channel",
    "Failures, approval requests, safe mode alerts, and delivery problems should reach an operator.",
    "Add a Slack or Discord webhook, then send a test notification to verify it.",
    "Set notifications",
    "create-channel",
  ]) {
    assert.equal(notificationsPanel.includes(expected), true, `notifications panel missing ${expected}`);
  }

  assert.doesNotMatch(notificationsPanel, />\s*No channels\s*</);
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
    "Set up Context",
    "What CiteLoop checks",
    "Product understanding",
    "Writing boundaries",
    "View source pages",
    "Source coverage",
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

  assert.doesNotMatch(context, /No voice rules yet/);
  assert.doesNotMatch(context, /<SectionHeader title="Source pages"/);
});

test("connected context refreshes the fixed project domain and shows crawl freshness", () => {
  const context = read("projects/[id]/knowledge/knowledge-client.tsx");
  const connectedPanel = context.slice(context.indexOf("function ContextHealthPanel"), context.indexOf("function SummaryGroup"));

  assert.match(connectedPanel, /Update context/);
  assert.match(connectedPanel, /Last updated/);
  assert.match(context, /api\.refreshContext\(projectId\)/);
  assert.doesNotMatch(connectedPanel, /placeholder="https:\/\/product-domain\.com"/);
  assert.doesNotMatch(connectedPanel, /onLandingChange/);
});

test("connected context keeps update action top-right and crawl freshness at the bottom", () => {
  const context = read("projects/[id]/knowledge/knowledge-client.tsx");
  const connectedPanel = context.slice(context.indexOf("function ContextHealthPanel"), context.indexOf("function SummaryGroup"));
  const updateIndex = connectedPanel.indexOf("Update context");
  const metricsIndex = connectedPanel.indexOf("What CiteLoop knows");
  const lastUpdatedIndex = connectedPanel.indexOf("Last updated");

  assert.match(connectedPanel, /lg:grid-cols-\[minmax\(0,1fr\)_auto\]/);
  assert.match(connectedPanel, /flex flex-col items-stretch gap-2 sm:items-end/);
  assert.ok(updateIndex > -1 && updateIndex < metricsIndex, "Update context should stay in the top-right action area");
  assert.ok(lastUpdatedIndex > metricsIndex, "Last updated should render as bottom metadata");
  assert.doesNotMatch(connectedPanel, /Updated \{formatDate\(updatedAt\)\}/);
});

test("analysis page presents decisions and results page presents impact reports", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  const analysisPage = read("projects/[id]/analysis/page.tsx");
  const resultsPage = read("projects/[id]/results/page.tsx");

  for (const copy of [
    "Opportunities",
    "Analysis",
    "Opportunity Queue",
    "Site Fixes",
    "Loop in motion",
    "View results",
    "Impact Reports",
    "Results and learning",
    "Weekly analysis brief",
    "Add to Content Plan",
    "Public crawl only",
    "Opportunity details",
    "Open details",
    "Drawer actions",
  ]) {
    assert.match(seo, new RegExp(copy));
  }

  assert.match(analysisPage, /AnalysisClient/);
  assert.match(resultsPage, /ResultsClient/);
  assert.doesNotMatch(seo, /title="SEO"/);
  assert.doesNotMatch(seo, /title="Visibility overview"/);
});

test("activity log defaults to user-facing attention cards and opens fix details in a drawer", () => {
  const activity = read("projects/[id]/runs/runs-client.tsx");

  for (const copy of [
    "Activity Log",
    "Needs attention",
    "Recent successful activity",
    "User impact",
    "Next action",
    "AttentionRunCard",
    "RightDrawer",
    "What failed",
    "How to fix",
    "AI-ready fix brief",
    "Copy AI fix brief",
    "userVisibleActivityRuns",
    "isUserAttentionRun",
    'dataAttribute="activity-run-drawer"',
    "aspect-square",
  ]) {
    assert.match(activity, new RegExp(copy));
  }

  assert.match(activity, /setSelectedRun\(run\)/);
  assert.match(activity, /rawError\(run\)/);
  assert.match(activity, /fixGuidance\(run\)/);
  assert.match(activity, /aiFixBrief\(selectedRun, projectId\)/);
  assert.match(activity, /summary\.visible\.length === 0/);
  assert.doesNotMatch(activity, /Advanced details/);

  for (const defaultHeader of [">Agent<", ">Cost<", ">Model<"]) {
    assert.doesNotMatch(activity, new RegExp(defaultHeader.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.doesNotMatch(activity, /title="Runs"/);
});

test("content plan removes the legacy domain strategist entry point", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");

  assert.doesNotMatch(topics, /busy === "strategist"/);
  assert.doesNotMatch(topics, /Generating content plan/);
  assert.doesNotMatch(topics, /Running strategist/);
  assert.doesNotMatch(topics, /Domain generation stays available/);
  assert.match(topics, /Starting draft generation/);
});

test("content plan exposes an Auto switch for the automatic workflow", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");

  assert.match(topics, /<ContentWorkflowStageHeaderAction>\s*\{autoSwitch\}\s*<\/ContentWorkflowStageHeaderAction>/);
  assert.match(topics, /auto_advance_enabled/);
  assert.match(topics, /const autoEnabled = Boolean\(config\?\.auto_advance_enabled\)/);
  assert.match(topics, /toggleAutoAdvance/);
  assert.match(topics, /const autoSwitch = \(/);
  assert.match(topics, /role="switch"/);
  assert.match(topics, /aria-checked=\{autoEnabled\}/);
  assert.match(topics, /aria-describedby="content-plan-auto-help"/);
  assert.match(topics, /id="content-plan-auto-help"/);
  assert.match(topics, /group-hover:opacity-100/);
  assert.match(topics, /Auto On: accepted opportunities become planned topics and drafts on cadence\./);
  assert.match(topics, /Auto Off: automatic planning and drafting pause; manual drafting stays available from reviewed briefs and planned topics\./);
  assert.match(topics, />Auto<\/span>/);
  assert.match(topics, /api\.updateConfig\(projectId, \{ \.\.\.base, auto_advance_enabled: nextEnabled \}\)/);
  assert.doesNotMatch(topics, /const autoPlan:/);
  assert.doesNotMatch(topics, /autoPlanToneClass/);
  assert.doesNotMatch(topics, /autoPlan\.title/);
  assert.doesNotMatch(topics, /Turn Auto on to convert accepted opportunities into backlog topics and draft them on cadence/);
});

test("content plan treats topic generation as a per-topic background operation", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");

  assert.match(topics, /generatingIds/);
  assert.match(topics, /isGenerating/);
  assert.match(topics, /Starting draft generation/);
  assert.doesNotMatch(topics, /disabled=\{!!busy \|\| topic\.status === "archived"\} size="sm" variant="primary" onClick=\{\(\) => generate\(topic\)\}/);
});

test("content plan polls accepted opportunity actions until topics appear", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");

  assert.match(topics, /api\.getVisibilitySummary\(projectId\)/);
  assert.match(topics, /summaryPendingPlanActions/);
  assert.match(topics, /\["added_to_plan", "planned", "drafting", "ready_for_review"\]\.includes\(action\.lifecycle_stage\)/);
  assert.match(topics, /hasPendingPlanActions/);
  assert.match(topics, /autoEnabled && summaryPendingPlanActions > 0 && topics\.length === 0/);
  assert.match(topics, /const hasDueScheduled = autoEnabled && topics\.some/);
  assert.match(topics, /window\.setInterval\(refresh, hasGenerating \? 10_000 : hasPendingPlanActions \? 5_000 : 30_000\)/);
  assert.doesNotMatch(topics, /api\.listSEOOpportunities\(projectId, \{ status: "open", limit: 50 \}\)/);
  assert.doesNotMatch(topics, /api\.listSEOContentActions\(projectId, \{ limit: 50 \}\)/);
});

test("content plan lets users draft accepted opportunities manually when Auto is off", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");

  assert.match(topics, /draftAcceptedAction/);
  assert.match(topics, /api\.planSEOContentAction\(projectId, action\.id\)/);
  assert.match(topics, /generate\(topic\)/);
  assert.match(topics, /Review brief/);
  assert.match(topics, /selectedContentPlanActionID/);
  assert.match(topics, /dataAttribute="content-plan-action-drawer"/);
  assert.match(topics, /footerLabel="Content plan drawer actions"/);
  assert.match(topics, /Draft Content/);
  assert.match(topics, /aria-busy=\{selectedActionDraftBusy\}/);
  assert.match(topics, /disabled=\{reviewingContentPlanAction\}/);
  assert.match(topics, /reviewHrefForAction\(projectId, selectedContentPlanAction\)/);
  assert.doesNotMatch(topics, /onClick=\{\(\) => draftAcceptedAction\(action\)\}/);
  assert.doesNotMatch(topics, /Waiting in Content Plan/);
});

test("content plan reviews accepted opportunities in the shared right drawer before drafting", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");
  const drawer = read("components/right-drawer.tsx");

  assert.match(drawer, /export function RightDrawer/);
  assert.match(drawer, /drawerFocusableSelector/);
  assert.match(drawer, /document\.body\.style\.overflow = "hidden"/);
  assert.match(drawer, /motion-safe:animate-\[citeloop-drawer-panel-in_220ms_cubic-bezier\(0\.16,1,0\.3,1\)\]/);
  assert.match(drawer, /footer/);
  assert.match(topics, /import \{ RightDrawer \} from "\.\.\/\.\.\/\.\.\/components\/right-drawer"/);
  assert.match(topics, /selectedContentPlanAction/);
  assert.match(topics, /setSelectedContentPlanActionID\(action\.id\)/);
  assert.match(topics, /AI Visibility \/ GEO Impact/);
  assert.match(topics, /Why write this/);
  assert.match(topics, /Evidence source/);
  assert.match(topics, /dismissAcceptedAction/);
  assert.match(topics, /api\.dismissSEOOpportunity\(projectId, action\.opportunity_id\)/);
});

test("content plan only links to Review after a draft article exists", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");
  const logic = read("lib/content-plan-logic.ts");

  assert.match(logic, /hasReviewableDraft/);
  assert.match(topics, /hasReviewableDraft\(selectedContentPlanAction\)/);
  assert.match(topics, /const actionHasReviewContent = hasReviewableDraft\(action\)/);
  assert.match(topics, /actionHasReviewContent \? \(/);
  assert.match(topics, /View in Review/);
  assert.doesNotMatch(topics, /action\.lifecycle_stage === "ready_for_review" \|\| Boolean\(action\.draft_article_id\)/);
  assert.doesNotMatch(topics, /const selectedActionHasReviewContent = Boolean\(selectedContentPlanAction\?\.draft_article_id\)/);
  assert.match(topics, /\/projects\/\$\{projectId\}\/review\?article=\$\{action\.draft_article_id\}/);
  assert.doesNotMatch(topics, /: `\/projects\/\$\{projectId\}\/review`/);
});

test("content plan backlog excludes drafted topics", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");

  assert.match(topics, /isBacklogStatus/);
  assert.match(topics, /backlogTopics/);
  assert.match(topics, /isBacklogStatus\(topic\.status\)/);
});

test("content plan presents planned topics without legacy backlog search or summary chrome", () => {
  const topics = read("projects/[id]/topics/topics-client.tsx");
  const workflow = read("projects/[id]/content-workflow-client.tsx");

  assert.match(workflow, /title: "Content Plan"/);
  for (const copy of [
    "Planned topics",
    "Draft queue",
    "Plan status",
    "Ready to draft",
    "Scheduled intent",
    "Needs priority",
    "Recommended next",
    "Why this exists",
    "High priority",
    "Medium priority",
    "Low priority",
  ]) {
    assert.match(topics, new RegExp(copy));
  }
  assert.doesNotMatch(topics, /Pick signal/);
  assert.doesNotMatch(topics, /priority \{topic\.priority\}/);
  assert.doesNotMatch(topics, /Search topics/);
  assert.doesNotMatch(topics, /Planned topic channel/);
  assert.doesNotMatch(topics, /All channels/);
  assert.doesNotMatch(topics, /const \[channel, setChannel\]/);
  assert.doesNotMatch(topics, /data-content-plan-summary-section/);
  assert.doesNotMatch(topics, /<SectionHeader title="Backlog"/);
  assert.doesNotMatch(topics, /No backlog topics found/);
  assert.doesNotMatch(topics, /adjust filters/);
  assert.doesNotMatch(topics, /No planned topics in this channel/);

  assert.doesNotMatch(topics, /PlanView/);
  assert.doesNotMatch(topics, /const \[view, setView\]/);
  assert.doesNotMatch(topics, /setView\("list"\)/);
  assert.doesNotMatch(topics, /setView\("grid"\)/);
  assert.doesNotMatch(topics, /setView\("compact"\)/);
  assert.doesNotMatch(topics, /aria-pressed=\{view === "grid"\}/);
  assert.match(topics, /planHealthForTopics\(topics\)/);
  assert.match(topics, /planHealth\.backlog/);
  assert.match(topics, /planStatusItems/);
  assert.doesNotMatch(topics, /data-content-plan-topic-toolbar/);
  assert.doesNotMatch(topics, /Generate from domain/);
  assert.doesNotMatch(topics, /api\.runStrategist/);
  assert.doesNotMatch(topics, /List view/);
  assert.doesNotMatch(topics, /Two-column view/);
  assert.doesNotMatch(topics, /Three-column view/);
  assert.doesNotMatch(topics, /Draft next topic/);
  assert.doesNotMatch(topics, /<RefreshCw/);
  assert.doesNotMatch(topics, /planPulseForTopics/);
  assert.doesNotMatch(topics, /<SectionHeader title="Plan health"/);
  assert.doesNotMatch(topics, /<SectionHeader title="Plan pulse"/);
  assert.doesNotMatch(topics, /<SectionHeader title="Topic summary"/);
  assert.doesNotMatch(topics, /Topics waiting for draft generation\./);
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

test("review cards keep QA and score details inside the drawer", () => {
  const review = read("projects/[id]/review/review-client.tsx");
  const cardBlock = review.slice(review.indexOf("function ReviewDecisionCard"), review.indexOf("function StateBadge"));

  assert.match(cardBlock, /data-review-card/);
  assert.match(cardBlock, /Open details/);
  assert.doesNotMatch(cardBlock, /formatScore/);
  assert.doesNotMatch(cardBlock, /source evidence/);
  assert.doesNotMatch(cardBlock, /Topic \{topicId\.slice/);
  assert.match(review, /Claim evidence map/);
  assert.match(review, /SEO contribution/);
});

test("blocking mutations expose button-level progress and keep opportunity review local", () => {
  const ui = read("components/ui.tsx");
  const visibility = read("projects/[id]/seo/seo-client.tsx");
  const topics = read("projects/[id]/topics/topics-client.tsx");
  const review = read("projects/[id]/review/review-client.tsx");
  const publishing = read("projects/[id]/publishing/publishing-client.tsx");
  const settings = read("projects/[id]/settings/settings-client.tsx");
  const context = read("projects/[id]/knowledge/knowledge-client.tsx");
  const adminPage = read("projects/[id]/admin/page.tsx");
  const admin = read("projects/[id]/admin/admin-client.tsx");
  const workspace = read("projects/[id]/workspace.tsx");

  assert.match(ui, /export function ButtonProgress/);
  assert.match(ui, /Loader2/);
  assert.match(ui, /aria-live=\{busy \? "polite" : undefined\}/);

  assert.match(visibility, /opportunityBusy/);
  assert.match(visibility, /createActionBusy/);
  assert.match(visibility, /dismissBusy/);
  assert.match(visibility, /Adding to plan/);
  assert.match(visibility, /Creating site fix/);
  assert.match(visibility, /Dismissing/);

  const createActionBlock = visibility.slice(visibility.indexOf("async function createAction"), visibility.indexOf("async function dismiss"));
  assert.match(createActionBlock, /api\.createSEOContentAction/);
  assert.match(createActionBlock, /setOpportunities\(\(current\) => current\.map/);
  assert.match(createActionBlock, /status: "converted"/);
  assert.match(createActionBlock, /setActions\(\(current\) => \[action, \.\.\.current\.filter/);
  assert.doesNotMatch(createActionBlock, /await refresh\(\)/);

  const dismissBlock = visibility.slice(visibility.indexOf("async function dismiss"), visibility.indexOf("async function savePolicy"));
  assert.match(dismissBlock, /setOpportunities\(\(current\) => current\.filter/);
  assert.doesNotMatch(dismissBlock, /await refresh\(\)/);

  for (const [source, markers] of [
    [topics, ["Saving topic", "Scheduling", "Archiving"]],
    [review, ["Approving", "Rejecting", "Saving content"]],
    [publishing, ["Checking status", "Retrying", "Marking distributed"]],
    [settings, ["Saving publisher", "Saving token", "Testing", "Retrying", "Saving settings"]],
    [context, ["Refreshing context", "Confirming context", "Saving source page", "Saving advanced context"]],
    [admin, ["Saving", "Testing", "Removing"]],
  ]) {
    assert.match(source, /ButtonProgress/);
    for (const marker of markers) {
      assert.match(source, new RegExp(marker));
    }
  }

  assert.match(adminPage, /<AdminClient projectId=\{id\} \/>/);
  assert.ok(!adminPage.includes("redirect(`/admin?from="));
  assert.match(admin, /api\.testLLMCredentials/);
  assert.match(admin, /api\.deleteLLMCredentials/);
  assert.match(admin, /api\.listRuns\(projectId, \{ limit: 100 \}\)/);
  assert.match(admin, /activePlatformRuntimeIncidents/);
  assert.match(admin, /activityRawError/);
  assert.match(admin, /Test connection/);
  assert.match(admin, /Delete key/);
  assert.match(admin, /TokenGate API key/);
  assert.match(admin, /Default model/);
  assert.match(admin, /Writer model/);
  assert.match(admin, /QA model/);
  assert.match(admin, /type AdminTabId = "runtime" \| "geo" \| "incidents"/);
  assert.match(admin, /GEO providers/);
  assert.match(admin, /Runtime incidents/);
  assert.match(admin, /Platform AI operations/);
  assert.match(admin, /TokenGate key for Perplexity/);
  assert.match(admin, /OpenAI or Anthropic can run GEO workflows/);
  assert.doesNotMatch(admin, /counts for GEO automation activation/);
  assert.match(admin, /TokenGate key for OpenAI/);
  assert.match(admin, /TokenGate key for Anthropic/);
  assert.match(admin, /TokenGate key for Gemini/);
  assert.match(admin, /geoProviders\.map/);
  assert.match(admin, /api\.listGEOCredentials/);
  assert.match(admin, /api\.updateGEOCredentials/);
  assert.match(admin, /api\.testGEOCredentials/);
});

test("GEO provider observation defaults to OpenAI instead of required Perplexity", () => {
  const results = read("projects/[id]/seo/seo-client.tsx");
  const observeBlock = results.slice(results.indexOf("async function observeGEOProvider"), results.indexOf("async function monitorGEOExternalSurfaces"));

  assert.match(observeBlock, /api\.observeGEOProvider\(projectId, \{ engine: "OpenAI", max_prompts: 10 \}\)/);
  assert.doesNotMatch(observeBlock, /engine: "Perplexity"/);
});

test("temporary page feedback uses the global auto-dismissing toast system", () => {
  const layout = read("layout.tsx");
  const toastProvider = read("components/toast-provider.tsx");
  const globals = read("globals.css");
  const feedbackFiles = [
    "admin/page.tsx",
    "projects/project-management-client.tsx",
    "projects/[id]/admin/admin-client.tsx",
    "projects/[id]/workspace.tsx",
    "projects/[id]/topics/topics-client.tsx",
    "projects/[id]/review/review-client.tsx",
    "projects/[id]/knowledge/knowledge-client.tsx",
    "projects/[id]/seo/seo-client.tsx",
    "projects/[id]/publishing/publishing-client.tsx",
    "projects/[id]/settings/settings-client.tsx",
  ];

  assert.match(layout, /import \{ ToastProvider \} from "\.\/components\/toast-provider";/);
  assert.match(layout, /<ToastProvider>\{children\}<\/ToastProvider>/);

  assert.match(toastProvider, /export function ToastProvider/);
  assert.match(toastProvider, /export function useToast/);
  assert.match(toastProvider, /setTimeout/);
  assert.match(toastProvider, /fixed right-4 top-4/);
  assert.match(toastProvider, /role=\{toast\.tone === "red" \? "alert" : "status"\}/);
  assert.match(toastProvider, /toast-progress-edge toast-progress-top/);
  assert.match(toastProvider, /toast-progress-edge toast-progress-right/);
  assert.match(toastProvider, /toast-progress-edge toast-progress-bottom/);
  assert.match(toastProvider, /toast-progress-edge toast-progress-left/);

  for (const keyframe of ["toast-progress-top", "toast-progress-right", "toast-progress-bottom", "toast-progress-left"]) {
    assert.match(globals, new RegExp(`@keyframes ${keyframe}`));
  }

  for (const relativePath of feedbackFiles) {
    const source = read(relativePath);
    assert.match(source, /useToast\(\)/, `${relativePath} should publish temporary feedback through toast context`);
    assert.doesNotMatch(source, /const \[message, setMessage\] = useState<Message>\(null\);/, `${relativePath} should not keep inline message state`);
    assert.doesNotMatch(source, /\{message && <Notice/, `${relativePath} should not render temporary page feedback inline`);
  }
});
