# Doctor Site Fix Handoff and Persistent Card Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep Doctor and every workflow mutation on its current review surface, remove handed-off Doctor findings reliably, and make cross-surface handoffs persistently highlight their target card without opening a drawer.

**Architecture:** Keep ID resolution inside each destination because Content Plan, Review, Publish, Site Fixes, Results, and Doctor load and resolve different record types. Separate each page's handoff-highlight state from its drawer-selection state, use one non-animated visual class for all handoff targets, and update Doctor from the canonical Site Fix list rather than the narrower Recent Findings list.

**Tech Stack:** Next.js 15 App Router, React 18, TypeScript, Node test runner contract/unit tests, Tailwind CSS, Go backend regression suite.

---

## File Structure

- Modify `web/app/lib/doctor-recent-findings.ts`: add a tested immutable Site Fix upsert used by Doctor mutation state.
- Modify `web/app/lib/doctor-recent-findings.test.mjs`: unit-test new and existing-fix upserts.
- Modify `web/app/lib/seo-doctor-contract.test.mjs`: lock Doctor filtering, no-navigation, and no-auto-drawer behavior.
- Modify `web/app/lib/workflow-handoff-contract.test.mjs`: lock persistent Content Plan/Review/Publish/Results handoffs.
- Modify `web/app/lib/workflow-handoff-link-cards-contract.test.mjs`: lock Recently Decided target routing.
- Modify `web/app/lib/action-portfolio-contract.test.mjs`: lock Site Fix deep links to highlight without opening the drawer.
- Modify `web/app/lib/dashboard-ux-phase1-contract.test.mjs`: replace the Review auto-drawer expectation with persistent highlight behavior.
- Modify `web/app/lib/visibility-summary-contract.test.mjs`: require persistent Content Plan highlight styling.
- Modify `web/app/lib/results-attribution-contract.test.mjs`: require Results Site Fix handoffs to resolve/pin without opening details.
- Modify `web/app/globals.css`: define the shared non-animated persistent handoff-card visual.
- Modify `web/app/projects/[id]/doctor/doctor-client.tsx`: fix canonical filtering, update local state after create, stay on Doctor, and highlight active finding deep links without a drawer.
- Modify `web/app/projects/[id]/content-workflow-client.tsx`: preserve search parameters while the combined workflow synchronizes stage paths.
- Modify `web/app/projects/[id]/topics/topics-client.tsx`: persist Content Plan handoff highlight until card interaction.
- Modify `web/app/projects/[id]/review/review-client.tsx`: split linked article highlight from selected article drawer state.
- Modify `web/app/projects/[id]/publishing/publishing-client.tsx`: persist Publish handoff highlight and consume it on direct card actions.
- Modify `web/app/projects/[id]/site-fixes/site-fixes-client.tsx`: split linked Site Fix highlight from drawer selection and preserve legacy alias resolution.
- Modify `web/app/projects/[id]/seo/seo-client.tsx`: route Recently Decided Site Fixes with an ID and make Results content, Site Fix, and watchlist handoffs highlight-only.
- Modify `web/app/projects/[id]/results/site-fix-results-card.tsx`: expose persistent selected styling without an animated pulse.

### Task 1: Reproduce and Fix Doctor Canonical Handoff State

**Files:**
- Modify: `web/app/lib/doctor-recent-findings.test.mjs`
- Modify: `web/app/lib/seo-doctor-contract.test.mjs`
- Modify: `web/app/lib/doctor-recent-findings.ts`
- Modify: `web/app/projects/[id]/doctor/doctor-client.tsx`

- [ ] **Step 1: Write failing unit tests for immutable Site Fix upsert**

Append tests that exercise real helper behavior:

```js
test("upsertDoctorSiteFix prepends a new canonical fix without mutating input", async () => {
  const { upsertDoctorSiteFix } = await loadModule();
  const original = [fix("existing", "finding-a", "2026-07-10T10:00:00Z")];
  const created = fix("created", "finding-b", "2026-07-11T10:00:00Z");

  const next = upsertDoctorSiteFix(original, created);

  assert.deepEqual(next.map((item) => item.id), ["created", "existing"]);
  assert.deepEqual(original.map((item) => item.id), ["existing"]);
});

test("upsertDoctorSiteFix replaces an existing canonical fix with its latest response", async () => {
  const { upsertDoctorSiteFix } = await loadModule();
  const original = [fix("existing", "finding-a", "2026-07-10T10:00:00Z", { status: "proposed" })];
  const updated = fix("existing", "finding-a", "2026-07-10T10:00:00Z", {
    status: "awaiting_deploy",
    application: { github_pr_number: 42 },
  });

  const next = upsertDoctorSiteFix(original, updated);

  assert.equal(next.length, 1);
  assert.equal(next[0].status, "awaiting_deploy");
  assert.equal(next[0].application.github_pr_number, 42);
});
```

- [ ] **Step 2: Write failing Doctor wiring contracts**

Replace the incorrect `siteFixLinks` assertion and add mutation/deep-link constraints:

```js
assert.match(client, /activeDoctorFindings\(actionableFindings, siteFixes\)/);
assert.match(client, /recentDoctorFindingLinks\(actionableFindings, siteFixLinks\)/);
assert.match(client, /setSiteFixes\(\(current\) => upsertDoctorSiteFix\(current, fix\)\)/);
assert.match(client, /setSiteFixLinks\(\(current\) => upsertDoctorSiteFix\(current, fix\)\)/);
assert.doesNotMatch(client, /router\.push\(`\/projects\/\$\{projectId\}\/site-fixes/);
assert.doesNotMatch(client, /const router = useRouter\(\)/);

const initialHandoffStart = client.indexOf("if (loading || initialSelectionHandled.current) return;");
const initialHandoffEnd = client.indexOf("useEffect(() => {", initialHandoffStart + 1);
const initialHandoff = client.slice(initialHandoffStart, initialHandoffEnd);
assert.match(initialHandoff, /setHighlightedFindingID\(initialFindingId\)/);
assert.doesNotMatch(initialHandoff, /setSelectedFindingID\(initialFindingId\)/);
assert.doesNotMatch(initialHandoff, /setRecentDrawerOpen\(true\)/);
```

- [ ] **Step 3: Run focused tests and verify RED**

Run:

```bash
cd web
node --test app/lib/doctor-recent-findings.test.mjs app/lib/seo-doctor-contract.test.mjs
```

Expected: FAIL because `upsertDoctorSiteFix` does not exist, Doctor still filters with `siteFixLinks`, still calls `router.push`, and deep links still select/open drawers.

- [ ] **Step 4: Implement the immutable upsert**

Add to `doctor-recent-findings.ts`:

```ts
export function upsertDoctorSiteFix(siteFixes: SiteFix[], updated: SiteFix) {
  const existingIndex = siteFixes.findIndex((siteFix) => siteFix.id === updated.id);
  if (existingIndex < 0) return [updated, ...siteFixes];
  return siteFixes.map((siteFix) => siteFix.id === updated.id ? updated : siteFix);
}
```

- [ ] **Step 5: Fix Doctor filtering and create behavior**

In `doctor-client.tsx`:

```ts
const activeFindings = useMemo(
  () => activeDoctorFindings(actionableFindings, siteFixes),
  [actionableFindings, siteFixes],
);
const recentFindingLinks = useMemo(
  () => recentDoctorFindingLinks(actionableFindings, siteFixLinks),
  [actionableFindings, siteFixLinks],
);
```

Replace the mutation success block with:

```ts
const fix = await api.createDoctorSiteFix(projectId, finding.id);
setSiteFixes((current) => upsertDoctorSiteFix(current, fix));
setSiteFixLinks((current) => upsertDoctorSiteFix(current, fix));
notify({
  tone: "green",
  title: "Added to Site Fixes",
  detail: "Review, approve, apply, deploy, and verify the canonical Site Fix.",
});
setSelectedFindingID(null);
```

Remove `useRouter`, `const router = useRouter()`, and the route push.

- [ ] **Step 6: Make Doctor finding deep links highlight-only**

Add:

```ts
const [highlightedFindingID, setHighlightedFindingID] = useState<string | null>(null);
const findingCardRefs = useRef<Record<string, HTMLButtonElement | null>>({});
```

When `initialFindingId` resolves to an active finding, set the filter and highlight, then focus with reduced-motion support:

```ts
setFilter("all");
setHighlightedFindingID(initialFindingId);
window.requestAnimationFrame(() => {
  const target = findingCardRefs.current[initialFindingId];
  if (!target) return;
  const reduced = window.matchMedia?.("(prefers-reduced-motion: reduce)")?.matches ?? false;
  target.scrollIntoView({ behavior: reduced ? "auto" : "smooth", block: "center" });
  target.focus({ preventScroll: true });
});
```

Do not open Finding Details or Recent Findings from the initial ID. Bind each card ref, use `citeloop-handoff-card-selected` while highlighted, add `aria-current={highlightedFindingID === finding.id ? "true" : undefined}`, and clear the handoff highlight before a direct card selection.

- [ ] **Step 7: Run focused tests and verify GREEN**

Run:

```bash
cd web
node --test app/lib/doctor-recent-findings.test.mjs app/lib/seo-doctor-contract.test.mjs
npx tsc --noEmit
```

Expected: all focused tests pass and TypeScript reports no errors.

- [ ] **Step 8: Commit Doctor fix**

```bash
git add web/app/lib/doctor-recent-findings.ts \
  web/app/lib/doctor-recent-findings.test.mjs \
  web/app/lib/seo-doctor-contract.test.mjs \
  'web/app/projects/[id]/doctor/doctor-client.tsx'
git commit -m "fix: keep Doctor site fix handoffs local"
```

### Task 2: Make Content Workflow Handoffs Persistent and Drawer-free

**Files:**
- Modify: `web/app/lib/workflow-handoff-contract.test.mjs`
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
- Modify: `web/app/lib/visibility-summary-contract.test.mjs`
- Modify: `web/app/globals.css`
- Modify: `web/app/projects/[id]/content-workflow-client.tsx`
- Modify: `web/app/projects/[id]/topics/topics-client.tsx`
- Modify: `web/app/projects/[id]/review/review-client.tsx`
- Modify: `web/app/projects/[id]/publishing/publishing-client.tsx`

- [ ] **Step 1: Rewrite contracts for persistent highlight-only behavior**

Add a shared-style contract:

```js
const globals = await readFile(new URL("../globals.css", import.meta.url), "utf8");
assert.match(globals, /\.citeloop-handoff-card-selected/);
assert.doesNotMatch(globals, /\.citeloop-handoff-card-selected[\s\S]*animation:/);
```

Update Content Plan, Review, and Publish assertions:

```js
assert.match(topics, /citeloop-handoff-card-selected/);
assert.doesNotMatch(
  topics.slice(topics.indexOf("if (!requestedActionID"), topics.indexOf("const actionsWithDrafts")),
  /setTimeout\([\s\S]*setHighlightContentPlanAction\(null\)/,
);

const linkedReviewEffect = review.slice(
  review.indexOf("if (!requestedArticleId"),
  review.indexOf("useEffect(() => {", review.indexOf("if (!requestedArticleId") + 1),
);
assert.match(linkedReviewEffect, /setHighlightedArticleId\(requestedArticleId\)/);
assert.doesNotMatch(linkedReviewEffect, /setSelectedArticleId\(requestedArticleId\)/);

const publishHandoff = publishing.slice(
  publishing.indexOf("A review handoff link lands here with ?article="),
  publishing.indexOf("const nextIds = new Set", publishing.indexOf("A review handoff link lands here with ?article=")),
);
assert.match(publishHandoff, /setHighlightedPublishArticleId\(linkedArticleId\)/);
assert.doesNotMatch(publishHandoff, /setTimeout\(\(\) => setHighlightedPublishArticleId\(null\)/);
```

Add query-preservation coverage:

```js
assert.match(workflow, /`\$\{workflowHref\(projectId, step\)\}\$\{window\.location\.search\}`/);
```

- [ ] **Step 2: Run workflow tests and verify RED**

Run:

```bash
cd web
node --test \
  app/lib/workflow-handoff-contract.test.mjs \
  app/lib/dashboard-ux-phase1-contract.test.mjs \
  app/lib/visibility-summary-contract.test.mjs
```

Expected: FAIL because Review still selects the linked article, Content Plan and Publish clear highlights on timers, the shared class is absent, and stage path synchronization drops the query.

- [ ] **Step 3: Add the shared persistent visual**

Add to `globals.css`:

```css
.citeloop-handoff-card-selected {
  border-color: #d93820;
  background-color: #fff4f1;
  box-shadow: 0 0 0 2px rgb(217 56 32 / 18%);
}
```

This class deliberately contains no animation.

- [ ] **Step 4: Preserve workflow query parameters**

Change `syncPathToStep` to:

```ts
const nextHref = `${workflowHref(projectId, step)}${window.location.search}`;
if (`${window.location.pathname}${window.location.search}` !== nextHref) {
  window.history.replaceState(window.history.state, "", nextHref);
}
```

Dispatch the pathname-only value in `CONTENT_WORKFLOW_PATH_CHANGE_EVENT` so sidebar active-state behavior remains unchanged.

- [ ] **Step 5: Persist Content Plan selection**

Remove the highlight clear timer from the requested-action effect. Keep scrolling/focus, replace `citeloop-linked-card-pulse` with `citeloop-handoff-card-selected`, add `aria-current`, and clear `highlightContentPlanAction` in the action card's direct `onClick` before setting `selectedContentPlanActionID`.

- [ ] **Step 6: Split Review highlight from drawer selection**

Add:

```ts
const [highlightedArticleId, setHighlightedArticleId] = useState<string | null>(null);
```

The query effect must set `highlightedArticleId`, scroll, and focus without changing `selectedArticleId`. Pass:

```tsx
selected={selectedArticleId === item.article.id}
linked={highlightedArticleId === item.article.id}
onSelect={(trigger) => {
  setHighlightedArticleId(null);
  reviewReturnFocusRef.current = trigger;
  setSelectedArticleId(item.article.id);
}}
```

Update `ReviewDecisionCard` to apply `citeloop-handoff-card-selected` for `linked` and expose `aria-current`.

- [ ] **Step 7: Persist Publish selection**

Remove the 2.35-second highlight clear timer from the review handoff effect. Replace the pulse class with `citeloop-handoff-card-selected` and add `aria-current` to the Ready to post container.

Pass an `onConsumeHandoff` callback into `ReadyNowStrip`:

```tsx
onConsumeHandoff={() => setHighlightedPublishArticleId(null)}
```

Call it before the card's user-initiated SEO, move-back, destination, retry, or publish actions so the highlight persists as guidance until the user acts.

- [ ] **Step 8: Run workflow tests and verify GREEN**

Run:

```bash
cd web
node --test \
  app/lib/workflow-handoff-contract.test.mjs \
  app/lib/dashboard-ux-phase1-contract.test.mjs \
  app/lib/visibility-summary-contract.test.mjs
npx tsc --noEmit
```

Expected: all focused tests pass and TypeScript reports no errors.

- [ ] **Step 9: Commit content workflow behavior**

```bash
git add web/app/globals.css \
  web/app/lib/workflow-handoff-contract.test.mjs \
  web/app/lib/dashboard-ux-phase1-contract.test.mjs \
  web/app/lib/visibility-summary-contract.test.mjs \
  'web/app/projects/[id]/content-workflow-client.tsx' \
  'web/app/projects/[id]/topics/topics-client.tsx' \
  'web/app/projects/[id]/review/review-client.tsx' \
  'web/app/projects/[id]/publishing/publishing-client.tsx'
git commit -m "fix: persist content workflow handoff selection"
```

### Task 3: Make Site Fix and Results Handoffs Highlight-only

**Files:**
- Modify: `web/app/lib/action-portfolio-contract.test.mjs`
- Modify: `web/app/lib/results-attribution-contract.test.mjs`
- Modify: `web/app/lib/workflow-handoff-contract.test.mjs`
- Modify: `web/app/lib/workflow-handoff-link-cards-contract.test.mjs`
- Modify: `web/app/projects/[id]/site-fixes/site-fixes-client.tsx`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Modify: `web/app/projects/[id]/results/site-fix-results-card.tsx`

- [ ] **Step 1: Write failing Site Fix and Results contracts**

For Site Fixes:

```js
const initialFixBlock = siteFixes.slice(
  siteFixes.indexOf("const canonicalInitialFixID"),
  siteFixes.indexOf("fullListLoadingRef.current = false"),
);
assert.match(initialFixBlock, /setHighlightedFixID\(canonicalInitialFixID\)/);
assert.doesNotMatch(initialFixBlock, /setSelectedID\(canonicalInitialFixID\)/);
assert.match(siteFixes, /citeloop-handoff-card-selected/);
```

For Results content/article handoffs:

```js
const focusBlock = source.slice(
  source.indexOf("const focusResultActionForHandoff"),
  source.indexOf("const closeResultSiteFixDrawer"),
);
assert.match(focusBlock, /setHighlightedResultActionID\(actionID\)/);
assert.doesNotMatch(focusBlock, /setSelectedResultActionID\(actionID\)/);
assert.doesNotMatch(focusBlock, /openTimer/);
assert.doesNotMatch(focusBlock, /setHighlightedResultActionID\(null\)/);
```

For Results Site Fix handoffs:

```js
const siteFixFocusBlock = seo.slice(
  seo.indexOf("const focusResultSiteFixForHandoff"),
  seo.indexOf("useEffect(() => clearResultHandoffTimers"),
);
assert.match(siteFixFocusBlock, /setHighlightedResultActionID\(summary\.id\)/);
assert.doesNotMatch(siteFixFocusBlock, /openResultSiteFix/);
assert.doesNotMatch(siteFixFocusBlock, /setSelectedResultSiteFixID/);
```

For watchlist selection:

```js
assert.match(source, /const \[highlightedWatchOpportunityID, setHighlightedWatchOpportunityID\]/);
assert.match(source, /setHighlightedWatchOpportunityID\(requestedWatchOpportunityID\)/);
assert.match(source, /highlightedWatchOpportunityID === item\.source_opportunity_id/);
```

For Recently Decided Site Fix routing:

```js
assert.match(source, /if \(surface === "Site Fixes"\) return `\/projects\/\$\{projectId\}\/site-fixes\?fix=\$\{action\.id\}`/);
```

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```bash
cd web
node --test \
  app/lib/action-portfolio-contract.test.mjs \
  app/lib/results-attribution-contract.test.mjs \
  app/lib/workflow-handoff-contract.test.mjs \
  app/lib/workflow-handoff-link-cards-contract.test.mjs
```

Expected: FAIL because Site Fixes opens its drawer from `initialFixId`, Results opens content and Site Fix drawers after delay, watchlist has no persistent highlight state, and Recently Decided omits the Site Fix target ID.

- [ ] **Step 3: Split Site Fix handoff highlight from drawer selection**

Add:

```ts
const [highlightedFixID, setHighlightedFixID] = useState<string | null>(null);
const siteFixCardRefs = useRef<Record<string, HTMLButtonElement | null>>({});
```

After `canonicalFixIDForAlias` resolves, set only `highlightedFixID`, then scroll and focus the card after the list renders. In `renderCard`, bind the ref, apply `citeloop-handoff-card-selected`, expose `aria-current`, and clear `highlightedFixID` inside `openFix` before setting `selectedID`.

- [ ] **Step 4: Route Recently Decided Site Fix cards with their alias**

Change:

```ts
if (surface === "Site Fixes") {
  return `/projects/${projectId}/site-fixes?fix=${action.id}`;
}
```

The existing `canonicalFixIDForAlias` remains the only canonical/legacy resolver.

- [ ] **Step 5: Remove Results content auto-drawer timers**

Simplify `focusResultActionForHandoff` to clear conflicting selected Site Fix state, set `highlightedResultActionID`, then schedule only scroll/focus. Remove the delayed `setSelectedResultActionID` and delayed highlight clear. Direct Results card clicks still clear the highlight and set the selected drawer ID.

- [ ] **Step 6: Remove Results Site Fix auto-detail opening**

Keep the async `resultSiteFixHandoff` reducer and pinning because it makes an off-page measurement visible even when the feed arrives in either order. Once resolved, call a focus helper that only:

```ts
setHighlightedResultActionID(summary.id);
window.setTimeout(() => {
  const target = document.querySelector<HTMLElement>(`[data-results-site-fix-card="${summary.id}"]`);
  if (!target) return;
  const reduced = window.matchMedia?.("(prefers-reduced-motion: reduce)")?.matches ?? false;
  target.scrollIntoView({ behavior: reduced ? "auto" : "smooth", block: "center" });
  target.focus({ preventScroll: true });
}, 120);
```

Do not fetch/open detail from the focus helper. A direct `SiteFixResultsCard` click continues to call `openResultSiteFix`.

- [ ] **Step 7: Persist watchlist target selection**

Add `highlightedWatchOpportunityID`, set it when `?watch=` resolves, apply `citeloop-handoff-card-selected` and `aria-current` to the matching watchlist card, and clear it when the user directly opens/acts on a watchlist card.

Replace `citeloop-linked-card-pulse` with `citeloop-handoff-card-selected` in Results content and Site Fix result cards. Keep ordinary selected-drawer styling as the secondary branch.

- [ ] **Step 8: Run focused tests and verify GREEN**

Run:

```bash
cd web
node --test \
  app/lib/action-portfolio-contract.test.mjs \
  app/lib/results-attribution-contract.test.mjs \
  app/lib/workflow-handoff-contract.test.mjs \
  app/lib/workflow-handoff-link-cards-contract.test.mjs
npx tsc --noEmit
```

Expected: all focused tests pass and TypeScript reports no errors.

- [ ] **Step 9: Commit Site Fix and Results behavior**

```bash
git add web/app/lib/action-portfolio-contract.test.mjs \
  web/app/lib/results-attribution-contract.test.mjs \
  web/app/lib/workflow-handoff-contract.test.mjs \
  web/app/lib/workflow-handoff-link-cards-contract.test.mjs \
  'web/app/projects/[id]/site-fixes/site-fixes-client.tsx' \
  'web/app/projects/[id]/seo/seo-client.tsx' \
  'web/app/projects/[id]/results/site-fix-results-card.tsx'
git commit -m "fix: keep workflow handoff targets drawer-free"
```

### Task 4: Full Verification, Review, PR, Deployment, and Production Check

**Files:**
- Verify all modified files.
- Update implementation only if verification exposes a gap.

- [ ] **Step 1: Run all local verification**

Run:

```bash
cd web
npm test
npx tsc --noEmit
npm run build
cd ..
go test ./...
git diff --check origin/main...HEAD
git status --short
```

Expected: 436 or more web tests pass, TypeScript passes, Next production build succeeds, all Go packages pass, no whitespace errors exist, and only intentional files are changed.

- [ ] **Step 2: Perform a local browser behavior check**

Start the development server:

```bash
cd web
npm run dev
```

Verify with the configured local project/session where available:

1. Doctor Add to Site Fixes stays on Doctor and removes the source finding.
2. Recent Findings opens Site Fixes with a persistent highlighted card and no drawer.
3. Recently Decided, Drafted, Reviewed, Published, and Watched links focus a persistent target without a drawer.
4. Clicking a highlighted target opens its normal detail drawer.
5. Reduced-motion mode uses non-smooth scrolling.

Expected: every checked path matches the design. If local authenticated data is unavailable, record which paths require production verification instead of inventing fixtures.

- [ ] **Step 3: Invoke verification and code-review skills**

Use `superpowers:verification-before-completion`, then `superpowers:requesting-code-review`. Address every actionable finding and rerun the relevant focused plus full verification commands.

- [ ] **Step 4: Push and open a ready PR**

```bash
git push -u origin codex/doctor-site-fix-handoff-navigation
```

Create a ready PR to `origin/main` with a summary of the Doctor state fix, no-navigation mutations, persistent handoff behavior, and verification evidence.

- [ ] **Step 5: Wait for checks and merge**

Wait for all required GitHub checks. If any check fails, diagnose it with `superpowers:systematic-debugging`, fix it with TDD, push, and wait again. Merge the PR only after all required checks pass.

- [ ] **Step 6: Wait for production deployment**

Inspect the production deployment for the merged commit until it reaches a successful terminal state. Do not treat the PR merge alone as completion.

- [ ] **Step 7: Verify production end to end**

Using the production project shown in the bug report:

1. Add an actionable Doctor finding to Site Fixes and confirm the URL remains on Doctor.
2. Confirm the finding disappears immediately and remains absent after reload.
3. Open Recent Findings, click the handoff card, and confirm Site Fixes highlights the correct canonical card without opening its drawer.
4. Click the highlighted card and confirm its drawer opens.
5. Exercise available Recently Decided, Drafted, Reviewed, Published, and Watched cards and confirm persistent highlight-only landings.

If any production behavior differs, return to a failing regression test, implement the smallest fix, push it through checks, and verify production again.

- [ ] **Step 8: Report completion**

Report the final PR link only after the merged production deployment and all available production scenarios pass.
