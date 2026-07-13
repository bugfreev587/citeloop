# Returned Content Action Visibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep successfully returned or dismissed content actions out of Content Plan after a full refresh while preserving the reopened Opportunity.

**Architecture:** Visibility Summary is the authoritative active-loop read model, so its SQL query will exclude terminal reconsideration statuses. Content Plan will also apply a pure defensive predicate before rendering, preventing stale API payloads from reviving terminal cards.

**Tech Stack:** Go, PostgreSQL/sqlc, Next.js/React, TypeScript, Node test runner.

---

### Task 1: Exclude terminal reconsideration actions from Visibility Summary

**Files:**
- Modify: `internal/db/seo_contract_test.go`
- Modify: `internal/db/queries/seo.sql`
- Regenerate: `internal/db/seo.sql.go`

- [ ] **Step 1: Write the failing database query contract test**

Add this test to `internal/db/seo_contract_test.go`:

```go
func TestListVisibilityActionRowsExcludesReturnedAndDismissedActions(t *testing.T) {
	query := strings.ToLower(listVisibilityActionRows)
	for _, want := range []string{
		"where ca.project_id = $1",
		"ca.status not in ('returned','dismissed')",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("ListVisibilityActionRows must exclude terminal reconsideration actions; missing %q in %s", want, query)
		}
	}
}
```

- [ ] **Step 2: Run the targeted test and verify RED**

Run:

```bash
go test ./internal/db -run TestListVisibilityActionRowsExcludesReturnedAndDismissedActions -count=1
```

Expected: FAIL because `ListVisibilityActionRows` currently includes every action status.

- [ ] **Step 3: Add the authoritative SQL filter**

Change the `ListVisibilityActionRows` predicate in `internal/db/queries/seo.sql` to:

```sql
where ca.project_id = $1
  and ca.status not in ('returned','dismissed')
order by ca.updated_at desc, ca.created_at desc
limit $2;
```

- [ ] **Step 4: Regenerate sqlc output**

Run:

```bash
make sqlc
```

Expected: `internal/db/seo.sql.go` contains the terminal-status exclusion.

- [ ] **Step 5: Run the targeted test and verify GREEN**

Run:

```bash
go test ./internal/db -run TestListVisibilityActionRowsExcludesReturnedAndDismissedActions -count=1
```

Expected: PASS.

### Task 2: Defensively reject terminal actions in Content Plan

**Files:**
- Modify: `web/app/lib/content-plan-logic.test.mjs`
- Modify: `web/app/lib/content-plan-logic.ts`
- Modify: `web/app/projects/[id]/topics/topics-client.tsx`

- [ ] **Step 1: Write the failing frontend behavior test**

Add this test to `web/app/lib/content-plan-logic.test.mjs`:

```js
test("Content Plan never renders returned or dismissed actions from stale visibility payloads", async () => {
  const { isActiveContentPlanLoopAction } = await loadContentPlanLogicModule();

  assert.equal(
    isActiveContentPlanLoopAction({ status: "ready_for_review", lifecycle_stage: "added_to_plan", opportunity_status: "converted" }),
    true,
  );
  assert.equal(
    isActiveContentPlanLoopAction({ status: "returned", lifecycle_stage: "added_to_plan", opportunity_status: "open" }),
    false,
  );
  assert.equal(
    isActiveContentPlanLoopAction({ status: "dismissed", lifecycle_stage: "added_to_plan", opportunity_status: "dismissed" }),
    false,
  );
});
```

- [ ] **Step 2: Run the targeted frontend test and verify RED**

Run:

```bash
cd web && node --test --test-name-pattern="Content Plan never renders returned" app/lib/content-plan-logic.test.mjs
```

Expected: FAIL because `isActiveContentPlanLoopAction` does not exist.

- [ ] **Step 3: Implement the pure lifecycle predicate**

Add to `web/app/lib/content-plan-logic.ts`:

```ts
export type ContentPlanLoopAction = {
  status?: string | null;
  lifecycle_stage?: string | null;
  opportunity_status?: string | null;
};

const contentPlanLifecycleStages = new Set(["added_to_plan", "planned", "drafting", "ready_for_review"]);
const terminalContentActionStatuses = new Set(["returned", "dismissed"]);
const terminalContentOpportunityStatuses = new Set(["dismissed", "archived"]);

export function isActiveContentPlanLoopAction(action: ContentPlanLoopAction | null | undefined) {
  if (!action) return false;
  const status = String(action.status ?? "").trim().toLowerCase();
  const stage = String(action.lifecycle_stage ?? "").trim().toLowerCase();
  const opportunityStatus = String(action.opportunity_status ?? "").trim().toLowerCase();
  return (
    !terminalContentActionStatuses.has(status) &&
    !terminalContentOpportunityStatuses.has(opportunityStatus) &&
    contentPlanLifecycleStages.has(stage)
  );
}
```

- [ ] **Step 4: Route Content Plan filtering through the predicate**

Import `isActiveContentPlanLoopAction` in `web/app/projects/[id]/topics/topics-client.tsx`, then replace the inline lifecycle/opportunity conditions with:

```ts
isContentPlanAction(action) && isActiveContentPlanLoopAction(action)
```

- [ ] **Step 5: Run the targeted frontend test and verify GREEN**

Run:

```bash
cd web && node --test --test-name-pattern="Content Plan never renders returned" app/lib/content-plan-logic.test.mjs
```

Expected: PASS.

### Task 3: Verify, commit, publish, and validate production

**Files:**
- All modified files above
- Design and plan documents

- [ ] **Step 1: Regenerate SQL and inspect generated diff**

Run:

```bash
make sqlc
git diff --check
git status --short
```

Expected: only the intended SQL, generated Go, frontend predicate/call site, tests, and documents are changed.

- [ ] **Step 2: Run full backend verification**

Run:

```bash
go test ./... -count=1
go vet ./...
go build ./...
```

Expected: all commands exit 0.

- [ ] **Step 3: Run full frontend verification**

Run:

```bash
cd web
npm test
npm run typecheck
npm run build
```

Expected: all tests pass and both typecheck and build exit 0.

- [ ] **Step 4: Commit the implementation**

Run:

```bash
git add internal/db/queries/seo.sql internal/db/seo.sql.go internal/db/seo_contract_test.go web/app/lib/content-plan-logic.ts web/app/lib/content-plan-logic.test.mjs 'web/app/projects/[id]/topics/topics-client.tsx' docs/superpowers/plans/2026-07-12-returned-content-plan-visibility.md
git commit -m "fix: hide returned actions from Content Plan"
```

- [ ] **Step 5: Push, create a PR, merge, and wait for deployment**

Push `codex/fix-returned-content-plan-resurface`, open a PR to `main`, merge it after required checks, and wait for both Railway API and Vercel production deployments to succeed.

- [ ] **Step 6: Validate the affected production record**

In the authenticated production project:

1. Refresh `/projects/1459b054-cdc3-4d9b-9dd4-18e12458c61a/plan`.
2. Confirm the returned title `Expand the existing page or create a supporting section for the query intent` is absent from Content briefs.
3. Open Opportunities and confirm the same title is present in `Needs decision`.
4. Confirm browser error and warning logs are empty.

Expected: the terminal action never reappears after refresh, so a duplicate move-back request cannot be triggered from Content Plan.
