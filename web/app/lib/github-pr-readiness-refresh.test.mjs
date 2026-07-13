import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadRefreshCoordinatorModule() {
  const source = await readFile(new URL("./github-pr-readiness-refresh.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`;
  return import(moduleUrl);
}

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((nextResolve, nextReject) => {
    resolve = nextResolve;
    reject = nextReject;
  });
  return { promise, resolve, reject };
}

async function flushMicrotasks() {
  await Promise.resolve();
  await Promise.resolve();
}

test("disable skips a queued stale POST and keeps its persisted GET result", async () => {
  const {
    createGithubPRReadinessRefreshCoordinator,
    createGithubPRReadinessRequestOrder,
  } = await loadRefreshCoordinatorModule();
  const order = createGithubPRReadinessRequestOrder("project-1");
  const livePOST = deferred();
  const calls = [];
  let renderedReadiness = null;

  const livePOSTScope = order.forProject("project-1");
  const coordinator = createGithubPRReadinessRefreshCoordinator(async () => {
    if (!order.isCurrent(livePOSTScope)) return null;
    calls.push("POST");
    const readiness = await livePOST.promise;
    if (order.isCurrent(livePOSTScope)) renderedReadiness = readiness;
    return readiness;
  });
  const pendingPOST = coordinator.request();
  const queuedPOST = coordinator.request("after-mutation");
  assert.deepEqual(calls, ["POST"]);

  const disableGETScope = order.invalidate(livePOSTScope);
  assert.ok(disableGETScope);
  calls.push("GET");
  if (order.isCurrent(disableGETScope)) renderedReadiness = { status: "not_connected" };
  assert.deepEqual(renderedReadiness, { status: "not_connected" });

  livePOST.resolve({ status: "ready" });
  await pendingPOST;
  await queuedPOST;
  assert.deepEqual(calls, ["POST", "GET"], "the invalidated queued generation must not send another POST");
  assert.deepEqual(
    renderedReadiness,
    { status: "not_connected" },
    "a POST started before disable must not repaint the newer persisted result",
  );
});

test("request order synchronously invalidates results from the previous project", async () => {
  const { createGithubPRReadinessRequestOrder } = await loadRefreshCoordinatorModule();
  const order = createGithubPRReadinessRequestOrder("project-1");
  const oldProjectScope = order.forProject("project-1");

  const newProjectScope = order.forProject("project-2");

  assert.equal(order.isCurrent(oldProjectScope), false);
  assert.equal(order.isCurrent(newProjectScope), true);
  assert.equal(newProjectScope.projectId, "project-2");
  assert.equal(newProjectScope.epoch, oldProjectScope.epoch + 1);
  assert.equal(order.invalidate(oldProjectScope), null, "a stale handler must not invalidate the current project");
  assert.equal(order.isCurrent(newProjectScope), true);
});

test("Publisher entry refresh is stable across coordinator rebuilds and repeats only after leaving", async () => {
  const { createGithubPRReadinessPublisherEntryTracker } = await loadRefreshCoordinatorModule();
  const tracker = createGithubPRReadinessPublisherEntryTracker();
  let livePOSTCalls = 0;

  function render(projectId, activeTab) {
    if (tracker.shouldRefresh(projectId, activeTab)) livePOSTCalls += 1;
  }

  render("project-1", "project");
  render("project-1", "publisher");
  assert.equal(livePOSTCalls, 1);

  render("project-1", "publisher");
  render("project-1", "publisher");
  assert.equal(livePOSTCalls, 1, "an epoch or coordinator identity change is not a new tab entry");

  render("project-1", "project");
  render("project-1", "publisher");
  assert.equal(livePOSTCalls, 2, "leaving and re-entering Publisher starts a fresh live check");

  render("project-2", "publisher");
  assert.equal(livePOSTCalls, 3, "a project change while Publisher is active starts that project's entry check");
});

test("normal readiness refreshes share the in-flight generation", async () => {
  const { createGithubPRReadinessRefreshCoordinator } = await loadRefreshCoordinatorModule();
  const firstRun = deferred();
  let executions = 0;
  const coordinator = createGithubPRReadinessRefreshCoordinator(() => {
    executions += 1;
    return firstRun.promise;
  });

  const first = coordinator.request();
  const second = coordinator.request("normal");

  assert.equal(executions, 1);
  assert.equal(first, second, "normal callers should share the same generation promise");

  firstRun.resolve("first result");
  assert.equal(await first, "first result");
  assert.equal(await second, "first result");
  assert.equal(executions, 1);
});

test("an after-mutation refresh waits for a generation after the in-flight check", async () => {
  const { createGithubPRReadinessRefreshCoordinator } = await loadRefreshCoordinatorModule();
  const runs = [deferred(), deferred()];
  let executions = 0;
  const coordinator = createGithubPRReadinessRefreshCoordinator(() => runs[executions++].promise);

  const initial = coordinator.request();
  const afterMutation = coordinator.request("after-mutation");
  let mutationSettled = false;
  void afterMutation.then(
    () => {
      mutationSettled = true;
    },
    () => {
      mutationSettled = true;
    },
  );

  assert.equal(executions, 1);
  runs[0].resolve("before mutation");
  assert.equal(await initial, "before mutation");
  await flushMicrotasks();

  assert.equal(executions, 2, "the mutation must trigger a fresh execution");
  assert.equal(mutationSettled, false, "the mutation promise must wait for its fresh generation");

  runs[1].resolve("after mutation");
  assert.equal(await afterMutation, "after mutation");
});

test("busy stays asserted while a queued physical refresh is still draining", async () => {
  const { createGithubPRReadinessRefreshCoordinator } = await loadRefreshCoordinatorModule();
  const runs = [deferred(), deferred()];
  const busyChanges = [];
  let executions = 0;
  const coordinator = createGithubPRReadinessRefreshCoordinator(
    () => runs[executions++].promise,
    (busy) => busyChanges.push(busy),
  );

  const initial = coordinator.request();
  const afterMutation = coordinator.request("after-mutation");
  assert.deepEqual(busyChanges, [true]);

  runs[0].resolve("initial");
  await initial;
  await flushMicrotasks();
  assert.equal(executions, 2);
  assert.deepEqual(busyChanges, [true], "busy must not clear between queued network checks");

  runs[1].resolve("fresh");
  assert.equal(await afterMutation, "fresh");
  assert.deepEqual(busyChanges, [true, false]);
});

test("multiple mutations during one generation coalesce to one follow-up", async () => {
  const { createGithubPRReadinessRefreshCoordinator } = await loadRefreshCoordinatorModule();
  const runs = [deferred(), deferred()];
  let executions = 0;
  const coordinator = createGithubPRReadinessRefreshCoordinator(() => runs[executions++].promise);

  const initial = coordinator.request();
  const firstMutation = coordinator.request("after-mutation");
  const secondMutation = coordinator.request("after-mutation");

  assert.equal(firstMutation, secondMutation);
  runs[0].resolve("initial");
  await initial;
  await flushMicrotasks();
  assert.equal(executions, 2);

  runs[1].resolve("follow-up");
  assert.deepEqual(await Promise.all([firstMutation, secondMutation]), ["follow-up", "follow-up"]);
  assert.equal(executions, 2);
});

test("a mutation during a follow-up requests a third generation", async () => {
  const { createGithubPRReadinessRefreshCoordinator } = await loadRefreshCoordinatorModule();
  const runs = [deferred(), deferred(), deferred()];
  let executions = 0;
  const coordinator = createGithubPRReadinessRefreshCoordinator(() => runs[executions++].promise);

  const initial = coordinator.request();
  const firstMutation = coordinator.request("after-mutation");
  runs[0].resolve("initial");
  await initial;
  await flushMicrotasks();
  assert.equal(executions, 2);

  const secondMutation = coordinator.request("after-mutation");
  assert.notEqual(firstMutation, secondMutation);
  runs[1].resolve("first follow-up");
  assert.equal(await firstMutation, "first follow-up");
  await flushMicrotasks();

  assert.equal(executions, 3, "the mutation during generation two must not be lost");
  runs[2].resolve("second follow-up");
  assert.equal(await secondMutation, "second follow-up");
});

test("a failed generation does not strand queued freshness or later retries", async () => {
  const { createGithubPRReadinessRefreshCoordinator } = await loadRefreshCoordinatorModule();
  const runs = [deferred(), deferred(), deferred()];
  let executions = 0;
  const coordinator = createGithubPRReadinessRefreshCoordinator(() => runs[executions++].promise);

  const initial = coordinator.request();
  const afterMutation = coordinator.request("after-mutation");
  runs[0].reject(new Error("first check failed"));

  await assert.rejects(initial, /first check failed/);
  await flushMicrotasks();
  assert.equal(executions, 2, "queued freshness must continue after an earlier failure");

  runs[1].resolve("recovered");
  assert.equal(await afterMutation, "recovered");

  const retry = coordinator.request();
  assert.equal(executions, 3, "the coordinator must accept later refreshes after draining");
  runs[2].resolve("retried");
  assert.equal(await retry, "retried");
});
