# SEO Doctor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build SEO Doctor as a user-facing technical SEO health check that runs on onboarding, weekly, and manual triggers, shows staged progress, and produces human and AI-coding-tool repair reports.

**Architecture:** Add dedicated `seo_doctor_runs` and `seo_doctor_findings` tables while reusing existing SEO property normalization, `technical_checks`, `seo_runs`, `seo_opportunities`, `content_actions`, onboarding, and scheduler patterns. The backend owns run dedupe, rate limits, progress persistence, health scoring, findings, and action handoff; the frontend consumes stable API contracts through `web/app/lib/api.ts` and renders Home entry plus `/projects/[id]/doctor`.

**Tech Stack:** Go 1.x, chi, pgx, sqlc, PostgreSQL migrations, Next.js App Router, React client components, lucide-react, Node test runner, Go tests.

---

## File Map

- Create `internal/migrations/0034_seo_doctor.sql`: Doctor run/finding schema, indexes, constraints.
- Modify `internal/db/queries/seo.sql`: sqlc queries for Doctor runs, findings, weekly eligibility, manual rate limit, conversion, and dismissal.
- Generate `internal/db/seo.sql.go`, `internal/db/models.go`, `internal/db/querier.go`: sqlc output after query/schema changes.
- Create `internal/seo/doctor.go`: Doctor service, progress stages, dedupe, run execution, finding classification, health score, report summaries.
- Create `internal/seo/doctor_test.go`: unit tests for progress, soft 404 classification, health score caps, stable finding keys, report counts.
- Create `internal/api/handlers_seo_doctor.go`: Doctor HTTP handlers and response shaping.
- Modify `internal/api/server.go`: register Doctor routes under `/seo/doctor`.
- Modify `internal/api/onboarding.go` and `internal/api/onboarding_test.go`: start onboarding Doctor after URL project creation via a test seam.
- Modify `internal/scheduler/scheduler.go`, `internal/scheduler/helpers.go`, and `internal/scheduler/seo_tick_test.go`: add weekly Doctor runner and scheduler registration.
- Modify `internal/api/seo_routes_test.go`: contract coverage for all Doctor endpoints.
- Modify `internal/db/seo_contract_test.go`: schema/query contract coverage for tables, progress write throttle fields, active-run dedupe, and weekly freshness.
- Modify `web/app/lib/api.ts`: Doctor types, normalizers, and methods.
- Add `web/app/projects/[id]/doctor/page.tsx` and `web/app/projects/[id]/doctor/doctor-client.tsx`: Doctor page UI.
- Modify `web/app/components/project-shell.tsx`: add Doctor under Home.
- Modify `web/app/projects/[id]/workspace.tsx`: Home Doctor module and active progress summary.
- Add `web/app/lib/seo-doctor-contract.test.mjs`: frontend route/API/UI contract tests.

## Task 1: Backend Contract Tests First

**Files:**
- Modify: `internal/api/seo_routes_test.go`
- Modify: `internal/db/seo_contract_test.go`
- Create: `internal/seo/doctor_test.go`

- [ ] **Step 1: Add failing API route registration tests**

Add these cases to `TestSEORoutesAreRegistered`:

```go
{name: "doctor summary", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/doctor"},
{name: "doctor create run", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/doctor/runs"},
{name: "doctor run detail", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/doctor/runs/not-an-id"},
{name: "doctor run findings", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/doctor/runs/not-an-id/findings"},
{name: "doctor latest", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/doctor/latest"},
{name: "doctor convert finding", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/doctor/findings/not-an-id/convert"},
{name: "doctor dismiss finding", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/doctor/findings/not-an-id/dismiss"},
```

- [ ] **Step 2: Add failing DB contract tests**

Add tests asserting generated query constants contain:

```go
for _, query := range []string{
  createSEODoctorRun,
  getActiveSEODoctorRun,
  countManualSEODoctorRunsSince,
  listSEODoctorFindingsForRun,
  upsertSEODoctorFinding,
  latestCompletedSEODoctorRun,
} {
  if strings.TrimSpace(query) == "" {
    t.Fatal("doctor query constant should exist")
  }
}
```

Also assert the migration text contains `seo_doctor_runs`, `seo_doctor_findings`, `block_reason`, `progress_percent`, `finding_key`, and the partial uniqueness for active findings.

- [ ] **Step 3: Add failing Doctor engine tests**

Create tests for:

```go
func TestDoctorHealthScoreCapsActiveP0At69(t *testing.T)
func TestDoctorHealthScoreCapsActiveP1At84(t *testing.T)
func TestDoctorSummaryIssueCountExcludesInfo(t *testing.T)
func TestDoctorProgressInterpolatesWithinCheckingStage(t *testing.T)
func TestDoctorSoft404HighConfidenceCanBeP0(t *testing.T)
func TestDoctorSoft404MediumConfidenceDefaultsBelowP0(t *testing.T)
func TestDoctorFindingKeyKeepsEvidenceVariantsOutOfHash(t *testing.T)
```

- [ ] **Step 4: Run tests and verify RED**

Run:

```bash
go test ./internal/api ./internal/db ./internal/seo -count=1
```

Expected: FAIL because Doctor routes, queries, and engine symbols do not exist.

## Task 2: Schema and sqlc Queries

**Files:**
- Create: `internal/migrations/0034_seo_doctor.sql`
- Modify: `internal/db/queries/seo.sql`
- Generate: `internal/db/seo.sql.go`, `internal/db/models.go`, `internal/db/querier.go`

- [ ] **Step 1: Create Doctor tables**

`seo_doctor_runs` columns:

```sql
id uuid primary key default gen_random_uuid(),
project_id uuid not null references projects(id) on delete cascade,
trigger text not null check (trigger in ('onboarding','manual','weekly','post_publish')),
status text not null default 'queued' check (status in ('queued','running','completed','failed','blocked')),
stage text not null default 'queued',
progress_percent int not null default 0 check (progress_percent >= 0 and progress_percent <= 100),
message text not null default '',
block_reason text,
pages_discovered int not null default 0,
pages_fetched int not null default 0,
pages_checked int not null default 0,
issues_found int not null default 0,
health_score int,
input_snapshot jsonb not null default '{}',
output_summary jsonb not null default '{}',
error text,
created_by_user_id text,
started_at timestamptz,
updated_at timestamptz not null default now(),
finished_at timestamptz,
created_at timestamptz not null default now()
```

`seo_doctor_findings` columns:

```sql
id uuid primary key default gen_random_uuid(),
project_id uuid not null references projects(id) on delete cascade,
run_id uuid not null references seo_doctor_runs(id) on delete cascade,
finding_key text not null,
severity text not null check (severity in ('P0','P1','P2','Info')),
category text not null,
issue_type text not null,
status text not null default 'active' check (status in ('active','resolved','dismissed','converted')),
affected_urls jsonb not null default '[]',
normalized_urls jsonb not null default '[]',
evidence jsonb not null default '{}',
why_it_matters text not null default '',
fix_intent text not null default '',
developer_instructions text not null default '',
likely_files_or_surfaces jsonb not null default '[]',
acceptance_tests jsonb not null default '[]',
risk_level text not null default 'low' check (risk_level in ('low','medium','high')),
review_required boolean not null default true,
autofix_eligible boolean not null default false,
linked_opportunity_id uuid references seo_opportunities(id) on delete set null,
linked_content_action_id uuid references content_actions(id) on delete set null,
first_seen_at timestamptz not null default now(),
last_seen_at timestamptz not null default now(),
resolved_at timestamptz,
created_at timestamptz not null default now(),
updated_at timestamptz not null default now()
```

Indexes: project/status/updated run lookup, active-run dedupe where status in `queued,running`, project/finding key active lookup, run findings by severity, and `idx_seo_doctor_runs_project_finished`.

- [ ] **Step 2: Add sqlc queries**

Add queries:

```sql
-- name: CreateSEODoctorRun :one
-- name: GetSEODoctorRun :one
-- name: GetActiveSEODoctorRun :one
-- name: UpdateSEODoctorRunProgress :one
-- name: CompleteSEODoctorRun :one
-- name: FailSEODoctorRun :one
-- name: LatestSEODoctorRun :one
-- name: LatestCompletedSEODoctorRun :one
-- name: CountManualSEODoctorRunsSince :one
-- name: ListSEODoctorRunsDueWeekly :many
-- name: UpsertSEODoctorFinding :one
-- name: ResolveMissingSEODoctorFindings :exec
-- name: ListSEODoctorFindingsForRun :many
-- name: GetSEODoctorFinding :one
-- name: DismissSEODoctorFinding :one
-- name: LinkSEODoctorFindingToAction :one
```

- [ ] **Step 3: Generate sqlc**

Run:

```bash
go run github.com/sqlc-dev/sqlc/cmd/sqlc generate
```

Expected: generated Go includes Doctor row structs and query params.

- [ ] **Step 4: Run DB contract tests**

Run:

```bash
go test ./internal/db -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit schema/query work**

```bash
git add internal/migrations/0034_seo_doctor.sql internal/db/queries/seo.sql internal/db/*.go
git commit -m "feat: add seo doctor persistence"
```

## Task 3: Doctor Engine

**Files:**
- Create: `internal/seo/doctor.go`
- Modify: `internal/seo/service.go` only for shared helper reuse if needed
- Test: `internal/seo/doctor_test.go`

- [ ] **Step 1: Implement exported service contract**

Add:

```go
type DoctorTrigger string
type DoctorStage string

type DoctorRunRequest struct {
  ProjectID uuid.UUID
  Trigger DoctorTrigger
  SiteURL string
  CreatedByUserID *string
}

type DoctorReport struct {
  Run db.SeoDoctorRun `json:"run"`
  Findings []db.SeoDoctorFinding `json:"findings"`
  Human DoctorHumanReport `json:"human_report"`
  AICodingTool DoctorAIReport `json:"ai_coding_tool_report"`
}
```

Service methods:

```go
func (s Service) StartDoctorRun(ctx context.Context, req DoctorRunRequest) (db.SeoDoctorRun, error)
func (s Service) RunDoctor(ctx context.Context, runID uuid.UUID) (DoctorReport, error)
func (s Service) DoctorLatest(ctx context.Context, projectID uuid.UUID) (DoctorReport, error)
func (s Service) DoctorReport(ctx context.Context, projectID, runID uuid.UUID) (DoctorReport, error)
func (s Service) ConvertDoctorFinding(ctx context.Context, projectID, findingID uuid.UUID) (db.ContentAction, error)
func (s Service) DismissDoctorFinding(ctx context.Context, projectID, findingID uuid.UUID) (db.SeoDoctorFinding, error)
```

- [ ] **Step 2: Implement progress stages**

Use fixed starts:

```go
queued: 0
discovering: 10
crawling: 25
checking: 50
classifying: 75
writing_report: 88
handoff: 95
completed: 100
```

`DoctorProgressPercent(stage, completed, total)` returns interpolated values inside `crawling` and `checking`, capped at the next stage start minus one.

- [ ] **Step 3: Implement report generation from existing checks**

For Phase 1, collect:

- Latest `technical_checks` from existing `ListLatestTechnicalChecks`.
- Published canonical articles from `ListPublishedCanonicalArticlesForSEO`.
- SEO property via `ensureProperty`.

Create findings for broken HTTP, redirect/canonical mismatch, noindex/robots issues when present in raw details, metadata/social gaps, structured data gaps, sitemap status, internal link gaps, and unsafe MDX.

- [ ] **Step 4: Implement active probe classifiers**

Add pure helpers for the Phase 1B contract:

```go
func ClassifySoft404(input Soft404Evidence) DoctorFindingCandidate
func ConfidenceValue(label string) int
```

Map `high=90`, `medium=70`, `low=50`. Only high-confidence soft 404 can become P0.

- [ ] **Step 5: Implement health score and display status**

Use PRD formula:

```go
raw = sum(P0 * 20 * importance * confidence) +
      sum(P1 * 8 * importance * confidence) +
      sum(P2 * 2 * importance * confidence)
score = max(0, round(100 - min(raw, 100)))
```

Then cap active P0 at 69 and active P1 at 84. Display status: `blocked`, `needs_attention`, `healthy`.

- [ ] **Step 6: Implement finding key normalization**

Hash project ID, issue type, normalized URL set, normalized canonical target, and normalized structural locator. Preserve evidence variants in `evidence`, not the hash.

- [ ] **Step 7: Run engine tests**

Run:

```bash
go test ./internal/seo -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit engine work**

```bash
git add internal/seo/doctor.go internal/seo/doctor_test.go internal/seo/service.go
git commit -m "feat: add seo doctor engine"
```

## Task 4: API, Onboarding, Scheduler

**Files:**
- Create: `internal/api/handlers_seo_doctor.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/onboarding.go`
- Modify: `internal/api/onboarding_test.go`
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/scheduler/helpers.go`
- Modify: `internal/scheduler/seo_tick_test.go`
- Modify: `internal/api/seo_routes_test.go`

- [ ] **Step 1: Add Doctor handlers**

Handlers:

```go
func (s *Server) getSEODoctor(w http.ResponseWriter, r *http.Request)
func (s *Server) createSEODoctorRun(w http.ResponseWriter, r *http.Request)
func (s *Server) getSEODoctorRun(w http.ResponseWriter, r *http.Request)
func (s *Server) listSEODoctorRunFindings(w http.ResponseWriter, r *http.Request)
func (s *Server) getLatestSEODoctor(w http.ResponseWriter, r *http.Request)
func (s *Server) convertSEODoctorFinding(w http.ResponseWriter, r *http.Request)
func (s *Server) dismissSEODoctorFinding(w http.ResponseWriter, r *http.Request)
```

Manual create returns active onboarding/weekly/manual run when one exists. If no active run exists, enforce 3 manual runs per project per hour.

- [ ] **Step 2: Register routes**

Under `/api/projects/{projectID}/seo`:

```go
r.Get("/doctor", s.getSEODoctor)
r.Post("/doctor/runs", s.createSEODoctorRun)
r.Get("/doctor/runs/{runID}", s.getSEODoctorRun)
r.Get("/doctor/runs/{runID}/findings", s.listSEODoctorRunFindings)
r.Get("/doctor/latest", s.getLatestSEODoctor)
r.Post("/doctor/findings/{findingID}/convert", s.convertSEODoctorFinding)
r.Post("/doctor/findings/{findingID}/dismiss", s.dismissSEODoctorFinding)
```

- [ ] **Step 3: Add onboarding trigger**

Add `DoctorOnboardingRunner func(context.Context, projectOnboardingInput)` to `Server`. `runProjectSEOOnboarding` calls SEO sync/analyze and then Doctor onboarding. Tests assert onboarding starts Doctor without waiting on quick profile.

- [ ] **Step 4: Add weekly scheduler**

Extend `seoRunner` with `StartDoctorRun` and `RunDoctor`, or add a dedicated Doctor runner seam. Implement `TickSEODoctor` that lists weekly-due projects and skips projects with a completed manual/onboarding/weekly/post_publish run in the last 6 days.

- [ ] **Step 5: Register cron**

Add `@weekly` registration in `Start` and log `seo_doctor`.

- [ ] **Step 6: Run API/scheduler tests**

Run:

```bash
go test ./internal/api ./internal/scheduler -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit API/scheduler work**

```bash
git add internal/api internal/scheduler
git commit -m "feat: expose seo doctor api and triggers"
```

## Task 5: Frontend Doctor Contracts and UI

**Files:**
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/components/project-shell.tsx`
- Modify: `web/app/projects/[id]/workspace.tsx`
- Create: `web/app/projects/[id]/doctor/page.tsx`
- Create: `web/app/projects/[id]/doctor/doctor-client.tsx`
- Create: `web/app/lib/seo-doctor-contract.test.mjs`

- [ ] **Step 1: Add failing frontend contract tests**

Assertions:

```js
assert.match(api, /export type SEODoctorRun/)
assert.match(api, /getSEODoctor/)
assert.match(api, /startSEODoctorRun/)
assert.match(shell, /label: "Doctor"/)
assert.equal(exists("projects/[id]/doctor/page.tsx"), true)
assert.match(doctorClient, /progress_percent/)
assert.match(doctorClient, /ai_coding_tool_report/)
assert.match(workspace, /getSEODoctor/)
```

- [ ] **Step 2: Add API types and normalizers**

Types:

```ts
export type SEODoctorRunStatus = "queued" | "running" | "completed" | "failed" | "blocked" | string;
export type SEODoctorStage = "queued" | "discovering" | "crawling" | "checking" | "classifying" | "writing_report" | "handoff" | "completed" | string;
export type SEODoctorFindingSeverity = "P0" | "P1" | "P2" | "Info" | string;
export type SEODoctorRun = { id: string; trigger: string; status: SEODoctorRunStatus; stage: SEODoctorStage; progress_percent: number; message: string; pages_discovered: number; pages_fetched: number; pages_checked: number; issues_found: number; health_score?: number | null; block_reason?: string | null; output_summary?: any; updated_at?: any; finished_at?: any };
export type SEODoctorFinding = { id: string; finding_key: string; severity: SEODoctorFindingSeverity; category: string; issue_type: string; status: string; affected_urls: string[]; evidence: any; why_it_matters: string; fix_intent: string; developer_instructions: string; likely_files_or_surfaces: string[]; acceptance_tests: string[]; risk_level: string; review_required: boolean; autofix_eligible: boolean };
export type SEODoctorReport = { run?: SEODoctorRun | null; findings: SEODoctorFinding[]; human_report?: any; ai_coding_tool_report?: any };
```

Methods:

```ts
getSEODoctor(id): Promise<SEODoctorReport>
getLatestSEODoctor(id): Promise<SEODoctorReport>
startSEODoctorRun(id): Promise<SEODoctorRun>
getSEODoctorRun(id, runID): Promise<SEODoctorRun>
listSEODoctorRunFindings(id, runID): Promise<SEODoctorFinding[]>
convertSEODoctorFinding(id, findingID): Promise<SEOContentAction>
dismissSEODoctorFinding(id, findingID): Promise<SEODoctorFinding>
```

- [ ] **Step 3: Add nav entry**

Add `{ label: "Doctor", href: "doctor", icon: Stethoscope }` to the primary section under Home.

- [ ] **Step 4: Add Home Doctor module**

Fetch `api.getSEODoctor(projectId).catch(() => null)` in `refresh`. Show card states:

- no run: "Run your first site health check"
- running: stage, progress bar, checked page count, "View Doctor"
- completed: score, issue count excluding Info, last run, "Open report"
- blocked/failed: block reason/error and retry link

- [ ] **Step 5: Add Doctor page**

`page.tsx` renders `DoctorClient`. Client supports manual run, active progress, report summary, severity filters, AI report copy/export section, finding convert/dismiss actions, and automatic polling while status is queued/running.

- [ ] **Step 6: Run frontend tests and typecheck**

Run:

```bash
cd web && npm test -- --runInBand
cd web && npm run typecheck
```

Expected: PASS.

- [ ] **Step 7: Commit frontend work**

```bash
git add web/app/lib/api.ts web/app/components/project-shell.tsx web/app/projects/[id]/workspace.tsx web/app/projects/[id]/doctor web/app/lib/seo-doctor-contract.test.mjs
git commit -m "feat: add seo doctor ui"
```

## Task 6: End-to-End Verification and Release

**Files:**
- All changed files.

- [ ] **Step 1: Run full local verification**

Run:

```bash
go test ./... -count=1
cd web && npm test -- --runInBand
cd web && npm run typecheck
```

Expected: PASS.

- [ ] **Step 2: Start local app and verify manually**

Start the repo's normal API/web dev commands. Verify:

- Home contains Doctor under Home.
- `/projects/{id}/doctor` loads.
- Manual run starts and shows progress.
- Latest report renders health score and findings.
- AI coding tool report is visible.

- [ ] **Step 3: Open PR**

Push branch and create PR to `origin/main`.

- [ ] **Step 4: Merge PR and wait for deployment**

Follow repo rule: merge PR, wait for deployment finish, then verify production.

- [ ] **Step 5: Production verification**

On production:

- Create or use a project with a URL and confirm onboarding Doctor run exists.
- Trigger manual Doctor run and confirm active-run progress polling.
- Confirm weekly scheduler configuration includes Doctor.
- Confirm latest report renders, including AI coding tool report and issue count excluding Info.
- Confirm manual completion resets weekly freshness window through DB/API state.

## Self-Review

- Spec coverage: covers schema, reuse, progress, health score, soft 404 confidence, APIs, onboarding, weekly scheduler, Home, Doctor page, action handoff, and production verification.
- Placeholder scan: no deferred implementation placeholders; each task names concrete files, symbols, commands, and expected results.
- Type consistency: backend `seo_doctor_runs` and frontend `SEODoctorRun` share `progress_percent`, `stage`, `health_score`, `block_reason`, page counters, and status names.
