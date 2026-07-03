# Doctor Read-Only PRD Execution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align the shipped Doctor implementation with the revised read-only SEO/GEO diagnosis PRD while preserving legacy report readability.

**Architecture:** Backend Doctor runs remain project-authenticated and report-only; new runs must not enter a handoff stage or create Growth Loop objects. The canonical API namespace moves to `/api/projects/{projectID}/doctor`, with `/seo/doctor` retained as a compatibility alias, and the frontend Doctor report removes per-finding repair/action affordances from the default report.

**Tech Stack:** Go service/API with sqlc queries, Next.js TypeScript frontend, Node contract tests, Go unit and route tests.

---

### Task 1: Backend Doctor Run Contract

**Files:**
- Modify: `internal/seo/doctor_test.go`
- Modify: `internal/seo/doctor.go`

- [x] **Step 1: Write the failing test**

Add a test that asserts `DoctorStageHandoff` is not in the new-run progress sequence and that `writing_report` progresses directly toward completion.

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/seo -run 'TestDoctorProgressSequenceExcludesHandoffForNewRuns'`
Observed: failed because the existing sequence still included `handoff`.

- [x] **Step 3: Write minimal implementation**

Remove `DoctorStageHandoff` from `doctorStageOrder` and from the new-run progress map while keeping the constant for historical rows.

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./internal/seo -run 'TestDoctorProgressSequenceExcludesHandoffForNewRuns|TestDoctorProgressInterpolatesWithinCheckingStage'`
Observed: pass.

### Task 2: Canonical Doctor API Namespace

**Files:**
- Modify: `internal/api/seo_routes_test.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/handlers_seo_doctor.go`

- [x] **Step 1: Write the failing test**

Update route registration tests so canonical `/api/projects/{id}/doctor` routes are registered, including `runs`, `latest`, `findings`, and `start-growth-loop`. Keep legacy `/seo/doctor` read-compatible.

- [x] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api -run 'TestSEORoutesAreRegistered|TestLegacyDoctorConvertRouteIsNotRegistered'`
Observed: failed because canonical `/doctor` routes were not registered and legacy convert was still live.

- [x] **Step 3: Write minimal implementation**

Add a shared `registerDoctorRoutes` helper used by both `/doctor` and `/seo/doctor`. Keep create/read/latest/findings/dismiss on both namespaces; add a start-growth-loop endpoint that explicitly requires selected finding IDs and uses the conversion service path only after user confirmation.

- [x] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api -run 'TestSEORoutesAreRegistered|TestLegacyDoctorConvertRouteIsNotRegistered'`
Observed: pass.

### Task 3: Frontend Read-Only Doctor Contract

**Files:**
- Modify: `web/app/lib/seo-doctor-contract.test.mjs`
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/projects/[id]/doctor/doctor-client.tsx`
- Modify: `web/app/projects/[id]/workspace.tsx`

- [x] **Step 1: Write the failing test**

Change the contract test to require canonical `/doctor` client calls and to forbid default Doctor UI strings/functions for `Fix with AI`, `Create action`, `buildAIRepairPayload`, and `convertSEODoctorFinding`.

- [x] **Step 2: Run test to verify it fails**

Run: `cd web && node --test app/lib/seo-doctor-contract.test.mjs`
Observed: failed because current UI still exposed AI repair and convert affordances.

- [x] **Step 3: Write minimal implementation**

Move API calls to `/projects/{id}/doctor`, remove default per-finding AI repair modal/action conversion UI, keep dismiss as report hygiene, and update copy from repair language to diagnosis language.

- [x] **Step 4: Run test to verify it passes**

Run: `cd web && node --test app/lib/seo-doctor-contract.test.mjs`
Observed: pass.

### Task 4: Verification and Release

**Files:**
- Verify: `docs/PRD-CiteLoop-SEO-Doctor.md`
- Verify: backend and frontend tests

- [x] **Step 1: Run focused verification**

Run: `go test ./internal/seo ./internal/api ./internal/db`
Run: `cd web && npm test`
Run: `cd web && npm run typecheck`
Run: `cd web && npm run build`
Run: `git diff --check`
Observed: all commands exited 0. `next build` emitted the existing multiple-lockfile root warning.

- [ ] **Step 2: Push, PR, merge, and production verify**

Push `codex/doctor-readonly-prd`, create a PR to `origin/main`, merge it after checks, wait for deployment, then verify production Doctor is authenticated, canonical project route loads, and the report has no default per-finding AI repair/action handoff.
