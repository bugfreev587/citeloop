# Site Fix Resizable JSON Design

## Goal

Keep Site Fix detail drawers compact by showing five lines of every JSON/detail payload by default, while preserving access to the complete payload through vertical scrolling, downward resizing, text selection, and copying.

## Scope

The behavior applies uniformly to every structured payload in the Site Fix drawer:

- Evidence
- Proposed fix
- Acceptance checks
- Legacy provenance, when present
- Verification
- Verification attempts, when present
- AI coding fix JSON

The lifecycle strip, summary notice, finding metadata, application details, and drawer footer are unchanged.

## Interaction Design

Each payload uses a visible text viewport with a content height equal to five rows at the configured line height. Content beyond those rows remains in the DOM and can be reached with the viewport's vertical scrollbar.

The viewport exposes the browser's vertical resize handle. Its minimum height equals the initial five-row height, so the user can drag it downward to reveal more content but cannot collapse it below the default. A bounded maximum height prevents a resized payload from becoming impractically tall inside the drawer.

Payload text remains selectable. Users can drag across text and use the operating system's normal copy command. The existing **Copy fix JSON** action remains available for copying the complete canonical repair payload in one click.

Long values continue to wrap within the drawer instead of creating horizontal page overflow.

## Implementation

Define one shared viewport class inside `web/app/projects/[id]/site-fixes/site-fixes-client.tsx`. Apply it to the reusable `DetailBlock` payload and the AI coding payload so all JSON/detail surfaces have identical height, vertical resize, overflow, and selection behavior.

The viewport uses a content-box height of five `1.5rem` lines. The minimum height matches the initial height, and the maximum height is capped at `30rem`. Existing light and dark visual treatments remain specific to their containing blocks.

No API, data model, lifecycle, or clipboard logic changes are required.

## Accessibility

All payload content remains native text inside a `pre` element, preserving keyboard text selection and assistive-technology access. Scrolling and resizing use native browser behavior. Existing headings continue to label each payload.

## Testing

Add a focused contract test that fails until both payload render paths use the shared viewport contract. It will assert:

- a five-line content height;
- the matching minimum height, so resizing only expands from the default;
- a bounded maximum height;
- vertical-only resizing;
- internal overflow scrolling;
- explicit text selection;
- reuse by both `DetailBlock` and AI coding fix JSON.

Run the focused contract test first for the red/green cycle, then the complete Web test suite, TypeScript typecheck, production build, and a browser interaction check of scrolling, resizing, and text selection.
