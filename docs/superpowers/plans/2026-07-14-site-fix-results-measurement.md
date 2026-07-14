# Site Fix Results Measurement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Route only measurement-ready Site Fixes into Results while keeping Site Fix status permanently verified and measurement state independently durable.

**Architecture:** Add a dedicated Site Fix measurement aggregate, checkpoint ledger, learning/quality records, and handoff outbox. Classify Site Fixes deterministically at creation, freeze eligible measurement plans before apply, activate them after verification, advance them with the existing finite measurement semantics, and expose normalized Site Fix rows in Results.

**Tech Stack:** PostgreSQL migrations, sqlc, Go services/scheduler/API, Next.js/TypeScript, Node contract tests.

---

### Task 1: Schema and sqlc persistence

**Files:**
- Create: internal/migrations/0087_site_fix_measurements.sql
- Create: internal/migrations/0088_site_fix_measurements_validate.sql
- Create: internal/db/queries/site_fix_measurements.sql
- Test: internal/db/site_fix_measurements_contract_test.go

- [x] Write failing contract tests for classification columns, composite project scope, immutable generation, bounded policy, checkpoint taxonomy, outbox idempotency, and validation migration.
- [x] Run go test ./internal/db and confirm the schema expectations fail.
- [x] Add rolling-compatible nullable/defaulted Site Fix classification columns plus dedicated measurements, checkpoints, terminal records, learning/quality children, and handoff outbox.
- [x] Add sqlc queries for idempotent plan creation, handoff enqueue/claim/complete/retry/reclaim, due measurements, idempotent checkpoint/terminal children, and paginated Results reads.
- [x] Run all migrations against disposable PostgreSQL 16 and exercise concurrent generation replay, project scoping, finite policy validation, lease fencing/reclaim, append-only replay, and pagination.
- [x] Run make sqlc, go test ./internal/db, and go test ./....
- [x] Commit the schema slice and quality hardening.

### Task 2: Deterministic classification and immutable plan

**Files:**
- Create: internal/sitefix/measurement_policy.go
- Create: internal/sitefix/measurement_policy_test.go
- Modify: internal/sitefix/creator.go
- Modify: internal/db/queries/site_fixes.sql
- Test: internal/sitefix/creator_test.go

- [ ] Write failing table tests for fix_type precedence, unknown fallback, presentation-only metadata, CTR metadata, schema repair/entity optimization, deterministic rule version, and readiness gates.
- [ ] Verify RED with go test ./internal/sitefix.
- [ ] Implement a pure classifier returning fix_type, impact_mode, measurement_policy, origin, confidence, hypothesis, metric, baseline, and finite policy snapshot.
- [ ] Persist classification in CreateCanonicalSiteFix; do not inspect prose to upgrade policy.
- [ ] Do not create a measurement generation during creation. Optional, unknown, and incomplete required candidates create no measurement row; complete required plans are frozen atomically at approval.
- [ ] Run focused tests and commit.

### Task 3: Approval baseline and verified handoff

**Files:**
- Modify: internal/db/queries/site_fixes.sql
- Modify: internal/api/handlers_site_fixes.go
- Test: internal/api/site_fixes_api_test.go
- Test: internal/sitefix/measurement_policy_test.go

- [ ] Write failing tests proving Site Fix remains verified, approved plans freeze before apply, late opt-in is prospective/low-confidence, and verification creates one outbox event.
- [ ] Verify RED.
- [ ] Extend approval to create exactly one idempotent generation and freeze eligible policy/baseline metadata before apply.
- [ ] Extend verification transition to enqueue the handoff in the same SQL statement.
- [ ] Add an idempotent opt-in endpoint that creates a prospective generation without changing Site Fix status.
- [ ] Run focused tests and commit.

### Task 4: Worker, checkpoints, and terminal outcomes

**Files:**
- Create: internal/scheduler/site_fix_measurements.go
- Create: internal/scheduler/site_fix_measurements_test.go
- Modify: internal/scheduler/scheduler.go
- Modify: internal/scheduler/helpers.go

- [ ] Write failing tests for outbox activation, retry, reconciliation, due checkpoint evaluation, bounded follow-ups, absolute terminalization, and taxonomy.
- [ ] Verify RED.
- [ ] Implement handoff worker and reconciliation using project advisory locks, skip-locked claims, finite backoff, and immutable generation keys.
- [ ] Reuse measurement evidence/evaluator semantics for Site Fix target URL/query and baseline snapshot.
- [ ] Insert checkpoints idempotently and create directional learning or quality records at terminal state.
- [ ] Register the worker with the hourly measurement tick.
- [ ] Run scheduler tests and commit.

### Task 5: API and Results read model

**Files:**
- Modify: internal/api/handlers_seo.go
- Modify: internal/api/results_routes_test.go
- Modify: internal/api/site_fixes_api_test.go

- [ ] Write failing API contract tests for source_type=site_fix, actual-row-only visibility, deep links, measurement status, and project scoping.
- [ ] Verify RED.
- [ ] Add normalized Site Fix Results rows and detail reads without changing content-action writes.
- [ ] Merge/paginate content action and Site Fix Results rows deterministically.
- [ ] Return handoff status and measurement summary on Site Fix detail.
- [ ] Run API tests and commit.

### Task 6: Web UI

**Files:**
- Modify: web/app/lib/types.ts
- Modify: web/app/lib/api.ts
- Modify: web/app/projects/[id]/site-fixes/site-fixes-client.tsx
- Modify: web/app/projects/[id]/seo/seo-client.tsx
- Test: web/app/lib/site-fix-measurement-policy-contract.test.mjs
- Test: web/app/lib/results-attribution-contract.test.mjs

- [ ] Write failing contract tests for Outcome type, verification-only completion, handoff pending/failed, Results deep link, source badge, and prospective warning.
- [ ] Verify RED with npm test.
- [ ] Normalize classification/measurement fields in the API client.
- [ ] Render Site Fix outcome policy without changing its lifecycle milestones.
- [ ] Render actual Site Fix measurement rows in Results with independent status/taxonomy.
- [ ] Run web tests and typecheck; commit.

### Task 7: Full verification, review, PR, deploy, production

- [ ] Run make sqlc, git diff --check, gofmt, go test ./..., go vet ./..., go build ./....
- [ ] Run web npm test, npm run typecheck, npm run build.
- [ ] Rebase on latest origin/main and rerun the full verification set.
- [ ] Push branch and create a ready PR to main.
- [ ] Wait for all required checks, merge the PR, and wait for backend and Vercel production deployments for the merge SHA.
- [ ] Verify production Site Fix detail shows verification-only for the metadata readability fix and Results does not contain it.
- [ ] Verify an eligible fixture/API path creates one independent measurement and Results deep link while Site Fix remains verified.
- [ ] If any boundary fails, patch on a fresh follow-up branch and repeat production verification.
