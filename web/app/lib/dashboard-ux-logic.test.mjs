import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadDashboardUXLogicModule() {
  const source = await readFile(new URL("./dashboard-ux-logic.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`;
  return import(moduleUrl);
}

test("nextWorkspaceAction prioritizes context before content generation", async () => {
  const { nextWorkspaceAction } = await loadDashboardUXLogicModule();

  const action = nextWorkspaceAction({
    projectId: "project_1",
    hasProfile: false,
    failedPublishCount: 0,
    hasBlockedDrafts: false,
    reviewCount: 0,
    readyCount: 0,
    topicsCount: 0,
  });

  assert.equal(action.title, "Refresh context");
  assert.equal(action.href, "/projects/project_1/context");
  assert.match(action.detail, /before generating/i);
});

test("nextWorkspaceAction requires confirmation before opportunity discovery", async () => {
  const { nextWorkspaceAction } = await loadDashboardUXLogicModule();

  const input = {
    projectId: "project_1",
    hasProfile: true,
    contextConfirmed: false,
    failedPublishCount: 0,
    hasBlockedDrafts: false,
    reviewCount: 0,
    readyCount: 0,
    topicsCount: 0,
  };

  assert.equal(nextWorkspaceAction(input).title, "Confirm context");
  assert.equal(nextWorkspaceAction(input).href, "/projects/project_1/context");
});

test("buildActionableMomentum hides zero values and returns the next useful empty action", async () => {
  const { buildActionableMomentum } = await loadDashboardUXLogicModule();

  const momentum = buildActionableMomentum({
    projectId: "project_1",
    hasProfile: true,
    publishedThisMonthCount: 0,
    approvedDraftCount: 0,
    opportunitiesConvertedCount: 0,
    readyToDistributeCount: 0,
    activeLoopItemCount: 0,
  });

  assert.deepEqual(momentum.items, []);
  assert.equal(momentum.emptyAction.title, "Context is ready");
  assert.equal(momentum.emptyAction.actionLabel, "Review opportunities");
  assert.equal(momentum.emptyAction.href, "/projects/project_1/analysis");
});

test("buildActionableMomentum turns non-zero metrics into actions", async () => {
  const { buildActionableMomentum } = await loadDashboardUXLogicModule();

  const momentum = buildActionableMomentum({
    projectId: "project_1",
    hasProfile: true,
    publishedThisMonthCount: 3,
    approvedDraftCount: 0,
    opportunitiesConvertedCount: 2,
    readyToDistributeCount: 1,
    activeLoopItemCount: 4,
  });

  assert.deepEqual(
    momentum.items.map((item) => item.label),
    ["Ready to publish", "Published this month", "Opportunities converted", "Active loop items"],
  );
  assert.deepEqual(
    momentum.items.map((item) => item.href),
    [
      "/projects/project_1/publish",
      "/projects/project_1/results",
      "/projects/project_1/analysis",
      "/projects/project_1",
    ],
  );
  assert.deepEqual(
    momentum.items.map((item) => item.actionLabel),
    ["Publish", "View impact", "Review loop", "Timeline"],
  );
  assert.ok(momentum.items.every((item) => item.value > 0));
  assert.ok(momentum.items.every((item) => item.actionLabel.length <= 11));
  assert.equal(momentum.emptyAction, null);
});

test("home AI citation metric sends opportunity counts to Analysis", async () => {
  const { homeAICitationMetric } = await loadDashboardUXLogicModule();

  const metric = homeAICitationMetric({
    projectId: "project_1",
    citationGapCount: 1,
  });

  assert.equal(metric.label, "AI citation gaps");
  assert.equal(metric.value, 1);
  assert.equal(metric.detail, "1 finding ready in Opportunities");
  assert.equal(metric.metricChangeLabel, "Review opportunities");
  assert.equal(metric.href, "/projects/project_1/analysis");
});

test("home in-motion metric lands opportunity actions on Analysis instead of Content Plan", async () => {
  const { homeInMotionMetric } = await loadDashboardUXLogicModule();

  const metric = homeInMotionMetric({
    projectId: "project_1",
    analysisActionCount: 4,
    reviewDraftCount: 0,
    readyToPublishCount: 0,
    measuringActionCount: 0,
  });

  assert.equal(metric.value, 4);
  assert.equal(metric.detail, "4 opportunity actions already in execution");
  assert.equal(metric.metricChangeLabel, "View opportunities");
  assert.equal(metric.href, "/projects/project_1/analysis");
});

test("home pipeline stage counts match each destination page queue", async () => {
  const { homePipelineStageCounts } = await loadDashboardUXLogicModule();

  const counts = homePipelineStageCounts({
    topics: [{ id: "topic-1" }, { id: "topic-2" }],
    reviewGroups: [],
    approvedArticles: [
      { id: "variant-1", kind: "syndication_variant" },
      { id: "variant-2", kind: "syndication_variant" },
      { id: "variant-3", kind: "syndication_variant" },
    ],
    readyDistribute: [
      { article: { id: "variant-1", kind: "syndication_variant" } },
      { article: { id: "variant-2", kind: "syndication_variant" } },
      { article: { id: "variant-3", kind: "syndication_variant" } },
    ],
    failedPublishArticles: [],
    verifyingArticles: [],
    visibilityLifecycleCounts: {
      added_to_plan: 3,
      planned: 0,
      published_or_applied: 0,
      measuring: 0,
      learned: 0,
    },
  });

  assert.deepEqual(
    {
      contentPlan: counts.contentPlan,
      review: counts.review,
      publish: counts.publish,
    },
    {
      contentPlan: 2,
      review: 0,
      publish: 3,
    },
  );
  assert.equal(counts.planGenerationPending, false);
});

test("buildHomeEventStream orders live work, recent events, then the next scheduled event", async () => {
  const { buildHomeEventStream } = await loadDashboardUXLogicModule();

  const stream = buildHomeEventStream({
    projectId: "project_1",
    liveActivities: [
      {
        id: "crawl",
        title: "Reading your site",
        detail: "Refreshing domain context now",
        href: "/projects/project_1/context",
      },
    ],
    recentEvents: [
      {
        id: "published",
        title: "Published homepage comparison",
        detail: "Delivered 2 hours ago",
        href: "/projects/project_1/results",
      },
    ],
    nextEvent: {
      title: "Next publish slot",
      detail: "Tomorrow at 09:00",
      href: "/projects/project_1/publish",
    },
  });

  assert.deepEqual(
    stream.items.map((item) => item.kind),
    ["live", "recent", "next"],
  );
  assert.equal(stream.items[0].title, "Reading your site");
  assert.equal(stream.items[0].timeLabel, "Now");
  assert.equal(stream.emptyAction, null);
});

test("visibleHomeSectionIds keeps the control center compact in steady state", async () => {
  const { visibleHomeSectionIds } = await loadDashboardUXLogicModule();

  const budget = visibleHomeSectionIds(
    [
      { id: "this-week", count: 2, priority: 40 },
      { id: "needs-review", count: 3, priority: 90 },
      { id: "ready", count: 1, priority: 80 },
      { id: "activity-warnings", count: 0, priority: 100 },
      { id: "waiting-canonical", count: 4, priority: 30 },
    ],
    { limit: 2 },
  );

  assert.deepEqual(budget.visibleIds, ["needs-review", "ready"]);
  assert.deepEqual(budget.overflowIds, ["this-week", "waiting-canonical"]);
});

test("nextWorkspaceAction still exposes urgent work for in-page surfaces", async () => {
  const { nextWorkspaceAction } = await loadDashboardUXLogicModule();

  const urgent = nextWorkspaceAction({
    projectId: "project_1",
    hasProfile: true,
    failedPublishCount: 2,
    hasBlockedDrafts: false,
    reviewCount: 0,
    readyCount: 0,
    topicsCount: 5,
  });

  assert.equal(urgent.title, "Fix publishing");
  assert.equal(urgent.href, "/projects/project_1/publish");

  const healthy = nextWorkspaceAction({
    projectId: "project_1",
    hasProfile: true,
    failedPublishCount: 0,
    hasBlockedDrafts: false,
    reviewCount: 0,
    readyCount: 0,
    topicsCount: 5,
  });

  assert.equal(healthy.title, "Refresh context");
  assert.equal(healthy.href, "/projects/project_1/context");
});

test("dashboard UX logic no longer exports sidebarPrimaryAction", async () => {
  const module = await loadDashboardUXLogicModule();

  assert.equal("sidebarPrimaryAction" in module, false);
});

test("nextWorkspaceAction sends confirmed projects to opportunity review before planning", async () => {
  const { nextWorkspaceAction } = await loadDashboardUXLogicModule();

  const action = nextWorkspaceAction({
    projectId: "project_1",
    hasProfile: true,
    contextConfirmed: true,
    failedPublishCount: 0,
    hasBlockedDrafts: false,
    reviewCount: 0,
    readyCount: 0,
    topicsCount: 0,
    openOpportunityCount: 2,
  });

  assert.equal(action.title, "Review opportunities");
  assert.equal(action.href, "/projects/project_1/analysis");
  assert.match(action.detail, /ready to review/i);
});

test("nextWorkspaceAction follows opportunity review work before creating a first plan", async () => {
  const { nextWorkspaceAction } = await loadDashboardUXLogicModule();

  const action = nextWorkspaceAction({
    projectId: "project_1",
    hasProfile: true,
    contextConfirmed: true,
    failedPublishCount: 0,
    hasBlockedDrafts: false,
    reviewCount: 0,
    readyCount: 0,
    topicsCount: 0,
    openOpportunityCount: 3,
    currentPathname: "/projects/project_1/analysis",
  });

  assert.equal(action.title, "Review opportunities");
  assert.equal(action.href, "/projects/project_1/analysis");
  assert.match(action.detail, /ready to review/i);
});

test("profilePayloadFromDraft saves structured fields even when advanced JSON is invalid", async () => {
  const { profilePayloadFromDraft } = await loadDashboardUXLogicModule();

  const payload = profilePayloadFromDraft(
    {
      positioning: "Evidence-backed content engine",
      icp: "Growth teams\nFounders",
      value_props: "Turns visibility gaps into content",
      features: "Context extraction",
      differentiators: "Domain-bound evidence",
      competitors: "SuperX",
      key_terms: "GEO\nSEO",
      tone: "Precise",
      banned_claims: "Guaranteed #1 ranking",
      content_rules: "Cite sources",
      advancedJSON: "{not valid json",
    },
    {
      context_confirmed_at: "2026-06-09T00:00:00Z",
      custom_field: "preserve me",
      voice: { legacy: true },
    },
  );

  assert.equal(payload.positioning, "Evidence-backed content engine");
  assert.deepEqual(payload.icp, ["Growth teams", "Founders"]);
  assert.deepEqual(payload.banned_claims, ["Guaranteed #1 ranking"]);
  assert.equal(payload.custom_field, "preserve me");
  assert.equal(payload.context_confirmed_at, "2026-06-09T00:00:00Z");
  assert.deepEqual(payload.voice, { legacy: true, tone: "Precise", rules: ["Cite sources"] });
});

test("profilePayloadFromAdvancedJSON keeps advanced editing explicit", async () => {
  const { profilePayloadFromAdvancedJSON } = await loadDashboardUXLogicModule();

  assert.deepEqual(profilePayloadFromAdvancedJSON('{"custom":true}'), { custom: true });
  assert.throws(() => profilePayloadFromAdvancedJSON("{not valid json"), /Unexpected token|Expected property name/);
});

test("visibilityLifecycleLabel matches real opportunity and content action enums", async () => {
  const { visibilityLifecycleLabel, visibilityLifecycleTone } = await loadDashboardUXLogicModule();

  assert.equal(visibilityLifecycleLabel("open"), "Opportunity detected");
  assert.equal(visibilityLifecycleLabel("accepted"), "Added to Content Plan");
  assert.equal(visibilityLifecycleLabel("converted"), "Added to Content Plan");
  assert.equal(visibilityLifecycleLabel("drafting"), "Draft in progress");
  assert.equal(visibilityLifecycleLabel("ready_for_review"), "Draft waiting for review");
  assert.equal(visibilityLifecycleLabel("approved"), "Approved for publish");
  assert.equal(visibilityLifecycleLabel("measuring"), "Measuring impact");
  assert.equal(visibilityLifecycleLabel("completed"), "Loop closed");
  assert.equal(visibilityLifecycleLabel("done"), "Loop closed");
  assert.equal(visibilityLifecycleLabel("stale"), "Needs re-check");
  assert.equal(visibilityLifecycleTone("failed"), "red");
  assert.equal(visibilityLifecycleTone("completed"), "green");
});

test("contextBuildTracks reports parallel onboarding work from observed outputs", async () => {
  const { contextBuildTracks } = await loadDashboardUXLogicModule();

  const starting = contextBuildTracks({
    hasProfile: false,
    sourcePageCount: 0,
    evidencePageCount: 0,
    evidenceCount: 0,
    pollCount: 2,
    pollLimit: 18,
    runs: [],
  });
  assert.equal(starting.active, true);
  assert.equal(starting.title, "Building domain context");
  assert.equal(starting.progress, undefined);
  assert.deepEqual(
    starting.tracks.map((track) => track.label),
    ["Product profile", "Source crawl", "Evidence snippets"],
  );
  assert.deepEqual(
    starting.tracks.map((track) => track.state),
    ["running", "running", "waiting"],
  );

  const noBackendReport = contextBuildTracks({
    hasProfile: false,
    sourcePageCount: 0,
    evidencePageCount: 0,
    evidenceCount: 0,
    pollCount: 18,
    pollLimit: 18,
    runs: [],
  });
  assert.equal(noBackendReport.exhausted, true);
  assert.match(noBackendReport.detail, /backend progress report/i);
  assert.deepEqual(
    noBackendReport.tracks.map((track) => track.state),
    ["attention", "attention", "attention"],
  );

  const startedButStalled = contextBuildTracks({
    hasProfile: false,
    sourcePageCount: 0,
    evidencePageCount: 0,
    evidenceCount: 0,
    pollCount: 18,
    pollLimit: 18,
    runs: [
      { input: { step: "profile", phase: "started" }, output: { profile_stage: "started" }, status: "ok" },
      { input: { step: "crawl", phase: "started" }, output: { target_pages: 20 }, status: "ok" },
    ],
  });
  assert.match(startedButStalled.detail, /started but has not completed/i);
  assert.deepEqual(
    startedButStalled.tracks.map((track) => track.state),
    ["attention", "attention", "attention"],
  );
  assert.match(startedButStalled.tracks[0].detail, /LLM settings/i);

  const withPartialEvidence = contextBuildTracks({
    hasProfile: true,
    sourcePageCount: 12,
    evidencePageCount: 8,
    evidenceCount: 20,
    pollCount: 4,
    pollLimit: 18,
    runs: [
      {
        input: { step: "crawl_summary" },
        output: { crawl_summary: { fetched_count: 20, errors: ["skip https://example.com/blog/slow: timeout"] } },
        status: "ok",
      },
    ],
  });
  assert.equal(withPartialEvidence.tracks[0].state, "done");
  assert.equal(withPartialEvidence.tracks[1].state, "done");
  assert.equal(withPartialEvidence.tracks[2].state, "running");
  assert.equal(withPartialEvidence.tracks[1].progress, 100);
  assert.equal(withPartialEvidence.tracks[2].progress, 40);

  const ready = contextBuildTracks({
    hasProfile: true,
    sourcePageCount: 20,
    evidencePageCount: 20,
    evidenceCount: 40,
    pollCount: 9,
    pollLimit: 18,
    runs: [],
  });
  assert.equal(ready.active, false);
  assert.equal(ready.tracks.every((track) => track.state === "done"), true);
});

test("contextBuildTracks completes evidence when crawl summary reports finished inventory with skips", async () => {
  const { contextBuildTracks } = await loadDashboardUXLogicModule();

  const build = contextBuildTracks({
    hasProfile: true,
    sourcePageCount: 20,
    evidencePageCount: 19,
    evidenceCount: 231,
    pollCount: 1,
    pollLimit: 18,
    runs: [
      {
        input: { step: "crawl_summary" },
        output: {
          crawl_summary: {
            fetched_count: 20,
            inventory_count: 19,
            errors: null,
          },
        },
        status: "ok",
      },
    ],
  });

  assert.equal(build.active, false);
  assert.equal(build.tracks[2].state, "done");
  assert.equal(build.tracks[2].progress, 100);
  assert.match(build.tracks[2].detail, /19 source pages/);
});

test("contextInventoryProgress ignores generated empty evidence rows during onboarding", async () => {
  const { contextInventoryProgress } = await loadDashboardUXLogicModule();

  const stats = contextInventoryProgress([
    { source: "existing", evidence_snippets: ["Existing source evidence"] },
    { source: "existing", evidence_snippets: ["More source evidence"] },
    { source: "generated", evidence_snippets: [] },
  ]);

  assert.deepEqual(stats, {
    sourcePageCount: 2,
    evidencePageCount: 2,
    evidenceCount: 2,
  });
});
