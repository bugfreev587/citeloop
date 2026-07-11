# Discovery Phase 1B Semantic Arbitration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the shared arbitration foundation with lock-free AI preparation, versioned bucket snapshots, review memory, stale-safe internal review APIs, and a short-transaction reservation boundary that later Doctor and Growth writers must use.

**Architecture:** Phase A materializes conflict buckets, reads an immutable snapshot, records the AI call, and computes a structured advisory decision without holding database locks. Phase B accepts that prepared decision, locks every bucket in stable order, rebuilds the deterministic snapshot, rejects stale input, and atomically reserves the signature together with a pluggable product-object creator. Automatic suppression remains gated off until a real-data gold-set evaluation marks the configured threshold launch-ready.

**Tech Stack:** Go 1.25, PostgreSQL, pgx v5, sqlc, Chi, existing TokenGate-compatible `llm.Provider`, SHA-256 canonical fingerprints.

---

## Scope boundaries

Included:

- canonical `ai_call_records` for arbitration calls;
- arbitration decisions, snapshot versions, semantic fingerprints, review memory and aliases;
- deterministic-safe, exact-merge, semantic create/merge/suppress/block decisions;
- provider failure and confidence `< 0.80` fail-closed holds;
- short-transaction compare-and-reserve with a `WorkCreator` callback;
- internal admin review list/detail/resolve and semantic evaluation APIs;
- gold-set metrics and an explicit launch gate.

Excluded until their named plans:

- creating canonical `site_fixes` or `growth_actions` (Phase 2/3 creators implement the callback);
- changing current legacy user queues or schedulers;
- automatic semantic suppression before launch readiness;
- user-facing review navigation.

## File map

- Create `internal/migrations/0047_discovery_semantic_arbitration.sql`: arbitration, AI ledger, review memory, alias, evaluation and configuration schema.
- Extend `internal/db/queries/discovery.sql` and generated sqlc output: snapshots, prepared decisions, review memory, Ops reads.
- Create `internal/discovery/semantic.go`: normalized comparison prompt, semantic fingerprint, structured response parser.
- Create `internal/discovery/arbitration.go`: Phase A policy and hold decisions.
- Create `internal/discovery/reservation.go`: prepared-decision validation and pluggable atomic creator contract.
- Create `internal/discovery/postgres_arbitration.go`: pgx pool snapshot and transaction implementation.
- Create `internal/discovery/review.go`: expected-version review resolution.
- Create `internal/discovery/evaluation.go`: labeled gold-set threshold metrics and launch gate.
- Create `internal/api/handlers_admin_discovery_review.go`: internal/admin APIs.

### Task 1: Add Phase 1B schema contracts

**Files:**

- Create: `internal/db/discovery_arbitration_contract_test.go`
- Create: `internal/migrations/0047_discovery_semantic_arbitration.sql`

- [ ] **Step 1: Write the failing schema contract test**

Require the migration to add `candidate_version` and create:

```go
for _, want := range []string{
    "create table if not exists ai_call_records",
    "create table if not exists discovery_arbitration_decisions",
    "create table if not exists work_review_memory",
    "create table if not exists work_signature_aliases",
    "create table if not exists discovery_semantic_gold_cases",
    "create table if not exists discovery_arbitration_configs",
    "expected_bucket_versions jsonb not null",
    "compared_work_ids jsonb not null",
    "automatic_suppression_enabled boolean not null default false",
    "check (not automatic_suppression_enabled or launch_ready)",
} {
    if !strings.Contains(migration, want) { t.Fatalf("missing %q", want) }
}
```

- [ ] **Step 2: Run RED**

Run: `go test ./internal/db -run TestDiscoverySemanticArbitrationSchemaContract -count=1`

Expected: fail because migration 0047 is absent.

- [ ] **Step 3: Add the migration**

Use JSON object/array checks, confidence `0..1`, immutable prompt/model/rules versions, unique active review-memory aliases, and foreign keys to candidates/signatures. Add reservation linkage columns to `work_signature_registry`: `arbitration_decision_id`, `reserved_work_type`, `reserved_work_id`, and `evidence_fingerprint`.

- [ ] **Step 4: Run GREEN and commit**

Run: `go test ./internal/db -run TestDiscoverySemanticArbitrationSchemaContract -count=1`

Commit: `feat: add semantic arbitration schema`

### Task 2: Add snapshot, decision, memory and Ops queries

**Files:**

- Modify: `internal/db/queries/discovery.sql`
- Modify: `internal/db/discovery_arbitration_contract_test.go`
- Generate: `internal/db/discovery.sql.go`, `internal/db/models.go`, `internal/db/querier.go`

- [ ] **Step 1: Add failing query contract assertions**

Require generated queries for `CreateAICallRecord`, `FinishAICallRecord`, `CreateArbitrationDecision`, `MaterializeConflictBuckets`, `GetConflictBucketSnapshot`, `ListSnapshotActiveSignatures`, `ListSnapshotReviewMemory`, `ListDiscoveryReviewItems`, `GetDiscoveryReviewItem`, `UpsertWorkReviewMemory`, and `UpsertWorkSignatureAlias`.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/db -run TestDiscoveryArbitrationQueries -count=1`

Expected: compilation fails because query constants are absent.

- [ ] **Step 3: Add project-scoped queries and generate**

Snapshot reads must use `conflict_bucket_keys ?| sqlc.arg(bucket_keys)::text[]`, filter registry rows to `mode='enforced' and active=true`, and return signature/fingerprint/evidence versions. Review list filters state, age and assignee without exposing it through user project routes.

- [ ] **Step 4: Run GREEN and commit**

Run: `sqlc generate && go test ./internal/db -count=1`

Commit: `feat: add arbitration persistence queries`

### Task 3: Implement semantic comparison outside locks

**Files:**

- Create: `internal/discovery/semantic.go`
- Create: `internal/discovery/semantic_test.go`

- [ ] **Step 1: Write failing tests**

Define:

```go
type SemanticComparator interface {
    Compare(context.Context, SemanticRequest) (SemanticDecision, CallUsage, error)
}

type SemanticDecision struct {
    Decision   DecisionKind
    Owner      Owner
    Overlaps   []uuid.UUID
    Reason     string
    Confidence float64
}
```

Cover canonical prompt stability, valid `create|merge_evidence|suppress|block_on_other_line`, malformed JSON rejection, unknown overlap rejection, confidence bounds, and provider errors.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/discovery -run TestSemantic -count=1`

- [ ] **Step 3: Implement `LLMSemanticComparator`**

Use `llm.Provider.Complete` with JSON output, temperature `0`, a versioned prompt, and no owner/source/recommendation wording in the semantic material. Compute a SHA-256 fingerprint over normalized mutations, artifact intent/spec, topic/entity and audience plus model/prompt version.

- [ ] **Step 4: Run GREEN and commit**

Commit: `feat: compare discovery work semantically`

### Task 4: Implement Phase A preparation and fail-closed policy

**Files:**

- Create: `internal/discovery/arbitration.go`
- Create: `internal/discovery/arbitration_test.go`

- [ ] **Step 1: Write failing service tests**

Cover:

- no possible overlap records `deterministic_safe` without provider invocation;
- exact active hash returns `merge_evidence` without provider invocation;
- possible overlap calls the comparator before any store transaction method;
- confidence `<0.80`, provider failure, incomplete evidence, and provider disagreement create `needs_arbitration_review` without reserve;
- exact/alias review memory applies dismissed/snoozed/watching policy;
- automatic suppress is converted to an internal hold while launch gate is false;
- prepared decision records compared IDs, bucket versions, rules/prompt/model versions and AI call ID.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/discovery -run TestArbitrationPrepare -count=1`

- [ ] **Step 3: Implement Phase A**

Expose `Prepare(ctx, candidateID) (PreparedDecision, error)`. The store interface contains only short materialize/read/write methods and no transaction callback, making provider-in-lock impossible by construction.

- [ ] **Step 4: Run GREEN and commit**

Commit: `feat: prepare discovery arbitration decisions`

### Task 5: Implement Phase B atomic compare-and-reserve

**Files:**

- Create: `internal/discovery/reservation.go`
- Create: `internal/discovery/reservation_test.go`
- Create: `internal/discovery/postgres_arbitration.go`
- Create: `internal/db/discovery_reservation_contract_test.go`

- [ ] **Step 1: Write failing transaction and SQL contract tests**

Define:

```go
type WorkCreator interface {
    CreateInTransaction(context.Context, *db.Queries, ReservedWork) (WorkReference, error)
}

var ErrSnapshotStale = errors.New("arbitration snapshot stale")
```

Require stable ordered `FOR UPDATE` bucket locks, expected/current version equality, active-overlap equality, same-transaction enforced registry insert + creator callback + bucket increments, exact unique conflict merge, and full rollback on creator failure or stale snapshot.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/discovery ./internal/db -run 'TestReserve|TestDiscoveryReservation' -count=1`

- [ ] **Step 3: Implement the pgx transaction boundary**

`ReservePrepared` accepts only persisted prepared decisions with `create` disposition and confidence at or above the frozen threshold. It never accepts a provider/comparator and returns `ErrSnapshotStale` for a caller-controlled Phase A retry.

- [ ] **Step 4: Run GREEN and commit**

Commit: `feat: reserve discovery work atomically`

### Task 6: Add review memory and stale-safe Ops resolution

**Files:**

- Create: `internal/discovery/review.go`
- Create: `internal/discovery/review_test.go`
- Modify: `internal/discovery/postgres_arbitration.go`

- [ ] **Step 1: Write failing tests**

Require `expected_candidate_version` and the complete expected bucket-version map. Test 409-equivalent stale errors, deterministic material-change reopening, signature-version aliases, high-confidence semantic inheritance, and low-confidence semantic hold.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/discovery -run TestReview -count=1`

- [ ] **Step 3: Implement transactional resolution**

Resolution locks the same buckets in the same order, validates versions, writes audited resolution/review memory/aliases, increments all affected bucket versions, and commits. It never creates user work directly.

- [ ] **Step 4: Run GREEN and commit**

Commit: `feat: preserve discovery review memory`

### Task 7: Add gold-set evaluation and launch gate

**Files:**

- Create: `internal/discovery/evaluation.go`
- Create: `internal/discovery/evaluation_test.go`

- [ ] **Step 1: Write failing metrics tests**

Calculate duplicate-safety recall, false suppression rate, comparator coverage, hold rate, threshold-specific backlog, and weekly capacity. Require recall `>=0.95`, false suppression `<0.02`, non-empty versioned dataset, and configured Ops capacity before `launch_ready=true`.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/discovery -run TestSemanticEvaluation -count=1`

- [ ] **Step 3: Implement pure evaluation and persisted configuration update**

Automatic suppression remains false unless launch-ready; config validation rejects attempts to enable it earlier.

- [ ] **Step 4: Run GREEN and commit**

Commit: `feat: gate semantic suppression on evaluation`

### Task 8: Expose internal/admin diagnostics and review APIs

**Files:**

- Create: `internal/api/handlers_admin_discovery_review.go`
- Create: `internal/api/admin_discovery_review_contract_test.go`
- Modify: `internal/api/server.go`

- [ ] **Step 1: Write failing route/auth contracts**

Register inside `requireAdmin`:

```text
POST /api/admin/projects/{projectID}/discovery-arbitration/{candidateID}/prepare
GET  /api/admin/projects/{projectID}/discovery-review
GET  /api/admin/projects/{projectID}/discovery-review/{candidateID}
POST /api/admin/projects/{projectID}/discovery-review/{candidateID}/resolve
GET  /api/admin/projects/{projectID}/discovery-semantic-evaluation
POST /api/admin/projects/{projectID}/discovery-semantic-evaluation/run
```

- [ ] **Step 2: Run RED**

Run: `go test ./internal/api -run TestAdminDiscoveryReview -count=1`

- [ ] **Step 3: Implement handlers**

Use `s.Pool`, `s.Q`, and `s.LLM`; return `409` for stale versions, `422` for incomplete candidates, `503` for provider unavailability with a persisted hold, and never expose reserve without a Phase 2/3 `WorkCreator`.

- [ ] **Step 4: Run GREEN and commit**

Commit: `feat: expose discovery ops review APIs`

### Task 9: Full verification, review, deploy and production proof

- [ ] Run `sqlc generate` and require no generated diff.
- [ ] Run `go test ./... && go vet ./...`.
- [ ] Run `cd web && npm test && npm run typecheck`.
- [ ] Run `git diff --check origin/main...HEAD` and require a clean worktree.
- [ ] Obtain independent review with no Critical/Important findings.
- [ ] Push, create a ready PR, merge after CI, and wait for Railway/Vercel.
- [ ] Authenticated production proof: prepare one identity-ready real candidate, verify an `ai_call_records` row or deterministic-safe skip, read the persisted decision/review detail, and confirm no legacy Doctor/Opportunity row changed.
- [ ] Export real candidate pairs for the gold set; keep automatic semantic suppression disabled unless the measured launch gate passes.
