import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("middleware keeps the landing page public", async () => {
  const source = await readFile(new URL("../../middleware.ts", import.meta.url), "utf8");

  assert.equal(source.includes('"/"'), true);
  assert.equal(source.includes('"/docs(.*)"'), true);
  assert.equal(source.includes('"/sign-in(.*)"'), true);
  assert.equal(source.includes('"/sign-up(.*)"'), true);
});

test("middleware passes through when Clerk is not configured", async () => {
  const source = await readFile(new URL("../../middleware.ts", import.meta.url), "utf8");

  assert.equal(source.includes("allowUnconfiguredClerkBypass"), true);
  assert.equal(source.includes("NextResponse.next()"), true);
});

test("middleware fails closed when Clerk is not configured in production", async () => {
  const source = await readFile(new URL("../../middleware.ts", import.meta.url), "utf8");

  assert.equal(source.includes("allowUnconfiguredClerkBypass"), true);
  assert.equal(source.includes("status: 503"), true);
});
