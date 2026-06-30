# Home Human Action Queue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn Home's `Needs you` module into the primary human action queue and jump-off point for manual gates.

**Architecture:** Keep the first version front-end derived from existing Home page state. Add a small typed action-item model inside `workspace.tsx`, rank items by human urgency, and render a concise top set with direct CTAs to Context, Analysis, Review, Publish, or Settings. Protect the behavior with the existing dashboard UX contract tests.

**Tech Stack:** Next.js App Router, React client component, Tailwind CSS, Node test runner, TypeScript.

---

### Task 1: Contract Test

**Files:**
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Write the failing test**

Add a test that asserts Home's `Needs you` module includes the human action queue concepts:

```js
test("home turns Needs you into the main human action queue", () => {
  const workspace = read("projects/[id]/workspace.tsx");

  for (const copy of [
    "Manual gates and setup",
    "Blocking now",
    "Needs review",
    "Improves results",
    "Confirm Context",
    "Review analysis",
    "Connect Search Console",
    "View all open actions",
  ]) {
    assert.match(workspace, new RegExp(copy));
  }

  assert.match(workspace, /humanActionItems/);
  assert.match(workspace, /visibleHumanActionItems/);
  assert.match(workspace, /primaryAction = humanActionItems\[0\]/);
  assert.doesNotMatch(workspace, /Automation warnings/);
  assert.doesNotMatch(workspace, /Variants waiting on canonical/);
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs`
Expected: FAIL because the queue strings and model do not exist yet.

### Task 2: Home Queue Implementation

**Files:**
- Modify: `web/app/projects/[id]/workspace.tsx`

- [ ] **Step 1: Add a small Home action item model**

Add `HumanActionItem` and supporting severity/category types near the other local Home types. Use existing page state only.

- [ ] **Step 2: Replace resource-count Needs You rows**

Build `humanActionItems` from:
- Context confirmation or context build attention.
- Publishing failures.
- Drafts needing human review or QA blocking decisions.
- Analysis opportunities waiting for review.
- Publisher/GSC setup that improves or unlocks the loop.
- Automation warnings that require attention.

- [ ] **Step 3: Render top actions as a Home module**

Show the highest priority action through `primaryAction`. In the Home module, render the first five actions with severity/category badges, short impact copy, and CTA text. Show an expandable `View all open actions` control when there are more than five.

### Task 3: Verification

**Files:**
- Verify: `web/app/projects/[id]/workspace.tsx`
- Verify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Run focused web tests**

Run: `cd web && npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs`
Expected: PASS.

- [ ] **Step 2: Run full web verification**

Run:
```bash
cd web && npm test
cd web && npm run typecheck
cd web && npm run build
```

Expected: all commands exit 0.

- [ ] **Step 3: Run backend regression tests**

Run: `make test`
Expected: all Go packages pass.

### Task 4: Ship and Verify Production

**Files:**
- No additional source files.

- [ ] **Step 1: Push branch**

Run: `git push -u origin codex/home-human-action-queue`
Expected: branch pushed.

- [ ] **Step 2: Create and merge PR**

Create PR to `origin/main`, wait for checks, merge it, and confirm `main` contains the merge commit.

- [ ] **Step 3: Wait for deployment**

Use Vercel deployment tooling or project status to wait for the production deployment triggered by the merge.

- [ ] **Step 4: Verify production**

Open the production Home page for a project and verify `Needs you` renders as the human action queue with Context, Analysis, Review, Publish, and setup actions when applicable.
