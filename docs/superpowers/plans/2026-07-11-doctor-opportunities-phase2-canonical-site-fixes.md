# Doctor-Owned Canonical Site Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `site_fixes` the canonical Doctor-owned work object, migrate active legacy technical work without loss or dual-write, and close every Doctor fix at immediate verification rather than Growth measurement.

**Architecture:** Extend the Phase 1 candidate/arbitration transaction with a Doctor `WorkCreator` that creates one `site_fix` and one enforced work signature atomically. New Doctor work never creates `seo_opportunities` or `content_actions`; legacy technical rows are conserved through immutable migration batches, aliases, and versioned inverse operations. `site_change_applications` accepts exactly one canonical source, while Doctor application/deploy/verification updates `site_fixes` only and releases the signature only at terminal failure.

**Tech Stack:** Go 1.25, chi, pgx/sqlc, PostgreSQL migrations, Next.js/React/TypeScript, Node contract tests, Railway, Vercel.

---

## Delivery rules

- Phase 2 is one product cutover but remains deploy-safe through additive schema, transactional data migration, then canonical writer activation in the same release.
- There is never a Doctor dual-write to `site_fixes` plus `content_actions`.
- Existing legacy technical actions remain readable through aliases until migrated; after cutover their rows are read-only provenance.
- Every database writer that changes a candidate, active signature, review memory, Site Fix, or application source locks the relevant conflict buckets and increments their versions in the same transaction.
- All behavior changes follow RED → verify RED → GREEN → verify GREEN. Generated sqlc files are regenerated only after the migration/query contract is green.
- Technical Site Fixes stop at `verified`; only Growth actions may enter `measuring`.

### Task 1: Add the canonical Site Fix and migration schema

**Files:**
- Create: `internal/migrations/0048_doctor_site_fixes.sql`
- Create: `internal/db/doctor_site_fix_schema_contract_test.go`
- Modify: `internal/db/migrations_test.go`

- [ ] **Step 1: Write the failing schema contract test**

Add a test that loads `0048_doctor_site_fixes.sql` and requires these exact contracts:

```go
for _, required := range []string{
    "create table if not exists site_fixes",
    "create table if not exists site_fix_verifications",
    "doctor_finding_id uuid not null",
    "candidate_id uuid not null",
    "work_signature_id uuid not null",
    "supersedes_site_fix_id uuid",
    "deferrable initially deferred",
    "create table if not exists migration_batches",
    "create table if not exists migration_ledger",
    "create table if not exists migration_review_items",
    "create table if not exists product_writer_authority",
    "inverse_operation jsonb not null",
    "add column if not exists site_fix_id uuid",
    "num_nonnulls(site_fix_id, content_action_id) = 1",
} {
    if !strings.Contains(sql, required) { t.Fatalf("migration missing %q", required) }
}
```

Also reject any new `site_fixes.seo_opportunity_id`, `site_fixes.content_action_id`, or finding-side `site_fix_id` pointer.

- [ ] **Step 2: Run the contract test and verify RED**

Run: `go test ./internal/db -run TestDoctorSiteFixSchemaContract -count=1`

Expected: FAIL because migration 0048 does not exist.

- [ ] **Step 3: Add the additive schema**

The migration must create:

```sql
create table if not exists site_fixes (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  doctor_finding_id uuid not null references seo_doctor_findings(id) on delete restrict,
  candidate_id uuid not null references discovery_candidates(id) on delete restrict,
  work_signature_id uuid not null,
  supersedes_site_fix_id uuid references site_fixes(id) on delete restrict,
  status text not null check (status in (
    'proposed','approved','preparing','ready_to_apply','applying',
    'awaiting_deploy','verifying','verified','failed_retryable',
    'reopened','failed_terminal','superseded','migration_rolled_back'
  )),
  finding_kind text not null check (finding_kind in ('broken','optimization')),
  target_urls jsonb not null default '[]'::jsonb check (jsonb_typeof(target_urls) = 'array'),
  evidence_snapshot jsonb not null default '{}'::jsonb,
  proposed_fix jsonb not null default '{}'::jsonb,
  acceptance_tests jsonb not null default '[]'::jsonb,
  verification_snapshot jsonb not null default '{}'::jsonb,
  failure_reason text,
  retry_count int not null default 0 check (retry_count >= 0),
  max_retries int not null default 3 check (max_retries >= 0),
  legacy_opportunity_id uuid references seo_opportunities(id) on delete set null,
  legacy_content_action_id uuid references content_actions(id) on delete set null,
  migration_batch_id uuid,
  approved_at timestamptz,
  applied_at timestamptz,
  deployed_at timestamptz,
  verified_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint fk_site_fixes_work_signature foreign key (work_signature_id)
    references work_signature_registry(id) deferrable initially deferred,
  unique (project_id, candidate_id, supersedes_site_fix_id)
);
```

Create a partial unique index preventing two active Site Fixes for the same signature. Create append-only `site_fix_verifications` rows containing evidence reads, acceptance results, AI call linkage, retry classification, and timestamps. Create `migration_batches`, `migration_ledger`, `migration_review_items`, `legacy_object_aliases`, and project-scoped `product_writer_authority`/write-fence state with immutable batch/object/operation/inverse snapshots. Add `site_change_applications.site_fix_id`, make `content_action_id` nullable, and add a `NOT VALID` one-of-source check followed by validation after existing rows are confirmed. Extend rollback records to a one-of-source model so canonical Site Fix apply rollback does not depend on a `content_action_id`.

Add `finding_kind` and healthy coverage JSON to Doctor findings/runs without adding a finding-side current-fix pointer. Expand `seo_doctor_runs.trigger` for `migration` and discovery run mode for canonical creation without renaming legacy tables yet.

- [ ] **Step 4: Run migration and schema tests**

Run: `go test ./internal/db -run 'TestDoctorSiteFixSchemaContract|TestMigrations' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/migrations/0048_doctor_site_fixes.sql internal/db/doctor_site_fix_schema_contract_test.go internal/db/migrations_test.go
git commit -m "feat: add canonical Doctor Site Fix schema"
```

### Task 2: Add SQLC persistence and state transitions

**Files:**
- Create: `internal/db/queries/site_fixes.sql`
- Create: `internal/db/site_fixes_query_contract_test.go`
- Modify: `internal/db/queries/discovery.sql`
- Modify generated files under: `internal/db/`

- [ ] **Step 1: Write failing query contract tests**

Require named queries for create/get/list/approve/apply/deploy/verify/retry/terminalize, append-only verification attempts, writer authority/fences, alias resolution, migration batch/review/ledger writes, application source repointing, and signature lifecycle updates. The verification query must update application + Site Fix + work registry atomically and must not mention `content_actions` or `measuring`.

```go
if strings.Contains(verifySQL, "content_actions") || strings.Contains(verifySQL, "measuring") {
    t.Fatal("Doctor verification must stop at canonical Site Fix verified")
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/db -run 'TestCanonicalSiteFixQueries|TestDoctorVerificationStopsAtVerified' -count=1`

Expected: FAIL because queries do not exist.

- [ ] **Step 3: Implement SQL transitions**

Implement narrow state guards, including:

```sql
-- name: MarkCanonicalSiteFixVerified :one
with verified_application as (
  update site_change_applications
  set status = 'verified', deployment_snapshot = sqlc.arg(deployment_snapshot)::jsonb,
      verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
      failure_reason = null, deployed_at = coalesce(deployed_at, sqlc.arg(verified_at)),
      verified_at = coalesce(verified_at, sqlc.arg(verified_at)), updated_at = now()
  where id = sqlc.arg(application_id) and project_id = sqlc.arg(project_id)
    and site_fix_id = sqlc.arg(site_fix_id) and content_action_id is null
  returning site_fix_id
), verified_fix as (
  update site_fixes sf
  set status = 'verified', verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
      failure_reason = null, deployed_at = coalesce(deployed_at, sqlc.arg(verified_at)),
      verified_at = coalesce(verified_at, sqlc.arg(verified_at)), updated_at = now()
  from verified_application va
  where sf.id = va.site_fix_id and sf.project_id = sqlc.arg(project_id)
  returning sf.*
)
update work_signature_registry w
set status = 'verified', active = false, updated_at = now()
from verified_fix vf
where w.id = vf.work_signature_id and w.project_id = vf.project_id
returning vf.*;
```

Keep `failed_retryable` and `reopened` active; only `verified`, `failed_terminal`, `superseded`, and migration rollback deactivate the registry. All transition queries lock/update conflict bucket versions.

- [ ] **Step 4: Regenerate sqlc and prove no drift**

Run: `sqlc generate`

Run: `git diff --exit-code -- internal/db/queries internal/db/*.go` only after staging expected generated output in the task review; generated output must match the source queries.

- [ ] **Step 5: Run database tests**

Run: `go test ./internal/db -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/db/queries/site_fixes.sql internal/db/queries/discovery.sql internal/db
git commit -m "feat: persist canonical Site Fix lifecycle"
```

### Task 3: Make arbitration create the Site Fix and signature atomically

**Files:**
- Modify: `internal/discovery/reservation.go`
- Modify: `internal/discovery/postgres_arbitration.go`
- Modify: `internal/discovery/reservation_test.go`
- Create: `internal/sitefix/creator.go`
- Create: `internal/sitefix/creator_test.go`
- Create: `internal/sitefix/candidate.go`
- Create: `internal/sitefix/candidate_test.go`

- [ ] **Step 1: Write failing reservation tests**

Require `ReservedWork` to expose a pre-generated `WorkSignatureID`, require the creator to receive it before its insert, and verify a creator error leaves no Site Fix, signature, decision update, or bucket increment.

```go
type ReservedWork struct {
    ProjectID, CandidateID, DecisionID, WorkSignatureID uuid.UUID
    Owner Owner
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/discovery -run 'TestReservationPassesPreallocatedSignature|TestReservationRollsBackCreatorFailure' -count=1`

Expected: FAIL because `WorkSignatureID` is absent.

- [ ] **Step 3: Preallocate the registry ID in the serializable transaction**

Generate the UUID after snapshot validation but before `CreateInTransaction`, pass it to the creator, and insert the enforced signature using that exact ID. The deferred FK lets the Site Fix insert precede the signature insert while the transaction commits only when both exist.

- [ ] **Step 4: Write failing Site Fix creator tests**

Test that a Doctor candidate produces exactly one `site_fixes` row with `doctor_finding_id`, `candidate_id`, `work_signature_id`, evidence, mutation proposal, acceptance tests, and no legacy opportunity/action IDs. Reject non-Doctor owner, incomplete candidate, Healthy finding, mismatched project, and a revision whose predecessor is still active.

- [ ] **Step 5: Implement `sitefix.Creator` and candidate materialization**

`sitefix.Creator.CreateInTransaction` must use only the transaction-bound `*db.Queries`:

```go
func (c Creator) CreateInTransaction(ctx context.Context, q *db.Queries, work discovery.ReservedWork) (discovery.WorkReference, error) {
    if work.Owner != discovery.OwnerDoctor { return discovery.WorkReference{}, ErrWrongOwner }
    finding, err := q.GetSEODoctorFindingForUpdate(ctx, ...)
    // validate candidate source/version and finding_kind != healthy
    row, err := q.CreateCanonicalSiteFix(ctx, db.CreateCanonicalSiteFixParams{
        ID: uuid.New(), ProjectID: work.ProjectID, DoctorFindingID: finding.ID,
        CandidateID: work.CandidateID, WorkSignatureID: work.WorkSignatureID,
        Status: "proposed", ...,
    })
    return discovery.WorkReference{Type: "site_fix", ID: row.ID}, err
}
```

Materialize one enforced candidate from `discovery.ProjectDoctorFinding`, reusing the existing candidate schema/version and conflict buckets. Do not call a semantic provider inside a transaction.

- [ ] **Step 6: Run focused tests**

Run: `go test ./internal/discovery ./internal/sitefix -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/discovery internal/sitefix
git commit -m "feat: reserve Doctor Site Fixes atomically"
```

### Task 4: Cut Doctor creation over to canonical APIs without dual-write

**Files:**
- Modify: `internal/api/server.go`
- Modify: `internal/api/handlers_seo_doctor.go`
- Create: `internal/api/handlers_site_fixes.go`
- Create: `internal/api/site_fixes_api_test.go`
- Modify: `internal/api/seo_routes_test.go`
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Write failing route and no-dual-write tests**

Require:

```text
POST /projects/{id}/doctor/findings/{findingID}/site-fixes
GET  /projects/{id}/doctor/site-fixes
GET  /projects/{id}/doctor/site-fixes/{fixID}
POST /projects/{id}/doctor/site-fixes/{fixID}/approve
POST /projects/{id}/doctor/site-fixes/{fixID}/apply
POST /projects/{id}/doctor/site-fixes/{fixID}/verify
```

The legacy `/doctor/findings/{findingID}/convert` alias must return the canonical Site Fix and `Deprecation`/`Link` headers. Source-contract tests must prove the handler no longer calls `UpsertSEOOpportunity`, `persistContentActionFromOpportunity`, or `LinkSEODoctorFindingToAction`.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/api -run 'TestCanonicalDoctorSiteFixRoutes|TestDoctorSiteFixCreationHasSingleWriter' -count=1`

Expected: FAIL.

- [ ] **Step 3: Implement prepare → reserve → create flow**

The creation handler must:

1. Load and validate the finding.
2. Persist/refresh its enforced candidate outside locks.
3. Prepare arbitration; if held, return 409/422 with the internal reason but never expose a third product queue.
4. Call `ReservationService.ReservePrepared` with `sitefix.Creator`.
5. Return the Site Fix with candidate/signature provenance.
6. On an existing exact reservation or repeated request, resolve the registry work reference and return 200 idempotently.

List/get/approve are canonical `site_fixes` reads/writes only. Healthy coverage cannot create a Site Fix.

- [ ] **Step 4: Verify focused API tests**

Run: `go test ./internal/api ./internal/sitefix ./internal/discovery -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api cmd/api
git commit -m "feat: expose Doctor-owned Site Fix APIs"
```

### Task 5: Move apply, PR reconciliation, deploy, and verify to `site_fix_id`

**Files:**
- Create: `internal/sitefix/apply.go`
- Create: `internal/sitefix/apply_test.go`
- Modify: `internal/api/handlers_site_fixes.go`
- Modify: `internal/api/handlers_seo.go`
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/scheduler/sitefix_verify.go`
- Modify: `internal/scheduler/sitefix_verify_test.go`
- Create: `internal/scheduler/canonical_sitefix_contract_test.go`

- [ ] **Step 1: Write failing lifecycle tests**

Cover:

- proposed → approved → preparing/ready_to_apply → applying → awaiting_deploy;
- PR merged is applied/awaiting deploy, never verified;
- deployment observed → verifying;
- acceptance pass → verified;
- retryable failure keeps registry active;
- retry exhaustion/user termination → failed_terminal and releases registry;
- canonical scheduler queries require `site_fix_id is not null and content_action_id is null`;
- no fuzzy auto-verify: an unverifiable fix becomes `failed_retryable`/manual verification, because verification must execute acceptance tests.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/sitefix ./internal/scheduler -run 'TestCanonical|TestDoctor' -count=1`

Expected: FAIL on legacy content-action transitions and fuzzy auto-verification.

- [ ] **Step 3: Extract a canonical apply service**

Reuse GitHub source mapping/patch generation helpers, but take a `db.SiteFix` and create/reuse an application with only `site_fix_id`. Record AI generation in `ai_call_records(stage='fix_generation', linked_object_type='site_fix')`; every retry is a new call record. State changes must be transactionally paired with application changes.

- [ ] **Step 4: Replace canonical reconcile and verification transitions**

Keep legacy reconciliation only for unmigrated legacy rows behind a read-only compatibility path. Canonical paths update Site Fix and application together. Verification fetches/render-checks the target and evaluates stored acceptance tests; AI review, when authorized, is outside the DB transaction and is recorded in `ai_call_records(stage='verification')`.

Remove the canonical use of `MarkSiteChangeApplicationAndContentActionVerified`. Keep that query only until Phase 5 removes legacy compatibility.

- [ ] **Step 5: Run lifecycle tests**

Run: `go test ./internal/api ./internal/sitefix ./internal/scheduler ./internal/db -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/api internal/sitefix internal/scheduler internal/db
git commit -m "feat: close Doctor fixes with immediate verification"
```

### Task 6: Migrate legacy technical work with ledger-backed rollback

**Files:**
- Create: `internal/sitefix/migration.go`
- Create: `internal/sitefix/migration_test.go`
- Create: `internal/api/admin_site_fix_migration.go`
- Create: `internal/api/admin_site_fix_migration_test.go`
- Modify: `internal/api/server.go`
- Modify: `internal/db/queries/site_fixes.sql`
- Regenerate: `internal/db/*.go`

- [ ] **Step 1: Write failing conservation and rollback tests**

Use a table-driven fixture containing unexecuted technical opportunities, approved/in-flight content actions, exact duplicates, ambiguous rows, existing applications, and a technical action without a Doctor finding. Prove:

```text
N legacy technical actions
= migrated site_fixes
+ archived exact duplicates
+ migration_review
```

Prove every application has exactly one source, every legacy ID resolves to a canonical alias, every created row has a ledger operation and versioned inverse, and rollback restores counts/authority while leaving canonical tombstones.

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/sitefix -run 'TestMigrationDryRunConservesRows|TestMigrationRollbackRestoresSingleWriter' -count=1`

Expected: FAIL.

- [ ] **Step 3: Implement dry-run classification**

Classify by current owner-neutral identity. Create a trigger=`migration` Doctor run/finding when no canonical finding exists. Evidence/target ambiguity becomes `migration_review`; never guess or create an orphan Site Fix.

- [ ] **Step 4: Implement forward migration and inverse projector**

Inside project-scoped write fences:

- create immutable batch/ledger records;
- create migration findings before Site Fixes;
- create Site Fixes and enforced registry rows atomically;
- repoint applications to `site_fix_id` using one-of-source;
- write aliases for legacy opportunity/action/application IDs;
- mark legacy rows canonical-read-only, never hard-delete;
- retain `dismissed/snoozed/watching` through work review memory/aliases;
- refuse rollback when a post-cutover canonical-only revision/action cannot be losslessly inverse-projected.
- switch `product_writer_authority` only after all conservation checks pass, and never expose legacy and canonical technical writers simultaneously.

- [ ] **Step 5: Add authenticated admin dry-run/apply/rollback/report APIs**

```text
POST /api/admin/projects/{id}/site-fix-migration/dry-run
POST /api/admin/projects/{id}/site-fix-migration/apply
POST /api/admin/projects/{id}/site-fix-migration/{batchID}/rollback
GET  /api/admin/projects/{id}/site-fix-migration/{batchID}
```

Mutating calls require expected snapshot hash and return 409 on drift.

- [ ] **Step 6: Run migration tests and regenerate sqlc**

Run: `sqlc generate`

Run: `go test ./internal/sitefix ./internal/api ./internal/db -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/sitefix internal/api internal/db
git commit -m "feat: migrate legacy technical work to Doctor"
```

### Task 7: Stop Opportunities technical repair writers and expand Doctor coverage

**Files:**
- Modify: `internal/seo/actionable_opportunities.go`
- Modify: `internal/seo/actionable_opportunities_test.go`
- Modify: `internal/seo/service.go`
- Modify: `internal/geo/service.go`
- Modify: `internal/api/handlers_autopilot.go`
- Modify: `internal/seo/doctor.go`
- Modify: `internal/seo/doctor_test.go`
- Create: `internal/seo/doctor_coverage_test.go`

- [ ] **Step 1: Write failing ownership tests**

Require Opportunity candidate generation to exclude immediate repair types (`schema_gap`, `technical_visibility_issue`, basic/zero internal links, robots/canonical/indexability, crawler access repair, missing metadata). Require delayed growth hypotheses such as CTR title tests and internal-link strategy to remain.

- [ ] **Step 2: Write failing Doctor coverage tests**

Require `broken`, `optimization`, and per-check `healthy` coverage. Add deterministic Optimization examples that preserve intent/propositions (metadata readability/duplicate template, supported-fact extractability, source association, entity naming) and never add new facts. Healthy coverage must report checked/passed/failed/skipped URLs and must not classify unchecked/partial-crawl pages as healthy.

- [ ] **Step 3: Verify RED**

Run: `go test ./internal/seo ./internal/geo -run 'TestOpportunityFindingExcludesImmediateRepairs|TestDoctorReportsBrokenOptimizationHealthy' -count=1`

Expected: FAIL.

- [ ] **Step 4: Implement ownership cutover**

Remove technical candidate append paths from Opportunities, direct `indexing_anomaly` writes, and guarded-autopilot technical action creation. GEO crawler-access observations remain shared evidence and feed Doctor; they do not directly create Growth opportunities. Existing legacy technical opportunities remain readable/migratable but no new rows are written. Enforce the cutover with `product_writer_authority`, not asset-type heuristics alone.

- [ ] **Step 5: Implement Doctor optimization and healthy coverage**

Persist `finding_kind` for Broken/Optimization and structured healthy coverage in run output. Healthy is coverage, not an actionable Site Fix. Citation-readiness optimizations must persist `preserved_propositions`, `added_propositions`, `removed_propositions`, and `source_association_changes`; any non-empty `added_propositions` fails closed and routes to Opportunities candidate generation instead.

- [ ] **Step 6: Run SEO/GEO tests**

Run: `go test ./internal/seo ./internal/geo ./internal/opportunityfinding -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/seo internal/geo internal/opportunityfinding
git commit -m "feat: give Doctor exclusive technical repair ownership"
```

### Task 8: Switch the Doctor and Site Fixes UI to canonical objects

**Files:**
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/types.ts`
- Modify: `web/app/projects/[id]/doctor/doctor-client.tsx`
- Modify: `web/app/projects/[id]/site-fixes/site-fixes-client.tsx`
- Modify: `web/app/lib/site-fix.ts`
- Modify: `web/app/lib/api.test.mjs`
- Modify: `web/app/lib/seo-client-contract.test.mjs`
- Modify: `web/app/lib/action-portfolio-contract.test.mjs`

- [ ] **Step 1: Write failing API/UI contract tests**

Require canonical methods and paths:

```ts
listDoctorSiteFixes(projectID)
createDoctorSiteFix(projectID, findingID)
approveDoctorSiteFix(projectID, fixID)
applyDoctorSiteFix(projectID, fixID)
verifyDoctorSiteFix(projectID, fixID)
```

Reject use of `SEOContentAction`, `/seo/actions`, opportunity status, Growth measuring labels, and `api.createSiteFixGitHubPR` inside the Site Fixes page.

- [ ] **Step 2: Verify RED**

Run: `npm test -- --test-name-pattern='canonical Doctor Site Fix|Site Fixes page'`

Expected: FAIL.

- [ ] **Step 3: Add canonical types and normalization**

Define `DoctorSiteFix` with finding/candidate/signature IDs, PR/application snapshot, exact lifecycle status, acceptance/verification evidence, legacy provenance, and revision linkage. Do not make Growth fields required or synthesize `measuring`.

- [ ] **Step 4: Replace UI data flows**

Doctor “Add to Site Fixes” calls the canonical create endpoint. Site Fixes lists canonical rows, renders proposed → approved → apply → deploy → verify, keeps `applied` distinct from `verified`, shows retryable/terminal failure accurately, and links the source finding. Legacy aliases returned by the backend normalize to the canonical ID before selection/deep-linking.

Do not perform the broader Phase 4 Home/Results redesign in this task.

- [ ] **Step 5: Run Web verification**

Run: `npm test`

Run: `npm run typecheck`

Run: `npm run build`

Expected: 0 failures and successful Next.js build.

- [ ] **Step 6: Commit**

```bash
git add web/app
git commit -m "feat: render canonical Doctor Site Fixes"
```

### Task 9: Full verification, review, migration rehearsal, and production proof

**Files:**
- Modify only files required by review findings.

- [ ] **Step 1: Run generated-code and repository checks**

Run: `sqlc generate`

Run: `git diff --exit-code`

If generated changes appear, inspect and commit them; then rerun until clean.

Run: `go test ./...`

Run: `go vet ./...`

Run: `go test -race ./internal/discovery ./internal/sitefix ./internal/api ./internal/scheduler`

Run in `web/`: `npm test && npm run typecheck && npm run build`

- [ ] **Step 2: Execute two-stage review**

Dispatch a fresh spec-compliance reviewer against PRD Phase 2 and acceptance criteria 3–17, 29–31, 35, 37, 39–46 plus the executable concurrency, blocker, migration rollback, settings-authority, and Site Fix conservation scenarios relevant to this phase. Fix all gaps and re-review. Then dispatch a fresh code-quality reviewer, fix all Important/Critical issues, and re-review.

- [ ] **Step 3: Run local database migration drills**

On representative fixtures, run dry-run → apply → verify counts/relationships → rollback → verify conservation/single-writer → apply again. Capture batch IDs, before/after hashes, and invariant reports.

- [ ] **Step 4: Publish and merge through PR**

Push `codex/doctor-opportunities-phase2-canonical-site-fix`, open a PR to `main`, wait for required checks, merge, then wait for Railway and Vercel production deployments for the merge SHA.

- [ ] **Step 5: Run authenticated production migration dry-run**

Verify production counts before mutation. Run the admin dry-run and confirm every legacy technical row is classified exactly once. Apply only when conservation, aliases, review memory, rollback eligibility, and one-of-source checks are all green.

- [ ] **Step 6: Prove the live Doctor loop**

On a real production Doctor finding:

1. create a canonical Site Fix;
2. prove the candidate, arbitration decision, Site Fix, enforced signature, and bucket version committed atomically;
3. repeat concurrently and prove exactly one active reservation/work item;
4. if a semantic overlap exists, prove the AI call ledger row and no locks during provider latency;
5. approve/apply through a real or safe no-op GitHub source path;
6. prove applied/merged is not verified;
7. observe deployment and run acceptance verification;
8. prove Site Fix ends at `verified`, registry becomes inactive, and no `content_action` enters `measuring`;
9. verify retryable failure retains the active signature and terminal failure releases it;
10. confirm Opportunities no longer emits immediate technical repair work.

- [ ] **Step 7: Production health and rollback evidence**

Confirm API health, Railway/Vercel success for the merge SHA, no new error logs, production row conservation, alias resolution, one-of-source application constraints, and a completed rollback rehearsal on an isolated/reversible batch. Only then mark Phase 2 complete and start Phase 3 from a fresh `origin/main` worktree.

## Self-review mapping

- Dedicated `site_fixes`, authoritative finding/candidate/signature references, revisions without a finding current pointer: Tasks 1–3.
- Atomic owner-neutral reservation and no provider inside locks: Tasks 2–4 and 9.
- No Doctor → Opportunity/Content dual-write: Tasks 4 and 7.
- Apply → deploy → verify, retryable/terminal signature semantics, stop at verified: Tasks 2 and 5.
- Migration findings, aliases, row conservation, ledger/inverse, rollback drills: Tasks 1, 6, and 9.
- Broken/Optimization/Healthy and proposition preservation: Task 7.
- Canonical Doctor/Site Fix API and UI without Phase 4 scope creep: Tasks 4 and 8.
- Production proof, AI ledger, concurrency, migration, and deployment: Task 9.
