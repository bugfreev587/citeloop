# Visibility Analysis Content Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Execute the Visibility/Analysis-to-Content PRD so reviewed opportunities remain traceable from analysis through plan, draft, publish/apply, measurement, and learned outcome.

**Architecture:** Reuse existing `seo_opportunities`, `content_actions`, `topics`, `articles`, and workflow events. Add a project-scoped visibility summary contract that derives lifecycle presentation state from existing raw states, make `acceptSEOOpportunity` call the action creation path, surface loop-in-motion state on the Analysis page, and add measurement due processing that writes `outcome_summary`.

**Tech Stack:** Go API + sqlc/Postgres queries, scheduler workflow events, Next.js App Router client UI, Node `node:test` contract tests, Go tests.

---

### Task 1: Visibility Summary Contract

**Files:**
- Modify: `internal/api/server.go`
- Modify: `internal/api/handlers_seo.go`
- Modify: `internal/db/queries/seo.sql`
- Generated: `internal/db/seo.sql.go`
- Generated: `internal/db/querier.go`
- Test: `internal/api/visibility_summary_contract_test.go`

- [x] **Step 1: Write the failing contract test**

Create `internal/api/visibility_summary_contract_test.go` with assertions that the API registers `GET /seo/visibility/summary`, the handler emits `actions_in_loop`, `lifecycle_counts`, `top_measurement_updates`, `diagnostics_health`, and lifecycle values `detected`, `added_to_plan`, `planned`, `drafting`, `ready_for_review`, `approved`, `published_or_applied`, `measuring`, `learned`, `blocked`.

- [x] **Step 2: Run the red test**

Run: `go test ./internal/api -run VisibilitySummary -count=1`

Expected: FAIL because the route and handler do not exist.

- [x] **Step 3: Add SQL query support**

Add a query such as `ListVisibilityContentActionRows` that left joins `content_actions` to `seo_opportunities`, `topics`, and `articles`, preserving current raw fields and linked topic/article IDs needed for lifecycle derivation.

- [x] **Step 4: Generate sqlc**

Run: `make sqlc`

Expected: generated query methods compile.

- [x] **Step 5: Implement the handler**

Add a `VisibilitySummary` response shape in `handlers_seo.go` and derive lifecycle counts from open opportunities plus joined action rows.

- [x] **Step 6: Run the green test**

Run: `go test ./internal/api -run VisibilitySummary -count=1`

Expected: PASS.

### Task 2: Accept Opportunity Alias

**Files:**
- Modify: `internal/api/handlers_seo.go`
- Test: `internal/api/accept_opportunity_alias_contract_test.go`

- [x] **Step 1: Write the failing contract test**

Create `internal/api/accept_opportunity_alias_contract_test.go` asserting `acceptSEOOpportunity` calls the shared action creation implementation and no longer passes `"accepted"` to `updateSEOOpportunityStatus`.

- [x] **Step 2: Run the red test**

Run: `go test ./internal/api -run AcceptOpportunityAlias -count=1`

Expected: FAIL because `acceptSEOOpportunity` still only sets status to accepted.

- [x] **Step 3: Extract shared action creation**

Move the core of `createSEOContentAction` into a helper that accepts a default HTTP status and optional request body. Call it from both `/actions` and `/accept`.

- [x] **Step 4: Run the green test**

Run: `go test ./internal/api -run 'AcceptOpportunityAlias|WorkflowEvents|CreateSEOContentAction' -count=1`

Expected: PASS.

### Task 3: Frontend API Types and Lifecycle Helpers

**Files:**
- Modify: `web/app/lib/api.ts`
- Create: `web/app/lib/visibility-lifecycle.ts`
- Test: `web/app/lib/visibility-summary-contract.test.mjs`

- [x] **Step 1: Write the failing frontend contract test**

Create `web/app/lib/visibility-summary-contract.test.mjs` asserting `VisibilitySummary`, `VisibilityLifecycleStage`, `getVisibilitySummary`, `deriveVisibilityLifecycleStage`, and `lifecycle_counts` are present.

- [x] **Step 2: Run the red test**

Run: `npm test --prefix web -- app/lib/visibility-summary-contract.test.mjs`

Expected: FAIL because the types/helper/API do not exist.

- [x] **Step 3: Implement client contract**

Add exported TypeScript types, normalize empty arrays/maps, and add `getVisibilitySummary(projectId)`.

- [x] **Step 4: Implement shared lifecycle helper**

Add a pure helper that maps raw opportunity/action/topic/article fields to the PRD lifecycle stage.

- [x] **Step 5: Run the green test**

Run: `npm test --prefix web -- app/lib/visibility-summary-contract.test.mjs`

Expected: PASS.

### Task 4: Analysis Page Loop in Motion

**Files:**
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Test: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [x] **Step 1: Extend existing contract test**

Add assertions that Analysis renders `Loop in motion`, lifecycle-specific counts, `View measurement`, and does not rely only on raw `active tasks`.

- [x] **Step 2: Run the red test**

Run: `npm test --prefix web -- app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: FAIL because the loop summary is not present.

- [x] **Step 3: Render lifecycle summary**

Fetch `api.getVisibilitySummary(projectId)`, use it to render compact loop status above or beside the decision queue, and keep open review cards first.

- [x] **Step 4: Run the green test**

Run: `npm test --prefix web -- app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: PASS.

### Task 5: Measurement Closure

**Files:**
- Modify: `internal/workflow/worker.go`
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/db/queries/seo.sql`
- Generated: `internal/db/seo.sql.go`
- Generated: `internal/db/querier.go`
- Test: `internal/scheduler/measurement_closure_contract_test.go`

- [x] **Step 1: Write the failing scheduler contract test**

Create `internal/scheduler/measurement_closure_contract_test.go` asserting `workflow.EventMeasurementWindowDue` is handled, due measuring actions are selected, `outcome_summary` is updated, and completed/inconclusive outcomes are supported.

- [x] **Step 2: Run the red test**

Run: `go test ./internal/scheduler -run MeasurementClosure -count=1`

Expected: FAIL because the event is defined but not processed.

- [x] **Step 3: Add due action queries**

Add queries to select due `measuring` content actions from `measurement_window` and update `outcome_summary` plus status.

- [x] **Step 4: Implement scheduler handler**

Handle `measurement.window_due` by computing a conservative outcome from available action/window data. If data is insufficient, write an `inconclusive` result instead of pretending lift.

- [x] **Step 5: Run the green test**

Run: `go test ./internal/scheduler -run MeasurementClosure -count=1`

Expected: PASS.

### Task 6: Verification, Build, PR, Deployment

**Files:**
- All modified files

- [x] **Step 1: Run backend tests**

Run: `go test ./...`

Expected: PASS.

- [x] **Step 2: Run frontend tests and typecheck**

Run: `npm test --prefix web && npm run typecheck --prefix web`

Expected: PASS.

- [x] **Step 3: Run build checks**

Run: `go build ./... && npm run build --prefix web`

Expected: PASS.

- [ ] **Step 4: Commit and push**

Run: `git status --short`, then stage only this branch's files and commit with `feat: close visibility analysis content loop`.

- [ ] **Step 5: Create PR to `origin/main`, merge, wait for deployment**

Use the repository's available GitHub/Vercel tooling. Record the PR URL and production deployment URL.

- [ ] **Step 6: Verify production**

Verify the production Analysis/Results flow after redeployment: opportunity review remains first, loop-in-motion state appears, accepting an opportunity creates a content action, and measurement state renders without false GSC metrics for disconnected projects.
