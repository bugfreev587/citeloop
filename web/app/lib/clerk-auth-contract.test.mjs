import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("home page avoids server auth and backend fetch on the public landing route", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  assert.equal(source.includes("from \"@clerk/nextjs/server\""), false);
  assert.equal(source.includes("clerkServerAuthConfigured"), false);
  assert.equal(source.includes("requireConfiguredClerk()"), false);
  assert.equal(source.includes("createApi("), false);
  assert.equal(source.includes("listProjects("), false);
});

test("project layout avoids server auth when Clerk is not configured", async () => {
  const source = await readFile(new URL("../projects/[id]/layout.tsx", import.meta.url), "utf8");

  assert.equal(source.includes("clerkServerAuthConfigured"), true);
  assert.equal(source.includes("requireConfiguredClerk()"), true);
  assert.equal(source.includes("createApi(token ? { token } : undefined)"), true);
});

test("settings page avoids server auth when Clerk is not configured", async () => {
  const source = await readFile(new URL("../projects/[id]/settings/page.tsx", import.meta.url), "utf8");

  assert.equal(source.includes("requireConfiguredClerk()"), true);
  assert.equal(source.includes("auth()"), false);
  assert.equal(source.includes("canUseInternalTools"), false);
  assert.equal(source.includes("notFound()"), false);
});

test("server auth config distinguishes production fail-closed from preview bypass", async () => {
  const source = await readFile(new URL("auth-config.ts", import.meta.url), "utf8");

  assert.equal(source.includes("process.env.VERCEL_ENV"), true);
  assert.equal(source.includes("process.env.NODE_ENV"), true);
  assert.equal(source.includes("allowUnconfiguredClerkBypass"), true);
  assert.equal(source.includes("throw new Error"), true);
});
