# PRD: Content Workflow Section Identity

> Date: 2026-07-02
> Status: execution PRD
> Scope: Content Plan, Review, and Publish continuous workflow surface.

## 1. Problem

Content Plan, Review, and Publish now form one vertically continuous workflow.
That helps users understand the content lifecycle, but it also creates two UX
risks:

1. Users can scroll across stage boundaries without a strong sense of which page
   they are currently reading.
2. Sidebar entry clicks can land on the wrong vertical position while async
   child content is still changing height. In production this showed two bugs:
   clicking Review left the Review section lower than the top of the work area,
   and clicking Publish could still show Content Plan selected and visible.

## 2. Product Goal

Keep the continuous scroll behavior, but make each workflow stage clearly
identifiable as its own page. The left navigation entries stay unchanged and
clicking them must land the corresponding workflow stage at the top of the main
work area.

## 3. UX Requirements

### 3.1 Stage Identity

Each workflow stage must have a visible page-level identity:

- Content Plan is Step 1 of 3.
- Review is Step 2 of 3.
- Publish is Step 3 of 3.

The page-level title must be visually stronger than module titles inside the
stage. The stage title text is:

- `Content Plan`
- `Review`
- `Publish`

The existing `Publishing` page title must be normalized to `Publish` so the page
title, route, and left navigation label match.

### 3.2 Heading Hierarchy

The stage title is the primary heading for the stage. Module titles inside the
stage remain secondary headings.

Examples:

- Primary stage title: `Content Plan`
- Secondary module titles: `Plan pulse`, `Backlog`
- Primary stage title: `Review`
- Secondary module titles: `Overall Metrics`, `Needs Your Decision`
- Primary stage title: `Publish`
- Secondary module titles: `Ready to publish`, `Scheduled`, `Publishing failed`

### 3.3 Visual Separation

Stage boundaries must be visible but calm. Use lightweight treatment rather than
large, unrelated page backgrounds:

- A subtle tinted section shell per stage.
- A small stage accent stripe.
- A step label, for example `Step 2 of 3`.
- Existing cards and controls remain visually subordinate to the stage shell.

The workflow must not use scroll snapping. Users must still be able to see the
bottom of one stage and the top of the next stage in the same viewport.

### 3.4 Sidebar Entry Behavior

Clicking each left navigation entry must:

- Navigate to the matching route.
- Keep the matching sidebar entry active.
- Scroll the matching stage to the top of the main work area.
- Avoid being overridden by the scroll spy while the page is still settling.

## 4. Acceptance Criteria

### 4.1 Documentation Gate

- [ ] This PRD exists in `docs/`.
- [ ] The PRD names all three workflow stages and the two reported click bugs.
- [ ] The PRD includes stage-by-stage acceptance criteria.

### 4.2 Content Plan Acceptance Criteria

- [ ] Opening `/projects/{id}/plan` shows `Content Plan` as the visible
      stage-level title near the top of the main work area.
- [ ] The Content Plan stage displays `Step 1 of 3`.
- [ ] `Plan pulse` and `Backlog` render as module-level headings visually below
      the stage title.
- [ ] The Content Plan section has a subtle stage shell or accent that is
      distinguishable from Review and Publish without breaking the continuous
      workflow.
- [ ] The left navigation highlights `Content Plan`.

### 4.3 Review Acceptance Criteria

- [ ] Clicking the left navigation `Review` entry lands the Review stage at the
      top of the main work area; the top of Review must not remain below the
      previous stage's Backlog content.
- [ ] Opening `/projects/{id}/review` directly also lands the Review stage at
      the top of the main work area.
- [ ] The Review stage displays `Step 2 of 3`.
- [ ] `Review` is the visible stage-level title, and review module titles such
      as `Overall Metrics` and `Needs Your Decision` remain visually secondary.
- [ ] The left navigation highlights `Review`.

### 4.4 Publish Acceptance Criteria

- [ ] Clicking the left navigation `Publish` entry lands the Publish stage at
      the top of the main work area.
- [ ] Opening `/projects/{id}/publish` directly also lands the Publish stage at
      the top of the main work area.
- [ ] The Publish stage displays `Step 3 of 3`.
- [ ] `Publish` is the visible stage-level title; the UI must not use
      `Publishing` as the page-level heading.
- [ ] The left navigation highlights `Publish`; `Content Plan` must not remain
      selected after clicking Publish.

### 4.5 Scroll Continuity Acceptance Criteria

- [ ] Manual scrolling can show the lower part of one stage and the upper part
      of the next stage in one viewport.
- [ ] Manual scrolling updates the browser URL to `/plan`, `/review`, or
      `/publish` according to the current stage.
- [ ] Manual scrolling updates the left navigation active state without a full
      page refresh.
- [ ] No `scroll-snap` or mandatory snapping styles are introduced.

### 4.6 Verification Acceptance Criteria

- [ ] Contract tests fail before implementation and pass after implementation.
- [ ] `npm test` passes in `web`.
- [ ] `npm run typecheck` passes in `web`.
- [ ] `npm run build` passes in `web`.
- [ ] `go test ./...` passes at repo root.
- [ ] Browser verification confirms the three sidebar click flows and manual
      scroll flow.
- [ ] After PR merge and deployment, the same acceptance criteria are verified
      on production before the work is reported complete.

## 5. Non-Goals

- Do not change the left navigation IA.
- Do not split the three workflow stages back into isolated pages.
- Do not introduce scroll snapping.
- Do not redesign Review cards, Publish lanes, or Content Plan backlog cards
  beyond the heading and section-identity requirements.
- Do not add new frontend dependencies.

## 6. Implementation Notes

The likely root cause of the click bugs is that route-driven initial scroll
currently happens once. Child content can load or resize after that first scroll,
which pushes the target stage out of position. The workflow shell should keep a
short-lived pending target when the route asks for a stage and retry the scroll
until the stage is aligned or a small timeout expires. While that pending target
is settling, the scroll spy should not replace the route with a different stage.
