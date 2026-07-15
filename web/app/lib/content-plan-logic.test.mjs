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

test("reviewArticleIDForAction uses a pending sibling when the linked canonical is approved", async () => {
  const { reviewArticleIDForAction } = await loadContentPlanLogicModule();

  assert.equal(
    reviewArticleIDForAction(
      { draft_article_id: "canonical-1", draft_article_status: "approved" },
      "variant-1",
    ),
    "variant-1",
  );
  assert.equal(
    reviewArticleIDForAction(
      { draft_article_id: "canonical-1", draft_article_status: "pending_review" },
      "variant-1",
    ),
    "variant-1",
  );
});

test("hasAdvancedDraftHandoff excludes approved drafts from accepted content work", async () => {
  const { hasAdvancedDraftHandoff } = await loadContentPlanLogicModule();

  assert.equal(
    hasAdvancedDraftHandoff({ draft_article_id: "canonical-1", draft_article_status: "approved" }),
    true,
  );
  assert.equal(
    hasAdvancedDraftHandoff({ draft_article_id: "canonical-1", draft_article_status: "pending_review" }),
    false,
  );
  assert.equal(
    hasAdvancedDraftHandoff({ draft_article_id: "canonical-1", draft_article_status: "rejected" }),
    false,
  );
});

test("Content Plan never renders returned or dismissed actions from stale visibility payloads", async () => {
  const { isActiveContentPlanLoopAction } = await loadContentPlanLogicModule();

  assert.equal(
    isActiveContentPlanLoopAction({ status: "ready_for_review", lifecycle_stage: "added_to_plan", opportunity_status: "converted" }),
    true,
  );
  assert.equal(
    isActiveContentPlanLoopAction({ status: "returned", lifecycle_stage: "added_to_plan", opportunity_status: "open" }),
    false,
  );
  assert.equal(
    isActiveContentPlanLoopAction({ status: "dismissed", lifecycle_stage: "added_to_plan", opportunity_status: "dismissed" }),
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

test("page update actions hide publishing controls and use update language", async () => {
  const {
    contentPlanActionPrimaryCTA,
    contentPlanActionPublishControlsVisible,
    contentPlanActionSurfaceLabel,
    isPageUpdateAction,
    pageUpdateDraftIDForAction,
  } = await loadContentPlanLogicModule();

  const pageUpdate = {
    work_type: "improve_page",
    asset_type: "page_update",
    action_type: "Strengthen the evidence block on this existing page",
  };
  const newContent = {
    work_type: "create_content",
    asset_type: "blog_post",
    action_type: "Create a supporting article",
  };

  assert.equal(isPageUpdateAction(pageUpdate), true);
  assert.equal(contentPlanActionPublishControlsVisible(pageUpdate), false);
  assert.equal(contentPlanActionPrimaryCTA(pageUpdate), "Draft Update");
  assert.equal(contentPlanActionSurfaceLabel(pageUpdate), "Page updates");
  assert.equal(pageUpdateDraftIDForAction({ output_snapshot: { page_update_draft_id: "draft-1" } }), "draft-1");
  assert.equal(pageUpdateDraftIDForAction({ output_snapshot: { page_update_draft_id: " " } }), null);

  assert.equal(isPageUpdateAction(newContent), false);
  assert.equal(contentPlanActionPublishControlsVisible(newContent), true);
  assert.equal(contentPlanActionPrimaryCTA(newContent), "Draft Content");
  assert.equal(contentPlanActionSurfaceLabel(newContent), "Content briefs");
});

test("draft-started content actions leave active Content Plan even before review article exists", async () => {
  const {
    hasDraftStartedHandoff,
    reviewArticleIDForAction,
  } = await loadContentPlanLogicModule();

  assert.equal(
    hasDraftStartedHandoff({ lifecycle_stage: "drafting", topic_status: "generating" }),
    true,
    "background generation should count as draft-started handoff",
  );
  assert.equal(
    hasDraftStartedHandoff({ lifecycle_stage: "ready_for_review", draft_article_status: "pending_review", draft_article_id: "article-1" }),
    true,
    "reviewable draft should count as draft-started handoff",
  );
  assert.equal(
    hasDraftStartedHandoff({ lifecycle_stage: "added_to_plan" }),
    false,
    "plain planned actions should remain in Content Plan",
  );
  assert.equal(reviewArticleIDForAction({ draft_article_id: "article-1", draft_article_status: "pending_review" }), "article-1");
});

test("page update GitHub PR apply results expose PR action instead of publish flow", async () => {
  const {
    pageUpdateDraftGitHubPRURL,
    pageUpdateDraftHasOpenGitHubPR,
    pageUpdateDraftPrimaryCTA,
    pageUpdateDraftBusyCTA,
  } = await loadContentPlanLogicModule();

  const githubDraft = {
    status: "verification_pending",
    publisher_result: {
      mode: "github_pr",
      status: "github_pr_open",
      github_pr_url: "https://github.com/acme/site/pull/42",
    },
  };
  const manualDraft = {
    status: "manual_apply_required",
    publisher_result: { mode: "manual_patch", status: "manual_apply_required" },
  };

  assert.equal(pageUpdateDraftGitHubPRURL(githubDraft), "https://github.com/acme/site/pull/42");
  assert.equal(pageUpdateDraftHasOpenGitHubPR(githubDraft), true);
  assert.equal(pageUpdateDraftPrimaryCTA(githubDraft), "Open PR");
  assert.equal(pageUpdateDraftBusyCTA(githubDraft), "Opening PR");

  assert.equal(pageUpdateDraftGitHubPRURL(manualDraft), null);
  assert.equal(pageUpdateDraftHasOpenGitHubPR(manualDraft), false);
  assert.equal(pageUpdateDraftPrimaryCTA(manualDraft), "Verify Update");
});
