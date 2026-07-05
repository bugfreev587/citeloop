# Publish Preview SEO Details Drawer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Publish `Preview` open the real reader preview in a new tab and move publish-time SEO inspection into a right-side `SEO Details` drawer.

**Architecture:** Keep Publish as the queue surface. Reuse the existing standalone preview URL helper for reader preview, and add an in-page drawer that renders readable SEO metadata for the selected article. Protect the behavior through the existing dashboard contract test before changing the component.

**Tech Stack:** Next.js App Router, React client component, Tailwind CSS, lucide-react, Node `node:test`.

---

### Task 1: Lock The Publish Action Contract

**Files:**
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Write the failing contract test**

Update the `publishing Ready now strip uses publish retry and preview actions` test so the `ReadyNowStrip` block must include:

```js
assert.match(readyNowBlock, /articlePreviewHref\(projectId, item\.article\)/);
assert.match(readyNowBlock, /target="_blank"/);
assert.match(readyNowBlock, /rel="noopener noreferrer"/);
assert.match(readyNowBlock, /SEO Details/);
assert.match(readyNowBlock, /onSeoDetails\(item\.article\)/);
assert.doesNotMatch(readyNowBlock, /href=\{`\/projects\/\$\{projectId\}\/articles\/\$\{item\.articleId\}`\}/);
```

Add Publish-level assertions that the component owns a `seo_details` drawer and uses `data-publish-seo-details-drawer`.

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: FAIL because Publish still links `Preview` to the article detail page and has no SEO Details drawer.

### Task 2: Implement Preview And SEO Details Drawer

**Files:**
- Modify: `web/app/projects/[id]/publishing/publishing-client.tsx`

- [ ] **Step 1: Add preview helper import and drawer state**

Import `articlePreviewHref` from `../../../lib/review-insights`. Extend `DrawerKind` with `"seo_details"` and add `selectedSEOArticle` state.

- [ ] **Step 2: Update ReadyNow actions**

Change the ready item `Preview` link to:

```tsx
<a href={articlePreviewHref(projectId, item.article)} target="_blank" rel="noopener noreferrer">
  <Eye size={14} />
  {item.secondaryActionLabel}
  <ExternalLink size={13} />
</a>
```

Add a secondary button:

```tsx
<Button size="sm" onClick={() => onSeoDetails(item.article)}>
  <Search size={14} />
  SEO Details
</Button>
```

- [ ] **Step 3: Add readable SEO Details drawer**

Add an `openSEODetails(article)` helper that stores the article and opens the drawer. Add a `SEO Details` drawer with `dataAttribute="publish-seo-details-drawer"` showing search appearance, slug/canonical/keyword/H1, score/status rows, destination/timing, and collapsible raw metadata.

- [ ] **Step 4: Run the focused contract test**

Run:

```bash
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: PASS.

### Task 3: Verify And Ship

**Files:**
- No additional source files unless verification reveals a defect.

- [ ] **Step 1: Run full local verification**

Run:

```bash
go test ./...
npm test
npm run typecheck
npm run build
```

Expected: all commands PASS.

- [ ] **Step 2: Commit and push**

Run:

```bash
git add docs/superpowers/plans/2026-07-05-publish-preview-seo-details-drawer.md web/app/lib/dashboard-ux-phase1-contract.test.mjs web/app/projects/[id]/publishing/publishing-client.tsx
git commit -m "feat(publish): add seo details drawer"
git push -u origin codex/publish-seo-details-drawer
```

- [ ] **Step 3: Open, merge, and verify production**

Create a PR against `origin/main`, merge it after checks pass, wait for deployment, then verify production Publish behavior:

- `Preview` opens a new tab to the reader preview or live canonical URL.
- `SEO Details` opens and closes as a right drawer without leaving Publish.
- The drawer shows readable SEO metadata and raw metadata remains available below the summary.
