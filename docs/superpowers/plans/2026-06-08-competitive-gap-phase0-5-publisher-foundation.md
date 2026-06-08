# Publisher Connection Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement PRD Phase 0.5 so each project can declare a publisher connection with capabilities and GitHub/Next.js publish settings without exposing raw credentials.

**Architecture:** Add a project-scoped `publisher_connections` table and sqlc queries. Expose project-owner guarded API routes for listing, upserting, testing, and reading publisher health. Reuse the existing GitHub MDX publisher for actual publication, but teach the scheduler to prefer per-project GitHub connection config and fall back to env config only for internal/dev compatibility.

**Tech Stack:** Go API, PostgreSQL migrations, sqlc, pgx, Next.js TypeScript API client, existing settings UI.

---

### Task 1: Add Publisher Connection Schema

**Files:**
- Create: `internal/migrations/0011_publisher_connections.sql`
- Create: `internal/db/queries/publisher_connections.sql`
- Generated: `internal/db/publisher_connections.sql.go`
- Generated: `internal/db/models.go`
- Generated: `internal/db/querier.go`

- [x] **Step 1: Write the migration**

```sql
create table if not exists publisher_connections (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  kind text not null check (kind in ('github_nextjs','webhook','wordpress')),
  label text not null default '',
  status text not null default 'missing' check (status in ('missing','connected','error','revoked')),
  is_default boolean not null default false,
  capabilities jsonb not null default '{}',
  capability_schema_version int not null default 1,
  credential_ref text,
  config jsonb not null default '{}',
  oauth_access_expires_at timestamptz,
  oauth_refresh_status text,
  revoked_at timestamptz,
  last_verified_at timestamptz,
  last_error text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index if not exists publisher_connections_default_kind_key
  on publisher_connections (project_id, kind)
  where is_default;

create index if not exists publisher_connections_project_idx
  on publisher_connections (project_id, kind, status);
```

- [x] **Step 2: Add sqlc queries**

```sql
-- name: ListPublisherConnections :many
select * from publisher_connections
where project_id = $1
order by is_default desc, created_at desc;

-- name: GetPublisherConnectionForProject :one
select * from publisher_connections
where id = $1 and project_id = $2;

-- name: GetDefaultPublisherConnectionForProject :one
select * from publisher_connections
where project_id = $1 and kind = $2 and is_default
limit 1;

-- name: UpsertDefaultPublisherConnection :one
insert into publisher_connections
  (project_id, kind, label, status, is_default, capabilities, capability_schema_version, credential_ref, config, last_verified_at, last_error)
values
  ($1, $2, $3, $4, true, $5, $6, $7, $8, $9, $10)
on conflict (project_id, kind) where is_default
do update set
  label = excluded.label,
  status = excluded.status,
  capabilities = excluded.capabilities,
  capability_schema_version = excluded.capability_schema_version,
  credential_ref = excluded.credential_ref,
  config = excluded.config,
  last_verified_at = excluded.last_verified_at,
  last_error = excluded.last_error,
  updated_at = now()
returning *;

-- name: MarkPublisherConnectionVerified :one
update publisher_connections
set status = 'connected',
    last_verified_at = now(),
    last_error = null,
    updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: MarkPublisherConnectionError :one
update publisher_connections
set status = 'error',
    last_error = $3,
    updated_at = now()
where id = $1 and project_id = $2
returning *;
```

- [x] **Step 3: Generate sqlc**

Run: `make sqlc`

Expected: generated Go types include `PublisherConnection` and `ListPublisherConnections`.

### Task 2: Add Publisher Connection Domain Helpers

**Files:**
- Create: `internal/publisher/connections.go`
- Test: `internal/publisher/connections_test.go`

- [x] **Step 1: Implement capabilities and sanitized GitHub config**

```go
package publisher

import (
	"encoding/json"
	"errors"
	"strings"
)

const (
	ConnectionKindGitHubNextJS = "github_nextjs"
	CapabilityCreateArticle   = "create_article"
	CapabilityUpdateArticle   = "update_article"
	CapabilityMetadataUpdate  = "metadata_update"
	CapabilityCanonical       = "canonical"
	CapabilityDraftMode       = "draft_mode"
	CapabilityPublishMode     = "publish_mode"
	CapabilityRollback        = "rollback"
)

type Capabilities map[string]bool

func GitHubNextJSCapabilities() Capabilities {
	return Capabilities{
		CapabilityCreateArticle:  true,
		CapabilityUpdateArticle:  true,
		CapabilityMetadataUpdate: true,
		CapabilityCanonical:      true,
		CapabilityDraftMode:      false,
		CapabilityPublishMode:    true,
		CapabilityRollback:       false,
	}
}

type GitHubNextJSConfig struct {
	Repo       string `json:"repo"`
	Branch     string `json:"branch"`
	ContentDir string `json:"content_dir"`
	BaseURL    string `json:"base_url"`
	PublishMode string `json:"publish_mode"`
}

func ParseGitHubNextJSConfig(raw json.RawMessage) (GitHubNextJSConfig, error) {
	var cfg GitHubNextJSConfig
	if len(raw) == 0 {
		return cfg, errors.New("publisher config is empty")
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	cfg.Repo = strings.TrimSpace(cfg.Repo)
	cfg.Branch = strings.TrimSpace(cfg.Branch)
	cfg.ContentDir = strings.TrimSpace(cfg.ContentDir)
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.PublishMode = strings.TrimSpace(cfg.PublishMode)
	if cfg.Branch == "" {
		cfg.Branch = "citeloop-content"
	}
	if cfg.ContentDir == "" {
		cfg.ContentDir = "content/citeloop/blog"
	}
	if cfg.PublishMode == "" {
		cfg.PublishMode = "publish"
	}
	if cfg.Repo == "" || cfg.BaseURL == "" {
		return cfg, errors.New("repo and base_url are required")
	}
	return cfg, nil
}
```

- [x] **Step 2: Test normalization and secret redaction behavior**

Run: `go test ./internal/publisher`

Expected: PASS.

### Task 3: Expose Project-Scoped Publisher Connection API

**Files:**
- Modify: `internal/api/server.go`
- Create: `internal/api/handlers_publisher_connections.go`
- Test: `internal/api/publisher_connections_routes_test.go`

- [x] **Step 1: Add routes under project owner guard**

```go
r.Get("/publisher-connections", s.listPublisherConnections)
r.Put("/publisher-connections/github-nextjs", s.upsertGitHubNextJSPublisherConnection)
r.Post("/publisher-connections/{connectionID}/test", s.testPublisherConnection)
```

- [x] **Step 2: Implement handlers**

Handlers must:

- Accept only sanitized GitHub/Next.js config: `repo`, `branch`, `content_dir`, `base_url`, `publish_mode`, `credential_ref`.
- Reject any `token`, `secret`, `webhook_url`, or `password` field in request JSON.
- Store capability JSON from `publisher.GitHubNextJSCapabilities()`.
- Return the connection without raw credential values.
- Test connection by validating config and, when possible, calling `PublishedPathExists` only if token is available through env fallback.

- [x] **Step 3: Add route tests**

Tests must assert:

- Project-scoped endpoint paths exist.
- Raw secret-like fields are rejected.
- Response contains `capabilities.create_article = true`.
- API does not echo `token`.

Run: `go test ./internal/api -run PublisherConnection`

Expected: PASS.

### Task 4: Prefer Per-Project Publisher Config During Auto-Publish

**Files:**
- Modify: `internal/scheduler/scheduler.go`
- Test: `internal/scheduler/publisher_connection_test.go`

- [x] **Step 1: Add resolver**

Add a helper that attempts to load `github_nextjs` default connection for the project and builds a `publisher.BlogPublisher` from its config. If no connection exists, use `s.Blog`.

- [x] **Step 2: Keep env fallback explicit**

Implementation note: this slice prevents per-project connections without credentials from reusing the fallback env token. It does not yet implement a full encrypted credential store; `credential_ref=env:GITHUB_TOKEN` is the only supported resolver for now.

If per-project connection exists but has no usable credential, publish in dry-run with detail that says `publisher connection configured without credential; env fallback not used for this project`.

- [x] **Step 3: Add tests**

Tests must cover:

- Missing connection uses existing `s.Blog`.
- Connected GitHub config overrides repo/branch/content dir/base URL.
- Connection without credential never exposes raw secrets in errors.

Run: `go test ./internal/scheduler -run PublisherConnection`

Expected: PASS.

### Task 5: Add Web Client Types and Settings UI

**Files:**
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/api.test.mjs`
- Modify: `web/app/projects/[id]/settings/settings-client.tsx`

- [x] **Step 1: Add API client methods**

Add:

- `listPublisherConnections(projectId)`
- `upsertGitHubNextJSPublisherConnection(projectId, input)`
- `testPublisherConnection(projectId, connectionId)`

- [x] **Step 2: Add settings section**

Add a compact GitHub/Next.js publisher panel with fields for repo, branch, content path, base URL, and credential ref. Do not include a raw token field.

- [x] **Step 3: Add API client tests**

Run: `cd web && npm test -- --runInBand` if available, otherwise `cd web && npm test`.

Expected: PASS.

### Task 6: Verification

**Files:**
- No new files.

- [x] **Step 1: Run Go tests**

Run: `make test`

Expected: PASS.

- [x] **Step 2: Run web tests/build**

Run: `cd web && npm test`

Expected: PASS.

- [x] **Step 3: Update PRD execution status if needed**

If implementation differs from PRD, update `docs/PRD-CiteLoop-Competitive-Gap-Roadmap.md` or add notes to the final response.
