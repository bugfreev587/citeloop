# Visibility Action Loop Execution Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Execute `docs/PRD-CiteLoop-Visibility-Analysis-to-Content-Loop.md` phase by phase, with a PR, deployment wait, and production verification after each phase.

**Architecture:** Treat the PRD as the phase gate source. Phase 0 only lands the executable PRD and baseline verification. Later phases start from fresh `origin/main` worktrees and change product behavior only after failing tests define the phase acceptance gap.

**Tech Stack:** Go API and scheduler, sqlc/Postgres migrations and queries, Next.js App Router, Node `node:test` contract tests, browser production smoke checks, GitHub PR workflow.

---

### Task 1: Phase 0 PRD Gate

**Files:**
- Create: `docs/PRD-CiteLoop-Visibility-Analysis-to-Content-Loop.md`
- Create: `docs/superpowers/plans/2026-07-02-visibility-action-loop-execution.md`

- [ ] **Step 1: Verify PRD contains phase-specific gates**

Run:

```bash
rg -n "Phase [0-5]:|Exit criteria:|Cannot move|production verification|Acceptance Matrix" docs/PRD-CiteLoop-Visibility-Analysis-to-Content-Loop.md
```

Expected: output includes Phase 0 through Phase 5, exit criteria, cannot-move-next conditions, production verification language, and the acceptance matrix.

- [ ] **Step 2: Verify factual corrections are represented**

Run:

```bash
rg -n "input_snapshot|seo_opportunities.status|accepted-without-action|visibility summary is the single lifecycle source|measurement due" docs/PRD-CiteLoop-Visibility-Analysis-to-Content-Loop.md
```

Expected: output shows the corrected `input_snapshot` note, the six opportunity statuses, accept alias resolution, backend summary source of truth, and scheduler measurement due handling.

- [ ] **Step 3: Run backend baseline**

Run:

```bash
make test
```

Expected: `go test ./...` exits 0.

- [ ] **Step 4: Run frontend baseline**

Run:

```bash
cd web
npm test
npm run typecheck
```

Expected: Node tests exit 0 and Next.js typecheck exits 0.

- [ ] **Step 5: Commit Phase 0**

Run:

```bash
git status --short
git add docs/PRD-CiteLoop-Visibility-Analysis-to-Content-Loop.md docs/superpowers/plans/2026-07-02-visibility-action-loop-execution.md
git commit -m "docs: add visibility action loop phase gates"
```

Expected: commit succeeds with only the two documentation files staged.

### Task 2: Phase 0 PR, Merge, and Production Smoke

**Files:**
- No code files.

- [ ] **Step 1: Push and create PR**

Run:

```bash
git push -u origin codex/visibility-action-loop-phase0
gh pr create --base main --head codex/visibility-action-loop-phase0 --title "docs: add visibility action loop phase gates" --body "Adds the executable PRD and phase-by-phase acceptance gates for the Visibility Analysis to Action Loop."
```

Expected: GitHub returns a PR URL.

- [ ] **Step 2: Merge PR**

Run:

```bash
gh pr merge --squash --delete-branch
```

Expected: PR is merged into `origin/main`.

- [ ] **Step 3: Wait for deployment**

Run the repository's available deployment/status command. If no deployment is triggered because the PR is docs-only, record the provider output or GitHub check result that proves this.

Expected: deployment is complete, skipped as docs-only, or blocked by a documented access issue.

- [ ] **Step 4: Production smoke**

Open production and verify an existing project still loads these pages:

```text
/projects/{projectID}
/projects/{projectID}/analysis
/projects/{projectID}/plan
/projects/{projectID}/results
```

Expected: pages load without production console/runtime errors. If project access or production credentials are unavailable, record the blocker and do not claim production verification passed.

### Task 3: Phase 1 Shared Summary UX

**Files:**
- Modify: `web/app/projects/[id]/workspace.tsx`
- Modify: `web/app/projects/[id]/topics/topics-client.tsx`
- Modify: `web/app/lib/dashboard-ux-logic.ts`
- Modify: `web/app/lib/dashboard-ux-logic.test.mjs`
- Modify: `web/app/lib/visibility-summary-contract.test.mjs`

- [ ] **Step 1: Start from latest main**

Run:

```bash
git fetch origin
git worktree add /Users/xiaoboyu/.config/superpowers/worktrees/citeloop/visibility-action-loop-phase1 -b codex/visibility-action-loop-phase1 origin/main
```

Expected: new clean worktree is created from `origin/main`.

- [ ] **Step 2: Write failing contract tests**

Add assertions that `workspace.tsx` and `topics-client.tsx` call `api.getVisibilitySummary(projectId)` and do not use `api.listSEOOpportunities(projectId, { status: "open"` as their primary lifecycle count source.

Run:

```bash
cd web
npm test -- app/lib/visibility-summary-contract.test.mjs app/lib/dashboard-ux-logic.test.mjs
```

Expected: tests fail because Home and Content Plan still derive some counts separately.

- [ ] **Step 3: Implement shared summary consumption**

Update Home and Content Plan so lifecycle/open/in-loop counts use `VisibilitySummary` fields:

```typescript
const openOpportunityCount = visibilitySummary?.open_opportunities.length ?? 0;
const actionsInLoopCount = visibilitySummary?.actions_in_loop.length ?? 0;
const lifecycleCounts = visibilitySummary?.lifecycle_counts ?? {};
```

Keep independent topic/article/review counts only for topic and article state that the summary does not own.

- [ ] **Step 4: Verify green**

Run:

```bash
cd web
npm test -- app/lib/visibility-summary-contract.test.mjs app/lib/dashboard-ux-logic.test.mjs
npm run typecheck
```

Expected: tests and typecheck exit 0.

- [ ] **Step 5: Run full local verification**

Run:

```bash
make test
cd web
npm test
npm run typecheck
```

Expected: all commands exit 0.

- [ ] **Step 6: PR, merge, deploy, production verify**

Follow Task 2 with branch `codex/visibility-action-loop-phase1`. Production verification must include Home, Analysis, Content Plan, and Results screenshots plus the production visibility summary API response for one project.

### Task 4: Phase 2 Multi-Surface Action Routing

**Files:**
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/api/handlers_seo.go`
- Modify: `internal/db/queries/seo.sql`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Modify: `web/app/lib/action-portfolio-contract.test.mjs`
- Add or modify Go contract tests for topic-backed versus non-topic actions.

- [ ] **Step 1: Start from latest main**

Create branch `codex/visibility-action-loop-phase2` from `origin/main` after Phase 1 production verification passes.

- [ ] **Step 2: Write failing routing tests**

Tests must prove:

- article/page actions create or link `topics.source_content_action_id`;
- `metadata_patch`, `internal_link_patch`, `schema_patch`, and `technical_fix` actions do not create topics by default;
- non-topic actions expose `output_snapshot` or `diff_snapshot` before approval.

- [ ] **Step 3: Implement routing**

Update the scheduler/action planner to branch by action type and asset type. Keep topic creation for content assets and store direct patch outputs on `content_actions` for non-topic actions.

- [ ] **Step 4: Verify and ship**

Run full local verification, PR, merge, wait for deployment, and production-verify one topic-backed action and one non-topic action.

### Task 5: Phase 3 Analyzer Expansion

**Files:**
- Modify: `internal/seo/search_opportunities.go`
- Modify or add analyzer files under `internal/seo`
- Modify: `internal/seo/*_test.go`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

- [ ] **Step 1: Start from latest main**

Create branch `codex/visibility-action-loop-phase3` from `origin/main` after Phase 2 production verification passes.

- [ ] **Step 2: Write failing analyzer fixture tests**

Tests must cover internal-link gap, schema gap, cannibalization, thin evidence page, technical visibility issue, and GEO citation/source gap when supporting data exists.

- [ ] **Step 3: Implement analyzer candidates**

Each generated opportunity must include source, why-now evidence, scoring rationale, expected impact, recommendation, effort, risk, confidence, and idempotency key.

- [ ] **Step 4: Verify and ship**

Run full local verification, PR, merge, wait for deployment, and production-verify expected opportunity types on a project with supporting data or a controlled fixture project.

### Task 6: Phase 4 Measurement and Impact Reports

**Files:**
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/api/handlers_seo.go`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Modify: `web/app/lib/results-attribution-contract.test.mjs`
- Modify or add scheduler tests for due checkpoint behavior.

- [ ] **Step 1: Start from latest main**

Create branch `codex/visibility-action-loop-phase4` from `origin/main` after Phase 3 production verification passes.

- [ ] **Step 2: Write failing Results tests**

Tests must prove Results renders outcome labels, confidence, confounders, insufficient-data states, waiting/too-early states, and measurement queue separation.

- [ ] **Step 3: Implement any remaining measurement/report gaps**

Keep backend outcome data conservative. Do not claim causality unless the data supports it.

- [ ] **Step 4: Verify and ship**

Run full local verification, PR, merge, wait for deployment, and production-verify at least one action measurement update in Results.

### Task 7: Phase 5 IA, Diagnostics, and Learning

**Files:**
- Modify: `web/app/projects/[id]/workspace.tsx`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Modify: `web/app/projects/[id]/settings/activity/page.tsx`
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Start from latest main**

Create branch `codex/visibility-action-loop-phase5` from `origin/main` after Phase 4 production verification passes.

- [ ] **Step 2: Write failing IA/copy tests**

Tests must prove product pages visibly separate Opportunity Briefs, Action Portfolio, Impact Reports, and operations health, and no primary workflow copy implies everything ends in a blog post.

- [ ] **Step 3: Implement page-role refinements**

Home becomes the Growth Control Center; Analysis owns Opportunity Briefs; Content Plan owns topic backlog plus action handoff; Results owns Impact Reports and learning; Activity/Settings owns operational diagnostics.

- [ ] **Step 4: Verify and ship**

Run full local verification, PR, merge, wait for deployment, and production-verify desktop and mobile screenshots for Home, Analysis, Content Plan, Results, and Activity/Settings.

