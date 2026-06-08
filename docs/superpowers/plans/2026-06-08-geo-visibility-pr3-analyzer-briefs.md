# GEO Visibility PR3 Analyzer and Asset Briefs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert PR2 observations into idempotent GEO opportunities, create citation-ready asset briefs, and allow accepted briefs to become existing CiteLoop topics.

**Architecture:** Keep `seo_opportunities` as the single queue. Add only `geo_asset_briefs` for GEO-specific brief metadata. Analyzer reads stored `geo_observations`, writes/upserts open GEO opportunities, and creates one brief per opportunity; accepted briefs create a `topics` row so Writer + QA + Review gate stay unchanged.

**Tech Stack:** Go, sqlc, Chi, PostgreSQL JSONB, Next.js App Router, TypeScript.

---

## Acceptance Gates

- `make sqlc`
- `go test -count=1 ./internal/geo ./internal/db ./internal/api ./internal/seo`
- `make test`
- `cd web && npm test -- --runInBand`
- `cd web && npm run typecheck`

## Files

- Create: `internal/migrations/0013_geo_visibility_pr3.sql`
- Modify: `internal/db/queries/geo.sql`
- Modify generated sqlc files.
- Modify: `internal/geo/pr2.go` or create `internal/geo/pr3.go`
- Test: `internal/geo/service_pr3_test.go`
- Modify: `internal/api/server.go`, `internal/api/handlers_geo_pr2.go`
- Modify: `internal/api/seo_routes_test.go`
- Modify: `web/app/lib/api.ts`, `web/app/lib/api.test.mjs`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

## Task 1: Schema and Queries

- [ ] Add `geo_asset_briefs` with `opportunity_id`, `asset_type`, `status`, `target_prompts`, `required_evidence`, `recommended_outline`, `internal_link_plan`, `publication_surface`, and `created_by_run_id`.
- [ ] Add `UpsertGEOObservationOpportunity` that dedupes open/accepted/converted `seo_opportunities` by `project_id + type + query + normalized_page_url`.
- [ ] Add asset brief CRUD queries.
- [ ] Run `make sqlc && go test ./internal/db`.

## Task 2: Analyzer Service

- [ ] Write failing `TestAnalyzeObservationsCreatesIdempotentGEOOpportunitiesAndBriefs`.
- [ ] Implement `AnalyzeObservations(ctx, projectID, AnalyzeObservationsRequest)` that detects competitor-cited/project-absent and project-mentioned-without-citation gaps.
- [ ] Generate asset briefs with source-backed required evidence and outline metadata.
- [ ] Rerun analyzer in the test and assert no duplicate open opportunities/briefs.

## Task 3: Brief Acceptance

- [ ] Write failing `TestAcceptAssetBriefCreatesTopic`.
- [ ] Implement `AcceptAssetBrief(ctx, projectID, briefID)` to mark brief accepted and create a backlog topic with target prompt/asset type metadata.
- [ ] Run focused geo tests.

## Task 4: API and UI

- [ ] Add routes: `POST /geo/opportunities/analyze`, `GET /geo/asset-briefs`, `POST /geo/asset-briefs/{briefID}/accept`.
- [ ] Add web API contract tests and methods.
- [ ] Show top GEO opportunities and asset briefs in SEO page, with an accept button for briefs.
- [ ] Run web tests and typecheck.

## Task 5: Round 3 Verification

- [ ] Run all acceptance gates listed above.

## Self-Review

- PR3 covers analyzer-generated opportunities and citation-ready asset briefs.
- Accepted briefs enter the existing topic/content pipeline instead of bypassing review.
- Provider integrations remain excluded for PR4.
