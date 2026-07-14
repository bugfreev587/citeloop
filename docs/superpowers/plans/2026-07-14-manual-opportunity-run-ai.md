# Manual Opportunity Finding AI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make explicit Opportunity Finding runs execute AI/Growth Radar under `scheduled_only` and expose the policy where users expect it in Automation settings.

**Architecture:** Keep the current trigger-aware scheduler and policy keys. Change only manual authority evaluation, preserve scheduled/event semantics, then move the existing single Opportunities settings card to the Automation panel and align status copy and deep-link ownership.

**Tech Stack:** Go configuration and API status helpers; Next.js/React settings and Analysis clients; Go tests and Node contract tests.

---

### Task 1: Manual trigger authority

**Files:**
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/config.go`

- [ ] **Step 1: Write failing trigger and stage tests**

Change the `scheduled_only` expectations so `GrowthAITriggerManual` is authorized, `GrowthAITriggerScheduled` remains authorized, and `GrowthAITriggerEvent` remains rejected. Assert `OpportunityFindingStagesForTrigger(GrowthAITriggerManual).AIDiscovery` is true.

- [ ] **Step 2: Verify the tests fail for the missing manual authority**

Run: `go test ./internal/config -run 'TestGrowthAIPolicy|TestOpportunityFindingStages'`

Expected: FAIL because `scheduled_only` currently rejects the manual trigger.

- [ ] **Step 3: Add the minimal authority rule**

In `ProjectConfig.AllowsGrowthAI`, include `GrowthAIRunPolicyScheduledOnly` in the policies accepted by `GrowthAITriggerManual`. Do not change scheduled or event cases.

- [ ] **Step 4: Verify the focused tests pass**

Run: `go test ./internal/config -run 'TestGrowthAIPolicy|TestOpportunityFindingStages'`

Expected: PASS.

### Task 2: Settings ownership and user-facing semantics

**Files:**
- Modify: `web/app/lib/ai-authority-settings-contract.test.mjs`
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
- Modify: `web/app/lib/seo-client-contract.test.mjs`
- Modify: `web/app/projects/[id]/settings/settings-client.tsx`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Modify: `internal/api/opportunity_finding_status_contract_test.go`
- Modify: `internal/api/handlers_seo.go`

- [ ] **Step 1: Write failing ownership and copy contracts**

Assert that `#opportunity-finding` maps to Automation, the Opportunities card is inside `settings-panel-automation` and absent from `settings-panel-ai-assistance`, and `scheduled_only` is described as scheduled plus explicit manual runs. Assert backend and Analysis status no longer say “Scheduled runs only.”

- [ ] **Step 2: Verify the new contracts fail**

Run: `go test ./internal/api -run TestOpportunityFindingStatus` and `node --test web/app/lib/ai-authority-settings-contract.test.mjs web/app/lib/dashboard-ux-phase1-contract.test.mjs web/app/lib/seo-client-contract.test.mjs`

Expected: FAIL because the card and anchor are owned by AI assistance and current copy excludes manual execution.

- [ ] **Step 3: Move the existing Opportunities card and align copy**

Move, rather than duplicate, the Opportunities authority controls to the top of the Automation panel with `id="opportunity-finding"`. Keep a link from the AI assistance overview to `#opportunity-finding`. Change the `scheduled_only` label/detail and both status presentations to “Scheduled + manual.”

- [ ] **Step 4: Verify the focused contracts pass**

Run the same Go and Node commands from Step 2.

Expected: PASS.

### Task 3: Repository and production verification

**Files:**
- Verify all modified files and the two design documents.

- [ ] **Step 1: Format and run the complete relevant suite**

Run: `gofmt -w internal/config/config.go internal/config/config_test.go internal/api/handlers_seo.go internal/api/opportunity_finding_status_contract_test.go`, `go test ./...`, `npm test --prefix web`, `npm run typecheck --prefix web`, and `npm run build --prefix web`.

Expected: all commands exit 0.

- [ ] **Step 2: Commit, push, open a PR, and merge it**

Commit only the scoped files on `codex/manual-run-forces-growth-radar`, push the branch, open a PR targeting `main`, wait for required checks, and merge.

- [ ] **Step 3: Verify deployment and production behavior**

Wait for backend and frontend production deployments. Open UniPost Settings at `#automation`, confirm the Opportunity Finding controls and scheduled-plus-manual copy are visible, click **Run finding**, wait for completion, and confirm production recorded a new Growth Radar run/materialization while the project remains `scheduled_only`.
