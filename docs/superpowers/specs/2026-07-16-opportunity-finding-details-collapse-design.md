# Opportunity Finding Details Collapse Design

## Goal

Reduce visual density in the Opportunity Finding status panel by hiding the five stage summary cards and the three queue counts until the user asks to see them.

## User Experience

- Add a `Run details` disclosure control where the summary block currently begins.
- The control is closed on every page load.
- Opening it reveals the complete stage summary grid and the `open`, `in loop`, and `already handled` counts as one region.
- Closing it hides that entire region again.
- The disclosure choice is session-local component state and is not persisted.
- Opportunity Finding progress, actions, status badges, and durable failure alerts remain visible regardless of the disclosure state.

## Component Design

`OpportunityFindingStatusPanel` owns a boolean disclosure state initialized to `false`. A button toggles that state and exposes it through `aria-expanded`. The button references the conditional detail region through `aria-controls`; the region has the matching stable ID.

The existing summary fallback remains unchanged. Only its presentation changes: the summary grid and counts move inside the conditional detail region. A chevron communicates state and rotates when the region is open, following the existing controlled disclosure pattern used by the Opportunity Finding progress component.

Status refreshes do not force the region open or closed. Because the panel component remains mounted while data refreshes, a user who opens the region can continue reading updated details until navigation or reload resets the default.

## Error Handling

The durable `Last finding needs attention` alert remains outside the disclosure. Failures therefore stay visible even when run details are closed. Missing status data continues to use the existing fallback summary and does not change disclosure behavior.

## Accessibility

- Use a native button for keyboard activation.
- Set `aria-expanded` to the current disclosure state.
- Set `aria-controls` to the detail region ID.
- Mark the decorative chevron as hidden from assistive technology.
- Keep visible control copy concise and stable: `Run details`.

## Testing

Extend the Analysis page contract test to verify:

- disclosure state starts at `false`;
- the `Run details` button exposes `aria-expanded` and `aria-controls`;
- the summary grid and all three counts are inside the conditionally rendered region;
- the durable failure alert remains outside that region.

Run the focused contract test first, then the complete Web test suite, typecheck, and production build.

## Non-goals

- Persisting the disclosure preference.
- Changing Opportunity Finding API responses or backend behavior.
- Collapsing progress, errors, actions, or the status header.
- Redesigning the summary cards or their copy.

## Acceptance Criteria

1. On initial render, the five stage summary cards and three counts are not visible.
2. Activating `Run details` reveals all eight pieces of information together.
3. Activating the control again hides them.
4. The disclosure works with keyboard input and exposes its state to assistive technology.
5. Opportunity Finding progress and actionable errors remain visible while details are closed.
