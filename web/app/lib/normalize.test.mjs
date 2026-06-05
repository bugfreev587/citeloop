import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadNormalizeModule() {
  const source = await readFile(new URL("./normalize.ts", import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2020,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled).toString("base64")}`;
  return import(moduleUrl);
}

test("normalizeNumeric converts pgtype numeric and nullish values", async () => {
  const { normalizeNumeric } = await loadNormalizeModule();

  assert.equal(normalizeNumeric(null), null);
  assert.equal(normalizeNumeric({ Valid: false }), null);
  assert.equal(normalizeNumeric(4.25), 4.25);
  assert.equal(normalizeNumeric({ Int: 875, Exp: -2, Valid: true }), 8.75);
  assert.equal(normalizeNumeric({ Int: "12345", Exp: -3, Valid: true }), 12.345);
});

test("normalizeTime converts pgtype timestamp and invalid values", async () => {
  const { normalizeTime } = await loadNormalizeModule();

  assert.equal(normalizeTime(null), null);
  assert.equal(normalizeTime({ Valid: false }), null);
  assert.equal(normalizeTime("2026-06-05T12:00:00Z"), "2026-06-05T12:00:00Z");
  assert.equal(
    normalizeTime({ Time: "2026-06-05T12:00:00-07:00", Valid: true }),
    "2026-06-05T12:00:00-07:00",
  );
});

test("normalizeArticle returns clean scores, time fields, and qa issues", async () => {
  const { normalizeArticle } = await loadNormalizeModule();

  const article = normalizeArticle({
    id: "a1",
    project_id: "p1",
    topic_id: "t1",
    kind: "canonical",
    platform: null,
    content_md: "Draft",
    seo_meta: { title: "Title" },
    geo_score: { Int: 91, Exp: -1, Valid: true },
    seo_score: { Int: 82, Exp: -1, Valid: true },
    qa_issues: ["unsupported claim"],
    qa_blocking: true,
    canonical_url: null,
    status: "pending_review",
    scheduled_at: { Time: "2026-06-06T12:00:00Z", Valid: true },
    published_at: { Valid: false },
    created_at: { Time: "2026-06-05T12:00:00Z", Valid: true },
  });

  assert.equal(article.geo_score, 9.1);
  assert.equal(article.seo_score, 8.2);
  assert.equal(article.scheduled_at, "2026-06-06T12:00:00Z");
  assert.equal(article.published_at, null);
  assert.deepEqual(article.qa_issues, ["unsupported claim"]);
});
