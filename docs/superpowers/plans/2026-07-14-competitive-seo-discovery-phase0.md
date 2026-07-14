# Competitive SEO Discovery Phase 0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the Phase 0 foundations from `docs/PRD-CiteLoop-Competitive-SEO-Discovery.md`: evidence vocabulary flow, competitor domain write-path, and citation URL competitor attribution.

**Architecture:** Keep the first slice inside existing Growth Radar and GEO boundaries. `opportunityfinding` gets an explicit `growthradar.EvidenceIndex` handoff; `geo` gets deterministic domain extraction and URL-to-known-competitor attribution; SQL upsert preserves existing competitor domains when profile refreshes have no domains.

**Tech Stack:** Go, sqlc-generated query files, PostgreSQL JSONB, existing `internal/growthradar`, `internal/opportunityfinding`, and `internal/geo` packages.

---

### Task 1: Planner Evidence Vocabulary Handoff

**Files:**
- Modify: `internal/opportunityfinding/ai_planner.go`
- Modify: `internal/opportunityfinding/ai_discovery.go`
- Test: `internal/opportunityfinding/ai_discovery_test.go`
- Test: `internal/opportunityfinding/ai_planner_test.go`

- [x] **Step 1: Write the failing handoff test**

Add a test in `internal/opportunityfinding/ai_discovery_test.go` that constructs `AIDiscoveryOptions{DiscoveryEvidence: growthradar.EvidenceIndex{PublicTerms: []string{"free social content tools"}}}` and a fake planner. Assert the planner receives that evidence in `ManualDiscoveryPlanRequest`.

- [x] **Step 2: Run the failing handoff test**

Run: `go test ./internal/opportunityfinding -run TestManualAIDiscoveryPassesDiscoveryEvidenceToPlanner -count=1`

Expected: compile failure or assertion failure because the request/options do not expose discovery evidence yet.

- [x] **Step 3: Write the failing planner acceptance test**

Add a test in `internal/opportunityfinding/ai_planner_test.go` proving `AIManualDiscoveryPlanner` accepts a candidate whose `target_topic` maps only to `EvidenceIndex.PublicTerms`, not to profile features.

- [x] **Step 4: Run the failing planner acceptance test**

Run: `go test ./internal/opportunityfinding -run TestAIManualDiscoveryPlannerAllowsEvidenceBackedPublicTopic -count=1`

Expected: failure because `AIManualDiscoveryPlanner` currently calls `ClassifyContext` with an empty `EvidenceIndex`.

- [x] **Step 5: Implement minimal evidence plumbing**

Add `Evidence growthradar.EvidenceIndex` to `ManualDiscoveryPlanRequest`, `DiscoveryEvidence growthradar.EvidenceIndex` to `AIDiscoveryOptions`, and `Evidence growthradar.EvidenceIndex` to `AIManualDiscoveryPlanner`. Merge planner-level evidence with request-level evidence before calling `ClassifyContext`.

- [x] **Step 6: Verify task 1**

Run: `go test ./internal/opportunityfinding -run 'TestManualAIDiscoveryPassesDiscoveryEvidenceToPlanner|TestAIManualDiscoveryPlannerAllowsEvidenceBackedPublicTopic' -count=1`

Expected: PASS.

### Task 2: Competitor Domain Write-Path

**Files:**
- Modify: `internal/geo/pr2.go`
- Modify: `internal/db/queries/geo.sql`
- Modify: `internal/db/geo.sql.go`
- Test: `internal/geo/service_pr2_test.go`

- [x] **Step 1: Write the failing profile-domain extraction test**

Add a test in `internal/geo/service_pr2_test.go` where the active profile has a competitor value such as `PostSyncer https://postsyncer.com`; run prompt-set generation and assert the upserted competitor domains JSON includes `postsyncer.com`.

- [x] **Step 2: Run the failing extraction test**

Run: `go test ./internal/geo -run TestGeneratePromptSetWritesCompetitorDomainsFromProfileURLs -count=1`

Expected: FAIL because `pr2.go` currently writes `Domains: jsonBytes([]string{})`.

- [x] **Step 3: Write the failing no-erase SQL test**

Use an existing DB query contract test or add a focused query text assertion proving `UpsertGEOCompetitor` does not overwrite existing non-empty domains with an empty excluded domains array.

- [x] **Step 4: Run the failing SQL test**

Run: `go test ./internal/db -run TestGEOCompetitorUpsertPreservesExistingDomainsOnEmptyInput -count=1`

Expected: FAIL because the SQL currently sets `domains = excluded.domains`.

- [x] **Step 5: Implement minimal domain write-path**

Add deterministic URL/domain extraction for competitor strings in `internal/geo/pr2.go`, write extracted hosts into `UpsertGEOCompetitorParams.Domains`, and adjust SQL/generated Go query text to preserve existing domains when excluded domains is empty.

- [x] **Step 6: Verify task 2**

Run: `go test ./internal/geo -run TestGeneratePromptSetWritesCompetitorDomainsFromProfileURLs -count=1`

Run: `go test ./internal/db -run TestGEOCompetitorUpsertPreservesExistingDomainsOnEmptyInput -count=1`

Expected: both PASS.

### Task 3: Citation URL Known Competitor Attribution

**Files:**
- Modify: `internal/geo/provider.go`
- Test: `internal/geo/service_pr4_test.go`

- [x] **Step 1: Write the failing attribution test**

Add a test where active competitors include a known domain, provider output has a matching cited URL, and `CompetitorCitations` is empty. Assert the persisted observation has `competitor_citations` containing the cited competitor URL.

- [x] **Step 2: Run the failing attribution test**

Run: `go test ./internal/geo -run TestObserveAnswerProviderClassifiesKnownCompetitorCitationsFromURLs -count=1`

Expected: FAIL because `createProviderObservation` persists provider `CompetitorCitations` unchanged.

- [x] **Step 3: Implement deterministic URL attribution**

Fetch active competitors before creating observations, normalize known competitor domains, match cited URL hosts, and merge matched cited URLs into `CompetitorCitations` without duplicating provider-supplied citations.

- [x] **Step 4: Verify task 3**

Run: `go test ./internal/geo -run TestObserveAnswerProviderClassifiesKnownCompetitorCitationsFromURLs -count=1`

Expected: PASS.

### Task 4: Focused Regression Pass

**Files:**
- Verify only

- [x] **Step 1: Run focused package tests**

Run: `go test ./internal/opportunityfinding ./internal/geo ./internal/db -count=1`

Expected: PASS.

- [x] **Step 2: Check diff hygiene**

Run: `git diff --check && git diff --cached --check`

Expected: no output and exit 0.
