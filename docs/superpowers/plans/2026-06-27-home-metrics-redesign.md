# Home Metrics Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the project Home hero with a linked, first-viewport Metrics dashboard that shows current values and honest changes, then optional other projects, then Pipeline and Activity.

**Architecture:** Keep the change in `web/app/projects/[id]/workspace.tsx`, which already owns Home data loading and derived state. Add account project loading through the existing `api.listProjects()` client method and update the existing contract test in `web/app/lib/dashboard-ux-phase1-contract.test.mjs`.

**Tech Stack:** Next.js App Router, React client component, TypeScript, Tailwind CSS, lucide-react, Node test runner.

---

### Task 1: Lock the Home Contract

**Files:**
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Write the failing contract test**

Add assertions that `workspace.tsx` includes `accountProjects`, `otherProjects`, `metricGridCards`, `metricChangeLabel`, metric `href` values, and omits `growthHeadline`, `growthDetail`, `Your next step`, and the top refresh button.

- [ ] **Step 2: Run the focused test**

Run: `npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: FAIL because `workspace.tsx` still renders the old hero/next-step area and does not have the new metric/project identifiers.

### Task 2: Implement the Metrics-First Home

**Files:**
- Modify: `web/app/projects/[id]/workspace.tsx`

- [ ] **Step 1: Add account project state**

Add `accountProjects` state and include `api.listProjects().catch(() => [])` in the `refresh()` `Promise.all` batch. Store it with `setAccountProjects(projectRows)`.

- [ ] **Step 2: Replace hero-derived state with metric-derived state**

Remove `growthHeadline`, `growthDetail`, and normal-page rendering of `primaryAction`. Keep `primaryAction` for pipeline highlight logic. Create `metricGridCards` entries with `href`, `label`, `value`, `detail`, `metricChangeLabel`, `metricChangeTone`, `icon`, `featured`, and `muted`.

- [ ] **Step 3: Render linked metric cards**

Render a first section with a CSS grid. The featured Organic traffic card spans the wider area on large screens. Each card is an `<a>` with a stable height, icon, current value, detail, change/status label, and a small ArrowRight affordance.

- [ ] **Step 4: Render other projects conditionally**

Compute `otherProjects = accountProjects.filter((candidate) => candidate.id !== projectId)`. Render an "Other projects" section only when `otherProjects.length > 0`; each project links to `/projects/${candidate.id}`.

- [ ] **Step 5: Preserve existing lower sections**

Keep context-build progress, Pipeline, Needs you, Activity, and review preview behavior. Position context-build progress after Metrics so onboarding status remains visible without becoming the top hero.

- [ ] **Step 6: Run focused and full verification**

Run:

```bash
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
npm test
npm run typecheck
npm run build
```

Expected: all commands pass.

### Task 3: Visual Verification and Delivery

**Files:**
- No planned source changes unless verification finds layout issues.

- [ ] **Step 1: Start the local web app**

Run: `npm run dev`

Expected: Next.js starts on port 3000 or reports another usable local URL.

- [ ] **Step 2: Inspect Home in a browser**

Open a project Home route, confirm the hero and refresh-context card are gone, metric cards are first, each metric card has a destination, other projects only appears with more than one project, and Pipeline and Activity remain below.

- [ ] **Step 3: Commit and publish**

Run:

```bash
git add docs/superpowers/specs/2026-06-27-home-metrics-redesign-design.md docs/superpowers/plans/2026-06-27-home-metrics-redesign.md web/app/lib/dashboard-ux-phase1-contract.test.mjs web/app/projects/[id]/workspace.tsx
git commit -m "feat(home): make metrics the primary dashboard"
git push -u origin codex/home-metrics-redesign
```

Expected: commit and push succeed.

- [ ] **Step 4: PR, merge, deploy, and production check**

Create a PR to `origin/main`, merge it after checks pass, wait for deployment, then verify production Home matches the requested layout.
