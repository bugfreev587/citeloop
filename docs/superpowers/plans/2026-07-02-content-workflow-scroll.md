# Content Workflow Scroll Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect Content Plan, Review, and Publish into one continuous content workflow while keeping the existing left navigation entries.

**Architecture:** Add one client wrapper for the three existing content surfaces. The three route files keep their current URLs, but each route renders the same wrapper with a different initial section. The wrapper updates the browser URL with `history.replaceState` as the user scrolls between sections.

**Tech Stack:** Next.js App Router, React client component, existing dashboard components, existing node contract tests.

---

## Acceptance Criteria

- [ ] Clicking the left navigation entries still opens `/projects/{id}/plan`, `/projects/{id}/review`, and `/projects/{id}/publish`.
- [ ] The three routes render a shared continuous workflow surface instead of isolated page content.
- [ ] Content Plan, Review, and Publish appear as vertically adjacent sections, with no scroll snapping, so a viewport can show parts of two stages at once.
- [ ] Opening `/plan`, `/review`, or `/publish` scrolls directly to that stage.
- [ ] Manual scrolling updates the browser URL to the current stage path without a full route transition.
- [ ] Existing Review drawer behavior, Publish controls, and Content Plan actions remain in their original client components.
- [ ] Contract tests cover the route wiring, section markers, URL replacement behavior, and no-snap requirement.

## Implementation Tasks

- [x] Add a failing contract test for the continuous Content workflow acceptance criteria.
- [x] Add `web/app/projects/[id]/content-workflow-client.tsx`.
- [x] Render `TopicsClient`, `ReviewClient`, and `PublishingClient` as ordered sections.
- [x] Update `plan/page.tsx`, `review/page.tsx`, and `publish/page.tsx` to use the shared wrapper with their own initial section.
- [x] Use scroll and resize listeners with `requestAnimationFrame` to keep URL state synced.
- [x] Run full local verification.
- [ ] Push, open PR, merge, wait for production deployment, and verify production behavior.
