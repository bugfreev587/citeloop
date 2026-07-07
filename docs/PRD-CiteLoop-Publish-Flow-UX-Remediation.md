# PRD: CiteLoop Publish Flow UX Remediation

## 1. Summary

The Publish page currently mixes four concepts in ways that make the flow hard to trust:

- Content action: publishing a specific canonical article.
- Destination health: whether GitHub/Next.js can publish.
- Publish cadence: whether approved content waits for manual action or follows a schedule.
- Operational state: queued, publishing, verifying, published, or failed.

This PRD redesigns the Publish page around a content-first workflow. In Manual mode, the only primary publish action is on the specific content card. Destination drawers show destination status and recovery actions only. Publish cadence is controlled from the top Schedule control. Published content appears directly below Ready to Post in a collapsible Published section with a live URL that users can open in one click.

## 2. Problem Statement

The current experience creates ambiguity:

1. The GitHub/Next.js destination drawer includes `Publish next`, while each content card also includes `Publish`.
2. `Publish next` does not show which article it will publish, so users cannot predict the effect.
3. `When` can show `Publishes Jul 7, 9:22 AM` after a publish action, even when the article is only queued and not yet live.
4. The Schedule drawer offers `Auto`, `Scheduled`, and `Manual`, but the desired product model only needs `Manual` and `Scheduled`.
5. The content card has a `Timing` button even though cadence should be managed globally from the top Schedule control.
6. Settings can show `enabled` and `error` at the same time, which reads as contradictory.
7. Published articles are not close enough to the ready list, so users do not get a clear visual completion loop.
8. Publish failure copy can expose backend phrasing such as `publisher credential unavailable`, which does not tell users how to recover.

## 3. Goals

1. Make the primary publish action unambiguous in Manual mode.
2. Remove duplicate publish entry points from destination drawers.
3. Remove `Auto` publish cadence from the user-facing UI.
4. Keep cadence choice in one top-level Schedule control with only `Manual` and `Scheduled`.
5. Remove the per-card `Timing` button.
6. Show published content directly under Ready to Post in a collapsible Published section.
7. Show the live published URL on each published content row and provide a one-click open action.
8. Treat `enabled + error` as one blocking user state: `Needs attention`.
9. Disable publish actions when the canonical destination is unhealthy.
10. Replace backend error text with clear recovery-oriented UX copy.

## 4. Non-Goals

1. Do not add automatic publishing to Dev.to, Hashnode, Reddit, Medium, LinkedIn, Hacker News, WordPress, Webflow, Shopify, or custom CMS.
2. Do not move GitHub credential setup into Publish. Settings remains the configuration surface.
3. Do not redesign Review, Content Plan, or Results in this PRD.
4. Do not add a new multi-step publish wizard.
5. Do not require users to choose GitHub as a destination for every article. GitHub/Next.js is the only canonical blog destination in this scope.

## 5. Product Decisions

### 5.1 Content-First Publish

The Publish page must prioritize articles ready to publish. A user should read the first viewport as:

1. This content is ready.
2. It will publish to GitHub/Next.js.
3. The connection is ready or blocked.
4. I can publish this exact article.
5. After success, I can open the live URL.

### 5.2 Manual And Scheduled Only

The user-facing publish cadence options are:

| Mode | Meaning | Publish action |
| --- | --- | --- |
| `Manual` | Approved canonical articles wait for a user to click `Publish`. | Per-content-card `Publish` button. |
| `Scheduled` | Approved canonical articles are assigned future slots and publish according to the configured interval. | Top Schedule control and scheduled operational state. |

`Auto` is removed from the UI. The frontend must not render or submit `publish_mode: "auto"` from the Publish schedule control.

Legacy backend data with `publish_mode: "auto"` must not break the UI. Implementation should normalize legacy `auto` to a safe visible state. The recommended visible fallback is `Manual` unless product explicitly chooses a migration path.

### 5.3 GitHub/Next.js Is The Canonical Blog Destination

For canonical blog publishing in this release, GitHub/Next.js is the only active destination. The destination area answers health and configuration questions, not per-article publish confirmation.

Manual syndication destinations remain downstream surfaces that unlock after canonical publication and URL verification.

## 6. Page Information Architecture

### 6.1 Header Actions

Top header actions should include:

- Primary state-driven CTA only when it is not a duplicate publish action.
- `Schedule` control showing the current mode, for example `Schedule: Manual`.
- `View all`.
- `Check status`.
- `Refresh`.

The Schedule control is the only place where users change publishing cadence.

Header CTA rules:

| State | Header CTA |
| --- | --- |
| No GitHub/Next.js connection | `Connect GitHub` |
| GitHub/Next.js disabled | `Enable publishing` |
| GitHub/Next.js error or revoked | `Fix connection` |
| GitHub/Next.js connected and healthy | No `Publish next` header CTA in Manual mode |
| Only scheduled content exists | `View schedule` may appear as a secondary operational entry |

Manual mode must not render a global `Publish next` CTA. Publishing must happen from the content card so users know exactly which article they are shipping.

### 6.2 Main Content Order

Desktop and mobile order:

1. Connection warning or success summary.
2. `Ready to Post`.
3. Collapsible `Published`.
4. `Publish destinations`.
5. Manual syndication chips and drawers.
6. Operational `View all` drawer for less common groups.

The Published section must sit immediately under Ready to Post. It can be collapsed by default when there are many published items, but a newly published article should be visible or highlighted after a successful publish/verification cycle.

### 6.3 Ready To Post

Each ready canonical card shows:

- Article title.
- Content type: `Canonical content`.
- Destination: `GitHub/Next.js`.
- Timing label:
  - Manual mode: `Manual: publish when ready`.
  - Scheduled mode with future slot: `Scheduled for Jul 7, 9:22 AM`.
  - In-flight: `Queued`, `Publishing`, or `Verifying live URL`.
  - Failed: `Failed` with a short reason.
- Actions:
  - `Preview`.
  - `SEO Details`.
  - `Destination`.
  - `Publish` or `Retry`.

The per-card `Timing` button is removed. Users adjust cadence from the top Schedule control only.

### 6.4 Published Section

The Published section appears directly below Ready to Post.

Requirements:

- Section title: `Published`.
- Section is collapsible.
- Count badge shows published canonical article count.
- Each row/card shows:
  - Article title.
  - Published timestamp.
  - Destination: `GitHub/Next.js`.
  - Live URL as readable text.
  - One-click action: `Open live article`.
  - Optional action: `View Results`.
- If a published article has no `canonical_url`, do not silently show it as complete. Show `Published URL missing` with a recovery action to `Check status`.
- Newly published content should move out of Ready to Post after the backend reports `status: "published"` and a live URL is available.
- Content in `pending_url_verification` must not appear as fully published. It should remain in an in-flight state until verification succeeds or fails.

### 6.5 Publish Destinations

Destination tiles and drawers are status and recovery surfaces.

GitHub/Next.js drawer must not include `Publish next`.

Allowed drawer actions:

- `Retry test`.
- `Manage in Settings`.
- `Fix connection` when blocked.
- `View schedule` only when scheduled items exist.

Destination drawer content should explain:

- Current destination health.
- Repository, branch, content path, and base URL.
- Last health check error when present.
- Whether publishing is currently blocked.

Destination drawer must not contain a primary publish action.

## 7. State Model

### 7.1 Connection State

Connection health and enabled state must be summarized into one user-facing state.

| Raw state | User-facing state | Publish eligible | UX treatment |
| --- | --- | --- | --- |
| No connection or `missing` | `Not connected` | No | Show `Connect GitHub`. |
| `connected` and `enabled: false` | `Disabled` | No | Show `Enable publishing`. |
| `connected` and `enabled: true` | `Ready` | Yes | Publish buttons enabled. |
| `error` and any enabled value | `Needs attention` | No | Show error copy and `Fix connection`. |
| `revoked` and any enabled value | `Needs attention` | No | Show reconnect copy and `Fix connection`. |

Do not display `enabled` and `error` as equal-weight badges. If both are true in raw data, the main badge is `Needs attention`; `enabled` can appear only as secondary explanatory text.

Recommended copy:

- Main state: `Needs attention`.
- Detail: `Publishing is enabled, but the GitHub/Next.js connection is failing. Fix the connection before publishing.`

### 7.2 Publish Item State

Canonical articles should use an explicit publish state ladder:

| State | UI placement | User copy |
| --- | --- | --- |
| Approved and manual | Ready to Post | `Manual: publish when ready` |
| Approved and scheduled future | View all Scheduled, optionally Ready to Post if due soon | `Scheduled for {time}` |
| Publish request accepted | Ready to Post in-flight state | `Queued` |
| Backend publishing | Ready to Post in-flight state | `Publishing` |
| URL verification pending | Ready to Post in-flight state | `Verifying live URL` |
| Published with URL | Published section | `Published {time}` and live URL |
| Publish failed | Ready to Post retry state | Short reason plus `Retry` |

Do not use `Publishes {time}` for queued or already-clicked manual publish work. That phrasing is reserved for future scheduled content.

## 8. Interaction Requirements

### 8.1 Manual Publish

In Manual mode:

1. User clicks `Publish` on a specific Ready to Post card.
2. Button enters busy state: `Queuing`.
3. Card status changes to `Queued` or `Publishing`.
4. Duplicate publish actions are disabled for that article.
5. Page refreshes or polls until status changes.
6. On success with URL, article moves to Published.
7. Published row shows live URL and `Open live article`.
8. On failure, article remains in Ready to Post with `Retry` and recovery copy.

### 8.2 Scheduled Publish

In Scheduled mode:

1. User opens top Schedule control.
2. User chooses `Scheduled`.
3. User configures interval if required.
4. Future scheduled articles show `Scheduled for {time}`.
5. Per-card Timing button is not present.
6. Schedule changes apply globally to publish cadence.

### 8.3 Destination Recovery

If connection health is blocked:

1. Ready to Post cards remain visible.
2. `Publish` and `Retry` are disabled.
3. Inline reason appears near disabled action: `Fix GitHub/Next.js before publishing.`
4. Destination tile shows `Needs attention`.
5. Drawer offers `Retry test` and `Manage in Settings`, not `Publish next`.

## 9. Copy Requirements

Use user-facing recovery copy. Do not expose raw backend phrasing.

| Raw or current copy | Replacement |
| --- | --- |
| `publisher credential unavailable` | `CiteLoop cannot access this GitHub repository for publishing. Reconnect GitHub or grant repository access, then retry.` |
| `Auto publish` destination badge | `Ready` or `Connected` |
| `Publishes Jul 7, 9:22 AM` after manual publish click | `Queued`, `Publishing`, or `Verifying live URL` |
| `Publish next` in destination drawer | Removed |
| `Timing` button on content card | Removed |

## 10. Data And Backend Requirements

The PRD should be implementable with the existing article and publisher connection model, but the UI needs consistent fields.

Article fields needed:

- `status`.
- `scheduled_at`.
- `published_at`.
- `canonical_url`.
- `publish_path`.
- `last_publish_error`.
- `next_publish_retry_at`.
- `publish_attempts`.

Publisher connection fields needed:

- `kind`.
- `status`.
- `enabled`.
- `last_error`.
- `config.repo`.
- `config.branch`.
- `config.content_dir`.
- `config.base_url`.
- `is_default`.

Backend behavior:

1. Publish endpoint must not report success as published until the live URL is known or verification has completed.
2. `pending_url_verification` remains an in-flight state.
3. Connection `error` or `revoked` must make the connection ineligible even when `enabled` is true.
4. Legacy `publish_mode: "auto"` should be normalized for UI safety. Recommended: render as Manual and save as Manual on the next mode update.
5. API errors should preserve machine detail for logs but provide frontend enough context to show friendly copy.

## 11. Frontend Implementation Shape

Update the publish logic module so the component can render from pure state models.

Suggested model changes:

- Replace `PublishConnectionState` value `auto_publish` with `ready` or `connected`.
- Remove `PublishHeaderCta` variant `{ label: "Publish next"; kind: "publish_next" }` for Manual mode.
- Remove `timingActionLabel` from `ReadyNowItem`.
- Add `publishStateLabel` to `ReadyNowItem`.
- Add `disabledReason` copy specific to unhealthy GitHub/Next.js.
- Add `buildPublishedCanonicals(input)` or extend operational groups to expose a first-class Published section model.
- Add a `publishedUrl` field derived from `canonical_url` first, then `publish_path` only if it is a usable URL.

Component changes:

- Remove the `Timing` button from Ready to Post cards.
- Remove `Publish next` from the GitHub/Next.js drawer.
- Keep Schedule as the only cadence control.
- Render only `Manual` and `Scheduled` mode options.
- Render Published directly below Ready to Post with collapse behavior.
- Display live URLs and open actions in Published rows.
- Disable publish buttons when GitHub/Next.js is not user-facing `Ready`.

## 12. Testing Requirements

Add or update frontend contract tests for pure logic.

Required assertions:

1. GitHub `connected + enabled` maps to `Ready`, not `Auto publish`.
2. GitHub `error + enabled` maps to `Needs attention` and publish actions are disabled.
3. GitHub `revoked + enabled` maps to `Needs attention` and publish actions are disabled.
4. Header CTA in Manual mode does not return `Publish next`.
5. GitHub destination drawer model does not expose a publish action.
6. Ready to Post items do not include a Timing action.
7. Schedule mode options are exactly `manual` and `scheduled`.
8. Legacy `auto` mode is normalized to a safe visible mode.
9. Published canonical articles appear in the first-class Published section.
10. Published rows include a live URL and `Open live article` action when `canonical_url` is present.
11. Published rows with missing URL show a warning and do not claim a complete live article.
12. Manual publish in-flight state shows `Queued`, `Publishing`, or `Verifying live URL`, not `Publishes {time}`.
13. Raw error `publisher credential unavailable` maps to friendly recovery copy.

Backend or integration tests:

1. Publish endpoint leaves article in pending verification until URL verification completes.
2. Connection eligibility requires both `enabled: true` and `status: "connected"`.
3. Error and revoked connections are never eligible, regardless of enabled value.

## 13. Accessibility Requirements

1. Published section collapse control must be keyboard accessible and expose expanded/collapsed state.
2. Live URL action must have accessible text such as `Open live article`.
3. Disabled publish actions must expose the disabled reason through visible helper text, not title-only tooltip.
4. Status badges must not rely on color alone.
5. Focus should move or announce success when a newly published item appears in Published.

## 14. Acceptance Criteria

1. In Manual mode, users see one primary publish button per ready article and no global `Publish next`.
2. Users cannot trigger publish from the destination drawer.
3. Users choose only `Manual` or `Scheduled` in the top Schedule control.
4. Ready to Post cards no longer show a `Timing` button.
5. A successful publish moves the article to the Published section after the backend reports a published URL.
6. Published content is directly under Ready to Post and can be collapsed.
7. Published rows show a live URL and one-click open action.
8. An unhealthy GitHub/Next.js connection shows `Needs attention`, even if raw `enabled` is true.
9. Publish actions are disabled when the GitHub/Next.js connection is not healthy.
10. Users see recovery-oriented copy for credential or repository access failures.
11. The page no longer uses `Auto publish` to describe a destination capability.
12. Queued or verifying content is never labeled as already published.

## 15. Rollout Notes

This should ship as a focused Publish page UX remediation before any broader publish-platform expansion. The old platform-first PRD remains useful for destination taxonomy, but this PRD supersedes it for Manual mode action placement, cadence options, destination drawer actions, and Published section placement.

Recommended implementation order:

1. Update pure publish logic and contract tests.
2. Remove `Auto` from Schedule UI and normalize legacy mode.
3. Remove `Timing` and destination drawer `Publish next`.
4. Add Published section under Ready to Post.
5. Add friendly connection error copy and disabled publish reasons.
6. Verify the full manual publish path from ready article to live URL.
