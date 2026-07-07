# Publish Flow UX Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the reviewed Publish Flow UX Remediation PRD so Manual publishing has one clear per-content publish action, Scheduled/Manual are the only cadence modes, published content appears below Ready to Post with live URLs, and legacy `auto` mode is normalized safely.

**Architecture:** Keep behavior testable through existing pure logic modules and contract tests. Backend changes are limited to config parsing plus a migration for legacy `auto`; frontend changes render from updated publish models without adding new dependencies.

**Tech Stack:** Go config tests and SQL migrations; Next.js/React client components; TypeScript pure logic compiled by Node `node:test`; Tailwind CSS classes already in the app.

---

### Task 1: Normalize Legacy Auto Publish Mode

**Files:**
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/config.go`
- Create: `internal/migrations/0040_publish_auto_to_scheduled.sql`
- Modify: `internal/db/publishing_contract_test.go`

- [ ] **Step 1: Write failing config tests**

Add tests asserting `publish_mode:"auto"` parses to scheduled, invalid intervals become `1`, and explicit scheduled/manual still survive:

```go
func TestParseNormalizesLegacyAutoPublishModeToScheduled(t *testing.T) {
	c, err := Parse(json.RawMessage(`{"publish_mode":"auto","publish_interval_days":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.PublishMode != PublishModeScheduled {
		t.Fatalf("legacy auto normalized to %q, want scheduled", c.PublishMode)
	}
	if c.PublishIntervalDays != 5 {
		t.Fatalf("publish_interval_days = %d, want preserved 5", c.PublishIntervalDays)
	}
}

func TestParseNormalizesLegacyAutoPublishModeWithInvalidInterval(t *testing.T) {
	c, err := Parse(json.RawMessage(`{"publish_mode":"auto","publish_interval_days":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.PublishMode != PublishModeScheduled {
		t.Fatalf("legacy auto normalized to %q, want scheduled", c.PublishMode)
	}
	if c.PublishIntervalDays != 1 {
		t.Fatalf("legacy auto interval = %d, want 1", c.PublishIntervalDays)
	}
}
```

- [ ] **Step 2: Run config test and verify RED**

Run: `go test ./internal/config -run 'TestParseNormalizesLegacyAutoPublishMode'`

Expected: FAIL because `Parse` currently preserves `auto` and sets invalid intervals to `2`.

- [ ] **Step 3: Implement parser normalization**

Update `Parse` to treat `PublishModeAuto` as `PublishModeScheduled` and set interval `1` only for that legacy path:

```go
legacyAuto := c.PublishMode == PublishModeAuto
switch c.PublishMode {
case PublishModeScheduled, PublishModeManual:
case PublishModeAuto:
	c.PublishMode = PublishModeScheduled
default:
	c.PublishMode = PublishModeManual
}
if c.PublishIntervalDays <= 0 {
	if legacyAuto {
		c.PublishIntervalDays = 1
	} else {
		c.PublishIntervalDays = 2
	}
}
```

- [ ] **Step 4: Add migration and migration contract**

Create `internal/migrations/0040_publish_auto_to_scheduled.sql`:

```sql
update projects
set config =
  jsonb_set(
    jsonb_set(coalesce(config, '{}'::jsonb), '{publish_mode}', '"scheduled"'::jsonb, true),
    '{publish_interval_days}',
    to_jsonb(
      case
        when nullif(config->>'publish_interval_days', '') ~ '^[0-9]+$'
             and (config->>'publish_interval_days')::int > 0
          then (config->>'publish_interval_days')::int
        else 1
      end
    ),
    true
  )
where config->>'publish_mode' = 'auto';
```

Add a contract test in `internal/db/publishing_contract_test.go` that reads this file and asserts it contains `publish_mode`, `"scheduled"`, `publish_interval_days`, and `where config->>'publish_mode' = 'auto'`.

- [ ] **Step 5: Run backend tests**

Run: `go test ./internal/config ./internal/db -run 'TestParseNormalizesLegacyAutoPublishMode|TestPublishingContract'`

Expected: PASS.

### Task 2: Update Publish Pure Logic Models

**Files:**
- Modify: `web/app/lib/publish-destinations-logic.test.mjs`
- Modify: `web/app/lib/publish-destinations-logic.ts`

- [ ] **Step 1: Write failing logic tests**

Update tests to assert:

```js
assert.equal(result.github.stateLabel, "Ready");
assert.equal(result.github.state, "ready");
assert.equal(result.github.actionLabel, "Ready");
assert.deepEqual(buildPublishHeaderCta({ projectId: "project-1", github: connected.github, readyNowItems: readyNow.items, scheduledCount: 0 }), null);
assert.equal("timingActionLabel" in readyNow.items[0], false);
assert.equal(readyNow.items[0].publishStateLabel, "Manual: publish when ready");
```

Add tests for disabled reasons:

```js
assert.equal(disabledReady.items[0].disabledReason, "Enable GitHub/Next.js publishing before publishing.");
assert.equal(errorReady.items[0].disabledReason, "Fix GitHub/Next.js before publishing.");
```

Add tests for published rows and view-all groups:

```js
assert.equal(published.rows[0].publishedUrl, "https://example.com/live");
assert.equal(missingUrl.rows[0].urlMissing, true);
assert.equal(groups.some((group) => group.key === "published"), false);
```

- [ ] **Step 2: Run web logic test and verify RED**

Run: `npm test -- app/lib/publish-destinations-logic.test.mjs`

Expected: FAIL on `Auto publish`, `Publish next`, `Timing`, missing published section model, and duplicated published group.

- [ ] **Step 3: Implement pure logic changes**

Update types and builders:

```ts
type PublishConnectionState = "ready" | "not_connected" | "disabled" | "needs_attention";
export type PublishHeaderCta =
  | { label: "Connect GitHub" | "Enable publishing" | "Fix connection"; kind: "settings"; href: string }
  | { label: "View schedule"; kind: "view_all"; groupKey: "scheduled" }
  | null;
```

Add state-aware ready input:

```ts
activePublisherConnection: PublisherConnection | null;
githubState?: PublishConnectionState;
```

Add `buildPublishedCanonicalsModel` or equivalent exported function backed by the existing published data:

```ts
export function buildPublishedCanonicals(input: { publishedCanonicals: Article[] }): PublishedCanonicalsModel
```

Make `buildPublishingOperationalGroups` omit `published` because the first-class section owns it.

- [ ] **Step 4: Run web logic tests**

Run: `npm test -- app/lib/publish-destinations-logic.test.mjs`

Expected: PASS.

### Task 3: Render The New Publish Page UX

**Files:**
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
- Modify: `web/app/projects/[id]/publishing/publishing-client.tsx`

- [ ] **Step 1: Write failing component contract tests**

Add assertions that the client source:

```js
assert.doesNotMatch(publishingClient, /Publish next/);
assert.doesNotMatch(publishingClient, /timingActionLabel/);
assert.doesNotMatch(publishingClient, /\\bauto:\\s*\\{/);
assert.match(publishingClient, /data-publish-published-section/);
assert.match(publishingClient, /Open live article/);
assert.match(publishingClient, /Published URL missing/);
```

- [ ] **Step 2: Run contract test and verify RED**

Run: `npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: FAIL because current source still contains `Publish next`, `timingActionLabel`, and `auto`.

- [ ] **Step 3: Implement publishing client**

Update component behavior:

- `type PublishMode = "scheduled" | "manual"`.
- Remove `Zap` import and the `auto` mode metadata.
- Normalize current mode with `const publishMode: PublishMode = config?.publish_mode === "scheduled" ? "scheduled" : "manual";`.
- Remove per-card `Timing` button and `onTiming` prop from `ReadyNowStrip`.
- Remove destination drawer `Publish next`.
- Render `Published` section immediately below `ReadyNowStrip`.
- Filter `operationalGroups` so `published` does not appear in `View all`.
- Use `canonical_url` for `Open live article` only and show `Published URL missing` otherwise.

- [ ] **Step 4: Run component contract tests**

Run: `npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs`

Expected: PASS.

### Task 4: Full Verification And Local Browser Check

**Files:**
- Verify only unless fixes are required.

- [ ] **Step 1: Run focused backend and frontend tests**

Run:

```bash
go test ./internal/config ./internal/db
cd web && npm test -- app/lib/publish-destinations-logic.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs
cd web && npm run typecheck
```

Expected: PASS.

- [ ] **Step 2: Start local web app**

Run: `cd web && npm run dev`

Expected: Next dev server starts on an available local port.

- [ ] **Step 3: Browser verify Publish page**

Open a local project Publish page, verify no console errors, and inspect that:

- Schedule drawer shows only Manual and Scheduled.
- Ready cards have no Timing button.
- GitHub drawer has no Publish next.
- Published section is under Ready to Post and can show live URLs.

### Task 5: Ship, Deploy, And Production Verify

**Files:**
- Verify only unless release fixes are required.

- [ ] **Step 1: Commit implementation**

Run:

```bash
git status --short
git add docs internal web
git commit -m "Implement publish flow UX remediation"
```

- [ ] **Step 2: Push and open PR**

Run:

```bash
git push -u origin codex/publish-flow-ux-impl-20260707
```

Create a PR to `origin/main` with the PRD summary, test commands, and preflight `auto` query result or the inability to run it.

- [ ] **Step 3: Merge, wait for deployment, production verify**

Merge the PR, wait for deployment, then verify the production Publish flow against the PRD acceptance criteria. If production differs, fix forward and repeat verification.
