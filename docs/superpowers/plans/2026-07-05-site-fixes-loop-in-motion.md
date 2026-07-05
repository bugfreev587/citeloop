# Site Fixes Loop In Motion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Site Fixes visibly participate in Loop in motion without changing backend APIs or action routing.

**Architecture:** Use the existing merged `loopActions` source in `web/app/projects/[id]/seo/seo-client.tsx`. Add small presentation helpers that derive a destination label from each action and render that label in loop preview cards. Keep lifecycle counting shared through `visibilityLifecycleCounts`.

**Tech Stack:** Next.js App Router, React client component, Tailwind CSS, existing `Badge` component, Node `node:test` contract tests.

---

### Task 1: Contract Test

**Files:**
- Modify: `web/app/lib/seo-client-contract.test.mjs`

- [ ] **Step 1: Write the failing test**

Add this test near the existing Analysis Site Fixes tests:

```javascript
test("Analysis Loop in motion makes Site Fixes visible inside the lifecycle", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");

  const loopStart = source.indexOf("data-analysis-loop-strip");
  const loopEnd = source.indexOf("Finish automation setup in Settings", loopStart);
  const loopBlock = source.slice(loopStart, loopEnd);

  assert.notEqual(loopStart, -1, "seo-client.tsx missing Loop in motion section");
  assert.match(source, /function loopActionDestinationLabel/);
  assert.match(source, /function loopLifecycleSummaryLabel/);
  assert.match(loopBlock, /Published \\/ Applied/);
  assert.match(loopBlock, /loopActionDestinationLabel\\(action\\)/);
  assert.match(loopBlock, /Site Fixes/);
  assert.match(loopBlock, /Content Plan/);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd web
npm test -- app/lib/seo-client-contract.test.mjs
```

Expected: FAIL because `loopActionDestinationLabel`, `loopLifecycleSummaryLabel`, and `Published / Applied` do not exist yet.

### Task 2: Loop Presentation Helpers

**Files:**
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Test: `web/app/lib/seo-client-contract.test.mjs`

- [ ] **Step 1: Implement minimal helper functions**

Add these helpers near the existing lifecycle label helpers:

```typescript
function loopLifecycleSummaryLabel(stage: string) {
  if (stage === "published_or_applied") return "Published / Applied";
  return lifecycleStageLabel(stage);
}

function loopActionDestinationLabel(action: SEOContentAction | ResultsAction) {
  return destinationForAction(action);
}
```

- [ ] **Step 2: Render the helpers in Loop in motion**

Change `loopSummaryItems` to use `loopLifecycleSummaryLabel("published_or_applied")` for the published/applied stage label, and render a neutral badge in each loop preview card:

```tsx
<Badge tone="neutral">{loopActionDestinationLabel(action)}</Badge>
```

Keep the existing lifecycle badge beside it so users can see both destination and stage.

- [ ] **Step 3: Run focused test to verify it passes**

Run:

```bash
cd web
npm test -- app/lib/seo-client-contract.test.mjs
```

Expected: PASS.

### Task 3: Full Verification

**Files:**
- Verify all changed files.

- [ ] **Step 1: Run full frontend tests**

Run:

```bash
cd web
npm test
```

Expected: 0 failures.

- [ ] **Step 2: Run typecheck**

Run:

```bash
cd web
npm run typecheck
```

Expected: TypeScript exits 0.

- [ ] **Step 3: Review diff and commit**

Run:

```bash
git diff --check
git status --short
git add docs/superpowers/specs/2026-07-05-site-fixes-loop-in-motion-design.md docs/superpowers/plans/2026-07-05-site-fixes-loop-in-motion.md web/app/lib/seo-client-contract.test.mjs web/app/projects/[id]/seo/seo-client.tsx
git commit -m "feat: show site fixes in loop motion"
```

Expected: commit succeeds with only the intended files staged.
