# Recently Published Dual Buttons Design

## Problem

The `Recently Published` drawer currently renders each published-work card as one large link to Results. The stored published URL is visible inside the card, but users cannot open the published page from this surface.

Users need two explicit destinations for each published item:

1. The corresponding Results measurement item.
2. The published page represented by the canonical published URL.

## Desired Behavior

- A published-work card is a non-interactive container rather than a whole-card link.
- The card footer contains two distinct link-styled buttons:
  - `View Results` opens `/projects/{projectId}/results?article={articleId}` in the current tab.
  - `Open Published Page` opens the row's published URL in a new tab.
- The external link uses `target="_blank"` and `rel="noopener noreferrer"`.
- The published URL remains visible and truncated in the card body so users can identify the destination before opening it.
- Clicking either link closes the `Recently Published` drawer before navigation. The external page still opens in its new tab.
- The card retains its existing title, publish timestamp, workflow lineage, article type, live state, responsive two-column layout, and highlighted styling.

## Missing URL State

When a row has no published URL:

- The existing `URL missing` badge and `Published URL missing` body copy remain.
- `View Results` remains available.
- The second footer control is rendered as a disabled button labeled `Published Page Unavailable`.
- No empty `href`, new tab, or click handler is rendered for the unavailable destination.

This change does not add a new reconcile or status-check action to the recent-history card.

## Component Structure

`PublishedSection` in `web/app/projects/[id]/publishing/publishing-client.tsx` continues to own the card rendering.

Each row becomes:

- One plain card container with the existing published metadata.
- One footer action group with sibling controls.
- A Next.js `Link` for the internal Results destination.
- A standard anchor for the external published page when the URL exists.
- A disabled native button for the missing-URL state.

The controls must not be nested inside another link. This preserves valid HTML, predictable keyboard navigation, and independent click targets.

## Accessibility

- Both destinations expose explicit visible labels instead of relying on the card context.
- The published-page link includes an external-link icon marked `aria-hidden`.
- Focus-visible styling remains available on both controls.
- The unavailable published-page control is a real disabled button and cannot receive or trigger navigation.
- The card itself is not keyboard-focusable because it has no card-level action.

## Testing

Contract coverage will verify that:

- `PublishedSection` is no longer a whole-card Results `Link`.
- The footer contains both `View Results` and `Open Published Page`.
- The external link uses the row's published URL, opens in a new tab, and includes `noopener noreferrer`.
- The two controls are siblings rather than nested interactive elements.
- Both active links close the recent drawer when clicked.
- Missing URLs render the disabled `Published Page Unavailable` control without an external anchor.
- Existing Results deep-link behavior and highlighted-card behavior remain intact.

Local verification will include the targeted workflow handoff tests, the complete web test suite, TypeScript type checking, linting if supported by the repository configuration, and a production web build.

## Production Acceptance

After the PR is merged and the production deployment contains the merge:

1. Opening `Recently Published` shows two separate controls for a row with a published URL.
2. `View Results` opens the corresponding Results measurement item in the current tab.
3. `Open Published Page` opens the exact published URL in a new tab.
4. The card body and URL display do not trigger navigation.
5. A row without a published URL keeps Results available and shows a disabled published-page control.
6. The drawer has no browser console errors or broken navigation behavior.
