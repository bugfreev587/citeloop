# Doctor Recent Findings Design

## Problem

Doctor findings currently remain in the primary `Findings` grid after the user has already converted them into Site Fixes. This makes completed triage look unfinished, invites duplicate Site Fix creation, and obscures the handoff from diagnosis to execution.

Doctor needs a lightweight, durable handoff history without becoming a second Site Fixes workspace. The primary grid should contain findings that still need a decision, while recently handed-off findings should provide only a forward link into the canonical Site Fix.

## Desired Behavior

- As soon as any Doctor finding has a linked Site Fix, that finding leaves the primary `Findings` grid.
- A `Recent Findings` control appears at the upper right of the Findings section and shows the number of current forward links.
- The control opens a right-side drawer containing compact rows for recently handed-off findings.
- Selecting a row navigates to the corresponding item on the Site Fixes page.
- A recent link disappears automatically after its Site Fix has created a pull request.
- A terminal or unsuccessful Site Fix remains in Recent Findings when no pull request was created, so the user can still inspect it.
- The user can dismiss any recent link after confirming. Dismissal only removes Doctor's forward link; it never deletes, terminates, or changes the Site Fix or its pull request.
- Dismissal is stored per Site Fix. A later Site Fix revision for the same finding is eligible to appear again.

## Product Boundaries

Doctor owns diagnosis, optimization discovery, and the decision to hand a finding to execution. Site Fixes owns generation, approval, pull requests, application, and verification. Recent Findings is therefore a navigation history, not another execution queue: it exposes the finding identity, target, Site Fix state, and one forward link, but no generation or lifecycle controls.

The main Findings grid remains deduplicated even after a recent link is dismissed or removed because of a pull request. Once a finding has any Site Fix, it is already handed off and must not reappear as new diagnostic work.

## Data Model

Two nullable fields are added to `site_fixes`:

- `doctor_link_dismissed_at timestamptz`
- `doctor_link_dismissed_by text`

A consistency constraint requires both fields to be null or both to be populated. The existing `doctor_finding_id` remains immutable provenance and is not cleared by dismissal. Existing Site Fix rows start with both new fields null.

This metadata deliberately does not change Site Fix status, work identity, generation input, arbitration, or bucket versions. It records only a Doctor presentation preference.

## API

Doctor exposes an authenticated, project-scoped operation:

`POST /projects/{projectID}/doctor/site-fixes/{siteFixID}/dismiss-link`

The operation verifies that the Site Fix belongs to the project and is linked to a Doctor finding, then idempotently records the authenticated actor and dismissal time. It returns the updated Site Fix. Repeated requests return the already-dismissed resource without touching its lifecycle or application.

The existing Site Fix list response includes the two dismissal fields. The web client uses the latest Site Change application already included with each Site Fix to determine whether a pull request exists.

Doctor also exposes `GET /projects/{projectID}/doctor/finding-links`, a complete current-finding projection that returns the latest Site Fix and latest application for every active broken/optimization finding. This projection is intentionally separate from the 250-row Site Fixes workspace list: historical workspace pagination must never make a handed-off finding reappear as active work.

## Frontend Derivation

The Doctor client consumes the complete current-finding projection and creates one latest Site Fix per finding, ordered by creation time with a stable ID tiebreaker.

Primary findings are actionable report findings whose IDs have no linked Site Fix at all. This rule uses every loaded Site Fix, including dismissed links and fixes with pull requests, so presentation changes cannot recreate duplicate work.

Recent Findings contains the latest linked Site Fix for each current report finding when:

- `doctor_link_dismissed_at` is empty; and
- its latest application has no pull-request number, URL, or creation timestamp.

The Recent Findings button remains visible for discoverability and displays the current count. With no recent links it is disabled. Opening it closes the finding-detail drawer, and opening a finding detail closes Recent Findings, so only one right drawer can be active.

## Interaction and Accessibility

Each compact drawer row has a primary link region and a separate `Dismiss` button; interactive controls are not nested. The link includes the finding title, target URL, category/severity context, and Site Fix status. Keyboard focus returns to the button that opened a drawer or confirmation dialog.

Dismiss opens an explicit confirmation dialog:

- Title: `Remove this link from Doctor?`
- Body: `This only removes the link from Recent Findings. The Site Fix and any pull request are unchanged.`
- Actions: `Cancel` and `Dismiss link`

After success, the drawer count and list update immediately and a confirmation toast repeats that the Site Fix is unchanged. A failed request leaves the link visible and reports the error without closing the user's context.

## Error Handling

- A Site Fix missing from the selected project returns not found and creates no dismissal record.
- A Site Fix without `doctor_finding_id` cannot be dismissed through the Doctor endpoint.
- Concurrent dismissal calls are idempotent and preserve the first dismissal actor and timestamp.
- A missing or stale report finding never renders a broken Recent Findings row; the canonical Site Fix remains accessible from Site Fixes.
- Failed list or dismiss requests reuse the existing Doctor error and toast patterns and never make an optimistic lifecycle change.

## Testing

- Migration and SQL contract tests prove dismissal consistency, project scope, Doctor provenance, and lifecycle preservation.
- Go handler and service tests prove canonical routing, authorization, idempotency, response shape, and rejection of non-Doctor Site Fixes.
- Web API tests prove normalization and the dismiss-link request contract.
- Doctor UI contract tests prove handed-off findings leave the primary grid, PR-backed links disappear, terminal fixes without PRs remain, per-Site-Fix dismissal is honored, and the confirmation copy protects the Site Fix.
- Full Go tests, vet, web tests, and production builds run before publishing.

## Production Acceptance

After the PR is merged and both Railway and Vercel deployments contain the merge SHA, the production Doctor page must:

1. Omit findings that already have Site Fixes from the main grid.
2. Show those no-PR handoffs in Recent Findings with working deep links to Site Fixes.
3. Exclude any handoff whose application has created a pull request.
4. Show the approved confirmation dialog and leave the underlying Site Fix untouched when confirmation is cancelled.
5. Load without browser console errors or failed Doctor API requests.
