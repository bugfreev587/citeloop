# Opportunity Disclosure Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align the `Run details` chevron and label with `Run timeline` while preserving both disclosure controls and their behavior.

**Architecture:** Keep the existing component boundary: `OpportunityFindingProgress` continues to own the timeline disclosure and `OpportunityFindingStatusPanel` continues to own the details disclosure. Match the progress strip's 15-pixel border-box inset—its 1-pixel border plus 14-pixel `p-3.5` padding—by adding `ml-[15px]` to the details button, with a source contract test locking both sides of the alignment.

**Tech Stack:** Next.js, React, TypeScript, Tailwind CSS, Node.js test runner, Go

---

## File Map

- Modify `web/app/lib/seo-client-contract.test.mjs`: extend the existing details-disclosure contract with matching inset assertions.
- Modify `web/app/projects/[id]/seo/seo-client.tsx`: add the 15-pixel Tailwind margin to the existing `Run details` button.
- Reference only `web/app/projects/[id]/seo/opportunity-finding-progress.tsx`: verify the existing progress strip still supplies the matching `p-3.5` inset.

### Task 1: Lock and Implement the Shared Left Edge

**Files:**
- Modify: `web/app/lib/seo-client-contract.test.mjs:181-206`
- Modify: `web/app/projects/[id]/seo/seo-client.tsx:1020-1030`
- Reference: `web/app/projects/[id]/seo/opportunity-finding-progress.tsx:71-72`

- [ ] **Step 1: Write the failing alignment contract**

Change the start of the existing details-disclosure test to load both relevant components:

```js
test("Opportunity Finding run details default closed behind an accessible disclosure", async () => {
  const [source, progressSource] = await Promise.all([
    readFile(new URL("../projects/[id]/seo/seo-client.tsx", import.meta.url), "utf8"),
    readFile(new URL("../projects/[id]/seo/opportunity-finding-progress.tsx", import.meta.url), "utf8"),
  ]);
```

After the existing visible-label assertion, add:

```js
  assert.match(
    progressSource,
    /data-opportunity-finding-progress className="[^"]*\bborder\b[^"]*\bp-3\.5\b[^"]*"/,
    "Run timeline must retain the progress strip's 1px border and 14px content inset",
  );
  assert.match(
    toggleSource,
    /className="[^"]*\bml-\[15px\](?:\s|")/,
    "Run details must match the progress strip's 15px border-box inset",
  );
```

- [ ] **Step 2: Run the focused contract and verify the red state**

Run from `web/`:

```bash
node --test --test-name-pattern "Opportunity Finding run details" app/lib/seo-client-contract.test.mjs
```

Expected: FAIL only on `Run details must match the progress strip's 15px border-box inset` because the details button does not yet contain `ml-[15px]`.

- [ ] **Step 3: Add the minimal alignment class**

Update the existing details button class in `web/app/projects/[id]/seo/seo-client.tsx`:

```tsx
className="ml-[15px] mt-4 inline-flex items-center gap-1 rounded-md px-1 text-xs font-semibold text-emerald-700 transition hover:bg-emerald-100/70 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-emerald-500"
```

Do not add a wrapper or change state, ARIA attributes, event handling, copy, or the conditional detail region.

- [ ] **Step 4: Run the focused contract and verify the green state**

Run from `web/`:

```bash
node --test --test-name-pattern "Opportunity Finding run details" app/lib/seo-client-contract.test.mjs
```

Expected: PASS with one matching test and all non-matching tests skipped.

- [ ] **Step 5: Commit the tested UI change**

```bash
git add web/app/lib/seo-client-contract.test.mjs 'web/app/projects/[id]/seo/seo-client.tsx'
git commit -m "fix: align opportunity disclosure controls"
```

Expected: one commit containing only the contract and the presentation-only class change.

### Task 2: Verify the Complete Local Change

**Files:**
- Verify: `web/app/lib/seo-client-contract.test.mjs`
- Verify: `web/app/projects/[id]/seo/seo-client.tsx`

- [ ] **Step 1: Run the complete Web test suite**

Run from `web/`:

```bash
npm test
```

Expected: all Web tests pass with zero failures.

- [ ] **Step 2: Run TypeScript validation**

Run from `web/`:

```bash
npm run typecheck
```

Expected: exit code 0 with no TypeScript errors.

- [ ] **Step 3: Run the production Web build**

Run from `web/`:

```bash
npm run build
```

Expected: the Next.js production build exits successfully.

- [ ] **Step 4: Run the backend regression suite**

Run from the repository root:

```bash
go test ./...
```

Expected: every Go package passes or reports no test files.

- [ ] **Step 5: Inspect the final diff and worktree state**

```bash
git diff origin/main...HEAD --check
git diff --stat origin/main...HEAD
git status --short --branch
```

Expected: no whitespace errors; the branch contains the design, plan, test, and one Tailwind class change; the worktree is clean.

### Task 3: Publish, Merge, Deploy, and Verify Production

**Files:**
- No additional file changes expected.

- [ ] **Step 1: Push the isolated branch**

```bash
git push -u origin codex/align-opportunity-disclosure-toggles
```

Expected: the remote branch is created successfully.

- [ ] **Step 2: Open the pull request against `origin/main`**

```bash
gh pr create \
  --base main \
  --head codex/align-opportunity-disclosure-toggles \
  --title "Align Opportunity Finding disclosure controls" \
  --body "Aligns Run details with Run timeline using the progress strip's 15px border-box inset. Preserves disclosure behavior and adds a regression contract."
```

Expected: GitHub returns the new pull-request URL.

- [ ] **Step 3: Wait for required checks and merge**

```bash
gh pr checks --watch
gh pr merge --squash --delete-branch
```

Expected: required checks pass and GitHub reports the pull request merged into `main`.

- [ ] **Step 4: Wait for production deployments**

Inspect the merged pull request and the Vercel and Railway deployment checks until both production deployments report success. If either deployment fails, inspect its logs, fix the scoped failure on the same branch, push, merge the follow-up, and wait again.

- [ ] **Step 5: Verify the production UI**

Open the production Analysis page for a project with a completed Opportunity Finding run. At desktop and narrow viewport widths:

1. Confirm `Run timeline` remains inside the white progress strip.
2. Confirm `Run details` remains on its separate row.
3. Compare their rendered left coordinates and confirm the chevrons and labels share the same left edge.
4. Open and close each disclosure independently.
5. Confirm `aria-expanded` changes correctly and the controlled regions appear and disappear.

Expected: the alignment and all existing disclosure behavior match the approved design at both viewport widths.
