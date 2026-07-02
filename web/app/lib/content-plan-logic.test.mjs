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

test("planPulseForTopics turns plan health into one operator summary", async () => {
  const { planPulseForTopics } = await loadContentPlanLogicModule();

  assert.deepEqual(
    planPulseForTopics([
      topic({ id: "ready", status: "backlog", priority: 3 }),
      topic({ id: "scheduled", status: "scheduled", scheduled_at: "2026-07-03T10:00:00.000Z", priority: 2 }),
      topic({ id: "needs-priority", status: "backlog", priority: 0 }),
    ]),
    {
      title: "3 topics in the plan",
      detail: "1 ready to draft, 1 scheduled, 1 needs priority.",
      tone: "amber",
    },
  );
});

test("planPulseForTopics gives calm copy for an empty plan", async () => {
  const { planPulseForTopics } = await loadContentPlanLogicModule();

  assert.deepEqual(planPulseForTopics([]), {
    title: "No topics in the plan yet",
    detail: "Review opportunities or generate from domain to seed the first backlog.",
    tone: "neutral",
  });
});

test("topicCardSpanClass only spans three columns in compact view", async () => {
  const { topicCardSpanClass } = await loadContentPlanLogicModule();

  assert.equal(topicCardSpanClass("list", true), "");
  assert.equal(topicCardSpanClass("grid", true), "lg:col-span-2");
  assert.equal(topicCardSpanClass("compact", true), "lg:col-span-2 2xl:col-span-3");
  assert.equal(topicCardSpanClass("compact", false), "");
});
