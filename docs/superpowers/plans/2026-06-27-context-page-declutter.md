# Context Page Declutter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Context page feel like a focused confirmation and evidence center instead of a long, flat knowledge inventory page.

**Architecture:** Keep the existing `ContextClient` data loading, save, drawer, and toast behavior. Rework the render structure so empty setup, first-run confirmation, confirmed summaries, evidence review, source diagnostics, and advanced JSON have clearer hierarchy without adding dependencies or changing backend contracts.

**Tech Stack:** Next.js App Router, React client component, Tailwind CSS v3, Node contract tests.

---

### Task 1: Contract-Test the New Information Hierarchy

**Files:**
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Write the failing test**

Add assertions to the existing Context contract test:

```js
for (const copy of [
  "Set up Context",
  "What CiteLoop checks",
  "Product understanding",
  "Writing boundaries",
  "View source pages",
  "Source coverage",
]) {
  assert.match(context, new RegExp(copy));
}

assert.doesNotMatch(context, /No voice rules yet/);
assert.doesNotMatch(context, /<SectionHeader title="Source pages"/);
```

- [ ] **Step 2: Run test to verify it fails**

Run: `node --test app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: FAIL because the current component does not render the new setup panel, source coverage entry, or grouped summaries.

### Task 2: Rework ContextClient Layout

**Files:**
- Modify: `web/app/projects/[id]/knowledge/knowledge-client.tsx`

- [ ] **Step 1: Implement minimal render changes**

Add small presentational helpers inside `knowledge-client.tsx`: `SetupPanel`, `ContextHealthPanel`, `SummaryGroup`, and `SourceCoveragePanel`. Each helper accepts only display data and callbacks already owned by `ContextClient`; no helper fetches data or creates new state.

Then update `ContextClient` so:

- no profile renders one setup panel and exits before Evidence / Domain profile / Voice / Source pages;
- the header health panel has one primary next action and a compact `What CiteLoop knows` summary;
- Domain profile and Voice are grouped as `Product understanding` and `Writing boundaries`;
- Source pages move behind a `Source coverage` secondary panel and drawer trigger;
- `Advanced JSON` remains collapsed.

- [ ] **Step 2: Run contract test**

Run: `node --test app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: PASS.

### Task 3: Verify Build Health

**Files:**
- No new files.

- [ ] **Step 1: Run focused checks**

Run:

```bash
npm run typecheck
node --test app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: both commands exit 0.

- [ ] **Step 2: Commit**

Run:

```bash
git add docs/superpowers/plans/2026-06-27-context-page-declutter.md web/app/lib/dashboard-ux-phase1-contract.test.mjs web/app/projects/[id]/knowledge/knowledge-client.tsx
git commit -m "feat(context): declutter context page hierarchy"
```
