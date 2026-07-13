# Returned Content Action Visibility Design

## Problem

`Move back to Opportunities` completes successfully: the content action becomes
`returned` and its opportunity becomes `open`. After a full page refresh, the
Content Plan card reappears. A second click then receives the correct backend
404 because a `returned` action is no longer reversible.

Production evidence for action `9b7b8551-30a3-4e49-b545-4a50ccfe9707` showed:

- content action status: `returned`
- opportunity status: `open`
- no draft article and no publication
- the refreshed Content Plan still rendered the action as `added_to_plan`

## Root Cause

`ListVisibilityActionRows` returns every content action for a project. The
lifecycle derivation has no terminal branch for `returned` or `dismissed`, so
its default maps those statuses to `added_to_plan`. Content Plan consumes
`actions_in_loop` and therefore treats the returned action as accepted work.

The first successful mutation removed the card only from React state. A full
refresh rebuilt state from the incorrect Visibility Summary response.

## Design

Use defense in depth while keeping the backend authoritative:

1. `ListVisibilityActionRows` excludes `returned` and `dismissed` actions.
   These actions are no longer in the active loop and must not contribute to
   active lifecycle counts.
2. Content Plan also rejects `returned` and `dismissed` action statuses before
   rendering. This prevents stale, cached, or older API payloads from reviving
   a terminal card.
3. The returned opportunity remains `open`, so it appears once in
   `open_opportunities` and contributes to the `detected` count.
4. The return mutation and its 404 guard remain unchanged. A duplicate return
   request should continue to be rejected as non-reversible.

## Error Handling

No new error state is introduced. A first valid return remains a 200. A replay
against a terminal action remains a 404. The UI prevents the replay by removing
terminal actions from both authoritative and defensive read paths.

## Tests

- A database query contract test proves Visibility Summary excludes
  `returned` and `dismissed` actions.
- A frontend logic test proves Content Plan accepts active stages but rejects
  terminal action statuses even if the lifecycle stage is stale.
- Existing move-back transaction and API error-classification tests remain
  unchanged and must continue to pass.
- Full Go and web suites, typecheck, build, deployment, and production browser
  verification are required before completion.

## Production Acceptance

After deployment, refresh Content Plan for the affected project. The returned
card must remain absent, the opportunity must be visible in the Opportunity
Queue, and no move-back 404 should be triggerable from the stale card.
