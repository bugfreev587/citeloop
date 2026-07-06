# Dev.to Connection Instructions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add English platform connection instructions to Publisher settings and implement the first non-GitHub connector path for Dev.to API keys.

**Architecture:** Keep GitHub/Next.js as the primary canonical publisher and add Dev.to as a project-scoped publisher connection with an encrypted API key credential. The UI remains in Settings, with concise instructions per platform and a minimal Dev.to API key form that can save, test, enable, disable, and revoke the credential. Real Dev.to article posting remains a later publisher-client task; this change establishes the secure connection and verification path.

**Tech Stack:** Go API with sqlc/Postgres migrations, encrypted credentials via `secretbox`, Next.js/React settings UI, Node test contracts, Go unit tests.

---

### Task 1: Publisher Constants And Migration

**Files:**
- Modify: `internal/publisher/connections.go`
- Modify: `internal/publisher/connections_test.go`
- Add: `internal/migrations/0039_devto_publisher_connection.sql`

- [ ] **Step 1: Write the failing publisher tests**

Add tests that assert Dev.to has a `dev_to` connection kind, `dev_to_api_key` credential kind, safe draft/publish capabilities, and redaction that preserves only the key tail.

- [ ] **Step 2: Run publisher tests to verify they fail**

Run: `go test -count=1 ./internal/publisher`

- [ ] **Step 3: Implement constants and capabilities**

Add Dev.to constants, `DevToCapabilities()`, and `devto_****tail` redaction.

- [ ] **Step 4: Add migration**

Add a migration that drops and recreates `publisher_connections.kind` and `publisher_credentials.kind` check constraints with `dev_to` and `dev_to_api_key` included.

- [ ] **Step 5: Run publisher tests to verify they pass**

Run: `go test -count=1 ./internal/publisher`

### Task 2: Dev.to API Connection Backend

**Files:**
- Modify: `internal/api/server.go`
- Modify: `internal/api/handlers_publisher_connections.go`
- Modify: `internal/api/publisher_connections_routes_test.go`

- [ ] **Step 1: Write the failing API tests**

Cover route registration for `PUT /publisher-connections/dev-to`, Dev.to default upsert config, credential kind acceptance only for Dev.to connections, and unsupported credential kind rejection for GitHub.

- [ ] **Step 2: Run API tests to verify they fail**

Run: `go test -count=1 ./internal/api -run 'TestPublisher|TestDevTo'`

- [ ] **Step 3: Implement backend route and handlers**

Add `upsertDevToPublisherConnection`, allow `dev_to_api_key` credentials only on Dev.to connections, revoke the matching credential kind, and test Dev.to by making an authenticated request to `https://dev.to/api/users/me` with `api-key`.

- [ ] **Step 4: Run API tests to verify they pass**

Run: `go test -count=1 ./internal/api -run 'TestPublisher|TestDevTo'`

### Task 3: Web API Contract

**Files:**
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/api.test.mjs`

- [ ] **Step 1: Write the failing web API tests**

Assert the client exposes `upsertDevToPublisherConnection`, allows `dev_to_api_key` credentials, and posts to `/publisher-connections/dev-to`.

- [ ] **Step 2: Run web API tests to verify they fail**

Run: `npm test -- app/lib/api.test.mjs`

- [ ] **Step 3: Implement API client changes**

Add `DevToPublisherInput`, widen `PublisherCredentialInput`, normalize Dev.to config fields, and add the upsert method.

- [ ] **Step 4: Run web API tests to verify they pass**

Run: `npm test -- app/lib/api.test.mjs`

### Task 4: Settings UI Instructions And Dev.to Form

**Files:**
- Modify: `web/app/projects/[id]/settings/settings-client.tsx`
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Write the failing UI contract tests**

Assert Settings includes English `How to connect` steps for GitHub, Dev.to, Hashnode, LinkedIn, Reddit, Hacker News, and Medium; assert Dev.to has an API key form and does not pretend other roadmap connectors are live.

- [ ] **Step 2: Run UI contract tests to verify they fail**

Run: `npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 3: Implement UI**

Add a concise instruction component, keep GitHub controls intact, add Dev.to card with API key input/test/enable/revoke actions, and show roadmap/manual instructions for other platforms.

- [ ] **Step 4: Run UI contract tests to verify they pass**

Run: `npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs`

### Task 5: Full Verification And Release

**Files:**
- All modified files

- [ ] **Step 1: Run full checks**

Run: `go test ./...`, `npm test`, `npm run typecheck`, and `npm run build`.

- [ ] **Step 2: Commit and push branch**

Commit the implementation, push `codex/devto-connection-instructions`, open a PR to `origin/main`, merge it, and verify `origin/main` contains the merge.

- [ ] **Step 3: Production verification**

Wait for deployment to finish, then verify the production Settings publisher page shows English connection instructions and the Dev.to API key flow.
