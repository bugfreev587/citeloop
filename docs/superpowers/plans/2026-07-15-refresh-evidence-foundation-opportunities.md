# Refresh Evidence Substeps and Foundation Starter Opportunities Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Opportunity Finding transparent during evidence refresh and ensure Foundation-stage manual runs produce actionable starter opportunities when discovery finds credible but not yet strong growth evidence.

**Architecture:** Keep the existing six checkpointed Opportunity Finding stages. Add user-facing substeps inside the `evidence_refresh` stage summary, including duration/count/status. Add a Foundation-only starter path after normal AI hypothesis materialization: if manual discovery generates candidates but creates no opportunities because early-stage evidence is incomplete, promote a limited number of safe watchlist candidates into decision-ready starter opportunities.

**Tech Stack:** Go backend (`internal/opportunityfinding`, `internal/scheduler`, `internal/geo`, `internal/api`), Next.js frontend (`web/app`), Node contract tests, Go unit tests.

---

### Task 1: Evidence refresh substep contract

**Files:**
- Modify: `internal/opportunityfinding/ai_discovery.go`
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/opportunityfinding/ai_discovery_test.go`
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/projects/[id]/seo/opportunity-finding-progress.tsx`
- Modify: `web/app/lib/seo-client-contract.test.mjs`

- [ ] Write failing Go tests proving `AIDiscoveryStep` includes `duration_ms` and `RefreshAIDiscoveryEvidence` records named substeps for planning, search/competitive recall, seed enrichment, AI observations, and site/surface audit.
- [ ] Run `go test ./internal/opportunityfinding -run 'TestRefreshAIDiscoveryEvidence'` and confirm the new assertions fail.
- [ ] Add `DurationMs int64` to `AIDiscoveryStep` and introduce a small timer helper so sequential and goroutine-backed steps record elapsed time.
- [ ] In `executeOpportunityFindingStage`, convert signal scan and AI discovery steps into a common `substeps` list on the `evidence_refresh` stage summary.
- [ ] Extend the frontend type normalization to preserve substep arrays from stage summaries.
- [ ] Render `substeps` nested under `Refresh evidence` with label, status, count, duration, and error.
- [ ] Run the targeted Go and Node tests until they pass.

### Task 2: Foundation starter opportunities

**Files:**
- Modify: `internal/growthradar/score.go`
- Modify: `internal/growthradar/score_test.go`
- Modify: `internal/geo/pr3.go`
- Modify: `internal/geo/service_pr3_test.go`
- Modify: `internal/opportunityfinding/ai_discovery.go`
- Modify: `internal/opportunityfinding/ai_discovery_test.go`
- Modify: `internal/api/handlers_seo.go`
- Modify: `web/app/lib/growth-radar.ts`
- Modify: `web/app/lib/growth-radar.test.mjs`

- [ ] Write failing score tests for a Foundation starter disposition: safe, non-duplicate, non-sensitive candidates in the watchlist range can be promoted only for Foundation.
- [ ] Run `go test ./internal/growthradar -run Foundation` and confirm the test fails.
- [ ] Add a Foundation-specific `starter_opportunity` disposition or reason code that preserves lower confidence while allowing materialization.
- [ ] In GEO materialization, route `starter_opportunity` candidates through the existing canonical opportunity writer with explicit evidence metadata (`foundation_starter: true`, original reason codes, starter rationale).
- [ ] Limit starter creation to manual runs and to a small cap, default 3, so scheduled jobs do not flood the queue.
- [ ] Ensure sensitive/internal/duplicate candidates remain filtered or watchlisted.
- [ ] Update zero-result API explanation so a successful starter run is not labeled as zero-result.
- [ ] Run targeted Go tests until they pass.

### Task 3: Production-facing UX and copy

**Files:**
- Modify: `web/app/projects/[id]/seo/opportunity-finding-progress.tsx`
- Modify: `web/app/lib/growth-radar.ts`
- Modify: `web/app/lib/seo-client-contract.test.mjs`
- Modify: `web/app/lib/growth-radar.test.mjs`

- [ ] Write failing frontend contract tests that completed runs show substeps after completion and that Foundation starter results are described as actionable starter recommendations.
- [ ] Implement concise copy: "Foundation starter opportunity" and "Build the first citable asset/evidence base."
- [ ] Keep strict opportunities visually distinct from starter opportunities through text, not a separate queue.
- [ ] Run targeted Node tests until they pass.

### Task 4: Full validation and release

**Files:**
- No new files beyond the implementation files above.

- [ ] Run `go test ./...`.
- [ ] Run `cd web && npm install` if dependencies are missing.
- [ ] Run `cd web && npm test`.
- [ ] Run `cd web && npm run build`.
- [ ] Commit changes.
- [ ] Push branch.
- [ ] Open PR to `main`.
- [ ] Wait for CI and deployment checks.
- [ ] Merge PR.
- [ ] Verify production `citeloop.app` includes substep UI and Foundation starter copy in the deployed bundle; verify API deployment succeeds.
