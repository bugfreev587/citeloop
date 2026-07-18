# Disable Publishing Move-Back Design

## Problem

The Publish card keeps `Move back to Opportunities` enabled while its canonical article is actively publishing. Triggering a workflow rollback while the publisher is changing the same article can produce conflicting lifecycle transitions.

## Decision

Keep the existing control visible so the workflow action remains discoverable, but disable it whenever the card action is `publishing`.

- Before publishing, the control remains available for linked, returnable content actions.
- During publishing, the control is visibly disabled and cannot invoke the return API.
- After a publish failure, the control becomes available again when the underlying content action remains returnable.
- After successful publication, the item leaves the ready-to-post card, so the control is no longer rendered.
- Existing global busy-state disabling remains unchanged.

Hiding the control is rejected because it makes the temporary restriction look like a missing capability. Allowing the click and showing an explanatory message is rejected because it permits an avoidable conflicting action.

## Implementation Boundary

The change belongs in `ReadyNowStrip` in `web/app/projects/[id]/publishing/publishing-client.tsx`, where the card already knows `item.action`.

The button's disabled condition will cover both:

1. any existing in-flight UI mutation represented by `busy`; and
2. the card's `publishing` action.

No backend API, database, publisher state, copy, or layout changes are required.

## Accessibility

Use the existing shared `Button` disabled state so the rendered native button is non-interactive for pointer and keyboard users and retains the product's disabled styling.

## Testing

A focused source contract test will isolate the `ReadyNowStrip` block and assert that the `Move back to Opportunities` button is disabled for `item.action === "publishing"` in addition to the existing busy state. The test must fail against the current implementation before the component is changed.

Verification will include the focused contract test, the complete web test suite, TypeScript checking, and a production web build.

## Production Acceptance

After deployment:

1. A ready-to-post card in `Publishing` state still shows `Move back to Opportunities`.
2. The control is visibly disabled and cannot trigger the return request.
3. A failed pre-publication card with a returnable source action shows the control enabled again.
4. Other card actions and layout remain unchanged.
