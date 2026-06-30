# Analysis Review Decision Drawer Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign Analysis and Review decision surfaces so both use metric cards, standalone section titles, card grids, and wider animated decision drawers.

**Architecture:** This is a frontend-only change. Keep data loading, mutations, and API contracts unchanged; only reshape the React/Tailwind presentation in `seo-client.tsx`, `review-client.tsx`, and shared drawer animation CSS.

**Tech Stack:** Next.js App Router, React client components, TypeScript, Tailwind CSS v3, lucide-react icons, Node built-in test runner.

---

## File Structure

- Modify `web/app/lib/dashboard-ux-phase1-contract.test.mjs`: add failing contract assertions for Analysis and Review layout markers, drawer width, animations, and fixed action areas.
- Modify `web/app/globals.css`: add shared `citeloop-drawer-scrim-in` and `citeloop-drawer-panel-in` keyframes.
- Modify `web/app/projects/[id]/seo/seo-client.tsx`: move `Growth findings` out of the framed findings container and widen/animate the existing Analysis drawer.
- Modify `web/app/projects/[id]/review/review-client.tsx`: replace split-pane queue with Review metric cards, standalone `Needs Your Decision`, card grid, and a right-side drawer with fixed bottom actions.

## Task 1: Contract Tests

**Files:**
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [ ] **Step 1: Write the failing test assertions**

Add assertions to the existing `analysis surface uses a compact GSC status control and keeps decisions out of large cards` test:

```js
assert.match(seo, /data-analysis-growth-findings-section/);
assert.match(seo, /data-analysis-finding-card/);
assert.match(seo, /data-analysis-drawer/);
assert.match(seo, /animate-\[citeloop-drawer-panel-in_220ms_cubic-bezier\(0\.16,1,0\.3,1\)\]/);
assert.match(seo, /animate-\[citeloop-drawer-scrim-in_180ms_ease-out\]/);
assert.match(seo, /max-w-2xl/);
assert.doesNotMatch(seo, /<div className="min-w-0 rounded-xl border border-slate-200 bg-white">\s*<div className="flex flex-col gap-3 border-b border-slate-100 p-4/);
```

Add assertions to the existing `review page is built around automatic recovery, not manual triage` test:

```js
for (const copy of [
  "Overall Metrics",
  "Total in review",
  "data-review-overall-metrics",
  "data-review-metric-card",
  "data-review-decision-section",
  "data-review-card",
  "data-review-drawer",
  "Review drawer actions",
]) {
  assert.match(review, new RegExp(copy));
}
assert.match(review, /animate-\[citeloop-drawer-panel-in_220ms_cubic-bezier\(0\.16,1,0\.3,1\)\]/);
assert.match(review, /animate-\[citeloop-drawer-scrim-in_180ms_ease-out\]/);
assert.match(review, /max-w-2xl/);
assert.doesNotMatch(review, /xl:grid-cols-\[minmax\(0,1fr\)_minmax\(420px,0\.9fr\)\]/);
assert.doesNotMatch(review, /Loading the first draft/);
```

- [ ] **Step 2: Run the targeted test to verify it fails**

Run:

```bash
cd web
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: FAIL because the new marker strings and drawer animation classes are not present yet.

- [ ] **Step 3: Commit the red test**

```bash
git add web/app/lib/dashboard-ux-phase1-contract.test.mjs
git commit -m "test: cover analysis and review drawer redesign"
```

## Task 2: Shared Drawer Animation CSS

**Files:**
- Modify: `web/app/globals.css`

- [ ] **Step 1: Add shared drawer keyframes**

Add these keyframes near the existing drawer animations:

```css
@keyframes citeloop-drawer-scrim-in {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

@keyframes citeloop-drawer-panel-in {
  from {
    opacity: 0;
    transform: translateX(32px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}
```

- [ ] **Step 2: Run the targeted test**

Run:

```bash
cd web
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: still FAIL because React markup is not updated yet.

## Task 3: Analysis Page Layout and Drawer

**Files:**
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

- [ ] **Step 1: Move Growth findings title outside the framed container**

In the `mode === "analysis"` block, replace the current `<section className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">` wrapper and its left framed findings container with this shape:

```tsx
<section data-analysis-growth-findings-section className="space-y-3">
  <SectionHeader
    title="Growth findings"
    eyebrow="Decision-ready recommendations"
    action={
      <div className="flex flex-wrap gap-2">
        <Badge tone={opportunities.length ? "green" : "neutral"}>{opportunities.length ? "Ready to review" : "No review needed"}</Badge>
        <Badge tone="neutral">{loopActiveCount} in loop</Badge>
      </div>
    }
  />

  <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
    <div className="min-w-0">
      {opportunities.length === 0 ? (
        <EmptyState
          title="No analysis to review"
          detail="Refresh or Sync after Context changes. New findings will appear here when they need a decision."
        />
      ) : (
        <div className="grid gap-3 lg:grid-cols-2">
          {opportunities.slice(0, 12).map((opp) => {
            const cta = actionCtaForOpportunity(opp);
            return (
              <button
                data-analysis-finding-card
                key={opp.id}
                type="button"
                onClick={() => setSelectedOpportunityID(opp.id)}
                aria-label={`Open finding details: ${opportunityTitle(opp)}`}
                className={`group min-w-0 rounded-xl border bg-white p-4 text-left shadow-sm transition hover:-translate-y-0.5 hover:border-slate-300 hover:shadow-md active:translate-y-0 ${
                  selectedOpportunityID === opp.id ? "border-slate-400 ring-2 ring-slate-200" : "border-slate-200"
                }`}
              >
                <div className="flex flex-wrap items-center gap-2">
                  <Badge tone="blue">{findingTypeLabel(opp)}</Badge>
                  <Badge tone={toneForRisk(opp.risk_level)}>{opp.risk_level ?? "risk unknown"}</Badge>
                  <Badge tone="neutral">{sourceModeForOpportunity(opp, overview)}</Badge>
                </div>
                <div className="mt-3 flex items-start justify-between gap-3">
                  <h3 className="min-w-0 text-base font-bold leading-6 text-slate-950">{opportunityTitle(opp)}</h3>
                  <span className="shrink-0 font-mono text-xs font-bold uppercase text-slate-400">Score {metric(opp.priority_score)}</span>
                </div>
                <p className="mt-2 line-clamp-3 text-sm leading-6 text-slate-600">
                  {opp.expected_impact || "Review this finding against confirmed Context before creating downstream work."}
                </p>
                <div className="mt-4 grid gap-2 border-t border-slate-100 pt-3 text-xs leading-5 text-slate-500">
                  <div className="min-w-0 truncate">
                    <span className="font-semibold uppercase tracking-[0.1em] text-slate-400">Signal</span>{" "}
                    <span className="font-medium text-slate-700">{opp.query ?? opp.page_url ?? opp.normalized_page_url ?? "Project domain"}</span>
                  </div>
                  <div className="flex items-center justify-between gap-3">
                    <span className="truncate font-medium text-slate-500">{cta.label}</span>
                    <span className="font-semibold text-slate-700 transition group-hover:translate-x-0.5">Open details</span>
                  </div>
                </div>
              </button>
            );
          })}
        </div>
      )}
    </div>
  </div>
</section>
```

After the left column, move the current `<aside className="space-y-4">` block that starts with `Loop in motion` into the same grid. Keep the Loop in motion and Data note JSX unchanged. Do not keep the old framed findings header.

- [ ] **Step 2: Widen and animate the Analysis drawer**

In the selected opportunity drawer, update the scrim button and drawer panel:

```tsx
<button
  type="button"
  aria-label="Close finding details"
  onClick={() => setSelectedOpportunityID(null)}
  className="absolute inset-0 animate-[citeloop-drawer-scrim-in_180ms_ease-out] bg-slate-950/25"
/>
<aside
  data-analysis-drawer
  role="dialog"
  aria-modal="true"
  aria-labelledby="finding-details-title"
  className="absolute right-0 top-0 flex h-[100dvh] max-h-[100dvh] w-full max-w-2xl animate-[citeloop-drawer-panel-in_220ms_cubic-bezier(0.16,1,0.3,1)] flex-col overflow-hidden border-l border-slate-200 bg-white shadow-2xl"
>
```

- [ ] **Step 3: Run the targeted test**

Run:

```bash
cd web
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: Analysis assertions pass; Review assertions still FAIL.

## Task 4: Review Page Metric Cards and Card Grid

**Files:**
- Modify: `web/app/projects/[id]/review/review-client.tsx`

- [ ] **Step 1: Stop auto-opening the first draft**

Remove this effect:

```tsx
useEffect(() => {
  if (selectedArticleId || queueArticles.length === 0) return;
  setSelectedArticleId(queueArticles[0].article.id);
}, [queueArticles, selectedArticleId]);
```

- [ ] **Step 2: Replace split-pane wrapper with Overall Metrics and standalone Needs Your Decision**

Use this structure when `summary.total > 0`:

```tsx
<>
  <section data-review-overall-metrics className="space-y-3">
    <SectionHeader title="Overall Metrics" eyebrow="Review queue status" />
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <ReviewMetricCard label="Needs your decision" value={summary.needsHuman} detail="Only rare manual calls" tone="red" />
      <ReviewMetricCard label="Ready to approve" value={summary.ready} detail="QA cleared these drafts" tone="green" />
      <ReviewMetricCard label="CiteLoop is handling" value={summary.recovering} detail="Re-checking, repairing, regenerating" tone="amber" />
      <ReviewMetricCard label="Total in review" value={summary.total} detail="Drafts currently visible here" tone="neutral" />
    </div>
  </section>

  <section data-review-decision-section className="space-y-3">
    <SectionHeader title="Needs Your Decision" eyebrow="Open a card to inspect details and act" action={<Badge tone="neutral">{queueArticles.length}</Badge>} />
    {queueArticles.length === 0 ? (
      <EmptyState title="No review cards" detail="Drafts that need a decision or approval will appear here." />
    ) : (
      <div className="grid gap-3 lg:grid-cols-2">
        {queueArticles.map((item) => (
          <ReviewDecisionCard
            key={item.article.id}
            item={item}
            selected={selectedArticleId === item.article.id}
            onSelect={() => setSelectedArticleId(item.article.id)}
          />
        ))}
      </div>
    )}
  </section>
</>
```

Create `ReviewMetricCard`:

```tsx
function ReviewMetricCard({
  label,
  value,
  detail,
  tone,
}: {
  label: string;
  value: number;
  detail: string;
  tone: "green" | "amber" | "red" | "neutral";
}) {
  const valueClass = {
    green: "text-green-700",
    amber: "text-amber-700",
    red: value > 0 ? "text-red-700" : "text-slate-950",
    neutral: "text-slate-950",
  }[tone];

  return (
    <div data-review-metric-card className="rounded-xl border border-slate-200 bg-white p-4">
      <div className="text-[11px] font-bold uppercase tracking-[0.12em] text-slate-500">{label}</div>
      <div className={cx("mt-3 text-2xl font-bold leading-none", valueClass)}>{value}</div>
      <div className="mt-2 text-[13px] font-semibold leading-5 text-slate-400">{detail}</div>
    </div>
  );
}
```

Create `ReviewDecisionCard`:

```tsx
function ReviewDecisionCard({
  item,
  selected,
  onSelect,
}: {
  item: QueueArticle;
  selected: boolean;
  onSelect: () => void;
}) {
  const { article, topicId } = item;
  const state = reviewArticleState(article);
  const title = articleReviewTitle(article);

  return (
    <button
      data-review-card
      type="button"
      onClick={onSelect}
      aria-label={`Open review details: ${title}`}
      className={cx(
        "group min-w-0 rounded-xl border bg-white p-4 text-left shadow-sm transition hover:-translate-y-0.5 hover:border-slate-300 hover:shadow-md active:translate-y-0",
        selected ? "border-slate-400 ring-2 ring-slate-200" : "border-slate-200",
      )}
    >
      <div className="flex flex-wrap items-center gap-2">
        <StateBadge state={state} />
        <Badge tone={article.kind === "canonical" ? "green" : "neutral"}>{article.platform || article.kind}</Badge>
        <span className="text-xs font-semibold text-slate-400">Topic {topicId.slice(0, 8)}</span>
      </div>
      <h3 className="mt-3 text-base font-bold leading-6 text-slate-950">{title}</h3>
      <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-slate-100 pt-3 text-xs text-slate-500">
        {state.kind === "recovering" ? (
          <span className="inline-flex items-center gap-1.5 font-semibold text-amber-700">
            <Loader2 size={12} className="animate-spin" />
            {article.repair_status === "repairing" ? "Repairing draft" : "Re-checking with QA"}
          </span>
        ) : (
          <>
            <span>geo {formatScore(article.geo_score)}</span>
            <span>seo {formatScore(article.seo_score)}</span>
          </>
        )}
        <span className="ml-auto font-semibold text-slate-700 transition group-hover:translate-x-0.5">Open details</span>
      </div>
    </button>
  );
}
```

Delete `SummaryCard`, `QueueSection`, and `ReviewQueueRow` after replacing their call sites.

- [ ] **Step 3: Run the targeted test**

Run:

```bash
cd web
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: Review metric/card assertions pass; drawer assertions still FAIL.

## Task 5: Review Drawer and Fixed Bottom Actions

**Files:**
- Modify: `web/app/projects/[id]/review/review-client.tsx`

- [ ] **Step 1: Add Escape key and body scroll lock for selected Review article**

Add effects below the selected article memo:

```tsx
useEffect(() => {
  if (!selectedArticle) return;
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key === "Escape") setSelectedArticleId(null);
  };
  document.addEventListener("keydown", onKeyDown);
  return () => document.removeEventListener("keydown", onKeyDown);
}, [selectedArticle]);

useEffect(() => {
  if (!selectedArticle) return;
  const previousBodyOverflow = document.body.style.overflow;
  document.body.style.overflow = "hidden";
  return () => {
    document.body.style.overflow = previousBodyOverflow;
  };
}, [selectedArticle]);
```

- [ ] **Step 2: Render ReviewInspector inside an animated drawer overlay**

Render this after the Review sections when `selectedArticle` exists:

```tsx
{selectedArticle && (
  <div className="fixed inset-0 z-30">
    <button
      type="button"
      aria-label="Close review details"
      onClick={() => setSelectedArticleId(null)}
      className="absolute inset-0 animate-[citeloop-drawer-scrim-in_180ms_ease-out] bg-slate-950/25"
    />
    <ReviewInspector
      article={selectedArticle}
      topicId={selectedQueueArticle?.topicId ?? selectedArticle.topic_id}
      projectId={projectId}
      busy={selectedBusy}
      approveBusy={busy === `approve-${selectedArticle.id}` || busy === "bulk-approve"}
      rejectBusy={busy === `reject-${selectedArticle.id}`}
      saveBusy={busy === `save-${selectedArticle.id}`}
      editorOpen={editorOpen}
      content={content}
      onContentChange={setContent}
      onToggleEditor={() => setEditorOpen((value) => !value)}
      onApprove={() => onApprove(selectedArticle)}
      onReject={() => onReject(selectedArticle)}
      onSave={(next) => onSave(selectedArticle, next)}
      onApplyFix={(optionIndex, instruction) => onApplyFix(selectedArticle, optionIndex, instruction)}
      applyingIndex={busy?.startsWith(`apply-${selectedArticle.id}-`) ? Number(busy.split("-").pop()) : null}
      onRecheck={() => onRecheck(selectedArticle)}
      recheckBusy={busy === `recheck-${selectedArticle.id}`}
      onClose={() => setSelectedArticleId(null)}
    />
  </div>
)}
```

Add `onClose: () => void` to `ReviewInspector` props. Implement `ReviewInspector` as the drawer panel itself:

```tsx
<aside
  data-review-drawer
  role="dialog"
  aria-modal="true"
  aria-labelledby="review-details-title"
  className="absolute right-0 top-0 flex h-[100dvh] max-h-[100dvh] w-full max-w-2xl animate-[citeloop-drawer-panel-in_220ms_cubic-bezier(0.16,1,0.3,1)] flex-col overflow-hidden border-l border-slate-200 bg-white shadow-2xl"
>
```

Use `review-details-title` on the drawer heading. Add a close icon button in the drawer header.

- [ ] **Step 3: Move repeated primary actions to a fixed drawer footer**

Keep QA explanation and fix options in the body. Remove the bottom `Edit draft` and `Reject` button row from `DecisionPanel`. Remove the action button row from `ReadyPanel`.

Add this footer inside `ReviewInspector`, after the scrollable body:

```tsx
<ReviewDrawerActions
  article={article}
  state={state}
  busy={busy}
  approveBusy={approveBusy}
  rejectBusy={rejectBusy}
  recheckBusy={recheckBusy}
  previewHref={previewHref}
  detailHref={detailHref}
  onApprove={onApprove}
  onReject={onReject}
  onToggleEditor={onToggleEditor}
  onRecheck={onRecheck}
/>
```

Create `ReviewDrawerActions`:

```tsx
function ReviewDrawerActions({
  state,
  busy,
  approveBusy,
  rejectBusy,
  recheckBusy,
  previewHref,
  detailHref,
  onApprove,
  onReject,
  onToggleEditor,
  onRecheck,
}: {
  article: Article;
  state: ReviewArticleState;
  busy: boolean;
  approveBusy: boolean;
  rejectBusy: boolean;
  recheckBusy: boolean;
  previewHref: string;
  detailHref: string;
  onApprove: () => void;
  onReject: () => void;
  onToggleEditor: () => void;
  onRecheck: () => void;
}) {
  return (
    <div
      aria-label="Review drawer actions"
      className="shrink-0 flex flex-col gap-2 border-t border-slate-200 bg-white px-4 pb-[calc(1.5rem+env(safe-area-inset-bottom))] pt-4 sm:flex-row sm:justify-end"
    >
      {state.kind === "recovering" && (
        <>
          <a href={previewHref} target="_blank" rel="noopener noreferrer" className="inline-flex h-8 items-center justify-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50">
            Preview <ExternalLink size={14} />
          </a>
          <a href={detailHref} className="inline-flex h-8 items-center justify-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50">
            Detail
          </a>
        </>
      )}
      {state.kind === "needs_human" && (
        <>
          <Button disabled={busy} size="sm" onClick={onRecheck}>
            <ButtonProgress busy={recheckBusy} busyLabel="Re-running QA" idleIcon={<RefreshCw size={14} />}>
              Re-run QA
            </ButtonProgress>
          </Button>
          <Button disabled={busy} size="sm" onClick={onToggleEditor}>
            <FileText size={14} />
            Edit draft
          </Button>
          <Button disabled={busy} size="sm" variant="danger" onClick={onReject}>
            <ButtonProgress busy={rejectBusy} busyLabel="Rejecting" idleIcon={<XCircle size={14} />}>
              Reject
            </ButtonProgress>
          </Button>
        </>
      )}
      {state.kind === "ready" && (
        <>
          <Button disabled={busy} size="sm" onClick={onToggleEditor}>
            <FileText size={14} />
            Edit draft
          </Button>
          <a href={previewHref} target="_blank" rel="noopener noreferrer" className="inline-flex h-8 items-center justify-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50">
            Preview <ExternalLink size={14} />
          </a>
          <a href={detailHref} className="inline-flex h-8 items-center justify-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50">
            Detail
          </a>
          <Button disabled={busy} size="sm" variant="primary" onClick={onApprove}>
            <ButtonProgress busy={approveBusy} busyLabel="Approving" idleIcon={<CheckCircle2 size={14} />}>
              Approve
            </ButtonProgress>
          </Button>
        </>
      )}
    </div>
  );
}
```

- [ ] **Step 4: Run the targeted test**

Run:

```bash
cd web
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: PASS.

## Task 6: Full Verification

**Files:**
- No source edits unless verification finds a bug.

- [ ] **Step 1: Run all web tests**

```bash
cd web
npm test
```

Expected: 0 failures.

- [ ] **Step 2: Run TypeScript typecheck**

```bash
cd web
npm run typecheck
```

Expected: exit code 0.

- [ ] **Step 3: Run production build**

```bash
cd web
npm run build
```

Expected: exit code 0.

- [ ] **Step 4: Run Go tests**

```bash
go test ./...
```

Expected: exit code 0.

- [ ] **Step 5: Commit implementation**

```bash
git add web/app/lib/dashboard-ux-phase1-contract.test.mjs web/app/globals.css 'web/app/projects/[id]/seo/seo-client.tsx' 'web/app/projects/[id]/review/review-client.tsx'
git commit -m "feat: redesign analysis and review decision drawers"
```

## Task 7: Browser and Production Follow-Through

**Files:**
- No source edits unless verification finds a bug.

- [ ] **Step 1: Run local app for visual verification**

```bash
cd web
npm run dev
```

Expected: local Next.js server starts successfully.

- [ ] **Step 2: Verify local Analysis and Review pages**

Open:

```text
http://localhost:3000/projects/1459b054-cdc3-4d9b-9dd4-18e12458c61a/analysis
http://localhost:3000/projects/1459b054-cdc3-4d9b-9dd4-18e12458c61a/review
```

Expected: Analysis and Review render, cards fit, drawers slide in, drawer actions stay fixed.

- [ ] **Step 3: Push branch and create PR**

```bash
git push -u origin codex/review-analysis-redesign
gh pr create --base main --head codex/review-analysis-redesign --title "Redesign Analysis and Review decision drawers" --body "Redesigns Analysis and Review decision surfaces with metric cards, standalone section titles, card grids, and wider animated drawers."
```

- [ ] **Step 4: Merge PR**

```bash
gh pr merge --squash --delete-branch
```

- [ ] **Step 5: Wait for deployment and verify production**

Verify:

```text
https://citeloop.app/projects/1459b054-cdc3-4d9b-9dd4-18e12458c61a/analysis
https://citeloop.app/projects/1459b054-cdc3-4d9b-9dd4-18e12458c61a/review
```

Expected: production matches the accepted redesign. If production differs, fix, push, merge, wait for redeploy, and verify again.
