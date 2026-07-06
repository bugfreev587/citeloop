import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadActivityRunsModule() {
  const source = await readFile(new URL("./activity-runs.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`;
  return import(moduleUrl);
}

const oldTokengateTimeout = {
  id: "old-platform-timeout",
  project_id: "project-1",
  agent: "insight",
  input: { step: "profile", scope: "full_crawl" },
  output: {},
  model: "claude-sonnet-4-6",
  tokens: 0,
  cost_usd: 0,
  status: "error",
  error: 'Post "https://tokengate-production.up.railway.app/v1/chat/completions": context deadline exceeded',
  created_at: "2026-07-05T22:49:00.000Z",
};

const refreshedContext = {
  id: "new-context-refresh",
  project_id: "project-1",
  agent: "insight",
  input: { step: "profile", scope: "full_crawl" },
  output: { profile_stage: "full" },
  model: "claude-sonnet-4-6",
  tokens: 430,
  cost_usd: 0.004,
  status: "ok",
  error: null,
  created_at: "2026-07-06T00:10:00.000Z",
};

test("TokenGate connectivity failures are platform incidents, not user activity attention", async () => {
  const { isPlatformRuntimeFailure, isUserAttentionRun, userVisibleActivityRuns } = await loadActivityRunsModule();

  assert.equal(isPlatformRuntimeFailure(oldTokengateTimeout), true);
  assert.equal(isUserAttentionRun(oldTokengateTimeout, [oldTokengateTimeout]), false);
  assert.deepEqual(userVisibleActivityRuns([oldTokengateTimeout]).map((run) => run.id), []);
});

test("admin runtime incidents omit platform failures superseded by successful context refresh", async () => {
  const { activePlatformRuntimeIncidents } = await loadActivityRunsModule();
  const currentTokengateTimeout = {
    ...oldTokengateTimeout,
    id: "current-platform-timeout",
    created_at: "2026-07-06T01:10:00.000Z",
  };

  assert.deepEqual(
    activePlatformRuntimeIncidents([refreshedContext, oldTokengateTimeout, currentTokengateTimeout]).map((run) => run.id),
    ["current-platform-timeout"],
  );
});

test("successful context refresh clears older context refresh failures from user activity", async () => {
  const { isPlatformRuntimeFailure, isUserAttentionRun, userVisibleActivityRuns } = await loadActivityRunsModule();
  const crawlInputFailure = {
    ...oldTokengateTimeout,
    id: "old-crawl-input-failure",
    error: "crawl: temporary upstream fetch failure",
  };

  assert.equal(isPlatformRuntimeFailure(crawlInputFailure), false);
  assert.equal(isUserAttentionRun(crawlInputFailure, [crawlInputFailure]), true);
  assert.equal(isUserAttentionRun(crawlInputFailure, [refreshedContext, crawlInputFailure]), false);
  assert.deepEqual(userVisibleActivityRuns([refreshedContext, crawlInputFailure]).map((run) => run.id), ["new-context-refresh"]);
});

test("current user-fixable failures stay in user activity", async () => {
  const { isUserAttentionRun, userVisibleActivityRuns } = await loadActivityRunsModule();
  const publisherFailure = {
    ...oldTokengateTimeout,
    id: "publisher-failure",
    agent: "publisher",
    input: { step: "publish" },
    error: "GitHub token revoked",
    created_at: "2026-07-06T01:00:00.000Z",
  };

  assert.equal(isUserAttentionRun(publisherFailure, [publisherFailure]), true);
  assert.deepEqual(userVisibleActivityRuns([publisherFailure]).map((run) => run.id), ["publisher-failure"]);
});
