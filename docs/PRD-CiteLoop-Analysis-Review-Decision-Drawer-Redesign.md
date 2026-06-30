# PRD: Analysis and Review Decision Drawer Redesign

> Date: 2026-06-30
> Scope: `/projects/[id]/analysis` and `/projects/[id]/review`
> Status: Approved for implementation

## Summary

The Analysis and Review pages should use the same decision pattern as the Home page: compact top-level metric cards, independent work cards below, and a right-side drawer for details and actions. The current Review page still feels like a split-pane queue, while Analysis already has a drawer but keeps the Growth findings header inside a framed container and uses a drawer that is slightly too narrow.

This redesign makes both pages feel like one workflow:

1. Operators scan metrics first.
2. Operators scan individual cards.
3. Operators click a card to open a wide, animated drawer.
4. Operators make the decision from a fixed action area at the bottom of the drawer.

## Goals

- Put Review top metrics into independent cards under an `Overall Metrics` heading, matching the Home page's metric-card language.
- Render Review queue items as card-style items similar to Analysis findings, not as rows in a split pane.
- Open Review details in a right-side drawer only after a card click.
- Keep Review drawer actions fixed at the bottom, including approve, edit, reject, preview, detail, and re-check actions when applicable.
- Make the Analysis and Review drawers wider than the current Analysis drawer.
- Animate both drawer scrims and panels when they appear, so the drawer slides in instead of appearing instantly.
- Move `Growth findings` on Analysis and `Needs Your Decision` on Review into standalone section titles, outside framed card containers.
- Preserve existing Review behavior for automatic recovery, one-click QA fix, manual edit, reject, re-check, preview, and approve.
- Preserve existing Analysis behavior for creating content, refresh, and technical tasks, dismissing findings, and viewing measurement.

## Non-Goals

- No backend API changes.
- No new persistence model.
- No change to Analysis recommendation ranking.
- No change to Review approval, rejection, QA repair, or publish scheduling semantics.
- No redesign of Results, Content Plan, Context, Publish, or Settings.

## User Experience

### Analysis Page

The top page header remains `Review analysis` with GSC status, Refresh, and Sync controls.

The search performance snapshot remains above the findings because it gives useful context for prioritization.

`Growth findings` becomes a standalone section title with badges for the number of findings and loop items. It must not live inside a framed container. Finding cards remain the primary scan surface. Clicking a finding card opens a right-side drawer with details, evidence, confidence, and a fixed bottom action area.

The Analysis drawer should:

- Use a scrim fade-in.
- Slide in from the right.
- Use a width around `max-w-2xl` on desktop and full width on small screens.
- Keep details scrollable.
- Keep actions fixed at the bottom with safe-area padding.

### Review Page

The top page header remains `Review` with Refresh and bulk approve when ready drafts exist.

Below the header, Review shows an `Overall Metrics` section with independent metric cards:

- `Needs your decision`
- `Ready to approve`
- `CiteLoop is handling`
- `Total in review`

Below metrics, Review shows a standalone `Needs Your Decision` section title. This title is outside any framed list container. The section contains card-style draft items ordered by current review state. Cards show state badges, platform/kind, topic, title, scores or recovery status, and a short reason. Clicking a card opens the Review drawer.

The Review drawer should:

- Use a scrim fade-in.
- Slide in from the right.
- Use a width around `max-w-2xl` on desktop and full width on small screens.
- Keep the inspector body scrollable.
- Keep primary actions fixed at the bottom.
- Preserve body panels for QA reason, suggested fixes, claim evidence map, search appearance, SEO contribution, and draft editor.
- Put repeated decision actions in the bottom action area instead of burying them inside the details body.

## Interaction Rules

- Card click opens the drawer.
- Escape closes the drawer.
- Scrim click closes the drawer.
- If the selected item disappears after refresh or mutation, close the drawer.
- No Review drawer should open automatically on page load.
- Drawer content must not scroll the body underneath.
- Drawer animation must use transform and opacity, not width/left/top animation.

## Acceptance Criteria

1. `/projects/[id]/analysis` renders `Growth findings` as a standalone section title outside the findings card grid.
2. `/projects/[id]/analysis` finding cards still open a details drawer.
3. `/projects/[id]/analysis` drawer uses a wider panel and animated slide-in/fade-in classes.
4. `/projects/[id]/review` renders `Overall Metrics` above the review card grid.
5. `/projects/[id]/review` renders one independent metric card for each overall metric.
6. `/projects/[id]/review` renders `Needs Your Decision` as a standalone section title outside a framed queue container.
7. `/projects/[id]/review` review items render as card-style buttons.
8. `/projects/[id]/review` opens a right-side drawer only after selecting a card.
9. `/projects/[id]/review` drawer actions are fixed at the bottom and include the existing relevant actions for the selected draft state.
10. Both Analysis and Review drawers animate in with a slide effect.
11. Existing web contract tests pass.
12. TypeScript typecheck passes.
13. Production deployment is verified at the user-provided Analysis and Review URLs.

## Test Plan

- Add contract assertions for the new section markers, drawer width, animation classes, and bottom action areas.
- Run the new contract assertions and confirm they fail before implementation.
- Implement the UI changes.
- Run the targeted contract test and confirm it passes.
- Run the full web test suite.
- Run TypeScript typecheck.
- Run a local visual check in a browser against Analysis and Review.
- After PR merge and deployment, verify production Analysis and Review URLs.
