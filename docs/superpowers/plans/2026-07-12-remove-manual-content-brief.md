# Remove Manual Content Brief Creation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent operators and API clients from creating source-less Content Briefs while preserving Opportunity-derived planning and all existing Topic lifecycle operations.

**Architecture:** Remove the manual creation surface at the React client, typed API client, and Go router/handler boundaries. Keep the internal Topic persistence model and all read/update/schedule/generate/archive paths so accepted Opportunities and historical records remain functional.

**Tech Stack:** Next.js 15, React 18, TypeScript, Node test runner, Go 1.24, Chi router.

---

## File Map

- `web/app/projects/[id]/topics/topics-client.tsx`: Content Plan UI; remove create-only state, handler, form, and copy.
- `web/app/lib/api.ts`: typed browser API client; remove the arbitrary Topic create mutation.
- `web/app/lib/api.test.mjs`: lock the reduced Topic client surface while retaining update/schedule/archive behavior.
- `web/app/lib/dashboard-ux-phase1-contract.test.mjs`: lock the Content Plan UI against manual creation regressions.
- `internal/api/server.go`: remove the public project-scoped manual Topic POST route.
- `internal/api/handlers_agents.go`: remove the create-only HTTP handler while retaining Topic lifecycle handlers.
- `internal/api/topics_routes_test.go`: prove POST on the Topic collection is no longer registered.
- `internal/api/opportunity_routing_work_queues_contract_test.go`: replace the obsolete positive route contract with a negative contract.
- `docs/PRD-CiteLoop-Content-Plan-Brief-First-Redesign.md`: supersede the old manual Brief decision.
- `docs/PRD-CiteLoop-Analysis-Workflow.md`: remove manual seed intake from the current Content Plan contract.
- `docs/PRD-CiteLoop-Doctor-Opportunities-Two-Line-Optimization.md`: state Doctor and Opportunities as the only work sources.

### Task 1: Lock The Frontend Against Manual Creation

**Files:**
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
- Modify: `web/app/lib/api.test.mjs`

- [ ] **Step 1: Write failing Content Plan source-boundary assertions**

Change the existing Content Plan contract to require the create UI and handler to be absent:

```js
assert.doesNotMatch(topics, /New Content Brief/);
assert.doesNotMatch(topics, /createManualBrief/);
assert.doesNotMatch(topics, /api\.createTopic\(projectId/);
assert.doesNotMatch(topics, /create a new content brief/);
```

- [ ] **Step 2: Write a failing API client assertion**

Remove the create request from the Topic mutation test and assert the unsupported method is absent:

```js
assert.equal(client.createTopic, undefined);
await client.updateTopic("project-1", "topic-1", { title: "Updated topic", priority: 3 });
await client.scheduleTopic("project-1", "topic-1", "2026-06-10T09:00:00.000Z");
await client.archiveTopic("project-1", "topic-1");
```

- [ ] **Step 3: Run the focused tests and verify RED**

Run:

```bash
cd web
node --test app/lib/dashboard-ux-phase1-contract.test.mjs app/lib/api.test.mjs
```

Expected: failures report that `New Content Brief`, `createManualBrief`, and `createTopic` still exist.

- [ ] **Step 4: Commit the failing tests**

```bash
git add web/app/lib/dashboard-ux-phase1-contract.test.mjs web/app/lib/api.test.mjs
git commit -m "test: forbid manual content brief creation"
```

### Task 2: Remove The Manual Content Plan UI And Client Mutation

**Files:**
- Modify: `web/app/projects/[id]/topics/topics-client.tsx`
- Modify: `web/app/lib/api.ts`

- [ ] **Step 1: Remove create-only UI state and behavior**

Delete `defaultContentBriefDraft`, `newBriefOpen`, `newBriefDraft`, and
`createManualBrief`. Keep `TopicDraft`, `draftFromTopic`, and existing Topic
editing because they serve historical and Opportunity-linked records.

- [ ] **Step 2: Remove the button and form**

Keep the section action count and Recently Drafted control, but remove the
create control:

```tsx
<div className="flex flex-wrap items-center justify-end gap-2">
  <Badge tone="green">{acceptedPlanActions.length + legacyBriefTopics.length}</Badge>
  <Button data-content-plan-recent-drawer-trigger>{/* existing content */}</Button>
</div>
```

Use the Opportunity-only empty-state copy:

```tsx
<EmptyState
  title="No content briefs yet"
  detail="Accept AI-generated content opportunities from Opportunities to start planning."
/>
```

Change the legacy eyebrow to `Earlier briefs without an Opportunity link`.

- [ ] **Step 3: Remove the typed create mutation**

Delete `TopicCreateInput` and the `createTopic` method from `createApi`. Preserve
`TopicUpdateInput`, `listTopics`, `updateTopic`, `scheduleTopic`, `archiveTopic`,
and `generateTopic`.

- [ ] **Step 4: Run the focused tests and verify GREEN**

Run:

```bash
cd web
node --test app/lib/dashboard-ux-phase1-contract.test.mjs app/lib/api.test.mjs
npm run typecheck
```

Expected: focused tests and TypeScript checking pass.

- [ ] **Step 5: Commit the frontend implementation**

```bash
git add web/app/projects/[id]/topics/topics-client.tsx web/app/lib/api.ts
git commit -m "feat: remove manual content brief entry"
```

### Task 3: Lock The Backend Against Arbitrary Topic Creation

**Files:**
- Modify: `internal/api/topics_routes_test.go`
- Modify: `internal/api/opportunity_routing_work_queues_contract_test.go`

- [ ] **Step 1: Add a failing route behavior test**

Add a collection-route case that expects POST to be unavailable:

```go
func TestManualTopicCreationRouteIsNotRegistered(t *testing.T) {
	router := (&Server{}).Router()
	req := httptest.NewRequest(http.MethodPost, "/api/projects/not-a-uuid/topics", nil)
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("manual topic create status = %d, want %d", res.Code, http.StatusMethodNotAllowed)
	}
}
```

- [ ] **Step 2: Replace the stale positive route contract**

Remove `r.Post("/topics", s.createTopic)` from the required route list and add:

```go
if strings.Contains(routes, `r.Post("/topics", s.createTopic)`) {
	t.Fatal("manual Topic creation route must not be registered")
}
```

- [ ] **Step 3: Run the focused tests and verify RED**

Run:

```bash
go test ./internal/api -run 'TestManualTopicCreationRouteIsNotRegistered|TestOpportunityWorkQueuesSchemaAndRoutes' -count=1
```

Expected: the manual route test receives HTTP 400 because the old handler is still registered.

- [ ] **Step 4: Commit the failing tests**

```bash
git add internal/api/topics_routes_test.go internal/api/opportunity_routing_work_queues_contract_test.go
git commit -m "test: reject arbitrary topic creation"
```

### Task 4: Remove The Manual Topic HTTP Endpoint

**Files:**
- Modify: `internal/api/server.go`
- Modify: `internal/api/handlers_agents.go`

- [ ] **Step 1: Remove the collection POST route**

Delete only this registration:

```go
r.Post("/topics", s.createTopic)
```

Keep the collection GET and every item-scoped Topic lifecycle route.

- [ ] **Step 2: Remove the create-only handler**

Delete `createTopic` and `nullableTopicTextValue`. Keep `nullableTopicText` and
`validTopicChannel`, which are used by `updateTopic`.

- [ ] **Step 3: Run the focused tests and verify GREEN**

Run:

```bash
go test ./internal/api -run 'TestManualTopicCreationRouteIsNotRegistered|TestOpportunityWorkQueuesSchemaAndRoutes|TestTopicMutationRoutesAreRegistered' -count=1
```

Expected: all focused API tests pass.

- [ ] **Step 4: Commit the backend implementation**

```bash
git add internal/api/server.go internal/api/handlers_agents.go
git commit -m "feat: close manual topic creation endpoint"
```

### Task 5: Align Product Documentation

**Files:**
- Modify: `docs/PRD-CiteLoop-Content-Plan-Brief-First-Redesign.md`
- Modify: `docs/PRD-CiteLoop-Analysis-Workflow.md`
- Modify: `docs/PRD-CiteLoop-Doctor-Opportunities-Two-Line-Optimization.md`

- [ ] **Step 1: Remove current manual-intake requirements**

Replace manual seed requirements with the current source contract:

```text
Doctor and Opportunities are the only user-visible sources of new work.
Content Briefs are created only from accepted AI-generated Opportunities.
Content Plan does not provide manual Opportunity or Content Brief creation.
```

Keep historical migration compatibility language for existing Topics, but do
not advertise them as a supported intake mechanism.

- [ ] **Step 2: Verify documentation consistency**

Run:

```bash
rg -n -i 'New Content Brief|create a new content brief|accepted manually seeded topic|manually seeded topics|add a manual topic' \
  docs/PRD-CiteLoop-Content-Plan-Brief-First-Redesign.md \
  docs/PRD-CiteLoop-Analysis-Workflow.md \
  docs/PRD-CiteLoop-Doctor-Opportunities-Two-Line-Optimization.md
```

Expected: no current product requirement offers manual Opportunity or Content Brief creation; any remaining match is explicitly historical or rejected behavior.

- [ ] **Step 3: Commit documentation updates**

```bash
git add docs/PRD-CiteLoop-Content-Plan-Brief-First-Redesign.md docs/PRD-CiteLoop-Analysis-Workflow.md docs/PRD-CiteLoop-Doctor-Opportunities-Two-Line-Optimization.md
git commit -m "docs: make Doctor and Opportunities the only work sources"
```

### Task 6: Close The Legacy Strategist Bypass

**Files:**
- Modify: `web/app/lib/api.test.mjs`
- Modify: `web/app/lib/api.ts`
- Modify: `internal/api/topics_routes_test.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/handlers_agents.go`
- Modify: `internal/api/workflow_events_contract_test.go`

- [ ] **Step 1: Write failing browser and route contracts**

Require `client.runStrategist` to be absent, and require POST on the legacy
Strategist collection route to return HTTP 404 with a valid project UUID.
Replace the obsolete positive workflow-event test for `runStrategist` with a
negative server-source assertion.

- [ ] **Step 2: Verify RED**

```bash
cd web
node --test app/lib/api.test.mjs
cd ..
go test ./internal/api -run 'TestStrategistRouteIsNotRegistered|TestLegacyStrategistHandlerIsNotExposed' -count=1
```

Expected: the client still exposes `runStrategist`, and the registered route
returns a handler response instead of HTTP 404.

- [ ] **Step 3: Remove the exposed bypass**

Delete the browser `runStrategist` method, the `POST /strategist` registration,
and the `runStrategist` HTTP handler. Keep the internal Strategist package and
historical data; they are not externally executable after this change.

- [ ] **Step 4: Verify GREEN and commit**

```bash
cd web
node --test app/lib/api.test.mjs
cd ..
go test ./internal/api -count=1
git diff --check
git add web/app/lib/api.test.mjs web/app/lib/api.ts internal/api/topics_routes_test.go internal/api/server.go internal/api/handlers_agents.go internal/api/workflow_events_contract_test.go
git commit -m "feat: close legacy strategist entry point"
```

### Task 7: Full Verification And Delivery

**Files:**
- Verify all changed files.

- [ ] **Step 1: Run formatting and static checks**

```bash
gofmt -w internal/api/handlers_agents.go internal/api/server.go internal/api/topics_routes_test.go internal/api/opportunity_routing_work_queues_contract_test.go
git diff --check
go vet ./...
go build ./...
```

Expected: all commands succeed with no warnings introduced by this change.

- [ ] **Step 2: Run full Go verification**

```bash
go test ./...
```

Expected: all packages pass.

- [ ] **Step 3: Run full Web verification**

```bash
cd web
npm test
npm run typecheck
npm run build
```

Expected: all 355+ tests pass, type checking succeeds, and Next.js production build completes.

- [ ] **Step 4: Review the complete diff**

```bash
git status --short
git diff origin/main...HEAD --check
git diff --stat origin/main...HEAD
```

Expected: only the design, plan, tests, focused UI/API removals, and aligned PRDs changed.

- [ ] **Step 5: Push, open, and merge the PR**

```bash
git push -u origin codex/remove-manual-content-brief
gh pr create --base main --head codex/remove-manual-content-brief --title "Remove manual Content Brief creation" --body "Removes the manual Content Brief UI and arbitrary Topic POST endpoint while preserving Opportunity-derived planning and existing Topic lifecycle operations."
gh pr merge --merge --delete-branch
```

Expected: PR is merged into `origin/main` after required checks pass.

- [ ] **Step 6: Wait for deployment and verify production**

Verify on the canonical production Content Plan page:

```text
- New Content Brief is absent.
- No manual creation form can be opened.
- Empty-state copy routes the user to Opportunities.
- Existing/accepted work still renders.
- An accepted Opportunity can still be planned and drafted.
```

If production differs, fix on the same branch or a follow-up branch as required,
merge, wait for deployment, and repeat verification before reporting completion.
