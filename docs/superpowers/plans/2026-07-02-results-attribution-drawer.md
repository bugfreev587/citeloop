# Results Attribution Drawer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert Results action-level attribution into compact cards that open a right-side drawer with the full attribution report.

**Architecture:** Keep all UI state local to `SEOClient` because Results and Analysis already share that component. Add a selected Results action id plus drawer refs parallel to the existing Analysis drawer refs. Preserve all existing attribution helper functions and move the existing expanded row detail markup into a conditional drawer.

**Tech Stack:** Next.js App Router, React client component state/effects, Tailwind CSS, Node contract tests.

---

### Task 1: Contract Test

**Files:**
- Modify: `web/app/lib/results-attribution-contract.test.mjs`
- Test: `web/app/lib/results-attribution-contract.test.mjs`

- [ ] **Step 1: Write the failing test**

Add assertions to the existing Results attribution tests for these exact source markers:

```js
"selectedResultActionID"
"resultDrawerRef"
"resultReturnFocusRef"
"data-results-action-card"
"data-results-drawer"
"aria-label={`Open attribution details: ${action.action_type}`}"
"Measurement details"
"Manual verify"
"Verification failed"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npm test -- app/lib/results-attribution-contract.test.mjs`

Expected: FAIL because `selectedResultActionID`, `resultDrawerRef`, `data-results-action-card`, and `data-results-drawer` are not present yet.

### Task 2: Results Drawer Implementation

**Files:**
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Test: `web/app/lib/results-attribution-contract.test.mjs`

- [ ] **Step 1: Add Results drawer state**

Add `selectedResultActionID`, `resultsSurfaceRef`, `resultDrawerRef`, and `resultReturnFocusRef` beside the existing Analysis drawer state. Derive `selectedResultAction` from `attributionActions`.

- [ ] **Step 2: Add shared drawer effects**

Extend the current Escape, focus trap, body scroll lock, inert surface, and focus restore effects so they work for either `selectedOpportunity` or `selectedResultAction`.

- [ ] **Step 3: Replace expanded action rows with compact card triggers**

Render each attribution action as a `<button type="button" data-results-action-card>` with badges, title, target URL, published date, measurement window, and outcome reason. Set `resultReturnFocusRef.current` and `selectedResultActionID` on click.

- [ ] **Step 4: Add the Results drawer**

Render `{mode === "results" && selectedResultAction && (...)}` after the main surface. Use the same `citeloop-drawer-scrim-in` and `citeloop-drawer-panel-in` classes as Analysis. Add `data-results-drawer`, close button, detail fields, measurement details, and the existing manual verification buttons.

- [ ] **Step 5: Run focused test to verify it passes**

Run: `cd web && npm test -- app/lib/results-attribution-contract.test.mjs`

Expected: PASS for all Results attribution contract tests.

### Task 3: Verification

**Files:**
- Verify only.

- [ ] **Step 1: Run relevant web tests**

Run: `cd web && npm test -- app/lib/results-attribution-contract.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs app/lib/seo-client-contract.test.mjs`

Expected: PASS.

- [ ] **Step 2: Run typecheck**

Run: `cd web && npm run typecheck`

Expected: PASS.

- [ ] **Step 3: Inspect diff**

Run: `git diff -- web/app/lib/results-attribution-contract.test.mjs web/app/projects/[id]/seo/seo-client.tsx`

Expected: Diff only contains the contract test and Results drawer UI changes.
