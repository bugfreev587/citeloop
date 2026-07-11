# Doctor / Opportunities Phase 1A Work Identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship an internal, shadow-only candidate and owner-neutral work-identity foundation that can inventory current Doctor findings and SEO opportunities, calculate deterministic signatures, and report exact/possible conflicts without changing user queues.

**Architecture:** Add forward-only PostgreSQL tables for shadow runs, candidates, conflict buckets, signature registry rows, and future review items. A focused `internal/discovery` package normalizes legacy Doctor/Opportunity objects into the PRD candidate envelope, computes deterministic hashes and bucket keys, and persists them through a repository. Admin-only endpoints run the projection and return diagnostics; Phase 1A never reserves enforced work, suppresses legacy rows, calls an AI provider, or changes Doctor/Opportunity behavior.

**Tech Stack:** Go 1.x, PostgreSQL 16, pgx v5, sqlc 1.30, Chi, existing Go test suite.

---

## Scope boundaries

Included:

- PRD Phase 0 inventory and provisional work signatures.
- PRD Phase 1 internal candidate envelope and deterministic identity.
- Shadow exact-duplicate and conflict-bucket diagnostics.
- Admin-only run/report endpoints.
- Schema foundations for later enforced reservation and review queues.

Excluded from this PR:

- AI/embedding semantic calls and semantic suppression.
- Enforced reservations or bucket-lock compare-and-reserve.
- Changes to existing `UpsertSEOOpportunity`, Doctor writers, user queues, schedulers, or navigation.
- Site Fix ownership migration, Growth contract enforcement, and UI loops.

## File map

- Create `internal/migrations/0046_discovery_work_identity.sql`: Phase 1A schema and constraints.
- Create `internal/db/queries/discovery.sql`: sqlc persistence and inventory/report queries.
- Create `internal/db/discovery_contract_test.go`: migration/query contract tests.
- Generate `internal/db/discovery.sql.go`; update generated `internal/db/models.go` and `internal/db/querier.go` through `sqlc generate`.
- Create `internal/discovery/identity.go`: candidate validation, canonical payload, exact hash, and bucket keys.
- Create `internal/discovery/identity_test.go`: deterministic identity tests.
- Create `internal/discovery/projector.go`: legacy Doctor/Opportunity candidate projection.
- Create `internal/discovery/projector_test.go`: ownership, mutation, and hold-state tests.
- Create `internal/discovery/service.go`: shadow run orchestration and report aggregation.
- Create `internal/discovery/service_test.go`: real orchestration against an in-memory repository fake.
- Create `internal/discovery/postgres.go`: sqlc-backed repository adapter.
- Create `internal/api/handlers_admin_discovery.go`: admin run/report handlers.
- Create `internal/api/admin_discovery_contract_test.go`: route/auth/source contract tests.
- Modify `internal/api/server.go`: register two admin-only endpoints.

### Task 1: Define the shadow schema with contract tests

**Files:**

- Create: `internal/db/discovery_contract_test.go`
- Create: `internal/migrations/0046_discovery_work_identity.sql`

- [ ] **Step 1: Write the failing migration contract test**

The test must read `0046_discovery_work_identity.sql` and require:

```go
for _, want := range []string{
    "create table if not exists discovery_shadow_runs",
    "create table if not exists discovery_candidates",
    "create table if not exists work_conflict_buckets",
    "create table if not exists work_signature_registry",
    "create table if not exists discovery_review_items",
    "mode text not null default 'shadow'",
    "where mode = 'enforced' and active = true",
    "unique (project_id, bucket_key)",
    "unique (candidate_id)",
} {
    if !strings.Contains(migration, want) {
        t.Fatalf("discovery migration missing %q", want)
    }
}
```

- [ ] **Step 2: Run the test and verify RED**

Run: `go test ./internal/db -run TestDiscoveryWorkIdentitySchemaContract -count=1`

Expected: FAIL because migration `0046_discovery_work_identity.sql` does not exist.

- [ ] **Step 3: Add the migration**

Create the five tables with project foreign keys, JSON object/array checks, status/owner/mode checks, timestamps, and these invariants:

```sql
create unique index if not exists uniq_enforced_active_work_signature
  on work_signature_registry (project_id, exact_signature_hash)
  where mode = 'enforced' and active = true;

create unique index if not exists uniq_discovery_candidate_source_version
  on discovery_candidates
    (project_id, source_kind, source_object_type, source_object_id, candidate_schema_version);
```

Shadow registry rows intentionally allow the same exact hash more than once so duplicate reporting can observe collisions. Add indexes for project/run/status, exact hash, and GIN conflict bucket lookup.

- [ ] **Step 4: Run the contract test and verify GREEN**

Run: `go test ./internal/db -run TestDiscoveryWorkIdentitySchemaContract -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/migrations/0046_discovery_work_identity.sql internal/db/discovery_contract_test.go
git commit -m "feat: add discovery work identity schema"
```

### Task 2: Add sqlc queries and generated database types

**Files:**

- Modify: `internal/db/discovery_contract_test.go`
- Create: `internal/db/queries/discovery.sql`
- Generate: `internal/db/discovery.sql.go`
- Generate: `internal/db/models.go`
- Generate: `internal/db/querier.go`

- [ ] **Step 1: Write failing query contract assertions**

Require non-empty generated queries for:

```go
[]string{
    createDiscoveryShadowRun,
    completeDiscoveryShadowRun,
    upsertDiscoveryCandidate,
    upsertShadowWorkSignature,
    ensureWorkConflictBucket,
    listActiveSEOOpportunitiesForDiscoveryShadow,
    listActiveDoctorFindingsForDiscoveryShadow,
    getLatestDiscoveryShadowRun,
    listDiscoveryShadowSignaturesForRun,
}
```

Also assert the legacy inventory queries are project-scoped and bounded to active/reviewable statuses.

- [ ] **Step 2: Run the test and verify RED**

Run: `go test ./internal/db -run TestDiscoveryQueriesExposeShadowFoundation -count=1`

Expected: FAIL to compile because generated query constants do not exist.

- [ ] **Step 3: Add `discovery.sql`**

Implement idempotent candidate/signature upserts, bucket materialization at version `0`, run completion counters, active legacy inventory queries, and a report query returning candidate/signature rows for one run. No query may update `seo_opportunities` or `seo_doctor_findings`.

- [ ] **Step 4: Generate sqlc code**

Run: `sqlc generate`

Expected: generated `internal/db/discovery.sql.go` plus model/interface updates.

- [ ] **Step 5: Run database tests and verify GREEN**

Run: `go test ./internal/db -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/db/queries/discovery.sql internal/db/discovery.sql.go internal/db/models.go internal/db/querier.go internal/db/discovery_contract_test.go
git commit -m "feat: add discovery shadow queries"
```

### Task 3: Implement deterministic owner-neutral identity with TDD

**Files:**

- Create: `internal/discovery/identity_test.go`
- Create: `internal/discovery/identity.go`

- [ ] **Step 1: Write failing identity tests**

Tests must define the desired API:

```go
identity, err := BuildIdentity(Candidate{
    ProjectID:           projectID,
    NormalizedTargetSet: []string{"https://example.com/pricing", "https://example.com/pricing"},
    ChangeFamily:        "metadata.title",
    ProposedMutations: []Mutation{{Operation: "update", Field: "title"}},
    ArtifactIntent:      ArtifactRepairExistingSurface,
    TopicEntityIdentity: []string{"pricing-software"},
    AudienceIdentity:    []string{"smb", "en-US"},
    SignatureVersion:    "work-signature-v1",
})
```

Cover:

- target/mutation/entity/audience input order does not change hash;
- owner, detector/source, confidence, and success metric do not affect hash;
- `add:title` and `update:title` produce different hashes;
- same target/change produces stable target and change-family bucket keys;
- missing target or mutation returns `ErrNeedsSpecification`;
- `create_new_asset` without intended slug/canonical returns `ErrNeedsSpecification`.

- [ ] **Step 2: Run identity tests and verify RED**

Run: `go test ./internal/discovery -run 'TestBuildIdentity|TestValidateCandidate' -count=1`

Expected: FAIL because the package/API does not exist.

- [ ] **Step 3: Implement the minimal identity builder**

Use typed structs, whitespace/case normalization, stable sorting/deduplication, `encoding/json`, and SHA-256. The signature payload contains only PRD §10.2 fields. Bucket keys contain project plus target/topic/slug and a coarse change family; return them sorted and deduplicated.

- [ ] **Step 4: Run identity tests and verify GREEN**

Run: `go test ./internal/discovery -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/discovery/identity.go internal/discovery/identity_test.go
git commit -m "feat: build owner neutral work signatures"
```

### Task 4: Project legacy Doctor and Opportunity rows into candidates

**Files:**

- Create: `internal/discovery/projector_test.go`
- Create: `internal/discovery/projector.go`

- [ ] **Step 1: Write failing projection tests**

Cover these source-grounded cases:

```text
Doctor structured_data_missing -> doctor + immediate + add:jsonld
Opportunity schema_gap on same URL -> doctor suggestion + immediate + add:jsonld
Doctor title missing -> add:title
Opportunity low CTR title -> opportunities + delayed + update:title
AI citation gap needing new evidence -> opportunities + delayed + content evidence mutation
missing normalized target -> needs_specification and no signature
```

Tests must prove that equivalent Doctor/Opportunity schema work produces the same exact hash while title-missing and CTR-title work do not.

- [ ] **Step 2: Run projector tests and verify RED**

Run: `go test ./internal/discovery -run TestProject -count=1`

Expected: FAIL because projection functions do not exist.

- [ ] **Step 3: Implement focused mapping tables**

Create pure functions:

```go
func ProjectDoctorFinding(db.SeoDoctorFinding) Candidate
func ProjectSEOOpportunity(db.SeoOpportunity) Candidate
```

Use explicit issue/type mappings for schema, title, canonical, robots/noindex, availability, internal links, measurement, evidence content, comparison/new asset, and a conservative unknown fallback to `needs_specification`. Do not infer a precise mutation from free-form copy when the source type is unknown.

- [ ] **Step 4: Run projector and package tests**

Run: `go test ./internal/discovery -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/discovery/projector.go internal/discovery/projector_test.go
git commit -m "feat: project legacy discovery candidates"
```

### Task 5: Persist a shadow run and aggregate duplicate diagnostics

**Files:**

- Create: `internal/discovery/service_test.go`
- Create: `internal/discovery/service.go`
- Create: `internal/discovery/postgres.go`

- [ ] **Step 1: Write failing service tests**

Use an in-memory repository fake and verify:

- all active Doctor findings and Opportunities are projected;
- insufficient specs are stored with `needs_specification` and no registry row;
- valid candidates store one shadow registry row each;
- exact duplicates remain separate shadow rows and increment `exact_duplicate_groups` in the report;
- shared bucket/different hash pairs increment `possible_conflict_groups`;
- no repository method can mutate legacy source rows;
- rerunning the same source/version updates the candidate instead of duplicating it.

- [ ] **Step 2: Run service tests and verify RED**

Run: `go test ./internal/discovery -run TestShadowService -count=1`

Expected: FAIL because the service does not exist.

- [ ] **Step 3: Implement service and repository boundary**

Expose:

```go
func (s *Service) RunProject(ctx context.Context, projectID uuid.UUID) (Report, error)
func (s *Service) LatestReport(ctx context.Context, projectID uuid.UUID) (Report, error)
```

The service creates a run, projects sources, validates/builds identity, persists candidate/signature/buckets, aggregates exact hashes and intersecting buckets, and completes the run with counts. A failed projection completes the run as failed with its error; partial candidate holds are successful diagnostic output.

- [ ] **Step 4: Implement the sqlc-backed repository**

`postgres.go` converts package types to generated sqlc params and database rows back to report inputs. It must not start enforced reservations or call an AI provider.

- [ ] **Step 5: Run package tests and verify GREEN**

Run: `go test ./internal/discovery -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/discovery/service.go internal/discovery/service_test.go internal/discovery/postgres.go
git commit -m "feat: run discovery shadow inventory"
```

### Task 6: Add admin-only shadow run and report endpoints

**Files:**

- Create: `internal/api/admin_discovery_contract_test.go`
- Create: `internal/api/handlers_admin_discovery.go`
- Modify: `internal/api/server.go`

- [ ] **Step 1: Write failing route/source tests**

Register and require:

```text
POST /api/admin/projects/{projectID}/discovery-shadow/run
GET  /api/admin/projects/{projectID}/discovery-shadow/report
```

The route test must reject NotFound/MethodNotAllowed. The source contract must prove the routes live inside the `requireAdmin` group and handlers call only the discovery shadow service.

- [ ] **Step 2: Run API tests and verify RED**

Run: `go test ./internal/api -run TestAdminDiscoveryShadow -count=1`

Expected: FAIL because routes/handlers are missing.

- [ ] **Step 3: Implement handlers**

Parse `projectID`, require a configured database, construct `discovery.NewPostgresRepository(s.Q)`, run/read the service, and return a response containing:

```json
{
  "mode": "shadow",
  "run_id": "uuid",
  "status": "completed",
  "doctor_candidates": 0,
  "opportunity_candidates": 0,
  "identity_ready": 0,
  "needs_specification": 0,
  "exact_duplicate_groups": 0,
  "possible_conflict_groups": 0,
  "created_at": "timestamp",
  "finished_at": "timestamp"
}
```

No response field may expose a user action or imply suppression/enforcement.

- [ ] **Step 4: Run API and discovery tests**

Run: `go test ./internal/api ./internal/discovery -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/server.go internal/api/handlers_admin_discovery.go internal/api/admin_discovery_contract_test.go
git commit -m "feat: expose discovery shadow diagnostics"
```

### Task 7: Full verification and production handoff

**Files:**

- Modify only files required by failures found during verification.

- [ ] **Step 1: Verify generated code is current**

Run: `sqlc generate && git diff --exit-code -- internal/db/models.go internal/db/querier.go internal/db/discovery.sql.go`

Expected: exit `0` after generated files are committed.

- [ ] **Step 2: Run backend verification**

Run: `go test ./... && go vet ./...`

Expected: PASS.

- [ ] **Step 3: Run web regression verification**

Run: `cd web && npm test && npm run typecheck`

Expected: 334 tests pass and TypeScript exits `0` (or the current higher test count if main adds tests).

- [ ] **Step 4: Verify change scope**

Run: `git diff --check origin/main...HEAD && git status -sb`

Expected: no whitespace errors and a clean worktree.

- [ ] **Step 5: Request independent code review**

Reviewer must check schema safety, shadow-only behavior, identity correctness, project/admin scoping, generated code, and absence of legacy queue mutations. Fix all Critical/Important findings and rerun verification.

- [ ] **Step 6: Publish and verify production**

Push the branch, create a ready PR, wait for CI, merge, wait for Railway/Vercel, then verify:

```bash
curl -fsS https://api.citeloop.app/healthz
curl -fsS -o /dev/null -w '%{http_code}' https://citeloop.app
curl -sS -o /dev/null -w '%{http_code}' \
  -X POST https://api.citeloop.app/api/admin/projects/00000000-0000-0000-0000-000000000001/discovery-shadow/run
```

Expected: API health `ok`, web `200`, and unauthenticated production admin route `401`/`403` rather than `404`/`405`. Feature semantics are verified by CI/service tests; the production probe verifies deployment and auth-gated route registration without mutating data.
