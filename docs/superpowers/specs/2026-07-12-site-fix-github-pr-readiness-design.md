# Site Fix GitHub PR Readiness and Automatic PR Design

## Goal

Make every Site Fix an existing-site source change delivered through a reviewable GitHub pull request. A user who has a ready GitHub connection can approve a fix once, receive an `Open PR` action, merge it in GitHub, and then watch CiteLoop advance through deployment and production verification.

The Site Fix page must stay lightweight. It reads one persisted GitHub PR readiness status and must not perform live GitHub repository or permission checks during page load.

## Current Problems

The current lifecycle separates `Approve fix` from `Apply fix`. Approval only changes the Site Fix status; Apply later generates an application and attempts to create a PR. This creates an unexplained second confirmation even though GitHub PR review is already the human review boundary.

The drawer renders a PR URL only as a low-emphasis `Open source change` link inside the Application section. It does not expose an `Open PR` footer action, and it does not automatically refresh merge, deployment, or verification progress.

The lifecycle strip treats `Approved` as the current step rather than a completed milestone, so a successfully approved fix does not show an approval checkmark.

The automatic PR path is also artificially limited to title and meta-description mutations. Sitemap, canonical, schema, internal-link, robots, and other Site Fix families fall back to manual application even when GitHub is connected. That behavior conflicts with the product rule that all Site Fixes modify existing website source through PRs.

## Chosen Approach

Use a persisted GitHub PR readiness model plus a generic, repository-grounded multi-file patch engine.

This was chosen over two alternatives:

1. Keeping Approve and Apply separate would retain an unnecessary user action and would not solve the unclear lifecycle.
2. Adding one deterministic adapter per Site Fix family would repeatedly leave new repair types unsupported and would preserve the current metadata-only limitation.

The generic engine will read the actual selected repository and base branch, select relevant existing text files, generate bounded exact replacements, validate them against the base SHAs and approved evidence, and create one atomic multi-file commit and PR.

## GitHub PR Readiness

### Persisted state

The GitHub publisher connection owns these additional fields:

- `pr_readiness_status`
- `pr_readiness_checked_at`
- `pr_readiness_detail`

The public status values are:

- `not_connected`: no enabled, connected GitHub publisher exists.
- `not_checked`: the connection or repository target changed and needs a fresh permission check.
- `ready`: the selected repository and base branch are accessible and the credential has the required write permissions.
- `permission_missing`: the credential lacks `contents: write` or `pull requests: write`.
- `repository_unavailable`: the selected repository or base branch cannot be accessed.
- `error`: the latest readiness check failed for another safe, user-presentable reason.

`checking` is a transient Settings UI state, not a durable state that can become stranded after a process interruption.

Changing the GitHub installation, credential, selected repository, base branch, or enabled state resets readiness to `not_checked` or `not_connected`. Readiness errors never expose tokens, installation secrets, or raw GitHub response bodies.

### Settings behavior

Settings -> Publisher is the live-check boundary. It displays the current status, its last-check time, and a user-facing explanation.

Settings runs the live readiness check when:

- the Publisher settings panel loads;
- a GitHub installation is connected or reused;
- a repository or base branch is selected;
- a credential is saved, revoked, or replaced;
- the connection is enabled again; or
- the user selects `Check again`.

For a GitHub App connection, the check verifies the installation grants `contents: write` and `pull_requests: write`, the selected repository belongs to the installation, and the configured base branch can be read. Advanced token connections must prove equivalent repository write access; an ambiguous token is not treated as ready.

Settings persists the result and shows one of these user-facing states:

- `Ready to create repair PRs`
- `Connect GitHub to create repair PRs`
- `GitHub needs contents and pull-request write access`
- `The selected repository or branch is unavailable`
- `GitHub readiness could not be checked`

### API boundary

`GET /api/projects/{projectID}/integrations/github/pr-readiness` reads only the persisted database state. It performs no GitHub network calls. This is the only readiness request made by the Site Fix page.

`POST /api/projects/{projectID}/integrations/github/pr-readiness/check` performs the live check, persists the result, and is used by Settings.

Both return a redacted object containing `status`, `checked_at`, `detail`, `repo`, and `branch`.

The approval/PR operation performs a final server-side check before mutation. This protects against stale readiness if permissions were revoked after the last Settings check. A permission or repository failure updates the persisted readiness state so the next lightweight read immediately reflects it.

## Approval and Automatic PR Flow

### Proposed fixes

When persisted readiness is `ready`, `Approve Fix` is enabled. One click:

1. revalidates GitHub mutation authority on the server;
2. persists approval and its timestamp;
3. prepares repository-grounded file changes;
4. independently validates the generated change against the approved evidence;
5. creates the branch, commit, and pull request; and
6. returns the updated Site Fix, application, and PR URL.

The button shows `Approving & creating PR...` while the request runs. The click is an explicit on-demand authorization for this Site Fix generation; it does not enable scheduled Doctor AI work for the project.

If PR generation fails after approval, approval remains durable. The drawer shows the safe failure reason and a `Retry creating PR` action. The user is never asked to approve the same revision twice.

### Historical approved fixes

Existing `approved` rows without an application or PR predate the combined flow. They show a one-time `Create PR` recovery action when readiness is `ready`, or the same readiness warning when it is not. New approvals should not normally stop in this state.

### Not-ready behavior

When readiness is not `ready`, `Approve Fix` cannot execute. A focusable wrapper preserves hover and keyboard behavior around the disabled control. Hover or focus displays a warning with the readiness-specific reason and a link to `/projects/{projectID}/settings#publisher`.

The default warning copy is:

> Connect GitHub and grant repository write access so CiteLoop can create a repair PR automatically.

The backend enforces the same gate even if a caller bypasses the UI.

## Generic Existing-Source Patch Engine

Every Site Fix uses the same repository-backed pipeline. There is no manual-apply success path.

1. Read the selected repository tree at the configured base branch.
2. Filter to existing text source files and exclude dependencies, generated output, binaries, secrets, lockfiles, and workflow files.
3. Select a bounded set of likely source files using the target URL, finding kind, proposed fix, acceptance tests, conventional web source names, and a repository-source selection model call.
4. Read the selected files and retain each GitHub blob SHA.
5. Generate a structured patch containing existing paths, base SHAs, and exact `old_text` -> `new_text` replacements.
6. Apply every replacement in memory. Each `old_text` must exist exactly once, every path and SHA must match the source snapshot, and the resulting files must remain bounded text files.
7. Run the existing independent grounding check over the approved evidence plus the actual before/after change. Reject added unsupported product claims, intent drift, unrelated edits, or source-association changes not authorized by the Site Fix.
8. Re-read the base SHAs immediately before mutation. A changed file produces a retryable source-conflict failure rather than overwriting newer work.
9. Create Git blobs, one tree, and one commit, then attach the fix-specific branch and open a pull request. The base branch is never mutated directly.

The first implementation limits one Site Fix to eight existing files, 128 KiB per file, and 512 KiB total source input. These are safety bounds, not repair-family restrictions. A fix that cannot be represented safely fails with a retryable explanation instead of falling back to manual application.

The PR branch remains deterministic (`citeloop/doctor-site-fix-{shortID}`), and existing-PR reconciliation remains idempotent. Retrying after an interrupted request reuses the matching branch or PR instead of creating duplicates.

## Drawer and Lifecycle UX

The drawer footer uses these actions:

- Proposed + ready: `Approve Fix`
- Proposed + not ready: disabled `Approve Fix` with readiness warning
- Historical approved or retryable preparation: `Create PR` or `Retry creating PR`
- PR URL available: primary `Open PR`, opening GitHub in a new tab
- Awaiting deploy or retryable verification: the existing verification action where applicable

The Application section also shows the PR number, repository, source branch, current PR state, and the same `Open PR` link.

While an active drawer is open, the page periodically refreshes the existing Site Fix list endpoint. It does not run readiness checks. Polling continues through preparing, PR-open, awaiting-deploy, and verifying states, and stops for stable or closed states.

User-facing status copy is:

- `Creating the repair PR`
- `Waiting for PR review and merge`
- `PR merged - waiting for deploy`
- `Checking the production change`
- `Verified`

The lifecycle strip uses milestone completion rather than treating the persisted status as a zero-based step:

- Proposed: `Finding` is complete; `Approved` is current.
- Approved or later: `Approved` is complete; `Applied / deploy` is current.
- Deployment observed or verifying: `Applied / deploy` is complete; `Verified` is current.
- Verified: all four milestones are complete.

This directly fixes the missing checkmark after user approval.

## Deployment and Verification

The existing scheduler remains responsible for polling open PRs. An open PR stays in `applying`. A merged PR transitions the canonical Site Fix to `awaiting_deploy`; production evidence then transitions it to `verifying`, and acceptance evidence closes it as `verified`.

The UI never labels PR creation or PR merge as verification. It exposes the PR throughout open, merged, deployed, and verified history so a user can always return to the review record.

## Failure Handling

- Connection or permission failure before approval: do not approve; update readiness and return a conflict response.
- Generation or grounding failure after approval: retain approval, record the application failure, and expose `Retry creating PR`.
- Source SHA conflict: do not write; reload repository source on retry.
- PR creation interrupted after commit creation: reconcile the deterministic branch and existing PR before any new mutation.
- PR closed without merge: retain the PR link, show that it was closed, and expose the existing follow-up path.
- Deploy or verification failure: preserve the merged PR link and use the existing retryable verification lifecycle.

## Testing

Implementation follows test-driven development and covers:

- migration and query contracts for persisted readiness;
- GitHub App permission parsing and redacted readiness responses;
- Settings-only live checks and Site Fix read-only readiness fetches;
- server-side approval gating and readiness invalidation on GitHub failures;
- repository tree filtering and bounded source selection;
- exact replacement validation, SHA conflicts, unrelated edits, and grounding rejection;
- atomic multi-file Git commit and idempotent PR reconciliation;
- approval automatically returning an application with a PR URL;
- disabled approval warning and Settings link;
- prominent `Open PR` rendering;
- lifecycle milestone checkmarks;
- active drawer polling without live readiness checks; and
- existing PR merge -> deploy -> verify scheduler behavior.

Full Go tests, Web tests, type checking, and production builds must pass. After merge, both the API and Web deployments must reach the merge commit. Production verification must cover Settings readiness, a Site Fix approval that creates a PR, the `Open PR` action, the Approved checkmark, and visible waiting states through merge/deploy/verification.

## Non-Goals

- CiteLoop will not merge repair PRs for the user.
- Site Fix approval will not write directly to the configured base branch.
- The Site Fix page will not perform live GitHub permission checks on load.
- This change will not enable scheduled Doctor AI authority or move Site Fixes into the Growth measurement loop.
