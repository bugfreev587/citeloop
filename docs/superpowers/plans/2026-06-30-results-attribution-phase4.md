# Results Attribution Phase 4 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete PRD Phase 4 so published content actions have durable measurement checkpoints, action-level Results APIs, and a Results page that explains before/after outcomes, insufficient data, and confounders.

**Architecture:** Add an `action_measurements` table as the checkpoint ledger, while keeping `content_actions.measurement_window` and `outcome_summary` as the action summary cache. The scheduler writes checkpoint rows when measurement windows are due, Results APIs return action rows with embedded measurements, and the existing Results surface consumes those rows instead of reconstructing attribution from generic SEO actions alone.

**Tech Stack:** Go, Chi, sqlc, PostgreSQL JSONB migrations, Next.js App Router, TypeScript, node test runner.

---

## Acceptance Gates

- [ ] `go test ./internal/db ./internal/api ./internal/scheduler -count=1`
- [ ] `go test ./...`
- [ ] `cd web && node --test app/lib/api.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs app/lib/visibility-summary-contract.test.mjs`
- [ ] `cd web && npm test`
- [ ] `cd web && npm run typecheck`
- [ ] `cd web && npm run build`
- [ ] PR checks pass.
- [ ] Production Vercel and Railway deployments reach the merge commit.
- [ ] Production Results page loads and shows Phase 4 Results attribution UI/API behavior without errors.

## Files

- Create: `internal/migrations/0030_action_measurements.sql`
- Modify: `internal/db/queries/seo.sql`
- Generate: `internal/db/seo.sql.go`, `internal/db/models.go`, `internal/db/querier.go`
- Test: `internal/db/measurement_attribution_contract_test.go`
- Modify: `internal/scheduler/scheduler.go`
- Test: `internal/scheduler/measurement_attribution_test.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/handlers_seo.go`
- Test: `internal/api/results_routes_test.go`
- Modify: `web/app/lib/api.ts`
- Test: `web/app/lib/api.test.mjs`, `web/app/lib/visibility-summary-contract.test.mjs`, `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

## Task 1: Contract Tests for Phase 4 Surface

- [ ] Add a DB contract test that requires `action_measurements`, `UpsertActionMeasurement`, `ListActionMeasurementsForProject`, and `ListResultsActionRows`.
- [ ] Add API route tests for:
  - `GET /api/projects/{projectID}/results/actions`
  - `GET /api/projects/{projectID}/results/actions/{actionID}`
  - `POST /api/projects/{projectID}/results/recompute`
- [ ] Add web API contract coverage for `ResultsAction`, `ActionMeasurement`, `listResultsActions`, `getResultsAction`, and `recomputeResults`.
- [ ] Run the targeted tests and verify they fail because the Phase 4 surface is missing.

## Task 2: Measurement Ledger

- [ ] Add migration `0030_action_measurements.sql` with:
  - action/project/article references;
  - `checkpoint_day`, `window_start`, `window_end`;
  - `seo_metrics`, `ga4_metrics`, `geo_metrics`, `execution_metrics`;
  - `outcome_label`, `outcome_reason`, `attribution_confidence`, `confounders`, `computed_at`;
  - unique `(project_id, content_action_id, checkpoint_day)`;
  - indexes for project/action lookup and admin cascade performance.
- [ ] Add sqlc queries to upsert/list/get measurement rows and list Results action rows.
- [ ] Run `sqlc generate`.
- [ ] Run DB contract tests and fix generated type mismatches.

## Task 3: Scheduler Attribution Writeback

- [ ] Refactor measurement closure helpers to produce:
  - completed checkpoint metadata;
  - `outcome_label` of `insufficient_data`, `positive`, `negative`, `mixed`, or `inconclusive`;
  - clear `outcome_reason`;
  - `attribution_confidence`;
  - `confounders`.
- [ ] Use the new `UpsertActionMeasurement` query for every completed due checkpoint.
- [ ] Keep `content_actions.outcome_summary` and `measurement_window` updated as the cached action summary.
- [ ] Add scheduler unit tests for no-data checkpoint closure and final completed window.

## Task 4: Results API

- [ ] Add response types for `ResultsAction` and `ActionMeasurement`.
- [ ] Implement `GET /results/actions` by reading action rows and embedding measurement rows per action.
- [ ] Implement `GET /results/actions/{actionID}` for focused drill-down.
- [ ] Implement `POST /results/recompute` to recompute due checkpoints for the project by using the same scheduler closure path when a scheduler is available, otherwise return the current action list with a no-op status.
- [ ] Ensure empty projects return empty arrays and insufficient-data explanations instead of errors.

## Task 5: Results UI

- [ ] Add web API types and normalizers for Phase 4 Results actions and measurements.
- [ ] Load `api.listResultsActions(projectId, { limit: 50 })` in `ResultsClient`.
- [ ] Render action-level before/after rows using measurement metrics when present.
- [ ] Show outcome label, reason, attribution confidence, and confounder notes in the default Results card.
- [ ] Keep advanced raw diagnostics collapsed.
- [ ] Add a Recompute button wired to `api.recomputeResults(projectId)`.

## Task 6: Verification, PR, Production

- [ ] Run all acceptance gates.
- [ ] Stage only Phase 4 files.
- [ ] Commit with a Phase 4 message.
- [ ] Push `codex/seo-geo-phase4-results-attribution`.
- [ ] Create PR to `origin/main`, wait for checks, merge it.
- [ ] Wait for Vercel and Railway production deploys.
- [ ] Verify production Results page and backend health.
