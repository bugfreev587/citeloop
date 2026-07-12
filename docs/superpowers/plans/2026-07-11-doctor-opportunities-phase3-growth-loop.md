# Doctor / Opportunities Phase 3 Growth Loop Implementation Plan

> **Execution rule:** Ship each slice from a fresh `origin/main` worktree. Every slice must merge through a PR, reach production, and receive production data/API/browser proof before the next slice starts.

**Goal:** Make Opportunities the only owner of decision-ready Growth Work, with a typed hypothesis and baseline contract, a finite measurement lifecycle, reusable learnings, shared evidence collection, and one durable Opportunity Finding orchestration path.

**Architecture:** Keep `seo_opportunities` and `content_actions` as the current canonical physical Growth Opportunity and Growth Action tables during Phase 3, but add explicit typed contracts and invariants. Incomplete work remains an internal `discovery_candidate` in `needs_specification` or `needs_evidence`; it does not enter the user decision queue. A versioned measurement policy is copied immutably to each Growth Action when Measuring starts. Evidence refresh is collected once and consumed by both product lines; Opportunity evaluation never creates Doctor work. All provider calls enter `ai_call_records`.

**Primary PRD coverage:** Phase 3; AC 2, 5, 18–28, 38, 41, 42, 47. Preserve all Phase 0–2 invariants, especially single-owner arbitration, review memory, canonical writer fencing, and migration ledgers.

---

## Slice P3A — Decision-ready Growth Opportunity contract

### Task 1: Add the versioned Growth specification schema

**Files:**

- Create: `internal/migrations/0074_growth_opportunity_spec.sql`
- Modify: `internal/db/queries/seo.sql`
- Generate: `internal/db/models.go`, `internal/db/seo.sql.go`, `internal/db/querier.go`
- Create: `internal/db/growth_opportunity_spec_contract_test.go`

**Steps:**

1. Add a failing migration contract test for these `seo_opportunities` fields:
   - `growth_spec_state = legacy | needs_specification | needs_evidence | decision_ready`
   - `growth_spec_version`
   - `growth_spec` JSON object
   - `growth_spec_missing` JSON array
   - `decision_ready_at`
2. Add JSON shape checks and a `NOT VALID` canonical-open constraint. Newly inserted canonical Growth rows with `status = open` must be `decision_ready` and contain:
   - hypothesis;
   - audience;
   - baseline source/window/value;
   - primary metric;
   - expected direction and range/decision threshold;
   - versioned measurement policy;
   - attribution model;
   - stop and reconsider conditions.
3. Backfill existing rows as `legacy` without changing status or user decisions. Do not invent a baseline for historical rows.
4. Extend `CreateCanonicalGrowthOpportunity` and evidence merge queries to persist the typed specification in the same reservation transaction.
5. Run `sqlc generate`; run the DB contract tests.

### Task 2: Compile evidence into a strict Growth specification

**Files:**

- Create: `internal/growthspec/spec.go`
- Create: `internal/growthspec/spec_test.go`
- Modify: `internal/growthwork/service.go`
- Modify: `internal/growthwork/creator.go`
- Modify: `internal/seo/service.go`
- Modify: `internal/geo/pr3.go`

**Steps:**

1. Write table-driven tests for GSC CTR, ranking/striking-distance, content decay, cannibalization, AI citation, GA4 engagement/conversion, and new-asset hypotheses.
2. Implement a deterministic compiler returning `decision_ready`, `needs_evidence`, or `needs_specification` plus explicit missing fields. It must never turn provider-unavailable placeholders into a zero baseline.
3. Require source window, freshness, sample size, target identity, and primary metric evidence appropriate to the opportunity family.
4. Store typed target audience and expected direction without manufacturing numerical lift. If a defensible range is unavailable, persist a decision threshold and mark range confidence explicitly.
5. Pass the compiled specification through canonical candidate projection and Phase B reservation; no AI/provider call is allowed inside locks or transactions.

### Task 3: Hold incomplete candidates outside the user queue

**Files:**

- Modify: `internal/growthwork/service.go`
- Modify: `internal/discovery/projector.go`
- Modify: `internal/discovery/arbitration.go`
- Modify: `internal/db/queries/discovery.sql`
- Modify: `internal/db/queries/seo.sql`
- Modify: `internal/api/handlers_seo.go`
- Tests: matching `*_test.go` files

**Steps:**

1. Add failing tests proving incomplete Growth work persists as `needs_specification` or `needs_evidence` with an internal review item and does not create an open `seo_opportunity`.
2. Let canonical Growth materialization save held candidates before returning a typed held result.
3. Filter the user-visible Opportunity queue to `decision_ready` canonical rows plus preserved legacy/in-flight rows. Do not silently hide an already accepted or executing legacy action.
4. Return readiness, missing evidence, hypothesis, baseline, metric, and measurement policy through Opportunity detail/list APIs.
5. Production proof: run Opportunity Finding and show either decision-ready opportunities with complete fields or internal held candidates with no malformed user-visible row.

---

## Slice P3B — Finite measurement policy and real checkpoints

### Task 4: Bind an immutable measurement policy to each Growth Action

**Files:**

- Create: `internal/migrations/0075_growth_action_measurement_policy.sql`
- Modify: `internal/db/queries/seo.sql`
- Modify: `internal/scheduler/scheduler.go`
- Create: `internal/measurement/policy.go`
- Tests: DB, scheduler, and policy tests

**Steps:**

1. Add `measurement_policy_version`, `measurement_policy`, `measuring_started_at`, `absolute_terminal_at`, and terminal reason fields to `content_actions`.
2. Add checkpoint role and attempt metadata to `action_measurements`: `baseline | early | primary | follow_up`, policy version, attempt, data-quality state, and source freshness.
3. At the first `published/applied -> measuring` transition, copy the Opportunity policy and compute immutable `absolute_terminal_at`.
4. Enforce finite offsets, finite `max_measuring_duration`, bounded follow-ups, and bounded grace. Retries and policy upgrades cannot move `absolute_terminal_at` later.
5. Add DB constraints/triggers and concurrency tests for AC 42 and 47.

### Task 5: Compute real before/after outcomes

**Files:**

- Create: `internal/measurement/evaluator.go`
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/db/queries/seo.sql`
- Tests: evaluator and scheduler fixtures

**Steps:**

1. Read GSC, GA4, and AI citation observations for baseline and after windows using the spec's dimensions.
2. Classify `positive | negative | mixed | inconclusive | insufficient_data` from metric direction, decision threshold, guardrails, data completeness, and confounders.
3. Primary `insufficient_data` schedules only bounded follow-ups. The final follow-up or absolute deadline terminalizes as `insufficient_data`.
4. Persist metric deltas, source freshness, attribution confidence, and confounders—not only a label.
5. Production proof with a controlled due action and direct DB/API evidence that it cannot remain Measuring indefinitely.

---

## Slice P3C — Learning and scoring feedback

### Task 6: Add Growth learnings and measurement-quality records

**Files:**

- Create: `internal/migrations/0076_growth_learnings.sql`
- Create: `internal/db/queries/growth_learning.sql`
- Generate: sqlc output
- Create: `internal/learning/service.go`
- Modify: `internal/scheduler/scheduler.go`
- Tests: DB/service/scheduler tests

**Steps:**

1. Create immutable `growth_learnings` for positive, negative, mixed, and inconclusive terminal outcomes.
2. Create separate `measurement_quality_records` for insufficient data; these records cannot influence directional scoring.
3. Link candidate, Opportunity, Growth Action, applied/published artifact, baseline, checkpoints, and outcome.
4. Make terminalization plus learning/quality creation idempotent in one transaction.

### Task 7: Feed learnings into future candidate scores

**Files:**

- Create: `internal/learning/scoring.go`
- Modify: `internal/seo/search_opportunities.go`
- Modify: `internal/seo/actionable_opportunities.go`
- Modify: `internal/geo/pr3.go`
- Tests: scoring provenance tests

**Steps:**

1. Query applicable historical learnings by action family, target/entity, audience, and metric.
2. Apply bounded, explainable score adjustments; never allow one learning to dominate base evidence.
3. Persist learning IDs, adjustment, and model/version in candidate evidence.
4. Expose “learning used” provenance in Opportunity detail.

---

## Slice P3D — Shared evidence and canonical AI call ledger

### Task 8: Add durable evidence runs and observations

**Files:**

- Create: `internal/migrations/0077_shared_evidence.sql`
- Create: `internal/db/queries/evidence.sql`
- Create: `internal/evidence/service.go`
- Modify: Doctor and Opportunity evaluators
- Tests: idempotency/freshness/partial-failure tests

**Steps:**

1. Add `evidence_runs` and `evidence_observations` as source-neutral envelopes over crawl, render, GSC, GA4, and answer-engine evidence.
2. Deduplicate by project + source + target + window + collection-spec fingerprint. Different user agent, dimensions, prompt, provider, or provider version must not collapse.
3. Make one failed provider/URL partial, not fatal to unrelated sources.
4. Provider-unavailable and unchecked pages remain coverage gaps, never zero/healthy evidence.

### Task 9: Canonicalize all provider call accounting

**Files:**

- Create: `internal/migrations/0078_ai_call_records.sql`
- Create: `internal/db/queries/ai_calls.sql`
- Create: `internal/aicalls/recorder.go`
- Modify: diagnosis, arbitration, generation, QA, verification, and learning call sites
- Tests: aggregate/retry/skipped consistency tests

**Steps:**

1. Persist stage, object link, provider/model/prompt version, request fingerprint, status, tokens, cost, timestamps, and error code.
2. Give each retry a separate record with parent/request linkage.
3. Record `skipped` only when no provider call occurred; never create a fake observation.
4. Make object token/cost aggregates recomputable from this ledger.

---

## Slice P3E — One Opportunity Finding orchestration path

### Task 10: Checkpoint the durable Opportunity workflow

**Files:**

- Create: `internal/migrations/0079_opportunity_finding_stages.sql`
- Modify: `internal/workflow/worker.go`
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/opportunityfinding/ai_discovery.go`
- Tests: crash/retry/idempotency tests

**Steps:**

1. Model stages: evidence refresh, deterministic signals, AI hypotheses, arbitration, materialization, and summary.
2. Persist stage checkpoints and request fingerprints so completed billable stages are not repeated after a crash.
3. Join partial errors into a terminal run summary while allowing unaffected stages to finish.
4. Keep one active manual/scheduled run per project and expose stage progress.

### Task 11: Remove duplicate weekly GEO opportunity generation

**Files:**

- Modify: `internal/scheduler/scheduler.go`
- Modify: `cmd/api/main.go`
- Modify: project capability/config migration code
- Tests: scheduler authority tests

**Steps:**

1. Remove standalone weekly AI Discovery opportunity creation.
2. If a weekly job remains, limit it to shared evidence refresh governed by freshness and capability authority.
3. Ensure manual > approved event > scheduled precedence and no authority expansion.
4. Production proof: one provider-observation run for the same target/window/spec, even when schedules coincide.

---

## Slice P3F — API, UI, and production phase gate

### Task 12: Expose the complete Opportunities loop

**Files:**

- Modify: `internal/api/server.go`, `internal/api/handlers_seo.go`
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Modify: Results UI as needed
- Tests: route/API/UI contract tests

**Steps:**

1. Add canonical aliases for:
   - `POST /projects/{id}/opportunities/runs`
   - `GET /projects/{id}/opportunities/status`
   - `GET /projects/{id}/opportunities`
   - `GET /projects/{id}/opportunities/{opportunityID}`
   - `GET /projects/{id}/growth-actions`
   - `GET /projects/{id}/growth-actions/{actionID}/measurement`
   - `GET /projects/{id}/growth-learnings`
2. Show hypothesis, baseline, primary metric, expected direction/threshold, source freshness, and finite measurement policy before approval.
3. Show checkpoint roles, deadline, outcomes, learnings, and linked real artifacts after execution.
4. Remove user-facing Signal Scan / AI Discovery queue/product-mode language while retaining internal stage diagnostics.

### Task 13: Phase 3 production gate

1. Deploy all Phase 3 slices.
2. In the authenticated production project, run Opportunity Finding and capture:
   - one shared evidence refresh per collection spec;
   - no Doctor work generated;
   - only decision-ready Growth Opportunities shown;
   - held candidates visible only to internal diagnostics;
   - a Growth Action with immutable finite deadline;
   - terminal outcome plus learning or measurement-quality record;
   - future scoring provenance referencing a learning when applicable.
3. Query production constraints and invariant counts.
4. Record remaining gaps; do not start Phase 4 until this gate passes or a documented unrelated blocker is explicitly deferred under the user's skip rule.

---

## Verification commands per slice

Use the smallest targeted RED/GREEN test during implementation, then before merge:

```bash
sqlc generate
go test ./...
go vet ./...
go build ./...
cd web && npm test
cd web && npm run build
```

The user's production-first instruction permits skipping missing local dependency setup. GitHub CI, Vercel/Railway deployment state, production database invariants, and authenticated browser verification remain mandatory.
