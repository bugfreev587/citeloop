# SEO/GEO Automation Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the Phase 2 GEO Monitoring 1.0 launch gate by making the weekly background GEO loop use an automatic answer/citation provider when configured.

**Architecture:** The API already wires Perplexity into `geo.Service` for manual `/geo/runs/observe-provider` calls, while the scheduler currently constructs `geo.Service` without a provider. Add scheduler-level provider configuration, pass process env values from `cmd/api`, and keep the weekly automation path on the automatic provider endpoint rather than the manual fixture endpoint.

**Tech Stack:** Go 1.25, sqlc-generated PostgreSQL access, existing `internal/geo` service and `internal/scheduler` cron.

---

### Task 1: Scheduler Provider Wiring

**Files:**
- Modify: `internal/scheduler/scheduler.go`
- Create: `internal/scheduler/geo_tick_test.go`
- Modify: `cmd/api/main.go`
- Create: `cmd/api/geo_provider_wiring_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestSchedulerGEOTickUsesConfiguredAnswerProvider(t *testing.T) {
	s := &Scheduler{
		GEOAnswerProvider:       fakeGEOAnswerProvider{available: true},
		GEOProviderRunBudgetUSD: 0.25,
	}
	service := s.geoService(db.New(nil))
	if service.AnswerProvider == nil || !service.AnswerProvider.Available() {
		t.Fatal("geo service should include configured automatic answer provider")
	}
	if s.geoObserveRequest().MaxPrompts != 10 || s.geoObserveRequest().BudgetUSD != 0.25 {
		t.Fatalf("unexpected observe request: %+v", s.geoObserveRequest())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/scheduler -run TestSchedulerGEOTickUsesConfiguredAnswerProvider -count=1`

Expected: FAIL because `Scheduler` has no provider fields or helper methods.

- [ ] **Step 3: Write minimal implementation**

Add provider and budget fields to `Scheduler`, centralize `geo.Service` construction in `geoService`, centralize weekly request defaults in `geoObserveRequest`, and set these fields from `config.Env` in `cmd/api/main.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/scheduler -run TestSchedulerGEOTickUsesConfiguredAnswerProvider -count=1`

Expected: PASS.

### Task 2: Phase 2 Contract Regression

**Files:**
- Modify: `internal/config/config_test.go`
- Modify: `.env.example`

- [ ] **Step 1: Write the failing tests**

```go
func TestEnvExampleDocumentsGEOProviderConfig(t *testing.T) {
	raw, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}
	body := string(raw)
	for _, want := range []string{"PERPLEXITY_API_KEY=", "PERPLEXITY_BASE_URL=https://api.perplexity.ai", "PERPLEXITY_MODEL=sonar-pro", "GEO_PROVIDER_RUN_BUDGET_USD=1"} {
		if !strings.Contains(body, want) {
			t.Fatalf(".env.example missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run TestEnvExampleDocumentsGEOProviderConfig -count=1`

Expected: FAIL until `.env.example` documents the provider/budget contract.

- [ ] **Step 3: Write minimal implementation**

Update the scheduler contract and `.env.example` so production operators know `PERPLEXITY_API_KEY`, `PERPLEXITY_BASE_URL`, `PERPLEXITY_MODEL`, and `GEO_PROVIDER_RUN_BUDGET_USD` control automatic GEO observation.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/scheduler ./internal/config -count=1`

Expected: PASS.

### Task 3: Full Verification and Ship

**Files:**
- No further code files expected.

- [ ] **Step 1: Run backend tests**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 2: Run frontend contract checks**

Run: `cd web && npm test && npm run typecheck`

Expected: PASS.

- [ ] **Step 3: Push and create PR**

Run: `git push -u origin codex/seo-geo-automation-phase2`, then create a PR to `origin/main`.

Expected: PR contains only Phase 2 provider wiring and docs.

- [ ] **Step 4: Merge and verify production**

After merge and deployment, verify production exposes the deployed build and that GEO provider observation no longer degrades because the scheduler lacks provider wiring.
