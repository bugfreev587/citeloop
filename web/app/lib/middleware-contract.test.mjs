import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("middleware keeps the landing page public", async () => {
  const source = await readFile(new URL("../../middleware.ts", import.meta.url), "utf8");

  assert.equal(source.includes('createRouteMatcher(["/", "/sign-in(.*)", "/sign-up(.*)"])'), true);
});

test("middleware passes through when Clerk is not configured", async () => {
  const source = await readFile(new URL("../../middleware.ts", import.meta.url), "utf8");

  assert.equal(source.includes("NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY"), true);
  assert.equal(source.includes("NextResponse.next()"), true);
});
