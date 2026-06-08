# GEO Visibility PR1 Crawler Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement PR1 from `docs/PRD-CiteLoop-GEO-Visibility-Layer.md`: AI crawler access audit with honest CiteLoop probes, robots-static evidence, persisted snapshots, API routes, and a minimal access matrix UI.

**Architecture:** Add a small `internal/geo` package for PR1-specific audit logic so `internal/seo` stays focused on Search Console / SEO Operations Loop. Persist audit runs in new GEO tables generated through sqlc, expose `/projects/{projectID}/geo/crawler-audit` routes, and surface the latest matrix inside the existing SEO page. PR1 must not spoof third-party bot User-Agents; per-bot conclusions come from static robots parsing plus honest CiteLoop HTTP probes.

**Tech Stack:** Go, Chi, pgx/sqlc, httptest, Next.js App Router, TypeScript node tests.

---

## Round Acceptance

Round 0 acceptance:

- `make test` passes before PR1 work starts.
- `cd web && npm test -- --runInBand` passes before PR1 work starts.
- This plan passes the red-flag scan for unfinished planning language.

Round 1 acceptance:

- `make sqlc` succeeds.
- `go test ./internal/geo ./internal/db ./internal/api ./internal/seo` succeeds.
- `make test` succeeds.
- `cd web && npm test -- --runInBand` succeeds.
- Manual inspection confirms `POST /api/projects/{id}/geo/crawler-audit` and `GET /api/projects/{id}/geo/crawler-audit/latest` are registered.

## File Structure

- Create `internal/migrations/0010_geo_visibility_pr1.sql`: GEO run and AI crawler access snapshot tables.
- Create `internal/db/queries/geo.sql`: sqlc queries for GEO runs, snapshots, and PR1 crawler-block opportunities.
- Create `internal/geo/robots.go`: static robots parser for target user-agents.
- Create `internal/geo/auditor.go`: honest probe auditor and result classification.
- Create `internal/geo/auditor_test.go`: parser and honest-probe tests.
- Create `internal/geo/service.go`: project-scoped crawler audit orchestration and persistence.
- Create `internal/api/handlers_geo.go`: route handlers for PR1 API.
- Modify `internal/api/server.go`: mount `/geo` routes.
- Modify `internal/api/seo_routes_test.go`: route registration coverage for `/geo`.
- Modify generated `internal/db/*.go`: through `make sqlc` only.
- Modify `web/app/lib/api.ts`: GEO API types and client methods.
- Modify `web/app/lib/api.test.mjs`: request-path tests.
- Modify `web/app/projects/[id]/seo/seo-client.tsx`: minimal latest crawler access matrix.

## Task 1: Schema And sqlc

**Files:**
- Create: `internal/migrations/0010_geo_visibility_pr1.sql`
- Create: `internal/db/queries/geo.sql`
- Generated: `internal/db/geo.sql.go`
- Generated: `internal/db/models.go`
- Generated: `internal/db/querier.go`

- [ ] **Step 1: Add schema migration**

Create `internal/migrations/0010_geo_visibility_pr1.sql`:

```sql
-- GEO Visibility Layer PR1: crawler access audit.

create table if not exists geo_runs (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  agent text not null check (agent in ('geo_crawler_audit')),
  status text not null check (status in ('ok','degraded','error')),
  provider text not null default 'citeloop_honest_probe',
  started_at timestamptz not null default now(),
  finished_at timestamptz,
  input jsonb not null default '{}',
  output jsonb not null default '{}',
  error text,
  cost_usd numeric
);

create index if not exists idx_geo_runs_project_agent_started
  on geo_runs (project_id, agent, started_at desc);

create table if not exists ai_crawler_access_snapshots (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  run_id uuid not null references geo_runs(id) on delete cascade,
  page_url text not null,
  normalized_page_url text not null,
  target_user_agent text not null,
  probe_user_agent text not null,
  evidence_type text not null check (evidence_type in ('robots_static','honest_probe','manual_confirmation')),
  robots_state text not null check (robots_state in ('allowed','disallowed','unknown')),
  http_status int,
  access_state text not null check (access_state in ('ok','blocked','challenge','rate_limited','timeout','error')),
  confidence text not null check (confidence in ('high','medium','low')),
  inferred boolean not null default false,
  meta_robots_state text,
  sitemap_state text,
  body_extractable boolean not null default false,
  raw_details jsonb not null default '{}',
  checked_at timestamptz not null default now(),
  unique (project_id, run_id, normalized_page_url, target_user_agent, evidence_type)
);

create index if not exists idx_ai_crawler_access_project_checked
  on ai_crawler_access_snapshots (project_id, checked_at desc);

create index if not exists idx_ai_crawler_access_project_agent
  on ai_crawler_access_snapshots (project_id, target_user_agent, checked_at desc);
```

- [ ] **Step 2: Add sqlc queries**

Create `internal/db/queries/geo.sql`:

```sql
-- name: StartGEORun :one
insert into geo_runs (project_id, agent, status, provider, started_at, input)
values ($1, $2, 'degraded', $3, $4, $5)
returning *;

-- name: FinishGEORun :one
update geo_runs set
  status = $3,
  finished_at = $4,
  output = $5,
  error = $6,
  cost_usd = $7
where id = $1 and project_id = $2
returning *;

-- name: ListGEORuns :many
select * from geo_runs
where project_id = sqlc.arg(project_id)
  and (sqlc.arg(agent)::text = '' or agent = sqlc.arg(agent))
  and (sqlc.arg(status)::text = '' or status = sqlc.arg(status))
  and (sqlc.arg(cursor_started_at)::timestamptz is null or started_at < sqlc.arg(cursor_started_at))
order by started_at desc
limit sqlc.arg(limit_rows);

-- name: UpsertAICrawlerAccessSnapshot :one
insert into ai_crawler_access_snapshots
  (project_id, run_id, page_url, normalized_page_url, target_user_agent, probe_user_agent,
   evidence_type, robots_state, http_status, access_state, confidence, inferred,
   meta_robots_state, sitemap_state, body_extractable, raw_details, checked_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
on conflict (project_id, run_id, normalized_page_url, target_user_agent, evidence_type) do update set
  page_url = excluded.page_url,
  probe_user_agent = excluded.probe_user_agent,
  robots_state = excluded.robots_state,
  http_status = excluded.http_status,
  access_state = excluded.access_state,
  confidence = excluded.confidence,
  inferred = excluded.inferred,
  meta_robots_state = excluded.meta_robots_state,
  sitemap_state = excluded.sitemap_state,
  body_extractable = excluded.body_extractable,
  raw_details = excluded.raw_details,
  checked_at = excluded.checked_at
returning *;

-- name: ListLatestAICrawlerAccessSnapshots :many
select s.*
from ai_crawler_access_snapshots s
join geo_runs r on r.id = s.run_id
where s.project_id = $1
  and r.started_at = (
    select max(started_at)
    from geo_runs
    where project_id = $1 and agent = 'geo_crawler_audit'
  )
order by s.normalized_page_url asc, s.target_user_agent asc, s.evidence_type asc;

-- name: UpsertCrawlerAccessOpportunity :one
with updated as (
  update seo_opportunities set
    priority_score = $4,
    confidence = $5,
    page_url = $6,
    normalized_page_url = $7,
    evidence = seo_opportunities.evidence || $8,
    recommended_action = $9,
    expected_impact = $10,
    effort = $11,
    risk_level = $12,
    updated_at = now()
  where project_id = $1
    and type = $2
    and status in ('open','accepted','converted')
    and normalized_page_url = $7
    and coalesce(query, '') = ''
  returning *
)
insert into seo_opportunities
  (project_id, type, status, priority_score, confidence, page_url, normalized_page_url,
   query, evidence, recommended_action, expected_impact, effort, risk_level, created_by_run_id)
select $1, $2, $3, $4, $5, $6, $7, null, $8, $9, $10, $11, $12, null
where not exists (select 1 from updated)
returning *;
```

- [ ] **Step 3: Run sqlc**

Run: `make sqlc`

Expected: command exits 0 and regenerates `internal/db/geo.sql.go`, `internal/db/models.go`, and `internal/db/querier.go`.

- [ ] **Step 4: Run DB package tests**

Run: `go test ./internal/db`

Expected: PASS.

## Task 2: GEO Audit Domain Logic

**Files:**
- Create: `internal/geo/robots.go`
- Create: `internal/geo/auditor.go`
- Create: `internal/geo/auditor_test.go`

- [ ] **Step 1: Write failing robots and probe tests**

Create `internal/geo/auditor_test.go` with tests covering:

```go
package geo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRobotsRulesAreStaticPerTargetAgent(t *testing.T) {
	body := `User-agent: *
Disallow: /private

User-agent: OAI-SearchBot
Disallow: /blocked

User-agent: PerplexityBot
Allow: /
`
	rules := ParseRobots(strings.NewReader(body))

	if got := rules.StateFor("OAI-SearchBot", "/blocked/page"); got != RobotsDisallowed {
		t.Fatalf("OAI-SearchBot /blocked = %s, want %s", got, RobotsDisallowed)
	}
	if got := rules.StateFor("PerplexityBot", "/blocked/page"); got != RobotsAllowed {
		t.Fatalf("PerplexityBot /blocked = %s, want %s", got, RobotsAllowed)
	}
	if got := rules.StateFor("Claude-SearchBot", "/private/page"); got != RobotsDisallowed {
		t.Fatalf("Claude-SearchBot /private = %s, want %s", got, RobotsDisallowed)
	}
}

func TestAuditorUsesHonestProbeUserAgent(t *testing.T) {
	var seenUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("User-agent: OAI-SearchBot\nDisallow: /blocked\n"))
			return
		}
		seenUA = r.UserAgent()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><head><title>x</title></head><body>hello</body></html>"))
	}))
	defer server.Close()

	auditor := Auditor{HTTPClient: server.Client()}
	results := auditor.Audit(context.Background(), AuditRequest{
		SiteURL: server.URL,
		URLs: []string{server.URL + "/blocked"},
		TargetUserAgents: []string{"OAI-SearchBot"},
	})

	if seenUA != HonestProbeUserAgent {
		t.Fatalf("probe UA = %q, want %q", seenUA, HonestProbeUserAgent)
	}
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if results[0].RobotsState != RobotsDisallowed || results[0].Confidence != ConfidenceHigh || !results[0].Inferred {
		t.Fatalf("result = %+v, want high-confidence inferred robots disallow", results[0])
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/geo`

Expected: FAIL because `ParseRobots`, `Auditor`, and constants do not exist yet.

- [ ] **Step 3: Implement robots parser**

Create `internal/geo/robots.go` defining `RobotsState`, `RobotsRules`, `ParseRobots`, and `StateFor`. It must support `User-agent`, `Allow`, `Disallow`, wildcard fallback, and longest matching rule.

- [ ] **Step 4: Implement honest auditor**

Create `internal/geo/auditor.go` defining:

```go
const HonestProbeUserAgent = "CiteLoop GEO crawler access auditor"

type AuditRequest struct {
	SiteURL string
	URLs []string
	TargetUserAgents []string
}

type AuditResult struct {
	PageURL string
	NormalizedPageURL string
	TargetUserAgent string
	ProbeUserAgent string
	EvidenceType string
	RobotsState RobotsState
	HTTPStatus *int32
	AccessState string
	Confidence string
	Inferred bool
	MetaRobotsState string
	SitemapState string
	BodyExtractable bool
	RawDetails map[string]any
}
```

The implementation fetches `/robots.txt` once with the honest UA, probes each URL once with the honest UA, then combines the honest probe state with per-target robots rules.

- [ ] **Step 5: Run domain tests**

Run: `go test ./internal/geo`

Expected: PASS.

## Task 3: GEO Service And Opportunities

**Files:**
- Create: `internal/geo/service.go`
- Create: `internal/geo/service_test.go`

- [ ] **Step 1: Write service unit tests with a stub store**

Create tests that verify:

- `RunCrawlerAudit` starts and finishes a `geo_crawler_audit` run.
- snapshots preserve `evidence_type`, `confidence`, and `inferred`.
- `robots_disallowed` creates or updates one `geo_crawler_access_blocked` opportunity.
- inferred WAF warnings do not create high-confidence blocker opportunities.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/geo`

Expected: FAIL because `Service.RunCrawlerAudit` does not exist.

- [ ] **Step 3: Implement service**

Implement `Service` with dependencies:

```go
type Store interface {
	GetSEOPropertyForProject(context.Context, uuid.UUID) (db.SeoProperty, error)
	ListPublishedCanonicalArticlesForSEO(context.Context, uuid.UUID) ([]db.Article, error)
	StartGEORun(context.Context, db.StartGEORunParams) (db.GeoRun, error)
	FinishGEORun(context.Context, db.FinishGEORunParams) (db.GeoRun, error)
	UpsertAICrawlerAccessSnapshot(context.Context, db.UpsertAICrawlerAccessSnapshotParams) (db.AiCrawlerAccessSnapshot, error)
	ListLatestAICrawlerAccessSnapshots(context.Context, uuid.UUID) ([]db.AiCrawlerAccessSnapshot, error)
	UpsertCrawlerAccessOpportunity(context.Context, db.UpsertCrawlerAccessOpportunityParams) (db.SeoOpportunity, error)
}
```

The service samples the property site URL plus published canonical URLs, runs `Auditor`, persists snapshots, and creates high-confidence opportunities only for robots-disallowed target agents.

- [ ] **Step 4: Run service tests**

Run: `go test ./internal/geo`

Expected: PASS.

## Task 4: API Routes

**Files:**
- Create: `internal/api/handlers_geo.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/seo_routes_test.go`

- [ ] **Step 1: Add route registration tests**

Append these cases to `TestSEORoutesAreRegistered`:

```go
{name: "geo crawler audit", method: http.MethodPost, path: "/api/projects/not-a-uuid/geo/crawler-audit"},
{name: "geo crawler latest", method: http.MethodGet, path: "/api/projects/not-a-uuid/geo/crawler-audit/latest"},
```

- [ ] **Step 2: Run route test to verify failure**

Run: `go test ./internal/api -run TestSEORoutesAreRegistered`

Expected: FAIL with 404 for the new `/geo` routes.

- [ ] **Step 3: Add handlers and mount routes**

Create `handlers_geo.go` with:

- `runGEOCrawlerAudit`
- `getLatestGEOCrawlerAudit`

Mount under `/projects/{projectID}/geo` in `server.go`:

```go
r.Route("/geo", func(r chi.Router) {
	r.Post("/crawler-audit", s.runGEOCrawlerAudit)
	r.Get("/crawler-audit/latest", s.getLatestGEOCrawlerAudit)
})
```

- [ ] **Step 4: Run API tests**

Run: `go test ./internal/api -run TestSEORoutesAreRegistered`

Expected: PASS.

## Task 5: Web Client And Matrix

**Files:**
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/api.test.mjs`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

- [ ] **Step 1: Add failing API client test**

Add expectations that:

- `runGEOCrawlerAudit("project-1")` calls `/projects/project-1/geo/crawler-audit` with `POST`.
- `getLatestGEOCrawlerAudit("project-1")` calls `/projects/project-1/geo/crawler-audit/latest`.

- [ ] **Step 2: Run web tests to verify failure**

Run: `cd web && npm test -- --runInBand`

Expected: FAIL because the methods do not exist.

- [ ] **Step 3: Implement web API methods and types**

Add `AICrawlerAccessSnapshot`, `GEOCrawlerAuditResult`, `runGEOCrawlerAudit`, and `getLatestGEOCrawlerAudit` to `web/app/lib/api.ts`.

- [ ] **Step 4: Add minimal SEO page matrix**

In `seo-client.tsx`, fetch latest crawler audit alongside SEO data, add a "GEO crawler access" section, and add a button to run audit. The matrix must show target agent, page URL, robots state, access state, confidence, and inferred marker.

- [ ] **Step 5: Run web tests**

Run: `cd web && npm test -- --runInBand`

Expected: PASS.

## Task 6: Round 1 Final Verification

**Files:**
- All PR1 files above.

- [ ] **Step 1: Run generated-code verification**

Run: `make sqlc`

Expected: exits 0 with no unexpected generated drift beyond PR1 sqlc outputs.

- [ ] **Step 2: Run focused Go tests**

Run: `go test ./internal/geo ./internal/db ./internal/api ./internal/seo`

Expected: PASS.

- [ ] **Step 3: Run full Go tests**

Run: `make test`

Expected: PASS.

- [ ] **Step 4: Run web tests**

Run: `cd web && npm test -- --runInBand`

Expected: PASS.

- [ ] **Step 5: Round acceptance report**

Report:

- PR1 files changed.
- Acceptance commands and pass/fail output.
- Whether Round 1 qualifies to continue to PR2.
