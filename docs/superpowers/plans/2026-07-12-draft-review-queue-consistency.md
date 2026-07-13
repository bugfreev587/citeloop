# Draft and Review Queue Consistency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep a drafted content brief on exactly one correct workflow surface and make existing-draft messages match the actual Review queue.

**Architecture:** Add pure content-plan classification helpers that combine the linked canonical state with the topic-level pending Review index already loaded by the page. Preserve duplicate-generation protection in the API while returning a state-aware response that separates pending Review articles from advanced articles.

**Tech Stack:** Go 1.x, pgx/sqlc models, Next.js 15, TypeScript, Node test runner.

---

### Task 1: Topic-level Content Plan handoff classification

**Files:**
- Modify: `web/app/lib/content-plan-logic.ts`
- Test: `web/app/lib/content-plan-logic.test.mjs`
- Modify: `web/app/projects/[id]/topics/topics-client.tsx`

- [ ] **Step 1: Write failing helper tests**

Add tests that require a pending sibling article ID to win when the linked canonical is approved, and require an approved-only linked draft to count as already handed off rather than accepted.

- [ ] **Step 2: Verify the tests fail**

Run: `cd web && node --test app/lib/content-plan-logic.test.mjs`

Expected: FAIL because the topic-level Review resolver and completed-handoff helper do not exist.

- [ ] **Step 3: Implement the pure helpers**

Add helpers that return the pending Review article ID from the linked article or topic index and identify linked drafts that have advanced beyond Review while treating `rejected` as non-complete.

- [ ] **Step 4: Wire the helpers into TopicsClient**

Use the resolved pending article ID for `sentToReviewActions`, accepted-action exclusion, and Review deep links. Exclude completed handoffs from accepted work.

- [ ] **Step 5: Verify the focused web tests pass**

Run: `cd web && node --test app/lib/content-plan-logic.test.mjs app/lib/workflow-handoff-link-cards-contract.test.mjs`

Expected: PASS.

### Task 2: Accurate existing-draft API response

**Files:**
- Modify: `internal/api/handlers_agents.go`
- Test: `internal/api/topics_routes_test.go`
- Modify: `web/app/projects/[id]/topics/topics-client.tsx`

- [ ] **Step 1: Write a failing handler test**

Create a topic with one approved canonical and three pending variants. Assert that `generateTopic` reports the pending variants as Review articles without counting the approved canonical.

- [ ] **Step 2: Verify the handler test fails**

Run: `go test ./internal/api -run TestGenerateTopicExistingArticlesReportOnlyPendingReview -count=1`

Expected: FAIL because the response currently contains all non-rejected articles.

- [ ] **Step 3: Implement the minimal response split**

Keep all non-rejected rows as the duplicate-generation guard. Return `status: ready` with only pending Review rows when any exist; otherwise return `status: advanced` without describing advanced rows as Review drafts.

- [ ] **Step 4: Update client messaging**

Keep the existing Review-count message for `ready`; add an `advanced` message explaining that the brief has already moved beyond Review.

- [ ] **Step 5: Verify focused Go and web tests pass**

Run: `go test ./internal/api -run 'TestGenerateTopic(ReconcilesExistingDraftTopic|ExistingArticlesReportOnlyPendingReview)' -count=1`

Run: `cd web && node --test app/lib/content-plan-logic.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: PASS.

### Task 3: Full verification and delivery

**Files:**
- Verify all modified files and generated artifacts.

- [ ] **Step 1: Run formatting and focused verification**

Run: `gofmt -w internal/api/handlers_agents.go internal/api/topics_routes_test.go`

Run: `go test ./internal/api ./internal/topicstate`

Run: `cd web && npm test && npm run typecheck`

Expected: PASS with no test failures or type errors.

- [ ] **Step 2: Run repository-wide verification**

Run: `go test ./...`

Run: `cd web && npm run build`

Expected: PASS.

- [ ] **Step 3: Review the diff and commit**

Confirm only the queue classification, state-aware response, tests, spec, and plan changed. Commit with `fix: align draft status with review queue`.

- [ ] **Step 4: Create and merge the PR**

Push `codex/fix-draft-review-queue-count`, open a PR against `origin/main`, wait for checks, and merge it.

- [ ] **Step 5: Verify production**

Wait for deployment, reload project `1459b054-cdc3-4d9b-9dd4-18e12458c61a`, and verify the UniPost topic is not in Accepted content work while pending variants appear under Review/Recently Drafted with matching counts and correct links.

