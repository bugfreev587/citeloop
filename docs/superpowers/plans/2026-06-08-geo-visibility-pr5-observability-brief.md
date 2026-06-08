# GEO Visibility PR5 Observability + Brief Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the remaining PRD acceptance gaps by exposing GEO runs, recording skipped prompt/engine metadata, and surfacing GEO blockers/opportunities in the SEO operating brief.

**Architecture:** Reuse `geo_runs` and `seo_opportunities`; no new tables. Provider observation output gains explicit skipped metadata. SEO Brief keeps existing `actions` but adds dedicated `geo_blockers` and `geo_opportunities` fields derived from open `geo_*` opportunities.

**Tech Stack:** Go service/API with sqlc queries already present, pure SEO helper tests, Next.js API contract tests and existing SEO page UI.

---

## Files

- Modify: `internal/geo/provider.go` to add skipped prompt/engine metadata to provider observation results and run output.
- Modify: `internal/geo/service_pr4_test.go` and `internal/geo/service_test.go` to assert run output metadata.
- Modify: `internal/api/handlers_geo_pr2.go`, `internal/api/server.go`, `internal/api/seo_routes_test.go` to add `GET /geo/runs`.
- Modify: `internal/seo/service.go` and create `internal/seo/brief_geo_test.go` for GEO brief helper behavior.
- Modify: `web/app/lib/api.ts` and `web/app/lib/api.test.mjs` for `listGEORuns`, `geo_blockers`, and `geo_opportunities`.
- Modify: `web/app/projects/[id]/seo/seo-client.tsx` to display dedicated GEO brief blockers/opportunities.

## Acceptance

- `go test -count=1 ./internal/geo ./internal/api ./internal/seo` passes.
- `make test` passes.
- `cd web && npm test -- --runInBand` passes.
- `cd web && npm run typecheck` passes.
- `GET /projects/{projectID}/geo/runs` is registered and returns project-scoped GEO runs.
- Provider observation run output includes `skipped_prompts` and `skipped_engines` when budget/sampling/provider availability prevents full observation.
- SEO Brief response includes `geo_blockers` and `geo_opportunities`, and the SEO page renders them separately from generic SEO actions.

---

### Task 1: Provider Skipped Metadata

- [ ] **Step 1: Write failing test**

Extend `TestObserveAnswerProviderUnavailableMarksRunDegraded` or add a new test with two active prompts and `MaxPrompts: 1`. Assert `geoStoreStub.finishedOutput` contains:

```json
{
  "skipped_prompts": [{"prompt_id":"...","reason":"max_prompts"}],
  "skipped_engines": ["Perplexity"]
}
```

- [ ] **Step 2: Verify RED**

Run: `go test -count=1 ./internal/geo`

Expected: FAIL because provider result/run output lacks skipped metadata capture.

- [ ] **Step 3: Implement metadata**

Add fields to `ObserveAnswerProviderResult`:

```go
SkippedPrompts []SkippedPrompt `json:"skipped_prompts"`
SkippedEngines []string `json:"skipped_engines"`
```

Set skipped prompts when `MaxPrompts` samples a smaller prompt set. Set skipped engine when provider is nil/unavailable or returns an error.

- [ ] **Step 4: Verify GREEN**

Run: `go test -count=1 ./internal/geo`

Expected: PASS.

### Task 2: GEO Runs API

- [ ] **Step 1: Add failing route/client tests**

Add route test for `GET /api/projects/{projectID}/geo/runs`.

Add frontend contract test:

```js
await client.listGEORuns("project-1", { agent: "geo_observer", status: "degraded", limit: 10 });
```

Expected URL:

```text
https://api.example.test/api/projects/project-1/geo/runs?agent=geo_observer&status=degraded&limit=10
```

- [ ] **Step 2: Verify RED**

Run:
- `go test -count=1 ./internal/api`
- `cd web && npm test -- --runInBand`

Expected: FAIL because API route/client helper is missing.

- [ ] **Step 3: Implement route and client**

Backend handler uses existing `db.ListGEORuns` with `agent`, `status`, `limit`, and `cursor` query params.

Frontend adds `GEORun` type and `listGEORuns`.

- [ ] **Step 4: Verify GREEN**

Run:
- `go test -count=1 ./internal/api`
- `cd web && npm test -- --runInBand`

Expected: PASS.

### Task 3: SEO Brief GEO Fields

- [ ] **Step 1: Write failing helper test**

Create `internal/seo/brief_geo_test.go` for a pure helper:

```go
geoBlockers, geoOpps := briefGEOSections([]db.SeoOpportunity{
  {Type: "geo_crawler_access_blocked", PageUrl: ptr("https://example.com/blocked")},
  {Type: "geo_competitor_cited_project_absent", Query: ptr("best tools")},
  {Type: "indexing_anomaly"},
})
```

Assert one blocker string and one GEO opportunity.

- [ ] **Step 2: Verify RED**

Run: `go test -count=1 ./internal/seo`

Expected: FAIL because helper/fields are missing.

- [ ] **Step 3: Implement helper and brief fields**

Extend `seo.Brief`:

```go
GEOBlockers []string `json:"geo_blockers"`
GEOOpportunities []db.SeoOpportunity `json:"geo_opportunities"`
```

In `Brief`, fetch enough open opportunities (`LimitRows: 50`), set generic `Actions` to the top 10, then compute `geo_blockers` and top 5 non-blocker `geo_*` opportunities.

- [ ] **Step 4: Verify GREEN**

Run: `go test -count=1 ./internal/seo`

Expected: PASS.

### Task 4: Frontend Brief Rendering

- [ ] **Step 1: Add frontend contract test**

Update SEO brief normalization test to assert `geo_blockers` and `geo_opportunities` normalize nulls to arrays.

- [ ] **Step 2: Implement types and UI**

Extend `SEOBrief` and `normalizeSEOBrief`. Render GEO blockers/top opportunities in the existing SEO operating brief section when present.

- [ ] **Step 3: Verify frontend**

Run:
- `cd web && npm test -- --runInBand`
- `cd web && npm run typecheck`

Expected: PASS.

### Task 5: Full Round 5 Acceptance

- [ ] Run `go test -count=1 ./internal/geo ./internal/api ./internal/seo`
- [ ] Run `make test`
- [ ] Run `cd web && npm test -- --runInBand`
- [ ] Run `cd web && npm run typecheck`
- [ ] Continue only after all checks pass.
