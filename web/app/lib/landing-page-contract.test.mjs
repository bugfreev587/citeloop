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

test("flywheel segment labels follow curved paths inside each segment", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  for (const label of ["discover", "ship", "learn"]) {
    assert.match(source, new RegExp(`id="${label}-segment-label"`));
    assert.match(source, new RegExp(`href="#${label}-segment-label"`));
  }

  assert.match(source, /landing-segment-label/);
  assert.doesNotMatch(source, /transform="rotate\([^"]+"\s+className="fill-white text-\[48px\] font-black"/);
});

test("flywheel motion pauses while the wheel is hovered", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  assert.match(source, /className="landing-flywheel relative mx-auto min-w-0 w-full max-w-\[340px\] sm:max-w-\[650px\]"/);
  assert.match(source, /\.landing-flywheel:is\(:hover, :focus-within\) \.landing-outer-track/);
  assert.match(source, /\.landing-flywheel:is\(:hover, :focus-within\) \.landing-orbit-dot/);
  assert.match(source, /\.landing-flywheel:is\(:hover, :focus-within\) \.landing-segment/);
  assert.match(source, /animation-play-state: paused/);
});

test("outer flywheel arrows are attached to curved track ends", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  assert.match(source, /className="landing-outer-arc"/);
  assert.match(source, /className="landing-outer-arrow"/);
  assert.match(source, /d="M 87 121 A 278 278 0 0 1 541 161"/);
  assert.match(source, /d="M 574 252 A 278 278 0 0 1 252 574"/);
  assert.match(source, /d="M 161 541 A 278 278 0 0 1 121 87"/);
  assert.match(source, /d="M 514 132 L 560 158 L 532 194 Z"/);
  assert.match(source, /d="M 261 551 L 220 568 L 253 599 Z"/);
  assert.match(source, /d="M 128 112 L 145 67 L 98 76 Z"/);
  assert.doesNotMatch(source, /markerEnd="url\(#landing-outer-arrowhead\)"/);
  assert.doesNotMatch(source, /strokeDasharray="512 68 512 68 512 68"/);
  assert.doesNotMatch(source, /d="M 513 151 L 551 151 L 535 191 Z"/);
  assert.doesNotMatch(source, /d="M 485 514 L 521 535 L 480 551 Z"/);
  assert.doesNotMatch(source, /d="M 52 374 L 52 330 L 87 356 Z"/);
});

test("ship and learn labels sit near the middle of their colored bands", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  assert.match(source, /id="ship-segment-label" d="M 365 483 A 195 195 0 0 0 490 256"/);
  assert.match(source, /id="learn-segment-label" d="M 108 266 A 195 195 0 0 0 233 483"/);
  assert.doesNotMatch(source, /id="ship-segment-label" d="M 358 458 A 168 168 0 0 0 465 271"/);
  assert.doesNotMatch(source, /id="learn-segment-label" d="M 135 271 A 168 168 0 0 0 242 458"/);
});

test("bottom flywheel output label reads upright from left to right", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  assert.match(source, /id="published-assets-label" d="M 170 548 A 265 265 0 0 0 430 548"/);
  assert.doesNotMatch(source, /id="published-assets-label" d="M 430 548 A 265 265 0 0 1 170 548"/);
});

test("landing hero columns can shrink inside mobile viewport", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  assert.match(source, /className="min-w-0 max-w-xl"/);
  assert.match(source, /className="landing-flywheel relative mx-auto min-w-0 w-full max-w-\[340px\] sm:max-w-\[650px\]"/);
  assert.match(source, /className="h-auto w-full overflow-hidden" viewBox="-28 -28 656 656"/);
  assert.match(source, /text-\[2rem\] font-black leading-\[1\.04\] tracking-tight text-slate-950 break-words sm:text-4xl md:text-6xl/);
  assert.match(source, /aria-label="Turn your website into a self-improving growth loop\."/);
  assert.match(source, /className="block sm:inline"/);
  assert.match(source, /mt-5 max-w-\[31ch\] text-sm leading-6 text-stone-700 sm:max-w-\[58ch\] sm:text-base sm:leading-7 md:text-lg/);
  assert.match(source, /mx-auto -mt-3 max-w-\[30ch\] text-center text-xs font-semibold leading-5 text-stone-600 sm:-mt-4 sm:max-w-sm sm:text-sm sm:leading-6/);
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
