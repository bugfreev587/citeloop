import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const readApp = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");
const existsApp = (relativePath) => fs.existsSync(path.join(appRoot, relativePath));

test("admin projects page exposes all-account project management", () => {
  assert.equal(existsApp("admin/projects/page.tsx"), true, "/admin/projects route should exist");
  assert.equal(existsApp("admin/projects/projects-client.tsx"), true, "admin projects page should have an interactive client");

  const page = readApp("admin/projects/page.tsx");
  const client = readApp("admin/projects/projects-client.tsx");
  const adminHome = readApp("admin/page.tsx");

  assert.match(page, /ProjectsClient/);
  assert.match(adminHome, /href="\/admin\/projects"/, "admin home should link to project management");

  for (const copy of ["Projects", "Owner email", "Owner ID", "Created", "Last updated at", "Delete"]) {
    assert.match(client, new RegExp(copy.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  assert.match(client, /project\.updated_at/);
  assert.match(client, /api\.listAdminProjects\(\)/);
  assert.match(client, /api\.deleteAdminProject/);
  assert.match(client, /window\.confirm/);
  assert.match(client, /admin access required/i);
});

test("web API client exposes admin project list and delete endpoints", () => {
  const source = readApp("lib/api.ts");

  assert.match(source, /export type AdminProject/);
  assert.match(source, /normalizeAdminProject/);
  assert.match(source, /listAdminProjects/);
  assert.match(source, /\/admin\/projects/);
  assert.match(source, /deleteAdminProject/);
  assert.match(source, /owner_email/);
  assert.match(source, /updated_at/);
});
