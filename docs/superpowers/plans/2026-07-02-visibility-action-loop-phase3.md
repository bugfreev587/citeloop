# Visibility Action Loop Phase 3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expand CiteLoop analysis so it emits deterministic, actionable SEO/GEO recommendations beyond blog publishing.

**Architecture:** Keep existing GSC and GEO analyzers, add a focused SEO analyzer layer that converts latest technical crawl checks, content inventory, and GSC query rollups into first-class opportunities. Use the existing `seo_opportunities.opportunity_key` upsert contract for idempotency, and make Analysis CTAs identify non-blog actions.

**Tech Stack:** Go SEO service, sqlc/Postgres queries, existing GEO analyzer fixtures, Next.js Analysis UI contract tests, `node:test`, `go test`, `make test`.

---

### Task 1: Analyzer Fixture Tests

**Files:**
- Create: `internal/seo/actionable_opportunities_test.go`
- Modify: `internal/geo/service_pr3_test.go`

- [ ] **Step 1: Write failing SEO analyzer tests**

Add tests that call pure helper functions with deterministic fixtures:

```go
func TestActionableSEOOpportunityCandidatesUseTechnicalAndInventorySignals(t *testing.T) {
  checks := []technicalCheckRollup{
    {PageURL: "https://example.com/product", NormalizedPageURL: "/product", StructuredDataStatus: "missing", InternalLinkCount: int32Ptr(0), RawDetails: map[string]any{"body_bytes": 12000}},
    {PageURL: "https://example.com/docs/api", NormalizedPageURL: "/docs/api", RobotsStatus: "noindex", RawDetails: map[string]any{"body_bytes": 32000}},
  }
  inventory := []inventoryEvidenceRollup{
    {URL: "https://example.com/product", EvidenceCount: 0, Summary: "Short landing page"},
  }
  candidates := actionableSEOOpportunityCandidates(checks, inventory, nil)
  requireCandidateTypes(t, candidates, "internal_link_gap", "schema_gap", "thin_evidence_page", "technical_visibility_issue")
}
```

Expected red result: helper types/functions do not exist.

- [ ] **Step 2: Write failing absence tests**

Add a test with healthy crawl rows, enough evidence snippets, and a single query/page rollup. Expected candidate count is zero.

- [ ] **Step 3: Write failing cannibalization test**

Add a test where two page URLs have the same query, meaningful impressions, and close average positions. Expected candidate type is `gsc_query_cannibalization`.

- [ ] **Step 4: Harden GEO evidence test**

Extend the existing GEO PR3 test to assert GEO gap evidence contains `source`, `why_now`, `scoring_method`, and `idempotency_key`.

- [ ] **Step 5: Verify red**

Run:

```bash
go test ./internal/seo ./internal/geo
```

Expected: tests fail for missing SEO helper behavior and missing GEO metadata.

### Task 2: SEO Analyzer Implementation

**Files:**
- Create: `internal/seo/actionable_opportunities.go`
- Modify: `internal/geo/pr3.go`

- [ ] **Step 1: Implement candidate helpers**

Add pure functions for:
- `internal_link_gap`
- `schema_gap`
- `thin_evidence_page`
- `technical_visibility_issue`
- `gsc_query_cannibalization`

Each candidate must include source, why-now copy, scoring method/version, expected impact range, idempotency key, recommended action, expected impact, confidence, effort, and risk.

- [ ] **Step 2: Update GEO evidence metadata**

Add the same metadata keys to `observationEvidence` without changing the existing GEO opportunity types or asset brief behavior.

- [ ] **Step 3: Verify green**

Run:

```bash
go test ./internal/seo ./internal/geo
```

Expected: tests pass.

### Task 3: Analyze Integration and Idempotency

**Files:**
- Modify: `internal/db/queries/seo.sql`
- Modify generated sqlc files under `internal/db`
- Modify: `internal/seo/service.go`
- Modify or add: `internal/db/seo_contract_test.go`

- [ ] **Step 1: Write failing DB/query contract test**

Add assertions that `seo.sql` exposes `ListLatestTechnicalChecks` and that `UpsertSEOOpportunity` keeps hashing `reason` for stable dedupe.

- [ ] **Step 2: Add latest technical check query**

Add sqlc query selecting the latest `seo_sync` technical checks for a project, limited by `limit_rows`.

- [ ] **Step 3: Generate sqlc**

Run:

```bash
make sqlc
```

Expected: generated Go includes `ListLatestTechnicalChecks`.

- [ ] **Step 4: Wire Analyze**

In `Service.Analyze`, call a new `generateActionableSEOOpportunities` after GSC metric opportunities and before cold-start fallbacks. This method loads latest technical checks, inventory, and query rollups, then upserts candidates through the existing opportunity-key dedupe.

- [ ] **Step 5: Verify idempotency contract**

Run:

```bash
go test ./internal/db ./internal/seo
```

Expected: DB contract and SEO tests pass.

### Task 4: Analysis UI Non-Blog Recommendation Contract

**Files:**
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Modify: `web/app/lib/action-portfolio-contract.test.mjs`

- [ ] **Step 1: Write failing UI contract assertions**

Assert that Analysis recognizes `internal_link_gap`, `schema_gap`, `technical_visibility_issue`, `thin_evidence_page`, and `gsc_query_cannibalization`, and includes CTA/copy for direct patch, technical task, evidence refresh, and consolidation recommendations.

- [ ] **Step 2: Implement label/CTA mapping**

Update `findingTypeLabel`, `actionCtaForOpportunity`, and `assetTypeForOpportunity` only where needed so non-blog findings do not fall through to `blog_post`.

- [ ] **Step 3: Verify frontend**

Run:

```bash
cd web
npm test -- app/lib/action-portfolio-contract.test.mjs
npm run typecheck
```

Expected: contract test and typecheck pass.

### Task 5: Full Verification, PR, Deploy, Production Gate

**Files:**
- No additional files.

- [ ] **Step 1: Run local full verification**

Run:

```bash
make test
cd web
npm test
npm run typecheck
npm run build
```

Expected: all commands exit 0.

- [ ] **Step 2: Ship Phase 3**

Commit, push `codex/visibility-action-loop-phase3`, create PR to `origin/main`, merge it, and wait for production deployment.

- [ ] **Step 3: Production verification**

Run analysis on a controlled project with supporting data or create a controlled fixture action source in production if needed. Confirm expected opportunity types appear in the production API/UI, at least one recommendation is non-blog, and rerunning analysis does not create duplicate open opportunities.
