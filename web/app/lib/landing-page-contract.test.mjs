import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import ts from "typescript";

async function loadTsModule(relativePath) {
  const source = await readFile(new URL(relativePath, import.meta.url), "utf8");
  const output = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2022 },
  }).outputText;
  const moduleUrl = `data:text/javascript;base64,${Buffer.from(output).toString("base64")}`;
  return import(moduleUrl);
}

const projects = [
  { id: "project-new", name: "New", slug: "new" },
  { id: "project-old", name: "Old", slug: "old" },
];

test("landing dashboard route falls back to projects when no projects exist", async () => {
  const { dashboardHrefForProjects } = await loadTsModule("dashboard-routing.ts");

  assert.equal(dashboardHrefForProjects([], "project-new", false), "/projects");
});

test("landing dashboard route uses the stored project when it belongs to the user", async () => {
  const { dashboardHrefForProjects } = await loadTsModule("dashboard-routing.ts");

  assert.equal(dashboardHrefForProjects(projects, "project-old", false), "/projects/project-old");
});

test("landing dashboard route falls back to the first server-prefetched project", async () => {
  const { dashboardHrefForProjects } = await loadTsModule("dashboard-routing.ts");

  assert.equal(dashboardHrefForProjects(projects, "deleted-project", false), "/projects/project-new");
});

test("landing dashboard route falls back to projects when server prefetch failed", async () => {
  const { dashboardHrefForProjects } = await loadTsModule("dashboard-routing.ts");

  assert.equal(dashboardHrefForProjects(projects, "project-old", true), "/projects");
});

test("landing auth actions use Clerk Google OAuth and server-prefetched Dashboard projects", async () => {
  const source = await readFile(new URL("../landing-auth-actions.tsx", import.meta.url), "utf8");

  assert.match(source, /"use client"/);
  assert.match(source, /useSignIn/);
  assert.match(source, /authenticateWithRedirect/);
  assert.match(source, /strategy:\s*"oauth_google"/);
  assert.doesNotMatch(source, /href="\/sign-in"/);
  assert.match(source, /initialProjects/);
  assert.doesNotMatch(source, /listProjects\(/);
  assert.match(source, /LAST_PROJECT_STORAGE_KEY/);
  assert.match(source, /dashboardHrefForProjects/);
});

test("root page is a focused landing page with requested auth actions", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  assert.match(source, /export const dynamic = "force-dynamic"/);
  assert.match(source, /Turn your website into a self-improving growth loop\./);
  assert.match(source, /Connect your domain, Search Console, and publishing target\./);
  assert.match(source, /SEO\/GEO GROWTH LOOP/);
  assert.match(source, /Domain \+ GSC/);
  assert.match(source, /Opportunities/);
  assert.match(source, /Published assets/);
  assert.match(source, /Measured outcomes/);
  assert.match(source, /Discover/);
  assert.match(source, /Ship/);
  assert.match(source, /Learn/);
  assert.match(source, /prefers-reduced-motion/);
  assert.match(source, /JoinWithGoogleButton/);
  assert.match(source, /Start with your domain/);
  assert.match(source, /Start for free/);
  assert.match(source, /LandingDashboardButton/);
  assert.doesNotMatch(source, /content engine/i);
  assert.doesNotMatch(source, /ProjectManagementClient/);
  assert.doesNotMatch(source, /ProjectCreateForm/);
  assert.doesNotMatch(source, />\s*Docs\s*</);
  assert.doesNotMatch(source, />\s*Product\s*</);
});

test("root metadata matches the growth loop positioning", async () => {
  const source = await readFile(new URL("../layout.tsx", import.meta.url), "utf8");

  assert.match(source, /Turn your domain and Search Console data into a self-improving SEO\/GEO growth loop\./);
  assert.doesNotMatch(source, /content engine|automated content/i);
});

test("signed out projects page points users to the growth loop inputs", async () => {
  const source = await readFile(new URL("../projects/page.tsx", import.meta.url), "utf8");

  assert.match(source, /connect your domain, authorize Search Console, and start the growth loop/i);
  assert.doesNotMatch(source, /product URL|content engine/i);
});

test("projects page remains request-rendered for auth-gated project management", async () => {
  const source = await readFile(new URL("../projects/page.tsx", import.meta.url), "utf8");

  assert.match(source, /export const dynamic = "force-dynamic"/);
  assert.match(source, /requireConfiguredClerk/);
  assert.match(source, /auth\(/);
});

test("landing page copy avoids banned marketing phrases", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  for (const banned of ["elevate", "unlock", "next-gen", "seamless"]) {
    assert.equal(source.toLowerCase().includes(banned), false, `${banned} should not appear`);
  }
});

test("project visit recorder writes the recent project id from a client effect", async () => {
  const source = await readFile(new URL("../project-visit-recorder.tsx", import.meta.url), "utf8");

  assert.match(source, /"use client"/);
  assert.match(source, /useEffect/);
  assert.match(source, /window\.localStorage\.setItem/);
  assert.match(source, /LAST_PROJECT_STORAGE_KEY/);
});

test("project shell records project visits", async () => {
  const source = await readFile(new URL("../components/project-shell.tsx", import.meta.url), "utf8");

  assert.match(source, /ProjectVisitRecorder/);
  assert.match(source, /projectId=\{projectId\}/);
});

test("project deletion clears stale recent project storage", async () => {
  const source = await readFile(new URL("../projects/project-management-client.tsx", import.meta.url), "utf8");

  assert.match(source, /LAST_PROJECT_STORAGE_KEY/);
  assert.match(source, /window\.localStorage\.getItem/);
  assert.match(source, /window\.localStorage\.removeItem/);
});
