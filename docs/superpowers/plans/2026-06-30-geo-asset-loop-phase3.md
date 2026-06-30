# GEO Asset Loop Phase 3 Execution Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development and superpowers:verification-before-completion. Track each checkbox as it is completed.

**Goal:** Complete PRD Phase 3 so GEO asset briefs enter the normal CiteLoop content loop as asset-specific drafts, not generic articles.

**Scope:** Accepting a GEO asset brief creates a topic that preserves asset type and source evidence, immediately starts draft generation when runtime LLM credentials are available, gives Writer an asset-specific contract, and shows asset metadata in Review.

**Tech Stack:** Go, Chi, sqlc models, PostgreSQL JSONB, Next.js App Router, TypeScript.

---

## Acceptance Gates

- [x] `go test ./internal/geo ./internal/agents ./internal/api ./internal/scheduler -count=1`
- [x] `go test ./...`
- [x] `cd web && node --test app/lib/api.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs app/lib/content-plan-logic.test.mjs`
- [x] `cd web && npm test`
- [x] `cd web && npm run typecheck`
- [ ] Production deployment finishes.
- [ ] Production Admin/GEO flow verifies OpenAI or Anthropic TokenGate provider can execute Phase 2/3 path without Perplexity.

## Files

- Modify: `internal/geo/pr3.go`
- Test: `internal/geo/service_pr3_test.go`
- Modify: `internal/agents/writer.go`
- Test: `internal/agents/writer_test.go`
- Modify: `internal/api/handlers_geo_pr2.go`
- Modify: `web/app/lib/normalize.ts`
- Modify: `web/app/projects/[id]/review/review-client.tsx`
- Test: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
- Optional: `web/app/lib/api.ts`, `web/app/lib/api.test.mjs`

## Task 1: Failing Tests

- [x] Add GEO service coverage proving accepted briefs preserve asset type and evidence metadata on the topic.
- [x] Add writer coverage proving `comparison_page` topics receive the comparison contract and SEO meta carries asset metadata.
- [x] Add Review UI contract coverage for asset type and source evidence display.

## Task 2: Brief Metadata and Accept Flow

- [x] Encode brief metadata in topic JSON without adding schema.
- [x] Keep `topic.angle` as the stable asset type and `topic.format` as `geo_asset_brief`.
- [x] Have the API start background generation after accept when pool and LLM are available.
- [x] Keep no-LLM/test environments able to accept and convert briefs into backlog topics.

## Task 3: Asset-Specific Writer Contract

- [x] Parse GEO asset metadata from topic JSON.
- [x] Add asset-specific instructions for comparison, alternative, glossary, template/checklist, benchmark/report, integration/docs, source-backed evidence, and FAQ blocks.
- [x] Include source evidence and recommended outline in both JSON and Markdown fallback prompts.
- [x] Add `asset_type` and `source_evidence` to generated SEO meta for Review.

## Task 4: Review Surface

- [x] Normalize article `seo_meta` unchanged as the metadata carrier.
- [x] Show asset type in queue rows and inspector.
- [x] Show source evidence near QA evidence so reviewers can see why the asset exists.

## Task 5: Verification, PR, Production

- [ ] Run all local verification gates.
- [ ] Push `codex/seo-geo-phase3`.
- [ ] Open PR to `origin/main`, merge it, and wait for deploy.
- [ ] Verify production and fix any gaps before final report.
