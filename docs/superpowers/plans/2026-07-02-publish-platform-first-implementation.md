# Publish Platform-First Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Implement the approved Publish C2 direction so the first viewport is destination-first, with a compact Ready now strip and all operational states preserved behind drawers/details.

**Architecture:** Keep backend APIs unchanged. Add a pure frontend logic module that derives destination tiles, header CTA state, Ready now items, manual syndication counts, and operational drawer groups from existing article, distribution, connection, and project config data. Refactor `PublishingClient` to render the platform-first surface from that logic while preserving existing publish, retry, schedule, reconcile, copy, open compose, and mark-distributed actions.

**Tech Stack:** Next.js App Router, React client components, TypeScript, lucide-react, Tailwind CSS, Node test runner.

---

## File Map

- Create `web/app/lib/publish-destinations-logic.ts`: pure derivation helpers for PRD acceptance criteria.
- Create `web/app/lib/publish-destinations-logic.test.mjs`: contract tests for destination state, ready-now, manual syndication, header CTA, and drawer grouping rules.
- Modify `web/app/projects/[id]/publishing/publishing-client.tsx`: platform-first layout, destination tiles, Ready now strip, detail drawers, schedule drawer retention, and Settings boundary links.
- Modify `web/app/lib/dashboard-ux-phase1-contract.test.mjs`: replace old six-lane/header-popover contracts with C2 source contracts.

## Task 1: Pure Logic Tests First

**Files:**
- Create: `web/app/lib/publish-destinations-logic.test.mjs`
- Create: `web/app/lib/publish-destinations-logic.ts`

- [x] **Step 1: Add failing destination state tests**

Cover GitHub/Next.js states:

```js
"Auto publish"
"Not connected"
"Disabled"
"Needs attention"
```

Also assert `publish_mode: "auto"` does not create the `Auto publish` destination label without an enabled connected GitHub/Next.js connection.

- [x] **Step 2: Add failing destination matrix tests**

Assert the first viewport destinations include GitHub/Next.js, Dev.to, Hashnode, Reddit, and a CMS roadmap group. Assert Medium, LinkedIn, and Hacker News stay in More unless ready/waiting variants exist for those platforms. Assert roadmap CMS entries are never active publish actions.

- [x] **Step 3: Add failing Ready now tests**

Assert approved canonical items due now and retryable failed canonical items appear with `Publish` or `Retry`, scheduled future canonical items do not appear, and `Preview` is optional secondary text while `Review` is absent.

- [x] **Step 4: Add failing manual syndication tests**

Assert ready counts come only from unlocked `DistributeItem` rows, waiting `syndication_variant` rows are counted separately, and waiting rows expose no `Copy` or `Mark distributed` action.

- [x] **Step 5: Add failing operational drawer group tests**

Assert one grouped model contains `Ready`, `Scheduled`, `Published`, `Failed`, `Waiting on canonical`, and `Ready to distribute`.

- [x] **Step 6: Run focused test and verify RED**

Run:

```bash
cd web && npm test -- app/lib/publish-destinations-logic.test.mjs
```

Expected: FAIL because `publish-destinations-logic.ts` does not exist yet.

## Task 2: Pure Publish Logic Implementation

**Files:**
- Modify: `web/app/lib/publish-destinations-logic.ts`
- Test: `web/app/lib/publish-destinations-logic.test.mjs`

- [x] **Step 1: Implement destination derivation**

Add `buildPublishDestinations(input)` with canonical GitHub/Next.js, V1 manual platforms (`dev_to`, `hashnode`, `reddit`), More manual platforms (`medium`, `linkedin`, `hacker_news`), and CMS roadmap entries (`WordPress`, `Webflow`, `Shopify`, `Custom CMS`).

- [x] **Step 2: Implement header CTA derivation**

Add `buildPublishHeaderCta(input)` matching the PRD state table and deep-linking connection setup to `/projects/:id/settings#publisher`.

- [x] **Step 3: Implement Ready now derivation**

Add `buildReadyNow(input)` for due approved canonical items and retryable failures, with disabled actions when no active publisher connection exists.

- [x] **Step 4: Implement manual and drawer derivation**

Add `buildManualSyndicationSummary(input)` and `buildPublishingOperationalGroups(input)` so UI grouping is data-driven and testable.

- [x] **Step 5: Run focused logic tests**

Run:

```bash
cd web && npm test -- app/lib/publish-destinations-logic.test.mjs
```

Expected: PASS.

## Task 3: Platform-First Publish UI

**Files:**
- Modify: `web/app/projects/[id]/publishing/publishing-client.tsx`

- [x] **Step 1: Replace header copy and CTA controls**

Render title `Publish`, short subtitle `Choose a destination. Ship approved content.`, exactly one state-driven primary CTA, secondary `Schedule`, `View all`, status check, and refresh.

- [x] **Step 2: Replace first viewport lanes with C2 layout**

Render `Destinations` as the dominant area and `Ready now` as a compact action strip. On mobile source order must be GitHub/Next.js tile, Ready now, manual tiles, then More/CMS.

- [x] **Step 3: Add destination tiles**

Add accessible tiles for GitHub/Next.js, Dev.to, Hashnode, Reddit, More, and CMS roadmap. Tiles open detail drawers and never trigger direct publishing without a visible publish action.

- [x] **Step 4: Add Ready now strip**

Render a small number of due publish/retry items with `Publish`, `Retry`, and `Preview`. Use the compact empty state when no approved canonical items are ready.

- [x] **Step 5: Add detail drawers**

Support GitHub/Next.js status, manual platform drafts, More/manual list, CMS roadmap explanation, and unified `View all` operational grouping. Keep the existing schedule drawer.

- [x] **Step 6: Preserve existing mutations**

Keep `publishNow`, `retryPublish`, `markDistributed`, `reconcile`, `refresh`, and `saveMode` behavior, including button-level busy labels.

## Task 4: Source Contract Tests

**Files:**
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [x] **Step 1: Replace old publishing layout contracts**

Remove expectations that the first viewport is lane-based or that platforms live in a header popover.

- [x] **Step 2: Add C2 source contracts**

Assert the source contains destination-first markers, Ready now strip, single `View all` drawer grouping, settings boundary link, roadmap CMS copy, and no first-viewport six-lane rendering.

- [x] **Step 3: Run focused contract tests**

Run:

```bash
cd web && npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs app/lib/publish-destinations-logic.test.mjs
```

Expected: PASS.

## Task 5: Verification

**Files:**
- Verify only.

- [x] **Step 1: Run full web tests**

Run:

```bash
cd web && npm test
```

Expected: PASS.

- [x] **Step 2: Run web typecheck**

Run:

```bash
cd web && npm run typecheck
```

Expected: PASS.

- [x] **Step 3: Run backend tests**

Run:

```bash
make test
```

Expected: PASS.

- [x] **Step 4: Run visual verification**

Start the dev server and verify the Publish page at the current localhost URL or the assigned dev-server URL. Confirm the first viewport reads as platform-first, UI elements do not overlap on desktop/mobile widths, drawers open, and roadmap platforms do not look actionable.

- [x] **Step 5: Inspect acceptance criteria coverage**

Review `docs/superpowers/specs/2026-07-02-publish-platform-first-design.md` against the implementation and tests. Every section-level acceptance criterion should map to either pure logic tests, source contracts, or manual visual verification notes.
