# Site Fix Verified Status Design

## Problem

A source-backed Site Fix can complete its automation correctly while still looking unfinished in the Site Fixes UI. For UniPost PR #175, production data shows that CiteLoop detected the merge, verified the deployed metadata automatically, and advanced the content action to `measuring`. The action's nested `publisher_result`, however, remained `github_pr_merged`, and the Site Fix drawer labels every stored pull-request URL as `Open PR`.

This creates two conflicting views of the same action: the authoritative verification fields say the fix is complete, while the execution CTA still describes an open pull request.

## Desired Behavior

After CiteLoop automatically verifies a merged Site Fix:

- The content action remains in `measuring` with `verified_at` and `verification_snapshot` populated.
- Its `output_snapshot.publisher_result.status` advances to `verified` and retains the PR URL, merged state, repository, target URL, and verification provenance.
- Site Fix cards and drawers show `Verified automatically` when the verification source begins with `auto_`.
- A merged PR link is labeled `View merged PR`, not `Open PR`.
- The manual `Mark applied` action is hidden after verification has already succeeded.
- Open and closed PRs retain accurate link labels.

## Architecture and Data Flow

The scheduler remains the source of truth for automatic verification. When a URL check or grace-period check succeeds, it will build one verification snapshot and one verified publisher result. A dedicated SQL query will update the content action's lifecycle fields and publisher result together so the action cannot be left as `verification_pending` by the existing PR-result query.

The frontend will derive presentation labels from the normalized action. `verified_at` and `verification_snapshot.source` control the verification label; PR state and publisher-result status control the link label. The UI will not infer that a PR is still open merely because a URL exists.

## Error Handling

Scheduler errors continue to be logged and retried by the existing five-minute reconciliation loop. The new content-action update returns an error like the current verification update, so a failed persistence step is visible in scheduler logs. No manual database repair or migration is required for existing verified actions because the frontend treats `verified_at` as authoritative even when an older publisher result is stale.

## Testing

- A Go unit test will prove the verified publisher-result payload retains PR identity and records `status: verified`, `github_pr_state: merged`, source, and timestamp.
- A database contract test will prove the dedicated query sets `measuring`, verification fields, and the nested publisher result in one update.
- A web contract test will prove Site Fixes consume `Verified automatically` and `View merged PR`, and hide `Mark applied` after verification.
- Full Go tests, web tests, type checking, and a production web build will run before publishing.

## Production Acceptance

After merge and deployment, the project Site Fixes page must show the UniPost action as automatically verified/measuring, expose PR #175 as `View merged PR`, and omit the manual `Mark applied` control. The Railway API deployment and Vercel web deployment must both complete successfully before the production page is checked.
