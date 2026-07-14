# Site Fix Results Read Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add actual-only Site Fix measurements to the existing Results API without breaking the bare-array Content Action contract.

**Architecture:** A single PostgreSQL `UNION ALL` feed query owns cross-source ordering, status filtering, and keyset pagination. Go decodes each discriminated row into either the unchanged Content Action JSON shape plus `source_type`, or a dedicated Site Fix summary; a separate project-scoped endpoint exposes an explicit redacted Site Fix measurement detail DTO. Canonical Site Fix detail receives a summary derived only from persisted measurement and handoff rows.

**Tech Stack:** Go 1.24, chi, pgx/sqlc, PostgreSQL, `testing`/`httptest`.

---

### Task 1: Unified Results feed contract

**Files:**
- Modify: `internal/db/queries/site_fix_measurements.sql`
- Modify: `internal/db/site_fix_measurements_contract_test.go`
- Modify: `internal/db/site_fix_measurements_postgres_integration_test.go`

- [ ] Write failing SQL contract and PostgreSQL tests proving actual-only rows, project/status isolation, two generations, Content Action compatibility, and deterministic `(activity_at DESC, source_type ASC, id DESC)` boundaries.
- [ ] Run focused tests and confirm missing unified query/cursor behavior fails.
- [ ] Add `ListResultsFeedRows` as one `UNION ALL` query with opaque keyset fields, legacy time cursor support, and `limit + 1`.
- [ ] Generate sqlc and rerun focused tests.

### Task 2: Public Results list DTO and cursors

**Files:**
- Modify: `internal/api/handlers_seo.go`
- Modify: `internal/api/results_routes_test.go`
- Add: `internal/api/results_site_fixes_test.go`

- [ ] Write failing handler tests for a bare heterogeneous array, unchanged legacy Content Action fields, `source_type`, Site Fix summaries, opaque next cursor, legacy timestamp cursor, and status filtering.
- [ ] Implement a discriminated `ResultsFeedItem` conversion that never embeds a zero-value `db.ContentAction` for Site Fix rows.
- [ ] Encode/decode the opaque cursor and expose the next cursor in `X-Next-Cursor` while keeping the body a bare array.
- [ ] Run focused handler tests.

### Task 3: Project-scoped redacted Site Fix measurement detail

**Files:**
- Modify: `internal/api/server.go`
- Modify: `internal/api/handlers_seo.go`
- Modify: `internal/db/queries/site_fix_measurements.sql`
- Modify: `internal/api/results_routes_test.go`
- Modify: `internal/api/results_site_fixes_test.go`

- [ ] Write failing route/DTO tests for project isolation, ordered checkpoints, terminal outcome, and absence of target identity, classifier, baseline, outbox, and provider-evidence keys.
- [ ] Add project-scoped read queries for measurement/fix, checkpoints, terminal outcome, and latest handoff state.
- [ ] Implement `GET /results/site-fixes/{measurementID}` with explicit public structs only.
- [ ] Run focused tests.

### Task 4: Canonical Site Fix summary and handoff state

**Files:**
- Modify: `internal/api/handlers_site_fixes.go`
- Modify: `internal/api/site_fixes_api_test.go`
- Modify: `internal/db/queries/site_fix_measurements.sql`

- [ ] Write failing tests for `not_applicable`, `not_started`, `pending`, `started`, and `failed`, including a verified Site Fix whose lifecycle remains `verified`.
- [ ] Add `measurement_summary` and `measurement_handoff_status` fields populated from the latest real measurement/outbox rows only.
- [ ] Use the canonical deep link `/projects/{projectID}/results?source_type=site_fix&measurement={measurementID}`.
- [ ] Run focused tests.

### Task 5: Verification and commit

**Files:**
- Verify all Task 5 files above.

- [ ] Run `make sqlc`, `git diff --check`, focused API/DB tests, fresh PostgreSQL integration tests, `go test ./...`, and `go vet ./...`.
- [ ] Commit the implementation with a Task 5-specific message and report the SHA.
