# Dashboard Control Center Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the Home dashboard into a compact control center that answers status, next action, and recent/next activity without long default scrolling.

**Architecture:** Keep Phase 1 scoped to the existing Next.js dashboard surface. Add small pure UX helper functions in `web/app/lib/dashboard-ux-logic.ts`, cover them with Node tests, then wire the existing `workspace.tsx` and `project-shell.tsx` to those helpers without introducing a new design system.

**Tech Stack:** Next.js App Router, React client components, Tailwind CSS 3, lucide-react, Node test runner.

---

## File Map

- Modify `web/app/lib/dashboard-ux-logic.ts`: add pure helpers for actionable momentum, Home event stream, steady-state section budgeting, and sidebar primary action.
- Modify `web/app/lib/dashboard-ux-logic.test.mjs`: add focused behavior tests for the new helpers.
- Modify `web/app/lib/dashboard-ux-phase1-contract.test.mjs`: update contract assertions so Phase 1 protects the compact Home vocabulary.
- Modify `web/app/projects/[id]/workspace.tsx`: replace always-visible empty modules with priority-gated control-center sections.
- Modify `web/app/components/project-shell.tsx`: replace fixed `Review queue` CTA with the current primary action.
- Modify `web/app/page.tsx` and `web/app/project-create-form.tsx`: align onboarding copy with the new control-center mental model.
- Add `docs/PRD-CiteLoop-Dashboard-Control-Center-Redesign.md`: PRD baseline for the full phased program.

## Phase 1 Acceptance Rules

- Home default state shows a primary action, actionable momentum, event stream, and at most two active follow-up modules.
- Zero-value KPI tiles are hidden; healthy empty states explain the next useful action instead of showing four zeros.
- Event stream uses time-oriented language: live activity, recent events, and next scheduled item.
- Additional active modules are summarized in a compact `More waiting` strip instead of expanding the page.
- Sidebar CTA reflects the highest-priority action available to the user.
- `qa blocking` copy remains specific and must not collapse to `Needs evidence`.

## Tasks

### Task 1: Baseline And Red Tests

- [x] **Step 1: Run current focused baseline**

Run:

```bash
cd /Users/xiaoboyu/.config/superpowers/worktrees/citeloop/review-ux-main/web
npm test -- app/lib/dashboard-ux-logic.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: existing tests pass before Phase 1 edits.

- [x] **Step 2: Add failing pure-helper tests**

Add tests to `web/app/lib/dashboard-ux-logic.test.mjs` for:

```js
buildActionableMomentum({
  projectId: "p1",
  hasProfile: true,
  publishedThisMonthCount: 0,
  approvedDraftCount: 0,
  opportunitiesConvertedCount: 0,
  readyToDistributeCount: 0,
  activeLoopItemCount: 0,
})
```

Expected behavior: returns no metric items and an empty action pointing to `/projects/p1/plan`.

Add tests for non-zero values:

```js
buildActionableMomentum({
  projectId: "p1",
  hasProfile: true,
  publishedThisMonthCount: 3,
  approvedDraftCount: 0,
  opportunitiesConvertedCount: 2,
  readyToDistributeCount: 1,
  activeLoopItemCount: 4,
})
```

Expected behavior: includes only non-zero actionable items, each with `href` and `actionLabel`.

Add tests for `buildHomeEventStream`, `visibleHomeSectionIds`, and `sidebarPrimaryAction`.

- [x] **Step 3: Run helper tests and verify RED**

Run:

```bash
cd /Users/xiaoboyu/.config/superpowers/worktrees/citeloop/review-ux-main/web
npm test -- app/lib/dashboard-ux-logic.test.mjs
```

Expected: FAIL because the helper exports do not exist yet.

- [x] **Step 4: Update contract tests and verify RED**

Update `web/app/lib/dashboard-ux-phase1-contract.test.mjs` to assert:

```js
source.includes("Actionable momentum")
source.includes("Event stream")
source.includes("More waiting")
!source.includes("Results / Momentum")
!source.includes("Loop progress")
source.includes("Cannot approve:")
!source.includes("Needs evidence")
```

Run:

```bash
cd /Users/xiaoboyu/.config/superpowers/worktrees/citeloop/review-ux-main/web
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: FAIL until the UI copy is updated.

### Task 2: Pure UX Logic

- [x] **Step 1: Implement helper types and functions**

In `web/app/lib/dashboard-ux-logic.ts`, add:

```ts
export type ActionableMomentumInput = {
  projectId: string;
  hasProfile: boolean;
  publishedThisMonthCount: number;
  approvedDraftCount: number;
  opportunitiesConvertedCount: number;
  readyToDistributeCount: number;
  activeLoopItemCount: number;
};

export function buildActionableMomentum(input: ActionableMomentumInput) {
  // Only non-zero metrics become cards. Empty state returns the next setup action.
}
```

Add `buildHomeEventStream`, `visibleHomeSectionIds`, and `sidebarPrimaryAction` with deterministic ordering and no network or React dependency.

- [x] **Step 2: Run helper tests and verify GREEN**

Run:

```bash
cd /Users/xiaoboyu/.config/superpowers/worktrees/citeloop/review-ux-main/web
npm test -- app/lib/dashboard-ux-logic.test.mjs
```

Expected: PASS.

### Task 3: Home Control Center UI

- [x] **Step 1: Replace zero KPI section**

In `web/app/projects/[id]/workspace.tsx`, replace `Results / Momentum` and `momentumItems` with `Actionable momentum` rendered from `buildActionableMomentum`.

- [x] **Step 2: Replace lifecycle pipe on Home**

In `web/app/projects/[id]/workspace.tsx`, replace `Loop progress` with `Event stream` rendered from `buildHomeEventStream`.

- [x] **Step 3: Gate steady-state modules**

Build a local array of active modules and render only the first two from `visibleHomeSectionIds`; render the rest as `More waiting`.

- [x] **Step 4: Run contract tests and verify GREEN**

Run:

```bash
cd /Users/xiaoboyu/.config/superpowers/worktrees/citeloop/review-ux-main/web
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: PASS.

### Task 4: Sidebar CTA And Onboarding Copy

- [x] **Step 1: Wire sidebar primary action**

In `web/app/components/project-shell.tsx`, use `sidebarPrimaryAction` so the pinned CTA is no longer always `Review queue`.

- [x] **Step 2: Update onboarding copy**

Change public onboarding copy from generic service-console language to CiteLoop control-center language in `web/app/page.tsx` and `web/app/project-create-form.tsx`.

- [x] **Step 3: Run focused tests**

Run:

```bash
cd /Users/xiaoboyu/.config/superpowers/worktrees/citeloop/review-ux-main/web
npm test -- app/lib/dashboard-ux-logic.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: PASS.

### Task 5: Full Verification, Push, Online Check

- [x] **Step 1: Run full local verification**

Run:

```bash
cd /Users/xiaoboyu/.config/superpowers/worktrees/citeloop/review-ux-main/web
npm test
npm run typecheck
npm run build
```

Expected: commands pass, or any environment-related build blocker is captured with exact output.

- [x] **Step 2: Inspect git diff**

Run:

```bash
cd /Users/xiaoboyu/.config/superpowers/worktrees/citeloop/review-ux-main
git diff --stat
git diff -- web/app/lib/dashboard-ux-logic.ts web/app/projects/[id]/workspace.tsx web/app/components/project-shell.tsx
```

Expected: diff matches Phase 1 scope.

- [ ] **Step 3: Commit and push to main**

Run:

```bash
cd /Users/xiaoboyu/.config/superpowers/worktrees/citeloop/review-ux-main
git add docs/PRD-CiteLoop-Dashboard-Control-Center-Redesign.md docs/superpowers/plans/2026-06-10-dashboard-control-center-phase1.md web/app
git commit -m "feat: reshape dashboard home control center"
git push origin main
```

Expected: push succeeds.

- [ ] **Step 4: Online verification**

Use Vercel deployment tools or the deployment URL from the push result to verify the production dashboard loads and the Home surface shows the new control-center vocabulary.
