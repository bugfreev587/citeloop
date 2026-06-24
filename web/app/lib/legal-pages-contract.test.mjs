import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");
const exists = (relativePath) => fs.existsSync(path.join(appRoot, relativePath));
const has = (text, copy) => assert.match(text, new RegExp(copy.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));

test("public legal routes exist for Google OAuth app configuration", () => {
  assert.equal(exists("privacy/page.tsx"), true, "public /privacy page should exist");
  assert.equal(exists("terms/page.tsx"), true, "public /terms page should exist");

  const privacy = read("privacy/page.tsx");
  const terms = read("terms/page.tsx");

  for (const copy of [
    "Privacy Policy",
    "Google Search Console data",
    "OAuth access tokens and refresh tokens",
    "Google API Services User Data Policy",
    "Limited Use",
    "We do not sell personal data",
    "https://citeloop.app",
    "support@citeloop.app",
  ]) {
    has(privacy, copy);
  }

  for (const copy of [
    "Terms of Service",
    "Google Search Console",
    "Search Console property",
    "third-party services",
    "support@citeloop.app",
  ]) {
    has(terms, copy);
  }
});

test("home page exposes legal links for OAuth brand review", () => {
  const home = read("page.tsx");

  assert.match(home, /href="\/privacy"/);
  assert.match(home, /href="\/terms"/);
  assert.match(home, />\s*Privacy\s*</);
  assert.match(home, />\s*Terms\s*</);
});
