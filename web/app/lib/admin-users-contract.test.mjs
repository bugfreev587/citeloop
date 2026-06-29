import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

const appRoot = path.resolve(import.meta.dirname, "..");
const readApp = (relativePath) => fs.readFileSync(path.join(appRoot, relativePath), "utf8");
const existsApp = (relativePath) => fs.existsSync(path.join(appRoot, relativePath));

test("admin users page exposes owner-wide account cleanup", () => {
  assert.equal(existsApp("admin/users/page.tsx"), true, "/admin/users route should exist");
  assert.equal(existsApp("admin/users/users-client.tsx"), true, "admin users page should have an interactive client");

  const page = readApp("admin/users/page.tsx");
  const client = readApp("admin/users/users-client.tsx");
  const adminHome = readApp("admin/page.tsx");

  assert.match(page, /UsersClient/);
  assert.match(adminHome, /href="\/admin\/users"/, "admin home should link to users below projects");

  for (const copy of ["Users", "Owner email", "Owner ID", "Projects", "Last updated at", "Cancel", "Delete"]) {
    assert.match(client, new RegExp(copy.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }
  assert.match(client, /role="dialog"/);
  assert.match(client, /api\.listAdminUsers\(\)/);
  assert.match(client, /api\.deleteAdminUser/);
  assert.doesNotMatch(client, /window\.confirm/);
  assert.match(client, /admin access required/i);
});

test("web API client exposes admin user list and delete endpoints", () => {
  const source = readApp("lib/api.ts");

  assert.match(source, /export type AdminUser/);
  assert.match(source, /normalizeAdminUser/);
  assert.match(source, /listAdminUsers/);
  assert.match(source, /\/admin\/users/);
  assert.match(source, /deleteAdminUser/);
  assert.match(source, /encodeURIComponent/);
  assert.match(source, /deleted_projects/);
});
