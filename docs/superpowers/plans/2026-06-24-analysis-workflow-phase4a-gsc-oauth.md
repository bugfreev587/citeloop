# Analysis Workflow Phase 4A GSC OAuth Onboarding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add self-serve Google Search Console OAuth onboarding so project owners can connect, select, and revoke a GSC property without hitting admin-only settings.

**Architecture:** Use a frontend callback page that calls authenticated project-scoped API routes, so Google redirects do not need Clerk bearer headers. Protect OAuth with signed state containing project ID, owner ID, redirect URI, nonce, and expiry. Store refresh tokens encrypted in a new `seo_oauth_tokens` table; expose only sanitized connection/property metadata to the web app.

**Tech Stack:** Go + Chi + sqlc + pgx, `golang.org/x/oauth2`, Google Search Console API `webmasters.readonly`, Next.js App Router, existing React client API wrapper.

---

## File Structure

- Modify: `internal/config/config.go`, `internal/config/config_test.go`
  - Add Google OAuth client and public app URL envs.
- Create: `internal/migrations/0023_gsc_oauth.sql`
  - Adds encrypted OAuth token storage for SEO providers.
- Modify: `internal/db/queries/seo.sql`, generated `internal/db/seo.sql.go`, `internal/db/models.go`, `internal/db/querier.go`
  - Adds token upsert/read/revoke queries.
- Create: `internal/api/gsc_oauth_state.go`, `internal/api/gsc_oauth_state_test.go`
  - Signed OAuth state generation and validation.
- Create: `internal/googledata/oauth.go`, update `internal/googledata/client.go`
  - OAuth config, Search Console sites listing, and token-backed client helpers.
- Create: `internal/api/handlers_gsc_oauth.go`
  - GSC connection status, OAuth start/complete, property selection, and revoke handlers.
- Modify: `internal/api/server.go`, `internal/api/seo_routes_test.go`
  - Register project-scoped GSC routes.
- Modify: `web/app/lib/api.ts`, `web/app/lib/api.test.mjs`
  - Add typed GSC connection methods.
- Create: `web/app/projects/[id]/settings/gsc/callback/page.tsx`, `web/app/projects/[id]/settings/gsc/callback/gsc-callback-client.tsx`
  - Authenticated frontend callback that completes OAuth and returns users to Settings.
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`, `web/app/projects/[id]/workspace.tsx`, `web/app/projects/[id]/settings/settings-client.tsx`
  - Add Connect Search Console CTAs and property selection UI.
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
  - Contract-test Home / Analysis / Settings GSC OAuth entry points.

## Task 1: Write Phase 4A Contract Tests

**Files:**
- Modify: `internal/api/seo_routes_test.go`
- Modify: `web/app/lib/api.test.mjs`
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [x] **Step 1: Add backend route registration expectations**

In `TestSEORoutesAreRegistered`, add:

```go
{name: "gsc connection", method: http.MethodGet, path: "/api/projects/not-a-uuid/seo/gsc/connection"},
{name: "gsc oauth start", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/gsc/oauth/start"},
{name: "gsc oauth complete", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/gsc/oauth/complete"},
{name: "gsc property select", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/gsc/property"},
{name: "gsc revoke", method: http.MethodPost, path: "/api/projects/not-a-uuid/seo/gsc/revoke"},
```

- [x] **Step 2: Add frontend API expectations**

In `web/app/lib/api.test.mjs`, call:

```js
await client.getGSCConnection("project-1");
await client.startGSCOAuth("project-1", { redirect_uri: "https://app.example.test/projects/project-1/settings/gsc/callback" });
await client.completeGSCOAuth("project-1", { code: "code-1", state: "state-1" });
await client.selectGSCProperty("project-1", { site_url: "sc-domain:unipost.dev" });
await client.revokeGSCConnection("project-1");
```

Assert the URLs are:

```text
/seo/gsc/connection
/seo/gsc/oauth/start
/seo/gsc/oauth/complete
/seo/gsc/property
/seo/gsc/revoke
```

- [x] **Step 3: Add UI contract expectations**

Add a dashboard contract test that asserts:

```js
for (const [file, copies] of [
  ["projects/[id]/workspace.tsx", ["Connect Search Console", "first-party search data"]],
  ["projects/[id]/seo/seo-client.tsx", ["Connect Search Console", "Search Console property", "Select property"]],
  ["projects/[id]/settings/settings-client.tsx", ["Search Console connection", "Connect Search Console", "Authorized properties"]],
  ["projects/[id]/settings/gsc/callback/gsc-callback-client.tsx", ["Finishing Search Console connection", "Return to Settings"]],
]) {
  const source = read(file);
  for (const copy of copies) assert.match(source, new RegExp(copy));
}
```

- [x] **Step 4: Verify RED**

Run:

```bash
npm test -- app/lib/api.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs
make test
```

Expected: FAIL because routes, API methods, callback page, and UI copies do not exist.

## Task 2: Add Env, Signed State, and OAuth Helpers

**Files:**
- Modify: `internal/config/config.go`, `internal/config/config_test.go`
- Create: `internal/api/gsc_oauth_state.go`, `internal/api/gsc_oauth_state_test.go`
- Create: `internal/googledata/oauth.go`
- Modify: `internal/googledata/client.go`

- [x] **Step 1: Add env fields**

Add to `config.Env`:

```go
GoogleOAuthClientID     string
GoogleOAuthClientSecret string
PublicAppURL            string
```

Read from `GOOGLE_OAUTH_CLIENT_ID`, `GOOGLE_OAUTH_CLIENT_SECRET`, and `PUBLIC_APP_URL`.

- [x] **Step 2: Test env parsing**

Add `TestFromEnvReadsGoogleOAuthConfig` in `internal/config/config_test.go`.

- [x] **Step 3: Add signed OAuth state helper**

Implement HMAC-SHA256 signed state with project ID, owner ID, redirect URI, nonce, and expiry. Add tests that valid state parses, tampered payload fails, wrong owner fails, and expired state fails.

- [x] **Step 4: Add Google OAuth helpers**

Implement:

```go
const ScopeSearchConsoleReadonly = "https://www.googleapis.com/auth/webmasters.readonly"
func SearchConsoleOAuthConfig(clientID, clientSecret, redirectURI string) *oauth2.Config
func (c Client) ListSearchConsoleSites(ctx context.Context) ([]SearchConsoleSite, error)
func NewSearchConsoleOAuthClient(ctx context.Context, clientID, clientSecret, redirectURI string, token *oauth2.Token) Client
```

- [x] **Step 5: Verify helper tests**

Run:

```bash
go test ./internal/config ./internal/api ./internal/googledata
```

Expected: PASS.

## Task 3: Add OAuth Token Storage

**Files:**
- Create: `internal/migrations/0023_gsc_oauth.sql`
- Modify: `internal/db/queries/seo.sql`
- Generated: `internal/db/seo.sql.go`, `internal/db/models.go`, `internal/db/querier.go`

- [x] **Step 1: Create migration**

Create `seo_oauth_tokens` with encrypted refresh token, account metadata, selected property, authorized properties, expiry, revoked state, and last error.

- [x] **Step 2: Add sqlc queries**

Add queries:

```sql
-- name: UpsertSEOOAuthToken :one
-- name: GetActiveSEOOAuthToken :one
-- name: UpdateSEOOAuthSelectedProperty :one
-- name: RevokeSEOOAuthToken :one
```

- [x] **Step 3: Generate sqlc**

Run:

```bash
make sqlc
```

Expected: generated db code updates successfully.

## Task 4: Add GSC OAuth API Routes

**Files:**
- Create: `internal/api/handlers_gsc_oauth.go`
- Modify: `internal/api/server.go`

- [x] **Step 1: Register routes under `/seo/gsc`**

Add:

```go
r.Get("/gsc/connection", s.getGSCConnection)
r.Post("/gsc/oauth/start", s.startGSCOAuth)
r.Post("/gsc/oauth/complete", s.completeGSCOAuth)
r.Post("/gsc/property", s.selectGSCProperty)
r.Post("/gsc/revoke", s.revokeGSCConnection)
```

- [x] **Step 2: Implement sanitized connection DTO**

Return configured/status/selected property/recommended property/authorized properties/connectable errors, never encrypted tokens or refresh tokens.

- [x] **Step 3: Implement start**

Validate Google OAuth config, signed state, and `redirect_uri`; return `authorization_url` with `access_type=offline`, `prompt=consent`, and `webmasters.readonly`.

- [x] **Step 4: Implement complete**

Validate signed state against current owner/project, exchange code, list Search Console sites, encrypt refresh token with `secretbox`, upsert token, and upsert GSC integration as `property_selection_required`.

- [x] **Step 5: Implement property select**

Validate selected property exists in authorized properties, save `seo_properties.gsc_site_url`, set `seo_oauth_tokens.selected_property`, and set GSC integration `connected`.

- [x] **Step 6: Implement revoke**

Mark token revoked and GSC integration `revoked`.

- [x] **Step 7: Verify backend**

Run:

```bash
make test
```

Expected: PASS.

## Task 5: Add Frontend API and UI Entry Points

**Files:**
- Modify: `web/app/lib/api.ts`, `web/app/lib/api.test.mjs`
- Create: `web/app/projects/[id]/settings/gsc/callback/page.tsx`
- Create: `web/app/projects/[id]/settings/gsc/callback/gsc-callback-client.tsx`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Modify: `web/app/projects/[id]/workspace.tsx`
- Modify: `web/app/projects/[id]/settings/settings-client.tsx`

- [x] **Step 1: Add API types and methods**

Add `GSCConnection`, `GSCProperty`, `startGSCOAuth`, `completeGSCOAuth`, `selectGSCProperty`, and `revokeGSCConnection`.

- [x] **Step 2: Add callback page**

The callback page reads `code`, `state`, and `error`. On code+state, call `completeGSCOAuth`, then link back to Settings. On error, show denied-consent recovery copy.

- [x] **Step 3: Add Analysis CTA**

Replace the admin-only settings link in the Analysis search-data card with a button that starts GSC OAuth using the callback page URL.

- [x] **Step 4: Add Settings connection panel**

Show status, connect/reconnect, authorized properties, recommended property, select button, revoke button, and missing-config/error states.

- [x] **Step 5: Add Home gate**

When first-party search data is missing, show a compact `Connect Search Console` gate with one button.

- [x] **Step 6: Verify frontend**

Run:

```bash
npm test -- app/lib/api.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs
npm run typecheck
```

Expected: PASS.

## Task 6: Full Verification and Commit

**Files:**
- Modified and generated files from Tasks 1-5.

- [ ] **Step 1: Run checks**

Run:

```bash
npm test
npm run typecheck
make test
VERCEL_ENV=preview npm run build
git diff --check
```

Expected: all commands exit 0. Preview build may keep the existing multi-lockfile warning.

- [ ] **Step 2: Commit Phase 4A**

Run:

```bash
git add .
git commit -m "feat: add gsc oauth onboarding"
```

Expected: commit succeeds.

## Self-Review

- Spec coverage: Covers Phase 4 self-serve OAuth entry points, recoverable denied/missing-property/revoked states, property recommendation/selection, encrypted token storage, and non-admin-safe CTAs. It does not backfill GSC metrics; that belongs to Phase 5.
- Placeholder scan: No placeholders remain.
- Type consistency: Frontend DTO names match backend JSON fields; backend statuses align with `google_search_console` integration status.
