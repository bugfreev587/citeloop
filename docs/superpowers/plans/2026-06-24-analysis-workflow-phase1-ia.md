# Analysis Workflow Phase 1 IA Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Implement Phase 1 of `docs/PRD-CiteLoop-Analysis-Workflow.md`: isolate Analysis from Content Plan in navigation, rename the user-facing measurement surface to Results, and keep old routes working through redirects.

**Architecture:** Keep the existing SEO data/client implementation and only change the IA layer, route wrappers, user-facing labels, and route references. `SEOClient` remains the shared implementation for now, with `AnalysisClient` and `ResultsClient` naming the two canonical surfaces. Old `/opportunities` and `/visibility` routes become redirects.

**Tech Stack:** Next.js App Router, React Server Components for route wrappers, existing client component `web/app/projects/[id]/seo/seo-client.tsx`, Node test runner contract tests, TypeScript.

---

## File Structure

- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
  - Owns route/sidebar/home/docs IA contract assertions.
- Modify: `web/app/components/project-shell.tsx`
  - Owns project sidebar and mobile project nav.
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
  - Keeps shared SEO UI implementation while exporting `AnalysisClient` and `ResultsClient`.
- Create: `web/app/projects/[id]/analysis/page.tsx`
  - Canonical Analysis route.
- Create: `web/app/projects/[id]/results/page.tsx`
  - Canonical Results route.
- Modify: `web/app/projects/[id]/opportunities/page.tsx`
  - Legacy redirect to `/analysis`.
- Modify: `web/app/projects/[id]/visibility/page.tsx`
  - Legacy redirect to `/results`.
- Modify: `web/app/projects/[id]/seo/page.tsx`
  - Legacy redirect to `/results`.
- Modify: `web/app/lib/dashboard-ux-logic.ts`
  - Updates Home next-action and momentum links/copy.
- Modify: `web/app/projects/[id]/workspace.tsx`
  - Updates pipeline labels and links from Opportunities/Visibility to Analysis/Results.
- Modify: `web/app/projects/[id]/topics/topics-client.tsx`
  - Updates Content Plan CTA copy/links to Analysis.
- Modify: `web/app/docs/page.tsx`
  - Updates docs navigation and page descriptions to Analysis/Results.

## Task 1: Update IA Contract Tests

**Files:**
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [x] **Step 1: Change sidebar expectations to the new PRD IA**

Replace the first navigation test expectations with:

```js
for (const label of ["Home", "Context", "Analysis", "Content Plan", "Review", "Publish", "Results"]) {
  assert.match(shell, new RegExp(`label: "${label}"`));
}

for (const removed of ['label: "Opportunities"', 'label: "Visibility"', 'label: "SYSTEM"']) {
  assert.doesNotMatch(shell, new RegExp(removed.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
}
```

- [x] **Step 2: Require Settings and Admin in footer utility area**

In the shell test, assert that `Settings` appears after `Docs`, and that `Admin` is only rendered in the footer admin block:

```js
assert.match(shell, /Docs[\s\S]*Settings/);
assert.match(shell, /isPlatformAdmin[\s\S]*Admin/);
assert.doesNotMatch(shell, /id: "system"/);
```

- [x] **Step 3: Require canonical Analysis and Results routes**

Change route assertions to:

```js
for (const route of [
  "projects/[id]/context/page.tsx",
  "projects/[id]/analysis/page.tsx",
  "projects/[id]/plan/page.tsx",
  "projects/[id]/publish/page.tsx",
  "projects/[id]/results/page.tsx",
  "projects/[id]/settings/activity/page.tsx",
]) {
  assert.equal(exists(route), true, `${route} should exist`);
}

const redirects = new Map([
  ["projects/[id]/knowledge/page.tsx", "/context"],
  ["projects/[id]/topics/page.tsx", "/plan"],
  ["projects/[id]/publishing/page.tsx", "/publish"],
  ["projects/[id]/opportunities/page.tsx", "/analysis"],
  ["projects/[id]/visibility/page.tsx", "/results"],
  ["projects/[id]/seo/page.tsx", "/results"],
  ["projects/[id]/runs/page.tsx", "/settings/activity"],
]);
```

- [x] **Step 4: Update surface ownership assertions**

Replace the old opportunities/visibility ownership test with assertions for Analysis and Results:

```js
const analysisPage = read("projects/[id]/analysis/page.tsx");
const resultsPage = read("projects/[id]/results/page.tsx");
const seo = read("projects/[id]/seo/seo-client.tsx");

assert.match(analysisPage, /AnalysisClient/);
assert.match(resultsPage, /ResultsClient/);
assert.match(seo, /export function AnalysisClient/);
assert.match(seo, /export function ResultsClient/);
assert.match(seo, /mode="analysis"/);
assert.match(seo, /mode="results"/);

for (const copy of ["Review analysis", "Analyze opportunities", "Add to Content Plan", "What to decide", "Decision result"]) {
  assert.match(seo, new RegExp(copy));
}
for (const copy of ["Results", "Measurement and diagnostics", "GEO visibility"]) {
  assert.match(seo, new RegExp(copy));
}
```

- [x] **Step 5: Update Home route/copy assertions**

Change Home assertions from `/opportunities` and `/visibility` to:

```js
assert.match(workspace, /href: `\/projects\/\$\{projectId\}\/analysis`/);
assert.match(workspace, /href: `\/projects\/\$\{projectId\}\/results`/);
assert.match(workspace, /"Analysis"/);
assert.match(workspace, /"Results"/);
```

- [x] **Step 6: Run the focused contract test and verify RED**

Run:

```bash
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: FAIL because routes, exports, nav labels, and links still use the old IA.

## Task 2: Add Canonical Analysis and Results Routes

**Files:**
- Create: `web/app/projects/[id]/analysis/page.tsx`
- Create: `web/app/projects/[id]/results/page.tsx`
- Modify: `web/app/projects/[id]/opportunities/page.tsx`
- Modify: `web/app/projects/[id]/visibility/page.tsx`
- Modify: `web/app/projects/[id]/seo/page.tsx`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

- [x] **Step 1: Export renamed client wrappers**

In `web/app/projects/[id]/seo/seo-client.tsx`, change the mode type and wrappers to:

```tsx
type SEOClientMode = "analysis" | "results";

export function AnalysisClient({ projectId }: { projectId: string }) {
  return <SEOClient projectId={projectId} mode="analysis" />;
}

export function ResultsClient({ projectId }: { projectId: string }) {
  return <SEOClient projectId={projectId} mode="results" />;
}
```

- [x] **Step 2: Replace mode checks**

In the same file, replace `mode === "opportunities"` with `mode === "analysis"` and `mode === "visibility"` with `mode === "results"`.

- [x] **Step 3: Update SEO surface copy**

Use these user-facing labels:

```tsx
title={mode === "analysis" ? "Review analysis" : "Results"}
eyebrow={mode === "analysis" ? "Analyze opportunities" : "Measure impact"}
```

Update the Analysis summary copy from:

```text
Choose the opportunities worth turning into content work.
What to review
Review result
```

to:

```text
Choose the recommendations worth turning into content work.
What to decide
Decision result
```

- [x] **Step 4: Create canonical route wrappers**

Create `web/app/projects/[id]/analysis/page.tsx`:

```tsx
import { AnalysisClient } from "../seo/seo-client";

export default async function AnalysisPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <AnalysisClient projectId={id} />;
}
```

Create `web/app/projects/[id]/results/page.tsx`:

```tsx
import { ResultsClient } from "../seo/seo-client";

export default async function ResultsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <ResultsClient projectId={id} />;
}
```

- [x] **Step 5: Redirect legacy routes**

Change `web/app/projects/[id]/opportunities/page.tsx` to:

```tsx
import { redirect } from "next/navigation";

export default async function OpportunitiesPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  redirect(`/projects/${id}/analysis`);
}
```

Change `web/app/projects/[id]/visibility/page.tsx` to:

```tsx
import { redirect } from "next/navigation";

export default async function VisibilityPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  redirect(`/projects/${id}/results`);
}
```

Change `web/app/projects/[id]/seo/page.tsx` to redirect to `/results`.

- [x] **Step 6: Run the focused contract test**

Run:

```bash
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: still FAIL until sidebar and link references are updated.

## Task 3: Update Project Shell IA

**Files:**
- Modify: `web/app/components/project-shell.tsx`

- [x] **Step 1: Change main nav sections**

Replace `navSections` with:

```tsx
const navSections = [
  {
    id: "primary",
    label: null,
    items: [
      { label: "Home", href: "", icon: Home },
      { label: "Context", href: "context", icon: Database },
    ],
  },
  {
    id: "analyze",
    label: "ANALYZE",
    items: [{ label: "Analysis", href: "analysis", icon: Target }],
  },
  {
    id: "create",
    label: "CREATE",
    items: [
      { label: "Content Plan", href: "plan", icon: ListChecks },
      { label: "Review", href: "review", icon: PenLine },
    ],
  },
  {
    id: "deliver",
    label: "DELIVER",
    items: [{ label: "Publish", href: "publish", icon: Send }],
  },
  {
    id: "measure",
    label: "MEASURE",
    items: [{ label: "Results", href: "results", icon: Search }],
  },
];
```

- [x] **Step 2: Make Settings a bottom utility entry**

Keep `Admin` in the existing `isPlatformAdmin` footer block. Add Settings below Docs:

```tsx
{canAccessSettings && (
  <Link href={`/projects/${projectId}/settings`} ...>
    <Settings2 size={16} />
    Settings
  </Link>
)}
```

- [x] **Step 3: Remove settings/admin from `adminOnlyNavLeaves`**

Set:

```tsx
const adminOnlyNavLeaves = new Set<string>([]);
```

or remove the filter entirely if tests are updated accordingly. Keep the server-side `canAccessSettings` prop for the footer Settings link.

- [x] **Step 4: Add Settings to mobile utility chips**

After Docs in the mobile nav, render a Settings chip when `canAccessSettings` is true.

- [x] **Step 5: Run the focused contract test**

Run:

```bash
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: route/sidebar assertions should now pass; remaining failures should point to Home, Content Plan, or Docs copy/links.

## Task 4: Update Home, Content Plan, and Docs Links

**Files:**
- Modify: `web/app/lib/dashboard-ux-logic.ts`
- Modify: `web/app/projects/[id]/workspace.tsx`
- Modify: `web/app/projects/[id]/topics/topics-client.tsx`
- Modify: `web/app/docs/page.tsx`

- [x] **Step 1: Update Home action routing**

In `web/app/lib/dashboard-ux-logic.ts`, replace route targets:

```ts
`/projects/${projectId}/opportunities` -> `/projects/${projectId}/analysis`
`/projects/${input.projectId}/opportunities` -> `/projects/${input.projectId}/analysis`
`/projects/${input.projectId}/visibility` -> `/projects/${input.projectId}/results`
```

Update user-facing copy:

```text
Review opportunities -> Review analysis
visibility gaps entered the loop -> analysis work entered the loop
live assets feeding visibility -> live assets feeding results
```

- [x] **Step 2: Update Home pipeline labels and links**

In `web/app/projects/[id]/workspace.tsx`, change:

```tsx
label: "Opportunities" -> label: "Analysis"
href: `/projects/${projectId}/opportunities` -> href: `/projects/${projectId}/analysis`
label: "Measure" -> label: "Results"
href: `/projects/${projectId}/visibility` -> href: `/projects/${projectId}/results`
```

Update event and needs-you labels:

```text
Visibility opportunity detected -> Analysis opportunity detected
Opportunities to review -> Analysis to review
```

- [x] **Step 3: Update Content Plan CTAs**

In `web/app/projects/[id]/topics/topics-client.tsx`, update `/opportunities` links to `/analysis` and copy:

```text
Review opportunities -> Review analysis
opportunities are waiting for review -> analysis recommendations are waiting for review
```

- [x] **Step 4: Update Docs copy**

In `web/app/docs/page.tsx`, update docs terms and page labels:

```text
Opportunities -> Analysis
Visibility -> Results
Measure visibility -> Measure results
Settings > Activity Log remains unchanged.
```

- [x] **Step 5: Run focused contract and docs tests**

Run:

```bash
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs app/lib/docs-contract.test.mjs
```

Expected: PASS.

## Task 5: Full Verification and Commit

**Files:**
- All files changed above.

- [x] **Step 1: Run web checks**

Run:

```bash
npm test
npm run typecheck
```

Expected: both commands exit 0.

- [x] **Step 2: Run Go checks**

Run:

```bash
make test
```

Expected: exits 0.

- [x] **Step 3: Inspect diff**

Run:

```bash
git diff --check
git status --short
```

Expected: no whitespace errors; only intended files are modified.

- [x] **Step 4: Commit Phase 1**

Run:

```bash
git add docs/superpowers/plans/2026-06-24-analysis-workflow-phase1-ia.md \
  web/app/lib/dashboard-ux-phase1-contract.test.mjs \
  web/app/components/project-shell.tsx \
  web/app/projects/[id]/seo/seo-client.tsx \
  web/app/projects/[id]/analysis/page.tsx \
  web/app/projects/[id]/results/page.tsx \
  web/app/projects/[id]/opportunities/page.tsx \
  web/app/projects/[id]/visibility/page.tsx \
  web/app/projects/[id]/seo/page.tsx \
  web/app/lib/dashboard-ux-logic.ts \
  web/app/projects/[id]/workspace.tsx \
  web/app/projects/[id]/topics/topics-client.tsx \
  web/app/docs/page.tsx
git commit -m "feat: add analysis workflow phase 1 IA"
```

Expected: commit succeeds.

## Self-Review

- Spec coverage: This plan covers PRD Phase 1 IA, canonical `/analysis`, legacy redirects, `/results`, Settings footer utility placement, Admin footer placement, and action-first link/copy updates. It intentionally does not implement GSC OAuth, Analysis productization, Results cleanup, or CMS connectors because those start in later phases.
- Placeholder scan: No `TBD`, `TODO`, or unspecified "add tests" steps remain.
- Type consistency: The only new mode names are `"analysis"` and `"results"`, and route wrappers import `AnalysisClient` / `ResultsClient` from the existing SEO client module.
