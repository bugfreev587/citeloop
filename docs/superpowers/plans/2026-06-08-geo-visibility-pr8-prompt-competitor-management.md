# GEO Visibility PR8 Prompt + Competitor Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Satisfy PRD §7.2 / acceptance 3 by letting operators edit/pause/activate GEO prompts and competitors.

**Architecture:** Reuse existing `geo_prompts` and `geo_competitors` update queries. Add single-row getters for safe project-scoped updates, HTTP endpoints under `/geo`, frontend API helpers, and compact pause/activate controls in the existing SEO/GEO page.

**Tech Stack:** sqlc queries, Go API route handlers, Next.js API helper tests, existing SEO page UI.

---

## Files

- Modify: `internal/db/queries/geo.sql` to add `GetGEOPromptForProject` and `GetGEOCompetitorForProject`.
- Regenerate: `internal/db/geo.sql.go`, `internal/db/querier.go`.
- Modify: `internal/api/handlers_geo_pr2.go` and `internal/api/server.go` for update handlers/routes.
- Modify: `internal/api/seo_routes_test.go` for route registration.
- Modify: `web/app/lib/api.ts` and `web/app/lib/api.test.mjs` for client contracts.
- Modify: `web/app/projects/[id]/seo/seo-client.tsx` to add prompt/competitor pause/activate controls.

## Acceptance

- `make sqlc` passes.
- `go test -count=1 ./internal/db ./internal/api` passes.
- `make test` passes.
- `cd web && npm test -- --runInBand` passes.
- `cd web && npm run typecheck` passes.
- Operators can update prompt fields/status through API and UI.
- Operators can update competitor fields/status through API and UI.

---

### Task 1: Backend Red Tests

- [ ] Add route tests for:
  - `PUT /api/projects/{projectID}/geo/prompts/{promptID}`
  - `PUT /api/projects/{projectID}/geo/competitors/{competitorID}`

- [ ] Run `go test -count=1 ./internal/api`.

Expected: FAIL because routes are missing.

### Task 2: SQL + Handlers

- [ ] Add sqlc getters:

```sql
-- name: GetGEOPromptForProject :one
select * from geo_prompts where id = $1 and project_id = $2;

-- name: GetGEOCompetitorForProject :one
select * from geo_competitors where id = $1 and project_id = $2;
```

- [ ] Run `make sqlc`.

- [ ] Implement handlers:
  - `updateGEOPrompt`
  - `updateGEOCompetitor`

Both handlers load the existing row, merge partial JSON input, preserve unspecified fields, and call the existing update query.

### Task 3: Frontend Contract + UI

- [ ] Add failing client test for `updateGEOPrompt` and `updateGEOCompetitor`.
- [ ] Implement helpers in `web/app/lib/api.ts`.
- [ ] Add pause/activate buttons for the first visible prompts and active/paused competitors in the GEO section.

### Task 4: Full Acceptance

- [ ] Run `make sqlc`
- [ ] Run `go test -count=1 ./internal/db ./internal/api`
- [ ] Run `make test`
- [ ] Run `cd web && npm test -- --runInBand`
- [ ] Run `cd web && npm run typecheck`
