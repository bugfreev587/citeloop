# Opportunity Discovery Growth Radar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace repetitive static AI discovery with bounded prompt rotation, sanitized multi-source evidence, deterministic candidate scoring, explainable funnel dispositions, exact target Opportunities, and non-blocking article images.

**Architecture:** `internal/growthradar` owns normalized evidence, context classification, scoring, and funnel decisions while reusing canonical discovery arbitration and Growth Opportunity creation. GEO remains the answer-observation source but selects prompts through a bounded portfolio. Image assets extend Writer/Review/Publisher after canonical drafting through a provider/storage boundary.

**Tech Stack:** Go 1.24, PostgreSQL 16, sqlc, Brave Search API, existing GEO/GSC/crawl/discovery services, Next.js/TypeScript, OpenAI Images API adapter.

---

### Task 1: Bound and rotate the GEO prompt portfolio

**Files:**
- Create: `internal/migrations/0086_growth_radar.sql`
- Modify: `internal/db/queries/geo.sql`
- Regenerate: `internal/db/*.sql.go`, `internal/db/models.go`, `internal/db/querier.go`
- Create: `internal/opportunityfinding/portfolio.go`
- Test: `internal/opportunityfinding/portfolio_test.go`
- Modify: `internal/opportunityfinding/ai_discovery.go`

- [ ] **Step 1: Write failing cap and rotation tests**

Test 60 prompts/project, six/cluster, two/intent+audience, band-0 limit two, eight exploration slots, never-observed before overdue, LRU ordering, stable ID tiebreak, and archived overflow.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./internal/opportunityfinding -run 'PromptPortfolio|PromptRotation' -count=1`
Expected: FAIL against creation-order selection.

- [ ] **Step 3: Add prompt observation state and selector**

```go
type Selection struct {
    Prompts []db.GeoPrompt
    Reasons map[uuid.UUID]string
}

func SelectPrompts(now time.Time, prompts []PromptState, limit int) Selection
func RebuildPortfolio(candidates []PromptCandidate) PortfolioDecision
```

Persist cluster key, last/next observed times, observation count, targeted reason, and archived reason. `RefreshAIDiscoveryEvidence` passes selected prompt IDs to GEO rather than relying on `ListActiveGEOPrompts` ordering.

- [ ] **Step 4: Generate sqlc and run tests**

Run: `sqlc generate`
Expected: generated prompt state fields and queries.

Run: `go test ./internal/opportunityfinding -run 'PromptPortfolio|PromptRotation' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/migrations/0086_growth_radar.sql internal/db internal/opportunityfinding
git commit -m "fix: rotate bounded discovery prompts"
```

### Task 2: Sanitize Project Context before discovery

**Files:**
- Create: `internal/growthradar/context.go`
- Test: `internal/growthradar/context_test.go`
- Modify: `internal/geo/pr2.go`
- Modify: `internal/agents/strategist.go`

- [ ] **Step 1: Write failing classification tests**

Test public capabilities, problems, ICP, confirmed competitors, search language, unknown terms, and internal/sensitive patterns including `AES-256-GCM`, credentials, database and deployment details. Prove only configured competitors are accepted.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./internal/growthradar ./internal/geo ./internal/agents -run 'Context|InternalTerm|ConfirmedCompetitor' -count=1`
Expected: FAIL because raw key terms feed generation.

- [ ] **Step 3: Implement deterministic sanitizer**

```go
type ClassifiedTerm struct {
    Value string `json:"value"`
    Class string `json:"class"`
    Accepted bool `json:"accepted"`
    Reason string `json:"reason"`
}

func ClassifyContext(profile json.RawMessage, evidence EvidenceIndex) Classification
```

LLM suggestions remain `unknown`; only explicit Project Context or qualifying first-party/public evidence can promote them. Feed accepted vocabulary to prompt and topic generation.

- [ ] **Step 4: Run focused tests**

Run: `go test ./internal/growthradar ./internal/geo ./internal/agents -run 'Context|InternalTerm|ConfirmedCompetitor' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/growthradar internal/geo internal/agents
git commit -m "fix: sanitize discovery context"
```

### Task 3: Add budgeted Brave search evidence

**Files:**
- Create: `internal/growthradar/search.go`
- Test: `internal/growthradar/search_test.go`
- Modify: `internal/search/search.go`
- Modify: `internal/search/brave.go`
- Modify: `internal/config/config.go`
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Write failing provenance, cache, and budget tests**

Cover `brave_web_search`, provider order not Google rank, seven-day cache identity, 30/day, 60/weekly rebuild, 600/USD3 per project rolling 30 days, USD25 installation ceiling, mock exclusion, and degraded no-key production state.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./internal/growthradar ./internal/search ./internal/config -run 'SearchEvidence|SearchBudget|Brave' -count=1`
Expected: FAIL because search has no persisted budget/cache.

- [ ] **Step 3: Implement collector**

```go
type SearchBudget struct {
    DailyRequests int
    WeeklyRebuildRequests int
    RollingRequests int
    RollingCostUSD float64
    InstallationCostUSD float64
}

func (c *SearchCollector) Collect(ctx context.Context, req CollectSearchRequest) (EvidenceSet, error)
```

Persist normalized request hash, result-set hash, provider, cost, fetched time, and synthetic flag. Stop before limits; never score mock evidence.

- [ ] **Step 4: Run focused tests**

Run: `go test ./internal/growthradar ./internal/search ./internal/config -run 'SearchEvidence|SearchBudget|Brave' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/growthradar internal/search internal/config cmd/api
git commit -m "feat: collect budgeted search evidence"
```

### Task 4: Implement deterministic topic map and scoring

**Files:**
- Create: `internal/growthradar/topicmap.go`
- Create: `internal/growthradar/score.go`
- Test: `internal/growthradar/topicmap_test.go`
- Test: `internal/growthradar/score_test.go`
- Create: `internal/growthradar/testdata/scoring_v1.json`

- [ ] **Step 1: Write failing boundary and replay tests**

Cover every bucket for demand 25, coverage 20, relevance 15, commercial 15, freshness 10, reuse 10, evidence 5; prove byte-equivalent snapshot replay, LLM text exclusion, exact merge, near-duplicate filter, cannibalization arbitration, and thresholds 59/60/74/75.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./internal/growthradar -run 'TopicMap|Score|Replay|NearDuplicate' -count=1`
Expected: FAIL because scorer does not exist.

- [ ] **Step 3: Implement pure scoring functions**

```go
const FormulaVersion = "growth-radar-score-v1"

type Score struct {
    FormulaVersion string `json:"formula_version"`
    Demand int `json:"demand"`
    CoverageGap int `json:"coverage_gap"`
    Relevance int `json:"relevance"`
    CommercialValue int `json:"commercial_value"`
    Freshness int `json:"freshness"`
    ReusePotential int `json:"reuse_potential"`
    EvidenceQuality int `json:"evidence_quality"`
    Penalties []Penalty `json:"penalties"`
    Final int `json:"final"`
    Disposition string `json:"disposition"`
}

func ScoreCandidate(snapshot Snapshot) (Score, error)
```

Use only persisted facts and enum mappings from the approved PRD. Reject scoring snapshots with missing provenance rather than asking an LLM.

- [ ] **Step 4: Run focused tests**

Run: `go test ./internal/growthradar -run 'TopicMap|Score|Replay|NearDuplicate' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/growthradar
git commit -m "feat: score growth radar candidates"
```

### Task 5: Persist and expose the complete discovery funnel

**Files:**
- Modify: `internal/migrations/0086_growth_radar.sql`
- Modify: `internal/db/queries/seo.sql`
- Regenerate: `internal/db/*.sql.go`, `internal/db/models.go`, `internal/db/querier.go`
- Create: `internal/growthradar/funnel.go`
- Test: `internal/growthradar/funnel_test.go`
- Modify: `internal/opportunityfinding/ai_discovery.go`
- Modify: `internal/api/handlers_seo.go`

- [ ] **Step 1: Write failing funnel tests**

Assert source scheduled/success/skipped/failed, evidence added/changed/reused/expired, terms accepted/rejected/held, portfolio selection, candidate/duplicate/conflict/watchlist/filter/created counts, costs, and degraded-zero semantics.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./internal/growthradar ./internal/opportunityfinding ./internal/api -run 'Funnel|DegradedZero' -count=1`
Expected: FAIL because current result has only coarse steps.

- [ ] **Step 3: Implement funnel recorder and API**

```go
type Funnel struct {
    Sources SourceCounts `json:"sources"`
    Evidence EvidenceCounts `json:"evidence"`
    Terms TermCounts `json:"terms"`
    Prompts PromptCounts `json:"prompts"`
    Candidates CandidateCounts `json:"candidates"`
    CostUSD float64 `json:"cost_usd"`
    Status string `json:"status"`
    Reasons map[string]int `json:"reasons"`
}
```

Persist run and item dispositions transactionally. A run with no usable source or no rotation is `degraded`, never successful-zero.

- [ ] **Step 4: Generate sqlc and run tests**

Run: `sqlc generate`
Expected: funnel queries generated.

Run: `go test ./internal/growthradar ./internal/opportunityfinding ./internal/api -run 'Funnel|DegradedZero' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/migrations/0086_growth_radar.sql internal/db internal/growthradar internal/opportunityfinding internal/api
git commit -m "feat: explain discovery funnel"
```

### Task 6: Create versioned exact-target Opportunity Specs

**Files:**
- Modify: `internal/growthspec/spec.go`
- Modify: `internal/growthspec/spec_test.go`
- Create: `internal/growthradar/materialize.go`
- Test: `internal/growthradar/materialize_test.go`
- Modify: `internal/growthwork/service.go`

- [ ] **Step 1: Write failing spec/materialization tests**

Require intent, stage, audience, cluster, canonical asset type, canonical target, exact target platforms, evidence, action, optional image brief, success metric, dedupe identity, score, source versions, and `contract_matrix|legacy_derived` provenance.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./internal/growthspec ./internal/growthradar ./internal/growthwork -run 'OpportunitySpec|Materialize|LegacyDerived' -count=1`
Expected: FAIL against growth-opportunity-v1.

- [ ] **Step 3: Add v2 spec and canonical materialization**

```go
const VersionV2 = "growth-opportunity-v2"

type TargetSpec struct {
    CanonicalTarget platformcontract.Target `json:"canonical_target"`
    TargetPlatforms []platformcontract.Target `json:"target_platforms"`
    SelectionMode string `json:"selection_mode"`
}
```

Run every candidate through canonical discovery arbitration and Growth Work authority. Exact duplicates merge; filtered/watchlist/conflicts remain funnel items, not Opportunities.

- [ ] **Step 4: Run focused tests**

Run: `go test ./internal/growthspec ./internal/growthradar ./internal/growthwork -run 'OpportunitySpec|Materialize|LegacyDerived' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/growthspec internal/growthradar internal/growthwork
git commit -m "feat: materialize growth radar opportunities"
```

### Task 7: Add discovery diagnostics UI

**Files:**
- Modify: `web/app/lib/api.ts`
- Create: `web/app/lib/growth-radar.ts`
- Test: `web/app/lib/growth-radar.test.mjs`
- Modify: `web/app/projects/[id]/opportunities/page.tsx`

- [ ] **Step 1: Write failing UI logic tests**

Test healthy zero, degraded zero, source failures, prompt rotation, rejection reasons, watchlist and created counts, cost, and exact target summaries.

- [ ] **Step 2: Run test and verify failure**

Run: `cd web && node --test app/lib/growth-radar.test.mjs`
Expected: FAIL because diagnostics helper is absent.

- [ ] **Step 3: Implement diagnostics projection**

```ts
export function summarizeGrowthRadarRun(run: GrowthRadarRun): GrowthRadarSummary;
export function explainZeroOpportunities(run: GrowthRadarRun): string[];
```

Render compact funnel totals and drill-down reason groups on Opportunities without moving Review/Publisher ownership.

- [ ] **Step 4: Run web tests and build**

Run: `cd web && npm test && npm run build`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "feat: explain growth radar runs"
```

### Task 8: Persist non-blocking article image assets

**Files:**
- Create: `internal/migrations/0087_article_assets.sql`
- Modify: `internal/db/queries/articles.sql`
- Regenerate: `internal/db/*.sql.go`, `internal/db/models.go`, `internal/db/querier.go`
- Create: `internal/articleassets/types.go`
- Create: `internal/articleassets/service.go`
- Test: `internal/articleassets/service_test.go`

- [ ] **Step 1: Write failing schema and lifecycle tests**

Cover roles, planned/generating/ready/failed, article+role+brief-hash identity, ready reuse, alt/caption edit without regeneration, explicit revision, budget exhaustion, and non-blocking failure.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./internal/articleassets ./internal/db -run 'ArticleAsset' -count=1`
Expected: FAIL because schema/package are absent.

- [ ] **Step 3: Add migration, queries, and service**

```go
type Provider interface {
    Generate(context.Context, GenerateRequest) (GenerateResult, error)
}

type Store interface {
    Put(context.Context, string, []byte, string) (string, error)
}

func (s *Service) Plan(ctx context.Context, article db.Article, brief Brief) ([]db.ArticleAsset, error)
func (s *Service) Generate(ctx context.Context, projectID, assetID uuid.UUID) (db.ArticleAsset, error)
```

Enforce one hero plus two inline maximum; glossary/FAQ can plan zero; benchmark charts use deterministic renderer and cited data.

- [ ] **Step 4: Generate sqlc and run tests**

Run: `sqlc generate`
Expected: article asset queries generated.

Run: `go test ./internal/articleassets ./internal/db -run 'ArticleAsset' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/migrations/0087_article_assets.sql internal/db internal/articleassets
git commit -m "feat: persist article image assets"
```

### Task 9: Add OpenAI image adapter, Writer planning, Review, and Publisher reuse

**Files:**
- Create: `internal/articleassets/openai.go`
- Test: `internal/articleassets/openai_test.go`
- Modify: `internal/admin/credentials.go`
- Modify: `internal/agents/writer.go`
- Modify: `internal/api/handlers_review.go`
- Modify: `internal/publisher/render.go`
- Modify: `web/app/projects/[id]/review/review-client.tsx`
- Test: `web/app/lib/review-insights.test.mjs`

- [ ] **Step 1: Write failing adapter and integration tests**

Test separate encrypted image credential, approved brief/outline prompt, no raw scrape, informative-role planning, stable URL reuse across canonical/Dev.to/Hashnode, alt/caption editing, forced provider failure, text-only approval/publication, and upload reuse after publish retry.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./internal/articleassets ./internal/agents ./internal/api ./internal/publisher -run 'Image|ArticleAsset' -count=1`
Expected: FAIL because adapter/integration are absent.

- [ ] **Step 3: Implement adapter and integrations**

Use a dedicated encrypted admin credential and explicit image model configuration. Writer plans after canonical draft; generation failure sets `failed` and never changes article reviewability. Review exposes preview, role, alt, caption, omit and regenerate. Publisher rewrites stable stored URLs only where the platform contract allows the role.

- [ ] **Step 4: Run focused and web tests**

Run: `go test ./internal/articleassets ./internal/agents ./internal/api ./internal/publisher -run 'Image|ArticleAsset' -count=1`
Expected: PASS.

Run: `cd web && npm test && npm run build`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal web
git commit -m "feat: generate reviewable article images"
```

### Task 10: Verify Growth Radar end to end and enable guarded rollout

**Files:**
- Modify: `internal/opportunityfinding/workflow.go`
- Modify: `internal/config/config.go`
- Test: `internal/opportunityfinding/workflow_test.go`
- Modify: `docs/superpowers/specs/2026-07-13-opportunity-discovery-growth-radar-design.md`

- [ ] **Step 1: Add full-flow acceptance tests**

Cover sanitized context, rotated prompts, Brave evidence, scoring snapshot, dedupe/watchlist/filter, exact targets, Opportunity v2, platform artifacts, optional images, and complete zero-result explanation.

- [ ] **Step 2: Implement per-project rollout modes**

```go
type GrowthRadarMode string
const (
    GrowthRadarOff GrowthRadarMode = "off"
    GrowthRadarObserve GrowthRadarMode = "observe_only"
    GrowthRadarCreate GrowthRadarMode = "create_opportunities"
)
```

Default existing projects to observe-only during deployment; explicitly enable creation only after production funnel and target-contract acceptance.

- [ ] **Step 3: Run full verification**

Run: `go test ./...`
Expected: PASS.

Run: `go vet ./...`
Expected: PASS.

Run: `cd web && npm test && npm run build`
Expected: PASS.

- [ ] **Step 4: Commit acceptance coverage**

```bash
git add internal docs
git commit -m "test: verify growth radar workflow"
```
