# Manual AI Discovery Quality Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make manual Opportunity Finding perform fresh, stage-aware AI discovery with bounded candidate repair, accurate progress, and the approved Growth Stage UI.

**Architecture:** The workflow pins manual authority in its request, performs a tracked stage-aware planning call, collects fresh evidence, then retains deterministic validation, dedupe, arbitration, and materialization. Independent evidence collectors run concurrently; the existing checkpoint API drives the visible progress UI.

**Tech Stack:** Go, PostgreSQL/sqlc, TokenGate-compatible `llm.Provider`, Next.js/React/TypeScript, Node contract tests, Go tests.

---

### Task 1: Public context safety

**Files:**
- Modify: `internal/growthradar/context.go`
- Modify: `internal/db/queries/geo.sql`
- Test: `internal/growthradar/context_test.go`

- [ ] Write failing tests proving public topics such as “API key management” and “Postgres migration guide” are allowed while secret-shaped values, private keys, internal diagnostics, and credential-bearing URLs are blocked.
- [ ] Run `go test ./internal/growthradar -run 'Test.*Sensitive' -count=1` and confirm the new public-topic cases fail.
- [ ] Replace the broad noun blacklist with secret/disclosure patterns and make the active-prompt SQL follow the same boundary.
- [ ] Run `make sqlc` and the focused tests; confirm they pass.

### Task 2: Manual fresh evidence and parallel collection

**Files:**
- Modify: `internal/opportunityfinding/ai_discovery.go`
- Modify: `internal/evidence/service.go`
- Modify: `internal/geo/shared_evidence.go`
- Modify: `internal/scheduler/scheduler.go`
- Test: `internal/opportunityfinding/ai_discovery_test.go`
- Test: `internal/geo/shared_evidence_test.go`

- [ ] Write failing tests showing manual requests cannot reuse a completed weekly answer run, scheduled requests still reuse it, and the four independent evidence collectors overlap in time while preserving per-step results.
- [ ] Run the focused Go tests and confirm failure for missing `ForceFresh` and sequential execution.
- [ ] Add a manual-only fresh collection identity and refactor evidence refresh to launch crawler, search, answer observation, and external surfaces after prompt selection using cancellation-safe concurrent workers.
- [ ] Run the focused tests and `go test ./internal/evidence ./internal/geo ./internal/opportunityfinding -count=1`.

### Task 3: Stage-aware AI planner and one repair pass

**Files:**
- Create: `internal/opportunityfinding/ai_planner.go`
- Create: `internal/opportunityfinding/ai_planner_test.go`
- Modify: `internal/opportunityfinding/ai_discovery.go`
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/geo/service.go`

- [ ] Write failing tests for structured planner output, confirmed-capability validation, history exclusion, tracked physical calls, and exactly one repair call when first-pass candidates all duplicate or fail correctably.
- [ ] Run `go test ./internal/opportunityfinding -run 'Test.*Planner|Test.*Repair' -count=1` and confirm the missing planner failures.
- [ ] Implement the planner with versioned prompts and audited calls, inject the pinned stage/public context/evidence/history, and return targeted prompt hypotheses.
- [ ] Validate proposals deterministically, materialize targeted prompts, collect their evidence, and run one rejection-informed replacement pass only when zero new decision-ready work remains.
- [ ] Run focused tests and the complete opportunityfinding/geo/growthradar suites.

### Task 4: Run outcome and progress contract

**Files:**
- Modify: `internal/api/handlers_seo.go`
- Modify: `web/app/lib/api.ts`
- Test: `internal/api/opportunity_finding_status_contract_test.go`
- Test: `web/app/lib/api.test.mjs`

- [ ] Write failing API contract tests for AI substage, fresh-call count, proposal/repair counts, newly created count, and stable terminal zero-result codes.
- [ ] Run the focused Go and Node tests and confirm the fields are absent.
- [ ] Extend checkpoint summaries and the typed client normalization without exposing prompts or sensitive evidence.
- [ ] Re-run the focused tests and confirm pass.

### Task 5: Growth Stage listbox, dismissible notice, and progress UI

**Files:**
- Create: `web/app/projects/[id]/seo/growth-stage-selector.tsx`
- Create: `web/app/projects/[id]/seo/opportunity-finding-progress.tsx`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Modify: `web/app/components/ui.tsx`
- Test: `web/app/lib/seo-client-contract.test.mjs`

- [ ] Write failing contract tests for collapsed-name-only rendering, two-line listbox options, keyboard/accessibility attributes, versioned local-storage notice dismissal, determinate progress, active-stage copy, and polling during queued/running states.
- [ ] Run `node --test web/app/lib/seo-client-contract.test.mjs` and confirm the new assertions fail.
- [ ] Implement the focused components and wire them to the existing stage/status state; keep user-facing text concise and responsive.
- [ ] Re-run the contract test, `npm --prefix web run typecheck`, and `npm --prefix web run build`.

### Task 6: Regression, review, merge, and production verification

**Files:**
- Modify: `docs/superpowers/specs/2026-07-14-manual-ai-discovery-quality-design.md` only if verification reveals a contract correction.

- [ ] Run `go test ./... -count=1`, web tests, typecheck, build, and `git diff --check`.
- [ ] Commit the implementation, push `codex/growth-finding-ai-quality`, open a PR to `main`, review the diff/checks, and merge after green CI.
- [ ] Wait for Railway and web production deployments to reach the merge SHA.
- [ ] Trigger UniPost Run finding and verify checkpoint progress, at least one new `provider_called=true` AI record, TokenGate token usage, and either a new open Opportunity or an explicit non-correctable blocker.
- [ ] Verify the Growth Stage listbox and dismissible notice at `https://citeloop.app/projects/1459b054-cdc3-4d9b-9dd4-18e12458c61a/seo` and fix/redeploy any discrepancy before reporting completion.
