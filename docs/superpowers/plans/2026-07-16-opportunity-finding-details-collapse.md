# Opportunity Finding Details Collapse Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Opportunity Finding stage summaries and queue counts default to a user-controlled collapsed state.

**Architecture:** Keep disclosure state local to `OpportunityFindingStatusPanel`, reuse the controlled button-and-chevron pattern already present in `OpportunityFindingProgress`, and conditionally render the existing summary grid plus counts as one accessible region. No API, persistence, or backend changes are required.

**Tech Stack:** React 18, Next.js 15 App Router, TypeScript, Tailwind CSS, lucide-react, Node.js contract tests.

---

## File Map

- `web/app/lib/seo-client-contract.test.mjs`: owns the regression contract for the Analysis Opportunity Finding panel.
- `web/app/projects/[id]/seo/seo-client.tsx`: owns the panel state, disclosure control, and conditional details region.

### Task 1: Add the default-collapsed run details disclosure

**Files:**
- Modify: `web/app/lib/seo-client-contract.test.mjs:109-175`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx:6`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx:910-1025`

- [ ] **Step 1: Write the failing contract test**

Add this test after `Analysis page exposes Opportunity Finding run status`:

```javascript
test("Opportunity Finding run details default closed behind an accessible disclosure", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const panelStart = source.indexOf("function OpportunityFindingStatusPanel");
  const panelEnd = source.indexOf("function GrowthRadarDiagnosticsPanel");
  const panelSource = source.slice(panelStart, panelEnd);

  assert.match(panelSource, /const \[runDetailsExpanded, setRunDetailsExpanded\] = useState\(false\)/);
  assert.match(
    panelSource,
    /data-opportunity-finding-details-toggle[\s\S]*aria-expanded=\{runDetailsExpanded\}[\s\S]*aria-controls="opportunity-finding-run-details"[\s\S]*Run details/,
  );
  assert.match(
    panelSource,
    /\{runDetailsExpanded && \([\s\S]*data-opportunity-finding-run-details[\s\S]*summary\.slice\(0, 5\)[\s\S]*status\.counts\.open[\s\S]*status\.counts\.in_loop[\s\S]*status\.counts\.processed/,
  );

  const errorIndex = panelSource.indexOf("data-opportunity-finding-error");
  const toggleIndex = panelSource.indexOf("data-opportunity-finding-details-toggle");
  assert.ok(errorIndex > -1 && errorIndex < toggleIndex, "durable run errors must remain outside the collapsed details region");
});
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd web
node --test app/lib/seo-client-contract.test.mjs
```

Expected: FAIL in `Opportunity Finding run details default closed behind an accessible disclosure` because `runDetailsExpanded` and the disclosure attributes are absent.

- [ ] **Step 3: Implement the minimal disclosure**

Extend the lucide import in `seo-client.tsx` with `ChevronDown`:

```typescript
import { BarChart3, CheckCircle2, ChevronDown, ChevronRight, Clipboard, FileText, History, RefreshCw, Search, Settings, ShieldAlert, X } from "lucide-react";
```

At the start of `OpportunityFindingStatusPanel`, before `manualMode`, add local state:

```typescript
const [runDetailsExpanded, setRunDetailsExpanded] = useState(false);
```

Replace the existing summary grid and count row with this controlled disclosure, keeping it after the durable error alert:

```tsx
<div className="mt-4">
  <button
    type="button"
    data-opportunity-finding-details-toggle
    aria-expanded={runDetailsExpanded}
    aria-controls="opportunity-finding-run-details"
    onClick={() => setRunDetailsExpanded((value) => !value)}
    className="inline-flex items-center gap-1 rounded-md px-1 text-xs font-semibold text-emerald-700 transition hover:bg-emerald-100/70 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-emerald-500"
  >
    <ChevronDown
      aria-hidden="true"
      size={14}
      className={cx("transition-transform", runDetailsExpanded ? "" : "-rotate-90")}
    />
    Run details
  </button>

  {runDetailsExpanded && (
    <div id="opportunity-finding-run-details" data-opportunity-finding-run-details>
      <div className="mt-3 grid gap-2 md:grid-cols-2 xl:grid-cols-5">
        {summary.slice(0, 5).map((item) => (
          <div key={`${item.label}-${item.detail}`} className="rounded-lg bg-white/75 px-3 py-2 ring-1 ring-white/80">
            <div className="text-xs font-bold uppercase text-slate-500">{item.label}</div>
            <div className="mt-1 text-sm font-semibold leading-5 text-slate-800">{item.detail}</div>
          </div>
        ))}
      </div>

      {status && (
        <div className="mt-3 flex flex-wrap gap-2 text-xs font-semibold text-slate-600">
          <span>{status.counts.open} open</span>
          <span>{status.counts.in_loop} in loop</span>
          <span>{status.counts.processed} already handled</span>
        </div>
      )}
    </div>
  )}
</div>
```

- [ ] **Step 4: Run the focused test and verify GREEN**

Run:

```bash
cd web
node --test app/lib/seo-client-contract.test.mjs
```

Expected: all tests in `seo-client-contract.test.mjs` pass with zero failures.

- [ ] **Step 5: Review the diff and commit the behavior**

Run:

```bash
git diff --check
git diff -- web/app/lib/seo-client-contract.test.mjs web/app/projects/[id]/seo/seo-client.tsx
git add web/app/lib/seo-client-contract.test.mjs web/app/projects/[id]/seo/seo-client.tsx
git commit -m "fix: collapse opportunity finding run details"
```

Expected: `git diff --check` exits 0 and the commit contains only the regression test plus the panel disclosure.

### Task 2: Verify the complete frontend and repository

**Files:**
- Verify only; no expected file changes.

- [ ] **Step 1: Run the complete Web test suite**

Run:

```bash
cd web
npm test
```

Expected: 425 or more tests pass with zero failures.

- [ ] **Step 2: Run TypeScript checking**

Run:

```bash
cd web
npm run typecheck
```

Expected: exit 0 with no TypeScript errors.

- [ ] **Step 3: Build the production frontend**

Run:

```bash
cd web
npm run build
```

Expected: Next.js production build exits 0.

- [ ] **Step 4: Re-run the Go suite and verify repository cleanliness**

Run:

```bash
go test ./...
git status --short --branch
```

Expected: all Go packages pass; the branch is clean and ahead of `origin/main` only by the design, plan, and implementation commits.

### Task 3: Merge through a PR and verify production

**Files:**
- No source changes unless production verification finds a gap.

- [ ] **Step 1: Push the branch and open the PR**

Run:

```bash
git push -u origin codex/default-fold-opportunity-details
gh pr create --base main --head codex/default-fold-opportunity-details --title "Collapse Opportunity Finding run details by default" --body "## Summary
- hide Opportunity Finding stage summaries and counts by default
- add an accessible Run details disclosure
- keep run progress and durable errors visible

## Verification
- node --test app/lib/seo-client-contract.test.mjs
- npm test
- npm run typecheck
- npm run build
- go test ./..."
```

Expected: push succeeds and `gh pr create` returns the new GitHub PR URL.

- [ ] **Step 2: Wait for required checks and merge**

Run:

```bash
gh pr checks --watch
gh pr merge --merge --delete-branch
```

Expected: all required checks pass and the PR reports merged into `main`.

- [ ] **Step 3: Wait for the production deployment**

Use the repository's Vercel project deployment status for the merged `main` commit. Wait until the production deployment reaches `Ready`; if it reaches `Error`, inspect the deployment logs, fix the branch, push, and repeat the PR/check/merge flow.

- [ ] **Step 4: Verify the production interaction**

Open `https://citeloop.app`, sign in through the available authenticated browser session, open a project Analysis page, and inspect the Opportunity Finding panel.

Expected on initial render:

- `Run details` is visible;
- the five summary cards and the `open`, `in loop`, and `already handled` counts are hidden;
- progress and any durable error alert remain visible.

Activate `Run details`.

Expected after activation:

- all five summary cards appear;
- all three counts appear;
- the control reports expanded state.

Activate it again.

Expected: the summary cards and counts are hidden again.

- [ ] **Step 5: Record the final evidence**

Run:

```bash
gh pr view --json url,state,mergedAt,mergeCommit
```

Expected: `state` is `MERGED`, `mergedAt` is populated, and the production verification above has passed before reporting the PR URL.
