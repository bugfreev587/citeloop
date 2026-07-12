# Doctor / Opportunities Phase 5 Legacy Cleanup Implementation Plan

**Goal:** Remove the retired user-facing discovery-mode contract, close rollback-only migration review noise, and prove the canonical Doctor and Opportunities writers conserve production work with reversible migration evidence.

**Architecture:** Keep Signal Scan and AI Discovery as internal Opportunity searching stages, but make the versioned capability fields the only runtime and public settings authority. Growth rollback batches retain immutable audit rows while their review items close immediately as rollback history. Existing canonical aliases and legacy rows remain readable provenance; no legacy writer is re-enabled.

**Tech Stack:** Go, PostgreSQL/sqlc, Next.js/TypeScript, Node contract tests, Railway, Vercel, Chrome production verification.

---

## Task 1: Retire legacy discovery-mode settings

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/api/handlers_seo.go`
- Modify: `internal/api/opportunity_finding_status_contract_test.go`
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/api.test.mjs`
- Create: `internal/migrations/0080_retire_legacy_discovery_settings.sql`
- Create: `internal/db/legacy_discovery_settings_cleanup_contract_test.go`

1. Add failing contracts proving project config and Opportunity status no longer expose `opportunity_finding_source_mix`, `ai_discovery_automation`, or `source_mix`.
2. Add a migration that requires `capability_policy_version >= 1` and complete Growth capability fields, then removes the two retired JSON keys from every project config.
3. Remove the legacy Go config fields, constants, normalization fallback, status response fields, and source-mode summary branching. `growth_signal_enabled`, `growth_ai_enabled`, and `growth_ai_run_policy` become the only runtime controls.
4. Remove the retired TypeScript config/status fields and legacy client normalization. Preserve internal stage progress labels and evidence summaries.
5. Run Go, Web, SQL generation, formatting, and build checks.

## Task 2: Close rollback-only migration reviews

**Files:**
- Modify: `internal/growthwork/service.go`
- Modify: `internal/growthwork/service_test.go`
- Modify: `internal/db/queries/site_fixes.sql`
- Regenerate: `internal/db/site_fixes.sql.go`, `internal/db/querier.go`
- Create: `internal/migrations/0081_close_rolled_back_migration_reviews.sql`
- Create: `internal/db/rolled_back_migration_review_cleanup_contract_test.go`

1. Add a failing Growth cutover test proving a rolled-back batch retains its immutable review record but the review item is immediately `dismissed` with reason `migration_rolled_back`.
2. Add a project-scoped dismissal query and invoke it inside the same rollback audit transaction after the review item is created.
3. Backfill existing pending review items whose immutable batch status is `rolled_back`; do not touch forward/review-required items.
4. Prove conservation constraints, rollback ledger identity, and writer authority behavior remain unchanged.

## Task 3: Production migration and provenance cleanup gate

1. Merge the Phase 5 PR after Go/Web/Vercel checks, then wait for Railway and Vercel production deployment of the merge SHA.
2. Verify production project configs contain only versioned Doctor/Opportunities capability fields and that Opportunity Finding still runs both authorized internal stages.
3. Confirm rolled-back batches have zero pending review items.
4. Resolve the remaining Doctor review item only after proving its source action is canonical Opportunities-owned Growth work; record the operator resolution without changing or duplicating the action.
5. Run a read-only conservation report: canonical writer authority for both products, one-of-source application constraint validated, active signature uniqueness, alias provenance, legacy technical generator count, scheduler authority, and row counts.
6. Browser-verify Settings, Doctor, Opportunities, Home, Site Fixes, and Results with no retired source-mode copy and no console errors.

## Task 4: Deferred failure register

Record, without blocking unrelated cleanup, the Phase 4 native Site Fix generator failure for `d327f8c5-74ea-4215-a0b0-a2002a69c489`: three audited `fix_generation` calls returned `invalid_output`; no PR or publication occurred. Carry this item into the AC1–47 final audit for concentrated remediation.

## Completion gate

Phase 5 is complete only when production has zero retired config keys, zero pending review items from rolled-back batches, both writer authorities remain canonical and unfenced, constraints are validated, UI contains no legacy product-mode controls, and the production conservation report has no unexplained duplicate active work.
