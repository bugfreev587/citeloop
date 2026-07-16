# Recently Published Dual Buttons Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace each whole-card Recently Published Results link with two explicit controls: an internal Results button and a new-tab published-page button.

**Architecture:** Keep `PublishedSection` as the sole renderer for published-history cards, but change each row to a plain card container with sibling internal and external links in its footer. Preserve the existing published-row data model and Results deep link; represent a missing published URL with a disabled native button instead of inventing navigation or reconciliation behavior.

**Tech Stack:** Next.js App Router, React 18, TypeScript, Tailwind CSS, lucide-react, Node.js contract tests, GitHub Actions, Vercel.

---

## File Structure

- `web/app/projects/[id]/publishing/publishing-client.tsx`
  - Owns the Recently Published card markup, both navigation controls, drawer-close behavior, and the missing-URL disabled state.
- `web/app/lib/workflow-handoff-contract.test.mjs`
  - Guards the two-destination interaction contract, non-clickable card container, independent sibling controls, external-link safety attributes, and missing-URL fallback.
- `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
  - Updates the broader Publish surface contract from single-destination cards to explicit dual-button cards.

No data model, API, routing, or backend file changes are required because `PublishedCanonicalRow.publishedUrl` and the Results article deep link already exist.

## Task 1: Write the failing dual-destination contract

**Files:**
- Modify: `web/app/lib/workflow-handoff-contract.test.mjs:34-76`
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs:1091-1112`
- Test: `web/app/lib/workflow-handoff-contract.test.mjs`
- Test: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Replace the Publish handoff contract with the two-button requirements**

In `web/app/lib/workflow-handoff-contract.test.mjs`, rename the Publish test and replace its Recently Published assertions with:

```js
test("Publish exposes separate Results and published-page buttons and focuses ?article= on the main Publish card", async () => {
  const source = await readFile(new URL("../projects/[id]/publishing/publishing-client.tsx", import.meta.url), "utf8");
  for (const expected of [
    "data-publish-published-section",
    "data-publish-recent-drawer-trigger",
    'dataAttribute="publish-recent-drawer"',
    "Recently Published",
    "data-publish-results-link",
    "data-publish-live-link",
    "data-publish-live-unavailable",
    "data-publish-recent-card",
    "View Results",
    "Open Published Page",
    "Published Page Unavailable",
    "results?article=${row.articleId}",
    "onClose={() => setDrawer(null)}",
    "data-publish-ready-article-card={item.articleId}",
    "citeloop-linked-card-pulse",
    'searchParams.get("article")',
    "highlightedPublishArticleId === item.articleId",
  ]) {
    assert.equal(source.includes(expected), true, `publishing-client.tsx missing ${expected}`);
  }
  const handoffEffectStart = source.indexOf("A review handoff link lands here with ?article=");
  const handoffEffectEnd = source.indexOf("async function saveMode", handoffEffectStart);
  assert.notEqual(handoffEffectStart, -1, "publishing-client.tsx missing Review handoff focus effect");
  assert.notEqual(handoffEffectEnd, -1, "publishing-client.tsx missing Review handoff focus effect boundary");
  const handoffEffect = source.slice(handoffEffectStart, handoffEffectEnd);
  assert.equal(handoffEffect.includes('setDrawer("view_all")'), false, "Review handoff must not open the View all drawer");
  assert.equal(handoffEffect.includes("setDrawer(null)"), true, "Review handoff should close any open Publish drawer before focusing the target card");
  assert.equal(handoffEffect.includes("scrollIntoView"), true, "Review handoff should scroll the main Publish card into view");

  const recentDrawerStart = source.indexOf('dataAttribute="publish-recent-drawer"');
  const recentDrawerEnd = source.indexOf("</Drawer>", recentDrawerStart);
  assert.notEqual(recentDrawerStart, -1, "publishing-client.tsx missing Recently Published drawer");
  assert.notEqual(recentDrawerEnd, -1, "publishing-client.tsx missing Recently Published drawer boundary");
  const recentDrawer = source.slice(recentDrawerStart, recentDrawerEnd);
  assert.equal(recentDrawer.includes("onClose={() => setDrawer(null)}"), true, "Recently Published link clicks should close the drawer");

  const publishedSectionStart = source.indexOf("function PublishedSection");
  const publishedSectionEnd = source.indexOf("function OperationalRows", publishedSectionStart);
  assert.notEqual(publishedSectionStart, -1, "publishing-client.tsx missing PublishedSection");
  assert.notEqual(publishedSectionEnd, -1, "publishing-client.tsx missing PublishedSection boundary");
  const publishedSection = source.slice(publishedSectionStart, publishedSectionEnd);

  const cardMarker = publishedSection.indexOf("data-publish-published-article-card");
  const cardOpeningTag = publishedSection.lastIndexOf("<", cardMarker);
  assert.equal(
    publishedSection.slice(cardOpeningTag, cardMarker).trimStart().startsWith("<div"),
    true,
    "Recently Published cards must be plain containers rather than whole-card links",
  );
  assert.equal(
    publishedSection.match(/onClick=\{onClose\}/g)?.length,
    2,
    "both active destinations must close the recent drawer",
  );

  const resultsLinkStart = publishedSection.indexOf("data-publish-results-link");
  const resultsLinkEnd = publishedSection.indexOf("</Link>", resultsLinkStart);
  const liveLinkStart = publishedSection.indexOf("data-publish-live-link");
  const liveLinkEnd = publishedSection.indexOf("</a>", liveLinkStart);
  assert.ok(resultsLinkStart !== -1 && resultsLinkEnd !== -1, "Recently Published must render a Results link button");
  assert.ok(liveLinkStart > resultsLinkEnd && liveLinkEnd > liveLinkStart, "published-page link must be a sibling after the Results link");

  const liveLink = publishedSection.slice(liveLinkStart, liveLinkEnd);
  assert.equal(liveLink.includes("href={row.publishedUrl}"), true, "published-page button must use the stored published URL");
  assert.equal(liveLink.includes('target="_blank"'), true, "published-page button must open in a new tab");
  assert.equal(liveLink.includes('rel="noopener noreferrer"'), true, "new-tab published-page link must isolate the opener");
  assert.equal(liveLink.includes("onClick={onClose}"), true, "published-page button must close the recent drawer");

  const unavailableStart = publishedSection.indexOf("data-publish-live-unavailable");
  const unavailableEnd = publishedSection.indexOf("</button>", unavailableStart);
  const unavailableButton = publishedSection.slice(unavailableStart, unavailableEnd);
  assert.equal(unavailableButton.includes("disabled"), true, "missing published URLs must render a disabled control");
  assert.equal(unavailableButton.includes("href="), false, "missing published URLs must not render navigation");
});
```

- [ ] **Step 2: Update the broader Publish UI contract**

In `web/app/lib/dashboard-ux-phase1-contract.test.mjs`, replace the test title and the assertions through `Published URL missing` with:

```js
test("publishing Recently Published uses explicit Results and published-page buttons with publish completion state", () => {
  const publishing = read("projects/[id]/publishing/publishing-client.tsx");
  const logic = read("lib/publish-destinations-logic.ts");

  assert.match(publishing, /function PublishedSection/);
  assert.match(publishing, /data-publish-published-section/);
  assert.match(publishing, /data-publish-published-article-card/);
  assert.match(publishing, /data-publish-recent-card/);
  assert.match(publishing, /data-publish-results-link/);
  assert.match(publishing, /data-publish-live-link/);
  assert.match(publishing, /data-publish-live-unavailable/);
  assert.match(publishing, /data-publish-recent-drawer-trigger/);
  assert.match(publishing, /dataAttribute="publish-recent-drawer"/);
  assert.match(publishing, /Recently Published/);
  assert.match(publishing, /View Results/);
  assert.match(publishing, /Open Published Page/);
  assert.match(publishing, /Published Page Unavailable/);
  assert.match(publishing, /Published URL missing/);
  assert.match(publishing, /target="_blank"/);
  assert.match(publishing, /rel="noopener noreferrer"/);
```

Keep the existing assertions after that block for drawer opening, highlighting, published-only data, and publishing logic.

- [ ] **Step 3: Run the targeted tests and verify RED**

Run from `web/`:

```bash
node --test app/lib/workflow-handoff-contract.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: FAIL because `data-publish-live-link`, `data-publish-live-unavailable`, `Open Published Page`, and `Published Page Unavailable` do not exist, while the card is still a whole-card `Link`.

## Task 2: Render a static card with two independent navigation buttons

**Files:**
- Modify: `web/app/projects/[id]/publishing/publishing-client.tsx:529-569`
- Test: `web/app/lib/workflow-handoff-contract.test.mjs`
- Test: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Replace the whole-card Results link with a plain card and sibling controls**

In `PublishedSection`, replace the current `<Link ...>...</Link>` returned for each row with:

```tsx
<div
  key={row.articleId}
  id={`publish-published-${row.articleId}`}
  data-publish-published-article-card={row.articleId}
  data-publish-recent-card
  className={cx(
    "flex h-full min-h-[210px] min-w-0 flex-col rounded-lg border bg-white p-4 text-left shadow-sm transition",
    highlighted ? "citeloop-linked-card-pulse border-[#d93820] ring-2 ring-[#d93820]/15" : "border-slate-200",
  )}
>
  <div className="flex h-full min-w-0 flex-col justify-between gap-4">
    <div className="min-w-0">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="line-clamp-2 break-words text-sm font-bold leading-5 text-slate-950">{row.title}</div>
          <div className="mt-1 text-xs font-semibold text-slate-500">
            {row.publishedAt ? `Published ${formatDate(row.publishedAt)}` : "Published"}
          </div>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <Badge tone="neutral">{workflowTraceLabelForArticle(row.article)}</Badge>
            <Badge tone="blue">{workflowArticleTypeTag(row.article)}</Badge>
          </div>
        </div>
        <Badge tone={row.urlMissing ? "amber" : "green"}>{row.urlMissing ? "URL missing" : "Live"}</Badge>
      </div>
      <div className="mt-3 rounded-lg bg-slate-50 px-3 py-2">
        <div className="text-[11px] font-bold uppercase tracking-normal text-slate-400">URL</div>
        <div className={cx("mt-0.5 truncate text-xs font-semibold", row.publishedUrl ? "text-slate-700" : "text-amber-800")}>
          {row.publishedUrl || "Published URL missing"}
        </div>
      </div>
    </div>
    <div className="mt-auto flex flex-wrap items-center justify-end gap-2 border-t border-slate-100 pt-3">
      <Link
        data-publish-results-link
        href={`/projects/${projectId}/results?article=${row.articleId}`}
        onClick={onClose}
        className="inline-flex h-8 items-center gap-1 rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 transition hover:bg-slate-50 hover:text-slate-950 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820] active:scale-[0.97]"
      >
        <span>View Results</span>
        <ChevronRight aria-hidden="true" size={14} />
      </Link>
      {row.publishedUrl ? (
        <a
          data-publish-live-link
          href={row.publishedUrl}
          target="_blank"
          rel="noopener noreferrer"
          onClick={onClose}
          className="inline-flex h-8 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 transition hover:bg-slate-50 hover:text-slate-950 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820] active:scale-[0.97]"
        >
          <ExternalLink aria-hidden="true" size={14} />
          Open Published Page
        </a>
      ) : (
        <button
          type="button"
          data-publish-live-unavailable
          disabled
          className="inline-flex h-8 cursor-not-allowed items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 px-3 text-xs font-semibold text-slate-400 opacity-60"
        >
          <ExternalLink aria-hidden="true" size={14} />
          Published Page Unavailable
        </button>
      )}
    </div>
  </div>
</div>
```

Do not change the published-row data derivation, the Results route, drawer composition, or highlighted article ID behavior.

- [ ] **Step 2: Run the targeted tests and verify GREEN**

Run from `web/`:

```bash
node --test app/lib/workflow-handoff-contract.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: both files pass with zero failures.

- [ ] **Step 3: Review the focused diff**

Run:

```bash
git diff -- web/app/projects/'[id]'/publishing/publishing-client.tsx web/app/lib/workflow-handoff-contract.test.mjs web/app/lib/dashboard-ux-phase1-contract.test.mjs
git diff --check
```

Expected: only the Recently Published card contract and markup change; `git diff --check` exits 0.

- [ ] **Step 4: Commit the tested implementation**

```bash
git add web/app/projects/'[id]'/publishing/publishing-client.tsx \
  web/app/lib/workflow-handoff-contract.test.mjs \
  web/app/lib/dashboard-ux-phase1-contract.test.mjs
git commit -m "fix: add Recently Published destination buttons"
```

Expected: one implementation commit after the existing design-spec commit.

## Task 3: Run complete local verification

**Files:**
- Verify: `web/app/projects/[id]/publishing/publishing-client.tsx`
- Verify: `web/app/lib/workflow-handoff-contract.test.mjs`
- Verify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Run all web contract tests**

Run from `web/`:

```bash
npm test
```

Expected: all tests pass with zero failures.

- [ ] **Step 2: Run TypeScript checking**

Run from `web/`:

```bash
npm run typecheck
```

Expected: `tsc --noEmit` exits 0.

- [ ] **Step 3: Run a production-mode web build**

Run from `web/`:

```bash
VERCEL_ENV=preview npm run build
```

Expected: Next.js build exits 0. `VERCEL_ENV=preview` keeps unconfigured local Clerk secrets on the repository's preview-safe path.

The repository has no committed ESLint configuration. Its current `npm run lint` invokes an interactive `next lint` setup prompt, so it is not a usable non-interactive verification gate for this task; do not create unrelated lint configuration.

- [ ] **Step 4: Verify repository state before publishing**

Run from the worktree root:

```bash
git diff --check
git status --short --branch
git log --oneline origin/main..HEAD
```

Expected: no uncommitted files, no whitespace errors, and exactly the design and implementation commits on the feature branch.

## Task 4: Publish, merge, deploy, and verify production

**Files:**
- Release: branch `codex/recently-published-dual-buttons`
- Production page: `https://citeloop.app/projects/1459b054-cdc3-4d9b-9dd4-18e12458c61a/publish`

- [ ] **Step 1: Push the branch and create the PR**

Run from the worktree root:

```bash
git push -u origin codex/recently-published-dual-buttons
gh pr create \
  --base main \
  --head codex/recently-published-dual-buttons \
  --title "Add Recently Published destination buttons" \
  --body "$(cat <<'EOF'
## Summary
- replace whole-card Recently Published navigation with explicit controls
- keep View Results in-app and open the published URL in a new tab
- show a disabled published-page control when the URL is missing

## Test Plan
- node --test app/lib/workflow-handoff-contract.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs
- npm test
- npm run typecheck
- VERCEL_ENV=preview npm run build
EOF
)"
```

Expected: push succeeds and `gh pr create` returns a PR URL targeting `origin/main`.

- [ ] **Step 2: Wait for review and required checks**

Run:

```bash
gh pr checks --watch
```

Expected: Go, Web, and Vercel checks succeed. Inspect any review output; apply only actionable feedback, then rerun the full local verification and push the fix before continuing.

- [ ] **Step 3: Merge the PR**

Run:

```bash
gh pr merge --merge
```

Then capture the merge commit:

```bash
gh pr view --json url,state,mergedAt,mergeCommit
```

Expected: PR state is `MERGED` and a merge commit SHA is present on `main`.

- [ ] **Step 4: Wait for the production deployment**

Poll the merge commit status:

```bash
merge_sha="$(gh pr view --json mergeCommit --jq '.mergeCommit.oid')"
gh api "repos/bugfreev587/citeloop/commits/${merge_sha}/status"
```

Expected: the Vercel production status associated with the merged `main` commit reaches success. Also verify the public endpoints respond:

```bash
curl -fsS https://api.citeloop.app/healthz
curl -fsS -o /dev/null -w '%{http_code}\n' https://citeloop.app/
```

Expected: API health returns `ok` and the web root returns HTTP 200.

- [ ] **Step 5: Verify the authenticated production interaction**

Using the existing authenticated browser session:

1. Open `https://citeloop.app/projects/1459b054-cdc3-4d9b-9dd4-18e12458c61a/publish`.
2. Open `Recently Published`.
3. Confirm a live row is a static card and shows both `View Results` and `Open Published Page`.
4. Click `Open Published Page` and confirm a new browser tab opens at the exact visible published URL while the CiteLoop tab remains available.
5. Return to CiteLoop, reopen `Recently Published`, click `View Results`, and confirm the current tab navigates to `/results?article={articleId}` and focuses the matching measurement item.
6. Confirm no card-body or URL-text click triggers navigation and no browser console errors appear.

If production differs from the specification, add a failing regression assertion, fix the issue on the feature branch from the latest `origin/main`, push, merge the follow-up, wait for deployment, and repeat this production checklist before reporting completion.
