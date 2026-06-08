# GEO Visibility PR6 Score V2 + Surface Citation Freshness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring GEO scoring closer to PRD §10 and make external surface inventory reflect whether project-owned surfaces were cited in observations.

**Architecture:** Reuse existing `ai_crawler_access_snapshots`, `geo_observations`, `geo_external_surfaces`, and `geo_visibility_scores`. `scoreObservations` becomes a PRD-weighted v2 scorer, and manual/provider observation import touches `last_cited_at` for cited project-owned surfaces through the existing surface upsert path.

**Tech Stack:** Go GEO service, existing sqlc methods, unit tests with `geoStoreStub`.

---

## Files

- Modify: `internal/geo/pr2.go` to update scoring and touch cited surfaces.
- Modify: `internal/geo/provider.go` to touch cited surfaces for provider observations.
- Modify: `internal/geo/service_pr2_test.go` and `internal/geo/service_pr4_test.go` to assert freshness and v2 breakdown.

## Acceptance

- `go test -count=1 ./internal/geo` passes.
- `go test -count=1 ./internal/geo ./internal/api ./internal/seo` passes.
- `make test` passes.
- Score breakdown includes `scoring_version=geo_pr6_v2`, `crawler_access_health`, `citation_rank_score`, `external_surface_coverage`, and observed-only counts.
- `provider_unavailable`, `manual_required`, and `budget_skipped` observations are excluded from score rates when scoring is invoked.
- Any project-owned surface cited by a manual/provider observation gets `last_cited_at` updated.

---

### Task 1: Surface Citation Freshness

- [ ] **Step 1: Write failing tests**

Extend manual fixture and provider observation tests to assert the cited project-owned surface has `LastCitedAt.Valid == true` after observation import.

- [ ] **Step 2: Verify RED**

Run: `go test -count=1 ./internal/geo`

Expected: FAIL because observations currently store cited surface IDs but do not update `geo_external_surfaces.last_cited_at`.

- [ ] **Step 3: Implement freshness update**

Add a helper:

```go
func (s Service) touchCitedSurfaces(ctx context.Context, projectID uuid.UUID, surfaces []db.GeoExternalSurface, citedSurfaceIDs []string, citedAt time.Time) error
```

For each matched surface, call `UpsertGEOExternalSurface` with existing fields and `LastCitedAt: pgutil.TS(citedAt)`.

- [ ] **Step 4: Verify GREEN**

Run: `go test -count=1 ./internal/geo`

Expected: PASS.

### Task 2: Score V2

- [ ] **Step 1: Write failing test assertions**

Extend `TestImportManualFixtureObservationsComputesScore` to assert:

```go
strings.Contains(string(result.Score.Breakdown), "geo_pr6_v2")
strings.Contains(string(result.Score.Breakdown), "crawler_access_health")
strings.Contains(string(result.Score.Breakdown), "citation_rank_score")
strings.Contains(string(result.Score.Breakdown), "external_surface_coverage")
```

- [ ] **Step 2: Verify RED**

Run: `go test -count=1 ./internal/geo`

Expected: FAIL because score v1 breakdown lacks these fields.

- [ ] **Step 3: Implement PRD-weighted v2 scoring**

Observed-only calculations:
- 20: `crawler_access_health`
- 20: brand mention rate
- 25: project citation rate
- 15: citation rank/prominence
- 10: inverse competitor gap
- 10: external surface coverage

Do not count `provider_unavailable`, `manual_required`, `budget_skipped`, or `error` observations as negative observations.

- [ ] **Step 4: Verify GREEN**

Run: `go test -count=1 ./internal/geo`

Expected: PASS.

### Task 3: Full Round 6 Acceptance

- [ ] Run `go test -count=1 ./internal/geo`
- [ ] Run `go test -count=1 ./internal/geo ./internal/api ./internal/seo`
- [ ] Run `make test`
- [ ] Continue after all checks pass.
