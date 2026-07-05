import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");

test("web API exposes content action verification endpoint", () => {
  const api = read("lib/api.ts");
  for (const snippet of [
    "verifySEOContentAction",
    "/verify",
    "verification_snapshot",
    "status: \"verified\" | \"failed\" | \"recovery_required\" | string",
  ]) {
    assert.match(api, new RegExp(snippet.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("web API exposes content action dismiss endpoint", () => {
  const api = read("lib/api.ts");
  for (const snippet of [
    "dismissSEOContentAction",
    "/dismiss",
    "Promise<SEOContentAction>",
  ]) {
    assert.match(api, new RegExp(snippet.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("SEO action card renders manual verification controls", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  for (const snippet of [
    "verifyAction",
    "Manual verify",
    "Verification failed",
    "verification_snapshot",
    "manual_dashboard",
    "api.verifySEOContentAction",
  ]) {
    assert.match(seo, new RegExp(snippet.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});
