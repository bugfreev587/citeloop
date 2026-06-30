import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const readApp = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");

test("admin page groups runtime and GEO credentials behind top tabs", () => {
  const admin = readApp("admin/page.tsx");

  assert.match(admin, /type AdminTabId = "runtime" \| "geo"/);
  assert.match(admin, /const adminTabs:/);
  assert.match(admin, /role="tablist"/);
  assert.match(admin, /aria-selected=\{activeAdminTab === tab\.id\}/);
  assert.match(admin, /activeAdminTab === "runtime" && \(/);
  assert.match(admin, /activeAdminTab === "geo" && \(/);
  assert.match(admin, /Platform runtime/);
  assert.match(admin, /GEO providers/);
});

test("admin GEO tab configures four TokenGate-backed providers", () => {
  const admin = readApp("admin/page.tsx");

  for (const copy of [
    "TokenGate key for Perplexity",
    "TokenGate key for OpenAI",
    "TokenGate key for Anthropic",
    "TokenGate key for Gemini",
    "sonar-pro",
    "gpt-5.1",
    "claude-sonnet-4-6",
    "gemini-2.5-pro",
  ]) {
    assert.match(admin, new RegExp(copy.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.match(admin, /geoProviders\.map/);
  assert.match(admin, /api\.listGEOCredentials/);
  assert.match(admin, /api\.updateGEOCredentials/);
  assert.match(admin, /api\.testGEOCredentials/);
  assert.match(admin, /api\.deleteGEOCredentials/);
  assert.doesNotMatch(admin, /Perplexity API key/);
  assert.doesNotMatch(admin, /OpenAI API key/);
  assert.doesNotMatch(admin, /Anthropic API key/);
  assert.doesNotMatch(admin, /Gemini API key/);
});
