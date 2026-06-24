# Analysis Workflow Phase 5A GSC OAuth Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Use a customer's selected Google Search Console OAuth property as the default source for first-party search metrics, while keeping Analysis understandable when data is still importing, stale, revoked, or mismatched.

**Scope:** This phase activates OAuth-backed GSC sync through the existing SEO data model and adds user-facing connection states. Deeper opportunity expansion from query/page metrics remains Phase 5B.

**Architecture:** Keep the existing `seo.Sync` path as the source of truth. At request time, the project-scoped API swaps the Google data provider to an OAuth Search Console client when an active encrypted refresh token and selected property exist. The service then writes page and query metrics through the current `page_performance_daily` and `search_performance_daily` queries.

---

## File Structure

- Create: `internal/migrations/0024_gsc_activation_states.sql`
  - Expands GSC integration statuses for OAuth activation.
- Modify: `internal/api/handlers_gsc_oauth.go`
  - Derives `backfilling`, `stale`, `mismatch`, and `revoked` connection states.
  - Builds OAuth-backed Google data providers from encrypted refresh tokens.
- Modify: `internal/api/handlers_seo.go`
  - Uses the project OAuth provider for `/seo/sync` when available.
- Modify: `internal/seo/service.go`, `internal/seo/service_test.go`
  - Allows recoverable activation states to attempt sync.
- Modify: `web/app/lib/api.ts`
  - Adds frontend status types.
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
  - Shows action-first Analysis states for backfill, stale data, and mismatch.
- Modify: `web/app/projects/[id]/settings/settings-client.tsx`
  - Shows the same statuses in Settings.
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
  - Contract-tests that these states remain visible to users.
- Create: `internal/api/gsc_connection_state_test.go`
  - Unit-tests the status derivation rules.

## Task 1: Add Status Regression Tests

- [x] Add backend unit tests for property selection required, backfilling, connected, stale, mismatch, and revoked states.
- [x] Add frontend contract checks for backfill, stale, and mismatch copy.
- [x] Verify RED for missing backend derivation and UI copy.

## Task 2: Activate OAuth-backed Sync

- [x] Add migration support for `backfilling`, `stale`, and `mismatch` integration statuses.
- [x] Add `deriveGSCConnectionStatus` and project-level derivation from token, property, day count, and last verification.
- [x] When selecting a property, mark the connection `backfilling` until GSC rows exist.
- [x] Build a project-specific OAuth Search Console provider from encrypted refresh tokens.
- [x] Route `/seo/sync` through the project-specific provider.
- [x] Keep service-account fallback behavior when OAuth is not configured or no selected token exists.

## Task 3: Surface User-facing States

- [x] Analysis shows `Backfilling Search Console` while CiteLoop imports first-party rows.
- [x] Analysis shows `Search data is stale` when the last successful verification is older than the freshness window.
- [x] Analysis shows `Property mismatch` when the selected GSC property no longer matches the project property.
- [x] Settings exposes the same states without raw token or credential fields.

## Task 4: Verify

- [x] Run targeted backend status tests.
- [x] Run targeted SEO provider status tests.
- [x] Run targeted frontend contract tests.
- [x] Run full web tests.
- [x] Run frontend typecheck.
- [x] Run full Go test suite through `make test`.
- [x] Run preview production build.
- [x] Run whitespace diff check.
