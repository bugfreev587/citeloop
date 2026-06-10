import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");
const exists = (relativePath) => fs.existsSync(path.join(appRoot, relativePath));

test("global docs route exists and explains the CiteLoop loop with Phase 1 IA labels", () => {
  assert.equal(exists("docs/page.tsx"), true, "global /docs page should exist before a project is created");

  const docs = read("docs/page.tsx");

  for (const copy of [
    "Overview",
    "How CiteLoop turns your domain into evidence-backed SEO and GEO content.",
    "Feed opportunities back into the plan",
    "Start here",
    "Core concepts",
    "Workflow model",
    "Common states and signals",
    "Context",
    "Content Plan",
    "Publish",
    "Visibility",
    "Settings > Activity Log",
    "Create your first project",
  ]) {
    assert.match(docs, new RegExp(copy.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  for (const staleCopy of ["Open SEO", "Open Publishing", "Open Runs", "Knowledge / Topics"]) {
    assert.doesNotMatch(docs, new RegExp(staleCopy.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
});

test("root page exposes docs before a zero-project user creates a project", () => {
  const home = read("page.tsx");

  assert.match(home, /href="\/docs"/);
  assert.match(home, /Read the docs/);
});

test("project shell exposes Docs in the footer and keeps it reachable on mobile", () => {
  const shell = read("components/project-shell.tsx");
  const footer = shell.slice(shell.indexOf('className="mt-auto grid gap-2"'));
  const docsIndex = footer.indexOf("Docs");
  const accountIndex = footer.indexOf("<UserButton");

  assert.notEqual(docsIndex, -1, "Docs link should exist");
  assert.ok(docsIndex < accountIndex, "Docs should render above the project/account card");

  // The old Help entry linked to "/" (home), which was redundant with Docs and misleading; it was removed.
  assert.doesNotMatch(footer, />\s*Help\s*</, "redundant Help link should be removed");

  assert.match(shell, /BookOpen/);
  assert.match(shell, /href="\/docs"/);
  assert.match(shell, /isDocsActive/);
});
