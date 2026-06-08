# GEO Visibility PR7 Weekly Scheduler Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the PRD §12.3 weekly GEO scheduler hook so observation/analyzer work can run without manual clicks.

**Architecture:** Extend the existing scheduler with `TickGEO(ctx)`. It lists projects and, per project, calls the existing GEO service methods using honest HTTP probes and provider-unavailable degradation when no provider is configured. Cron registration is weekly and independent from daily generation/publish ticks.

**Tech Stack:** Go scheduler package, existing `geo.Service`, existing sqlc queries, scheduler helper tests.

---

## Files

- Modify: `internal/scheduler/scheduler.go` to add `TickGEO` and `geoForProject`.
- Modify: `internal/scheduler/helpers.go` to register `TickGEO` weekly.
- Modify: `internal/scheduler/helpers_test.go` to assert method exposure and cron registration.

## Acceptance

- `go test -count=1 ./internal/scheduler` passes.
- `go test -count=1 ./internal/geo ./internal/api ./internal/seo ./internal/scheduler` passes.
- `make test` passes.
- `Start()` includes a weekly GEO tick.
- `TickGEO` runs crawler audit, answer-provider observation, external surface monitor, and analyzer on each project while logging per-step errors and continuing to the next project.

---

### Task 1: Scheduler Red Tests

- [ ] Add `TestSchedulerExposesGEOTick`:

```go
var _ func(*Scheduler, context.Context) = (*Scheduler).TickGEO
```

- [ ] Extend `TestStartRegistersNotificationTick` to require `TickGEO` and `@weekly` in `helpers.go`.

- [ ] Run `go test -count=1 ./internal/scheduler`.

Expected: FAIL because `TickGEO` and cron registration are missing.

### Task 2: Implement TickGEO

- [ ] Import `internal/geo`.
- [ ] Add:

```go
func (s *Scheduler) TickGEO(ctx context.Context)
func (s *Scheduler) geoForProject(ctx context.Context, q *db.Queries, p db.Project) error
```

Per project:
- `RunCrawlerAudit(ctx, p.ID, geo.CrawlerAuditRequest{})`
- `ObserveAnswerProvider(ctx, p.ID, geo.ObserveAnswerProviderRequest{Engine:"Perplexity", MaxPrompts:10, BudgetUSD:1})`
- `MonitorExternalSurfaces(ctx, p.ID, geo.MonitorExternalSurfacesRequest{Limit:25})`
- `AnalyzeObservations(ctx, p.ID, geo.AnalyzeObservationsRequest{Limit:100})`

Each step logs and continues; the project returns nil unless listing projects fails.

### Task 3: Register Cron + Verify

- [ ] Add `_, _ = c.AddFunc("@weekly", func() { s.TickGEO(ctx) })` to `Start`.
- [ ] Update scheduler started log with `"geo", "weekly"`.
- [ ] Run acceptance commands.
