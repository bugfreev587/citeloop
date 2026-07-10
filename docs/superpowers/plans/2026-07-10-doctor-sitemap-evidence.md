# Doctor Sitemap Evidence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop Doctor from reporting sitemap gaps when the current technical crawl did not collect sitemap inclusion evidence.

**Architecture:** Keep `technical_checks.sitemap_status` as the evidence boundary. The Doctor classifier will ignore absent, empty, unknown, and invalid values and will emit a page-level sitemap finding only for the explicit observed value `missing`; the finding will carry that status in its evidence. No crawl behavior or Doctor write targets change.

**Tech Stack:** Go, `testing`, existing `internal/seo` Doctor classifier.

---

### Task 1: Reproduce the evidence-free sitemap false positive

**Files:**
- Modify: `internal/seo/doctor_test.go`

- [x] **Step 1: Write the failing regression test**

Add a table-driven test that builds otherwise healthy `db.TechnicalCheck` rows with `SitemapStatus` set to nil, empty, `unknown`, and `invalid`, calls `doctorFindingCandidatesFromChecks`, and asserts that no candidate has category `sitemap`.

- [x] **Step 2: Run the focused test and verify RED**

Run: `go test ./internal/seo -run TestDoctorSitemapFindingRequiresObservedMissingStatus -count=1`

Expected: FAIL because the current `missingOrUnknown` condition emits `sitemap_unknown` for every case.

### Task 2: Require affirmative sitemap gap evidence

**Files:**
- Modify: `internal/seo/doctor.go`
- Modify: `internal/seo/doctor_test.go`

- [x] **Step 1: Add the positive contract to the regression test**

Add an explicit `SitemapStatus: "missing"` case and assert one `important_page_missing_from_sitemap` candidate whose evidence includes `sitemap_status: "missing"`.

- [x] **Step 2: Implement the minimal classifier change**

Add:

```go
func sitemapGapObserved(value *string) bool {
	return statusValue(value) == "missing"
}
```

Replace the sitemap `missingOrUnknown` branch with `sitemapGapObserved`, emit issue type `important_page_missing_from_sitemap`, and attach the observed status to evidence.

- [x] **Step 3: Run focused tests and verify GREEN**

Run: `go test ./internal/seo -run 'TestDoctorSitemapFindingRequiresObservedMissingStatus|TestDoctor' -count=1`

Expected: PASS.

### Task 3: Verify scope and read-only behavior

**Files:**
- Verify: `internal/seo/doctor.go`
- Verify: `internal/seo/doctor_test.go`

- [x] **Step 1: Inspect the final diff**

Run: `git diff --check && git diff -- internal/seo/doctor.go internal/seo/doctor_test.go`

Expected: only sitemap candidate classification and its regression test change; `RunDoctor` still writes only Doctor run/finding/report state and does not create opportunities, content actions, or site fixes.

- [x] **Step 2: Run relevant and repository-wide verification**

Run: `go test ./internal/seo -count=1`, `go test ./... -count=1`, `npm test`, `npm run typecheck`, and `npm run build`.

Expected: all commands exit 0.

### Task 4: Deliver and verify production

**Files:**
- No additional source files expected.

- [ ] **Step 1: Commit and push the scoped diff**

Stage only the plan and Doctor files, commit with `fix: require sitemap evidence in Doctor`, and push `codex/fix-doctor-sitemap-unknown-20260710`.

- [ ] **Step 2: Open and merge a PR to `origin/main`**

Create a ready PR describing the root cause, evidence semantics, red-green test, and verification; wait for required checks and merge it.

- [ ] **Step 3: Wait for deployment and verify production**

Confirm the merged commit is deployed, run Doctor against a production project whose current technical checks have no sitemap inclusion evidence, and verify the resulting report has no systematic `sitemap_unknown` findings and that no opportunity, content action, or site fix is created by the Doctor run.
