import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadContentPlanLogicModule() {
  const source = await readFile(new URL("./content-plan-logic.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`;
  return import(moduleUrl);
}

function topic(overrides = {}) {
  return {
    id: "topic-1",
    channel: "blog",
    title: "Topic",
    target_keyword: null,
    target_prompt: null,
    angle: null,
    format: null,
    priority: 0,
    internal_links: [],
    status: "backlog",
    scheduled_at: null,
    created_at: "2026-06-01T00:00:00.000Z",
    ...overrides,
  };
}

test("topicPickSignal surfaces differentiators before generic priority", async () => {
  const { topicPickSignal } = await loadContentPlanLogicModule();

  assert.equal(
    topicPickSignal(topic({ priority: 50, internal_links: [{ url: "/a" }, { url: "/b" }, { url: "/c" }] })),
    "Strong internal-link base",
  );
  assert.equal(
    topicPickSignal(
      topic({
        priority: 50,
        target_keyword: "api pricing",
        angle: "Cost model",
        format: "Guide",
      }),
    ),
    "Complete brief",
  );
  assert.equal(topicPickSignal(topic({ priority: 50 })), "Priority set by plan");
});

test("recommendedTopicIds only ranks unscheduled backlog topics", async () => {
  const { recommendedTopicIds } = await loadContentPlanLogicModule();

  const ids = recommendedTopicIds([
    topic({ id: "scheduled", status: "scheduled", scheduled_at: "2026-06-12T10:00:00.000Z", priority: 90 }),
    topic({ id: "generating", status: "generating", priority: 90 }),
    topic({ id: "backlog-older", priority: 10, created_at: "2026-06-01T00:00:00.000Z" }),
    topic({
      id: "backlog-linked",
      priority: 10,
      internal_links: [{ url: "/a" }, { url: "/b" }, { url: "/c" }],
      created_at: "2026-06-02T00:00:00.000Z",
    }),
  ]);

  assert.deepEqual(ids, ["backlog-linked", "backlog-older"]);
});

test("priority 1 outranks priority 3 in plan recommendations", async () => {
  const { recommendedTopicIds, topicPickScore } = await loadContentPlanLogicModule();

  const p1 = topic({ id: "p1", priority: 1, created_at: "2026-06-01T00:00:00.000Z" });
  const p3 = topic({ id: "p3", priority: 3, created_at: "2026-06-02T00:00:00.000Z" });

  assert.ok(topicPickScore(p1) > topicPickScore(p3));
  assert.deepEqual(recommendedTopicIds([p3, p1]), ["p1", "p3"]);
});

test("planHealthForTopics reports whole-plan health independent of active filters", async () => {
  const { planHealthForTopics } = await loadContentPlanLogicModule();

  const health = planHealthForTopics([
    topic({ id: "visible-blog", channel: "blog", status: "backlog", priority: 0 }),
    topic({ id: "hidden-syndication", channel: "syndication", status: "backlog", priority: 20 }),
    topic({ id: "scheduled", channel: "both", status: "scheduled", scheduled_at: "2026-06-12T10:00:00.000Z", priority: 50 }),
    topic({ id: "drafted", status: "drafted", priority: 0 }),
  ]);

  assert.deepEqual(health, {
    backlog: 3,
    readyToDraft: 2,
    scheduledIntent: 1,
    needsPriority: 1,
  });
});

test("hasReviewableDraft only links actions with pending Review drafts", async () => {
  const { hasReviewableDraft } = await loadContentPlanLogicModule();

  assert.equal(
    hasReviewableDraft({ draft_article_id: "article-1", draft_article_status: "pending_review" }),
    true,
  );
  assert.equal(
    hasReviewableDraft({ draft_article_id: "article-1", draft_article_status: null }),
    false,
  );
  assert.equal(
    hasReviewableDraft({ draft_article_id: "article-1", draft_article_status: "drafted" }),
    false,
  );
  assert.equal(
    hasReviewableDraft({ draft_article_id: null, draft_article_status: "pending_review" }),
    false,
  );
});

test("publish strategy recommendations follow brief-first PRD defaults", async () => {
  const {
    normalizePublishStrategy,
    publishStrategyLabel,
    publishStrategyReasonForAction,
    recommendedPublishStrategyForAction,
  } = await loadContentPlanLogicModule();

  assert.equal(normalizePublishStrategy("Distribution"), "syndication");
  assert.equal(normalizePublishStrategy("source article"), "blog");
  assert.equal(normalizePublishStrategy("blog + syndication"), "both");
  assert.equal(publishStrategyLabel("both"), "Both");

  assert.equal(
    recommendedPublishStrategyForAction({
      work_type: "improve_page",
      asset_type: "metadata_rewrite",
      action_type: "Refresh title and meta description",
    }),
    "blog",
  );
  assert.equal(
    recommendedPublishStrategyForAction({
      work_type: "create_content",
      asset_type: "comparison_page",
      opportunity_recommended_action: "Create a comparison guide",
    }),
    "both",
  );
  assert.equal(
    recommendedPublishStrategyForAction({
      input_snapshot: { publish_to: "syndication" },
      work_type: "create_content",
      asset_type: "comparison_page",
    }),
    "syndication",
  );
  assert.equal(
    recommendedPublishStrategyForAction({
      action_type: "Share a community discussion on Reddit",
    }),
    "syndication",
  );
  assert.equal(
    recommendedPublishStrategyForAction({
      action_type: "Create a guide and distribute it on dev.to",
    }),
    "both",
  );
  assert.match(
    publishStrategyReasonForAction({ work_type: "improve_page", asset_type: "page_update" }, "blog"),
    /existing owned page/,
  );
});
