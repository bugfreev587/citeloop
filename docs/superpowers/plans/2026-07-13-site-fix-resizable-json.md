# Site Fix Resizable JSON Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every Site Fix JSON/detail payload default to five visible lines while supporting downward resizing, internal scrolling, text selection, and copying.

**Architecture:** Keep the behavior local to the existing Site Fix drawer. Define one shared Tailwind class contract and reuse it in both the generic `DetailBlock` renderer and the AI coding payload so every structured block behaves consistently without changing data or clipboard logic.

**Tech Stack:** Next.js 15, React 18, TypeScript, Tailwind CSS, Node.js test runner

---

### Task 1: Add the shared resizable JSON viewport

**Files:**
- Modify: `web/app/lib/seo-client-contract.test.mjs`
- Modify: `web/app/projects/[id]/site-fixes/site-fixes-client.tsx`

- [x] **Step 1: Write the failing contract test**

Add a test that reads `site-fixes-client.tsx`, locates the shared viewport class, and asserts that it contains the complete interaction contract:

```js
test("Canonical Site Fix JSON blocks default to five resizable scrollable lines", async () => {
  const source = await readFile(new URL("../projects/[id]/site-fixes/site-fixes-client.tsx", import.meta.url), "utf8");

  assert.match(source, /const SITE_FIX_JSON_VIEWPORT_CLASS = "[^"]*box-content[^"]*h-\[7\.5rem\][^"]*min-h-\[7\.5rem\][^"]*max-h-\[30rem\][^"]*resize-y[^"]*overflow-auto[^"]*select-text[^"]*"/);
  assert.equal((source.match(/SITE_FIX_JSON_VIEWPORT_CLASS/g) ?? []).length, 3, "shared viewport class should be declared once and used by both JSON render paths");
  assert.match(source, /function DetailBlock[\s\S]*<pre[\s\S]*SITE_FIX_JSON_VIEWPORT_CLASS/);
  assert.match(source, /data-site-fix-ai-payload[\s\S]*<pre[\s\S]*SITE_FIX_JSON_VIEWPORT_CLASS/);
});
```

- [x] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd web
node --test --test-name-pattern="Canonical Site Fix JSON blocks default" app/lib/seo-client-contract.test.mjs
```

Expected: FAIL because `SITE_FIX_JSON_VIEWPORT_CLASS` does not exist.

- [x] **Step 3: Implement the shared viewport contract**

In `site-fixes-client.tsx`, declare:

```ts
const SITE_FIX_JSON_VIEWPORT_CLASS = "box-content h-[7.5rem] min-h-[7.5rem] max-h-[30rem] resize-y overflow-auto select-text";
```

Apply it to the `DetailBlock` `pre`, retaining wrapped text and using `leading-6` so `7.5rem` represents five content rows:

```tsx
<pre className={`mt-3 ${SITE_FIX_JSON_VIEWPORT_CLASS} whitespace-pre-wrap break-words rounded-lg bg-slate-50 p-3 font-sans text-sm leading-6 text-slate-700`}>
  {prettyValue(value)}
</pre>
```

Apply the same class to the AI coding payload and use the same `leading-6` row height:

```tsx
<pre className={`mt-3 ${SITE_FIX_JSON_VIEWPORT_CLASS} whitespace-pre-wrap break-words rounded-lg bg-slate-950 p-3 text-xs leading-6 text-slate-100`}>
  {canonicalSiteFixAIJSON(selected)}
</pre>
```

- [x] **Step 4: Run the focused test and verify GREEN**

Run:

```bash
cd web
node --test --test-name-pattern="Canonical Site Fix JSON blocks default" app/lib/seo-client-contract.test.mjs
```

Expected: PASS.

- [x] **Step 5: Run full Web verification**

Run:

```bash
cd web
npm test
npm run typecheck
npm run build
```

Expected: 0 test failures, TypeScript exit code 0, and Next.js production build exit code 0.

- [x] **Step 6: Verify the interaction in a browser**

Start the Web app using the repository's local development configuration, open a Site Fix detail drawer, and verify:

1. Evidence, Proposed fix, Acceptance checks, Verification, and AI coding fix JSON initially show five content rows.
2. Overflowing text scrolls vertically inside its own payload.
3. Dragging the bottom-right resize handle downward increases the payload height.
4. The payload cannot be resized below five rows.
5. Text can be selected and copied, while **Copy fix JSON** continues copying the full repair payload.

- [ ] **Step 7: Commit the implementation**

```bash
git add web/app/lib/seo-client-contract.test.mjs web/app/projects/[id]/site-fixes/site-fixes-client.tsx docs/superpowers/plans/2026-07-13-site-fix-resizable-json.md
git commit -m "fix: constrain site fix json blocks"
```

### Task 2: Deliver and verify production

**Files:**
- No additional source files expected.

- [ ] **Step 1: Push and create a pull request to `origin/main`**

```bash
git push -u origin codex/site-fix-resizable-json
gh pr create --base main --head codex/site-fix-resizable-json --title "Fix resizable Site Fix JSON blocks" --body "## Summary
- default Site Fix JSON payloads to five visible lines
- allow vertical resizing, internal scrolling, selection, and copying

## Verification
- npm test
- npm run typecheck
- npm run build"
```

Expected: GitHub returns a pull-request URL.

- [ ] **Step 2: Wait for checks and merge the pull request**

```bash
gh pr checks <pr-number> --watch
gh pr merge <pr-number> --merge --delete-branch
```

Expected: required checks pass and GitHub reports the PR merged.

- [ ] **Step 3: Wait for production deployment and verify**

Use the deployment status associated with the merged `main` commit, wait until it reaches Ready, then repeat the browser interaction checklist against the production Site Fix drawer.

Expected: production matches all five interaction requirements with no regression in **Copy fix JSON**.
