# GEO Visibility PR2 Prompt Fixtures Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build PR2 from `docs/PRD-CiteLoop-GEO-Visibility-Layer.md`: deterministic GEO prompt sets, editable competitor/project-owned surface mapping, manual fixture observations, and a score v1 with coverage/confidence history.

**Architecture:** Extend the existing PR1 `internal/geo` package and `geo_runs` table instead of creating a separate task system. Prompt generation is deterministic from active product profile fields and topics; manual fixture observations are the first truth-bearing observation provider. Scoring writes `geo_visibility_scores` as a time series and marks low sample coverage as low confidence instead of pretending full answer-engine coverage.

**Tech Stack:** Go, Chi, pgx/sqlc, PostgreSQL JSONB, Next.js App Router, TypeScript, Node test runner.

---

## Acceptance Gates

- `make sqlc`
- `go test -count=1 ./internal/geo ./internal/db ./internal/api ./internal/seo`
- `make test`
- `cd web && npm test -- --runInBand`
- `cd web && npm run typecheck`

## Files

- Create: `internal/migrations/0012_geo_visibility_pr2.sql`
- Modify: `internal/db/queries/geo.sql`
- Generated: `internal/db/geo.sql.go`, `internal/db/models.go`, `internal/db/querier.go`
- Modify: `internal/geo/service.go`
- Test: `internal/geo/service_pr2_test.go`
- Create: `internal/api/handlers_geo_pr2.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/seo_routes_test.go`
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/api.test.mjs`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

## Task 1: PR2 Schema and Queries

- [ ] **Step 1: Add failing schema/query compile test**

Run before writing migration:

```bash
make sqlc
go test ./internal/db
```

Expected at this point: current PR1 schema compiles; PR2 query symbols do not exist until the migration and queries are added.

- [ ] **Step 2: Add `0012_geo_visibility_pr2.sql`**

Create tables:

- `geo_prompt_sets`
- `geo_competitors`
- `geo_prompts`
- `geo_observations`
- `geo_visibility_scores`
- `geo_external_surfaces`

Also replace the `geo_runs_agent_check` constraint so `geo_runs.agent` supports `geo_prompt_builder`, `geo_observer`, and `geo_analyzer`.

- [ ] **Step 3: Extend `internal/db/queries/geo.sql`**

Add sqlc queries for prompt-set CRUD, prompt CRUD, competitor CRUD, external-surface CRUD, observation insert/list, and score insert/latest.

- [ ] **Step 4: Generate sqlc and verify**

Run:

```bash
make sqlc
go test ./internal/db
```

Expected: both commands exit 0.

## Task 2: Prompt Builder and Mapping Service

- [ ] **Step 1: Write failing tests in `internal/geo/service_pr2_test.go`**

Tests must cover:

- `GeneratePromptSet` creates at least 30 prompts from active profile fields.
- Competitors from `profile.competitors` are upserted as active competitors.
- The project site URL is inserted as a project-owned external surface.

- [ ] **Step 2: Run red test**

Run:

```bash
go test ./internal/geo -run 'TestGeneratePromptSet'
```

Expected: FAIL because PR2 service methods and store methods do not exist.

- [ ] **Step 3: Implement minimal service**

Extend `Store` with the generated PR2 query methods. Add:

- `GeneratePromptSet(ctx, projectID, GeneratePromptSetRequest)`.
- deterministic prompt templates covering category, problem-solution, comparison, alternative, workflow, integration, buyer-intent, and definition/entity.
- profile parsing for `features`, `icp`, `key_terms`, `competitors`, `positioning`.

- [ ] **Step 4: Run green test**

Run:

```bash
go test ./internal/geo -run 'TestGeneratePromptSet'
```

Expected: PASS.

## Task 3: Manual Fixture Observations and Score v1

- [ ] **Step 1: Write failing service tests**

Tests must import 10 manual fixture observations and assert:

- observations are stored with `source_type='manual_fixture'`.
- `project_citation_count` is computed from owned surfaces when cited URLs match project-owned domain/surface.
- score row is written with non-zero `coverage`, `confidence`, and JSON `breakdown`.

- [ ] **Step 2: Run red test**

Run:

```bash
go test ./internal/geo -run 'TestImportManualFixtureObservations'
```

Expected: FAIL until observation import and scoring exist.

- [ ] **Step 3: Implement fixture import and scoring**

Add:

- `ImportManualFixtureObservations(ctx, projectID, ImportManualFixtureRequest)`.
- URL ownership matching against `geo_external_surfaces.owner_type='project'`.
- score calculation that stores `geo_visibility_scores`.
- coverage/confidence rules: below 10 observed prompts is low confidence; 10 or more is medium; 30 or more with coverage >= 0.7 is high.

- [ ] **Step 4: Run green test**

Run:

```bash
go test ./internal/geo -run 'TestImportManualFixtureObservations'
```

Expected: PASS.

## Task 4: PR2 API Routes

- [ ] **Step 1: Add route registration tests**

Extend route tests with valid-project assertions for:

- `POST /api/projects/{id}/geo/prompt-sets/generate`
- `GET /api/projects/{id}/geo/prompt-sets`
- `PUT /api/projects/{id}/geo/prompt-sets/{promptSetID}`
- `POST /api/projects/{id}/geo/runs/observe`
- `GET /api/projects/{id}/geo/observations`
- `GET /api/projects/{id}/geo/overview`
- `GET /api/projects/{id}/geo/external-surfaces`
- `POST /api/projects/{id}/geo/external-surfaces`

- [ ] **Step 2: Run red route test**

Run:

```bash
go test ./internal/api -run TestGEORoutesAreRegisteredForValidProject -count=1
```

Expected: FAIL for new PR2 routes.

- [ ] **Step 3: Implement handlers**

Use `internal/geo.Service` in a new `handlers_geo_pr2.go`. Handlers should return 400 for bad UUID/body, 500 for missing DB, and JSON for successful calls.

- [ ] **Step 4: Run green route test**

Run:

```bash
go test ./internal/api -run TestGEORoutesAreRegisteredForValidProject -count=1
```

Expected: PASS.

## Task 5: Web API and SEO Page UI

- [ ] **Step 1: Add failing web API contract tests**

Add expectations for `generateGEOPromptSet`, `listGEOPromptSets`, `updateGEOPromptSet`, `observeGEOManualFixtures`, `getGEOOverview`, and external surface methods.

- [ ] **Step 2: Run red web test**

Run:

```bash
cd web && npm test -- --runInBand
```

Expected: FAIL because methods are missing.

- [ ] **Step 3: Implement web API types and methods**

Add types for prompt sets, prompts, competitors, observations, scores, and surfaces in `web/app/lib/api.ts`.

- [ ] **Step 4: Add SEO page PR2 blocks**

Add compact sections for:

- latest GEO score with coverage/confidence and `insufficient_data` display when coverage is low.
- prompt set list and generate button.
- observation table.
- external surfaces table.

- [ ] **Step 5: Run web verification**

Run:

```bash
cd web && npm test -- --runInBand
cd web && npm run typecheck
```

Expected: both exit 0.

## Task 6: Round 2 Final Verification

- [ ] **Step 1: Generated code verification**

Run:

```bash
make sqlc
```

Expected: exit 0.

- [ ] **Step 2: Focused Go tests**

Run:

```bash
go test -count=1 ./internal/geo ./internal/db ./internal/api ./internal/seo
```

Expected: PASS.

- [ ] **Step 3: Full Go tests**

Run:

```bash
make test
```

Expected: PASS.

- [ ] **Step 4: Full web tests**

Run:

```bash
cd web && npm test -- --runInBand
cd web && npm run typecheck
```

Expected: PASS.

## Self-Review

- PR2 requirements covered: prompt sets, competitors, project-owned surfaces, manual fixture observations, observation table, score v1, coverage/confidence, score time series.
- Provider integrations are excluded because PR4 owns legal answer-provider adapters.
- Analyzer-generated GEO opportunities and asset briefs are excluded because PR3 owns those.
- No private-account scraping is introduced.
