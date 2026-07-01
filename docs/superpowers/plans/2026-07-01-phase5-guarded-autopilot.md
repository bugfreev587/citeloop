# Phase 5 Guarded Autopilot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship Phase 5 Guarded Autopilot MVP so Level 2 execution is gated by readiness, policy, publisher capability, rollback/recovery metadata, and auditability.

**Architecture:** Reuse the existing `seo_policies`, `autopilot_runs`, `seo_action_plans`, `safe_mode_events`, and `autopilot_audit_events` foundation. Add a deterministic readiness contract and a guarded plan execution endpoint that only converts low-risk, policy-allowed actions into `content_actions`; every executed or blocked action records guardrail, rollback/recovery, and audit context. Surface readiness and execution state in the existing Analysis Autopilot panel.

**Tech Stack:** Go HTTP handlers with sqlc-generated pgx queries, existing publisher/notification/SEO setup models, Next.js/React frontend, node test contracts, Go unit/route contract tests.

---

### Task 1: Autopilot Readiness Contract

**Files:**
- Modify: `internal/api/server.go`
- Modify: `internal/api/handlers_autopilot.go`
- Modify: `internal/api/seo_routes_test.go`
- Create: `internal/api/autopilot_readiness_contract_test.go`
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/seo-client-contract.test.mjs`

- [ ] **Step 1: Write failing backend contract tests**

Add tests that require `GET /api/projects/{projectID}/seo/autopilot/readiness` to be registered and require `handlers_autopilot.go` to expose typed readiness output with these JSON fields:

```go
func TestAutopilotReadinessRouteIsRegistered(t *testing.T) {
	router := (&Server{}).Router()
	req := httptest.NewRequest(http.MethodGet, "/api/projects/not-a-uuid/seo/autopilot/readiness", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want bad project id", res.Code)
	}
}

func TestAutopilotReadinessContractMentionsPhase5Gates(t *testing.T) {
	raw, err := os.ReadFile("handlers_autopilot.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{
		"type AutopilotReadiness",
		"type AutopilotReadinessGate",
		"ready_for_level_2",
		"publisher_write",
		"notification_write",
		"autopilot_policy_confirmed",
		"monthly_budget_configured",
		"safe_mode_clear",
		"kill_switch_clear",
		"rollback_or_recovery_ready",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("readiness contract missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run backend tests and verify RED**

Run: `go test ./internal/api -run 'TestAutopilotReadiness' -count=1`
Expected: FAIL because the route and readiness types do not exist.

- [ ] **Step 3: Implement readiness handler**

Add `AutopilotReadiness` and `AutopilotReadinessGate` structs. Implement `getAutopilotReadiness` by reading `ensureSEOPolicy`, SEO overview/setup checklist, publisher connections, notification channels, and safe mode events. A gate is blocking when required for Level 2 and not passed. `ready_for_level_2` is true only when search data, publisher write, notification write, policy confirmed, budget configured, safe mode clear, kill switch clear, and rollback-or-recovery metadata are all ready.

- [ ] **Step 4: Register route and add client type/API**

Register `r.Get("/readiness", s.getAutopilotReadiness)`. In `web/app/lib/api.ts`, add `AutopilotReadinessGate`, `AutopilotReadiness`, and `getAutopilotReadiness(projectId)`.

- [ ] **Step 5: Write failing frontend contract test then make it pass**

Add contract expectations that `api.ts` contains `getAutopilotReadiness`, `ready_for_level_2`, and `rollback_or_recovery_ready`.
Run: `cd web && npm test -- app/lib/seo-client-contract.test.mjs`
Expected: FAIL before client implementation, PASS after.

### Task 2: Guarded Execute Endpoint

**Files:**
- Modify: `internal/db/queries/autopilot.sql`
- Regenerate or update: `internal/db/autopilot.sql.go`, `internal/db/querier.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/handlers_autopilot.go`
- Create: `internal/api/autopilot_execute_contract_test.go`
- Modify: `web/app/lib/api.ts`

- [ ] **Step 1: Write failing execution contract tests**

Add tests that require `POST /api/projects/{projectID}/seo/autopilot/plans/{planID}/execute` to be registered and require the implementation to include these guardrail terms:

```go
func TestAutopilotExecuteRouteIsRegistered(t *testing.T) {
	router := (&Server{}).Router()
	req := httptest.NewRequest(http.MethodPost, "/api/projects/not-a-uuid/seo/autopilot/plans/not-a-plan/execute", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want bad project id", res.Code)
	}
}

func TestAutopilotExecuteContractRequiresGuardsAuditAndRollback(t *testing.T) {
	raw, err := os.ReadFile("handlers_autopilot.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{
		"executeAutopilotPlan",
		"auto_publish_allowed",
		"guardrail_results",
		"autopilot_audit_events",
		"manual_rollback_required",
		"recovery_plan",
		"publisher_capability",
		"policy_not_ready",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("execute contract missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run backend tests and verify RED**

Run: `go test ./internal/api -run 'TestAutopilotExecute' -count=1`
Expected: FAIL because the endpoint and implementation do not exist.

- [ ] **Step 3: Add required db queries**

Add sqlc queries:

```sql
-- name: GetSEOActionPlanForProject :one
select * from seo_action_plans
where id = $1 and project_id = $2;

-- name: UpdateSEOActionPlanStatus :one
update seo_action_plans set
  status = $3,
  updated_at = now()
where id = $1 and project_id = $2
returning *;
```

Run `sqlc generate` if available; otherwise update generated files consistently with nearby generated code.

- [ ] **Step 4: Implement guarded execution**

Implement `executeAutopilotPlan`:

1. Parse `{planID}` and load the plan.
2. Compute readiness with the same helper as the readiness endpoint.
3. If readiness fails, keep/mark plan `blocked`, insert an `autopilot_audit_events` row with `event_type = "autopilot_execute_blocked"` and return 409 with failed gates.
4. Parse `plan.Actions`/portfolio selected actions.
5. For each action, only execute when `auto_publish_allowed` is true, `risk_level == "low"`, and the action bucket is policy/publisher-capability allowed.
6. Convert eligible actions by calling `CreateContentAction` with `status = "approved"` and metadata via `UpdateContentActionExecutionMetadata` where `review_required = false`, `diff_snapshot` contains `manual_rollback_required` and `recovery_plan`, and `output_snapshot` contains guardrail results.
7. Update the source opportunity to `converted`.
8. Record `autopilot_audit_events` with `event_type = "autopilot_action_executed"` for each executed action and `event_type = "autopilot_action_deferred"` for blocked/deferred actions.
9. Update plan status to `executing` if any actions executed, otherwise `blocked`.
10. Return `{plan, executed_actions, deferred_actions, readiness}`.

- [ ] **Step 5: Add frontend client method**

Add `executeAutopilotPlan(projectId, planId)` to `web/app/lib/api.ts` and types for executed/deferred output.

### Task 3: Autopilot Panel UX

**Files:**
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Modify: `web/app/lib/seo-client-contract.test.mjs`

- [ ] **Step 1: Write failing UI contract test**

Require the SEO client source to contain user-facing labels for `Readiness`, `Ready for Level 2`, `Blocked gates`, `Execute guarded actions`, `Recovery plan`, and `Manual rollback required`.

- [ ] **Step 2: Run UI test and verify RED**

Run: `cd web && npm test -- app/lib/seo-client-contract.test.mjs`
Expected: FAIL before UI changes.

- [ ] **Step 3: Implement UI**

Fetch readiness alongside policy/plans. Render a compact readiness strip above the policy controls, list blocked gates with next actions, show Level 2 readiness, and add an `Execute guarded actions` button for the latest plan. Disable execution unless readiness is ready and the plan exists. Show execution result counts and expose recovery/rollback labels in the selected action list.

- [ ] **Step 4: Run UI test and typecheck**

Run: `cd web && npm test -- app/lib/seo-client-contract.test.mjs && npm run typecheck`
Expected: PASS.

### Task 4: Verification and Release

**Files:**
- All touched files.

- [ ] **Step 1: Full local verification**

Run:

```bash
go test ./...
cd web && npm test
cd web && npm run typecheck
cd web && npm run build
```

Expected: all pass. Existing npm audit warnings are non-blocking unless changed by this work.

- [ ] **Step 2: Commit, push, PR, merge, deploy, production verify**

Commit the branch, push to `origin/codex/seo-geo-phase5-guarded-autopilot`, create a PR to `origin/main`, merge it, wait for Vercel and Railway production deployments on the merge commit, and verify the production Analysis/Autopilot panel loads with readiness and no browser console/runtime errors.
