# Results Opportunity Trace ID Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Display the same stable `OPP-xxxxxx` lineage ID on each Results Content Action card and its attribution drawer.

**Architecture:** Reuse the existing client-side `workflowTraceLabelForAction(action)` formatter; the Results API already supplies `opportunity_id`, so no backend or schema changes are needed. Add contract coverage scoped to the Content Action card and drawer so Site Fix Results and multi-platform aggregation remain unchanged.

**Tech Stack:** Next.js App Router, React, TypeScript, Node test runner, existing `Badge` UI component

---

### Task 1: Add the Results lineage-ID contract

**Files:**
- Modify: `web/app/lib/results-attribution-contract.test.mjs`

- [ ] **Step 1: Write the failing test**

Append this test after `Results action attribution opens compact cards into a detail drawer`:

```js
test("Results Content Action cards and drawers display the shared opportunity trace ID", () => {
  const seo = read("projects/[id]/seo/seo-client.tsx");
  const resultsStart = seo.indexOf('{mode === "results"');
  const resultsBlock = seo.slice(resultsStart);

  const cardStart = resultsBlock.indexOf("data-results-action-card={action.id}");
  const cardEnd = resultsBlock.indexOf("</button>", cardStart);
  const cardBlock = resultsBlock.slice(cardStart, cardEnd);
  assert.notEqual(cardStart, -1, "Results Content Action card is missing");
  assert.match(
    cardBlock,
    />Content Action<\/Badge>[\s\S]*<Badge tone="neutral">\{workflowTraceLabelForAction\(action\)\}<\/Badge>/,
  );

  const drawerStart = resultsBlock.indexOf("data-results-drawer");
  const drawerEnd = resultsBlock.indexOf("</aside>", drawerStart);
  const drawerBlock = resultsBlock.slice(drawerStart, drawerEnd);
  assert.notEqual(drawerStart, -1, "Results Content Action drawer is missing");
  assert.match(
    drawerBlock,
    /<Badge tone="neutral">\{workflowTraceLabelForAction\(action\)\}<\/Badge>/,
  );
});
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd web
node --test app/lib/results-attribution-contract.test.mjs
```

Expected: FAIL in `Results Content Action cards and drawers display the shared opportunity trace ID` because neither Results location renders `workflowTraceLabelForAction(action)` yet.

### Task 2: Render the ID in both Results locations

**Files:**
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`
- Test: `web/app/lib/results-attribution-contract.test.mjs`

- [ ] **Step 1: Add the card badge**

In the Results Content Action card badge row, render the neutral lineage badge immediately after `Content Action`:

```tsx
<div className="flex flex-wrap items-center gap-2">
  <Badge tone="blue">Content Action</Badge>
  <Badge tone="neutral">{workflowTraceLabelForAction(action)}</Badge>
  <Badge tone={state.tone}>{state.label}</Badge>
  <Badge tone={queue.tone}>{queue.label}</Badge>
  <Badge tone={toneForStatus(action.status)}>{action.status}</Badge>
</div>
```

- [ ] **Step 2: Add the drawer badge**

At the start of the Content Action attribution drawer status row, render the same formatter output:

```tsx
<div className="mt-3 flex flex-wrap items-center gap-2">
  <Badge tone="neutral">{workflowTraceLabelForAction(action)}</Badge>
  <Badge tone={state.tone}>{state.label}</Badge>
  <Badge tone={queue.tone}>{queue.label}</Badge>
  <Badge tone={toneForStatus(action.status)}>{action.status}</Badge>
</div>
```

- [ ] **Step 3: Run the focused test and verify GREEN**

Run:

```bash
cd web
node --test app/lib/results-attribution-contract.test.mjs
```

Expected: all tests in the file PASS.

- [ ] **Step 4: Run the workflow handoff regression test**

Run:

```bash
cd web
node --test app/lib/workflow-handoff-contract.test.mjs
```

Expected: all tests PASS, including the Recently Published deep link and stable lineage-ID coverage.

- [ ] **Step 5: Commit the tested UI change**

```bash
git add web/app/lib/results-attribution-contract.test.mjs web/app/projects/'[id]'/seo/seo-client.tsx
git commit -m "fix: show opportunity ID in Results"
```

### Task 3: Verify, publish, and validate production

**Files:**
- Verify: `web/app/lib/results-attribution-contract.test.mjs`
- Verify: `web/app/projects/[id]/seo/seo-client.tsx`

- [ ] **Step 1: Run complete local verification**

Run sequentially:

```bash
cd web
npm test
npm run typecheck
VERCEL_ENV=preview npm run build
cd ..
git diff --check origin/main...HEAD
```

Expected: Web tests PASS with zero failures, typecheck exits 0, the Next.js build succeeds, and `git diff --check` prints nothing.

- [ ] **Step 2: Push and open the PR**

```bash
git push -u origin codex/results-opportunity-id
gh pr create --repo bugfreev587/citeloop --base main --head codex/results-opportunity-id --title "Show opportunity ID in Results" --body "Displays the same stable opportunity lineage ID on Results Content Action cards and attribution drawers."
```

Expected: GitHub returns a PR URL targeting `origin/main`.

- [ ] **Step 3: Wait for checks and merge**

```bash
gh pr checks --repo bugfreev587/citeloop --watch
gh pr merge --repo bugfreev587/citeloop --merge
```

Expected: required Go, Web, and deployment checks succeed and the PR state becomes `MERGED`. If a non-code external review service fails, inspect its logs and distinguish infrastructure failure from actionable code feedback before merging.

- [ ] **Step 4: Wait for production deployment**

Use the merged commit SHA from:

```bash
gh pr view --repo bugfreev587/citeloop --json url,state,mergedAt,mergeCommit,statusCheckRollup
```

Poll the merged commit checks until the Vercel production deployment and Railway API deployment report success. Confirm `https://citeloop.app/` returns HTTP 200 and `https://api.citeloop.app/healthz` returns `ok`.

- [ ] **Step 5: Verify the production artifact**

Confirm the production Vercel deployment metadata points to the merged Git commit and the deployed source contains both Results usages of `workflowTraceLabelForAction(action)`. Where an authenticated browser session is available, open Recently Published, choose `View Results`, and confirm the same `OPP-xxxxxx` appears on the focused Results card and in its drawer. If browser control is unavailable, record that limitation and use deployment-SHA/source-blob equivalence plus the passing interaction contracts as the production verification evidence.
