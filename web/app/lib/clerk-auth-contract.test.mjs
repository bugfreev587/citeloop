import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("home page avoids server auth when Clerk is not configured", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  assert.equal(source.includes("CLERK_SECRET_KEY"), true);
  assert.equal(source.includes("createApi(token ? { token } : undefined)"), true);
});

test("project layout avoids server auth when Clerk is not configured", async () => {
  const source = await readFile(new URL("../projects/[id]/layout.tsx", import.meta.url), "utf8");

  assert.equal(source.includes("CLERK_SECRET_KEY"), true);
  assert.equal(source.includes("createApi(token ? { token } : undefined)"), true);
});
