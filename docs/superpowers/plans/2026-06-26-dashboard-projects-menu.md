# Dashboard Projects Menu Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Dashboard sidebar footer popover for project switching and account controls.

**Architecture:** Extract a client-side `ProjectAccountMenu` leaf component from `ProjectShell`. It receives the current project, fetches the account project list through `useApi`, and owns popover/theme/account/logout interactions locally.

**Tech Stack:** Next.js App Router, React client components, Tailwind CSS v3, Clerk, lucide-react, Node contract tests.

---

### Task 1: Contract Tests

**Files:**
- Modify: `web/app/lib/project-management-contract.test.mjs`

- [ ] Replace the old footer-link contract with expectations for a popover button, project list fetch, Account Settings/Admin rows, Theme controls, Logout, and no direct `/projects` footer link.
- [ ] Run `node --test app/lib/project-management-contract.test.mjs` from `web` and confirm the new test fails.

### Task 2: Account Menu Component

**Files:**
- Create: `web/app/components/project-account-menu.tsx`
- Modify: `web/app/components/project-shell.tsx`

- [ ] Implement `ProjectAccountMenu` as a client component using `useApi`, `useClerk`, `useRouter`, and lucide icons.
- [ ] Fetch projects when the menu opens, fall back to the current project while loading, and navigate project rows to `/projects/{id}`.
- [ ] Provide Account Settings, conditional Admin, Light/Dark theme controls, and Logout sections with faint dividers.
- [ ] Replace the old footer identity link/UserButton area in `ProjectShell` with `ProjectAccountMenu`.
- [ ] Run `npm test -- app/lib/project-management-contract.test.mjs` and confirm it passes.

### Task 3: Verification

**Files:**
- Modify only as required by verification failures.

- [ ] Run `npm test`.
- [ ] Run `npm run typecheck`.
- [ ] Run `npm run build`.
- [ ] Start the local web app and verify the popover visually in the browser.
