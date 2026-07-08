# Opportunity Reconsideration And Dismissal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the Opportunity -> Content Plan move-back and dismissal semantics, including reviewed-state suppression and production verification.

**Architecture:** Backend owns lifecycle truth: opportunities get stable identity plus evidence fingerprint, content actions get a `returned` status, and action-level endpoints update action/opportunity state together. Frontend only calls action-level APIs after app-native confirmation modals and removes the active Content Plan card from local state after success.

**Tech Stack:** Go API, sqlc, PostgreSQL migrations, Next.js/React client UI, node contract tests.

---

### Task 1: Schema And Dedupe Contract

**Files:**
- Create: `internal/migrations/0042_opportunity_reconsideration_review_states.sql`
- Modify: `internal/db/queries/seo.sql`
- Modify after sqlc: `internal/db/seo.sql.go`, `internal/db/models.go`, `internal/db/querier.go`
- Test: `internal/db/seo_contract_test.go`

- [x] **Step 1: Write failing schema/query tests**

Add tests proving:

```go
func TestUpsertSEOOpportunitySeparatesStableIdentityFromEvidenceFingerprint(t *testing.T) {
	query := strings.ToLower(upsertSEOOpportunity)
	if strings.Contains(query, "evidence_window") && strings.Contains(query, "opportunity_identity_key") {
		t.Fatal("stable identity must not include evidence_window")
	}
	if !strings.Contains(query, "opportunity_identity_key") || !strings.Contains(query, "evidence_fingerprint") {
		t.Fatal("UpsertSEOOpportunity must write stable identity and evidence fingerprint")
	}
}
```

Also assert the latest migration adds `returned`, `opportunity_identity_key`, `evidence_fingerprint`, and a review-state table.

- [x] **Step 2: Run red tests**

Run: `go test ./internal/db -run 'TestUpsertSEOOpportunitySeparatesStableIdentityFromEvidenceFingerprint|TestOpportunityReconsiderationSchema'`

Expected: FAIL because the current query only writes `opportunity_key` and the migration does not exist.

- [x] **Step 3: Implement schema and query**

Create migration `0042...` that:
- Adds `returned` to `content_actions.status`.
- Adds `opportunity_identity_key text not null default ''`.
- Adds `evidence_fingerprint text not null default ''`.
- Backfills both values deterministically.
- Creates `seo_opportunity_review_states` keyed by `(project_id, opportunity_identity_key)`.
- Updates indexes so unchanged reviewed states do not create duplicate open opportunities.

Update `UpsertSEOOpportunity` so stable identity excludes evidence window/reason and evidence fingerprint includes evidence window/reason/severity-like evidence.

- [x] **Step 4: Generate sqlc and run green tests**

Run: `make sqlc`

Run: `go test ./internal/db -run 'TestUpsertSEOOpportunitySeparatesStableIdentityFromEvidenceFingerprint|TestOpportunityReconsiderationSchema'`

Expected: PASS.

### Task 2: Action-Level Return And Dismiss APIs

**Files:**
- Modify: `internal/api/server.go`
- Modify: `internal/api/handlers_seo.go`
- Modify: `internal/db/queries/seo.sql`
- Modify after sqlc: `internal/db/seo.sql.go`, `internal/db/querier.go`
- Test: `internal/api/action_routing_contract_test.go`
- Test: `internal/api/visibility_summary_contract_test.go`

- [x] **Step 1: Write failing API contract tests**

Assert:

```go
strings.Contains(routes, `r.Post("/actions/{actionID}/return-to-opportunity", s.returnSEOContentActionToOpportunity)`)
strings.Contains(source, "MarkContentActionReturnedToOpportunity")
strings.Contains(source, "DismissSEOContentActionAndOpportunity")
strings.Contains(source, "CreateOrUpdateSEOOpportunityReviewState")
```

- [x] **Step 2: Run red tests**

Run: `go test ./internal/api -run 'TestSEOActionReturnDismissContracts|TestSEORoutesAreRegistered'`

Expected: FAIL because the route and handlers are missing.

- [x] **Step 3: Implement handlers**

Add:
- `returnSEOContentActionToOpportunity`: validates action, rejects published/applied/measuring/completed states, marks action `returned`, sets source opportunity `open`, writes workflow event.
- `dismissSEOContentActionAndOpportunity`: marks action `dismissed`, source opportunity `dismissed`, writes review-state record, writes workflow event.

Protect stale async status changes by allowing `UpdateContentActionStatus`-style workers to update only active statuses where needed.

- [x] **Step 4: Run green tests**

Run: `go test ./internal/api -run 'TestSEOActionReturnDismissContracts|TestSEORoutesAreRegistered'`

Expected: PASS.

### Task 3: Content Plan UI Confirmation Flow

**Files:**
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/projects/[id]/topics/topics-client.tsx`
- Test: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [x] **Step 1: Write failing frontend contract tests**

Assert:

```js
assert.match(topics, /Move back to Opportunities/);
assert.match(topics, /returnSEOContentActionToOpportunity/);
assert.match(topics, /dismissSEOContentAction\\(projectId, action\\.id\\)/);
assert.match(topics, /Confirm/);
assert.doesNotMatch(topics, /api\\.dismissSEOOpportunity\\(projectId, action\\.opportunity_id\\)/);
```

- [x] **Step 2: Run red tests**

Run: `npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: FAIL because the UI has no move-back action and calls opportunity-level dismiss.

- [x] **Step 3: Implement UI and API client**

Add API methods:
- `returnSEOContentActionToOpportunity(id, actionID)`
- `dismissSEOContentAction(id, actionID)`

In Content Plan drawer:
- Add `Move back to Opportunities`.
- Add app-native modal state with exact Confirm/Cancel actions.
- Make dismiss and create/draft call through confirmation.
- Keep button progress states and local summary removal.

- [x] **Step 4: Run green frontend tests**

Run: `npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs`

Run: `npm run typecheck`

Expected: PASS.

### Task 4: Full Verification, PR, Merge, Production

**Files:**
- GitHub PR from `codex/opportunity-reconsider-dismissal-impl-20260708` to `main`.

- [x] **Step 1: Run full local verification**

Run:

```bash
go test ./...
cd web && npm test && npm run typecheck && npm run build
```

Expected: all commands exit 0.

- [ ] **Step 2: Push and create PR**

Run:

```bash
git status --short
git push -u origin codex/opportunity-reconsider-dismissal-impl-20260708
gh pr create --base main --head codex/opportunity-reconsider-dismissal-impl-20260708
```

- [ ] **Step 3: Merge and verify production**

After checks pass, merge the PR, wait for deployment, and verify production behavior through the live app/API:
- Content Plan item exposes Move back, Dismiss, Create/Draft.
- Confirm/Cancel modal gates state-changing actions.
- Dismiss removes the action and suppresses unchanged rediscovery.
- Move back restores the source opportunity to open and marks the action returned.
