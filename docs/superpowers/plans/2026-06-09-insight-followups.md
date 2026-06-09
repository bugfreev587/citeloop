# Insight Follow-Ups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the post-merge security and observability gaps from PR #9 while preserving fast landing-page onboarding.

**Architecture:** Add a shared web auth configuration helper so production fails closed when Clerk is missing, while preview/development can still bypass. Update background Insight crawl to record crawl failures and upgrade the active profile after full corpus crawl succeeds.

**Tech Stack:** Go Insight agent and crawler, sqlc `db.Queries`, Next.js middleware/server components, Node test runner.

---

### Task 1: Production Clerk Fail-Closed

**Files:**
- Create: `web/app/lib/auth-config.ts`
- Modify: `web/middleware.ts`
- Modify: `web/app/page.tsx`
- Modify: `web/app/projects/[id]/layout.tsx`
- Test: `web/app/lib/middleware-contract.test.mjs`
- Test: `web/app/lib/clerk-auth-contract.test.mjs`

- [ ] Add failing tests that require shared auth configuration and production fail-closed behavior.
- [ ] Implement `clerkServerAuthConfigured`, `allowUnconfiguredClerkBypass`, and `requireConfiguredClerk` in `web/app/lib/auth-config.ts`.
- [ ] Use the helper in middleware, home page, and project layout.
- [ ] Run `cd web && node --test app/lib/middleware-contract.test.mjs app/lib/clerk-auth-contract.test.mjs`.

### Task 2: Full Crawl Profile Upgrade

**Files:**
- Modify: `internal/agents/insight.go`
- Test: `internal/agents/insight_test.go`

- [ ] Add a failing unit test proving the full-crawl profile step emits source URLs that include article URLs.
- [ ] Update `RunInventoryFromCrawl` to extract and save a full-corpus profile after the crawl succeeds.
- [ ] Record the full-crawl profile run with `scope: "full_crawl"` and source metadata.
- [ ] Run `go test -count=1 ./internal/agents`.

### Task 3: Crawl Failure Run Recording

**Files:**
- Modify: `internal/agents/insight.go`
- Test: `internal/agents/insight_test.go`

- [ ] Add a failing unit test proving background crawl failures produce an `insight` generation run with `status: "error"`.
- [ ] Record a `step: "crawl"` run before returning crawl errors from `RunInventoryFromCrawl`.
- [ ] Run `go test -count=1 ./internal/agents`.

### Task 4: Full Verification and Delivery

**Files:**
- No additional files.

- [ ] Run `go test -count=1 ./...`.
- [ ] Run `cd web && npm test`.
- [ ] Run `cd web && npm run typecheck`.
- [ ] Run `git diff --check`.
- [ ] Commit, push, create PR, wait for deployment, and verify the deployed page.
