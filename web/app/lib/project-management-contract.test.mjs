import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const read = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");
const exists = (relativePath) => fs.existsSync(path.join(appRoot, relativePath));

test("project shell exposes a bottom projects management entry", () => {
  const shell = read("components/project-shell.tsx");
  const footer = shell.slice(shell.indexOf('className="mt-auto grid gap-2"'));

  assert.match(footer, /href="\/projects"/);
  assert.match(footer, />\s*Projects\s*</);
  assert.match(footer, /FolderKanban/);
});

test("projects management page lists, creates, opens, and hard-deletes projects", () => {
  assert.equal(exists("projects/page.tsx"), true, "global /projects management page should exist");
  assert.equal(
    exists("projects/project-management-client.tsx"),
    true,
    "project management client should own delete confirmation state",
  );

  const page = read("projects/page.tsx");
  assert.match(page, /api\.listProjects\(\)/);
  assert.match(page, /ProjectCreateForm/);
  assert.match(page, /ProjectManagementClient/);

  const client = read("projects/project-management-client.tsx");
  for (const copy of [
    "Open",
    "Delete project",
    "Permanently delete",
    "This permanently deletes the project and all associated data.",
  ]) {
    assert.match(client, new RegExp(copy.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  assert.match(client, /api\.deleteProject\(pendingDelete\.id\)/);
  assert.match(client, /confirmSlug/);
  assert.match(client, /project\.slug/);
  assert.match(client, /setProjects/);
  assert.match(client, /router\.refresh\(\)/);
});
