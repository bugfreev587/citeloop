# Doctor Recent Findings Implementation Plan

> **For Codex:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Move Doctor findings with Site Fixes out of the active Findings grid, preserve eligible forward links in a Recent Findings drawer, and let users persistently dismiss only those links.

**Architecture:** `site_fixes` retains immutable Doctor provenance and gains per-revision Doctor-link dismissal metadata. A project-scoped, idempotent Doctor endpoint owns the presentation mutation. The Doctor client derives the active and recent sets from the report plus canonical Site Fixes, keeping Site Fix execution exclusively on the Site Fixes page.

**Tech Stack:** Go, PostgreSQL/sqlc, Chi, Next.js/React/TypeScript, Node contract tests, Railway, Vercel, Chrome production verification.

---

## Task 1: Persist Doctor-link dismissal without changing Site Fix lifecycle

**Files:**
- Create: `internal/migrations/0083_doctor_recent_finding_links.sql`
- Modify: `internal/db/doctor_site_fix_schema_contract_test.go`
- Modify: `internal/db/site_fixes_query_contract_test.go`
- Modify: `internal/db/queries/site_fixes.sql`
- Regenerate: `internal/db/models.go`
- Regenerate: `internal/db/querier.go`
- Regenerate: `internal/db/site_fixes.sql.go`

1. Add failing schema and query contracts requiring paired dismissal fields, a project-scoped Doctor-only update, and preservation of status, provenance, application, and revision identity.
2. Run the focused database contract tests and confirm they fail for the missing migration/query.
3. Add migration 0083 with `doctor_link_dismissed_at`, `doctor_link_dismissed_by`, and a paired-null consistency constraint.
4. Add `DismissCanonicalSiteFixDoctorLink` as an idempotent update that requires project ID and non-null `doctor_finding_id`, preserves the first actor/time, and returns the Site Fix.
5. Run sqlc generation and the focused database tests until green.
6. Commit the database slice.

## Task 2: Expose an authenticated idempotent dismiss-link endpoint

**Files:**
- Modify: `internal/api/handlers_site_fixes.go`
- Modify: `internal/api/handlers_seo_doctor.go`
- Modify: `internal/api/site_fixes_api_test.go`

1. Add failing service and route tests for `POST /projects/{projectID}/doctor/site-fixes/{siteFixID}/dismiss-link`.
2. Cover project scope, malformed IDs, Doctor provenance, authenticated actor propagation, idempotent success, returned dismissal metadata, and no lifecycle/apply/verify calls.
3. Extend `DoctorSiteFixService` with `DismissDoctorLink` and implement it through the generated query, mapping missing/non-Doctor rows to not found.
4. Add the canonical handler and route using the existing project authorization and error response conventions.
5. Run focused API tests until green, then run the internal API package tests.
6. Commit the API slice.

## Task 3: Add the web API contract and set derivation helpers

**Files:**
- Modify: `web/app/lib/types.ts`
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/api.test.mjs`
- Modify: `web/app/lib/seo-doctor-contract.test.mjs`
- Modify: `web/app/projects/[id]/doctor/doctor-client.tsx`

1. Add failing API tests requiring dismissal fields to normalize and the dismiss call to use the canonical POST route.
2. Add failing Doctor contracts requiring linked findings to leave the main grid, latest-per-finding recent selection, PR-backed exclusion, terminal-without-PR retention, and dismissed-link exclusion.
3. Extend `SiteFix`, normalize the two fields, and add `api.dismissDoctorSiteFixLink(projectID, siteFixID)`.
4. Add small pure helpers in the Doctor client for stable latest-per-finding selection and pull-request existence. Derive:
   - active findings from the absence of any linked Site Fix;
   - recent findings from the latest non-dismissed, no-PR Site Fix per current report finding.
5. Run the focused Node contracts and TypeScript checks until green.
6. Commit the data-contract slice.

## Task 4: Build the Recent Findings drawer and safe dismissal flow

**Files:**
- Modify: `web/app/projects/[id]/doctor/doctor-client.tsx`
- Modify: `web/app/lib/seo-doctor-contract.test.mjs`

1. Add failing UI contracts for an always-visible count button, mutually exclusive drawers, compact navigable rows, separate Dismiss controls, approved confirmation copy, optimistic-safe success handling, and unchanged Site Fix messaging.
2. Add `Recent Findings` to the Findings header and a right drawer with count, empty behavior, status/context labels, target URL, and deep links to `/projects/{id}/site-fixes?fix={siteFixID}`.
3. Add the accessible confirmation dialog with `Cancel` and `Dismiss link`, focus restoration, busy/error states, and background inert handling consistent with the existing Doctor drawer.
4. On success, replace the returned Site Fix in local state, keep the finding out of the primary grid, and show a toast stating that Site Fix and PR state are unchanged.
5. Ensure initial finding selection and drawer state cannot expose a handed-off finding in the active detail flow.
6. Run Doctor contracts, all web tests, lint/type checks, and a production web build.
7. Commit the UI slice.

## Task 5: Cross-stack verification and code review

1. Run `go test ./...`, `go vet ./...`, SQL generation cleanliness checks, `npm test`, and `npm run build`.
2. Inspect the final diff for lifecycle mutations, provenance loss, inaccessible nested controls, stale selection states, and unrelated edits.
3. Apply the React best-practices review if the implementation expands across multiple TSX components; otherwise run the repository's normal frontend quality gates.
4. Use the verification-before-completion checklist and request a focused code review before publishing.
5. Fix every actionable issue and repeat the affected gates.

## Task 6: Publish, deploy, and production-verify

1. Push `codex/doctor-recent-findings`, open a PR to `origin/main`, and wait for Go, Web, and Vercel checks.
2. Merge the PR only after required checks pass, then wait for Railway and Vercel production deployments containing the merge SHA.
3. Verify the production Doctor page in the user's authenticated Chrome session:
   - existing Site-Fix-backed findings are absent from the main grid;
   - eligible links appear in Recent Findings with the correct count;
   - each row opens the matching Site Fix;
   - PR-backed links are absent and terminal no-PR links remain;
   - Dismiss opens the approved confirmation, Cancel leaves all state intact, and no console/network errors occur.
4. Verify persistence and lifecycle non-mutation through read-only production API/database evidence. Do not dismiss real user data solely for testing.
5. If production differs from the specification, fix on the same branch, repeat CI/deployment, and verify again.
6. Report completion with the merged PR link only after every production acceptance check passes.

## Deferred failure register

If an independent verification cannot pass after three evidence-based attempts, record the exact failure, continue unrelated tasks, and return to the register after the rest of the implementation is complete. No core acceptance item may be silently skipped.
