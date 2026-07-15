# Competitive Auto Seed Recall Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a lightweight automatic competitive seed recall path so AI Discovery can find high-signal competitor pages such as `https://postsyncer.com/tools` from search evidence, even when the user did not manually enter `seed_urls`.

**Architecture:** Reuse the existing `growthradar.SearchCollector` and `crawl.SeedURLEnrichment` pipeline. Generate a tiny deterministic set of competitive recall queries from active prompt text and evidence-backed public terms, collect search results under the existing search budget, filter for high-signal competitive URL paths, then pass those URLs through the same seed enrichment/materialization path used by manual seed URLs.

**Tech Stack:** Go, existing `internal/opportunityfinding`, `internal/growthradar`, `internal/search`, and `internal/crawl`.

---

### Task 1: Add failing tests for automatic competitive seed recall

**Files:**
- Modify: `internal/opportunityfinding/ai_discovery_test.go`

- [ ] **Step 1: Write the failing test**

Add a test that creates an active prompt like `best social publishing tools`, configures a fake search collector result containing `https://postsyncer.com/tools`, and calls `RefreshAIDiscoveryEvidence` without `SeedURLs`. Assert that `fakeAIDiscoveryService.seedRequests` contains `https://postsyncer.com/tools`, and that `CompetitiveSeedReports` carries the enriched tools hub.

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/opportunityfinding -run TestAIDiscoveryAutoRecallsCompetitiveSeedURLsFromSearchEvidence -count=1
```

Expected: fail because search results are currently counted as evidence only; they are not converted into competitive seed URLs for enrichment.

### Task 2: Implement deterministic competitive seed recall

**Files:**
- Modify: `internal/opportunityfinding/ai_discovery.go`

- [ ] **Step 1: Add small helper functions**

Add helpers near the existing search/seed code:

```go
func competitiveRecallQueries(prompts []db.GeoPrompt, evidence growthradar.EvidenceIndex) []string
func competitiveSeedURLsFromSearch(set growthradar.EvidenceSet) []string
func isCompetitiveSeedCandidateURL(raw string) bool
func mergeSeedURLs(manual []string, auto []string) []string
```

Behavior:

- Use at most three queries.
- Prefer `DiscoveryEvidence.PublicTerms`, then active prompt text.
- Normalize whitespace and dedupe queries.
- Filter result URLs to `http`/`https` URLs whose path contains `/tools`, `/alternatives`, `/compare`, `/comparison`, or `/scheduler`.
- Dedupe seed URLs and cap auto seeds to five per run.

- [ ] **Step 2: Wire recall into `RefreshAIDiscoveryEvidence`**

Inside the existing `SearchCollector` worker, collect existing selected-prompt evidence as before, then collect competitive recall queries with trigger `daily`. For usable result sets, append high-signal URLs to an `autoSeedURLs` slice. After workers complete, merge `opts.SeedURLs` and `autoSeedURLs`, run `enrichCompetitiveSeedURLs` when the merged list is non-empty, and keep the existing competitive seed counters/funnel steps.

- [ ] **Step 3: Preserve existing behavior**

Manual `SeedURLs` must still work when no `SearchCollector` is configured. Search provider failures must not block crawler audit, answer observation, external surfaces, or manual seed enrichment; they should only mark `search_evidence` degraded as today.

### Task 3: Verify full local behavior

**Files:**
- No new files.

- [ ] **Step 1: Run targeted tests**

```bash
go test ./internal/opportunityfinding -count=1
```

Expected: pass.

- [ ] **Step 2: Run relevant broader tests**

```bash
go test ./internal/geo ./internal/crawl ./internal/growthradar ./internal/opportunityfinding -count=1
```

Expected: pass.

- [ ] **Step 3: Run repository hygiene check**

```bash
git diff --check
```

Expected: no output and exit code 0.

### Task 4: Publish, merge, deploy, and verify production

**Files:**
- No new files.

- [ ] **Step 1: Commit the implementation**

```bash
git add docs/superpowers/plans/2026-07-15-competitive-auto-seed-recall.md internal/opportunityfinding/ai_discovery.go internal/opportunityfinding/ai_discovery_test.go
git commit -m "Add automatic competitive seed recall"
```

- [ ] **Step 2: Push and open a draft PR**

```bash
git push -u origin codex/competitive-auto-seed-recall
gh pr create --draft --base main --head codex/competitive-auto-seed-recall --title "Add automatic competitive seed recall"
```

- [ ] **Step 3: Wait for CI, merge, and verify production**

Wait for Go/Web/Vercel checks, mark ready, merge to `origin/main`, wait for the Vercel production deployment for the merge commit, then smoke:

```bash
curl -sS -o /tmp/citeloop-home.html -w '%{http_code}\n' https://citeloop.app/
curl -sS -w '\n%{http_code}\n' https://api.citeloop.app/healthz
```

Expected: app returns `200`; API returns `ok` and `200`.
