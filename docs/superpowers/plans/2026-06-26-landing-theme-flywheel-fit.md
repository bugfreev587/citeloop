# Landing Theme Icon And Flywheel Fit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the landing theme switch with a single icon control and keep all flywheel outer labels inside the gray ring.

**Architecture:** Keep the change in the existing landing surface. `web/app/landing-auth-actions.tsx` owns the client-side theme control, and `web/app/page.tsx` owns the SVG flywheel geometry.

**Tech Stack:** Next.js App Router, React client components, Tailwind CSS 3, lucide-react, Node test runner contract tests.

---

### Task 1: Contract Tests

**Files:**
- Modify: `web/app/lib/landing-page-contract.test.mjs`
- Modify: `web/app/lib/theme-contract.test.mjs`

- [ ] **Step 1: Write failing landing page geometry test**

Add assertions that the outer label paths use the inward label radius and reject the current oversized radius:

```js
assert.match(source, /id="domain-gsc-label" d="M 184 96 A 238 238 0 0 1 416 96"/);
assert.match(source, /id="opportunities-label" d="M 520 194 A 248 248 0 0 1 520 406"/);
assert.match(source, /id="measured-outcomes-label" d="M 100 430 A 232 232 0 0 1 100 170"/);
assert.match(source, /className="landing-ring-label text-\[22px\] font-black"/);
assert.doesNotMatch(source, /id="domain-gsc-label" d="M 172 79 A 255 255 0 0 1 428 79"/);
assert.doesNotMatch(source, /id="opportunities-label" d="M 531 192 A 255 255 0 0 1 531 408"/);
assert.doesNotMatch(source, /id="measured-outcomes-label" d="M 91 446 A 255 255 0 0 1 91 154"/);
```

- [ ] **Step 2: Write failing theme control test**

Add assertions that the landing theme control is a single icon trigger rather than a two-column switch:

```js
assert.match(landingActions, /const Icon = isDark \? Moon : Sun/);
assert.match(landingActions, /aria-label=\{label\}/);
assert.match(landingActions, /onClick=\{\(\) => chooseTheme\(nextTheme\)\}/);
assert.doesNotMatch(landingActions, /grid-cols-2/);
assert.doesNotMatch(landingActions, /aria-label="Use light mode"/);
assert.doesNotMatch(landingActions, /aria-label="Use dark mode"/);
```

- [ ] **Step 3: Verify red**

Run:

```bash
npm test -- app/lib/landing-page-contract.test.mjs app/lib/theme-contract.test.mjs
```

Expected: fails because the current flywheel label paths still use the old radius and `LandingThemeToggle` still renders two buttons.

### Task 2: Theme Icon Control

**Files:**
- Modify: `web/app/landing-auth-actions.tsx`

- [ ] **Step 1: Implement single icon trigger**

Replace the two-button switch with one button that toggles to the opposite theme:

```tsx
const isDark = theme === "dark";
const nextTheme: ThemeChoice = isDark ? "light" : "dark";
const Icon = isDark ? Moon : Sun;
const label = isDark ? "Switch to light theme" : "Switch to dark theme";
```

Use a single rounded button with `aria-label={label}` and `onClick={() => chooseTheme(nextTheme)}`.

- [ ] **Step 2: Verify green for theme tests**

Run:

```bash
npm test -- app/lib/theme-contract.test.mjs
```

Expected: passes with the single trigger contract.

### Task 3: Flywheel Label Geometry

**Files:**
- Modify: `web/app/page.tsx`

- [ ] **Step 1: Move outer label paths inward**

Change the four outer `textPath` definitions to sit within the gray ring:

```tsx
<path id="domain-gsc-label" d="M 184 96 A 238 238 0 0 1 416 96" />
<path id="opportunities-label" d="M 520 194 A 248 248 0 0 1 520 406" />
<path id="published-assets-label" d="M 176 518 A 248 248 0 0 0 424 518" />
<path id="measured-outcomes-label" d="M 100 430 A 232 232 0 0 1 100 170" />
```

Set the outer ring label text to `text-[22px] font-black` so long vertical labels stay inside the gray band.

- [ ] **Step 2: Verify green for landing tests**

Run:

```bash
npm test -- app/lib/landing-page-contract.test.mjs app/lib/theme-contract.test.mjs
```

Expected: passes with the inward path contracts and single icon trigger.

### Task 4: Full Verification And Release Prep

**Files:**
- No new files beyond this plan and the edited web files.

- [ ] **Step 1: Run full web checks**

Run:

```bash
npm test
npm run typecheck
npm run build
```

Expected: all commands exit 0.

- [ ] **Step 2: Commit and push**

Run:

```bash
git status --short
git add docs/superpowers/plans/2026-06-26-landing-theme-flywheel-fit.md web/app/lib/landing-page-contract.test.mjs web/app/lib/theme-contract.test.mjs web/app/landing-auth-actions.tsx web/app/page.tsx
git commit -m "fix: polish landing theme control and flywheel labels"
git push -u origin codex/landing-theme-icon-flywheel-fit
```

Expected: branch pushes cleanly for PR creation.
