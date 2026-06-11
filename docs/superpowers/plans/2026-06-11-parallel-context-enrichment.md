# Parallel Context Enrichment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make CiteLoop show a fast provisional product profile while full crawl, inventory evidence, SEO sync, and full profile upgrade continue in bounded parallel background work.

**Architecture:** Keep the existing landing-only quick profile path as the first active profile. After it completes, start full inventory/profile enrichment in its own detached worker and let SEO sync/analyze proceed independently. Inside inventory persistence, use a small worker pool so per-page LLM extraction runs concurrently without unbounded provider pressure.

**Tech Stack:** Go API, sqlc-backed PostgreSQL queries, existing Insight agent and crawler, existing Next.js progress UI.

---

### Task 1: Parallelize Post-Profile Onboarding Work

**Files:**
- Modify: `internal/api/onboarding.go`
- Test: `internal/api/onboarding_test.go`

- [ ] **Step 1: Write the failing test**

Add a test with an `OnboardingRunner`-level server fixture proving `runProjectOnboarding` starts inventory crawl before SEO sync blocks.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api -run TestRunProjectOnboardingStartsInventoryBeforeSEOCompletes`
Expected: FAIL because `runProjectOnboarding` calls `runInsightInventoryCrawl` synchronously before SEO starts.

- [ ] **Step 3: Write minimal implementation**

Change `runProjectOnboarding` to call `s.startInsightInventoryCrawl(...)` after quick profile succeeds, so inventory has its own detached timeout and SEO can continue in the onboarding goroutine.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/api -run TestRunProjectOnboardingStartsInventoryBeforeSEOCompletes`
Expected: PASS.

### Task 2: Add Bounded Inventory Worker Pool

**Files:**
- Modify: `internal/agents/insight.go`
- Test: `internal/agents/insight_test.go`

- [ ] **Step 1: Write the failing test**

Add a test proving multiple article inventory extractions overlap but never exceed a small fixed worker limit.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agents -run TestPersistInventoryUsesBoundedParallelWorkers`
Expected: FAIL because `persistInventory` is sequential.

- [ ] **Step 3: Write minimal implementation**

Replace the sequential inventory loop with a deterministic bounded worker pool using a small constant concurrency limit. Keep per-page errors isolated and continue saving successful inventory rows.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agents -run TestPersistInventoryUsesBoundedParallelWorkers`
Expected: PASS.

### Task 3: Preserve Quality And Add Timing Observability

**Files:**
- Modify: `internal/agents/insight.go`
- Test: `internal/agents/insight_test.go`

- [ ] **Step 1: Write the failing test**

Extend run-capture tests to assert quick and full profile run outputs include stage and duration metadata.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agents -run 'TestRunQuickProfileRecordsProvisionalStage|TestRunInventoryFromCrawlUpgradesProfileWithArticleSources'`
Expected: FAIL until run outputs include the metadata.

- [ ] **Step 3: Write minimal implementation**

Add timing metadata to generation run outputs for provisional and full profile extraction. Keep full profile upgrade replacing the active profile only after extraction and save succeed.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agents -run 'TestRunQuickProfileRecordsProvisionalStage|TestRunInventoryFromCrawlUpgradesProfileWithArticleSources'`
Expected: PASS.

### Task 4: Full Verification And Main Push

**Files:**
- Verify only.

- [ ] **Step 1: Run backend tests**

Run: `go test ./internal/api ./internal/agents ./internal/crawl`
Expected: PASS.

- [ ] **Step 2: Run frontend checks**

Run: `cd web && npm test`
Run: `cd web && npm run typecheck`
Run: `cd web && npm run build`
Expected: PASS.

- [ ] **Step 3: Push to main and verify deployment**

Run: `git push origin HEAD:main`
Expected: push succeeds, Vercel production deployment becomes READY, and the live app loads without blocking errors.
