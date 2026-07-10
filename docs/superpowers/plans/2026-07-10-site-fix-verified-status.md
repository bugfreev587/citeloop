# Site Fix Verified Status Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make automatically verified Site Fixes persist and display their completed state instead of looking like open pull requests.

**Architecture:** Add one Site Fix-specific SQL statement that updates the application and content action lifecycle/publisher-result state atomically. Derive explicit verification and PR-link labels in the shared frontend helper, then consume them in the dedicated Site Fixes page.

**Tech Stack:** Go 1.25, PostgreSQL/sqlc, Next.js/TypeScript, Node test runner

---

### Task 1: Persist the verified publisher result

**Files:**
- Modify: `internal/scheduler/sitefix_verify_test.go`
- Modify: `internal/db/queries/seo.sql`
- Modify: `internal/db/seo.sql.go`
- Modify: `internal/db/querier.go`
- Modify: `internal/scheduler/sitefix_verify.go`
- Test: `internal/api/content_action_verification_contract_test.go`

- [ ] **Step 1: Write failing scheduler and SQL contract tests**

Add a scheduler unit test that calls `siteFixVerifiedPublisherResult` for a merged PR and asserts the decoded payload includes `status == "verified"`, `github_pr_state == "merged"`, the PR URL, `verification_source`, and the RFC3339 verification time. Extend the database contract test to require `MarkSiteChangeApplicationAndContentActionVerified`, with one data-modifying CTE that updates both tables and sets `status = 'measuring'`, `verified_at`, `verification_snapshot`, and `output_snapshot.publisher_result`.

- [ ] **Step 2: Run the tests and verify RED**

Run:

```bash
go test ./internal/scheduler ./internal/db
```

Expected: FAIL because `siteFixVerifiedPublisherResult` and `MarkSiteChangeApplicationAndContentActionVerified` do not exist.

- [ ] **Step 3: Add the atomic SQL update and regenerate sqlc**

Add this query after `MarkContentActionSiteFixPRResult`:

```sql
-- name: MarkSiteChangeApplicationAndContentActionVerified :one
with verified_application as (
  update site_change_applications set
    status = 'verified',
    deployment_snapshot = sqlc.arg(deployment_snapshot)::jsonb,
    verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
    failure_reason = null,
    deployed_at = coalesce(deployed_at, sqlc.arg(verified_at)::timestamptz),
    verified_at = coalesce(verified_at, sqlc.arg(verified_at)::timestamptz),
    updated_at = now()
  where site_change_applications.id = sqlc.arg(application_id)
    and site_change_applications.project_id = sqlc.arg(project_id)
  returning content_action_id
)
update content_actions set
  status = 'measuring',
  verified_at = sqlc.arg(verified_at)::timestamptz,
  verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
  output_snapshot = coalesce(output_snapshot, '{}'::jsonb) ||
    jsonb_build_object('publisher_result', sqlc.arg(publisher_result)::jsonb),
  updated_at = now()
from verified_application
where content_actions.id = verified_application.content_action_id
  and content_actions.project_id = sqlc.arg(project_id)
returning content_actions.*;
```

Run `make sqlc` to regenerate `internal/db/seo.sql.go` and `internal/db/querier.go`.

- [ ] **Step 4: Build and persist the verified publisher result**

Add `siteFixVerifiedPublisherResult(app, source, now)` in `internal/scheduler/sitefix_verify.go`. It must retain the application and PR identity fields while setting:

```go
"status":              "verified",
"github_pr_state":     "merged",
"verification_source": source,
"verified_at":          now.UTC().Format(time.RFC3339),
```

Replace the separate application and content-action updates with `MarkSiteChangeApplicationAndContentActionVerified`, passing the application ID, deployment snapshot, verification time/snapshot, and new publisher result.

- [ ] **Step 5: Run the backend tests and verify GREEN**

Run:

```bash
go test ./internal/scheduler ./internal/db
```

Expected: PASS.

### Task 2: Render automatic verification and accurate PR state

**Files:**
- Modify: `web/app/lib/action-portfolio-contract.test.mjs`
- Modify: `web/app/lib/site-fix.ts`
- Modify: `web/app/projects/[id]/site-fixes/site-fixes-client.tsx`

- [ ] **Step 1: Write the failing web contract test**

Require the shared helper to expose `siteFixVerificationLabel` with `Verified automatically`, and `siteFixPRLinkLabel` with `View merged PR`. Require the Site Fixes client to use both helpers and guard the `Mark applied` button with `!drawerAction.verified_at`.

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd web && node --test app/lib/action-portfolio-contract.test.mjs
```

Expected: FAIL because the new helpers and UI guards are absent.

- [ ] **Step 3: Implement the shared labels**

In `web/app/lib/site-fix.ts`, add:

```ts
export function siteFixVerificationLabel(action: SEOContentAction | ResultsAction) {
  if (!action.verified_at) return "";
  const source = String(action.verification_snapshot?.source ?? "").trim().toLowerCase();
  return source.startsWith("auto_") ? "Verified automatically" : "Verified";
}

export function siteFixPRLinkLabel(action: SEOContentAction | ResultsAction) {
  const result = action.output_snapshot?.publisher_result ?? {};
  const status = siteFixPublisherResultStatus(action);
  const state = String(result.github_pr_state ?? "").trim().toLowerCase();
  if (status === "github_pr_closed" || state === "closed") return "View closed PR";
  if (status === "github_pr_open" || state === "open") return "Open PR";
  if (status === "github_pr_merged" || state === "merged") return "View merged PR";
  if (action.verified_at || status === "verified") return "View merged PR";
  return "Open PR";
}
```

- [ ] **Step 4: Consume the labels in Site Fix cards and the drawer**

Import both helpers. Show the verification label as a green badge on verified cards and in the drawer, use `siteFixPRLinkLabel(drawerAction)` for the PR button text, show the same verification label in the Verification field, and render the manual `Mark applied` button only when `verified_at` is absent.

- [ ] **Step 5: Run the focused test and verify GREEN**

Run:

```bash
cd web && node --test app/lib/action-portfolio-contract.test.mjs
```

Expected: PASS.

### Task 3: Verify and publish

**Files:**
- Review all modified files

- [ ] **Step 1: Run formatting and targeted tests**

Run:

```bash
gofmt -w internal/scheduler/sitefix_verify.go internal/scheduler/sitefix_verify_test.go internal/api/content_action_verification_contract_test.go
go test ./internal/scheduler ./internal/db
cd web && node --test app/lib/action-portfolio-contract.test.mjs
```

Expected: all commands pass.

- [ ] **Step 2: Run full verification**

Run:

```bash
make test
cd web && npm test && npm run typecheck && npm run build
```

Expected: all commands pass with no test, type, or build failures.

- [ ] **Step 3: Review the diff and commit**

Run `git diff --check`, inspect `git diff`, then commit the implementation with a focused message.

- [ ] **Step 4: Push, open, and merge the PR**

Push `codex/site-fix-verified-status`, open a PR against `origin/main`, wait for required checks, and merge it.

- [ ] **Step 5: Verify deployments and production**

Wait for Railway and Vercel to deploy the merge commit. Confirm the production API is on the merged revision, then verify the project Site Fixes page shows `Verified automatically`, `View merged PR`, and no `Mark applied` control for PR #175.
