# Opportunity Finding Speed and Progress Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Growth Stage control compact, make long-running discovery visibly active, and reduce AI evidence refresh latency with safe bounded concurrency.

**Architecture:** Keep the six durable workflow checkpoints as the source of truth, but render an indeterminate stage bar and client-side elapsed time while a checkpoint is running. Within answer evidence collection, execute prompt-scoped provider calls with a fixed three-worker pool, preserve input ordering, record every physical AI call, and return partial successes with the first joined error.

**Tech Stack:** Go 1.24, React 19, Next.js 15, TypeScript, Tailwind CSS, Node test runner.

---

### Task 1: Compact Growth Stage selector

**Files:**
- Modify: `web/app/lib/seo-client-contract.test.mjs`
- Modify: `web/app/projects/[id]/seo/growth-stage-selector.tsx`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

- [x] **Step 1: Write the failing contract test**

Assert that the selector trigger has a dedicated compact marker, does not render the `Growth Stage` label inside the closed button, and renders a `Growth Stage` header inside the open listbox. Assert that both the selector and GSC trigger use the shared `h-8 w-36` geometry.

- [x] **Step 2: Run the focused test and verify RED**

Run: `node --test app/lib/seo-client-contract.test.mjs`

Expected: FAIL because the existing selector is `h-11`, at least 14rem wide, and includes the label in its closed state.

- [x] **Step 3: Implement the compact control**

Change the trigger to `h-8 w-36 rounded-lg`, render only `selected.label` and the chevron, and add this listbox header before the options:

```tsx
<div className="px-3 pb-1.5 pt-2 text-[11px] font-semibold uppercase tracking-[0.08em] text-slate-500">
  Growth Stage
</div>
```

Give the GSC trigger the same `h-8 w-36` geometry so the visual contract is exact.

- [x] **Step 4: Run focused tests and typecheck**

Run: `npm test -- --test-name-pattern="Growth Stage" && npm run typecheck`

Expected: PASS.

### Task 2: Honest active-stage progress

**Files:**
- Modify: `web/app/lib/seo-client-contract.test.mjs`
- Modify: `web/app/projects/[id]/seo/opportunity-finding-progress.tsx`

- [x] **Step 1: Write the failing progress contract test**

Assert the component owns an elapsed-time timer, renders `Elapsed`, gives the active progress bar an indeterminate state, and explains the expected duration with `Usually 45–120 seconds`.

- [x] **Step 2: Run the focused test and verify RED**

Run: `node --test app/lib/seo-client-contract.test.mjs`

Expected: FAIL because the current component renders a static `0%` bar until the first checkpoint finishes.

- [x] **Step 3: Implement elapsed time and indeterminate motion**

Use a one-second `setInterval` while the run is active, initialized from `run.started_at`. When a stage is active, omit `aria-valuenow`, set `aria-valuetext` to the current stage and elapsed duration, and animate a fixed-width inner segment using transform rather than pretending the stage has a known percentage. Keep the durable percentage visible only as `Completed checkpoints: N%`.

- [x] **Step 4: Run focused tests and typecheck**

Run: `npm test -- --test-name-pattern="Growth Stage" && npm run typecheck`

Expected: PASS.

### Task 3: Three-worker answer evidence collection

**Files:**
- Modify: `internal/geo/shared_evidence_test.go`
- Modify: `internal/geo/shared_evidence.go`

- [x] **Step 1: Write failing concurrency tests**

Add a blocking prompt runner test that submits ten prompts, proves no more than three calls are simultaneously active, releases workers, and asserts results retain original prompt order. Add a partial-failure test proving successful rows and usage survive when one prompt fails.

- [x] **Step 2: Run the focused Go tests and verify RED**

Run: `go test ./internal/geo -run 'TestObserveAnswerPromptsBoundedConcurrency|TestObserveAnswerPromptsPreservesPartialSuccess' -count=1`

Expected: FAIL because prompt calls currently execute sequentially and return immediately on the first error.

- [x] **Step 3: Implement bounded concurrency**

Extract one prompt’s ledger/provider lifecycle into a helper. Use a three-worker job channel and indexed result slots:

```go
const answerPromptConcurrency = 3

type answerPromptResult struct {
    index int
    row   ProviderObservation
    usage answerCallUsage
    err   error
}
```

Workers must stop accepting new jobs when `ctx.Done()` closes, while already-started calls still finish their ledger rows with the existing cancellation-safe timeout. Fold results by input index, append only successful observations, sum all provider usage including failed calls, and join errors after every started call is accounted for.

- [x] **Step 4: Run focused and race tests**

Run: `go test ./internal/geo -run 'TestObserveAnswerPrompts' -count=1 && go test -race ./internal/geo -run 'TestObserveAnswerPrompts' -count=1`

Expected: PASS with no race report.

### Task 4: Full verification and delivery

**Files:**
- Verify all modified files

- [x] **Step 1: Run complete verification**

Run: `go test ./...`, `npm test`, `npm run typecheck`, and `npm run build`.

Expected: all commands PASS.

- [ ] **Step 2: Commit and request review**

Review the diff against this plan, commit the scoped changes, push the feature branch, and open a PR targeting `origin/main`.

- [ ] **Step 3: Merge and verify production**

Merge the PR, wait for API and Web deployments to succeed, confirm the compact Growth Stage selector in production, manually run Opportunity Finding for UniPost, and verify both live elapsed/indeterminate progress and reduced evidence-refresh duration from production checkpoint records.
