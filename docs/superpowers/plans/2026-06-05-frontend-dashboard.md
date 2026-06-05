# CiteLoop Frontend Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the V1 frontend dashboard inside `web/` according to `docs/PRD-CiteLoop-Frontend-Dashboard.md`, with local verification.

**Architecture:** Keep the Next.js App Router project lightweight. Add a fixed project shell, local UI primitives, centralized API normalization, and route-level client panels for workflow-heavy pages. Use existing backend routes when available and show honest unavailable states for missing backend contracts.

**Tech Stack:** Next.js 14, React 18, TypeScript, Tailwind CSS 3, lucide-react, Node test runner for DTO normalization, Next build/browser verification.

---

### Task 1: API Normalization

**Files:**
- Create: `web/app/lib/normalize.ts`
- Create: `web/app/lib/normalize.test.mjs`
- Modify: `web/package.json`
- Modify: `web/app/lib/api.ts`

- [x] Write failing tests for `normalizeNumeric`, `normalizeTime`, and `normalizeArticle`.
- [x] Run `npm test -- normalize.test.mjs` from `web/`; expected failure because `normalize.ts` is missing.
- [x] Implement `normalize.ts`.
- [x] Wire `api.ts` to return normalized frontend DTOs.
- [x] Re-run tests and typecheck.

### Task 2: App Shell and Design Tokens

**Files:**
- Modify: `web/app/globals.css`
- Modify: `web/app/layout.tsx`
- Create: `web/app/components/ui.tsx`
- Create: `web/app/components/project-shell.tsx`
- Create: `web/app/projects/[id]/layout.tsx`

- [x] Add SuperX-inspired stone background, Inter/system font stack, scrollbars, and focus styles.
- [x] Add reusable `Button`, `Badge`, `SectionHeader`, `EmptyState`, and `TextInput` primitives.
- [x] Add fixed project sidebar with project navigation, CTA, budget placeholder, and account card.
- [x] Verify the root project list still renders when API is unavailable.

### Task 3: Dashboard Pages

**Files:**
- Modify: `web/app/projects/[id]/workspace.tsx`
- Create: `web/app/projects/[id]/knowledge/page.tsx`
- Create: `web/app/projects/[id]/knowledge/knowledge-client.tsx`
- Create: `web/app/projects/[id]/topics/page.tsx`
- Create: `web/app/projects/[id]/topics/topics-client.tsx`
- Create: `web/app/projects/[id]/review/page.tsx`
- Create: `web/app/projects/[id]/review/review-client.tsx`
- Create: `web/app/projects/[id]/publishing/page.tsx`
- Create: `web/app/projects/[id]/publishing/publishing-client.tsx`
- Create: `web/app/projects/[id]/runs/page.tsx`
- Create: `web/app/projects/[id]/settings/page.tsx`

- [x] Build Home sections: learning row, next scheduled, needs review, ready distribute, recent runs unavailable state.
- [x] Build Knowledge using existing profile/inventory endpoints plus crawl summary unavailable state.
- [x] Build Topics using list/generate with duplicate-generation friendly error display.
- [x] Build Review with topic grouping, content edit, re-QA copy, backend 409 guard handling.
- [x] Build Publishing with published, ready, and waiting-on-canonical sections.
- [x] Build Runs and Settings as honest contract-aware pages.

### Task 4: Verification

**Files:**
- No new files unless defects are found.

- [x] Run `npm test`.
- [x] Run `npm run typecheck`.
- [x] Run `npm run build`.
- [x] Start the dev server.
- [x] Use browser verification on `/`, `/projects/demo`, `/projects/demo/knowledge`, `/projects/demo/topics`, `/projects/demo/review`, and `/projects/demo/publishing`.
- [x] Fix visual, console, layout, or type issues found during verification.
