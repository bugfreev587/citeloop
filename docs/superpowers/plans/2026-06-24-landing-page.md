# Landing Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `/` with a compact landing page and route signed-in users back to the right project dashboard using browser-local recent project state.

**Architecture:** Keep `/` as a server component that reads Clerk auth and server-prefetches projects for signed-in users. Use small client leaves for Google OAuth, Dashboard routing, and project-visit localStorage writes. Keep destination selection in a pure helper so it can be tested before UI changes.

**Tech Stack:** Next.js App Router, React 18, Clerk, Tailwind CSS 3, lucide-react, Node `--test` contract tests.

**Execution update:** Implemented in `codex/landing-page-prd`. During verification, the new auth-gated `/projects` page needed `export const dynamic = "force-dynamic"` to avoid build-time prerender auth failures, and Clerk v7 required importing the OAuth hook from `@clerk/nextjs/legacy` for `authenticateWithRedirect`.

---

### Task 1: Dashboard Routing Helper

**Files:**
- Create: `web/app/lib/dashboard-routing.ts`
- Test: `web/app/lib/landing-page-contract.test.mjs`

- [ ] **Step 1: Write the failing helper tests**

Add these tests to `web/app/lib/landing-page-contract.test.mjs`:

```js
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
```

- [ ] **Step 2: Run tests to verify RED**

Run: `cd web && npm test -- app/lib/landing-page-contract.test.mjs`

Expected: fail because `landing-page-contract.test.mjs` or `dashboard-routing.ts` does not exist.

- [ ] **Step 3: Implement minimal helper**

Create `web/app/lib/dashboard-routing.ts`:

```ts
type ProjectLike = {
  id: string;
};

export const LAST_PROJECT_STORAGE_KEY = "citeloop:last-project-id";

export function dashboardHrefForProjects(
  projects: ProjectLike[],
  storedProjectId: string | null,
  projectPrefetchFailed = false,
) {
  if (projectPrefetchFailed || projects.length === 0) {
    return "/projects";
  }
  const storedProject = storedProjectId ? projects.find((project) => project.id === storedProjectId) : null;
  return `/projects/${storedProject?.id ?? projects[0].id}`;
}
```

- [ ] **Step 4: Run tests to verify GREEN**

Run: `cd web && npm test -- app/lib/landing-page-contract.test.mjs`

Expected: helper tests pass.

---

### Task 2: Landing Client Components

**Files:**
- Create: `web/app/landing-auth-actions.tsx`
- Modify: `web/app/lib/landing-page-contract.test.mjs`

- [ ] **Step 1: Write failing client component contract tests**

Append these tests:

```js
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
```

- [ ] **Step 2: Run tests to verify RED**

Run: `cd web && npm test -- app/lib/landing-page-contract.test.mjs`

Expected: fail because `landing-auth-actions.tsx` does not exist.

- [ ] **Step 3: Implement client actions**

Create `web/app/landing-auth-actions.tsx` with two exported client components:

```tsx
"use client";

import { useSignIn } from "@clerk/nextjs/legacy";
import { ArrowRight, Loader2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";
import type { Project } from "./lib/api";
import { LAST_PROJECT_STORAGE_KEY, dashboardHrefForProjects } from "./lib/dashboard-routing";

export function JoinWithGoogleButton() {
  const { isLoaded, signIn } = useSignIn();
  const [busy, setBusy] = useState(false);

  async function joinWithGoogle() {
    if (!isLoaded || !signIn || busy) return;
    setBusy(true);
    await signIn.authenticateWithRedirect({
      strategy: "oauth_google",
      redirectUrl: "/sign-in/sso-callback",
      redirectUrlComplete: "/",
    });
  }

  return (
    <button type="button" onClick={joinWithGoogle} disabled={!isLoaded || busy}>
      {busy ? "Opening Google..." : "Join with Google"}
    </button>
  );
}

export function LandingDashboardButton({
  initialProjects,
  projectPrefetchFailed = false,
}: {
  initialProjects: Project[];
  projectPrefetchFailed?: boolean;
}) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);

  function openDashboard() {
    if (busy) return;
    setBusy(true);
    const storedProjectId =
      typeof window === "undefined" ? null : window.localStorage.getItem(LAST_PROJECT_STORAGE_KEY);
    router.push(dashboardHrefForProjects(initialProjects, storedProjectId, projectPrefetchFailed));
  }

  return (
    <button type="button" onClick={openDashboard} disabled={busy}>
      {busy ? "Opening..." : "Dashboard"}
      <ArrowRight size={16} />
    </button>
  );
}
```

Style the real implementation to match the landing page; the snippet above only defines required behavior.

- [ ] **Step 4: Run tests to verify GREEN**

Run: `cd web && npm test -- app/lib/landing-page-contract.test.mjs`

Expected: client component contract passes.

---

### Task 3: Landing Page Server Component

**Files:**
- Modify: `web/app/page.tsx`
- Create: `web/app/projects/page.tsx`
- Modify: `web/app/lib/docs-contract.test.mjs`
- Modify: `web/app/lib/project-management-contract.test.mjs`
- Modify: `web/app/lib/landing-page-contract.test.mjs`

- [ ] **Step 1: Write failing landing page contract tests**

Add or update tests to assert:

```js
test("root page is a focused landing page with requested auth actions", async () => {
  const source = await readFile(new URL("../page.tsx", import.meta.url), "utf8");

  assert.match(source, /export const dynamic = "force-dynamic"/);
  assert.match(source, /The content engine that already knows your site\./);
  assert.match(source, /SEO \+ GEO AUTOPILOT/);
  assert.match(source, /JoinWithGoogleButton/);
  assert.match(source, /Start for free/);
  assert.match(source, /LandingDashboardButton/);
  assert.doesNotMatch(source, /ProjectManagementClient/);
  assert.doesNotMatch(source, /ProjectCreateForm/);
  assert.doesNotMatch(source, />\s*Docs\s*</);
  assert.doesNotMatch(source, />\s*Product\s*</);
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
```

Update existing tests that currently expect `/` to list projects so they expect `/projects/page.tsx` to own project management.

- [ ] **Step 2: Run tests to verify RED**

Run: `cd web && npm test -- app/lib/landing-page-contract.test.mjs app/lib/clerk-auth-contract.test.mjs app/lib/project-management-contract.test.mjs`

Expected: fail because the current root page is still project management.

- [ ] **Step 3: Implement landing page**

Rewrite `web/app/page.tsx` as a server-rendered landing page:

```tsx
import Link from "next/link";
import { UserButton } from "@clerk/nextjs";
import { auth } from "@clerk/nextjs/server";
import { ArrowRight, BarChart3, CheckCircle2, PenLine, Search } from "lucide-react";
import { JoinWithGoogleButton, LandingDashboardButton } from "./landing-auth-actions";
import { clerkServerAuthConfigured, requireConfiguredClerk } from "./lib/auth-config";
import { createApi, Project } from "./lib/api";

export const dynamic = "force-dynamic";
```

Move the existing project management experience to `web/app/projects/page.tsx` and add `export const dynamic = "force-dynamic"` there as well. Keep existing Clerk guard and token logic, server-prefetch projects when signed in, render:

- Header with brand.
- Signed-out actions: `JoinWithGoogleButton`, `/sign-up` link labeled `Start for free`.
- Signed-in actions: `LandingDashboardButton`, `UserButton`.
- Hero label/headline/body from the PRD.
- Static product preview and compact three-step section.

- [ ] **Step 4: Run tests to verify GREEN**

Run: `cd web && npm test -- app/lib/landing-page-contract.test.mjs app/lib/clerk-auth-contract.test.mjs app/lib/project-management-contract.test.mjs`

Expected: focused contract tests pass.

---

### Task 4: Recent Project Recorder and Delete Cleanup

**Files:**
- Create: `web/app/project-visit-recorder.tsx`
- Modify: `web/app/components/project-shell.tsx`
- Modify: `web/app/projects/project-management-client.tsx`
- Modify: `web/app/lib/landing-page-contract.test.mjs`

- [ ] **Step 1: Write failing localStorage contract tests**

Append tests:

```js
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
```

- [ ] **Step 2: Run tests to verify RED**

Run: `cd web && npm test -- app/lib/landing-page-contract.test.mjs`

Expected: fail because recorder and cleanup do not exist.

- [ ] **Step 3: Implement recorder and cleanup**

Create `web/app/project-visit-recorder.tsx`:

```tsx
"use client";

import { useEffect } from "react";
import { LAST_PROJECT_STORAGE_KEY } from "./lib/dashboard-routing";

export function ProjectVisitRecorder({ projectId }: { projectId: string }) {
  useEffect(() => {
    window.localStorage.setItem(LAST_PROJECT_STORAGE_KEY, projectId);
  }, [projectId]);
  return null;
}
```

Render `<ProjectVisitRecorder projectId={projectId} />` inside `ProjectShell`.

In `ProjectManagementClient`, after successful delete, if the stored id equals the deleted id, remove the key.

- [ ] **Step 4: Run tests to verify GREEN**

Run: `cd web && npm test -- app/lib/landing-page-contract.test.mjs`

Expected: localStorage contract tests pass.

---

### Task 5: Full Verification and Commit

**Files:**
- Verify all modified web files and docs.

- [ ] **Step 1: Run full web tests**

Run: `cd web && npm test`

Expected: all tests pass.

- [ ] **Step 2: Run typecheck**

Run: `cd web && npm run typecheck`

Expected: TypeScript passes.

- [ ] **Step 3: Build**

Run: `cd web && npm run build`

Expected: Next.js build succeeds.

- [ ] **Step 4: Browser verification**

Run the dev server with `cd web && npm run dev`, open the page, and verify:

- Signed-out landing page renders.
- Header actions do not overlap on mobile.
- Product preview and supporting section fit desktop and mobile.
- No center `Docs` or `Product` links appear in the landing header.

Execution note: local verification ran without real Clerk production keys, so auth-specific header rendering was covered by source contracts while visible layout was verified in the keyless local browser session.

- [ ] **Step 5: Commit**

Run:

```bash
git add docs/PRD-CiteLoop-Landing-Page.md docs/superpowers/plans/2026-06-24-landing-page.md web/app
git commit -m "feat(web): add focused landing page"
```

Expected: commit contains PRD, plan, tests, and implementation.
