# Stage-Aware Opportunity Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement manual project growth stages that change Opportunity candidate policy, deterministic scoring, evidence gates, platform targeting, and review UI while preserving replay and existing work.

**Architecture:** Add a focused `growthstage` domain package for immutable stage profiles and persistence-facing DTOs. Growth Radar calculates canonical raw signals once, applies the pinned stage profile, and persists stage metadata and reason codes; the project API owns stage selection and version-safe watchlist rescoring. The Analysis/Opportunity page consumes the project-scoped API and exposes the selector without changing downstream Writer, Review, or Publisher ownership.

**Tech Stack:** Go 1.23, PostgreSQL 16, sqlc, Chi, Next.js/React/TypeScript, Node contract tests.

---

### Task 1: Persist project stage and immutable change events

**Files:**
- Create: `internal/migrations/0088_growth_stage.sql`
- Modify: `internal/db/queries/growth_radar.sql`
- Regenerate: `internal/db/growth_radar.sql.go`, `internal/db/models.go`, `internal/db/querier.go`
- Test: `internal/db/growth_stage_contract_test.go`

- [ ] **Step 1: Write a failing migration contract test**

Assert that migration 0088 creates `growth_stage_settings` with the four-stage check, optimistic `setting_version`, unconfirmed-default state, and `growth_stage_events` with pending/running/complete/failed rescore states.

- [ ] **Step 2: Run `go test ./internal/db -run GrowthStage -count=1` and verify it fails because migration 0088 is absent.**

- [ ] **Step 3: Add migration and sqlc queries**

Add queries for get/upsert-with-expected-version, insert/get/update stage event, active-watchlist count, and stage-aware watchlist score update. The upsert must return no row when the expected version is stale.

- [ ] **Step 4: Run `sqlc generate`, then rerun the focused DB tests.**

- [ ] **Step 5: Commit `feat: persist project growth stages`.**

### Task 2: Implement deterministic stage profiles and gates

**Files:**
- Create: `internal/growthstage/profile.go`
- Create: `internal/growthstage/profile_test.go`
- Modify: `internal/growthradar/score.go`
- Modify: `internal/growthradar/score_test.go`

- [ ] **Step 1: Write failing profile tests**

Cover the exact Foundation/Traction/Scale/Optimize weights and thresholds, weight totals of 100, virtual Foundation default, integer flooring, invalid-stage rejection, and stage gate precedence.

- [ ] **Step 2: Run `go test ./internal/growthstage ./internal/growthradar -count=1` and verify missing package/API failures.**

- [ ] **Step 3: Implement immutable V1 profiles and stage-aware score output**

Expose `ProfileFor(stage)`, `DefaultSetting()`, and `Apply(raw, profile, gates)`. Preserve `ScoreCandidate` as the legacy entry point for compatibility while adding stage/profile/version, canonical raw points, weighted contributions, and structured reasons to the stage-aware result.

- [ ] **Step 4: Rerun focused tests and keep all legacy score tests green.**

- [ ] **Step 5: Commit `feat: add stage-aware growth scoring`.**

### Task 3: Add stage API and version-safe watchlist rescoring

**Files:**
- Create: `internal/api/handlers_growth_stage.go`
- Create: `internal/api/growth_stage_routes_test.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/handlers_growth_radar.go`

- [ ] **Step 1: Write failing route and handler tests**

Require `GET /api/projects/{projectID}/opportunities/stage` and `PUT` on the same path. Test the virtual unconfirmed Foundation response, valid explicit Foundation selection, invalid stage, stale expected version conflict, audit event creation, and preservation of prior watchlist rows on rescore failure.

- [ ] **Step 2: Run `go test ./internal/api -run 'GrowthStage|GrowthRadarRoutes' -count=1` and verify route/handler failures.**

- [ ] **Step 3: Implement authorized GET/PUT handlers**

Use a transaction for optimistic setting update plus event creation. After commit, deterministically rescore active watchlist snapshots under the committed profile, allow only the latest setting version to update rows, and mark the event complete or failed. Return setting, stage descriptions, affected count, and rescore status.

- [ ] **Step 4: Add current stage/profile to Growth Radar diagnostics and rerun focused API tests.**

- [ ] **Step 5: Commit `feat: expose growth stage controls`.**

### Task 4: Add SEO/GEO demand lanes and claim-scoped answer evidence

**Files:**
- Modify: `internal/db/queries/growth_radar.sql`
- Regenerate: `internal/db/growth_radar.sql.go`, `internal/db/querier.go`
- Modify: `internal/growthradar/score.go`
- Modify: `internal/growthradar/score_test.go`
- Modify: `internal/geo/pr3.go`
- Modify: `internal/geo/service_pr3_test.go`

- [ ] **Step 1: Write failing GEO-demand tests**

Prove one provider contributes two GEO raw points but cannot satisfy multi-provider creation; two providers contribute five; three contribute seven; distinct UTC dates add recurrence; repeated same-provider/same-date observations do not stack. Prove an uncited, fully provenanced answer supports only an absence claim and never a citation claim.

- [ ] **Step 2: Write a failing query contract test for deterministic query aliases**

Require demand lookup to accept persisted normalized prompt/topic aliases instead of exact full-prompt equality and reject arbitrary semantic similarity.

- [ ] **Step 3: Run the focused growthradar, geo, and DB tests and verify expected failures.**

- [ ] **Step 4: Implement the two demand lanes and evidence qualification**

Aggregate independent engines and observation dates per normalized prompt gap, pass their identities into the snapshot, compute SEO 0–15 plus GEO 0–10, and persist supported claim types and provenance completeness.

- [ ] **Step 5: Rerun focused tests and commit `feat: score independent SEO and GEO demand`.**

### Task 5: Resolve real project platform targets and reuse inputs

**Files:**
- Modify: `internal/geo/pr3.go`
- Modify: `internal/geo/service_test.go`
- Modify: `internal/geo/service_pr3_test.go`
- Modify: `internal/platformcontract/capability.go`
- Modify: `internal/platformcontract/capability_test.go`

- [ ] **Step 1: Write failing target-resolution tests**

Cover blog-only zero reuse, enabled compatible project targets, current required target context, incompatible asset types, and the four populated snapshot fields: selected, compatible, additional output types, and covered targets.

- [ ] **Step 2: Run `go test ./internal/geo ./internal/platformcontract -run 'GrowthRadar|Capability|Target' -count=1` and verify failures.**

- [ ] **Step 3: Replace blog-only `growthRadarTarget`**

Build the contract capability matrix from active contracts, current project target contexts, and enabled publisher connections. Always pin the owned blog when available; include only configured, generation-compatible external targets and current required contexts. Populate all reuse inputs from the resolved plan.

- [ ] **Step 4: Rerun focused tests and commit `feat: resolve growth radar platform targets`.**

### Task 6: Enforce stage-aware candidate generation and structured reasons

**Files:**
- Modify: `internal/geo/pr2.go`
- Modify: `internal/geo/service_pr2_test.go`
- Modify: `internal/geo/pr3.go`
- Modify: `internal/geo/service_pr3_test.go`
- Modify: `internal/opportunityfinding/ai_discovery.go`
- Modify: `internal/opportunityfinding/ai_discovery_test.go`

- [ ] **Step 1: Write failing sanitization and policy tests**

Prove rejected internal terms from profile fields and existing Topic rows never enter prompt generation. Add fixtures showing Foundation can create without GSC when independent evidence is present, Traction requires demand, Scale requires success plus real expansion, and Optimize requires material change and chooses refresh work.

- [ ] **Step 2: Run focused geo and opportunityfinding tests and verify failures.**

- [ ] **Step 3: Pin the project stage at analysis start and apply candidate policy**

Filter all prompt-topic inputs through accepted public vocabulary, load the virtual or stored stage once per run, apply stage-aware scoring/gates, and persist structured reason codes instead of the disposition string alone.

- [ ] **Step 4: Extend funnel diagnostics with stage/profile, demand lane, rejected-context, target, and gate counts.**

- [ ] **Step 5: Rerun focused tests and commit `feat: apply stage candidate policies`.**

### Task 7: Add the Opportunity-page stage selector

**Files:**
- Modify: `web/app/lib/api.ts`
- Create: `web/app/lib/growth-stage.ts`
- Create: `web/app/lib/growth-stage.test.mjs`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Write failing frontend contract tests**

Require the upper-right `Growth Stage` selector, four approved options/descriptions, unconfirmed Foundation notice, expected-version update, confirmation dialog content, loading/error/conflict states, and no mutation of Opportunity records.

- [ ] **Step 2: Run `cd web && node --test app/lib/growth-stage.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs` and verify failures.**

- [ ] **Step 3: Implement API normalization and selector UI**

Load the stage with Analysis data, render the selector before Refresh/Sync, confirm changes, call PUT with `expected_version`, refresh diagnostics after success, and expose retryable rescore failure without reverting the selected stage.

- [ ] **Step 4: Rerun frontend tests and `cd web && npm run build`.**

- [ ] **Step 5: Commit `feat: add Opportunity growth stage selector`.**

### Task 8: Verify, review, deploy, and validate production

**Files:** No planned production changes. If a verification command exposes a defect, first add a failing regression test beside the affected file from Tasks 1–7, then make the smallest implementation correction.

- [ ] **Step 1: Run formatting and generated-code checks**

Run `gofmt` on changed Go files, `sqlc generate`, `git diff --check`, and confirm a second sqlc run produces no diff.

- [ ] **Step 2: Run backend verification**

Run `go test ./...`, `go vet ./...`, and `go build ./...` with zero failures.

- [ ] **Step 3: Run frontend verification**

Run `cd web && npm test` and `cd web && npm run build` with zero failures.

- [ ] **Step 4: Review the complete diff against every Production Acceptance item in the PRD and repair gaps using new red-green tests.**

- [ ] **Step 5: Push the clean branch, create a PR to `origin/main`, wait for required checks, merge it, and confirm `origin/main` contains the merge SHA.**

- [ ] **Step 6: Wait for production deployment and verify**

Verify health, stage GET virtual/default behavior, explicit Foundation selection for UniPost, Opportunity-page selector, stage/profile diagnostics, no active internal-sensitive prompts, populated target/reuse inputs, deterministic score replay, and a production discovery run. Any gap returns to a red-green fix and a new production verification cycle.
