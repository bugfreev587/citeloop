# Multi-Surface SEO Growth Phase 6 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a lightweight content action verification loop that records `verified_at`, `verification_snapshot`, and failure status on existing `content_actions`.

**Architecture:** Reuse `content_actions`; add one SQLC update query and one project-scoped API endpoint. The web dashboard calls the endpoint from action cards for manual verification while automated verifiers can later reuse the same contract.

**Tech Stack:** Go API/SQL source-contract tests, sqlc, Next.js API client/source-contract tests.

---

## Task 1: Backend Verification Contract

**Files:**
- Create: `internal/api/content_action_verification_contract_test.go`
- Modify: `internal/db/queries/seo.sql`
- Modify: `internal/api/server.go`
- Modify: `internal/api/handlers_seo.go`

- [ ] **Step 1: Write failing backend contract test**

Create `internal/api/content_action_verification_contract_test.go` checking for `MarkContentActionVerification`, `/verify`, `verifySEOContentAction`, `verification_failed`, `verified_at`, and `verification_snapshot`.

- [ ] **Step 2: Verify red**

Run `go test ./internal/api -run ContentActionVerification -count=1`; expect failure.

- [ ] **Step 3: Add SQLC query and handler**

Add `MarkContentActionVerification` to `internal/db/queries/seo.sql`, run `make sqlc`, add route `r.Post("/actions/{actionID}/verify", s.verifySEOContentAction)`, and implement `verifySEOContentAction` with statuses `verified`/`ok` -> `measuring`, `failed` -> `verification_failed`, `recovery_required` -> `recovery_required`.

- [ ] **Step 4: Verify green**

Run `go test ./internal/api -run ContentActionVerification -count=1`; expect pass.

## Task 2: Web Verification Control Contract

**Files:**
- Create: `web/app/lib/action-verification-contract.test.mjs`
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

- [ ] **Step 1: Write failing web contract test**

Create a contract test checking for `verifySEOContentAction`, `/verify`, `verification_snapshot`, `Manual verify`, `Verification failed`, and `verifyAction`.

- [ ] **Step 2: Verify red**

Run `cd web && node --test app/lib/action-verification-contract.test.mjs`; expect failure.

- [ ] **Step 3: Add API method and action-card controls**

Add `verifySEOContentAction(projectID, actionID, body)` to `api.ts`. Add `verifyAction(action, status)` in the SEO client and render manual verify/failure buttons in content action cards.

- [ ] **Step 4: Verify green**

Run the web contract test; expect pass.

## Task 3: Phase 6 Gate and Commit

- [ ] Run `make sqlc`, targeted tests, `make test`, `make build`, all web contract tests, and `VERCEL_ENV=preview npm run build`.
- [ ] Commit with `feat(seo): add content action verification loop`.

## Self-Review

- This phase records verification status only; it does not implement sitemap/canonical/link crawling workers yet.
- This phase reuses `content_actions` and does not introduce a parallel verification workflow.
