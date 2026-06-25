# TokenGate Role Models Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make CiteLoop's global Admin LLM configuration TokenGate-only with role-specific model routing for writer and QA requests.

**Architecture:** Keep one global credential row and one runtime provider. Persist TokenGate base URL plus default/writer/QA model IDs, add an LLM request purpose, and choose the model inside the Admin runtime provider before calling the OpenAI-compatible TokenGate client.

**Tech Stack:** Go backend with pgx migrations and table-driven unit tests; Next.js Admin client with Node contract tests.

---

### Task 1: Backend Credential Model And Routing

**Files:**
- Modify: `internal/llm/llm.go`
- Modify: `internal/admin/credentials.go`
- Modify: `internal/admin/credentials_test.go`
- Create: `internal/migrations/0018_tokengate_role_models.sql`

- [ ] **Step 1: Write failing backend tests**

Add tests in `internal/admin/credentials_test.go` asserting that Admin updates save TokenGate provider with `DefaultModel`, `WriterModel`, and `QAModel`, preserve existing models when blank, and route `llm.PurposeWriter` / `llm.PurposeQA` to different `*llm.OpenAIChat.Model` values.

- [ ] **Step 2: Run backend tests to verify failure**

Run: `go test ./internal/admin ./internal/llm`

Expected: FAIL because the model fields and request purpose do not exist yet.

- [ ] **Step 3: Implement minimal backend support**

Add `CompletionPurpose` constants to `llm.CompletionReq`, model fields to Admin credentials/status/update input, SQL migration columns, and purpose-based selection in `ProviderFromCredentials`.

- [ ] **Step 4: Mark writer and QA requests**

Update writer completions to use `llm.PurposeWriter` and QA completions to use `llm.PurposeQA`; leave Insight and Strategist on the default purpose.

- [ ] **Step 5: Run backend tests**

Run: `go test ./internal/admin ./internal/agents ./cmd/api ./internal/config ./internal/llm`

Expected: PASS.

### Task 2: TokenGate-Only Admin UI

**Files:**
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/api.test.mjs`
- Modify: `web/app/projects/[id]/admin/admin-client.tsx`
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Write failing frontend tests**

Update API tests to expect model fields in normalized status and update payloads. Add source-contract assertions that Admin UI no longer renders OpenAI or Claude provider options and does render default/writer/QA model inputs.

- [ ] **Step 2: Run frontend tests to verify failure**

Run: `npm test -- app/lib/api.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: FAIL because the Admin UI and API types still expose provider switching and do not handle model fields.

- [ ] **Step 3: Implement UI/API changes**

Make `LLMProvider` TokenGate-only in practice, extend credential status/update types, send model fields in `updateLLMCredentials`, and simplify Admin client to one TokenGate panel with base URL and three model inputs.

- [ ] **Step 4: Run frontend tests**

Run: `npm test -- app/lib/api.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: PASS.

### Task 3: Full Verification

**Files:**
- No new source files beyond Tasks 1 and 2.

- [ ] **Step 1: Run full backend tests**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 2: Run full frontend tests**

Run: `npm test`

Expected: PASS.

- [ ] **Step 3: Run production build**

Run: `npm run build`

Expected: PASS.
