# Growth IA Sidebar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the project sidebar match the clearer daily workflow: Analysis/Opportunities, Content, Results, with Context moved to the low-frequency reference area between Docs and Settings.

**Architecture:** Keep existing routes stable (`/analysis`, `/context`, `/results`) and change the visible information architecture in `ProjectShell`. Update contract tests and user-facing copy so the sidebar no longer exposes the abstract `Intelligence`, `Execution`, or `Outcomes` section labels.

**Tech Stack:** Next.js App Router, React 18, Tailwind CSS 3, Node test runner contract tests.

---

### Task 1: Sidebar Contract

**Files:**
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
- Modify: `web/app/lib/docs-contract.test.mjs`

- [x] **Step 1: Write the failing contract tests**

Add assertions that the shell uses `Analysis`, `Content`, and `Results` section labels, shows `Opportunities` as the Analysis entry, omits `Intelligence`, `Execution`, and `Outcomes`, and renders footer reference links in this order: `Docs`, `Context`, `Settings`.

- [x] **Step 2: Run test to verify it fails**

Run: `cd web && npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs app/lib/docs-contract.test.mjs`

Expected: FAIL because the current shell still renders `Intelligence`, `Execution`, `Outcomes`, keeps `Context` near Home, and does not render Context between Docs and Settings.

- [x] **Step 3: Implement sidebar changes**

Update `web/app/components/project-shell.tsx` so `navSections` renders `Home`, then `Analysis -> Opportunities`, `Content -> Content Plan/Review/Publish`, `Results -> Results`. Add a footer `Context` link between `Docs` and `Settings` on desktop and mobile.

- [x] **Step 4: Run test to verify it passes**

Run: `cd web && npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs app/lib/docs-contract.test.mjs`

Expected: PASS.

### Task 2: User-Facing Copy Alignment

**Files:**
- Modify: `web/app/lib/dashboard-ux-logic.ts`
- Modify: `web/app/lib/dashboard-ux-logic.test.mjs`
- Modify: `web/app/docs/page.tsx`
- Modify: `web/app/lib/content-plan-logic.ts`
- Modify: `web/app/lib/content-plan-logic.test.mjs`

- [x] **Step 1: Write failing copy expectations**

Change tests that currently expect `Review analysis`, `finding ready in Analysis`, and `View in Analysis` to expect `Review opportunities`, `finding ready in Opportunities`, and `View opportunities` where the action is the user reviewing opportunities rather than reading analysis.

- [x] **Step 2: Run test to verify it fails**

Run: `cd web && npm test -- app/lib/dashboard-ux-logic.test.mjs app/lib/content-plan-logic.test.mjs app/lib/docs-contract.test.mjs`

Expected: FAIL because production copy still says `analysis`.

- [x] **Step 3: Update copy minimally**

Keep route destinations unchanged at `/analysis`, but change action labels and explanatory copy to use `Opportunities` for user decisions. Preserve `Analysis` as the section-level concept only where it describes the broad dashboard category.

- [x] **Step 4: Run test to verify it passes**

Run: `cd web && npm test -- app/lib/dashboard-ux-logic.test.mjs app/lib/content-plan-logic.test.mjs app/lib/docs-contract.test.mjs`

Expected: PASS.

### Task 3: Verification And Release Prep

**Files:**
- No new implementation files.

- [x] **Step 1: Run frontend checks**

Run:

```bash
cd web
npm test
npm run typecheck
npm run build
```

Expected: all commands pass.

- [x] **Step 2: Run backend regression check**

Run: `go test ./...`

Expected: PASS.

- [x] **Step 3: Review final diff and commit**

Run:

```bash
git diff --check
git status --short
git add docs/superpowers/plans/2026-07-02-growth-ia-sidebar.md web/app
git commit -m "feat: clarify growth workflow navigation"
```

Expected: commit succeeds on `codex/growth-ia-sidebar`.
