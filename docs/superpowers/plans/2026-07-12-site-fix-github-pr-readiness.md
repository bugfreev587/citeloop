# Site Fix GitHub PR Readiness and Automatic PR Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Gate Site Fix approval on a persisted GitHub PR readiness status, create a repository-grounded pull request from one approval click for every Site Fix family, and show the PR and accurate approval/deploy/verification progress in the drawer.

**Architecture:** Settings owns live GitHub permission checks and persists a redacted PR-readiness state; Site Fixes only reads that state. The canonical Site Fix apply service loads a bounded snapshot of existing repository files, asks the existing Site Fix model for exact replacements, independently grounds the actual change, and hands an atomic multi-file commit to the GitHub publisher. Approval orchestrates apply/PR creation, while existing scheduler reconciliation continues to own merge, deployment, and verification transitions.

**Tech Stack:** Go 1.25, PostgreSQL/sqlc, GitHub REST API, Next.js/TypeScript, Node test runner

---

### Task 1: Persist one GitHub PR readiness state

**Files:**
- Create: `internal/migrations/0084_github_pr_readiness.sql`
- Create: `internal/db/github_pr_readiness_contract_test.go`
- Modify: `internal/db/queries/publisher_connections.sql`
- Modify (generated): `internal/db/models.go`
- Modify (generated): `internal/db/publisher_connections.sql.go`
- Modify (generated): `internal/db/querier.go`

- [ ] **Step 1: Write the failing migration/query contract test**

Add tests that require the migration to add `pr_readiness_status`, `pr_readiness_checked_at`, and `pr_readiness_detail`, constrain the status values, backfill connected GitHub rows as `not_checked`, and require named queries `GetGitHubPRReadinessForProject` and `SetGitHubPRReadinessIfUnchanged`. The contract must also prove readiness invalidation occurs atomically inside every target, credential, installation/config, and enabled-state mutation.

```go
func TestGitHubPRReadinessSchemaAndQueries(t *testing.T) {
	migration := readContractFile(t, "../migrations/0084_github_pr_readiness.sql")
	queries := readContractFile(t, "queries/publisher_connections.sql")
	for _, token := range []string{"pr_readiness_status", "pr_readiness_checked_at", "pr_readiness_detail", "permission_missing", "repository_unavailable", "not_checked"} {
		if !strings.Contains(migration, token) { t.Fatalf("migration missing %q", token) }
	}
	for _, name := range []string{"GetGitHubPRReadinessForProject", "SetGitHubPRReadinessIfUnchanged"} {
		if !strings.Contains(queries, "-- name: "+name) { t.Fatalf("queries missing %s", name) }
	}
}
```

- [ ] **Step 2: Run the contract test and verify RED**

Run: `go test ./internal/db -run TestGitHubPRReadinessSchemaAndQueries -count=1`

Expected: FAIL because the migration and queries do not exist.

- [ ] **Step 3: Add migration and query semantics**

Use this durable status contract:

```sql
alter table publisher_connections
  add column if not exists pr_readiness_status text not null default 'not_connected',
  add column if not exists pr_readiness_checked_at timestamptz,
  add column if not exists pr_readiness_detail text;

alter table publisher_connections
  add constraint publisher_connections_pr_readiness_status_check
  check (pr_readiness_status in ('not_connected','not_checked','ready','permission_missing','repository_unavailable','error')) not valid;

update publisher_connections
set pr_readiness_status = case
  when kind = 'github_nextjs' and status = 'connected' and enabled then 'not_checked'
  else 'not_connected'
end;
```

`SetGitHubPRReadinessIfUnchanged` sets all three readiness fields only when the connection ID and the check's captured `updated_at` still match. A no-row result means a concurrent target or credential mutation won and its invalidation must remain intact. Update connection upsert, credential, repository/branch/installation selection, revoke, and enabled-state queries so each material change atomically clears the timestamp/detail and derives `not_connected` versus `not_checked` inside the same SQL statement.

- [ ] **Step 4: Regenerate sqlc and verify GREEN**

Run:

```bash
make sqlc
gofmt -w internal/db/github_pr_readiness_contract_test.go
go test ./internal/db -run 'TestGitHubPRReadinessSchemaAndQueries|TestPublisher' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit readiness persistence**

```bash
git add internal/migrations/0084_github_pr_readiness.sql internal/db/queries/publisher_connections.sql internal/db/models.go internal/db/publisher_connections.sql.go internal/db/querier.go internal/db/github_pr_readiness_contract_test.go
git commit -m "feat: persist GitHub PR readiness"
```

### Task 2: Check and expose GitHub PR permissions from Settings

**Files:**
- Modify: `internal/githubapp/githubapp.go`
- Modify: `internal/githubapp/githubapp_test.go`
- Create: `internal/publisher/github_readiness.go`
- Create: `internal/publisher/github_readiness_test.go`
- Create: `internal/api/handlers_github_readiness.go`
- Modify: `internal/api/handlers_github_integration.go`
- Modify: `internal/api/handlers_publisher_connections.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/publisher_connections_routes_test.go`

- [ ] **Step 1: Write failing permission and route tests**

Add a GitHub App response test that decodes installation token permissions and a publisher probe test for exact write requirements:

```go
func TestGitHubPRPermissionsRequireContentsAndPullRequestsWrite(t *testing.T) {
	for _, tc := range []struct {
		permissions map[string]string
		ready bool
	}{
		{map[string]string{"contents":"write", "pull_requests":"write"}, true},
		{map[string]string{"contents":"read", "pull_requests":"write"}, false},
		{map[string]string{"contents":"write", "pull_requests":"read"}, false},
	} {
		if got := HasGitHubPRWritePermissions(tc.permissions); got != tc.ready { t.Fatalf("got %v", got) }
	}
}
```

Extend the API route test with GET and POST `/integrations/github/pr-readiness` and `/integrations/github/pr-readiness/check`. The GET handler test must prove no GitHub client method is invoked; the POST test must persist a redacted status. Add a stale-check test proving a slow successful probe cannot overwrite a newer connection invalidation with `ready`.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```bash
go test ./internal/githubapp ./internal/publisher ./internal/api -run 'GitHub.*Readiness|GitHubPRPermissions|PublisherConnectionRoutes' -count=1
```

Expected: FAIL because installation access metadata, readiness probes, and routes are absent.

- [ ] **Step 3: Return installation permission metadata without breaking token callers**

Add:

```go
type InstallationAccess struct {
	Token       string            `json:"-"`
	Permissions map[string]string `json:"permissions"`
}

func (s *Service) InstallationAccess(ctx context.Context, installationID string) (InstallationAccess, error)
```

Decode both `token` and `permissions` from the access-token response. Keep `InstallationToken` as a compatibility wrapper returning `access.Token`. Extend `githubAppAPI` with `InstallationAccess` and update its test fake.

- [ ] **Step 4: Implement the live readiness checker**

Define a redacted domain result:

```go
type GitHubPRReadiness struct {
	Status    string     `json:"status"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	Detail    string     `json:"detail,omitempty"`
	Repo      string     `json:"repo,omitempty"`
	Branch    string     `json:"branch,omitempty"`
}
```

The checker must classify missing/disabled connections as `not_connected`; incomplete config as `repository_unavailable`; missing App permissions as `permission_missing`; and only return `ready` after the exact selected repository and configured base ref can be read. Do not use a paginated installation-repository list as access proof. For advanced tokens, require provable repository push/write authority; ambiguous scopes or response permission headers return `permission_missing` with guidance to connect the GitHub App.

- [ ] **Step 5: Implement read-only GET and Settings-owned POST handlers**

The GET reads the default GitHub connection and maps its stored readiness fields without network access, synthesizing `not_connected` when no row exists. The POST captures the connection identity/version, calls the live checker, persists through compare-and-set, and returns only redacted fields. If compare-and-set loses to a concurrent mutation, reload and return the newer stored status rather than the stale probe result. Wire both routes under the existing owner-scoped project router. Reset readiness atomically during installation/repository/branch/credential/enable mutations. Add a small checker seam on `Server` so handlers can be exercised without a giant database or GitHub fake.

- [ ] **Step 6: Verify GREEN and commit**

Run:

```bash
gofmt -w internal/githubapp/githubapp.go internal/githubapp/githubapp_test.go internal/publisher/github_readiness.go internal/publisher/github_readiness_test.go internal/api/handlers_github_readiness.go internal/api/handlers_github_integration.go internal/api/handlers_publisher_connections.go internal/api/server.go internal/api/publisher_connections_routes_test.go
go test ./internal/githubapp ./internal/publisher ./internal/api -run 'GitHub.*Readiness|GitHubPRPermissions|PublisherConnectionRoutes' -count=1
git add internal/githubapp internal/publisher/github_readiness.go internal/publisher/github_readiness_test.go internal/api/handlers_github_readiness.go internal/api/handlers_github_integration.go internal/api/handlers_publisher_connections.go internal/api/server.go internal/api/publisher_connections_routes_test.go
git commit -m "feat: check GitHub repair PR readiness"
```

Expected: tests PASS and no raw credential or GitHub error body appears in JSON.

### Task 3: Add Web readiness APIs and Settings status

**Files:**
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/api.test.mjs`
- Modify: `web/app/projects/[id]/settings/settings-client.tsx`
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Write failing Web API and Settings contracts**

Add an API test that expects one GET and one POST method:

```js
const stored = await client.getGithubPRReadiness("project-1");
const checked = await client.checkGithubPRReadiness("project-1");
assert.equal(stored.status, "not_checked");
assert.equal(checked.status, "ready");
assert.match(calls[0].url, /integrations\/github\/pr-readiness$/);
assert.equal(calls[1].init.method, "POST");
```

Extend the Settings contract to require `Ready to create repair PRs`, `Check again`, `checkGithubPRReadiness`, and `settings#publisher` ownership.

- [ ] **Step 2: Run tests and verify RED**

Run: `cd web && node --test app/lib/api.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: FAIL because the type, client methods, and Settings card do not exist.

- [ ] **Step 3: Add normalized readiness API methods**

Add the type and normalizer:

```ts
export type GithubPRReadinessStatus = "not_connected" | "not_checked" | "ready" | "permission_missing" | "repository_unavailable" | "error";
export type GithubPRReadiness = { status: GithubPRReadinessStatus; checked_at?: string | null; detail?: string; repo?: string; branch?: string };
```

`getGithubPRReadiness` issues only a GET. `checkGithubPRReadiness` issues the Settings-owned POST. Do not add a composite method that silently runs both.

- [ ] **Step 4: Render and refresh readiness in Settings**

Settings keeps `githubPRReadiness` and `githubReadinessBusy` state. When the Publisher tab becomes active, call the POST once; also call it after connect/reuse/select-repo/save/revoke/enable operations and from `Check again`. Render a compact status block using green only for `ready`, amber for setup states, and red for permission/repository/error states. Include the saved repo, branch, checked time, and safe detail.

- [ ] **Step 5: Verify GREEN and commit**

Run:

```bash
cd web
node --test app/lib/api.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs
npm run typecheck
git add app/lib/api.ts app/lib/api.test.mjs 'app/projects/[id]/settings/settings-client.tsx' app/lib/dashboard-ux-phase1-contract.test.mjs
git commit -m "feat: show GitHub PR readiness in Settings"
```

Expected: tests and typecheck PASS.

### Task 4: Create atomic multi-file GitHub pull requests

**Files:**
- Modify: `internal/publisher/github_pr.go`
- Modify: `internal/publisher/github_pr_test.go`

- [ ] **Step 1: Write failing tree/read/multi-file tests**

Add tests for recursive tree listing, truncated-tree rejection, multiple blob uploads, mode preservation, one derived tree, one commit with the base commit as parent, deterministic ref creation, and PR opening. Assert no ref mutation occurs when any SHA validation/blob/tree/commit preparation call fails. Cover orphan branch/commit reuse after a lost response, rejection of an unknown divergent deterministic branch, and closed-PR reconciliation without relabeling it open.

```go
result, err := client.CreateFileUpdatesPR(ctx, GitHubFileUpdatesPRInput{
	WorkingBranch: "citeloop/doctor-site-fix-abc",
	BaseCommitSHA: "base-commit",
	Files: []GitHubFileUpdate{
		{Path:"app/sitemap.ts", BaseBlobSHA:"old-a", Content:[]byte("new-a")},
		{Path:"app/page.tsx", BaseBlobSHA:"old-b", Content:[]byte("new-b")},
	},
	CommitMessage:"fix: apply CiteLoop Doctor Site Fix", Title:"Apply CiteLoop Doctor Site Fix", Body:"body",
})
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/publisher -run 'GitHubPRClient.*(Tree|FileUpdates|Atomic)' -count=1`

Expected: FAIL because the types and methods are absent.

- [ ] **Step 3: Add bounded repository reads**

Add `ListTree(ctx, ref)` plus blob-SHA-pinned reads, retain `ReadFile` for legacy callers, and expose `GitHubTreeEntry{Path, Mode, Type, SHA, Size}`. Reject truncated trees and non-blob entries before the Site Fix layer ranks candidates.

- [ ] **Step 4: Add the atomic Git-data write sequence**

`CreateFileUpdatesPR` must:

1. reconcile an existing PR by deterministic head branch and preserve its actual open/closed/merged state;
2. resolve the configured base ref to its current commit/tree and verify every file's base blob SHA;
3. POST all blobs;
4. POST one tree with `base_tree`;
5. POST one commit with the base commit parent;
6. create the deterministic ref, or reuse it only when its tree/commit matches the desired change; and
7. open or reuse the PR.

No direct Contents API writes, force updates of an unknown divergent branch, or base-branch mutation are allowed. Keep `CreatePageUpdatePR` intact for legacy callers. Use stable commit metadata or desired-tree comparison so an interrupted request can reconcile an orphan commit/ref without generating a second change.

- [ ] **Step 5: Verify GREEN and commit**

Run:

```bash
gofmt -w internal/publisher/github_pr.go internal/publisher/github_pr_test.go
go test ./internal/publisher -count=1
git add internal/publisher/github_pr.go internal/publisher/github_pr_test.go
git commit -m "feat: create atomic multi-file GitHub PRs"
```

Expected: package tests PASS.

### Task 5: Generate grounded patches from existing repository files

**Files:**
- Create: `internal/sitefix/repository_patch.go`
- Create: `internal/sitefix/repository_patch_test.go`
- Create: `internal/sitefix/repository_source_selector.go`
- Create: `internal/sitefix/repository_source_selector_test.go`
- Modify: `internal/sitefix/apply.go`
- Modify: `internal/sitefix/apply_test.go`
- Modify: `internal/sitefix/grounding_verifier.go`
- Create: `internal/api/site_fix_repository_source.go`
- Modify: `internal/api/handlers_site_fixes.go`
- Modify: `internal/db/queries/site_fixes.sql`
- Modify (generated): `internal/db/site_fixes.sql.go`
- Modify (generated): `internal/db/querier.go`
- Modify: `internal/db/site_fixes_query_contract_test.go`
- Modify: `internal/db/canonical_sitefix_postgres_integration_test.go`

- [ ] **Step 1: Write failing repository patch tests**

Cover path filtering/ranking for sitemap, canonical, schema, internal-link, robots, and metadata findings; truncated-tree rejection; model-selected paths being intersected with the safe repository tree; exact-one and non-overlapping replacement semantics; per-file SHA matching; eight-file/128-KiB/512-KiB input-and-result bounds; invalid UTF-8/NUL/binary content; unchanged/empty patches; and generated/vendor/dependency/secret/lock/workflow exclusions.

```go
patch := RepositoryPatch{Files: []RepositoryFilePatch{{
	Path: "app/sitemap.ts", BaseSHA: "sha-1",
	Replacements: []ExactReplacement{{OldText:"return []", NewText:"return [{ url: canonicalURL }]"}},
}}}
updates, diff, err := ApplyRepositoryPatch(snapshot, patch)
if err != nil { t.Fatal(err) }
if got := string(updates[0].Content); !strings.Contains(got, "canonicalURL") { t.Fatalf("content=%s", got) }
```

- [ ] **Step 2: Run focused tests and verify RED**

Run: `go test ./internal/sitefix -run 'Repository|Application' -count=1`

Expected: FAIL because repository snapshot/patch types and validation are absent.

- [ ] **Step 3: Define the source and patch contracts**

Use these boundaries:

```go
type RepositorySource struct { Path, SHA, Content string }
type RepositorySnapshot struct { Repo, Branch, BaseCommitSHA string; Sources []RepositorySource }
type RepositorySourceLoader interface {
	Candidates(context.Context, db.SiteFix) (RepositoryTarget, []RepositorySourceCandidate, error)
	LoadSelected(context.Context, RepositoryTarget, []string) (RepositorySnapshot, error)
}
type RepositorySourceSelector interface {
	Describe(db.SiteFix, []RepositorySourceCandidate) GenerationCall
	Select(context.Context, db.SiteFix, []RepositorySourceCandidate, siteFixAICallAttempt) ([]string, GenerationResult, error)
}
type ExactReplacement struct { OldText string `json:"old_text"`; NewText string `json:"new_text"` }
type RepositoryFilePatch struct { Path string `json:"path"`; BaseSHA string `json:"base_sha"`; Replacements []ExactReplacement `json:"replacements"` }
type RepositoryPatch struct { Files []RepositoryFilePatch `json:"files"` }
```

`ApplyRepositoryPatch` returns final file contents plus a compact actual-before/after diff for grounding. It rejects paths not present in the snapshot, duplicate paths, ambiguous/missing/overlapping old text, unchanged replacements, invalid text, size overruns, and SHA mismatches.

- [ ] **Step 4: Load a bounded GitHub source snapshot**

`site_fix_repository_source.go` resolves the exact enabled GitHub publisher, configured repository/branch, and token from the final readiness snapshot; it must not override the configured branch from the target URL. It lists the base tree and applies safe text-file filtering and deterministic scoring using target path/finding/proposed-fix terms. `LLMRepositorySourceSelector` receives only bounded safe candidate metadata (paths, sizes, and finding context), returns candidate paths as structured JSON, and is called once per preparation attempt. The loader intersects the response with the safe candidate set, adds only validated source-path hints, then reads at most eight blob-SHA-pinned existing sources within the size budget. Empty, unknown, unsafe, or over-limit selections fail retryably; they never broaden into an unrestricted repository read.

- [ ] **Step 5: Make generation repository-grounded**

Add `SourceLoader` and `SourceSelector` to the canonical preparation path, run the selector, load the resulting snapshot, and pass it to `FixGenerator.Generate`. Change `LLMApplicationGenerator` to include the selected source paths, SHAs, and contents in its prompt and require one JSON `RepositoryPatch`. Validate and apply that patch locally before grounding, persist the structured patch with repo/base commit/per-file blob SHAs in `patch_snapshot`, the actual before/after diff in `diff_snapshot`, and always finalize successful generation as `ready_for_pr`. Source selection and patch generation are separate physical model calls with separate `fix_generation` ledger rows, prompt versions/fingerprints, and a causal link; they must not share one attempt observer.

The user-triggered Apply path must use the repository-backed LLM preparation even when scheduled Doctor AI is disabled: the explicit click authorizes these bounded on-demand calls and does not alter project automation settings. Remove the production fallback to `DeterministicApplicationGenerator`/`manual_apply_required`; provider or safe-patch failure remains retryable in `preparing`.

- [ ] **Step 6: Ground the actual repository change**

Update `LLMPatchGroundingVerifier` input to include the locally computed before/after diff, never a model self-report. Preserve all existing proposition and intent checks and explicitly reject unrelated file edits or unauthorized source-association changes outside the approved Site Fix. Preserve scheduler-consumed top-level resolution metadata such as `asset_type` and proposed-value fields.

- [ ] **Step 7: Persist prepared patches before GitHub effects**

Add `SaveCanonicalSiteFixPreparedPatch`, fenced by project/application/fix ID, active PR claim token, claim expiry, and writer-authority fingerprint. It persists the exact prepared patch and actual diff, including all source paths, repository/base branch/base commit, per-file blob SHAs in JSON, mapping/grounding data, and aggregate hashes. Scalar `source_file_path`/`base_file_sha` remain nullable first-file compatibility fields only. A lost claim cannot overwrite a newer preparation. An interrupted request reuses a persisted patch when its base SHAs still match; an explicit source conflict forces fresh bounded preparation on recovery. Sanitize stored failure codes/details instead of persisting raw `error.Error()` strings.

- [ ] **Step 8: Verify GREEN and commit**

Run:

```bash
make sqlc
gofmt -w internal/sitefix/repository_patch.go internal/sitefix/repository_patch_test.go internal/sitefix/repository_source_selector.go internal/sitefix/repository_source_selector_test.go internal/sitefix/apply.go internal/sitefix/apply_test.go internal/sitefix/grounding_verifier.go internal/api/site_fix_repository_source.go internal/api/handlers_site_fixes.go internal/db/site_fixes_query_contract_test.go internal/db/canonical_sitefix_postgres_integration_test.go
go test ./internal/sitefix ./internal/api ./internal/db -run 'Repository|CanonicalGitHub|DoctorAIProviderAuthority|PreparedPatch|Apply' -count=1
git add internal/sitefix internal/api/site_fix_repository_source.go internal/api/handlers_site_fixes.go internal/db/queries/site_fixes.sql internal/db/site_fixes.sql.go internal/db/querier.go internal/db/site_fixes_query_contract_test.go internal/db/canonical_sitefix_postgres_integration_test.go
git commit -m "feat: ground Site Fixes in repository source"
```

Expected: focused tests PASS and no successful canonical application uses `manual_apply_required`.

### Task 6: Combine approval, PR creation, and readiness enforcement

**Files:**
- Modify: `internal/api/handlers_site_fixes.go`
- Modify: `internal/api/site_fixes_api_test.go`
- Modify: `internal/api/handlers_github_readiness.go`

- [ ] **Step 1: Write failing orchestration tests**

Add handler tests for:

- non-`ready` persisted status returns 409 without a live check or Approve;
- persisted-ready but live-downgraded status is atomically saved, returns 409, and never calls Approve;
- ready proposed fix orders final live check before durable approval, then calls Apply and PR creation once and returns `SiteFixLifecycleResult` with `github_pr_url`;
- Apply failure after approval preserves the approved record and is retryable;
- an already-approved historical fix can call the apply recovery endpoint;
- sitemap/canonical/schema/internal-link/robots mutations are not rejected by a metadata-family switch; and
- permission/repository GitHub errors persist the corresponding readiness state;
- a persisted prepared patch is reused after a lost GitHub response, while a source-SHA conflict forces fresh preparation; and
- configured repository/branch authority is used consistently, with no target-URL branch override.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `go test ./internal/api -run 'ApproveDoctorSiteFix|CanonicalGitHubApply|Readiness' -count=1`

Expected: FAIL because approval returns only a Site Fix and metadata-only gating remains.

- [ ] **Step 3: Extract one idempotent application orchestrator**

Create an internal method used by both approve and legacy recovery apply routes:

```go
func (s *Server) createCanonicalSiteFixPR(ctx context.Context, projectID, fixID uuid.UUID) (sitefix.ApplyResult, error)
```

It first requires stored `ready`, then checks live readiness and retains the exact checked connection/repository/branch snapshot for the entire operation. It applies the canonical Site Fix, reopens retryable applications when allowed, parses or freshly prepares the persisted `RepositoryPatch`, re-reads/validates base SHAs immediately before mutation, calls `CreateFileUpdatesPR`, and persists the PR identity. Generate the deterministic branch suffix from 12 hexadecimal UUID characters after removing hyphens. Delete `canonicalSiteFixPRMutationFamily`, target-URL branch overrides, single-file/pseudo-content-action metadata adapters, and the manual-handoff fallback from the success path.

- [ ] **Step 4: Make Approve a single user operation**

`approveDoctorSiteFix` checks stored and live readiness before mutation, persists approval idempotently for the same revision, then calls `createCanonicalSiteFixPR` and returns the lifecycle result. If generation fails after approval, return a controlled safe error; the next list refresh exposes `approved`/`preparing` plus its safe failure reason. Keep `/apply` only as the historical/retry recovery endpoint and apply the same live readiness boundary there. Preserve the real state of reconciled closed PRs and never mark them `github_pr_open`.

- [ ] **Step 5: Verify GREEN and commit**

Run:

```bash
gofmt -w internal/api/handlers_site_fixes.go internal/api/site_fixes_api_test.go internal/api/handlers_github_readiness.go
go test ./internal/api ./internal/sitefix ./internal/publisher -count=1
git add internal/api/handlers_site_fixes.go internal/api/site_fixes_api_test.go internal/api/handlers_github_readiness.go
git commit -m "feat: create a Site Fix PR on approval"
```

Expected: tests PASS.

### Task 7: Render readiness, Open PR, polling, and correct milestones

**Files:**
- Modify: `web/app/lib/types.ts`
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/api.test.mjs`
- Modify: `web/app/lib/site-fix.ts`
- Create: `web/app/lib/site-fix-pr-readiness-logic.test.mjs`
- Modify: `web/app/lib/action-portfolio-contract.test.mjs`
- Modify: `web/app/lib/seo-client-contract.test.mjs`
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
- Modify: `web/app/projects/[id]/site-fixes/site-fixes-client.tsx`

- [ ] **Step 1: Write failing lifecycle and drawer tests**

Require:

- the initial/manual Site Fix refresh requests `listDoctorSiteFixes` and `getGithubPRReadiness`, but background polling requests only `listDoctorSiteFixes` and never `checkGithubPRReadiness` or `listPublisherConnections`;
- not-ready approval has a focus/hover warning and `/settings#publisher` link;
- `approveDoctorSiteFix` returns/replaces both `site_fix` and `application`;
- historical `approved` rows expose `Create PR`;
- any `github_pr_url` exposes a primary `Open PR` footer link;
- active drawer statuses install and clean up a polling interval; and
- Approved is complete after approval, Applied/deploy completes only after deployment evidence, and Verified completes only at `verified`.

Add pure table assertions for every readiness status (including null/loading/fetch-error/unknown), action precedence, polling start/stop cases, safe warning copy, and milestone completion rather than testing CSS alone.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```bash
cd web
node --test app/lib/action-portfolio-contract.test.mjs app/lib/seo-client-contract.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs app/lib/api.test.mjs app/lib/site-fix-pr-readiness-logic.test.mjs
```

Expected: FAIL on the new readiness, response, PR action, polling, and milestone contracts.

- [ ] **Step 3: Normalize combined approval responses**

Change `approveDoctorSiteFix` to return `SiteFixLifecycleResult` with the mutation timeout used by `/apply`. Preserve `/apply` for recovery. Expose repository, base/working branch, commit, PR number, URL, and state fields through the application type/normalizer. Add a validated PR URL helper and keep the primary action labeled `Open PR` in every PR-backed state, including merged, closed, and verified history; preserve it when a separate verification/retry action is also available.

- [ ] **Step 4: Add pure lifecycle milestone derivation**

Expose:

```ts
export type SiteFixMilestone = { label: string; complete: boolean; current: boolean };
export function canonicalSiteFixMilestones(fix: SiteFix): SiteFixMilestone[];
```

Derive completion from `approved_at/status`, `deployed_at/application.deployed_at/status`, and `verified_at/status`, not only an integer current index. PR creation and PR merge alone must not complete `Applied / deploy`. Progress copy receives the whole Site Fix/application so `applying` can distinguish PR creation from waiting for PR review.

- [ ] **Step 5: Implement the lightweight readiness gate**

The Site Fix page fetches stored readiness alongside fixes. It never calls the live check. Treat missing, loading, fetch-error, unknown, and every non-`ready` value as blocked. Wrap the disabled Approve control in a hoverable/focusable disclosure that contains the status-specific warning and an interactive Settings link; do not use `role="tooltip"` on the container holding that link. `ready` runs one `approveFix` request with `Approving & creating PR...` progress; historical approved/preparing rows use `Create PR`/`Retry creating PR` through `/apply`. A server readiness rejection reloads both stored readiness and the Site Fix list, and retry copy considers application failure detail as well as `fix.failure_reason`.

- [ ] **Step 6: Add Open PR and active-state polling**

Render the PR action prominently in the drawer footer and Application section, including PR number/state, repository, and working branch. While the selected fix is actively preparing/creating a PR, has an open PR, is awaiting deploy, or is verifying, silently refresh only the existing fix list on a bounded interval (10 seconds) and retain selection/focus. Stop on drawer close, retry-waiting failures, closed/follow-up PRs, and terminal states. Do not poll readiness or run any GitHub check.

- [ ] **Step 7: Verify GREEN and commit**

Run:

```bash
cd web
node --test app/lib/action-portfolio-contract.test.mjs app/lib/seo-client-contract.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs app/lib/api.test.mjs app/lib/site-fix-pr-readiness-logic.test.mjs
npm test
npm run typecheck
git add app/lib/types.ts app/lib/api.ts app/lib/api.test.mjs app/lib/site-fix.ts app/lib/site-fix-pr-readiness-logic.test.mjs app/lib/action-portfolio-contract.test.mjs app/lib/seo-client-contract.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs 'app/projects/[id]/site-fixes/site-fixes-client.tsx'
git commit -m "feat: expose Site Fix pull request progress"
```

Expected: all Web tests and typecheck PASS.

### Task 8: Full verification and release

**Files:**
- Review all modified files
- Update checklist state in this plan

- [ ] **Step 1: Verify formatting, generated code, and focused behavior**

Run:

```bash
git diff --check origin/main...HEAD
make sqlc
git diff --exit-code -- internal/db/models.go internal/db/publisher_connections.sql.go internal/db/site_fixes.sql.go internal/db/querier.go
go test ./internal/db ./internal/githubapp ./internal/publisher ./internal/sitefix ./internal/api -count=1
cd web && node --test app/lib/api.test.mjs app/lib/action-portfolio-contract.test.mjs app/lib/seo-client-contract.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs app/lib/site-fix-pr-readiness-logic.test.mjs
```

Expected: all commands exit 0 and regeneration leaves no diff.

- [ ] **Step 2: Run repository-wide verification**

Run:

```bash
make test
make vet
make build
cd web && npm test && npm run typecheck && npm run build
```

Expected: all Go packages, 355+ Web tests, typecheck, and both production builds pass with zero failures.

- [ ] **Step 3: Review requirements and diff**

Re-read `docs/superpowers/specs/2026-07-12-site-fix-github-pr-readiness-design.md` and verify each requirement against a test or production path. Inspect `git diff origin/main...HEAD`, `git status --short`, and migration ordering. Confirm no secrets, raw GitHub errors, manual-apply success path, or unrelated refactor entered the diff.

- [ ] **Step 4: Push, create PR, and merge**

Push `codex/site-fix-pr-readiness`, create a PR against `origin/main`, wait for all required GitHub checks, and merge without bypassing protections.

- [ ] **Step 5: Wait for both deployments**

Confirm Railway/API and Vercel/Web production deployments both report the merge commit as ready. Do not infer deployment from the PR merge alone.

- [ ] **Step 6: Verify production**

In production:

1. open Settings -> Publisher and confirm the persisted GitHub PR readiness status and `Check again` behavior;
2. open Site Fixes and confirm page load reads the stored status without triggering a live check;
3. use a safe existing proposed or historical approved Site Fix to create a review PR;
4. confirm `Approved` has a checkmark and `Open PR` opens the exact GitHub pull request;
5. confirm the drawer shows `Waiting for PR review and merge` while open; and
6. use existing merged/deployed/verified records, or a dedicated safe test repository, to verify the later milestone labels without merging a customer's repair PR on their behalf.

If production differs, add a failing regression test, fix on the same branch, push, re-merge, wait for deployments, and repeat verification.
