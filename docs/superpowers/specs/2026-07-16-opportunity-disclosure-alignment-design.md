# Opportunity Disclosure Alignment Design

## Goal

Align the `Run details` disclosure with the `Run timeline` disclosure so their chevrons and labels share the same left edge.

## User Experience

- Keep `Run timeline` inside the existing white progress strip.
- Keep `Run details` on its separate row below the strip.
- Move only `Run details` 15 pixels to the right, matching the progress strip's 1-pixel border plus its existing `p-3.5` inset.
- Preserve the current vertical spacing, typography, hover and focus treatments, collapsed defaults, and expansion behavior.

## Component Design

`OpportunityFindingProgress` already places `Run timeline` inside a bordered container with `p-3.5`. Both disclosure buttons use the same `px-1` internal padding, so adding `ml-[15px]` to the existing `Run details` button matches the 1-pixel border plus 14-pixel content inset without adding a wrapper or coupling the two components.

The change remains presentation-only in `OpportunityFindingStatusPanel`. It does not move either disclosure, change component ownership, add state, or alter the detail and timeline regions.

## Accessibility and Responsive Behavior

The native button, `aria-expanded`, `aria-controls`, chevron behavior, keyboard interaction, and focus ring remain unchanged. The fixed 15-pixel inset applies at every viewport size because the progress strip uses the same border and padding at every viewport size.

## Testing

Extend the existing Analysis page contract test to assert that the progress strip retains its border and `p-3.5`, and that the `Run details` toggle carries the matching `ml-[15px]` alignment class while retaining its disclosure semantics. Run the focused contract test first, followed by the complete Web test suite, typecheck, production build, and Go test suite.

After deployment, verify in production that the two chevrons and labels share a left edge on desktop and narrow viewports, while both disclosures still open and close independently.

## Non-goals

- Moving `Run details` into the white progress strip.
- Changing the content or styling of either disclosure.
- Changing the detail cards, counts, run timeline, or backend data.
- Introducing a shared disclosure component for two existing controls.

## Acceptance Criteria

1. The `Run details` chevron and label align with the `Run timeline` chevron and label.
2. `Run details` remains outside the white progress strip.
3. Both disclosures retain their current default state, accessibility attributes, and independent behavior.
4. Alignment remains correct on desktop and narrow viewports.
