# Analysis Workflow Phase 3 Results Surface Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Results default to published-work outcomes, measurement states, and collapsed measurement details instead of a raw SEO operations console.

**Architecture:** Reuse the existing `SEOClient` fetches and `SEOContentAction` data. Add small presentation helpers in `web/app/projects/[id]/seo/seo-client.tsx` to derive waiting / positive / negative / inconclusive states from action status and `outcome_summary`. Keep existing diagnostics available, but move them behind an `Advanced diagnostics` disclosure so the default page answers what changed after publishing.

**Tech Stack:** Next.js App Router, React client component, existing UI primitives, Node contract tests, TypeScript.

---

## File Structure

- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
  - Adds Phase 3 Results contract assertions to the existing dashboard contract suite.
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
  - Adds Results outcome helpers, default outcome summary, measurement queue, AI citation summary, and collapses legacy diagnostics.

## Task 1: Write Phase 3 Results Contract Test

**Files:**
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [x] **Step 1: Add Results surface expectations**

Add this test near the Analysis / Results tests:

```js
test("results surface defaults to published outcomes with collapsed measurement details", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  const resultsStart = seo.indexOf('{mode === "results"');
  const nextAnalysisStart = seo.indexOf('{mode === "analysis" && (', resultsStart + 1);
  const resultsBlock = seo.slice(resultsStart, nextAnalysisStart);

  for (const copy of [
    "Outcome summary",
    "Published work",
    "Measurement queue",
    "Waiting",
    "Positive",
    "Negative",
    "Inconclusive",
    "Measurement details",
    "Measurement window",
    "AI citation signals",
    "No published work is measuring yet",
    "Advanced diagnostics",
  ]) {
    assert.match(resultsBlock, new RegExp(copy));
  }

  assert.match(seo, /function actionMeasurementState/);
  assert.match(seo, /const measuredActions = actions\.filter/);
  assert.match(resultsBlock, /<details[\s\S]*Measurement details/);
  assert.match(resultsBlock, /<details[\s\S]*Advanced diagnostics/);
  assert.doesNotMatch(resultsBlock, /Add to Content Plan/);
  assert.doesNotMatch(resultsBlock, /Dismiss/);
  assert.doesNotMatch(resultsBlock, /Opportunity queue/);
});
```

- [x] **Step 2: Verify RED**

Run:

```bash
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: FAIL because Results does not yet show outcome summary, measurement states, measurement details, or advanced diagnostics disclosure.

## Task 2: Add Results Outcome Helpers

**Files:**
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

- [x] **Step 1: Add measurement-state helper**

Add this helper above `SEOClient`:

```tsx
function actionMeasurementState(action: SEOContentAction) {
  const rawResult = String(action.outcome_summary?.result ?? action.outcome_summary?.state ?? "").toLowerCase();
  if (["improved", "positive", "won", "up"].includes(rawResult)) {
    return { key: "positive" as const, label: "Positive", tone: "green" as const, detail: "Measured signals improved after publishing." };
  }
  if (["worsened", "negative", "lost", "down"].includes(rawResult) || ["failed", "verification_failed", "recovery_required"].includes(action.status)) {
    return { key: "negative" as const, label: "Negative", tone: "red" as const, detail: "The result needs follow-up before it can be treated as a win." };
  }
  if (["inconclusive", "neutral", "flat"].includes(rawResult) || action.status === "completed") {
    return { key: "inconclusive" as const, label: "Inconclusive", tone: "amber" as const, detail: "The measurement window closed without a clear positive or negative signal." };
  }
  return { key: "waiting" as const, label: "Waiting", tone: "neutral" as const, detail: "Published work is still inside the measurement window." };
}
```

- [x] **Step 2: Add outcome summary helper**

Add this helper above `SEOClient`:

```tsx
function compactOutcomeText(outcome: any) {
  if (!outcome || (typeof outcome === "object" && Object.keys(outcome).length === 0)) return "No outcome summary yet.";
  if (typeof outcome === "string") return outcome;
  if (typeof outcome === "object") {
    return Object.entries(outcome)
      .slice(0, 5)
      .map(([key, value]) => `${key}: ${typeof value === "object" ? JSON.stringify(value) : String(value)}`)
      .join(" / ");
  }
  return String(outcome);
}
```

- [x] **Step 3: Compute measured action rows**

Inside `SEOClient`, after `latestPortfolioPlan`, add:

```tsx
const measuredActions = actions.filter((action) =>
  ["published", "measuring", "completed", "failed", "verification_failed", "recovery_required"].includes(action.status) ||
  Boolean(action.published_at || action.verified_at),
);
const outcomeCounts = measuredActions.reduce(
  (counts, action) => {
    counts[actionMeasurementState(action).key] += 1;
    return counts;
  },
  { waiting: 0, positive: 0, negative: 0, inconclusive: 0 },
);
const measurementExceptions = measuredActions.filter((action) => ["negative", "inconclusive"].includes(actionMeasurementState(action).key));
```

- [x] **Step 4: Verify helper types**

Run:

```bash
npm run typecheck
```

Expected: PASS.

## Task 3: Replace Results Default Surface

**Files:**
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

- [x] **Step 1: Add default Results summary before legacy diagnostics**

Inside `mode === "results"`, add a default `Outcome summary` section with four state cards:

```tsx
<SectionHeader
  title="Outcome summary"
  eyebrow="Published work"
  action={<Badge tone={measurementExceptions.length ? "amber" : measuredActions.length ? "green" : "neutral"}>{measuredActions.length}</Badge>}
/>
```

The four cards must render the labels `Waiting`, `Positive`, `Negative`, and `Inconclusive` with counts from `outcomeCounts`.

- [x] **Step 2: Add measurement queue cards**

Add a `Measurement queue` section. If `measuredActions.length === 0`, render:

```tsx
<EmptyState title="No published work is measuring yet" detail="Published or URL-verified actions will appear here once the Publish step finishes." />
```

Otherwise render cards for `measuredActions.slice(0, 12)`. Each card should show the action state, action type, target URL, published date, and `Measurement window`.

- [x] **Step 3: Collapse per-action details**

Inside each measurement card, add:

```tsx
<details className="mt-3 rounded-lg border border-slate-100 bg-slate-50 px-3 py-2">
  <summary className="cursor-pointer text-sm font-semibold text-slate-700">Measurement details</summary>
  ...
</details>
```

The details body should include `Outcome`, `Measurement window`, `Verification`, and `Target URL`.

- [x] **Step 4: Add AI citation summary**

Add an `AI citation signals` section before diagnostics with compact cards for visibility score, coverage, prompts, and asset briefs using existing `geoOverview`, `showGeoScore`, `geoScoreValue`, `geoCoverage`, and `assetBriefs`.

- [x] **Step 5: Collapse legacy Results tooling**

Wrap the existing Results-only `Setup`, traffic metric cards, `Settings`, `GEO crawler access`, `GEO visibility`, and `Autopilot` sections in:

```tsx
<details className="rounded-xl border border-slate-200 bg-white p-4">
  <summary className="cursor-pointer text-sm font-bold text-slate-900">Advanced diagnostics</summary>
  <div className="mt-4 space-y-7">
    ...
  </div>
</details>
```

Expected: those tools remain available, but the default visible Results page is outcome-first.

- [x] **Step 6: Verify GREEN**

Run:

```bash
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
npm run typecheck
```

Expected: both pass.

## Task 4: Full Verification and Commit

**Files:**
- Modified files from Tasks 1-3.

- [x] **Step 1: Run checks**

Run:

```bash
npm test
npm run typecheck
make test
VERCEL_ENV=preview npm run build
git diff --check
```

Expected: all commands exit 0. `npm run build` may keep the existing multi-lockfile warning, but must finish successfully under `VERCEL_ENV=preview`.

- [x] **Step 2: Commit Phase 3**

Run:

```bash
git add docs/superpowers/plans/2026-06-24-analysis-workflow-phase3-results-surface.md \
  web/app/lib/dashboard-ux-phase1-contract.test.mjs \
  web/app/projects/[id]/seo/seo-client.tsx
git commit -m "feat: productize results surface"
```

Expected: commit succeeds.

## Self-Review

- Spec coverage: Covers Phase 3 AC for Results outcome ownership, no opportunity acceptance in Results, measurement windows, waiting/inconclusive/positive/negative states, and collapsed long details. It does not implement new measurement ingestion; it uses existing `content_actions` fields.
- Placeholder scan: No placeholders remain.
- Type consistency: Helper functions use existing `SEOContentAction`, `Badge` tone values, `measurementWindowLabel`, `formatDate`, `metric`, and `percent`.
