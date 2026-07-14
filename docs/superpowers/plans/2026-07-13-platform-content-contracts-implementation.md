# Platform Content Contracts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace broad channel-only planning and fixed syndication rewrites with exact target plans, immutable platform contracts, native artifact validation, and versioned Reddit target context.

**Architecture:** PostgreSQL owns immutable contract/context versions and exact target plans. `internal/platformcontract` resolves platform, asset, project, and target constraints into structured generation contracts; Writer generates one independently validated artifact per exact target. Existing `topic.channel`, `articles.platform`, and Publisher behavior remain compatibility projections while API/UI move to exact targets.

**Tech Stack:** Go 1.24, PostgreSQL 16, sqlc, Next.js/TypeScript, Node test runner, existing Writer/QA/Publisher interfaces.

---

### Task 1: Persist immutable contracts, target contexts, and exact plans

**Files:**
- Create: `internal/migrations/0085_platform_content_contracts.sql`
- Modify: `internal/db/queries/seo.sql`
- Modify: `internal/db/queries/articles.sql`
- Modify: `internal/db/queries/topics.sql`
- Regenerate: `internal/db/*.sql.go`, `internal/db/models.go`, `internal/db/querier.go`
- Test: `internal/db/platform_contracts_migration_contract_test.go`

- [ ] **Step 1: Write the failing migration contract test**

Assert migration `0085` defines `platform_content_contracts`, `platform_target_contexts`, `content_target_plans`, `content_target_plan_items`, adds `topics.asset_type`, `articles.platform_contract_id`, `articles.platform_contract_version`, `articles.target_context_id`, `articles.output_type`, and seeds immutable contracts for all seven registered platforms.

- [ ] **Step 2: Run the test and verify it fails**

Run: `go test ./internal/db -run PlatformContentContractsMigration -count=1`
Expected: FAIL because migration 0085 is absent.

- [ ] **Step 3: Add the schema and queries**

Use these invariants in SQL:

```sql
create unique index uniq_active_platform_content_contract
  on platform_content_contracts(platform)
  where status = 'active';

create unique index uniq_current_target_context
  on platform_target_contexts(project_id, platform, target_key)
  where status = 'confirmed';

create unique index uniq_target_plan_item
  on content_target_plan_items(plan_id, platform, target_key);
```

Seed `blog`, `dev_to`, `hashnode`, `medium`, `linkedin`, `reddit`, and `hacker_news` with `platform-contract-v1`; add `source_backed_evidence_page` and `faq_answer_block` to `seo_asset_types`; backfill `source_backed_evidence_page` through GEO briefs, linked actions/topics, and article metadata without overwriting conflicts.

- [ ] **Step 4: Generate sqlc and run focused tests**

Run: `sqlc generate`
Expected: generated models expose the new fields and query methods.

Run: `go test ./internal/db -run PlatformContentContractsMigration -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/migrations/0085_platform_content_contracts.sql internal/db
git commit -m "feat: persist platform content contracts"
```

### Task 2: Implement canonical asset keys and capability matrix

**Files:**
- Create: `internal/platformcontract/types.go`
- Create: `internal/platformcontract/registry.go`
- Create: `internal/platformcontract/capability.go`
- Test: `internal/platformcontract/registry_test.go`
- Test: `internal/platformcontract/capability_test.go`
- Modify: `internal/platform/platform.go`

- [ ] **Step 1: Write failing registry and matrix tests**

Cover one active immutable version per platform, aliases `template_checklist → template_or_checklist` and `integration_docs_page → integration_page`, output types, canonical requirements, publishing modes, and incompatible direct actions.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./internal/platformcontract -count=1`
Expected: FAIL because package does not exist.

- [ ] **Step 3: Implement the public contract**

```go
type Capability struct {
    Platform string `json:"platform"`
    ContractID uuid.UUID `json:"contract_id"`
    ContractVersion string `json:"contract_version"`
    GenerationSupported bool `json:"generation_supported"`
    TargetContextReady bool `json:"target_context_ready"`
    ConnectionReady bool `json:"connection_ready"`
    PublishMode string `json:"publish_mode"`
    OutputType string `json:"output_type"`
    BlockReasons []string `json:"block_reasons"`
}

func CanonicalAssetType(value string) (string, bool)
func Matrix(ctx context.Context, q Querier, projectID uuid.UUID, assetType string) ([]Capability, error)
```

Keep `internal/platform` as the legacy capability facade, but stop treating `SyndicationTargets` as future Writer authority.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/platformcontract ./internal/platform -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platformcontract internal/platform/platform.go
git commit -m "feat: add platform capability matrix"
```

### Task 3: Add manual Reddit target-context lifecycle

**Files:**
- Create: `internal/platformcontract/context.go`
- Test: `internal/platformcontract/context_test.go`
- Modify: `internal/api/server.go`
- Create: `internal/api/handlers_platform_contracts.go`
- Test: `internal/api/platform_contracts_routes_test.go`
- Modify: `web/app/lib/api.ts`

- [ ] **Step 1: Write failing lifecycle and route tests**

Test project isolation, normalization of `r/SaaS`, immutable version increments, one confirmed revision, 30-day expiry, unchanged reconfirmation, supersession, contradiction rejection, and API secret-safe responses.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./internal/platformcontract ./internal/api -run 'TargetContext|PlatformContractRoutes' -count=1`
Expected: FAIL with missing lifecycle and routes.

- [ ] **Step 3: Implement lifecycle and authenticated routes**

```go
type ConfirmTargetContextInput struct {
    Platform string `json:"platform"`
    TargetKey string `json:"target_key"`
    SourceURL string `json:"source_url"`
    RulesText string `json:"rules_text"`
    AllowedPostTypes []string `json:"allowed_post_types"`
    RequiredFlair string `json:"required_flair"`
    LinkPolicy string `json:"link_policy"`
    SelfPromotionPolicy string `json:"self_promotion_policy"`
    Verified bool `json:"verified"`
}
```

Expose `GET/POST /projects/{projectID}/platform-target-contexts` and `POST .../{contextID}/reconfirm`. Never claim API retrieval; set `source_kind=user_pasted_rules` or `user_confirmed_rules`.

- [ ] **Step 4: Run focused tests**

Run: `go test ./internal/platformcontract ./internal/api -run 'TargetContext|PlatformContractRoutes' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platformcontract/context.go internal/platformcontract/context_test.go internal/api web/app/lib/api.ts
git commit -m "feat: version platform target context"
```

### Task 4: Persist exact Opportunity target plans

**Files:**
- Create: `internal/platformcontract/plan.go`
- Test: `internal/platformcontract/plan_test.go`
- Modify: `internal/api/handlers_seo.go`
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/api/action_routing_contract_test.go`
- Modify: `internal/scheduler/action_routing_contract_test.go`

- [ ] **Step 1: Write failing plan and handoff tests**

Prove one canonical target is required, targets are ordered/deduplicated, exact platform lists survive Opportunity → Content Action → Topic, incompatible targets fail before generation, `channel` is derived, and legacy derivation records `legacy_derived` provenance.

- [ ] **Step 2: Run focused tests and verify failure**

Run: `go test ./internal/platformcontract ./internal/api ./internal/scheduler -run 'TargetPlan|ActionRouting' -count=1`
Expected: FAIL because target plans are not persisted.

- [ ] **Step 3: Implement target planning**

```go
type PlanInput struct {
    ProjectID uuid.UUID
    OpportunityID uuid.UUID
    AssetType string
    CanonicalTarget Target
    Targets []Target
    SelectionMode string // contract_matrix | legacy_derived
}

func CreatePlan(ctx context.Context, q Querier, input PlanInput) (Plan, error)
func DeriveChannel(plan Plan) string
```

Update both API and scheduler `topicFromContentAction` implementations to set `topics.asset_type` explicitly and link the plan. Remove free-text asset inference from the forward path while preserving legacy rows.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/platformcontract ./internal/api ./internal/scheduler -run 'TargetPlan|ActionRouting' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platformcontract/plan.go internal/platformcontract/plan_test.go internal/api internal/scheduler
git commit -m "feat: plan exact content targets"
```

### Task 5: Resolve native generation contracts and deterministic validation

**Files:**
- Create: `internal/platformcontract/resolve.go`
- Create: `internal/platformcontract/validate.go`
- Create: `internal/platformcontract/contracts_v1.go`
- Test: `internal/platformcontract/validate_test.go`
- Test: `internal/platformcontract/testdata/*.json`
- Modify: `internal/agents/writer.go`
- Modify: `internal/agents/writer_test.go`

- [ ] **Step 1: Write failing platform fixtures**

Fixtures must prove Blog MDX, Dev.to metadata/Markdown, Hashnode publication context, Medium canonical story package, LinkedIn article fields, subreddit-specific Reddit post, and HN link submission have different schemas. Include negative fixtures for aliases, MDX leakage, missing canonical, stale Reddit rules, promotional HN titles, and unresolved placeholders.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./internal/platformcontract ./internal/agents -run 'PlatformContract|ExactTargets|NativeArtifact' -count=1`
Expected: FAIL because Writer still loops over the fixed target list.

- [ ] **Step 3: Implement resolver and validators**

```go
type ResolvedContract struct {
    Platform string
    Version string
    AssetType string
    OutputType string
    Prompt string
    RequiredFields []string
    Rules RuleSet
}

type ValidationReport struct {
    Passed bool `json:"passed"`
    Failures []Failure `json:"failures"`
    Warnings []Failure `json:"warnings"`
}

func Resolve(input ResolveInput) (ResolvedContract, error)
func Validate(contract ResolvedContract, artifact Artifact) ValidationReport
```

Writer loads exact plan items, generates each independently, pins contract/context IDs, validates, repairs at most twice, and persists successful siblings even if one target fails. HN produces a link-submission package and no article body/comment.

- [ ] **Step 4: Run focused tests**

Run: `go test ./internal/platformcontract ./internal/agents -run 'PlatformContract|ExactTargets|NativeArtifact' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/platformcontract internal/agents/writer.go internal/agents/writer_test.go
git commit -m "feat: generate native platform artifacts"
```

### Task 6: Add exact-target selection and Reddit setup UI

**Files:**
- Modify: `web/app/lib/api.ts`
- Create: `web/app/lib/platform-contracts.ts`
- Test: `web/app/lib/platform-contracts.test.mjs`
- Modify: `web/app/projects/[id]/topics/topics-client.tsx`
- Modify: `web/app/projects/[id]/settings/settings-client.tsx`

- [ ] **Step 1: Write failing UI logic tests**

Test capability grouping, exact selection, canonical immutability, incompatible-target explanations, manual/automatic distinction, Reddit context expiry, and derived legacy labels.

- [ ] **Step 2: Run tests and verify failure**

Run: `cd web && node --test app/lib/platform-contracts.test.mjs`
Expected: FAIL because helper module is absent.

- [ ] **Step 3: Implement helpers and UI**

```ts
export type ExactTargetSelection = {
  canonical_target: PlatformTarget;
  target_platforms: PlatformTarget[];
  asset_type: string;
};

export function validateTargetSelection(
  selection: ExactTargetSelection,
  capabilities: PlatformCapability[],
): TargetSelectionValidation;
```

Replace the three-button editor with exact target cards. Keep Blog/Syndication/Both as read-only summary. Add the manual Reddit rules confirmation flow and prevent target selection until context is current.

- [ ] **Step 4: Run web tests and build**

Run: `cd web && npm test`
Expected: PASS.

Run: `cd web && npm run build`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "feat: select exact content platforms"
```

### Task 7: Show contract validation in Review and preserve Publisher behavior

**Files:**
- Modify: `internal/api/handlers_review.go`
- Modify: `web/app/projects/[id]/review/review-client.tsx`
- Create: `web/app/lib/platform-preview.ts`
- Test: `web/app/lib/platform-preview.test.mjs`
- Modify: `internal/publisher/semimanual.go`
- Test: `internal/publisher/semimanual_test.go`

- [ ] **Step 1: Write failing Review and Publisher tests**

Test contract version, native preview, deterministic failures, per-artifact approval, canonical lock, HN link package, and unchanged compose URLs/current blog automatic path.

- [ ] **Step 2: Run tests and verify failure**

Run: `go test ./internal/api ./internal/publisher -run 'ContractPreview|SemiManual' -count=1`
Expected: FAIL on missing contract projection.

Run: `cd web && node --test app/lib/platform-preview.test.mjs`
Expected: FAIL on missing preview helper.

- [ ] **Step 3: Implement Review projection and previews**

Return the pinned contract/context and validation report with each article. Render platform-specific metadata and block approval on deterministic failure; warnings remain reviewer-visible. Keep Publisher delivery separate from generation validity.

- [ ] **Step 4: Run focused and full tests**

Run: `go test ./internal/api ./internal/publisher -count=1`
Expected: PASS.

Run: `cd web && npm test && npm run build`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api internal/publisher web
git commit -m "feat: review platform contract artifacts"
```

### Task 8: Verify migration and platform acceptance end to end

**Files:**
- Modify: `internal/platformcontract/*_test.go`
- Modify: `internal/api/topics_routes_test.go`
- Modify: `docs/superpowers/specs/2026-07-13-platform-content-contracts-design.md`

- [ ] **Step 1: Add end-to-end contract tests**

Cover exact Dev.to+Reddit selection, no implicit Hashnode, Reddit context pinning, HN link-only output, source-backed evidence migration, and legacy draft preservation.

- [ ] **Step 2: Run repository verification**

Run: `go test ./...`
Expected: PASS.

Run: `go vet ./...`
Expected: PASS.

Run: `cd web && npm test && npm run build`
Expected: PASS.

- [ ] **Step 3: Commit acceptance coverage**

```bash
git add internal docs
git commit -m "test: verify platform content contracts"
```
