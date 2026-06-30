# SEO Analyzer Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the first executable PRD slice: GSC-driven SEO opportunities with query-gap coverage, explainable scoring metadata, and the SEO/GEO automation roadmap PRD in the repo.

**Architecture:** Reuse the existing `internal/seo/search_opportunities.go` pure candidate builder and `internal/seo.Service.Analyze` flow. Avoid schema changes by storing Phase 1 scoring metadata, `why_now`, and `expected_impact_range` in opportunity `evidence` JSON. Keep the existing SQL rollups and content action measurement contract.

**Tech Stack:** Go, sqlc-generated DB layer, Postgres JSON evidence, existing SEO service tests.

---

### Task 1: Add the PRD Document

**Files:**
- Create: `docs/PRD-CiteLoop-SEO-GEO-Automation-Upgrade.md`

- [ ] **Step 1: Add the reviewed PRD**

Copy the reviewed roadmap PRD into `docs/PRD-CiteLoop-SEO-GEO-Automation-Upgrade.md`. It must include the post-review scope decisions: P0 measurement contract only, automatic GEO provider launch gate, Results as independent page, GitHub/Next.js publisher first, and priority-score-only learning loop.

- [ ] **Step 2: Verify doc hygiene**

Run: `grep -nE 'PLACEHOLDER|UNRESOLVED|OPEN_DECISION' docs/PRD-CiteLoop-SEO-GEO-Automation-Upgrade.md || true`

Expected: no output.

Run: `awk '/[ \t]$/ { print FNR ": trailing whitespace"; bad=1 } END { exit bad }' docs/PRD-CiteLoop-SEO-GEO-Automation-Upgrade.md`

Expected: exit 0.

### Task 2: Add Failing Tests for Query Gap and Scoring Metadata

**Files:**
- Modify: `internal/seo/search_opportunities_test.go`

- [ ] **Step 1: Extend the test expectations**

Update `TestSearchMetricOpportunityCandidatesUseGSCSignals` to include a third query row:

```go
{
	PageURL:           "https://example.com/guides/source-backed-seo",
	NormalizedPageURL: "/guides/source-backed-seo",
	Query:             "source backed seo workflow",
	Clicks:            28,
	Impressions:       700,
	CTR:               0.04,
	Position:          9.8,
	WindowStart:       windowStart,
	WindowEnd:         windowEnd,
}
```

Change expected candidate count from `3` to `4`, and require candidate type `gsc_query_gap`. Add assertions that every candidate evidence contains non-empty `scoring_method`, `scoring_version`, `expected_impact_range`, and `why_now`.

- [ ] **Step 2: Run test to verify RED**

Run: `go test ./internal/seo -run TestSearchMetricOpportunityCandidatesUseGSCSignals -count=1`

Expected: FAIL because `gsc_query_gap` and scoring metadata are not implemented yet.

### Task 3: Implement Query Gap and Evidence Metadata

**Files:**
- Modify: `internal/seo/search_opportunities.go`

- [ ] **Step 1: Add query-gap candidate logic**

In `searchMetricOpportunityCandidates`, add a candidate when:

```go
row.Impressions >= 250 && row.Position > 3 && row.Position <= 15 && row.CTR > 0.025
```

Use:

- `Type`: `gsc_query_gap`
- `RecommendedAction`: `Expand the existing page or create a supporting section for the query intent`
- `ExpectedImpact`: `Captures demand where Search Console shows relevance but the page is not yet the strongest answer.`
- `RiskLevel`: `medium`

- [ ] **Step 2: Add evidence metadata helper**

Add helper logic so all generated candidates include:

- `scoring_method`
- `scoring_version`
- `expected_impact_range`
- `why_now`
- `data_source_notes`

Do this in evidence JSON, without adding database columns.

- [ ] **Step 3: Run GREEN**

Run: `go test ./internal/seo -run TestSearchMetricOpportunityCandidatesUseGSCSignals -count=1`

Expected: PASS.

### Task 4: Verification

**Files:**
- No new files.

- [ ] **Step 1: Run focused tests**

Run: `go test ./internal/seo ./internal/api ./internal/db -count=1`

Expected: PASS.

- [ ] **Step 2: Run repository validation**

Run: `go test ./... -count=1`

Expected: PASS.

- [ ] **Step 3: Review diff**

Run: `git diff --stat && git diff --check`

Expected: doc plus SEO test/code changes, no whitespace errors.
