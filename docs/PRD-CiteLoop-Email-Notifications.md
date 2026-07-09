# PRD: CiteLoop Email Notifications

> Date: 2026-07-09
> Status: Draft for PRD review
> Scope: Account-level notification channels, project event subscriptions, Resend email delivery
> Entry point: `/projects/:id/settings#notifications`
> Code baseline: `origin/main@eb84f81`

## 1. Summary

CiteLoop should support Email as a first-class notification channel alongside Slack and Discord. Notification channels must be account-level, not project-level: one account can own multiple projects, and one Email channel should be reusable across any project in that account.

The existing Settings entry point remains project-scoped because users naturally configure notifications while working in a project. The product model changes underneath it:

- Channels belong to the account that owns the project.
- Event subscriptions remain project-specific, because each project decides which events send to which account channel.
- Deliveries remain project-specific, because every notification event originates from a project.

V1 uses Resend for outbound email. Account users add recipient email addresses in Settings, send a test email, then choose which project events should notify that recipient. A successful Resend test means the send request was accepted by Resend; it does not prove that the message reached the recipient inbox until delivery webhooks are added.

## 2. Problem

The current notification system supports only webhook destinations and stores channels under `notification_channels.project_id`. That creates two product problems:

1. Email cannot be configured without adding a new channel kind and sender.
2. A user with several projects would have to recreate the same channel in every project, even though notification destinations are an account preference.

This is especially awkward for Email. A user expects `ops@example.com` or their personal inbox to be reusable across all projects in the account, while still letting each project choose its own event subscriptions.

## 3. Goals

- Add Email as a notification channel type in Settings.
- Make notification channels account-level for all channel kinds, including existing Slack and Discord.
- Keep subscriptions project-specific so a channel can receive different events from different projects.
- Keep deliveries project-specific and visible from the project that generated the event.
- Send Email through Resend using server-side credentials only.
- Let users test an Email channel before it can receive project event subscriptions.
- Preserve existing delivery queue, retry, `dead` status, and notification-delivery-dead behavior.
- Avoid exposing raw recipient email addresses or Resend credentials to the browser.

## 4. Non-Goals

- Do not add marketing broadcasts, newsletters, audiences, or campaign management.
- Do not let users bring their own Resend API key in V1.
- Do not add per-user notification preference centers in V1.
- Do not add Resend inbound processing or reply handling.
- Do not require a new onboarding wizard.
- Do not auto-subscribe every account channel to every project event.
- Do not change which notification events exist, except for copy updates that clarify Email behavior.
- Do not claim human inbox receipt or final delivery state from a Resend 2xx response alone.

## 5. Product Decisions

### 5.1 Account Scope

The account scope for V1 is the current project owner identity:

```text
account_id := projects.owner_id
```

`projects.owner_id` is the existing Clerk user identifier stored as text. It is a denormalized account scope key, not a foreign key to an `accounts` table. CiteLoop does not need a separate `accounts` table before shipping V1. If a future accounts table is introduced, notification channels can migrate from `owner_id` to that stable account primary key.

### 5.2 Settings Entry Point

The Notifications tab remains reachable at:

```text
/projects/:id/settings#notifications
```

The tab must clearly show two scopes:

- Account channels: reusable destinations available to all projects in the account.
- Project subscriptions: event choices for the current project only.

The user should not need to leave project Settings to add a channel or choose event subscriptions for that project.

### 5.3 V1 Email Recipient Model

V1 uses manual recipient entry:

- A user enters an Email channel label and recipient address.
- The channel sends a test email through Resend.
- After Resend accepts the test send, the channel can be subscribed to project events.

This matches option A from product discussion. Owner-default email and user-level preferences remain future work.

### 5.4 Resend Ownership

Resend is a CiteLoop platform integration, not an account-owned credential in V1.

Server environment variables:

| Variable | Required | Purpose |
| --- | --- | --- |
| `RESEND_API_KEY` | Yes for Email sends | Server-side Resend API key. |
| `NOTIFICATION_EMAIL_FROM` | Yes for Email sends | Verified sender, for example `CiteLoop <notifications@citeloop.app>`. |
| `NOTIFICATION_EMAIL_REPLY_TO` | No | Optional reply-to address for support. |
| `NOTIFICATION_SECRET_KEY` | Yes | Existing secret used to encrypt channel destination config. |

The implementation cannot be production-verified until Resend is connected, the sender domain is verified, and the above variables are set in the production runtime. Production verification must separate "Resend accepted the send request" from "a human confirmed receipt in the inbox."

## 6. User Experience

### 6.1 Notifications Tab Layout

The Notifications tab should keep the current compact operational style:

1. Account channels
2. Project event subscriptions
3. Project deliveries

The current single table can remain, but labels must avoid implying that channels belong only to the current project.

Recommended copy:

```text
Account channels
Reusable destinations for projects in this account.

Project subscriptions
Choose which events from this project send to each channel.

Deliveries
Recent notification attempts from this project.
```

### 6.2 Channel Creation

The channel kind selector adds `Email` next to Slack and Discord.

When Email is selected:

- Destination input label: `Email address`
- Placeholder: `ops@example.com`
- Input type: `email`
- Default label: `Email`

When Slack or Discord is selected:

- Destination input label: `Webhook URL`
- Placeholder stays webhook-specific if possible.
- Input type remains password.

The create action remains `Add`.

### 6.3 Channel Table

The table should rename `Webhook` to `Destination`.

For Email channels:

- Kind badge: `Email`
- Destination cell: redacted email, for example `o***@example.com`
- Status: `Test accepted` after Resend accepts a test email request; otherwise `Untested`
- Test action title: `Send test email`

For Slack and Discord:

- Existing redacted webhook display remains.
- Test action title stays webhook-oriented.

For all channel kinds:

- The row should show how many projects in the account currently use the channel, for example `Used by 3 projects`.
- The row copy must make account scope visible so users understand that deleting a channel from one project's Settings affects every project subscription that uses it.

### 6.4 Event Subscription Modal

The modal title remains channel-centered:

```text
Events for Ops Email
```

The description must be project-scoped:

```text
Choose which events from this project send to this account channel.
```

This avoids implying that selecting an event in one project automatically subscribes all projects.

### 6.5 Empty States

If no channels exist:

```text
No account channels
Add Slack, Discord, or Email once, then reuse it across this account's projects.
```

If the project has no subscriptions:

```text
No project subscriptions
Choose Events on an account channel to send this project's notifications.
```

### 6.6 Channel Edit And Delete

V1 should avoid a hidden cross-project footgun:

- Delete must show a confirmation that includes the channel's account scope and project usage count.
- If a channel is used by more than one project, the confirmation must explicitly say that deleting it stops future notifications for all subscribed projects.
- Editing a destination in place is allowed only if the implementation resets the test status and preserves the same `channel_id`.
- If in-place destination edit is not implemented in V1, the UI should support label-only edit and require delete plus recreate for destination changes.
- Any destination change for Email must clear the accepted/tested state until a new test send succeeds.

## 7. Notification Events

Email should support the same subscription and delivery path as existing channels. The PRD must distinguish between events that are supported in the UI/API and events that are actually emitted by the current backend.

Events emitted in the current code baseline:

- `generation.failed`
- `publish.failed`
- `budget.stopped`
- `review.overdue`
- `sitefix.pr.awaiting_merge`
- `webhook.delivery.dead`

Events currently supported by UI/API selection but not emitted by a backend producer in this baseline:

- `seo.sync.failed`
- `seo.auth.expired`
- `seo.opportunity.ready`
- `seo.brief.ready`
- `seo.action.measurement_ready`
- `seo.indexing.anomaly`

If the implementation baseline adds producers for additional notification events, Email must support them automatically through the same subscription and delivery queue.

`webhook.delivery.dead` should keep its event type unchanged for compatibility. User-facing copy may rename it to `Notification delivery dead` or `Notification failed permanently` because Email failures are not webhooks. This is a display-only copy change.

The existing dead-delivery follow-up event already has a one-hop guard: a failed `webhook.delivery.dead` delivery must not enqueue another `webhook.delivery.dead` event for itself. V1 should preserve that behavior for Email.

## 8. Data Model

### 8.1 Channel Scope Migration

Current baseline:

```text
notification_channels.project_id -> projects.id
notification_subscriptions.project_id -> projects.id
notification_deliveries.project_id -> projects.id
notification_subscriptions.channel_id -> notification_channels.id
notification_deliveries.channel_id -> notification_channels.id
```

Target V1:

```text
notification_channels.owner_id text not null
notification_subscriptions.project_id -> projects.id
notification_subscriptions.channel_id -> notification_channels.id
notification_deliveries.project_id -> projects.id
notification_deliveries.channel_id -> notification_channels.id
```

`notification_channels.owner_id` stores the same text value as `projects.owner_id`. It is not a foreign key in the current schema because there is no account table. The migration should backfill `owner_id` from existing channels' projects, then make it `not null`.

`notification_channels.project_id` should no longer be required for new channels after the account-scope migration. The implementation must keep enough compatibility to avoid losing existing webhook channels during rollout.

Required invariant:

```text
subscription.project_id belongs to the same owner_id as subscription.channel_id
```

A channel from account A must never be subscribable from account B's project. This invariant must be enforced in the database, preferably with a trigger/function on `notification_subscriptions` insert and update that checks:

```text
projects.owner_id for new.project_id == notification_channels.owner_id for new.channel_id
```

Application queries and API handlers must also apply owner-scoped filters, and tests must prove both layers reject cross-owner subscription attempts.

### 8.2 Email Channel Config

Email channel config must not expose the raw recipient address in API responses.

Recommended stored config shape:

```json
{
  "encrypted_to": "...",
  "redacted_to": "o***@example.com",
  "address_hash": "hmac-sha256..."
}
```

`encrypted_to` uses `NOTIFICATION_SECRET_KEY`. `address_hash` enables duplicate detection within an account without storing plaintext. The hash should be HMAC-SHA256 over a normalized, lowercased email address and the owner scope, for example:

```text
HMAC-SHA256(NOTIFICATION_SECRET_KEY, owner_id + "\n" + lower(trim(email)))
```

This keeps duplicate detection account-local and prevents the same address from producing a reusable global fingerprint across accounts.

API response config:

```json
{
  "redacted_to": "o***@example.com"
}
```

Webhook response config remains:

```json
{
  "redacted_url": "https://hooks.slack.com/services/T000/B000/****"
}
```

### 8.3 Delivery Payload

No new delivery payload shape is required for V1. Email rendering may use the existing notification payload fields:

- `message`
- `title`
- `error`
- `dashboard_url`
- project identifiers or object identifiers when present

The Email sender should produce a readable subject and HTML/text body from those fields.

## 9. API Requirements

The existing project-scoped endpoints may remain:

```text
GET    /api/projects/:projectID/notifications/channels
POST   /api/projects/:projectID/notifications/channels
PATCH  /api/projects/:projectID/notifications/channels/:channelID
DELETE /api/projects/:projectID/notifications/channels/:channelID
POST   /api/projects/:projectID/notifications/channels/:channelID/test
GET    /api/projects/:projectID/notifications/subscriptions
PUT    /api/projects/:projectID/notifications/subscriptions
GET    /api/projects/:projectID/notifications/deliveries
POST   /api/projects/:projectID/notifications/deliveries/:deliveryID/retry
```

Their semantics change as follows:

- Channel list returns account-level channels for the owner of `projectID`.
- Channel list returns usage metadata, including how many projects in the account currently subscribe to each channel.
- Channel create creates an account-level channel for the owner of `projectID`.
- Channel update can change the label. If it changes the destination, it must preserve the same `channel_id`, reset the accepted/tested state, and block subscriptions until a new test succeeds. If destination update is deferred, the endpoint must reject destination changes clearly.
- Channel delete soft-deletes the account channel and stops future deliveries from any project subscriptions using it. The API response or preflight metadata must expose the affected project subscription count so the UI can show a precise confirmation.
- Channel test sends through the correct destination type. For Email, Resend 2xx marks the test as accepted, stores the provider message ID when available, and updates the existing `verified_at` field or its replacement timestamp. The UI label should be `Test accepted`, not `Verified`.
- Subscription list remains scoped to `projectID`.
- Subscription upsert must reject channels whose `owner_id` does not match the project owner.
- Subscription upsert must reject Email channels that have not passed a test send. Slack and Discord can preserve existing behavior in V1, but automation readiness copy should only count channels that have a successful test timestamp.
- Deliveries remain scoped to `projectID`.

Future account settings can add account-native endpoints, but V1 does not need them.

## 10. Sending Architecture

### 10.1 Sender Interface

The current worker assumes every destination is a webhook URL. V1 should refactor the notification sender boundary so the worker can dispatch by channel kind and config.

Conceptual interface:

```go
type DeliveryTarget struct {
  Kind   string
  Config json.RawMessage
}

type Sender interface {
  Send(ctx context.Context, target DeliveryTarget, payload json.RawMessage) error
}
```

The concrete sender can route:

- `slack_webhook` -> existing webhook body and HTTP POST
- `discord_webhook` -> existing webhook body and HTTP POST
- `email` -> Resend email send

### 10.2 Email Rendering

V1 can use server-side Go rendering rather than React Email because the active notification worker is Go.

Minimum email fields:

| Field | Requirement |
| --- | --- |
| `from` | `NOTIFICATION_EMAIL_FROM` |
| `to` | Decrypted Email channel recipient |
| `reply_to` | `NOTIFICATION_EMAIL_REPLY_TO` when set |
| `subject` | `CiteLoop: <event label>` or payload title |
| `text` | Plain text message and dashboard URL when available |
| `html` | Simple readable HTML body with the same information |

React Email can be reconsidered if email templates become complex or if a Next.js email-rendering path is introduced later.

### 10.3 Idempotency

The existing delivery table already deduplicates `(event_id, channel_id)`. The Resend send call should additionally pass an idempotency key derived from the stable notification delivery ID so worker retries do not create duplicate outbound emails when Resend receives a request but the worker times out.

Manual retries after Resend's idempotency retention window may create a second outbound email. That is acceptable in V1 if the retry action remains explicit and the delivery history shows each attempt outcome.

## 11. Error Handling

- Missing `NOTIFICATION_SECRET_KEY`: channel creation and worker processing fail with existing secret guidance.
- Missing `RESEND_API_KEY`: Email test sends and Email deliveries fail with a clear `RESEND_API_KEY is required` error.
- Missing `NOTIFICATION_EMAIL_FROM`: Email test sends and Email deliveries fail with a clear sender configuration error.
- Invalid Email address: channel creation returns 400 and does not store a channel.
- Resend non-2xx/API error: delivery remains pending until retry policy marks it `dead`.
- Resend synchronous suppression, bounce, or validation rejection: treat as send failure, store the provider error, and do not mark the Email test as accepted.
- Resend 2xx/API success: treat as provider acceptance only. Store the provider message ID when returned.
- Final delivered, bounced, complained, and suppressed states from asynchronous Resend webhooks are out of scope for V1 and should be tracked as a follow-up.

### 11.1 Volume And Rate Limits

The current notification worker processes a bounded batch of pending deliveries per tick, with a default limit of 20. V1 can rely on that existing queue control.

Per-recipient digesting, event coalescing, and user-configurable frequency limits are out of scope for V1. High-volume projects may send one Email per subscribed event, subject to worker retries and Resend rate-limit errors.

## 12. Migration And Rollout

### 12.1 Phase 1: Account-Level Foundation

- Add account ownership to notification channels using `projects.owner_id`.
- Backfill existing Slack and Discord channels.
- Update channel queries to list and create by owner.
- Preserve project-scoped subscriptions and deliveries.
- Add database-level cross-owner subscription protection plus API/query tests.

### 12.2 Phase 2: Email Channel

- Add `email` channel kind.
- Alter the `notification_channels.kind` CHECK constraint to allow `email`.
- Update channel kind validation and default error copy so `email` is treated as supported, including the current `validateWebhookURL` path or its replacement.
- Add email config preparation, encryption, redaction, validation, and duplicate detection.
- Add Resend-backed sender.
- Update Settings UI for Email channel creation and destination display.
- Update channel list/delete UI to show account scope and project usage counts.
- Add destination update or explicitly reject destination changes while supporting label edit.
- Add tests for create, list, test, worker send, retry, and response redaction.

### 12.3 Phase 3: Production Verification

- Connect Resend.
- Verify the sender domain.
- Configure production environment variables.
- Create an Email channel in production Settings.
- Send a test email and confirm Resend acceptance plus human inbox receipt.
- Subscribe the Email channel to a low-risk event that is emitted by the current backend, such as `sitefix.pr.awaiting_merge` or `generation.failed`.
- Trigger or enqueue that emitted event and verify delivery history reaches `sent`.

## 13. Acceptance Criteria

1. A user can open `/projects/:id/settings#notifications` and add an Email channel with a recipient address.
2. The Email channel appears as an account channel in every project owned by the same account.
3. The Email channel does not appear in projects owned by another account.
4. The browser never receives the raw recipient address after channel creation.
5. A successful test email marks the channel as `Test accepted`, stores the Resend message ID when available, and does not claim inbox delivery.
6. A failed test email leaves the channel unaccepted and shows a clear error.
7. A user can subscribe an Email channel to events for project A without subscribing it to project B.
8. A project event enqueues one delivery per enabled subscription, including Email subscriptions.
9. The notification worker sends Email deliveries through Resend.
10. Delivery retry and `dead` behavior works for Email the same way it works for webhooks.
11. Delivery history on a project shows that project's Email notification attempts.
12. Existing Slack and Discord channels continue to work after the account-scope migration.
13. Existing Slack and Discord subscriptions are preserved during migration.
14. Cross-owner subscription is rejected by both API behavior and database-level protection.
15. Channel list shows account-scope usage, and delete confirmation states how many project subscriptions will be affected.
16. Changing an Email destination resets the accepted/tested state, or destination changes are explicitly rejected in V1.
17. Production verification confirms Resend accepts an Email for an event the current backend actually emits, delivery history reaches `sent`, and a human manually confirms inbox receipt.

## 14. Test Plan

### 14.1 Backend

- Migration contract test: `notification_channels` supports account ownership and `email`.
- Migration contract test: `notification_channels.kind` accepts `email` and rejects unsupported kinds.
- Migration contract test: cross-owner inserts or updates into `notification_subscriptions` fail at the database layer.
- Query contract test: channel list is owner-scoped, subscription list remains project-scoped.
- Authorization test: project B cannot subscribe to project A owner's channel.
- Config test: Email addresses are encrypted, redacted, hashed, and validated.
- Config test: `address_hash` is stable within an owner scope but differs for the same normalized email under a different owner.
- API test: Email channel response omits plaintext recipient.
- API test: Email test requires Resend sender config and marks the channel test accepted only after Resend acceptance.
- API test: subscription upsert rejects unaccepted Email channels.
- API test: channel list/delete metadata returns usage counts for cross-project delete confirmation.
- API test: destination update resets accepted/tested status, or the API rejects destination changes with a clear response.
- Worker test: Email pending delivery calls the Email sender and marks sent.
- Worker test: Email sender failure follows retry/dead behavior.
- Worker test: Resend synchronous suppression, bounce, or validation rejection follows failure behavior.
- Worker test: Resend idempotency key uses the stable delivery ID.
- Worker test: failed `webhook.delivery.dead` delivery does not recursively enqueue another dead-delivery event.
- Regression test: Slack and Discord sender behavior remains unchanged.

### 14.2 Frontend

- API client type includes `email`.
- Settings channel kind selector includes Email.
- Email selected state uses email input copy and type.
- Channel table uses `Destination`, not `Webhook`.
- Channel table shows account-scope usage counts.
- Delete confirmation explains cross-project impact when the channel is used by multiple projects.
- Email status label is `Test accepted` or equivalent, not `Verified`.
- Event subscription modal uses project-scoped wording.
- Event subscription modal blocks unaccepted Email channels or sends the user through the test flow first.
- Delivery table remains project-scoped.

### 14.3 Production

- Confirm Resend domain verification.
- Confirm `RESEND_API_KEY`, `NOTIFICATION_EMAIL_FROM`, and `NOTIFICATION_SECRET_KEY` are configured.
- Add a production Email channel.
- Send a test email, confirm Resend acceptance, and manually confirm inbox receipt.
- Subscribe to one low-risk event.
- Trigger or enqueue an event emitted by the current backend, not a `seo.*` event unless a producer has been added.
- Confirm the delivery row reaches `sent`.

## 15. Open Questions

1. What sender address should CiteLoop use in production: `notifications@citeloop.app`, `ops@citeloop.app`, or another verified address?
2. Should account-level channel management also appear in a future Account Settings page, or is project Settings sufficient for V1?
3. Should V1 allow multiple recipient addresses in one Email channel, or require one address per channel for clearer verification and retry behavior?
4. Should Resend delivery webhooks be added in a follow-up to track delivered, bounced, complained, and suppressed states?

## 16. Implementation Dependencies

- Resend account access.
- Verified sending domain.
- Production environment variable access.
- Existing `NOTIFICATION_SECRET_KEY` must be configured in every environment that creates or sends notification channels.
