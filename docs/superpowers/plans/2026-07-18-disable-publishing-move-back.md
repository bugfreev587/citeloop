# Disable Publishing Move-Back Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent `Move back to Opportunities` from being triggered while a canonical article is actively publishing.

**Architecture:** Keep publishing lifecycle derivation unchanged and enforce the temporary interaction guard in `ReadyNowStrip`, the component that already receives each card's `item.action`. Extend the existing source contract test to prove the move-back button combines the global mutation lock with the publishing-state lock.

**Tech Stack:** Next.js 15, React 18, TypeScript, Node.js built-in test runner

---

### Task 1: Guard the Publish move-back control

**Files:**
- Modify: `web/app/lib/workflow-handoff-link-cards-contract.test.mjs`
- Modify: `web/app/projects/[id]/publishing/publishing-client.tsx`

- [ ] **Step 1: Write the failing contract test**

Add this focused test to `web/app/lib/workflow-handoff-link-cards-contract.test.mjs`:

```js
test("Publish disables Move back to Opportunities while actively publishing", async () => {
  const source = await read("projects/[id]/publishing/publishing-client.tsx");
  const readyStart = source.indexOf("function ReadyNowStrip");
  const readyEnd = source.indexOf("function SEODetailTile", readyStart);
  const readyBlock = source.slice(readyStart, readyEnd);
  const moveLabelIndex = readyBlock.indexOf("Move back to Opportunities");
  const moveButtonStart = readyBlock.lastIndexOf("<Button", moveLabelIndex);
  const moveButtonEnd = readyBlock.indexOf("</Button>", moveLabelIndex);
  const moveButton = readyBlock.slice(moveButtonStart, moveButtonEnd);

  assert.ok(readyBlock.length > 0, "ReadyNowStrip must exist");
  assert.ok(moveLabelIndex >= 0, "ReadyNowStrip must render the move-back control");
  assert.match(
    moveButton,
    /disabled=\{Boolean\(busy\) \|\| item\.action === "publishing"\}/,
    "publishing cards must not allow a workflow rollback while the publisher is active",
  );
  assert.match(moveButton, /onClick=\{\(\) => onMoveBack\(item\.article\)\}/);
});
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
node --test --test-name-pattern="Publish disables Move back" app/lib/workflow-handoff-link-cards-contract.test.mjs
```

from `web/`.

Expected: FAIL because the current button only contains `disabled={Boolean(busy)}`.

- [ ] **Step 3: Implement the minimal interaction guard**

In `ReadyNowStrip`, change only the move-back button's disabled expression:

```tsx
disabled={Boolean(busy) || item.action === "publishing"}
```

Keep the existing visibility condition, click handler, accessible label, progress state, copy, layout, and backend API unchanged.

- [ ] **Step 4: Run the focused test and verify GREEN**

Run:

```bash
node --test --test-name-pattern="Publish disables Move back" app/lib/workflow-handoff-link-cards-contract.test.mjs
```

from `web/`.

Expected: PASS with the new test selected and all other tests in that file skipped.

- [ ] **Step 5: Run frontend verification**

Run from `web/`:

```bash
npm test
npm run typecheck
npm run build
```

Expected: 0 test failures, 0 TypeScript errors, and a successful production build.

- [ ] **Step 6: Review and commit the implementation**

Run:

```bash
git diff --check
git diff -- web/app/lib/workflow-handoff-link-cards-contract.test.mjs 'web/app/projects/[id]/publishing/publishing-client.tsx'
git add web/app/lib/workflow-handoff-link-cards-contract.test.mjs 'web/app/projects/[id]/publishing/publishing-client.tsx'
git commit -m "fix: disable publish move-back while publishing"
```

Expected: one focused test change and one disabled-condition change committed on the task branch.

### Task 2: Deliver and verify production

**Files:**
- No source files expected unless production verification finds a gap.

- [ ] **Step 1: Push and open the pull request**

Run:

```bash
git push -u origin codex/disable-publishing-move-back
gh pr create \
  --base main \
  --head codex/disable-publishing-move-back \
  --title "Disable publish move-back while publishing" \
  --body "## Summary
- keep Move back to Opportunities visible but disabled during Publishing
- add focused regression coverage for the interaction guard

## Verification
- npm test
- npm run typecheck
- npm run build"
```

Expected: a PR targeting `origin/main` containing the design, plan, regression test, and component guard.

- [ ] **Step 2: Wait for required checks and merge**

Run:

```bash
gh pr checks --watch
gh pr merge --squash --delete-branch
```

Expected: required checks pass and the PR merges into `main`.

- [ ] **Step 3: Wait for production deployment**

Inspect the deployment associated with the merged commit using the repository's configured deployment provider and wait until it reaches a successful terminal state.

Expected: production serves the merged commit.

- [ ] **Step 4: Verify production behavior**

Open a Publish card whose status is `Publishing` and confirm:

1. `Move back to Opportunities` remains visible.
2. The button is visibly disabled.
3. Pointer and keyboard interaction cannot trigger the return request.
4. The card's Preview, SEO Details, Destination, and Publishing status controls remain unchanged.

If a failed pre-publication card is available, confirm its move-back control is enabled when the source action remains returnable.

- [ ] **Step 5: Repair any production gap**

If production behavior differs from the acceptance criteria, reproduce the gap, add a failing regression test, implement the smallest correction, rerun frontend verification, push the fix, wait for deployment, and repeat production verification.
