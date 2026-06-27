# Context Refresh Controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement fixed-domain Context refresh controls with 24-hour manual cooldown, last crawl timestamps, and weekly lightweight automatic refresh.

**Architecture:** Store crawl metadata in the active product profile JSON and route all post-setup context refreshes through a new `POST /projects/{id}/context/refresh` endpoint that reads `config.site_url`. Reuse the existing detached insight inventory crawl path, but add source-aware metadata updates and a weekly scheduler tick.

**Tech Stack:** Go Chi API, sqlc query layer, existing scheduler cron, Next.js App Router client component, node contract tests.

---

### Task 1: Backend Refresh Metadata and Manual Endpoint

**Files:**
- Modify: `internal/api/handlers_agents.go`
- Modify: `internal/api/onboarding.go`
- Modify: `internal/api/server.go`
- Test: `internal/api/context_refresh_test.go`

- [ ] **Step 1: Write failing API tests**

Create `internal/api/context_refresh_test.go` with tests that:

- `POST /projects/{id}/context/refresh` uses `config.site_url` and ignores request body domains.
- A manual refresh within 24 hours returns `429`.
- A profile with `context_crawl_started_at` returns `409`.
- `/insight` rejects a different `landing_url` when `config.site_url` exists.

- [ ] **Step 2: Verify red**

Run: `GOCACHE=/private/tmp/citeloop-go-cache go test ./internal/api -run 'ContextRefresh|RunInsight'`

Expected: FAIL because the new route and validation are missing.

- [ ] **Step 3: Implement minimal endpoint**

Add `r.Post("/context/refresh", s.refreshContext)` in `internal/api/server.go`.

Implement `refreshContext` to load project config, require `cfg.SiteURL`, load active profile, enforce:

- no `context_crawl_started_at`
- no manual crawl newer than 24 hours

Then save profile metadata with `context_crawl_started_at` and `context_crawl_source: "manual"`, start the detached crawl using `cfg.SiteURL`, and return the updated profile.

- [ ] **Step 4: Verify green**

Run: `GOCACHE=/private/tmp/citeloop-go-cache go test ./internal/api -run 'ContextRefresh|RunInsight'`

Expected: PASS.

### Task 2: Crawl Completion Metadata and Confirmation Preservation

**Files:**
- Modify: `internal/agents/insight.go`
- Modify: `internal/api/onboarding.go`
- Test: `internal/agents/insight_test.go`
- Test: `internal/api/onboarding_test.go`

- [ ] **Step 1: Write failing tests**

Add tests proving a full crawl preserves `context_confirmed_at` and writes `context_last_crawled_at` when saving the active profile.

- [ ] **Step 2: Verify red**

Run: `GOCACHE=/private/tmp/citeloop-go-cache go test ./internal/agents ./internal/api -run 'Insight|ContextRefresh|Onboarding'`

Expected: FAIL because full crawl overwrites profile metadata.

- [ ] **Step 3: Implement preservation**

Before inserting the new full-crawl profile, merge metadata keys from the existing active profile into the new profile JSON:

- `context_confirmed_at`
- `confirmed_at`
- `context_last_manual_crawled_at`
- `context_last_crawled_at`

On crawl completion, set `context_last_crawled_at` to now, clear `context_crawl_started_at`, and if source is manual, set `context_last_manual_crawled_at`.

- [ ] **Step 4: Verify green**

Run: `GOCACHE=/private/tmp/citeloop-go-cache go test ./internal/agents ./internal/api -run 'Insight|ContextRefresh|Onboarding'`

Expected: PASS.

### Task 3: Weekly Lightweight Scheduler

**Files:**
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/scheduler/helpers.go`
- Test: `internal/scheduler/helpers_test.go`
- Test: `internal/scheduler/context_refresh_test.go`

- [ ] **Step 1: Write failing scheduler tests**

Add tests proving `TickContextRefresh` exists, `Start` registers it with `@weekly`, and the tick skips projects without `site_url` or without confirmed Context.

- [ ] **Step 2: Verify red**

Run: `GOCACHE=/private/tmp/citeloop-go-cache go test ./internal/scheduler -run 'ContextRefresh|StartRegisters'`

Expected: FAIL because the tick is missing.

- [ ] **Step 3: Implement scheduler tick**

Add `TickContextRefresh(ctx)` that lists projects, parses config, gets active profile, checks confirmation and last crawl age, and starts a lightweight refresh with:

- `MaxPages: 5`
- `SitemapURLCap: 20`
- `RequestTimeoutMs: 4000`
- `RateLimitRPS: 3`

- [ ] **Step 4: Verify green**

Run: `GOCACHE=/private/tmp/citeloop-go-cache go test ./internal/scheduler -run 'ContextRefresh|StartRegisters'`

Expected: PASS.

### Task 4: Frontend API and Context UI

**Files:**
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/normalize.ts`
- Modify: `web/app/projects/[id]/knowledge/knowledge-client.tsx`
- Test: `web/app/lib/api.test.mjs`
- Test: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Write failing frontend tests**

Update contract tests to require:

- `createApi().refreshContext(projectId)` calls `/projects/{id}/context/refresh`.
- connected Context does not render the `https://product-domain.com` editable input.
- the connected panel includes `Update context` and `Last updated`.
- the button uses cooldown/running disabled copy.

- [ ] **Step 2: Verify red**

Run: `npm test -- --test-name-pattern='context|api'`

Expected: FAIL because the API method and UI contract are missing.

- [ ] **Step 3: Implement minimal UI**

Add `refreshContext(id)` to the API client. Add crawl metadata fields to `ProductProfile` normalization. Replace the connected-domain form with an action row:

- `Update context` button
- `Last updated {formatDate(profile.profile.context_last_crawled_at ?? profile.updated_at)}`
- cooldown helper text when `context_last_manual_crawled_at` is within 24 hours

Keep the setup form for projects without a profile.

- [ ] **Step 4: Verify green**

Run: `npm test -- --test-name-pattern='context|api'`

Expected: PASS.

### Task 5: Full Verification and Integration

**Files:**
- All changed files

- [ ] **Step 1: Format**

Run: `gofmt -w internal/api internal/agents internal/scheduler`

- [ ] **Step 2: Backend tests**

Run: `GOCACHE=/private/tmp/citeloop-go-cache go test ./internal/api ./internal/agents ./internal/scheduler`

Expected: PASS.

- [ ] **Step 3: Frontend tests**

Run: `npm test -- --test-name-pattern='context|knowledge|api'`

Expected: PASS.

- [ ] **Step 4: Typecheck**

Run: `npm run typecheck`

Expected: PASS.

- [ ] **Step 5: Commit implementation**

Commit the finished change on `codex/context-refresh-controls-impl`.
