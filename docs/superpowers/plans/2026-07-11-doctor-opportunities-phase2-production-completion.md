# Doctor + Opportunities Phase 2 Production Completion Plan

> **For Codex:** Execute this plan with `superpowers:subagent-driven-development`, keep each implementation slice test-first, and use `superpowers:verification-before-completion` before merge or completion claims.

**Goal:** Remove the three production blockers that prevent Phase 2 from being fully operable: multi-event migration ledger writes, the historical Doctor “no repair needed” placeholder, and the manual Opportunity Finding request timeout.

**Architecture:** Preserve the append-only ledger and sequence ordering, but make event idempotency identify the canonical object created by an event. Normalize the obsolete Doctor placeholder in data so it becomes resolved healthy coverage and cannot enter Site Fixes. Move manual Opportunity Finding onto the existing durable workflow queue, return immediately, and make status reflect the queued/running/completed/failed workflow rather than only the Signal Scan analyzer row.

**Tech Stack:** PostgreSQL migrations, Go services and contract tests, Next.js/TypeScript API client, Node test runner, Railway production, Chrome production verification.

---

## Task 1: Correct migration-ledger event identity

**Files:**
- Create: `internal/migrations/0071_migration_ledger_event_identity_index.sql`
- Create: `internal/migrations/0072_migration_ledger_event_identity_cutover.sql`
- Create: `internal/db/migration_ledger_event_identity_contract_test.go`

**Step 1: Write the failing contract test**

Add a schema migration contract that requires:

- a non-transactional `CREATE UNIQUE INDEX CONCURRENTLY` on `(migration_batch_id, source_object_type, source_object_id, operation, canonical_object_type, canonical_object_id)`;
- `NULLS NOT DISTINCT`, so an event whose canonical ID is intentionally null remains idempotent;
- a bounded-lock cutover migration that verifies the replacement index exists and is valid before dropping `migration_ledger_migration_batch_id_source_object_type_sour_key`;
- retention of the independent `(migration_batch_id, sequence_number)` uniqueness.

Run `go test ./internal/db -run TestMigrationLedgerEventIdentityContract -count=1` and confirm it fails because migrations 0071/0072 do not exist.

**Step 2: Add the concurrent replacement index**

Create migration 0071 with the repository's non-transactional/index markers and `CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uniq_migration_ledger_canonical_event ... NULLS NOT DISTINCT` over the full event identity.

**Step 3: Cut over under a bounded lock**

Create migration 0072 with `lock_timeout = '5s'`. In a `DO` block, fail closed unless the replacement index is present and `indisvalid`; then drop only the obsolete source+operation constraint. Reset the timeout.

**Step 4: Verify the focused contract**

Run `go test ./internal/db -run 'TestMigrationLedger(EventIdentity|Operation)Contract' -count=1` and require PASS.

## Task 2: Normalize obsolete Doctor healthy placeholders

**Files:**
- Create: `internal/migrations/0073_doctor_legacy_healthy_placeholder.sql`
- Create: `internal/db/doctor_legacy_healthy_placeholder_contract_test.go`
- Modify: `web/app/projects/[id]/doctor/doctor-client.test.mjs`

**Step 1: Write failing data and UI contracts**

Require migration 0073 to target only `issue_type = 'no_active_technical_blockers'`, set `finding_kind = 'healthy'`, transition active rows to `resolved`, clear all repair/autofix eligibility, and add an explicit normalization marker to evidence. Require affected completed runs whose coverage is empty to receive a non-actionable `legacy_report_health` coverage item with the original project-surface identity in both `checked_urls` and `passed_urls`.

Extend the Doctor client source contract so healthy findings are excluded from cards, drawer selection, and Site Fix creation affordances.

Run the focused Go and Node tests and confirm the migration contract fails before implementation.

**Step 2: Add the bounded normalization migration**

Create migration 0073 with a five-second lock timeout. Use a CTE to identify only the obsolete issue type, update those findings to resolved healthy records, and update only completed affected runs with empty `healthy_coverage`. Preserve historical rows; do not delete them.

**Step 3: Verify focused contracts**

Run `go test ./internal/db -run TestDoctorLegacyHealthyPlaceholderContract -count=1` and `cd web && npm test -- --test-name-pattern='Doctor'`; require PASS.

## Task 3: Make manual Opportunity Finding durable and observable

**Files:**
- Modify: `internal/workflow/worker.go`
- Modify: `internal/db/queries/workflow.sql`
- Modify generated `internal/db/workflow.sql.go`
- Modify: `internal/api/handlers_seo.go`
- Modify: `internal/api/opportunity_finding_status_contract_test.go`
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/scheduler/helpers_test.go`
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/api.test.mjs`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Modify: `web/app/lib/seo-client-contract.test.mjs`

**Step 1: Write failing durable-run contracts**

Require a new `opportunity_finding.requested` workflow event, a query for the latest event for the project, an API handler that enqueues/reuses active work and returns HTTP 202, a scheduler handler that executes both configured stages, and a web client that polls status while the last run is queued/running.

Run focused API, scheduler, workflow, and web contract tests and confirm they fail before implementation.

**Step 2: Add the durable workflow event and status query**

Add `EventOpportunityFindingRequested`. Add a sqlc query that returns the newest matching workflow event for a project, ordered by creation time. Regenerate sqlc output.

**Step 3: Make POST enqueue and GET report the actual workflow**

Replace the synchronous POST pipeline with active-event reuse or a new uniquely keyed event and return 202 with the refreshed status. Map workflow states to product states (`pending` to `queued`, `succeeded` to `completed`, `dead` to `failed`) and expose created/processed/error timing. Fall back to the latest analyzer run only for pre-workflow history.

**Step 4: Execute the pipeline in the scheduler**

Refactor the existing scheduled runner to accept scheduled/manual policy. Handle the new workflow event with manual policy so both Signal Scan and AI Discovery execute as persisted worker work. Until per-stage checkpoints exist, classify pipeline/partial-stage errors and interrupted stale claims as terminal for that request so successful billable stages are not blindly repeated; fence all terminal updates by status and claimed attempt, expose the joined error, and let the user explicitly retry. Do not launch an untracked goroutine.

**Step 5: Poll from the product UI**

Treat the POST as start acknowledgement, refresh immediately, and poll `getOpportunityFindingStatus` while queued/running. Show success only after completed and surface failed/dead errors. The POST itself remains an ordinary short request and no long browser timeout override is needed.

**Step 6: Verify the focused contracts**

Run the focused Go and Node tests for Opportunity Finding and workflow handling; require PASS.

## Task 4: Complete repository verification and review

**Files:**
- Modify only if verification exposes a root-cause defect.

**Step 1: Run backend verification**

Run `go test ./...`, `go vet ./...`, and the repository build command used by CI.

**Step 2: Run frontend verification**

Run `cd web && npm test` and `cd web && npm run build`.

**Step 3: Inspect the diff and migrations**

Confirm no unrelated workspace changes, no destructive data deletion, no loss of ledger immutability, and no secrets in changes or logs. Request code review; address only technically valid findings and rerun affected tests.

## Task 5: Merge, deploy, and prove production behavior

**Files:**
- No persistent source changes unless production verification finds a gap.

**Step 1: Publish through the required GitHub flow**

Commit the scoped changes, push `codex/doctor-phase2-production-completion`, open a PR to `origin/main`, merge after required checks, and wait for Railway and Vercel production deployments to succeed.

**Step 2: Prove migration safety in production**

Using the production database through Railway without printing environment values:

1. dry-run the Site Fix migration and record source/migrated/review/conservation counts;
2. apply it and confirm Doctor writer authority becomes canonical;
3. rerun apply and confirm idempotent results with no duplicate ledger event;
4. execute the supported rollback and confirm writer authority and aliases return to the pre-cutover state;
5. re-apply and confirm canonical authority and conservation again.

**Step 3: Verify with the user's logged-in Chrome session**

On production citeloop:

- open Doctor and confirm the obsolete “No repair needed” item is absent from findings and represented only under healthy coverage;
- confirm no healthy item has Repair JSON or Add to Site Fixes actions;
- open Site Fixes and confirm canonical migrated state renders;
- run Opportunity Finding manually and wait for completion without an eight-second timeout;
- refresh Opportunities and confirm updated run status/counts plus visible Signal Scan and AI Discovery evidence.

**Step 4: Fix any production gap and repeat**

If production differs from the contracts, create a fresh latest-main worktree for the follow-up fix, merge/deploy it through a new PR, and repeat the failed production proof before reporting completion.
