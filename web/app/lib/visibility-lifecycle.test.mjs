import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadVisibilityLifecycleModule() {
  const source = await readFile(new URL("./visibility-lifecycle.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`;
  return import(moduleUrl);
}

test("unpublished draft articles override stale parent measurement statuses", async () => {
  const { deriveVisibilityLifecycleStage } = await loadVisibilityLifecycleModule();

  assert.equal(
    deriveVisibilityLifecycleStage({
      status: "completed",
      draft_article_id: "article-approved",
      draft_article_status: "approved",
      published_at: null,
      verified_at: "2026-06-30T21:45:35.449Z",
      outcome_summary: { state: "insufficient_data" },
    }),
    "approved",
    "a completed parent action must not be learned while its child article is still waiting in Publish",
  );

  assert.equal(
    deriveVisibilityLifecycleStage({
      status: "measuring",
      draft_article_id: "article-review",
      draft_article_status: "pending_review",
      published_at: null,
      verified_at: "2026-06-30T21:45:35.449Z",
    }),
    "ready_for_review",
    "a measuring parent action must not be in Results while its child article is still in Review",
  );

  assert.equal(
    deriveVisibilityLifecycleStage({
      status: "completed",
      draft_article_id: "article-published",
      draft_article_status: "published",
      published_at: "2026-07-16T12:00:00.000Z",
      outcome_summary: { state: "inconclusive" },
    }),
    "learned",
    "completed work can stay learned after the child article is actually published",
  );
});
