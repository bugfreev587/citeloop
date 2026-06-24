# Analysis Workflow Phase 5B GSC Opportunity Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for behavior changes and superpowers:executing-plans for task tracking.

**Goal:** Turn first-party Google Search Console query and page performance data into compact Analysis recommendations that users can approve or dismiss without reading raw metric tables.

**Scope:** Generate the first set of GSC-backed opportunity cards from already-synced query/page rows. Keep evidence collapsed in the Analysis UI. Do not add a separate analytics dashboard.

---

## File Structure

- Create: `internal/seo/search_opportunities.go`
  - Pure candidate generation for low CTR, striking-distance queries, and content decay.
- Create: `internal/seo/search_opportunities_test.go`
  - TDD coverage for the candidate classifier and evidence payloads.
- Modify: `internal/db/queries/seo.sql`, generated db files
  - Add query/page rollup queries.
  - Make `UpsertSEOOpportunity` write stable `opportunity_key` values and dedupe on active opportunities.
- Modify: `internal/db/multi_surface_schema_contract_test.go`
  - Contract-test that the query layer writes and dedupes by opportunity key.
- Modify: `internal/seo/service.go`
  - Run metric opportunity generation during Analysis after canonical checks and before cold-start fallback.

## Task 1: Add Candidate RED Test

- [x] Write a failing test for three user-facing GSC opportunity types:
  - `gsc_low_ctr_query`
  - `gsc_striking_distance_query`
  - `gsc_content_decay`
- [x] Verify RED because helper types/functions are missing.

## Task 2: Implement Pure Candidate Generation

- [x] Add query/page rollup input structs.
- [x] Add candidate output struct with action, impact, effort, risk, score, confidence, and collapsed evidence.
- [x] Keep evidence compact: source, reason, 28-day clicks/impressions/CTR/position, or click-drop ratio.
- [x] Verify GREEN for the candidate test.

## Task 3: Add DB Rollups and Stable Opportunity Upsert

- [x] Add `ListSearchQueryOpportunityRollups`.
- [x] Add `ListPageDecayOpportunityRollups`.
- [x] Update `UpsertSEOOpportunity` to write `opportunity_key` and dedupe active opportunities by that key.
- [x] Preserve accepted/converted status on repeated analyzer runs.
- [x] Regenerate sqlc.
- [x] Verify DB contract test.

## Task 4: Wire Analyzer

- [x] Run GSC metric opportunity generation during `Analyze`.
- [x] Add `gsc_metric_opportunities:N` to run notes when generated.
- [x] Only fall back to cold-start opportunities when no canonical or metric opportunities were generated.
- [x] Verify targeted SEO tests.

## Task 5: Verify

- [x] Run full Go test suite through `make test`.
- [x] Run full web tests.
- [x] Run frontend typecheck.
- [x] Run preview production build.
- [x] Run whitespace diff check.
