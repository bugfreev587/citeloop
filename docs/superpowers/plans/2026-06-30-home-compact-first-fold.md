# Home Compact First Fold Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Compress the Home metrics board and Needs you queue so Pipeline is visible in the first desktop viewport without scrolling.

**Architecture:** Keep the existing `Workspace` client component and data derivation intact. Change only Home presentation classes and supporting contract tests so the overview becomes a compact dashboard summary while manual actions stay discoverable.

**Tech Stack:** Next.js App Router, React 18, Tailwind CSS 3, Node `node:test` contract tests.

---

### Task 1: Contract The Compact First Fold

**Files:**
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
- Read: `web/app/projects/[id]/workspace.tsx`

- [x] **Step 1: Write the failing test**

Add this contract test near the existing Home tests:

```js
test("home keeps Pipeline in the first desktop fold with compact metrics and actions", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const contract of [
    "compactMetricCardClass",
    "compactActionTileClass",
    "lg:grid-cols-4",
    "lg:min-h-[148px]",
    "lg:min-h-[118px]",
    "text-3xl",
    "line-clamp-2",
    "First-fold pipeline",
  ]) {
    assert.match(workspace, new RegExp(contract.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
  }

  assert.doesNotMatch(workspace, /lg:row-span-2/);
  assert.doesNotMatch(workspace, /md:text-6xl/);
  assert.doesNotMatch(workspace, /sm:aspect-\[1\.05\/1\]/);
});
```

- [x] **Step 2: Run test to verify it fails**

Run:

```bash
cd web && npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: FAIL because `compactMetricCardClass`, `compactActionTileClass`, and `First-fold pipeline` do not exist yet.

### Task 2: Compact Metrics And Needs You

**Files:**
- Modify: `web/app/projects/[id]/workspace.tsx`
- Test: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [x] **Step 1: Add compact helper classes**

Add helper functions near the other Home styling helpers:

```tsx
function compactMetricCardClass(featured: boolean) {
  return cx(
    "group flex min-h-[124px] flex-col rounded-xl border border-slate-200 bg-white p-4 transition-colors hover:border-slate-300 hover:bg-slate-50 lg:min-h-[148px]",
    featured && "lg:col-span-1",
  );
}

function compactActionTileClass(tone: StageTone) {
  return cx(
    "group flex min-h-[116px] flex-col justify-between overflow-hidden rounded-xl border border-slate-200 p-3 shadow-[0_14px_30px_-26px_rgba(15,23,42,0.45)] transition-all duration-200 hover:-translate-y-0.5 active:translate-y-0 lg:min-h-[118px]",
    humanActionTileToneClass(tone),
  );
}
```

- [x] **Step 2: Update metrics board markup**

Change the metrics board section to a four-card compact grid:

```tsx
<section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
```

Use `compactMetricCardClass(item.featured)` for each metric card, remove `lg:row-span-2`, and render metric values at compact sizes:

```tsx
<div className={cx("mt-3 font-bold leading-none tracking-tight", item.featured ? "text-3xl" : "text-2xl", item.muted ? "text-slate-400" : "text-slate-950")}>
```

- [x] **Step 3: Update Needs you tiles**

Use `compactActionTileClass(item.tone)`, remove `sm:aspect-[1.05/1]`, reduce icon to `h-8 w-8`, reduce internal margins, and keep descriptions to `line-clamp-2`.

- [x] **Step 4: Add Pipeline anchor comment**

Change the Pipeline comment to:

```tsx
{/* First-fold pipeline — the flywheel as a connected progress spine */}
```

- [x] **Step 5: Run the focused test**

Run:

```bash
cd web && npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: PASS.

### Task 3: Verify And Ship

**Files:**
- Modified files from Tasks 1 and 2

- [x] **Step 1: Run full local verification**

Run:

```bash
cd web && npm test
cd web && npm run typecheck
make test
cd web && npm run build
git diff --check
```

Expected: all commands exit 0. `npm run build` may keep the existing multiple-lockfile warning.

- [ ] **Step 2: Commit and open PR**

Run:

```bash
git add docs/superpowers/plans/2026-06-30-home-compact-first-fold.md web/app/lib/dashboard-ux-phase1-contract.test.mjs 'web/app/projects/[id]/workspace.tsx'
git commit -m "feat(home): compact first fold"
git push -u origin codex/home-compact-first-fold
gh pr create --base main --head codex/home-compact-first-fold --title "feat(home): compact first fold" --body "..."
```

- [ ] **Step 3: Merge after checks and verify production**

Wait for GitHub and Vercel checks, merge the PR, confirm the production deployment commit, then verify `https://citeloop.app/projects/1459b054-cdc3-4d9b-9dd4-18e12458c61a` shows compact metrics, compact Needs you cards, and Pipeline in the first viewport.
