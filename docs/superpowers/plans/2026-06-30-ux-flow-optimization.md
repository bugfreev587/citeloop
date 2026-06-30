# UX Flow Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tighten the operator flow for Content Plan, Review, and Publish so each page leads with the user's next decision instead of neutral system inventory.

**Architecture:** Keep the current Next.js client-component structure and existing route model. Add small presentational helpers inside the existing feature modules, and cover behavior with the existing string/contract tests plus focused logic tests.

**Tech Stack:** Next.js App Router, React 18, Tailwind CSS v3, lucide-react, Node `node:test`, TypeScript.

---

### Task 1: Content Plan Pulse

**Files:**
- Modify: `web/app/lib/content-plan-logic.ts`
- Modify: `web/app/lib/content-plan-logic.test.mjs`
- Modify: `web/app/projects/[id]/topics/topics-client.tsx`
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [x] **Step 1: Write the failing tests**

Add `planPulseForTopics` tests to `web/app/lib/content-plan-logic.test.mjs`:

```js
test("planPulseForTopics turns plan health into one operator summary", async () => {
  const { planPulseForTopics } = await loadContentPlanLogicModule();

  assert.deepEqual(
    planPulseForTopics([
      topic({ id: "ready", status: "backlog", priority: 3 }),
      topic({ id: "scheduled", status: "scheduled", scheduled_at: "2026-07-03T10:00:00.000Z", priority: 2 }),
      topic({ id: "needs-priority", status: "backlog", priority: 0 }),
    ]),
    {
      title: "3 topics in the plan",
      detail: "1 ready to draft, 1 scheduled, 1 needs priority.",
      tone: "amber",
    },
  );
});

test("planPulseForTopics gives calm copy for an empty plan", async () => {
  const { planPulseForTopics } = await loadContentPlanLogicModule();

  assert.deepEqual(planPulseForTopics([]), {
    title: "No topics in the plan yet",
    detail: "Review analysis recommendations or generate from domain to seed the first backlog.",
    tone: "neutral",
  });
});
```

Add contract assertions to `dashboard-ux-phase1-contract.test.mjs` that `topics-client.tsx` uses `planPulseForTopics`, renders `Plan pulse`, and no longer renders the old four-card `Plan health` block.

- [x] **Step 2: Run tests to verify RED**

Run: `npm test -- app/lib/content-plan-logic.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: FAIL because `planPulseForTopics` and the new Content Plan copy do not exist yet.

- [x] **Step 3: Implement minimal code**

Export `planPulseForTopics(topics)` from `content-plan-logic.ts`, import it in `topics-client.tsx`, and replace the old four metric cards with one compact `Plan pulse` panel plus inline chips for backlog, ready, scheduled, and priority attention. Keep `planHealthForTopics` for existing tests and any future consumers.

- [x] **Step 4: Run tests to verify GREEN**

Run: `npm test -- app/lib/content-plan-logic.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: PASS.

### Task 2: Publish Action-Only Lanes

**Files:**
- Modify: `web/app/projects/[id]/publishing/publishing-client.tsx`
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [x] **Step 1: Write the failing test**

Add contract assertions that Publish computes `hasCanonicalPublishingWork`, shows a single `No publishing work is waiting` state when all canonical lanes are empty, conditionally renders `Ready to publish`, `Scheduled to publish`, `Published`, and `Publishing failed`, and labels the reconcile button `Check status`.

- [x] **Step 2: Run tests to verify RED**

Run: `npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: FAIL because the Publish page still renders the canonical lanes unconditionally and labels the action `Reconcile`.

- [x] **Step 3: Implement minimal code**

In `PublishingClient`, compute `hasCanonicalPublishingWork = readyCanonicals.length + scheduledCanonicals.length + published.length + inflight.length + failed.length > 0`. If false, show one `EmptyState`. If true, render each lane only when it has content, with `Publishing failed` first when present, then ready, scheduled, and published. Change visible button copy from `Reconcile` to `Check status`, while leaving the `reconcile()` function and API unchanged.

- [x] **Step 4: Run tests to verify GREEN**

Run: `npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: PASS.

### Task 3: Review Auto-Selects First Draft

**Files:**
- Modify: `web/app/projects/[id]/review/review-client.tsx`
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [x] **Step 1: Write the failing test**

Add contract assertions that Review has an effect setting `selectedArticleId` to `queueArticles[0].article.id` when there is a queue but no current selection, and remove the stale empty-inspector copy expectation.

- [x] **Step 2: Run tests to verify RED**

Run: `npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: FAIL because Review currently leaves the inspector empty until the operator selects a row.

- [x] **Step 3: Implement minimal code**

Add a `useEffect` after `queueArticles` is computed:

```ts
useEffect(() => {
  if (selectedArticleId || queueArticles.length === 0) return;
  setSelectedArticleId(queueArticles[0].article.id);
}, [queueArticles, selectedArticleId]);
```

Then replace the desktop fallback copy with `Loading the first draft...` so it is not framed as a required user selection.

- [x] **Step 4: Run tests to verify GREEN**

Run: `npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: PASS.

### Final Verification

- [x] Run `npm test`
- [x] Run `npm run typecheck`
- [x] Run `npm run build`
- [x] Run `go test ./...`
- [x] Run `git diff --check`
