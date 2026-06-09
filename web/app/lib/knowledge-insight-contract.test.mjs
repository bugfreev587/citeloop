import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("knowledge insight copy reflects background crawl", async () => {
  const source = await readFile(new URL("../projects/[id]/knowledge/knowledge-client.tsx", import.meta.url), "utf8");

  assert.equal(source.includes("background_crawl"), true);
  assert.equal(source.includes("Product profile ready"), true);
  assert.equal(source.includes("Crawl completed within configured bounds."), false);
});
