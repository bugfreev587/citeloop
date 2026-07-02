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

test("landing auth actions use Clerk Google OAuth and client-only Dashboard routing", async () => {
  const source = await readFile(new URL("../landing-auth-actions.tsx", import.meta.url), "utf8");

  assert.match(source, /"use client"/);
  assert.match(source, /useSignIn/);
  assert.match(source, /useAuth/);
  assert.match(source, /isLoaded/);
  assert.match(source, /isSignedIn/);
  assert.doesNotMatch(source, /<SignedIn\b/);
  assert.doesNotMatch(source, /<SignedOut\b/);
  assert.match(source, /authenticateWithRedirect/);
  assert.match(source, /strategy:\s*"oauth_google"/);
  assert.doesNotMatch(source, /href="\/sign-in"/);
  assert.doesNotMatch(source, /initialProjects/);
  assert.doesNotMatch(source, /listProjects\(/);
  assert.match(source, /LAST_PROJECT_STORAGE_KEY/);
  assert.match(source, /dashboardHrefForProjects/);
});

test("Google OAuth callback route completes Clerk redirect authentication", async () => {
  const source = await readFile(new URL("../sign-in/sso-callback/page.tsx", import.meta.url), "utf8");

  assert.match(source, /"use client"/);
  assert.match(source, /AuthenticateWithRedirectCallback/);
  assert.match(source, /signInFallbackRedirectUrl=(\{["']\/["']\}|["']\/["'])/);
  assert.match(source, /signUpFallbackRedirectUrl=(\{["']\/["']\}|["']\/["'])/);
});

test("root page is a focused landing page with requested auth actions", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  assert.doesNotMatch(source, /export const dynamic = "force-dynamic"/);
  assert.doesNotMatch(source, /from "@clerk\/nextjs\/server"/);
  assert.doesNotMatch(source, /requireConfiguredClerk/);
  assert.doesNotMatch(source, /createApi/);
  assert.doesNotMatch(source, /listProjects\(/);
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
  assert.match(source, /LandingHeaderActions/);
  assert.match(source, /LandingHeroActions/);
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

test("colored flywheel segments include directional arrow overlays", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  const arrowPaths = [
    ["landing-segment-flow-arrow-discover-ship", "M 499 185 L 482 268 L 406 239 Z", "#f3bd5b"],
    ["landing-segment-flow-arrow-ship-learn", "M 300 530 L 238 469 L 300 422 Z", "#0fb8a0"],
    ["landing-segment-flow-arrow-learn-discover", "M 101 185 L 184 162 L 194 239 Z", "#0da2b3"],
  ];

  for (const [className, path, color] of arrowPaths) {
    const match = source.match(
      new RegExp(`<path(?:(?!<path)[\\s\\S])*?className="landing-segment-flow-arrow ${className}"(?:(?!<path)[\\s\\S])*?/>`),
    );
    assert.ok(match, `${className} arrow should render`);
    assert.match(match[0], new RegExp(`d="${path}"`));
    assert.match(match[0], new RegExp(`fill="${color}"`));
    assert.match(match[0], new RegExp(`stroke="${color}"`));
    assert.match(match[0], /strokeLinejoin="round"/);
    assert.match(match[0], /strokeWidth="8"/);
    assert.doesNotMatch(match[0], /stroke="#26384b"/);
  }
});

test("flywheel motion pauses while the wheel is hovered", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  assert.match(source, /className="landing-flywheel relative mx-auto min-w-0 w-full max-w-\[340px\] sm:max-w-\[650px\]"/);
  assert.match(source, /\.landing-flywheel:is\(:hover, :focus-within\) \.landing-outer-track/);
  assert.match(source, /\.landing-flywheel:is\(:hover, :focus-within\) \.landing-orbit-dot/);
  assert.match(source, /\.landing-flywheel:is\(:hover, :focus-within\) \.landing-segment/);
  assert.match(source, /\.landing-flywheel:is\(:hover, :focus-within\) \.landing-segment-flow-arrow/);
  assert.match(source, /animation-play-state: paused/);
});

test("outer flywheel arrows hug the wheel and follow stage order", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  assert.match(source, /className="landing-outer-arc"/);
  assert.match(source, /className="landing-outer-arrow landing-outer-arrow-discover-ship"/);
  assert.match(source, /className="landing-outer-arrow landing-outer-arrow-ship-learn"/);
  assert.match(source, /className="landing-outer-arrow landing-outer-arrow-learn-discover"/);
  assert.match(source, /stroke-width: 38/);
  assert.match(source, /d="M 89 163 A 252 252 0 0 1 525 186"/);
  assert.match(source, /d="M 525 186 A 252 252 0 0 1 287 552"/);
  assert.match(source, /d="M 287 552 A 252 252 0 0 1 89 163"/);
  assert.match(source, /d="M 542 177 L 541 218 L 507 195 Z"/);
  assert.match(source, /d="M 286 572 L 251 550 L 288 532 Z"/);
  assert.match(source, /d="M 72 152 L 108 133 L 105 174 Z"/);
  assert.doesNotMatch(source, /stroke-width: 46/);
  assert.doesNotMatch(source, /d="M 87 121 A 278 278 0 0 1 541 161"/);
  assert.doesNotMatch(source, /d="M 574 252 A 278 278 0 0 1 252 574"/);
  assert.doesNotMatch(source, /d="M 161 541 A 278 278 0 0 1 121 87"/);
  assert.doesNotMatch(source, /markerEnd="url\(#landing-outer-arrowhead\)"/);
  assert.doesNotMatch(source, /strokeDasharray="512 68 512 68 512 68"/);
  assert.doesNotMatch(source, /d="M 513 151 L 551 151 L 535 191 Z"/);
  assert.doesNotMatch(source, /d="M 485 514 L 521 535 L 480 551 Z"/);
  assert.doesNotMatch(source, /d="M 52 374 L 52 330 L 87 356 Z"/);
});

test("outer flywheel labels sit inside the gray orbit ring", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  assert.match(source, /id="domain-gsc-label" d="M 184 96 A 238 238 0 0 1 416 96"/);
  assert.match(source, /id="opportunities-label" d="M 520 194 A 248 248 0 0 1 520 406"/);
  assert.match(source, /id="published-assets-label" d="M 176 518 A 248 248 0 0 0 424 518"/);
  assert.match(source, /id="measured-outcomes-label" d="M 100 430 A 232 232 0 0 1 100 170"/);
  assert.match(source, /className="landing-ring-label text-\[22px\] font-black"/);
  assert.doesNotMatch(source, /id="domain-gsc-label" d="M 172 79 A 255 255 0 0 1 428 79"/);
  assert.doesNotMatch(source, /id="opportunities-label" d="M 531 192 A 255 255 0 0 1 531 408"/);
  assert.doesNotMatch(source, /id="published-assets-label" d="M 173 521 A 255 255 0 0 0 428 521"/);
  assert.doesNotMatch(source, /id="measured-outcomes-label" d="M 91 446 A 255 255 0 0 1 91 154"/);
  assert.doesNotMatch(source, /landing-ring-label text-\[24px\]/);
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

  assert.match(source, /id="published-assets-label" d="M 176 518 A 248 248 0 0 0 424 518"/);
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

test("landing header actions stay out of narrow mobile viewports", async () => {
  const source = await readFile(new URL("../landing-auth-actions.tsx", import.meta.url), "utf8");

  assert.match(source, /className="hidden flex-wrap items-center justify-end gap-2 sm:flex"/);
  assert.match(source, /Start for free/);
  assert.match(source, /LandingDashboardButton/);
  assert.match(source, /UserButton/);
  assert.doesNotMatch(source, /className="flex flex-wrap items-center justify-end gap-2"/);
});

test("landing hero actions stack cleanly on narrow mobile viewports", async () => {
  const pageSource = await readFile(new URL("../page.tsx", import.meta.url), "utf8");
  const actionSource = await readFile(new URL("../landing-auth-actions.tsx", import.meta.url), "utf8");

  assert.match(pageSource, /className="mt-7 grid w-full max-w-sm grid-cols-1 gap-3 sm:flex sm:max-w-none"/);
  assert.match(actionSource, /inline-flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-slate-950 px-5 text-sm font-semibold text-white transition-colors hover:bg-slate-800 active:scale-\[0\.98\] sm:w-auto/);
  assert.match(actionSource, /<JoinWithGoogleButton className="h-11 w-full px-5 sm:w-auto" \/>/);
  assert.match(actionSource, /className="h-11 w-full px-5 sm:w-auto"/);
  assert.doesNotMatch(pageSource, /className="mt-7 flex flex-wrap gap-3"/);
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

test("projects page does not render an empty project count when the API fails", async () => {
  const source = await readFile(new URL("../projects/page.tsx", import.meta.url), "utf8");

  assert.match(source, /const projectsLoaded = !signedOut && !error/);
  assert.match(source, /projectsLoaded && <Badge tone="neutral">\{projects\.length\} total<\/Badge>/);
  assert.match(source, /projectsLoaded \? \(/);
  assert.doesNotMatch(source, /!signedOut && <Badge tone="neutral">\{projects\.length\} total<\/Badge>/);
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
