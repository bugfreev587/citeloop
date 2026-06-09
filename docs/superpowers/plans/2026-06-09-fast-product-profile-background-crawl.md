# Fast Product Profile Background Crawl Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make onboarding create a usable product profile from the landing page quickly while public crawl and content inventory continue in the background.

**Architecture:** Split Insight into a landing-only profile path and a full crawl inventory path. The API returns after the active profile is saved, then launches the slower crawl/inventory completion asynchronously with run records that the existing Activity UI can display.

**Tech Stack:** Go API with sqlc-generated PostgreSQL queries, internal crawler/agent packages, Next.js onboarding form, Node test runner for frontend contract tests.

---

### Task 1: Crawler Landing-Only Fetch

**Files:**
- Modify: `internal/crawl/crawler.go`
- Test: `internal/crawl/crawler_test.go`

- [ ] **Step 1: Write the failing test**
  Add a test proving `FetchLanding` returns the landing page without fetching sitemap/article URLs.

- [ ] **Step 2: Run test to verify it fails**
  Run: `go test ./internal/crawl -run TestFetchLandingSkipsDiscovery`
  Expected: compile failure because `FetchLanding` does not exist.

- [ ] **Step 3: Write minimal implementation**
  Add `Crawler.FetchLanding(ctx, landingURL)` that parses robots, fetches only the landing page, and returns a `Result{Landing: page}`.

- [ ] **Step 4: Run test to verify it passes**
  Run: `go test ./internal/crawl -run TestFetchLandingSkipsDiscovery`
  Expected: PASS.

### Task 2: Insight Quick Profile And Background Inventory

**Files:**
- Modify: `internal/agents/insight.go`
- Test: `internal/agents/insight_test.go`

- [ ] **Step 1: Write failing agent tests**
  Add tests for landing-only profile corpus, quick profile persistence source URLs, and background completion summary/inventory behavior where possible.

- [ ] **Step 2: Run tests to verify failure**
  Run: `go test ./internal/agents -run 'Test(ExtractProfileUsesLandingOnly|SummarizeCrawl|Insight)'`
  Expected: FAIL until new APIs exist.

- [ ] **Step 3: Implement minimal agent split**
  Add `RunQuickProfile`, `RunInventoryFromCrawl`, and shared profile persistence helpers. Keep `Run` behavior compatible by composing the new pieces synchronously.

- [ ] **Step 4: Run tests to verify pass**
  Run: `go test ./internal/agents`
  Expected: PASS.

### Task 3: API And Onboarding Async Insight

**Files:**
- Modify: `internal/api/handlers_agents.go`
- Modify: `internal/api/onboarding.go`
- Test: `internal/api/onboarding_test.go`

- [ ] **Step 1: Write failing onboarding tests**
  Add tests proving project onboarding calls quick profile first and then full inventory/SEO work in the detached background task.

- [ ] **Step 2: Run test to verify failure**
  Run: `go test ./internal/api -run TestStartProjectOnboarding`
  Expected: FAIL until the split onboarding runner exists.

- [ ] **Step 3: Implement async handler and onboarding split**
  Make `runInsight` return after `RunQuickProfile`, launch `RunInventoryFromCrawl` in a detached background task, and make project creation onboarding run quick profile before the slower full crawl/inventory/SEO sequence.

- [ ] **Step 4: Run API tests**
  Run: `go test ./internal/api -run 'Test(StartProjectOnboarding|RunRoutes|ProjectLifecycle)'`
  Expected: PASS.

### Task 4: Onboarding Contract

**Files:**
- Modify: `web/app/project-create-form.tsx`
- Test: `web/app/lib/project-create-form-contract.test.mjs`

- [ ] **Step 1: Write frontend contract test**
  Assert onboarding does not call `runInsight` or `syncSEO` from the form and that labels make the background job behavior visible.

- [ ] **Step 2: Run test**
  Run: `cd web && npm test -- app/lib/project-create-form-contract.test.mjs`
  Expected: PASS if latest `main` already has the non-blocking form, otherwise FAIL before the UI copy fix.

- [ ] **Step 3: Implement minimal form change if needed**
  Keep only `createProject` in the submit path, then route to the project immediately. Use copy that says profile/SEO jobs are started rather than completed synchronously.

- [ ] **Step 4: Run frontend tests**
  Run: `cd web && npm test -- app/lib/project-create-form-contract.test.mjs`
  Expected: PASS.

### Task 5: Verification

**Files:**
- Verify only.

- [ ] **Step 1: Run Go tests**
  Run: `go test ./...`
  Expected: PASS.

- [ ] **Step 2: Run frontend contract/type checks**
  Run: `cd web && npm test`
  Run: `cd web && npm run typecheck`
  Expected: PASS.

- [ ] **Step 3: Review diff**
  Run: `git diff --stat && git diff --check`
  Expected: scoped diff, no whitespace errors.
