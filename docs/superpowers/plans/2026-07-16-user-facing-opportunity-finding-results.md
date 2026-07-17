# User-facing Opportunity Finding Results Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the customer-facing Growth Radar diagnostics funnel with no success panel and concise plain-language zero-result or incomplete-result messages.

**Architecture:** Keep all Growth Radar backend collection and APIs unchanged. Add a small pure presentation mapper in `web/app/lib/growth-radar.ts`, then make the Analysis client derive zero/incomplete outcomes from the current `OpportunityFindingStatus.last_run` while the existing Opportunity Queue remains the sole success presentation. Replace the durable run failure's raw backend error with the same approved retry language.

**Final implementation note:** The current-run status is authoritative for customer messaging. It supersedes the diagnostics-based mapper inputs shown in the original step-by-step snippets below, which are retained as planning history.

**Tech Stack:** Next.js 15, React 18, TypeScript, Node test runner, Tailwind CSS

---

## File structure

- `web/app/lib/growth-radar.ts`: owns the pure mapping from an internal funnel to a customer-facing result or `null`.
- `web/app/lib/growth-radar.test.mjs`: verifies success, healthy-zero, degraded-zero, and failed-zero mapping without React.
- `web/app/projects/[id]/seo/seo-client.tsx`: renders the mapped result and removes all diagnostics dashboard markup and raw failure details.
- `web/app/lib/seo-client-contract.test.mjs`: protects the Analysis source contract against reintroducing engineering labels or backend errors.

### Task 1: Add the customer-facing result mapper

**Files:**
- Modify: `web/app/lib/growth-radar.test.mjs`
- Modify: `web/app/lib/growth-radar.ts`

- [ ] **Step 1: Write failing mapper tests**

Append these focused cases to `web/app/lib/growth-radar.test.mjs`:

```js
test("user-facing result is hidden when opportunities were created", async () => {
  const { userFacingGrowthRadarResult } = await loadModule();
  const run = { ...base, candidates: { ...base.candidates, created: 5 } };
  assert.equal(userFacingGrowthRadarResult(run), null);
});

test("user-facing result explains a healthy zero without diagnostics", async () => {
  const { userFacingGrowthRadarResult } = await loadModule();
  assert.deepEqual(userFacingGrowthRadarResult(base), {
    kind: "empty",
    tone: "neutral",
    title: "No new opportunities found",
    detail: "Your current opportunity queue is up to date. CiteLoop will keep looking as your site and market change.",
  });
});

test("user-facing result turns incomplete zero runs into retry guidance", async () => {
  const { userFacingGrowthRadarResult } = await loadModule();
  const expected = {
    kind: "incomplete",
    tone: "amber",
    title: "Opportunity finding couldn't finish",
    detail: "We couldn't complete every check. Please try again.",
  };
  assert.deepEqual(userFacingGrowthRadarResult({ ...base, status: "degraded" }), expected);
  assert.deepEqual(userFacingGrowthRadarResult({ ...base, status: "failed" }), expected);
});
```

- [ ] **Step 2: Run the mapper tests and verify RED**

Run:

```bash
cd web && node --test app/lib/growth-radar.test.mjs
```

Expected: FAIL because `userFacingGrowthRadarResult` is not exported.

- [ ] **Step 3: Implement the minimal mapper**

Add this focused presentation type and function to `web/app/lib/growth-radar.ts` after `GrowthRadarFunnel`:

```ts
export type GrowthRadarUserResult = {
  kind: "empty" | "incomplete";
  tone: "neutral" | "amber";
  title: string;
  detail: string;
};

export function userFacingGrowthRadarResult(run: GrowthRadarFunnel): GrowthRadarUserResult | null {
  if (Number(run.candidates?.created ?? 0) > 0) return null;
  if (run.status === "degraded" || run.status === "failed") {
    return {
      kind: "incomplete",
      tone: "amber",
      title: "Opportunity finding couldn't finish",
      detail: "We couldn't complete every check. Please try again.",
    };
  }
  return {
    kind: "empty",
    tone: "neutral",
    title: "No new opportunities found",
    detail: "Your current opportunity queue is up to date. CiteLoop will keep looking as your site and market change.",
  };
}
```

- [ ] **Step 4: Run the mapper tests and verify GREEN**

Run:

```bash
cd web && node --test app/lib/growth-radar.test.mjs
```

Expected: all Growth Radar tests pass.

- [ ] **Step 5: Commit the mapper**

```bash
git add web/app/lib/growth-radar.ts web/app/lib/growth-radar.test.mjs
git commit -m "feat: map opportunity finding results to user copy"
```

### Task 2: Replace the diagnostics panel and raw error

**Files:**
- Modify: `web/app/lib/seo-client-contract.test.mjs`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx`

- [ ] **Step 1: Write the failing Analysis presentation contract**

Update the existing `Analysis page exposes Opportunity Finding run status` test so it requires `status.last_run.error` to be absent and the approved failure copy to be present. Add this separate contract test:

```js
test("Analysis hides Growth Radar engineering diagnostics from customers", async () => {
  const source = await readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8");
  const resultStart = source.indexOf("function GrowthRadarResultMessage");
  const resultEnd = source.indexOf("type SEOClientMode", resultStart);
  const resultSource = source.slice(resultStart, resultEnd);

  assert.notEqual(resultStart, -1, "seo-client.tsx missing GrowthRadarResultMessage");
  assert.notEqual(resultEnd, -1, "seo-client.tsx missing GrowthRadarResultMessage boundary");
  assert.match(resultSource, /userFacingGrowthRadarResult/);
  assert.match(resultSource, /<Notice/);
  assert.equal(source.includes("GrowthRadarDiagnosticsPanel"), false);
  assert.equal(source.includes("data-growth-radar-diagnostics"), false);
  for (const removed of [
    "Growth Radar funnel",
    "Deterministic evidence, policy and target diagnostics",
    "Prompt rotation",
    "Provider cost",
  ]) {
    assert.equal(resultSource.includes(removed), false, `customer result must hide ${removed}`);
  }
});
```

Replace the old durable-error assertion with:

```js
assert.equal(panelSource.includes("status.last_run.error"), false, "raw workflow errors must stay out of customer copy");
assert.equal(panelSource.includes("Opportunity finding couldn't finish"), true);
assert.equal(panelSource.includes("We couldn't complete every check. Please try again."), true);
```

- [ ] **Step 2: Run the Analysis contract and verify RED**

Run:

```bash
cd web && node --test app/lib/seo-client-contract.test.mjs
```

Expected: FAIL because `GrowthRadarDiagnosticsPanel` and `status.last_run.error` are still present and `GrowthRadarResultMessage` is missing.

- [ ] **Step 3: Replace the raw run failure detail**

In `OpportunityFindingStatusPanel`, render the durable failure whenever `runStatus === "failed"`, without requiring or reading `status.last_run.error`:

```tsx
{runStatus === "failed" && (
  <div
    data-opportunity-finding-error
    role="alert"
    className="mt-4 rounded-lg border border-red-200 bg-white/85 px-3 py-2 text-sm text-red-900"
  >
    <div className="font-bold">Opportunity finding couldn't finish</div>
    <div className="mt-1 leading-5">We couldn't complete every check. Please try again.</div>
  </div>
)}
```

- [ ] **Step 4: Replace the diagnostics component with a plain result message**

Change the Growth Radar import to `userFacingGrowthRadarResult`. Replace `GrowthRadarDiagnosticsPanel` with:

```tsx
function GrowthRadarResultMessage({
  diagnostics,
  runFailed,
}: {
  diagnostics: GrowthRadarDiagnostics | null;
  runFailed: boolean;
}) {
  if (!diagnostics) return null;
  const result = userFacingGrowthRadarResult(diagnostics.summary);
  if (!result || (runFailed && result.kind === "incomplete")) return null;
  return <Notice title={result.title} detail={result.detail} tone={result.tone} />;
}
```

Update the Analysis render call to suppress a duplicate incomplete message when the durable run-failure alert is already visible:

```tsx
<GrowthRadarResultMessage
  diagnostics={growthRadarDiagnostics}
  runFailed={opportunityFindingStatus?.last_run?.status === "failed"}
/>
```

- [ ] **Step 5: Run the contract and mapper tests and verify GREEN**

Run:

```bash
cd web && node --test app/lib/growth-radar.test.mjs app/lib/seo-client-contract.test.mjs
```

Expected: both test files pass with no failures.

- [ ] **Step 6: Run TypeScript checking**

Run:

```bash
cd web && npm run typecheck
```

Expected: exit 0 with no TypeScript errors.

- [ ] **Step 7: Commit the Analysis UI change**

```bash
git add web/app/lib/seo-client-contract.test.mjs 'web/app/projects/[id]/seo/seo-client.tsx'
git commit -m "fix: hide Growth Radar diagnostics from customers"
```

### Task 3: Verify the complete change before publishing

**Files:**
- Verify: `web/app/lib/growth-radar.ts`
- Verify: `web/app/projects/[id]/seo/seo-client.tsx`

- [ ] **Step 1: Confirm forbidden customer-facing terms are gone from the Analysis component**

Run:

```bash
rg -n "Growth Radar funnel|Deterministic evidence, policy and target diagnostics|Prompt rotation|Provider cost|status\.last_run\.error" 'web/app/projects/[id]/seo/seo-client.tsx'
```

Expected: no matches.

- [ ] **Step 2: Run all web tests**

Run:

```bash
cd web && npm test
```

Expected: all tests pass with zero failures.

- [ ] **Step 3: Run the production build**

Run:

```bash
cd web && npm run build
```

Expected: Next.js production build exits 0.

- [ ] **Step 4: Run all Go tests to protect the unchanged diagnostics backend**

Run:

```bash
go test ./...
```

Expected: all Go packages pass.

- [ ] **Step 5: Review repository state**

Run:

```bash
git diff --check origin/main...HEAD
git status --short --branch
git log --oneline origin/main..HEAD
```

Expected: no whitespace errors, a clean worktree, and only the approved design, plan, mapper, tests, and Analysis presentation commits.

### Task 4: Publish, merge, deploy, and verify production

**Files:**
- No additional files unless production verification finds a gap.

- [ ] **Step 1: Push the task branch and open a PR to `origin/main`**

Push `codex/hide-growth-radar-diagnostics`, open a PR summarizing the removed diagnostics and state-based replacement, and include the fresh test/build evidence.

- [ ] **Step 2: Merge the PR**

Merge only after required GitHub checks pass. Confirm the PR's merge commit is present on `origin/main`.

- [ ] **Step 3: Wait for the production deployment**

Inspect the production deployment for the merged commit until it reaches a successful terminal state. If deployment fails, diagnose and fix the failure on a fresh follow-up commit before continuing.

- [ ] **Step 4: Verify the production Analysis page**

Using an authenticated production project with a successful Opportunity Finding run, confirm that Opportunity Queue cards remain visible and no Growth Radar funnel, candidate metrics, watchlist count, prompt rotation, provider cost, or reason badges appear. Also verify a zero-result or failed-run state when available; if it is not safely reproducible in production, verify the deployed client bundle contains the approved messages and excludes the forbidden copy.

- [ ] **Step 5: Fix and repeat if production differs**

If production behavior differs from the approved design, add a regression test first, push the smallest fix, merge it, wait for redeployment, and repeat production verification before reporting completion.
