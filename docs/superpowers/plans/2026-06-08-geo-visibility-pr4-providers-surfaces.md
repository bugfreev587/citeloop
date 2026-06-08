# GEO Visibility PR4 Provider + Surface Monitor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a legal answer-provider observation path and an honest external-surface monitor for CiteLoop GEO visibility.

**Architecture:** Keep PR4 inside the existing GEO service boundary. Provider observation writes `geo_runs` and `geo_observations`; unavailable or over-budget providers are recorded as degraded `provider_unavailable` observations and do not create a score. Surface monitoring reuses `geo_external_surfaces` and honest CiteLoop HTTP probes, updating HTTP status without login scraping or private-account automation.

**Tech Stack:** Go service/API, sqlc/Postgres models already generated in earlier PRs, Next.js client contract helpers, Jest contract tests, Go unit/route tests.

---

## Files

- Modify: `internal/config/config.go` to expose `PERPLEXITY_API_KEY`, optional base URL/model, and a GEO provider budget default.
- Modify: `internal/geo/service.go` to hold an optional `AnswerProvider`.
- Create: `internal/geo/provider.go` for provider contracts, Perplexity/Sonar adapter, and provider observation service.
- Create: `internal/geo/surfaces.go` for honest external-surface monitoring.
- Create: `internal/geo/service_pr4_test.go` for TDD coverage of unavailable providers, successful provider observations, Perplexity HTTP contract, and honest surface monitor UA.
- Modify: `internal/api/handlers_geo.go` and `internal/api/handlers_geo_pr2.go` to wire the provider into `geoService()` and add handlers.
- Modify: `internal/api/server.go` and `internal/api/seo_routes_test.go` to register and verify new routes.
- Modify: `web/app/lib/api.ts` and `web/app/lib/api.test.mjs` for frontend API contracts.
- Modify: `web/app/projects/[id]/seo/seo-client.tsx` to add provider observation and surface-monitor controls to the existing GEO section.

## Acceptance

- `go test -count=1 ./internal/geo ./internal/api ./internal/config` passes.
- `make test` passes.
- `cd web && npm test -- --runInBand` passes.
- `cd web && npm run typecheck` passes.
- Provider unavailable path creates degraded run + `provider_unavailable` observations and does not create a misleading score.
- Perplexity adapter uses official Sonar API shape: `POST /v1/sonar`, Bearer auth, `model`, `messages`, `citations`, and `usage.cost.total_cost`.
- Surface monitor sends only an honest CiteLoop UA and updates `last_http_status` through `geo_external_surfaces`.

---

### Task 1: Provider Tests

**Files:**
- Create: `internal/geo/service_pr4_test.go`
- Modify later: `internal/geo/provider.go`

- [ ] **Step 1: Write failing tests**

Add tests for these behaviors:

```go
func TestObserveAnswerProviderUnavailableMarksRunDegraded(t *testing.T) {
	projectID := uuid.New()
	store := &geoStoreStub{
		runID: uuid.New(),
		prompts: []db.GeoPrompt{{ID: uuid.New(), ProjectID: projectID, PromptText: "best tools for social scheduling", Locale: "en-US", Status: "active"}},
	}

	result, err := Service{Q: store}.ObserveAnswerProvider(context.Background(), projectID, ObserveAnswerProviderRequest{Engine: "Perplexity", MaxPrompts: 1})
	requireNoError(t, err)
	assertStatus(t, store.finishedStatus, "degraded")
	assertObservationState(t, result.Observations, "provider_unavailable")
	assertScoreNotCreated(t, store.visibilityScores)
}
```

```go
func TestObserveAnswerProviderPersistsObservedCitationsAndScore(t *testing.T) {
	projectID := uuid.New()
	promptID := uuid.New()
	store := &geoStoreStub{
		runID: uuid.New(),
		prompts: []db.GeoPrompt{{ID: promptID, ProjectID: projectID, PromptText: "best tools for social scheduling", Locale: "en-US", Status: "active"}},
		surfaces: []db.GeoExternalSurface{{ID: uuid.New(), ProjectID: projectID, Url: "https://unipost.dev", NormalizedUrl: "https://unipost.dev/", OwnerType: "project", SurfaceType: "domain"}},
	}
	provider := fakeAnswerProvider{responses: []ProviderObservation{{
		PromptID: promptID,
		Engine: "Perplexity",
		AnswerSummary: "UniPost and Buffer are mentioned.",
		CitedURLs: []string{"https://unipost.dev/blog/social-scheduling", "https://buffer.com/resources"},
		BrandMentioned: true,
		Confidence: ConfidenceMedium,
		CostUSD: 0.02,
	}}}

	result, err := Service{Q: store, AnswerProvider: provider}.ObserveAnswerProvider(context.Background(), projectID, ObserveAnswerProviderRequest{Engine: "Perplexity", MaxPrompts: 1})
	requireNoError(t, err)
	assertStatus(t, store.finishedStatus, "ok")
	assertObservedCitationCount(t, result.Observations, 1)
	assertScoreCreated(t, store.visibilityScores)
}
```

```go
func TestPerplexityProviderUsesSonarContract(t *testing.T) {
	var sawAuth, sawModel bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization") == "Bearer test-key"
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		sawModel = body["model"] == "sonar-pro"
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "UniPost is cited."}}},
			"citations": []string{"https://unipost.dev/blog/social-scheduling"},
			"usage": map[string]any{"cost": map[string]any{"total_cost": 0.01}},
		})
	}))
	defer server.Close()

	provider := NewPerplexityProvider("test-key", server.URL, "sonar-pro", server.Client())
	rows, cost, err := provider.Observe(context.Background(), []db.GeoPrompt{{ID: uuid.New(), PromptText: "best tools", Locale: "en-US"}})
	requireNoError(t, err)
	assertBool(t, sawAuth && sawModel, "expected auth header and model")
	assertCost(t, cost, 0.01)
	assertCitations(t, rows, "https://unipost.dev/blog/social-scheduling")
}
```

- [ ] **Step 2: Run tests to verify RED**

Run: `go test -count=1 ./internal/geo`

Expected: FAIL because `AnswerProvider`, `ObserveAnswerProvider`, and `NewPerplexityProvider` are not implemented.

### Task 2: Provider Implementation

**Files:**
- Modify: `internal/geo/service.go`
- Create: `internal/geo/provider.go`

- [ ] **Step 1: Add the provider interface and service field**

```go
type AnswerProvider interface {
	Name() string
	Available() bool
	Observe(ctx context.Context, prompts []db.GeoPrompt) ([]ProviderObservation, float64, error)
}

type ProviderObservation struct {
	PromptID uuid.UUID
	Engine string
	Locale string
	AnswerSummary string
	CitedURLs []string
	BrandMentioned bool
	BrandPosition *int32
	CompetitorMentions []string
	CompetitorCitations []string
	EvidenceSnippets []string
	Confidence string
	CostUSD float64
}
```

Add `AnswerProvider AnswerProvider` to `Service`.

- [ ] **Step 2: Implement `ObserveAnswerProvider`**

Behavior:
- Start a `geo_runs` row with `agent=geo_observer`.
- Load active prompts and apply `MaxPrompts` as a budget cap.
- If no provider or provider unavailable: create `provider_unavailable` observations for sampled prompts, finish `degraded`, and do not call `scoreObservations`.
- If provider succeeds: persist observed observations as `source_type=answer_engine`, calculate owned citations via project-owned external surfaces, create a score, finish `ok`.
- If provider returns partial or error: persist what exists, finish `degraded`, and include the provider error in run output/error.

- [ ] **Step 3: Implement Perplexity/Sonar adapter**

Use the official Sonar chat completion shape:
- `POST {baseURL}/v1/sonar`
- `Authorization: Bearer <key>`
- JSON body with `model`, `messages`, `temperature`, and `max_tokens`
- read `choices[0].message.content`
- read top-level `citations`
- read `usage.cost.total_cost`

- [ ] **Step 4: Run tests to verify GREEN**

Run: `go test -count=1 ./internal/geo`

Expected: PASS.

### Task 3: Surface Monitor Tests + Implementation

**Files:**
- Modify: `internal/geo/service_pr4_test.go`
- Create: `internal/geo/surfaces.go`

- [ ] **Step 1: Write failing test**

```go
func TestMonitorExternalSurfacesUsesHonestUAAndUpdatesStatus(t *testing.T) {
	var userAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.UserAgent()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	projectID := uuid.New()
	store := &geoStoreStub{
		runID: uuid.New(),
		surfaces: []db.GeoExternalSurface{{ID: uuid.New(), ProjectID: projectID, Url: server.URL + "/citation", NormalizedUrl: server.URL + "/citation", OwnerType: "project", SurfaceType: "page"}},
	}

	result, err := Service{Q: store, HTTPClient: server.Client()}.MonitorExternalSurfaces(context.Background(), projectID, MonitorExternalSurfacesRequest{Limit: 10})
	requireNoError(t, err)
	assertStatus(t, store.finishedStatus, "ok")
	assertContains(t, userAgent, "CiteLoop GEO external surface monitor")
	assertSurfaceStatus(t, result.Surfaces, http.StatusAccepted)
}
```

- [ ] **Step 2: Verify RED**

Run: `go test -count=1 ./internal/geo`

Expected: FAIL because `MonitorExternalSurfaces` does not exist.

- [ ] **Step 3: Implement monitor**

Behavior:
- Start `geo_runs` with `agent=geo_external_surface_monitor` and `provider=citeloop_honest_probe`.
- Load `geo_external_surfaces`.
- Probe each URL with honest UA `CiteLoop GEO external surface monitor`.
- Upsert each surface with the existing fields plus `last_http_status`.
- Finish `ok` if at least one surface was probed, `degraded` if no surfaces or some probes fail.

- [ ] **Step 4: Verify GREEN**

Run: `go test -count=1 ./internal/geo`

Expected: PASS.

### Task 4: API + Config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/api/handlers_geo.go`
- Modify: `internal/api/handlers_geo_pr2.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/seo_routes_test.go`

- [ ] **Step 1: Add route tests first**

Add route checks for:
- `POST /api/projects/{projectID}/geo/runs/observe-provider`
- `POST /api/projects/{projectID}/geo/external-surfaces/monitor`

Run: `go test -count=1 ./internal/api`

Expected: FAIL because routes are missing.

- [ ] **Step 2: Add config and handlers**

Config:
- `PERPLEXITY_API_KEY`
- `PERPLEXITY_BASE_URL` default `https://api.perplexity.ai`
- `PERPLEXITY_MODEL` default `sonar-pro`
- `GEO_PROVIDER_RUN_BUDGET_USD` default `1.00`

Handlers:
- `observeGEOAnswerProvider`
- `monitorGEOExternalSurfaces`

Routes:
- `r.Post("/runs/observe-provider", s.observeGEOAnswerProvider)`
- `r.Post("/external-surfaces/monitor", s.monitorGEOExternalSurfaces)`

- [ ] **Step 3: Verify API**

Run: `go test -count=1 ./internal/api ./internal/config`

Expected: PASS.

### Task 5: Frontend Contract + UI

**Files:**
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/api.test.mjs`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

- [ ] **Step 1: Add failing API contract tests**

Assert the client exposes:
- `observeGEOProvider(projectID, body)` using `/geo/runs/observe-provider`
- `monitorGEOExternalSurfaces(projectID, body)` using `/geo/external-surfaces/monitor`

Run: `cd web && npm test -- --runInBand`

Expected: FAIL because client helpers are missing.

- [ ] **Step 2: Implement client helpers and UI buttons**

Add:
- Provider observation button near GEO observations.
- Surface monitor button near external surfaces.
- Busy states `geo-provider` and `geo-surface-monitor`.
- Refresh overview after each successful action.

- [ ] **Step 3: Verify frontend**

Run:
- `cd web && npm test -- --runInBand`
- `cd web && npm run typecheck`

Expected: PASS.

### Task 6: Full Round 4 Acceptance

- [ ] Run `go test -count=1 ./internal/geo ./internal/api ./internal/config`
- [ ] Run `make test`
- [ ] Run `cd web && npm test -- --runInBand`
- [ ] Run `cd web && npm run typecheck`
- [ ] Summarize PR4 acceptance and continue to next PRD round only if all checks pass.
