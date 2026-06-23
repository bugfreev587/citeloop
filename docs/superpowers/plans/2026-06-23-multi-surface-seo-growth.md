# Multi-Surface SEO Growth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `docs/PRD-CiteLoop-Multi-Surface-SEO-Growth-Layer.md` in gated phases, verifying each phase before moving to the next.

**Architecture:** Phase 1 extends the existing SEO/GEO/Autopilot schemas instead of creating parallel systems. Phase 2 wires multi-surface action metadata into publisher/review/API flows while keeping current blog and semi-manual distribution behavior stable. Phase 3 extends GEO/AI visibility actions on top of existing prompts, observations, external surfaces, and asset briefs.

**Tech Stack:** Go 1.x, PostgreSQL migrations embedded from `internal/migrations`, sqlc v1.30.0, pgx/v5, Next.js dashboard in `web`, existing `make test`/`make build`/`make web-build` verification.

---

## Phase Gates

- **Phase 1 Gate:** `make sqlc`, `make test`, and `make build` pass. Schema contract tests prove no duplicate action/portfolio/distribution tables were introduced.
- **Phase 2 Gate:** Phase 1 gate still passes, focused publisher/API/review tests pass, and web build passes after dashboard/API type updates.
- **Phase 3 Gate:** Phase 2 gate still passes, GEO tests pass, and the final PRD acceptance checklist is green.
- **Release Gate:** Branch is pushed, PR to `origin/main` is opened and merged, deployment completes, and production behavior is verified.

## File Structure

- `docs/PRD-CiteLoop-Multi-Surface-SEO-Growth-Layer.md`: Product spec, committed with implementation.
- `internal/migrations/0021_multi_surface_seo_growth.sql`: Adds/extends schema from PRD §8.
- `internal/db/queries/seo.sql`: Adds asset type and enriched content action queries.
- `internal/db/queries/geo.sql`: Extends surface and asset brief queries.
- `internal/db/queries/articles.sql`: Adds distribution metadata update/read helpers if direct columns are used.
- `internal/db/*sql.go`, `internal/db/models.go`, `internal/db/querier.go`: sqlc generated output.
- `internal/db/multi_surface_schema_contract_test.go`: Migration and query contract tests.
- `internal/autopilot/risk.go`: Extends existing risk classifier inputs.
- `internal/autopilot/risk_test.go`: TDD coverage for multi-surface risk dimensions.
- `internal/api/handlers_seo.go`: Exposes enriched content action fields.
- `internal/api/handlers_geo*.go`: Exposes generalized surface/brief fields where needed.
- `web/app/lib/api.ts`: Mirrors enriched API types when UI changes begin.
- `web/app/projects/[id]/**`: Existing dashboard/review/GEO pages extended in Phase 2/3.

---

## Phase 1: Planning and Inventory Foundation

### Task 1: Schema Contract Tests

**Files:**
- Create: `internal/db/multi_surface_schema_contract_test.go`
- Read: `internal/migrations/0021_multi_surface_seo_growth.sql`
- Read: `internal/db/queries/seo.sql`
- Read: `internal/db/queries/geo.sql`

- [ ] **Step 1: Write the failing migration contract test**

Create `internal/db/multi_surface_schema_contract_test.go`:

```go
package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readMultiSurfaceMigration(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "migrations", "0021_multi_surface_seo_growth.sql"))
	if err != nil {
		t.Fatalf("read multi-surface migration: %v", err)
	}
	return strings.ToLower(string(raw))
}

func TestMultiSurfaceMigrationExtendsExistingTables(t *testing.T) {
	migration := readMultiSurfaceMigration(t)
	for _, want := range []string{
		"create table if not exists seo_asset_types",
		"alter table content_actions",
		"add column if not exists asset_type",
		"add column if not exists target_surface_id",
		"add column if not exists risk_reasons",
		"add column if not exists evidence_snapshot",
		"add column if not exists diff_snapshot",
		"add column if not exists review_required",
		"add column if not exists verified_at",
		"alter table geo_external_surfaces",
		"add column if not exists source_url",
		"add column if not exists canonical_status",
		"add column if not exists indexability_status",
		"add column if not exists publication_status",
		"add column if not exists owner_confidence",
		"alter table geo_asset_briefs",
		"add column if not exists target_queries",
		"add column if not exists expected_citation_mechanism",
		"alter table seo_opportunities",
		"add column if not exists opportunity_key",
		"alter table articles",
		"add column if not exists publication_mode",
		"add column if not exists external_surface_id",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("multi-surface migration missing %q", want)
		}
	}
}

func TestMultiSurfaceMigrationDoesNotCreateParallelWorkflowTables(t *testing.T) {
	migration := readMultiSurfaceMigration(t)
	for _, forbidden := range []string{
		"create table if not exists seo_actions",
		"create table if not exists action_portfolios",
		"create table if not exists distribution_variants",
	} {
		if strings.Contains(migration, forbidden) {
			t.Fatalf("multi-surface migration must extend existing tables, found forbidden %q", forbidden)
		}
	}
}

func TestMultiSurfaceQueriesExposeFoundationRecords(t *testing.T) {
	seoRaw, err := os.ReadFile(filepath.Join("queries", "seo.sql"))
	if err != nil {
		t.Fatalf("read seo queries: %v", err)
	}
	geoRaw, err := os.ReadFile(filepath.Join("queries", "geo.sql"))
	if err != nil {
		t.Fatalf("read geo queries: %v", err)
	}
	queries := strings.ToLower(string(seoRaw) + "\n" + string(geoRaw))
	for _, want := range []string{
		"-- name: upsertseoassettype :one",
		"-- name: listseoassettypes :many",
		"asset_type",
		"target_surface_id",
		"verification_snapshot",
		"source_url",
		"expected_citation_mechanism",
	} {
		if !strings.Contains(queries, want) {
			t.Fatalf("multi-surface queries missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db -run MultiSurface -count=1`

Expected: FAIL because `0021_multi_surface_seo_growth.sql` does not exist yet.

- [ ] **Step 3: Do not implement schema yet**

Stop after the red test. Task 2 adds the migration and queries.

### Task 2: Schema Migration and sqlc Queries

**Files:**
- Create: `internal/migrations/0021_multi_surface_seo_growth.sql`
- Modify: `internal/db/queries/seo.sql`
- Modify: `internal/db/queries/geo.sql`
- Modify: `internal/db/queries/articles.sql`
- Generated: `internal/db/*.sql.go`, `internal/db/models.go`, `internal/db/querier.go`

- [ ] **Step 1: Add migration**

Create `internal/migrations/0021_multi_surface_seo_growth.sql` with:

```sql
-- Multi-Surface SEO Growth Layer foundation.

create table if not exists seo_asset_types (
  id uuid primary key default gen_random_uuid(),
  key text not null unique,
  name text not null,
  description text not null default '',
  default_risk_level text not null default 'medium'
    check (default_risk_level in ('low','medium','high')),
  default_measurement_window_days int not null default 28,
  supported_publication_surfaces jsonb not null default '[]',
  requires_evidence boolean not null default true,
  requires_review_by_default boolean not null default true,
  default_generation_path text not null default 'topic_article'
    check (default_generation_path in ('topic_article','direct_patch','external_draft','technical_task')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

insert into seo_asset_types
  (key, name, description, default_risk_level, default_measurement_window_days,
   supported_publication_surfaces, requires_evidence, requires_review_by_default, default_generation_path)
values
  ('blog_post', 'Blog post', 'Canonical article or supporting article.', 'medium', 56, '["blog"]', true, true, 'topic_article'),
  ('comparison_page', 'Comparison page', 'Competitor or product comparison page.', 'medium', 90, '["blog","landing","docs"]', true, true, 'topic_article'),
  ('alternative_page', 'Alternative page', 'Alternative-to page for buyer-intent searches.', 'medium', 90, '["blog","landing"]', true, true, 'topic_article'),
  ('template_or_checklist', 'Template or checklist', 'Reusable template, checklist, or downloadable asset.', 'medium', 56, '["blog","docs","hosted_asset"]', true, true, 'topic_article'),
  ('glossary_definition', 'Glossary definition', 'Definition or terminology page/block.', 'low', 56, '["blog","docs","landing"]', true, false, 'topic_article'),
  ('metadata_rewrite', 'Metadata rewrite', 'Title/meta rewrite for an existing page.', 'low', 28, '["owned_site"]', true, false, 'direct_patch'),
  ('internal_link_patch', 'Internal link patch', 'Internal link additions or edits.', 'low', 56, '["owned_site"]', true, false, 'direct_patch'),
  ('schema_patch', 'Structured data patch', 'JSON-LD or structured data update.', 'medium', 28, '["owned_site"]', true, true, 'direct_patch'),
  ('sitemap_update', 'Sitemap update', 'Sitemap creation, update, or submit action.', 'low', 14, '["owned_site"]', false, false, 'technical_task')
on conflict (key) do update set
  name = excluded.name,
  description = excluded.description,
  default_risk_level = excluded.default_risk_level,
  default_measurement_window_days = excluded.default_measurement_window_days,
  supported_publication_surfaces = excluded.supported_publication_surfaces,
  requires_evidence = excluded.requires_evidence,
  requires_review_by_default = excluded.requires_review_by_default,
  default_generation_path = excluded.default_generation_path,
  updated_at = now();

alter table content_actions
  add column if not exists asset_type text,
  add column if not exists target_surface_id uuid,
  add column if not exists risk_reasons jsonb not null default '[]',
  add column if not exists evidence_snapshot jsonb not null default '{}',
  add column if not exists input_snapshot jsonb not null default '{}',
  add column if not exists output_snapshot jsonb not null default '{}',
  add column if not exists diff_snapshot jsonb not null default '{}',
  add column if not exists review_required boolean not null default true,
  add column if not exists approved_by text,
  add column if not exists approved_at timestamptz,
  add column if not exists verified_at timestamptz,
  add column if not exists verification_snapshot jsonb not null default '{}';

alter table content_actions drop constraint if exists content_actions_status_check;
alter table content_actions
  add constraint content_actions_status_check
  check (status in (
    'drafting','ready_for_review','approved','published','measuring','completed','failed',
    'verification_failed','recovery_required'
  ));

alter table geo_external_surfaces
  add column if not exists source_url text,
  add column if not exists canonical_status text not null default 'unknown',
  add column if not exists indexability_status text not null default 'unknown',
  add column if not exists publication_status text not null default 'unknown',
  add column if not exists owner_confidence text not null default 'medium'
    check (owner_confidence in ('high','medium','low')),
  add column if not exists last_verified_at timestamptz,
  add column if not exists verification_snapshot jsonb not null default '{}',
  add column if not exists related_action_ids jsonb not null default '[]';

alter table geo_external_surfaces drop constraint if exists geo_external_surfaces_owner_type_check;
alter table geo_external_surfaces
  add constraint geo_external_surfaces_owner_type_check
  check (owner_type in ('project','user','owned','managed_external','third_party'));

alter table geo_asset_briefs
  add column if not exists target_queries jsonb not null default '[]',
  add column if not exists target_personas jsonb not null default '[]',
  add column if not exists expected_citation_mechanism text not null default '',
  add column if not exists source_type text not null default 'geo'
    check (source_type in ('geo','seo','distribution','technical'));

alter table seo_opportunities
  add column if not exists opportunity_key text;

update seo_opportunities
set opportunity_key = encode(digest(
  project_id::text || '|' || type || '|' || coalesce(normalized_page_url, '') || '|' ||
  coalesce(query, '') || '|' || coalesce((evidence->>'intent_type'), '') || '|' ||
  coalesce((evidence->>'engine'), '') || '|' || coalesce((evidence->>'evidence_window'), ''),
  'sha256'
), 'hex')
where opportunity_key is null or opportunity_key = '';

alter table seo_opportunities
  alter column opportunity_key set default '',
  alter column opportunity_key set not null;

create unique index if not exists uniq_open_seo_opportunity_key
  on seo_opportunities (project_id, opportunity_key)
  where status in ('open','accepted','converted');

alter table articles
  add column if not exists publication_mode text not null default 'auto'
    check (publication_mode in ('auto','draft_only','gated_publish','auto_allowed')),
  add column if not exists source_url text,
  add column if not exists external_url text,
  add column if not exists verification_status text not null default 'unknown'
    check (verification_status in ('unknown','pending','passed','failed')),
  add column if not exists external_surface_id uuid;
```

- [ ] **Step 2: Add asset type queries to `internal/db/queries/seo.sql`**

Append:

```sql
-- name: UpsertSEOAssetType :one
insert into seo_asset_types
  (key, name, description, default_risk_level, default_measurement_window_days,
   supported_publication_surfaces, requires_evidence, requires_review_by_default, default_generation_path)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
on conflict (key) do update set
  name = excluded.name,
  description = excluded.description,
  default_risk_level = excluded.default_risk_level,
  default_measurement_window_days = excluded.default_measurement_window_days,
  supported_publication_surfaces = excluded.supported_publication_surfaces,
  requires_evidence = excluded.requires_evidence,
  requires_review_by_default = excluded.requires_review_by_default,
  default_generation_path = excluded.default_generation_path,
  updated_at = now()
returning *;

-- name: ListSEOAssetTypes :many
select * from seo_asset_types
order by key asc;
```

- [ ] **Step 3: Extend `CreateContentAction` query**

Modify `CreateContentAction` to insert/update `asset_type`, `target_surface_id`, `risk_reasons`, `evidence_snapshot`, `input_snapshot`, `diff_snapshot`, and `review_required` using `sqlc.narg(...)`/`sqlc.arg(...)`. Keep existing parameters backward-compatible where callers can pass zero values.

- [ ] **Step 4: Add a focused content action enrichment query**

Append to `internal/db/queries/seo.sql`:

```sql
-- name: UpdateContentActionExecutionMetadata :one
update content_actions set
  asset_type = coalesce(sqlc.narg(asset_type), asset_type),
  target_surface_id = coalesce(sqlc.narg(target_surface_id), target_surface_id),
  risk_reasons = sqlc.arg(risk_reasons),
  evidence_snapshot = sqlc.arg(evidence_snapshot),
  input_snapshot = sqlc.arg(input_snapshot),
  output_snapshot = sqlc.arg(output_snapshot),
  diff_snapshot = sqlc.arg(diff_snapshot),
  review_required = sqlc.arg(review_required),
  approved_by = coalesce(sqlc.narg(approved_by), approved_by),
  approved_at = coalesce(sqlc.narg(approved_at), approved_at),
  verified_at = coalesce(sqlc.narg(verified_at), verified_at),
  verification_snapshot = sqlc.arg(verification_snapshot),
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;
```

- [ ] **Step 5: Extend GEO surface and brief queries**

Update `UpsertGEOExternalSurface` and `CreateGEOAssetBrief` to include the new columns, and make the conflict update preserve old data when new nullable text/time values are absent.

- [ ] **Step 6: Run sqlc**

Run: `make sqlc`

Expected: exit 0 and generated Go files updated.

- [ ] **Step 7: Run schema contract test**

Run: `go test ./internal/db -run MultiSurface -count=1`

Expected: PASS.

### Task 3: Multi-Surface Risk Classifier

**Files:**
- Modify: `internal/autopilot/risk.go`
- Modify: `internal/autopilot/risk_test.go`

- [ ] **Step 1: Write failing risk tests**

Add tests:

```go
func TestClassifyRiskUsesMultiSurfaceInputs(t *testing.T) {
	policy := RiskPolicy{}

	comparison := ClassifyRisk(RiskInput{
		ActionType:         "create comparison page",
		AssetType:          "comparison_page",
		PublicationSurface: "blog",
		Confidence:         90,
	}, policy)
	if comparison.Level != RiskMedium {
		t.Fatalf("comparison pages should be medium risk, got %s reasons=%v", comparison.Level, comparison.Reasons)
	}

	community := ClassifyRisk(RiskInput{
		ActionType:           "external distribution",
		AssetType:            "blog_post",
		PublicationSurface:   "external",
		DistributionPlatform: "reddit",
		Confidence:           90,
	}, policy)
	if community.Level != RiskHigh {
		t.Fatalf("community auto distribution should be high risk, got %s reasons=%v", community.Level, community.Reasons)
	}

	schemaPatch := ClassifyRisk(RiskInput{
		ActionType:    "schema patch",
		AssetType:     "schema_patch",
		SchemaChange:  true,
		Confidence:    90,
	}, policy)
	if schemaPatch.Level != RiskMedium {
		t.Fatalf("schema patch should be medium risk, got %s reasons=%v", schemaPatch.Level, schemaPatch.Reasons)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/autopilot -run MultiSurface -count=1`

Expected: FAIL because `RiskInput` lacks the new fields.

- [ ] **Step 3: Extend `RiskInput` and rules**

Add fields:

```go
AssetType            string
PublicationSurface   string
DistributionPlatform string
ExternalOwnerType    string
SchemaChange         bool
```

Add normalization and rules:

- `comparison_page` and `alternative_page` -> medium.
- `schema_patch` or `SchemaChange` -> medium.
- `PublicationSurface == "external"` with `reddit` or `hacker_news` -> high.
- `ExternalOwnerType == "third_party"` for write/distribution action -> medium unless already high.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/autopilot -run 'Risk|MultiSurface' -count=1`

Expected: PASS.

### Task 4: Phase 1 API and Store Compatibility

**Files:**
- Modify: `internal/api/handlers_seo.go`
- Modify: `internal/api/handlers_geo_pr2.go` or relevant GEO handlers only if generated query params require compile changes.
- Modify: tests/stubs affected by sqlc signature changes.

- [ ] **Step 1: Run compile to surface generated signature breakages**

Run: `go test ./...`

Expected: FAIL if sqlc query parameter changes require caller updates.

- [ ] **Step 2: Update callers with explicit defaults**

For each `CreateContentActionParams`, pass:

```go
AssetType:            pgtype.Text{String: "", Valid: false},
TargetSurfaceID:      pgtype.UUID{},
RiskReasons:          json.RawMessage("[]"),
EvidenceSnapshot:     json.RawMessage("{}"),
InputSnapshot:        json.RawMessage("{}"),
DiffSnapshot:         json.RawMessage("{}"),
ReviewRequired:       true,
```

For each `UpsertGEOExternalSurfaceParams`, pass defaults for new fields:

```go
SourceUrl:            pgtype.Text{},
CanonicalStatus:      "unknown",
IndexabilityStatus:   "unknown",
PublicationStatus:    "unknown",
OwnerConfidence:      "medium",
LastVerifiedAt:       pgtype.Timestamptz{},
VerificationSnapshot: json.RawMessage("{}"),
RelatedActionIds:     json.RawMessage("[]"),
```

For each `CreateGEOAssetBriefParams`, pass:

```go
TargetQueries:               json.RawMessage("[]"),
TargetPersonas:              json.RawMessage("[]"),
ExpectedCitationMechanism:   "",
SourceType:                  "geo",
```

- [ ] **Step 3: Run full Go tests**

Run: `make test`

Expected: PASS.

### Task 5: Phase 1 Verification and Commit

**Files:**
- All files modified in Phase 1.

- [ ] **Step 1: Run phase gate**

Run:

```bash
make sqlc
make test
make build
```

Expected: all exit 0.

- [ ] **Step 2: Inspect git status**

Run: `git status --short`

Expected: only PRD, plan, migration, db queries/generated files, risk files, and compile-fix files are changed.

- [ ] **Step 3: Commit Phase 1**

Run:

```bash
git add docs/PRD-CiteLoop-Multi-Surface-SEO-Growth-Layer.md docs/superpowers/plans/2026-06-23-multi-surface-seo-growth.md internal/migrations/0021_multi_surface_seo_growth.sql internal/db internal/autopilot internal/api internal/geo
git commit -m "feat(seo): add multi-surface growth foundation"
```

Expected: commit succeeds. Do not start Phase 2 unless Phase 1 gate passed.

---

## Post-Phase 1 Planning Gate

Do not execute Phase 2 or Phase 3 from this plan. After Task 5 passes and the Phase 1 commit exists, write the next detailed TDD plan section using the codebase state produced by Phase 1.

The next plan must cover exactly one phase and must include concrete tests, expected red failures, implementation snippets, and verification commands before any production code is changed.

Phase 2 scope to plan after Phase 1:

- Publisher readiness and the non-blog owned page target decision.
- Content action execution metadata through API responses.
- Dashboard portfolio view and review queue enrichment.
- `make sqlc`, `make test`, `make build`, and `make web-build` gate.

Phase 3 scope to plan after Phase 2:

- GEO asset brief extensions.
- Generalized surface inventory API alias.
- Citation gap to `geo_asset_briefs`/`content_actions` flow.
- `make sqlc`, `make test`, `make build`, and `make web-build` gate.

Release scope to plan after Phase 3:

- Final verification: `make sqlc`, `make test`, `make vet`, `make build`, `make web-build`.
- Push branch.
- Create PR to `origin/main`.
- Merge PR.
- Wait for deployment.
- Verify production behavior.

## Self-Review

- Spec coverage for this executable plan: Phase 1 maps PRD §8 and §9 foundation and creates the schema/API substrate required by later phases.
- Later phases are intentionally gated, not executable placeholders. Each later phase must get its own detailed TDD plan after the prior phase passes.
- No parallel tables: Plan explicitly forbids `seo_actions`, `action_portfolios`, and `distribution_variants`.
- Phase gating: Phase 1 has verification commands and a commit boundary.
