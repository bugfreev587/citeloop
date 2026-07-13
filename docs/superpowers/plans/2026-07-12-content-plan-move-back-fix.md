# Content Plan Move-Back Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore the Content Plan “Move back to Opportunities” action and stop masking database failures as irreversible-action 404s.

**Architecture:** Keep the existing transaction and lifecycle semantics. Repair the generated SQL source so draft withdrawal updates only real `articles` columns, then classify `pgx.ErrNoRows` separately from database execution failures at the HTTP boundary.

**Tech Stack:** Go, PostgreSQL, sqlc, Go HTTP tests, Next.js production UI verification.

---

### Task 1: Regression Tests

**Files:**
- Modify: `internal/db/seo_contract_test.go`
- Modify: `internal/api/action_routing_contract_test.go`

- [ ] **Step 1: Add the failing query/schema contract test**

Append a test that isolates the `withdrawn_article` CTE and rejects the nonexistent column reference while preserving the required draft withdrawal:

```go
func TestReturnContentActionWithdrawsDraftUsingArticleSchema(t *testing.T) {
	query := strings.ToLower(markContentActionReturnedToOpportunity)
	start := strings.Index(query, "withdrawn_article as")
	if start < 0 {
		t.Fatal("return query must retain linked draft withdrawal")
	}
	withdrawal := query[start:]
	for _, want := range []string{
		"update articles",
		"status = 'rejected'",
		"a.id = candidate.draft_article_id",
		"a.project_id = candidate.project_id",
	} {
		if !strings.Contains(withdrawal, want) {
			t.Fatalf("draft withdrawal missing %q: %s", want, withdrawal)
		}
	}
	if strings.Contains(withdrawal, "updated_at") {
		t.Fatalf("articles has no updated_at column: %s", withdrawal)
	}
}
```

- [ ] **Step 2: Add the failing HTTP error-classification test**

Extend the API test imports with `net/http`, `net/http/httptest`, and `github.com/jackc/pgx/v5`, then add:

```go
func TestWriteContentActionMutationErrorClassifiesNoRowsSeparately(t *testing.T) {
	tests := []struct {
		name string
		err error
		wantStatus int
		wantBody string
	}{
		{"irreversible action", pgx.ErrNoRows, http.StatusNotFound, "action not found or no longer reversible"},
		{"database failure", errors.New("column does not exist"), http.StatusInternalServerError, "could not move action back to Opportunities"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeContentActionMutationError(recorder, tt.err, "action not found or no longer reversible", "could not move action back to Opportunities")
			if recorder.Code != tt.wantStatus || !strings.Contains(recorder.Body.String(), tt.wantBody) {
				t.Fatalf("response status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
```

- [ ] **Step 3: Run the tests and verify RED**

Run:

```bash
go test ./internal/db -run TestReturnContentActionWithdrawsDraftUsingArticleSchema -count=1
go test ./internal/api -run TestWriteContentActionMutationErrorClassifiesNoRowsSeparately -count=1
```

Expected: the DB test fails on `articles has no updated_at column`; the API test fails to compile because `writeContentActionMutationError` is not implemented.

### Task 2: Minimal Query And Handler Fix

**Files:**
- Modify: `internal/db/queries/seo.sql`
- Regenerate: `internal/db/seo.sql.go`
- Modify: `internal/api/handlers_seo.go`

- [ ] **Step 1: Remove the invalid article column update**

Change the draft-withdrawal CTE to:

```sql
withdrawn_article as (
  update articles a set
    status = 'rejected'
  from candidate
  where a.id = candidate.draft_article_id
    and a.project_id = candidate.project_id
    and a.status in ('generating','pending_review','approved')
  returning a.id
)
```

- [ ] **Step 2: Add the shared error writer and use it for return**

Add beside the return handler:

```go
func writeContentActionMutationError(w http.ResponseWriter, err error, notFoundMessage, internalMessage string) {
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, notFoundMessage)
		return
	}
	writeErr(w, http.StatusInternalServerError, internalMessage)
}
```

Replace the return handler’s unconditional 404 with:

```go
if err != nil {
	writeContentActionMutationError(w, err, "action not found or no longer reversible", "could not move action back to Opportunities")
	return
}
```

- [ ] **Step 3: Regenerate sqlc**

Run:

```bash
make sqlc
```

Expected: `internal/db/seo.sql.go` changes only in the generated return query.

- [ ] **Step 4: Run targeted tests and verify GREEN**

Run:

```bash
go test ./internal/db -run TestReturnContentActionWithdrawsDraftUsingArticleSchema -count=1
go test ./internal/api -run 'TestWriteContentActionMutationErrorClassifiesNoRowsSeparately|TestSEOActionReturnDismissContracts' -count=1
```

Expected: PASS.

### Task 3: Full Verification And Delivery

**Files:**
- No additional source files.

- [ ] **Step 1: Run complete backend verification**

```bash
go test ./... -count=1
go vet ./...
go build ./...
```

Expected: all commands exit 0.

- [ ] **Step 2: Run complete web verification**

```bash
cd web
npm test
npm run typecheck
npm run build
```

Expected: 354 or more tests pass, typecheck exits 0, and the production build succeeds.

- [ ] **Step 3: Review and publish**

Inspect `git diff`, commit the focused fix, push `codex/fix-content-plan-move-back`, create a PR to `main`, wait for CI, merge, and wait for Railway/Vercel production deployment.

- [ ] **Step 4: Verify production closed loop**

In the authenticated Chrome session:

1. Open the reported Content Plan brief.
2. Confirm “Move back to Opportunities”.
3. Verify the success message, the Content Plan card count decreases, and the drawer closes.
4. Open Opportunities and verify the reopened source opportunity appears in the decision queue.
5. Verify no browser console errors were introduced.

