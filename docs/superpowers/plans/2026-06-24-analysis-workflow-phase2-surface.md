# Analysis Workflow Phase 2 Surface Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Productize the canonical Analysis page so it is action-first, explains search-data readiness, and keeps raw evidence collapsed by default.

**Architecture:** Reuse the existing `SEOClient` data fetching and API contracts. Add small local presentation helpers inside `web/app/projects/[id]/seo/seo-client.tsx` for search-data status and evidence summaries. Keep Results mode behavior unchanged.

**Tech Stack:** Next.js App Router, React client component, existing UI primitives, Node contract tests, TypeScript.

---

## File Structure

- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
  - Adds Phase 2 Analysis surface contract assertions to the existing dashboard contract suite.
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
  - Adds Analysis search-data status card, weekly brief shell, opportunity queue section, and collapsed evidence details.

## Task 1: Write Phase 2 Analysis Contract Test

**Files:**
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`

- [x] **Step 1: Add Analysis surface expectations**

Add this test near the existing Analysis/Results tests:

```js
test("analysis surface is action-first with search-data status and collapsed evidence", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");

  for (const copy of [
    "Search data status",
    "Search data not connected",
    "Search Console connected",
    "Connect Search Console",
    "Weekly analysis brief",
    "Opportunity queue",
    "Recommendation",
    "View evidence",
    "Evidence",
    "Confidence",
    "No analysis to review",
  ]) {
    assert.match(seo, new RegExp(copy));
  }

  assert.match(seo, /<details[\s\S]*View evidence/);
  assert.match(seo, /api\.listSEOOpportunities\(projectId, \{ status: "open", limit: 50 \}\)/);
  assert.doesNotMatch(seo, /Raw GSC rows/);
  assert.doesNotMatch(seo, /Full signal table/);
});
```

- [x] **Step 2: Verify RED**

Run:

```bash
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: FAIL because the Analysis page does not yet have search-data status, weekly analysis brief shell, or `View evidence`.

## Task 2: Add Search Data Status and Analysis Queue Structure

**Files:**
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

- [x] **Step 1: Add a local search-data presentation helper**

Add a helper above `SEOClient`:

```tsx
function analysisSearchDataStatus(overview: SEOOverview | null, gscStatus: string) {
  const capabilityMode = overview?.capability_mode ?? "public_only";
  const integration = overview?.integrations.find((item) => item.provider === "google_search_console");
  if (integration?.status === "connected" || capabilityMode === "customer_site_connected" || capabilityMode === "managed_content_connected") {
    return {
      tone: "green" as const,
      label: "Search Console connected",
      detail: "CiteLoop can use first-party search data when prioritizing recommendations.",
      action: null,
    };
  }
  if (["error", "expired", "revoked", "stale"].includes(gscStatus)) {
    return {
      tone: "red" as const,
      label: "Search Console needs attention",
      detail: "Reconnect Search Console before trusting fresh query, CTR, or position signals.",
      action: "Reconnect Search Console",
    };
  }
  if (capabilityMode === "customer_site_pending_verification") {
    return {
      tone: "amber" as const,
      label: "Search Console verification pending",
      detail: "Finish site verification to unlock first-party query, CTR, position, and decay signals.",
      action: "Finish setup",
    };
  }
  return {
    tone: "amber" as const,
    label: "Search data not connected",
    detail: "CiteLoop can still review public opportunities. Connect Search Console for query, CTR, position, and content decay evidence.",
    action: "Connect Search Console",
  };
}
```

- [x] **Step 2: Add an evidence summary helper**

Add:

```tsx
function compactEvidenceText(evidence: any) {
  if (!evidence) return "No structured evidence yet.";
  if (typeof evidence === "string") return evidence;
  if (Array.isArray(evidence)) return evidence.slice(0, 3).map(String).join(" / ");
  if (typeof evidence === "object") {
    return Object.entries(evidence)
      .slice(0, 5)
      .map(([key, value]) => `${key}: ${typeof value === "object" ? JSON.stringify(value) : String(value)}`)
      .join(" / ");
  }
  return String(evidence);
}
```

- [x] **Step 3: Compute status in the component**

Inside `SEOClient`, after `visibilityBlockers`, add:

```tsx
const analysisStatus = analysisSearchDataStatus(overview, gscStatus);
```

- [x] **Step 4: Replace the Analysis section shell**

Inside `mode === "analysis"`, add a first card titled `Search data status`, a section title `Opportunity queue`, and keep only decision-ready cards visible by default.

The Search data card should use:

```tsx
<div className="text-xs font-semibold uppercase text-slate-400">Search data status</div>
<Badge tone={analysisStatus.tone}>{analysisStatus.label}</Badge>
{analysisStatus.action && (
  <a href={`/projects/${projectId}/settings`} className="...">
    {analysisStatus.action}
  </a>
)}
```

- [x] **Step 5: Add collapsed evidence to every opportunity card**

Inside each opportunity card, below the source page row, add:

```tsx
<details className="mt-3 rounded-lg border border-slate-100 bg-slate-50 px-3 py-2">
  <summary className="cursor-pointer text-sm font-semibold text-slate-700">View evidence</summary>
  <div className="mt-3 grid gap-2 text-xs leading-5 text-slate-600 md:grid-cols-2">
    <div><span className="font-semibold text-slate-800">Evidence</span><br />{compactEvidenceText(opp.evidence)}</div>
    <div><span className="font-semibold text-slate-800">Confidence</span><br />{metric(opp.confidence, 2)}</div>
    <div><span className="font-semibold text-slate-800">Query</span><br />{opp.query ?? "Not query-specific"}</div>
    <div><span className="font-semibold text-slate-800">Effort</span><br />{opp.effort ?? "Unknown"}</div>
  </div>
</details>
```

- [x] **Step 6: Rename the empty state**

Change:

```text
No opportunities to review
```

to:

```text
No analysis to review
```

- [x] **Step 7: Rename the existing brief details shell**

Change the details summary title fallback from `Visibility brief` to `Weekly analysis brief`.

- [x] **Step 8: Verify GREEN**

Run:

```bash
npm test -- app/lib/dashboard-ux-phase1-contract.test.mjs
npm run typecheck
```

Expected: both pass.

## Task 3: Full Verification and Commit

**Files:**
- Modified files from Tasks 1-2.

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

- [x] **Step 2: Commit Phase 2**

Run:

```bash
git add docs/superpowers/plans/2026-06-24-analysis-workflow-phase2-surface.md \
  web/app/lib/dashboard-ux-phase1-contract.test.mjs \
  web/app/projects/[id]/seo/seo-client.tsx
git commit -m "feat: productize analysis surface"
```

Expected: commit succeeds.

## Self-Review

- Spec coverage: Covers Phase 2 AC for Analysis-owned acceptance, action-first queue, connection status, no-opportunity state, collapsed evidence, and open-only default queue. It does not implement OAuth states that require Phase 4 connection infrastructure.
- Placeholder scan: No placeholders remain.
- Type consistency: The helper uses existing `SEOOverview`, `SEOIntegration.status`, `capability_mode`, and existing UI `Badge` tone values.
