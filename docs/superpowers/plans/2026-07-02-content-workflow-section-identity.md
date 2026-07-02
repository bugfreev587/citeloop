# Content Workflow Section Identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the continuous Content Plan, Review, and Publish workflow clearly stage-based and fix sidebar clicks so each entry lands its stage at the top with the correct active navigation state.

**Architecture:** Keep the existing shared `ContentWorkflowClient` wrapper and the existing route files. Add a workflow stage frame inside the wrapper for section shell, step labels, and subtle color treatment. Extend the shared `SectionHeader` so top-level workflow headers render as page-level `h1` headings while module headers remain `h2`. Make route-driven scrolling resilient by retrying target alignment while child content settles. Cover the behavior with contract tests that map directly to the PRD acceptance criteria.

**Tech Stack:** Next.js App Router, React client component, Tailwind CSS 3, existing lucide icons, Node contract tests.

---

## Acceptance Criteria Mapping

- PRD 4.1: Add the focused PRD and keep this plan in docs.
- PRD 4.2: Content Plan has a stage title, `Step 1 of 3`, secondary module headings, distinct subtle shell, and active nav.
- PRD 4.3: Review sidebar click and direct route land Review at the top, show `Step 2 of 3`, and keep Review active.
- PRD 4.4: Publish sidebar click and direct route land Publish at the top, show `Step 3 of 3`, normalize page title to `Publish`, and keep Publish active.
- PRD 4.5: Manual scroll remains continuous, updates URL/nav, and does not introduce scroll snap.
- PRD 4.6: Contract tests, full local verification, browser verification, PR merge, deployment, and production verification pass.

## Files

- Modify: `web/app/projects/[id]/content-workflow-client.tsx`
- Modify: `web/app/components/ui.tsx`
- Modify: `web/app/projects/[id]/topics/topics-client.tsx`
- Modify: `web/app/projects/[id]/review/review-client.tsx`
- Modify: `web/app/projects/[id]/publishing/publishing-client.tsx`
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
- Add: `docs/PRD-CiteLoop-Content-Workflow-Section-Identity.md`
- Add: `docs/superpowers/plans/2026-07-02-content-workflow-section-identity.md`

## Task 1: Add Failing Contract Tests

- [ ] **Step 1: Add tests for PRD presence and workflow stage identity**

Add tests to `web/app/lib/dashboard-ux-phase1-contract.test.mjs` that assert:

```js
assert.equal(exists("../docs/PRD-CiteLoop-Content-Workflow-Section-Identity.md"), true);
assert.match(workflow, /Step 1 of 3/);
assert.match(workflow, /Step 2 of 3/);
assert.match(workflow, /Step 3 of 3/);
assert.match(ui, /data-content-workflow-stage-title/);
assert.match(topics, /title="Content Plan"[\s\S]*level="page"/);
assert.match(review, /title="Review"[\s\S]*level="page"/);
assert.match(publishing, /title="Publish"[\s\S]*level="page"/);
assert.match(workflow, /data-content-workflow-stage-accent/);
assert.match(workflow, /data-content-workflow-stage-shell/);
assert.match(publishing, /title="Publish"/);
assert.doesNotMatch(publishing, /title="Publishing"/);
```

- [ ] **Step 2: Add tests for route-driven scroll settling**

Add assertions that `ContentWorkflowClient` contains:

```js
assert.match(workflow, /pendingTargetRef/);
assert.match(workflow, /TARGET_SETTLE_TIMEOUT_MS/);
assert.match(workflow, /isStepAligned/);
assert.match(workflow, /scrollToStep\(initialStep, "auto"\)/);
assert.match(workflow, /pendingTargetRef\.current/);
assert.match(workflow, /if \(pendingTargetRef\.current/);
```

- [ ] **Step 3: Verify tests fail before implementation**

Run:

```bash
cd web
node --test app/lib/dashboard-ux-phase1-contract.test.mjs --test-name-pattern "content workflow"
```

Expected: fail because the stage shell, step labels, pending scroll target, and Publish title normalization are not implemented.

## Task 2: Implement Stage Identity Shell and Heading Hierarchy

- [ ] **Step 1: Extend shared `SectionHeader`**

In `web/app/components/ui.tsx`, add `level?: "page" | "section"` to `SectionHeader`. Default to `"section"`. Render page-level headers with `h1`, module-level headers with `h2`, and add `data-content-workflow-stage-title` only when `level === "page"`.

- [ ] **Step 2: Add stage metadata**

In `web/app/projects/[id]/content-workflow-client.tsx`, define metadata for each workflow step:

```ts
const STAGE_META = {
  plan: {
    stepLabel: "Step 1 of 3",
    title: "Content Plan",
    eyebrow: "Topic backlog and action handoff",
    toneClass: "border-sky-100 bg-sky-50/35",
    accentClass: "bg-sky-500",
  },
  review: {
    stepLabel: "Step 2 of 3",
    title: "Review",
    eyebrow: "Approval gate and draft decisions",
    toneClass: "border-amber-100 bg-amber-50/35",
    accentClass: "bg-amber-500",
  },
  publish: {
    stepLabel: "Step 3 of 3",
    title: "Publish",
    eyebrow: "Canonical and syndication lanes",
    toneClass: "border-emerald-100 bg-emerald-50/35",
    accentClass: "bg-emerald-500",
  },
} satisfies Record<ContentWorkflowStep, StageMeta>;
```

- [ ] **Step 3: Add a local `WorkflowStage` component**

Wrap each existing client component in a section shell that renders:

```tsx
<section data-content-workflow-section={step} data-content-workflow-stage-shell>
  <div data-content-workflow-stage-accent />
  <div data-content-workflow-stage-step>
    <div>{meta.stepLabel}</div>
    <p>{meta.eyebrow}</p>
  </div>
  {children}
</section>
```

- [ ] **Step 4: Mark workflow page headers as page-level**

Set `level="page"` on:

- `Content Plan` in `web/app/projects/[id]/topics/topics-client.tsx`
- `Review` in `web/app/projects/[id]/review/review-client.tsx`
- `Publish` in `web/app/projects/[id]/publishing/publishing-client.tsx`

- [ ] **Step 5: Keep scroll continuity**

Ensure the workflow wrapper and sections do not contain `scroll-snap`, `snap-y`, or `snap-mandatory`. Use padding, border, and tint only.

- [ ] **Step 6: Normalize Publish title**

In `web/app/projects/[id]/publishing/publishing-client.tsx`, change the primary `SectionHeader` title from `Publishing` to `Publish`. Keep module/lane labels such as `Ready to publish` unchanged.

## Task 3: Fix Sidebar Click Scroll Bugs

- [ ] **Step 1: Add route-driven pending target refs**

In `ContentWorkflowClient`, add refs and constants:

```ts
const TARGET_TOP_OFFSET = 24;
const TARGET_ALIGNMENT_TOLERANCE = 8;
const TARGET_SETTLE_TIMEOUT_MS = 1_200;
const pendingTargetRef = useRef<ContentWorkflowStep | null>(null);
const pendingStartedAtRef = useRef(0);
```

- [ ] **Step 2: Return alignment status from scroll helpers**

Implement:

```ts
const sectionTopForStep = useCallback((step: ContentWorkflowStep) => {
  const section = sectionRefs.current[step];
  if (!section) return null;
  return section.getBoundingClientRect().top + window.scrollY - TARGET_TOP_OFFSET;
}, []);

const isStepAligned = useCallback((step: ContentWorkflowStep) => {
  const section = sectionRefs.current[step];
  if (!section) return false;
  return Math.abs(section.getBoundingClientRect().top - TARGET_TOP_OFFSET) <= TARGET_ALIGNMENT_TOLERANCE;
}, []);
```

Update `scrollToStep` to return `true` when it found a section and called `window.scrollTo`.

- [ ] **Step 3: Retry initial route scroll while content settles**

In the `initialStep` effect, set `pendingTargetRef.current = initialStep`, dispatch the route path event for the initial step, and request animation frames until:

- the target is aligned;
- or `TARGET_SETTLE_TIMEOUT_MS` has elapsed.

- [ ] **Step 4: Guard scroll spy during settling**

At the start of `updateActiveStep`, if `pendingTargetRef.current` is not null and not expired, keep the current URL/nav on the pending target and return without replacing the route with the step currently visible during layout settling.

## Task 4: Verify Locally

- [ ] **Step 1: Run targeted contract tests**

```bash
cd web
node --test app/lib/dashboard-ux-phase1-contract.test.mjs --test-name-pattern "content workflow"
```

Expected: pass.

- [ ] **Step 2: Run full web and backend checks**

```bash
cd web && npm test
cd web && npm run typecheck
cd web && npm run build
go test ./...
git diff --check
```

Expected: all pass. Existing unrelated npm audit warnings are acceptable if unchanged.

- [ ] **Step 3: Browser QA**

Run the app locally or use production after deployment to verify:

- `/plan` lands Content Plan at top and highlights Content Plan.
- Clicking Review lands Review at top and highlights Review.
- Clicking Publish lands Publish at top and highlights Publish.
- Manual scrolling can show adjacent sections and updates URL/nav.

## Task 5: Ship and Production Verification

- [ ] **Step 1: Commit**

```bash
git add docs/PRD-CiteLoop-Content-Workflow-Section-Identity.md docs/superpowers/plans/2026-07-02-content-workflow-section-identity.md web/app/lib/dashboard-ux-phase1-contract.test.mjs web/app/projects/[id]/content-workflow-client.tsx web/app/projects/[id]/publishing/publishing-client.tsx
git commit -m "fix(web): clarify content workflow stages"
```

- [ ] **Step 2: Push, open PR, and wait for checks**

```bash
git push -u origin codex/content-workflow-section-identity
gh pr create --base main --head codex/content-workflow-section-identity --title "Clarify content workflow stage identity" --body-file /tmp/content-workflow-section-identity-pr.md
gh pr checks --watch
```

- [ ] **Step 3: Merge and verify production**

After merge and Vercel deployment:

- Confirm deployment commit matches the merge commit.
- Open production `/plan`, `/review`, and `/publish`.
- Verify every PRD 4.x acceptance criterion one by one.
- Check browser console and recent production runtime errors.
