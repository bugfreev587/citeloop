# Content Plan Brief-First Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Execute the Content Plan Brief-first PRD so Content Plan exposes Content Briefs, not user-managed Topics, while preserving scheduling, handoff cards, and content generation.

**Architecture:** Keep Topic as the backend generation/scheduling record for V1, but shift the user-facing model to Content Briefs derived from accepted content actions and linked legacy Topics. Store V1 publish strategy in Topic channel and derive it from the Content Brief selector before planning/generation; avoid a database migration until a later persistence pass proves it is needed.

**Tech Stack:** Go API/scheduler/sqlc queries, Next.js React client, Node contract tests, Go contract tests.

---

### Task 1: Lock Routing And Generation Contracts

**Files:**
- Modify: `internal/api/handlers_seo.go`
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/db/queries/seo.sql`
- Modify: `internal/api/opportunity_routing_work_queues_contract_test.go`
- Modify: `internal/scheduler/action_routing_contract_test.go`
- Modify: `web/app/lib/seo-client-contract.test.mjs`

- [x] **Step 1: Update tests for split metadata routing**

  Expected rule:
  - `metadata_rewrite` asset type stays topic-backed when the action is editorial page-refresh work.
  - Pure site-fix asset types remain direct: `schema_patch`, `sitemap_update`, `technical_fix`, `internal_link_patch`, robots/canonical/crawler terms.
  - UI contract no longer says all metadata work must avoid Site Fixes; it says `metadata_rewrite` is not in the direct action asset-type set.

- [x] **Step 2: Change `contentActionNeedsTopic` in both API and scheduler**

  Remove broad `metadata`, `title`, and `meta description` exclusions. Keep technical exclusions. This preserves editorial metadata/page refresh as content-plan work and lets pure site fixes route by work type/asset type before reaching planning.

- [x] **Step 3: Change `ListUnplannedContentActions`**

  Remove broad metadata/title/meta-description filters from the SQL query. Keep technical/direct-patch filters.

- [x] **Step 4: Add publish strategy derivation**

  Add shared helpers in API and scheduler:
  - `publishStrategyForContentAction(action, opp) string`
  - `normalizePublishStrategy(value string) string`

  V1 source order:
  1. Existing linked topic channel if present.
  2. `input_snapshot.publish_strategy` / `input_snapshot.publish_to`.
  3. `output_snapshot.publish_strategy` / `output_snapshot.publish_to`.
  4. Heuristic from asset/work/opportunity: create-content SEO pages default `both`, page refresh defaults `blog`, community/platform intent defaults `syndication`, unknown defaults `blog`.

- [x] **Step 5: Replace both `Channel: "blog"` hardcodes**

  Update `topicFromContentAction` in `internal/api/handlers_seo.go` and `internal/scheduler/scheduler.go` to set `Channel` from the helper, not a constant.

- [x] **Step 6: Run focused backend tests**

  Run:
  - `go test ./internal/api ./internal/scheduler ./internal/db`

### Task 2: Convert Content Plan UI To Brief-First

**Files:**
- Modify: `web/app/projects/[id]/content-workflow-client.tsx`
- Modify: `web/app/projects/[id]/topics/topics-client.tsx`
- Modify: `web/app/lib/content-plan-logic.ts`
- Modify: `web/app/lib/content-plan-logic.test.mjs`
- Modify: `web/app/lib/workflow-handoff-link-cards-contract.test.mjs`
- Modify: `web/app/lib/visibility-summary-contract.test.mjs`

- [x] **Step 1: Rename user-facing plan shell copy**

  Change Plan eyebrow from `Planned topics and action handoff` to `Content briefs and review handoff`.

- [x] **Step 2: Make Content Briefs the primary queue**

  Replace `Accepted opportunities` section title with `Content briefs`. Keep `data-content-plan-handoff-section` and `data-content-plan-action-card` markers for compatibility.

- [x] **Step 3: Remove separate Planned Topics grid for normal content actions**

  Hide the Planned Topics section as a primary queue. Continue using Topic data internally for linked/scheduled/drafted state.

- [x] **Step 4: Preserve scheduled legacy Topics visibly**

  Render scheduled/backlog legacy Topics that do not have a visible content action as temporary legacy Content Brief cards with schedule/cancel/reschedule and Draft Content controls. Do not allow scheduled Topics to auto-draft without a visible UI surface.

- [x] **Step 5: Preserve Sent to Review cards**

  Keep `data-content-plan-recently-sent` and `data-content-plan-sent-card`. Prefer content action draft state where available and fall back to drafted Topic state.

### Task 3: Add Publish To Selector For Content Briefs

**Files:**
- Modify: `web/app/projects/[id]/topics/topics-client.tsx`
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/content-plan-logic.ts`
- Modify: `web/app/lib/content-plan-logic.test.mjs`

- [x] **Step 1: Add publish strategy helpers**

  Implement:
  - `normalizePublishStrategy`
  - `recommendedPublishStrategyForAction`
  - `publishStrategyReasonForAction`
  - `publishStrategyLabel`

- [x] **Step 2: Add local selector state**

  Track selected publish strategy by action ID. Default to linked topic channel or recommendation.

- [x] **Step 3: Show `Publish to` selector in drawer**

  Add segmented buttons for Blog, Syndication, Both. Mark recommendation with `Recommended` and show a short reason.

- [x] **Step 4: Apply selection before Draft Content**

  If an action already has a linked Topic, update the Topic channel before generation. If no Topic exists, call the plan endpoint with the selected strategy when API support is added, or update the returned Topic before generation as V1 fallback.

### Task 4: Add Manual New Content Brief Entry

**Files:**
- Modify: `web/app/projects/[id]/topics/topics-client.tsx`
- Modify: `web/app/lib/api.ts`
- Modify: `internal/api/handlers_agents.go` if existing manual Topic route is still the right backend path.

- [x] **Step 1: Inventory current manual Topic creation route**

  Reuse existing Topic create API if available. The UI label becomes `New Content Brief`.

- [x] **Step 2: Add entry point**

  Put `New Content Brief` near the Content Briefs header or stage header action without restoring Topic language.

- [x] **Step 3: Keep Topic backend internal**

  Save the manual brief as a Topic internally, then render it as a legacy/manual Content Brief card.

### Task 5: Verification And Rollout Readiness

**Files:**
- Modify: contract tests as needed.

- [x] **Step 1: Run web contract tests**

  Run:
  - `npm test -- --test-reporter=spec app/lib/content-plan-logic.test.mjs app/lib/workflow-handoff-link-cards-contract.test.mjs app/lib/visibility-summary-contract.test.mjs app/lib/seo-client-contract.test.mjs`

- [x] **Step 2: Run typecheck**

  Run:
  - `npm run typecheck`

- [x] **Step 3: Run backend focused tests**

  Run:
  - `go test ./internal/api ./internal/scheduler ./internal/db`

- [ ] **Step 4: Manual browser verification**

  Start the web app and verify:
  - Content Plan title/eyebrow uses Content Brief language.
  - No primary `Planned topics` section appears.
  - Accepted content cards open drawer.
  - Drawer shows `Publish to` selector.
  - Sent to Review handoff still appears.
  - Scheduled legacy items stay visible.
