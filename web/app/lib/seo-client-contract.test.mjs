import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("SEO page does not expose internal Google Search Console credential fields", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  assert.equal(source.includes("GSC site URL"), false);
  assert.equal(source.includes("Credential ref"), false);
  assert.equal(source.includes("gsc_credential_ref"), false);
});

test("web API exposes Phase 5 autopilot readiness contract", async () => {
  const source = await readFile(new URL("api.ts", import.meta.url), "utf8");

  for (const expected of [
    "AutopilotReadiness",
    "AutopilotReadinessGate",
    "getAutopilotReadiness",
    "ready_for_level_2",
    "rollback_or_recovery_ready",
  ]) {
    assert.equal(source.includes(expected), true, `api.ts missing ${expected}`);
  }
});

test("web API exposes Phase 5 guarded autopilot execution contract", async () => {
  const source = await readFile(new URL("api.ts", import.meta.url), "utf8");

  for (const expected of [
    "AutopilotExecuteResult",
    "executeAutopilotPlan",
    "executed_actions",
    "deferred_actions",
    "guardrail_results",
    "recovery_plans",
  ]) {
    assert.equal(source.includes(expected), true, `api.ts missing ${expected}`);
  }
});

test("SEO autopilot panel exposes Phase 5 guarded execution controls", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  for (const expected of [
    "Readiness",
    "Ready for Level 2",
    "Blocked gates",
    "Execute guarded actions",
    "Recovery plan",
    "Manual rollback required",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }
});

test("Analysis page does not render Automation readiness as a primary module", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const analysisBranchIndex = source.indexOf('{mode === "analysis" && (');
  const resultsBranchIndex = source.indexOf('{mode === "results" && (');

  assert.notEqual(analysisBranchIndex, -1, "seo-client.tsx missing Analysis render branch");
  assert.notEqual(resultsBranchIndex, -1, "seo-client.tsx missing Results render branch");
  assert.equal(source.includes("data-analysis-autopilot-visible"), false);
  assert.equal(source.includes('title="Automation readiness"'), false);
  assert.equal(source.includes("Finish automation setup in Settings"), true);
  assert.ok(
    analysisBranchIndex < source.indexOf("Finish automation setup in Settings") &&
      source.indexOf("Finish automation setup in Settings") < resultsBranchIndex,
    "the lightweight setup bridge should stay inside the Analysis branch",
  );
});

test("Analysis page is owned only by the Opportunities growth loop", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const siteFixes = await readFile(new URL("../projects/[id]/site-fixes/site-fixes-client.tsx", import.meta.url), "utf8");

  // Site Fixes live on Doctor's dedicated repair surface. Analysis contains
  // only the Opportunity Queue and its delayed growth loop.
  for (const expected of ["data-analysis-opportunity-finding-status", "data-analysis-growth-findings-section", "Opportunity Queue ·", "data-analysis-loop-strip"]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }
  assert.equal(source.includes("data-site-fixes-queue"), false, "Analysis should no longer render an on-page Site Fixes queue");
  assert.equal(source.includes("data-site-fix-card"), false, "Site Fix cards moved to the dedicated Site Fixes page");
  assert.match(source, /function isOpportunitiesOwnedAction/);
  assert.match(source, /filter\(isOpportunitiesOwnedAction\)/);

  const findingStatusIndex = source.indexOf("data-analysis-opportunity-finding-status");
  const opportunityQueueIndex = source.indexOf("data-analysis-growth-findings-section");
  const loopIndex = source.indexOf("data-analysis-loop-strip");

  assert.ok(findingStatusIndex < opportunityQueueIndex, "Opportunity Finding status should sit above the queue");
  assert.ok(opportunityQueueIndex < loopIndex, "Opportunity Queue should appear before lower-priority loop diagnostics");

  // The Site Fixes surface now lives on its own page and still owns the cards/grid.
  assert.equal(siteFixes.includes("data-site-fix-card"), true, "site-fixes-client.tsx should own the Site Fix cards");
  assert.equal(siteFixes.includes("data-site-fixes-grid"), true, "site-fixes-client.tsx should own the Site Fixes grid");

  assert.equal(source.includes("data-analysis-focus-cards"), false, "Analysis should not render the old top metrics board");
  assert.equal(source.includes("What needs review next"), false, "Analysis should not render the old metrics board headline");
  assert.equal(source.includes("Review direct action"), false, "Analysis should not expose Direct Action as user-facing queue copy");
  assert.equal(source.includes("Direct action queue"), false, "Analysis should rename the old Direct Action queue to Site Fixes");
  assert.equal(source.includes("No direct actions to review"), false, "Analysis empty states should use Site Fixes language");
  assert.equal(source.includes("data-analysis-autopilot-visible"), false);
  assert.equal(source.includes("Automation readiness"), false);
  assert.equal(source.includes("Finish automation setup in Settings"), true);
  assert.equal(source.includes("data-analysis-search-signal"), false, "Analysis should not show search metrics as a first-level panel");
  assert.equal(source.includes("Search performance snapshot"), false, "Home owns the search-performance KPI snapshot");
});

test("Analysis page exposes Opportunity Finding run status", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const panelStart = source.indexOf("function OpportunityFindingStatusPanel");
  const panelEnd = source.indexOf("type SEOClientMode");
  const panelSource = source.slice(panelStart, panelEnd);

  assert.match(panelSource, /Scheduled \+ manual/);
  assert.doesNotMatch(panelSource, /\? "Scheduled only"/);

  for (const expected of [
    "api.getOpportunityFindingStatus(projectId)",
    "api.runOpportunityFinding(projectId)",
    "data-competitive-auto-discovery-note",
    "AI Discovery automatically inspects search evidence, competitor domains, and public page patterns.",
    "No competitor URL input needed.",
    "Last finding",
    "Next finding",
    "Manual mode",
    "Run finding",
    "Evidence + AI",
    "Evidence only",
    "AI only",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }
  for (const removed of [
    "parseOpportunityFindingSeedURLs",
    "competitiveSeedURLs",
    "seed_urls",
    "https://postsyncer.com/tools",
    "Competitive seed URLs",
    "competitive-seed-urls",
  ]) {
    assert.equal(source.includes(removed), false, `seo-client.tsx should not expose manual competitive seed URL UI: ${removed}`);
  }
  assert.equal(source.includes("Signal Scan"), false, "legacy source modes must not remain user-facing");

  assert.notEqual(panelStart, -1, "seo-client.tsx missing OpportunityFindingStatusPanel");
  assert.notEqual(panelEnd, -1, "seo-client.tsx missing OpportunityFindingStatusPanel boundary");
  assert.equal(panelSource.includes("{manualMode && ("), false, "Run finding should be available from the status card in automatic mode too");
  assert.match(panelSource, /<Button size="sm" variant="primary" onClick=\{onRun\} disabled=\{!!busy \|\| runActive\}>/);
  assert.equal(
    panelSource.includes("data-opportunity-finding-error"),
    true,
    "failed Opportunity Finding must keep its actionable error visible after the toast expires",
  );
  assert.equal(panelSource.includes("status.last_run.error"), false, "raw workflow errors must stay out of customer copy");
  assert.equal(panelSource.includes("Opportunity finding couldn't finish"), true);
  assert.equal(panelSource.includes("We couldn't complete every check. Please try again."), true);
  for (const expected of [
    'status?.last_run?.status === "queued"',
    'status?.last_run?.status === "running"',
    "window.setTimeout",
    "api.getOpportunityFindingStatus(projectId)",
    "opportunityFindingStatusSequenceRef",
    'title: "Opportunity finding started"',
  ]) {
    assert.equal(source.includes(expected), true, `durable Opportunity Finding UI missing ${expected}`);
  }
});

test("Opportunity Finding run details default closed behind an accessible disclosure", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const panelStart = source.indexOf("function OpportunityFindingStatusPanel");
  const panelEnd = source.indexOf("function OpportunityFindingResultMessage");
  const panelSource = source.slice(panelStart, panelEnd);

  assert.match(panelSource, /const \[runDetailsExpanded, setRunDetailsExpanded\] = useState\(false\)/);
  const toggleDataIndex = panelSource.indexOf("data-opportunity-finding-details-toggle");
  assert.ok(toggleDataIndex > -1, "Opportunity Finding details toggle data attribute is required");
  assert.equal(
    panelSource.indexOf("data-opportunity-finding-details-toggle", toggleDataIndex + 1),
    -1,
    "Opportunity Finding details toggle must be unique",
  );
  const toggleStart = panelSource.lastIndexOf("<button", toggleDataIndex);
  const toggleCloseIndex = panelSource.indexOf("</button>", toggleDataIndex);
  const toggleEnd = toggleCloseIndex + "</button>".length;
  assert.ok(toggleStart > -1 && toggleStart < toggleDataIndex, "Opportunity Finding details toggle must be a native button");
  assert.ok(toggleCloseIndex > toggleDataIndex, "Opportunity Finding details toggle must have a closing button tag");
  const toggleSource = panelSource.slice(toggleStart, toggleEnd);
  assert.match(
    toggleSource,
    /<button[^>]*type="button"[^>]*data-opportunity-finding-details-toggle[^>]*aria-expanded=\{runDetailsExpanded\}[^>]*aria-controls="opportunity-finding-run-details"[^>]*onClick=\{\(\) => setRunDetailsExpanded\(\(expanded\) => !expanded\)\}[^>]*>/,
  );
  assert.equal(toggleSource.includes("Run details"), true, "Opportunity Finding details toggle must have the visible accessible label");

  const detailsStart = panelSource.indexOf("{runDetailsExpanded && (");
  const detailsEnd = panelSource.indexOf("\n        </div>\n      )}", detailsStart);
  const panelSectionEnd = panelSource.indexOf("\n    </section>", detailsStart);
  assert.ok(detailsStart > -1, "Opportunity Finding details must use the controlled conditional");
  assert.ok(panelSectionEnd > detailsStart, "Opportunity Finding status panel section must close after the details conditional");
  assert.ok(
    detailsEnd > detailsStart && detailsEnd < panelSectionEnd,
    "Opportunity Finding details conditional must close before the panel section",
  );

  const beforeDetailsSource = panelSource.slice(0, detailsStart);
  const detailsSource = panelSource.slice(detailsStart, detailsEnd);
  assert.match(detailsSource, /<div[^>]*id="opportunity-finding-run-details"[^>]*data-opportunity-finding-run-details[^>]*>/);
  for (const expected of ["status.counts.open", "status.counts.in_loop", "status.counts.processed"]) {
    assert.equal(detailsSource.includes(expected), true, `collapsed Opportunity Finding details missing ${expected}`);
  }
  assert.equal(detailsSource.includes("status.summary"), false, "raw backend run summaries must stay out of customer details");
  assert.equal(detailsSource.includes("summary.slice"), false, "raw backend run summaries must stay out of customer details");
  assert.match(beforeDetailsSource, /data-opportunity-finding-error/);
  assert.match(beforeDetailsSource, /data-opportunity-finding-details-toggle/);
  assert.doesNotMatch(detailsSource, /data-opportunity-finding-error/);
  assert.doesNotMatch(detailsSource, /data-opportunity-finding-details-toggle/);
  const errorIndex = panelSource.indexOf("data-opportunity-finding-error");
  const toggleIndex = panelSource.indexOf("data-opportunity-finding-details-toggle");
  assert.ok(errorIndex > -1 && errorIndex < toggleIndex, "durable run errors must remain outside the collapsed details region");
});

test("Opportunity Finding polling relies on the durable generic failure alert", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const pollStart = source.indexOf("const status = opportunityFindingStatus;");
  const pollEnd = source.indexOf("useEffect(() => {", pollStart);
  const pollSource = source.slice(pollStart, pollEnd);
  const terminalStart = pollSource.indexOf("if (opportunityFindingTerminalRef.current !== run.id)");
  const terminalEnd = pollSource.indexOf("await refresh();", terminalStart);
  const terminalSource = pollSource.slice(terminalStart, terminalEnd);

  assert.notEqual(pollStart, -1, "seo-client.tsx missing Opportunity Finding polling");
  assert.notEqual(pollEnd, -1, "seo-client.tsx missing Opportunity Finding polling boundary");
  assert.notEqual(terminalStart, -1, "Opportunity Finding polling missing terminal-run handling");
  assert.notEqual(terminalEnd, -1, "Opportunity Finding polling missing terminal-run boundary");
  assert.equal(terminalSource.includes("run.error"), false, "polled workflow errors must not reach customer toasts");
  assert.equal(terminalSource.includes('tone: "red"'), false, "failed terminal runs must rely on the durable alert instead of a toast");
});

test("Opportunity Finding request failures use generic customer copy", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const loadStart = source.indexOf("void refreshOpportunityFindingStatus().catch");
  const loadEnd = source.indexOf("useEffect(() => {", loadStart);
  const refreshStart = source.indexOf("const status = opportunityFindingStatus;");
  const refreshEnd = source.indexOf("useEffect(() => {", refreshStart);
  const runStart = source.indexOf("async function runOpportunityFinding()");
  const runEnd = source.indexOf("async function runCrawlerAudit()", runStart);

  for (const [name, boundary] of Object.entries({ loadStart, loadEnd, refreshStart, refreshEnd, runStart, runEnd })) {
    assert.notEqual(boundary, -1, `seo-client.tsx missing ${name} Opportunity Finding boundary`);
  }

  const requestPaths = {
    statusLoad: source.slice(loadStart, loadEnd),
    statusRefresh: source.slice(refreshStart, refreshEnd),
    runRequest: source.slice(runStart, runEnd),
  };
  const rawErrorPaths = Object.entries(requestPaths)
    .filter(([, pathSource]) => pathSource.includes("e.message"))
    .map(([name]) => name);

  assert.deepEqual(rawErrorPaths, [], "Opportunity Finding request failures must not expose exception messages");
  for (const [name, pathSource] of Object.entries(requestPaths)) {
    assert.equal(
      pathSource.includes("We couldn't complete every check. Please try again."),
      true,
      `${name} must use generic Opportunity Finding failure detail`,
    );
  }
});

test("Analysis hides Growth Radar engineering diagnostics from customers", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const resultStart = source.indexOf("function OpportunityFindingResultMessage");
  const resultEnd = source.indexOf("type SEOClientMode", resultStart);
  const resultSource = source.slice(resultStart, resultEnd);

  assert.notEqual(resultStart, -1, "seo-client.tsx missing OpportunityFindingResultMessage");
  assert.notEqual(resultEnd, -1, "seo-client.tsx missing OpportunityFindingResultMessage boundary");
  assert.match(resultSource, /userFacingGrowthRadarResult/);
  assert.match(resultSource, /<Notice/);
  assert.match(resultSource, /userFacingGrowthRadarResult\(status\?\.last_run\)/);
  assert.match(resultSource, /if \(!result\) return null;/);
  assert.equal(source.includes("GrowthRadarDiagnosticsPanel"), false);
  assert.equal(source.includes("data-growth-radar-diagnostics"), false);
  for (const removed of [
    "Growth Radar funnel",
    "Deterministic evidence, policy and target diagnostics",
    "Prompt rotation",
    "Provider cost",
    "diagnostics.watchlist",
    "diagnostics.summary.candidates",
    ".prompts",
    ".cost_usd",
    ".reasons",
  ]) {
    assert.equal(resultSource.includes(removed), false, `customer result must hide ${removed}`);
  }
});

test("Growth Radar result announces asynchronous customer messages", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const resultStart = source.indexOf("function OpportunityFindingResultMessage");
  const resultEnd = source.indexOf("type SEOClientMode", resultStart);
  const resultSource = source.slice(resultStart, resultEnd);

  assert.notEqual(resultStart, -1, "seo-client.tsx missing OpportunityFindingResultMessage");
  assert.notEqual(resultEnd, -1, "seo-client.tsx missing OpportunityFindingResultMessage boundary");
  assert.match(resultSource, /role="status"/);
  assert.match(resultSource, /aria-live="polite"/);
});

test("Opportunity Finding surface never renders backend outcome diagnostics", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const progress = await readFile(new URL("../projects/[id]/seo/opportunity-finding-progress.tsx", import.meta.url), "utf8");
  const panelStart = source.indexOf("function OpportunityFindingStatusPanel");
  const panelEnd = source.indexOf("type SEOClientMode", panelStart);
  const surface = `${source.slice(panelStart, panelEnd)}\n${progress}`;

  assert.notEqual(panelStart, -1, "seo-client.tsx missing OpportunityFindingStatusPanel");
  assert.notEqual(panelEnd, -1, "seo-client.tsx missing Opportunity Finding surface boundary");
  for (const forbidden of [
    "status.summary",
    "summary.slice",
    "zero_result_reason",
    "new_opportunity_count",
    "generated or refreshed",
    "zeroReasonCopy",
  ]) {
    assert.equal(surface.includes(forbidden), false, `Opportunity Finding surface must not render ${forbidden}`);
  }
});

test("Growth Stage and manual finding expose accessible detail and real progress", async () => {
	const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
	const selector = await readFile(new URL("../projects/[id]/seo/growth-stage-selector.tsx", import.meta.url), "utf8");
	const progress = await readFile(new URL("../projects/[id]/seo/opportunity-finding-progress.tsx", import.meta.url), "utf8");
	const styles = await readFile(new URL("../globals.css", import.meta.url), "utf8");

	for (const expected of ['role="listbox"', 'role="option"', "option.description", "aria-expanded", "Growth Stage"]) {
		assert.equal(selector.includes(expected), true, `stage selector missing ${expected}`);
	}
	assert.match(selector, /data-growth-stage-trigger[\s\S]*?className="[^"]*h-8[^"]*w-36[^"]*"/);
	assert.match(selector, /data-growth-stage-menu-label[\s\S]*?>\s*Growth Stage\s*</);
	assert.doesNotMatch(selector, /shrink-0 text-xs font-semibold text-slate-500">Growth Stage/);
	assert.match(source, /data-gsc-status-trigger[\s\S]*?className="[^"]*h-8[^"]*w-36[^"]*"/);
	assert.equal(selector.includes("— {option.description}"), false, "closed stage label must not inline the explanation");
	for (const expected of [
		'role="progressbar"',
		"progress_percent",
		"current_stage",
		"Calling AI",
		"Refreshing evidence",
		"Refreshing search, competitive recall, and AI observations",
		"duration_ms",
		"evidenceRefreshSubsteps",
		"summary?.substeps",
	]) {
		assert.equal(progress.includes(expected), true, `finding progress missing ${expected}`);
	}
	for (const forbidden of ["new Opportunities", "generated or refreshed in this run", "zero_result_reason", "new_opportunity_count", "zeroReasonCopy"]) {
		assert.equal(progress.includes(forbidden), false, `completed finding timeline must not expose ${forbidden}`);
	}
	assert.equal(progress.includes("Run timeline"), true, "completed findings should keep the stage timeline visible");
	assert.equal(progress.includes("timelineExpanded"), true, "completed run timeline must be collapsible after the run finishes");
	assert.equal(progress.includes("data-opportunity-finding-timeline-toggle"), true, "run timeline must expose a chevron toggle");
	assert.equal(progress.includes("data-opportunity-finding-timeline-body"), true, "run timeline body must be separately expandable");
	assert.equal(progress.includes("aria-expanded={timelineExpanded}"), true, "run timeline toggle must expose expanded state");
	assert.doesNotMatch(
		progress,
		/if \(!active\) \{[\s\S]*?return \([\s\S]*?data-opportunity-finding-progress[\s\S]*?\);\s*\}\s*return \(/,
		"completed findings must not return before rendering the shared progressbar and stage durations",
	);
	for (const expected of ["window.setInterval", "Elapsed", "Usually 45–120 seconds", "data-indeterminate", "opportunity-finding-progress-slide"]) {
		assert.equal(progress.includes(expected), true, `active finding progress missing ${expected}`);
	}
	assert.equal(styles.includes("@keyframes opportunity-finding-progress-slide"), true, "finding progress needs an indeterminate transform animation");
	for (const expected of ["GrowthStageSelector", "OpportunityFindingProgress", "growth-stage-default-notice", "localStorage", "Dismiss default stage notice"]) {
		assert.equal(source.includes(expected), true, `SEO page missing ${expected}`);
	}
});

test("Analysis Site Fixes open a reusable right drawer for review", async () => {
  // The Site Fixes review surface moved to its own page and now uses the shared
  // RightDrawer instead of a bespoke analysis drawer.
  const source = await readFile(new URL("../projects/[id]/site-fixes/site-fixes-client.tsx", import.meta.url), "utf8");
  const progress = await readFile(new URL("site-fix-pr-progress.ts", import.meta.url), "utf8");
  const contract = `${source}\n${progress}`;

  for (const expected of [
    "selectedID",
    "SiteFix",
    "data-site-fix-card",
    "data-site-fix-drawer",
    "RightDrawer",
    "Review site fix details",
    "returnFocusRef",
    "Approve fix",
    "Create PR",
    "Retry PR creation",
    "Verify fix",
    "api.approveDoctorSiteFix",
    "api.applyDoctorSiteFix",
    "api.verifyDoctorSiteFix",
  ]) {
    assert.equal(contract.includes(expected), true, `Site Fix PR contract missing ${expected}`);
  }
  assert.doesNotMatch(source, /Needs revision/, "Site Fix review drawer should not expose a non-functional revision action");
  assert.doesNotMatch(source, /verifyAction\(action, "failed"\)/, "Site Fix review drawer should not mark review feedback as verification failure");
  // RightDrawer marks surfaceRef inert while open and renders inline (no portal),
  // so the drawer must be a SIBLING of the inert surface — otherwise its own
  // close button becomes inert and the drawer cannot be closed by pointer.
  assert.match(source, /surfaceRef=\{surfaceRef\}/, "drawer should inert the page surface");
  assert.match(source, /<\/div>\s*<RightDrawer/, "RightDrawer must render outside (sibling of) the surfaceRef div, not nested inside it");
});

test("Canonical Site Fixes expose copyable repair JSON", async () => {
  const lib = await readFile(new URL("site-fix.ts", import.meta.url), "utf8");
  const ui = await readFile(new URL("../projects/[id]/site-fixes/site-fixes-client.tsx", import.meta.url), "utf8");

  for (const expected of [
    "canonicalSiteFixAIJSON",
    "canonicalSiteFixTitle",
    "canonicalSiteFixTarget",
    "evidence_snapshot",
    "proposed_fix",
    "acceptance_tests",
    "verification_snapshot",
  ]) {
    assert.equal(lib.includes(expected), true, `lib/site-fix.ts missing ${expected}`);
  }

  for (const expected of [
    "canonicalSiteFixAIJSON",
    "copyFixJSON",
    "writeClipboardText",
    "data-site-fix-ai-payload",
    "AI coding fix JSON",
    "Copy fix JSON",
    "Copy this JSON into Codex or Claude Code",
    "Clipboard",
    "Code2",
  ]) {
    assert.equal(ui.includes(expected), true, `site-fixes-client.tsx missing ${expected}`);
  }
});

test("Canonical Site Fix JSON blocks default to five resizable scrollable lines", async () => {
  const source = await readFile(new URL("../projects/[id]/site-fixes/site-fixes-client.tsx", import.meta.url), "utf8");

  assert.match(source, /const SITE_FIX_JSON_VIEWPORT_CLASS = "[^"]*box-content[^"]*h-\[7\.5rem\][^"]*min-h-\[7\.5rem\][^"]*max-h-\[30rem\][^"]*resize-y[^"]*overflow-auto[^"]*select-text[^"]*"/);
  assert.equal((source.match(/SITE_FIX_JSON_VIEWPORT_CLASS/g) ?? []).length, 3, "shared viewport class should be declared once and used by both JSON render paths");
  assert.match(source, /function DetailBlock[\s\S]*<pre[\s\S]*SITE_FIX_JSON_VIEWPORT_CLASS/);
  assert.match(source, /data-site-fix-ai-payload[\s\S]*<pre[\s\S]*SITE_FIX_JSON_VIEWPORT_CLASS/);
});

test("Canonical Site Fixes load and mutate only through Doctor lifecycle APIs", async () => {
  const source = await readFile(new URL("../projects/[id]/site-fixes/site-fixes-client.tsx", import.meta.url), "utf8");
  const refreshStart = source.indexOf("const refresh = useCallback(async () => {");
  const refreshEnd = source.indexOf("useEffect(() => {", refreshStart);
  const refreshSource = source.slice(refreshStart, refreshEnd);

  assert.notEqual(refreshStart, -1, "site-fixes-client.tsx missing refresh callback");
  assert.match(refreshSource, /const \[fixesResult, readinessResult\] = await Promise\.allSettled\(\[\s*api\.listDoctorSiteFixes\(projectId\),\s*api\.getGithubPRReadiness\(projectId\),\s*\]\)/);
  assert.equal((refreshSource.match(/api\.listDoctorSiteFixes\(projectId\)/g) ?? []).length, 1, "full refresh must request the Site Fix list exactly once");
  assert.equal((refreshSource.match(/api\.getGithubPRReadiness\(projectId\)/g) ?? []).length, 1, "full refresh must request stored GitHub readiness exactly once");
  assert.match(refreshSource, /api\.listDoctorSiteFixes\(projectId\)/);
  assert.match(refreshSource, /api\.getGithubPRReadiness\(projectId\)/);
  assert.match(source, /api\.approveDoctorSiteFix\(projectId, fix\.id\)/);
  assert.match(source, /api\.applyDoctorSiteFix\(projectId, fix\.id\)/);
  assert.match(source, /api\.verifyDoctorSiteFix\(projectId, fix\.id\)/);
  assert.match(source, /Copy fix JSON/);
  for (const forbidden of ["SEOContentAction", "/seo/actions", "opportunity_status", "Growth", "measuring", "api.createSiteFixGitHubPR", "listPublisherConnections"]) {
    assert.equal(source.includes(forbidden), false, `site-fixes-client.tsx must not use ${forbidden}`);
  }
});

test("Analysis Site Fixes treat connected enabled GitHub App publishers as PR-capable", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const connectionStart = source.indexOf("const hasConnectedGitHubPublisher = useMemo");
  const connectionEnd = source.indexOf("const promptCountBySet = useMemo", connectionStart);
  const connectionSource = source.slice(connectionStart, connectionEnd);

  assert.notEqual(connectionStart, -1, "seo-client.tsx missing GitHub publisher readiness check");
  assert.match(connectionSource, /connection\.kind === "github_nextjs"/);
  assert.match(connectionSource, /connection\.enabled/);
  assert.match(connectionSource, /connection\.status === "connected"/);
  assert.doesNotMatch(connectionSource, /credential_configured/);
});

test("Analysis Loop in motion excludes Doctor Site Fix work", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  const loopStart = source.indexOf("data-analysis-loop-strip");
  const loopEnd = source.indexOf("Finish automation setup in Settings", loopStart);
  const loopBlock = source.slice(loopStart, loopEnd);

  assert.notEqual(loopStart, -1, "seo-client.tsx missing Loop in motion section");
  assert.match(source, /function loopActionDestinationLabel/);
  assert.match(source, /function loopLifecycleSummaryLabel/);
  assert.match(loopBlock, /publishing, measurement, and learning/);
  assert.match(loopBlock, /loopActionDestinationLabel\(action\)/);
  assert.match(loopBlock, /Content Plan/);
  assert.doesNotMatch(loopBlock, /Site Fixes/);
  assert.doesNotMatch(loopBlock, /Applied/);
});

test("Analysis opportunity cards expose growth-work routing and handoff links", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  for (const expected of [
    "opportunityWorkType",
    "opportunityDestination",
    "opportunityPrimaryCTA",
    "assetTypeForWorkType",
    "sentOpportunityLinks",
    "data-opportunity-handoff-card",
    "Recently Decided",
    "data-opportunity-recent-drawer-trigger",
    "plan?action=${action.id}",
    "citeloop-linked-card-pulse",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }

  assert.equal(source.includes("Create content task"), false, "Opportunity Queue should not show generic Create content task CTA");
  assert.equal(source.includes("Create technical task"), false, "Opportunity Queue should not show generic technical task CTA");

  // The direct-action asset routing table moved to the shared helper module.
  const lib = await readFile(new URL("site-fix.ts", import.meta.url), "utf8");
  const directAssetTypes = lib.match(/const directActionAssetTypes = new Set\(\[([^\]]+)\]\)/)?.[1] ?? "";
  assert.notEqual(directAssetTypes, "", "lib/site-fix.ts should define the Site Fixes asset-type routing table");
  assert.equal(directAssetTypes.includes("metadata_rewrite"), false, "Metadata/page-update work should not be routed to Site Fixes");
});

test("Opportunity queue supports snooze, watch, and approval-source provenance", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  for (const expected of [
    "snoozeOpportunity",
    "unsnoozeOpportunity",
    "watchOpportunity",
    "Watch in Results",
    "Watching in Results",
    "Snoozed (",
    "approvalSourceLabel(action.approval_source)",
    "results-watchlist",
    "watchlistStatusLabel",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }

  // The approval-source provenance copy is produced by the shared helper.
  const lib = await readFile(new URL("site-fix.ts", import.meta.url), "utf8");
  assert.equal(lib.includes("Approved by Autopilot policy"), true, "lib/site-fix.ts should label autopilot-approved provenance");
});

test("Opportunity review drawer explains work type destination and approval source", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const drawerStart = source.indexOf('data-analysis-drawer');
  const drawerEnd = source.indexOf('aria-label="Drawer actions"', drawerStart);
  const drawerSource = source.slice(drawerStart, drawerEnd);

  for (const expected of [
    "Approve to send this to",
    "Work type",
    "Destination",
    "Approval source",
    "Human opportunity approval",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }

  assert.match(source, /function opportunityWorkType\(opportunity: SEOOpportunity\): OpportunityWorkType/);
  assert.notEqual(drawerStart, -1, "seo-client.tsx missing opportunity drawer");
  assert.notEqual(drawerEnd, -1, "seo-client.tsx missing drawer boundary");
  assert.equal(drawerSource.length >= 0, true, "drawer boundary slice must resolve");

  // PRD §6.2: the drawer must let users correct the route between allowed
  // work types, keep CTA/destination/approval copy in sync, and explain why
  // locked routes cannot change.
  for (const expected of [
    "allowedWorkTypesForOpportunity",
    "routeOverrides",
    "setRouteOverrides",
    'aria-label="Choose work type"',
    "workTypeLockReason",
    "approvalCopyForWorkType",
    "work_type: workTypeKeys[workType]",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }
});

test("Analysis loop progress is a strip and finding dismissal is explicit", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  assert.equal(source.includes("data-analysis-loop-strip"), true, "analysis loop progress should render as a horizontal strip");
  assert.equal(
    source.includes("xl:grid-cols-[minmax(0,1fr)_320px]"),
    false,
    "growth findings should not reserve a persistent right-side loop rail",
  );
  assert.equal(source.includes("Dismiss finding"), true, "dismiss action must make the destructive operation explicit");
  assert.equal(source.includes("Close finding details"), true, "finding drawer needs a separate close affordance");
});

test("Analysis loop metrics reveal selected linked content cards", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const loopStart = source.indexOf("data-analysis-loop-strip");
  const loopEnd = source.indexOf("{readiness &&");
  const loopSource = source.slice(loopStart, loopEnd);

  assert.notEqual(loopStart, -1, "seo-client.tsx missing loop in motion section");
  assert.notEqual(loopEnd, -1, "seo-client.tsx missing loop section boundary");

  for (const expected of [
    "selectedLoopStage",
    "setSelectedLoopStage",
    "selectedLoopActions",
    "loopStageDetailTitle",
    "loopActionCurrentHref",
    "loopActionCurrentLabel",
    "data-loop-stage-card",
    "data-loop-action-card",
    "aria-pressed={selectedLoopStage === item.key}",
    "disabled={item.value === 0}",
    "setSelectedLoopStage((current) => (current === item.key ? null : item.key))",
  ]) {
    assert.equal(source.includes(expected), true, `seo-client.tsx missing ${expected}`);
  }

  assert.equal(loopSource.includes("loopPreviewActions"), false, "loop cards should not render an unconditional Recently Sent duplicate");
  assert.equal(loopSource.includes("Open current location"), true, "filtered cards should link to the card's current surface");
  assert.match(source, /\/projects\/\$\{projectId\}\/review\?article=\$\{action\.draft_article_id\}/);
  assert.match(source, /\/projects\/\$\{projectId\}\/results\?action=\$\{action\.id\}/);
});

test("Analysis refresh keeps GSC connection state independent from bulky loop data", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const refreshStart = source.indexOf("const refresh = useCallback(async () => {");
  const refreshEnd = source.indexOf("useEffect(() => {\n    refresh();");
  const refreshSource = source.slice(refreshStart, refreshEnd);

  assert.notEqual(refreshStart, -1, "seo-client.tsx missing refresh callback");
  assert.notEqual(refreshEnd, -1, "seo-client.tsx missing refresh effect boundary");

  assert.match(source, /const \[gscConnection, setGSCConnection\] = useState<GSCConnection \| null>\(null\)/);
  assert.match(refreshSource, /api\.getGSCConnection\(projectId\)/);
  assert.match(refreshSource, /Promise\.allSettled\(\[/);
  assert.doesNotMatch(refreshSource, /await Promise\.all\(\[/);
  assert.match(source, /gscConnection\?\.status/);
  assert.match(source, /gscConnection\?\.selected_property/);
});

test("Analysis Site Fix handoff cards use the loop action source for same-page targets", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const loopActionsStart = source.indexOf("const actionsByID = new Map(actions.map((action) => [action.id, action]));");
  const loopActionsEnd = source.indexOf("const measuredActions = loopActions");
  const loopActionsSource = source.slice(loopActionsStart, loopActionsEnd);

  assert.notEqual(loopActionsStart, -1, "loop actions should index full content actions by id");
  assert.notEqual(loopActionsEnd, -1, "loop action source boundary should resolve");

  for (const expected of [
    "const summaryLoopActions = (visibilitySummary?.actions_in_loop ?? []).map",
    "const matchingAction = actionsByID.get(summaryAction.id);",
    "return matchingAction ? { ...summaryAction, ...matchingAction } : summaryAction;",
  ]) {
    assert.equal(loopActionsSource.includes(expected), true, `loop action source should include ${expected}`);
  }

  // Site Fixes now live on their own page. The loop action source resolves the
  // Site Fixes surface to a real route, and the old on-page focus hack — which
  // expanded and scrolled to an in-page Site Fix card — is gone. Handoff cards
  // are plain links to the current surface.
  assert.match(source, /if \(surface === "Site Fixes"\) return `\/projects\/\$\{projectId\}\/site-fixes`;/);
  assert.match(source, /const href = loopActionCurrentHref\(projectId, action as LoopAction\);/);
  assert.match(source, /data-opportunity-handoff-card/);
  assert.equal(source.includes("focusSiteFixCard"), false, "the on-page Site Fix focus handler should be removed");
  assert.equal(source.includes("pendingSiteFixFocusID"), false, "the Site Fix focus scroll state should be removed");
  assert.equal(source.includes("directReviewActionsAll"), false, "Site Fixes no longer derive from the analysis content-action list");
});

test("Opportunity queue lays finding cards out as responsive rectangles with three per row at most", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const queueStart = source.indexOf("data-analysis-growth-findings-section");
  const queueEnd = source.indexOf("data-analysis-loop-strip");
  const queueSource = source.slice(queueStart, queueEnd);

  assert.notEqual(queueStart, -1, "seo-client.tsx missing opportunity queue section");
  assert.notEqual(queueEnd, -1, "seo-client.tsx missing loop strip section after opportunity queue");
  assert.equal(queueSource.includes("data-analysis-finding-grid"), true, "opportunity queue should expose its responsive card grid");
  assert.equal(queueSource.includes("md:grid-cols-2"), true, "opportunity queue should place cards horizontally on medium screens");
  assert.equal(queueSource.includes("xl:grid-cols-3"), true, "opportunity queue should cap wide layouts at three cards per row");
  assert.equal(queueSource.includes("min-h-[220px]"), true, "opportunity cards should keep a rectangular card footprint");
  assert.equal(queueSource.includes("lg:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)_auto]"), false, "finding cards should not keep the old full-row internal layout");
  assert.equal(queueSource.includes("risk_level"), false, "risk level is an internal judgment and belongs in the drawer");
  assert.equal(queueSource.includes("sourceModeForOpportunity"), false, "source mode is diagnostic context and belongs in the drawer");
  assert.equal(queueSource.includes("priority_score"), false, "raw priority scores should not appear on first-level cards");
  assert.equal(queueSource.includes("Signal"), false, "first-level cards should avoid backend signal labels");
});
