# User-facing Opportunity Finding Results

**Date:** 2026-07-16

## Goal

Remove the Growth Radar engineering funnel from the customer-facing Analysis page. Users should learn only whether Opportunity Finding produced actionable cards, found nothing new, or could not finish.

## Product decision

Opportunity cards in the existing Opportunity Queue are the success result. A separate success summary is redundant and must not render.

The page will present three outcomes:

1. **Opportunities created:** render no Growth Radar result panel. The new cards appear directly in Opportunity Queue.
2. **Completed with no new opportunities:** render one concise neutral message explaining that there are no new opportunities and that CiteLoop will keep checking as signals change.
3. **Incomplete or failed:** render one concise retry message. Do not expose the backend error, provider state, source failures, or diagnostic reason codes.

## User-facing content

The healthy zero-result message will use plain language:

- Title: `No new opportunities found`
- Detail: `Your current opportunity queue is up to date. CiteLoop will keep looking as your site and market change.`

The incomplete or failed message will use plain language:

- Title: `Opportunity finding couldn't finish`
- Detail: `We couldn't complete every check. Please try again.`

The existing `Run finding` action remains the retry path. The result message will not add another button.

## Removed customer-facing information

The Analysis page will no longer show:

- the `Growth Radar funnel` name;
- candidates generated;
- active watchlist and rejected counts;
- prompt rotation and targeted prompt counts;
- provider cost;
- raw or humanized diagnostic reason badges;
- raw backend failure details.

Growth Radar collection, scoring, persistence, watchlist lifecycle, and diagnostics APIs remain unchanged. This is a presentation-only change; internal data remains available to engineering through existing backend records and APIs.

## Component behavior and data flow

The Analysis client continues loading Growth Radar diagnostics because the summary status is needed to distinguish a healthy zero result from an incomplete result.

The customer-facing result component derives one of three presentation states from the latest combined funnel:

- `hidden` when `candidates.created > 0`;
- `empty` when no candidates were created and the funnel status is neither `degraded` nor `failed`;
- `incomplete` when no candidates were created and the funnel status is `degraded` or `failed`.

The existing Opportunity Finding run failure alert will stop rendering `last_run.error`. It will use the same plain-language failure title and detail so backend or provider terminology cannot leak through a separate path.

## Visual treatment

Zero-result and incomplete messages use a small, low-emphasis status surface in the current Analysis layout. They must not resemble a metrics dashboard and contain no badges, grids, counters, diagnostic labels, or expandable engineering details.

## Testing

Automated contract tests will verify that:

- successful runs do not render a Growth Radar panel or success summary;
- healthy zero-result runs produce the neutral user-facing message;
- degraded or failed zero-result runs produce the retry message;
- raw backend error details are not rendered;
- engineering labels and diagnostic reason badges are absent from the Analysis client.

Type checking, the complete web test suite, the production build, and relevant Go tests will run before merge. After the PR is merged and deployment completes, the production Analysis page will be checked to confirm that successful Opportunity Finding shows only the Opportunity Queue cards and that no Growth Radar engineering language remains.
