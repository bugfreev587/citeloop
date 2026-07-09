# PRD: CiteLoop Email Notifications

> Date: 2026-07-09
> Status: Draft for PRD review
> Scope: Account-level notification channels, project event subscriptions, Resend email delivery
> Entry point: `/projects/:id/settings#notifications`
> Code baseline: `origin/main@10caddf`

## 1. Summary

CiteLoop should support Email as a first-class notification channel alongside Slack and Discord. Notification channels must be account-level, not project-level: one account can own multiple projects, and one Email channel should be reusable across any project in that account.

The existing Settings entry point remains project-scoped because users naturally configure notifications while working in a project. The product model changes underneath it:

- Channels belong to the account that owns the project.
- Event subscriptions remain project-specific, because each project decides which events send to which account channel.
- Deliveries remain project-specific, because every notification event originates from a project.

V1 uses Resend for outbound email. Account users add recipient email addresses in Settings, send a test email, then choose which project events should notify that recipient.

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
- Let users verify an Email channel by sending a test notification before it counts as verified.
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

## 5. Product Decisions

### 5.1 Account Scope

The account scope for V1 is the current project owner identity:

```text
account_id := projects.owner_id
```

CiteLoop does not need a separate `accounts` table before shipping V1. If a future accounts table is introduced, notification channels can migrate from `owner_id` to that stable account primary key.

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
- The recipient address is verified by a test send.
- After verification, the channel can be subscribed to project events.

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

The implementation cannot be production-verified until Resend is connected, the sender domain is verified, and the above variables are set in the production runtime.

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
- Status: `Verified` after a successful test email; otherwise `Untested`
- Test action title: `Send test email`

For Slack and Discord:

- Existing redacted webhook display remains.
- Test action title stays webhook-oriented.

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

## 7. Notification Events

V1 uses the existing event list:

- `generation.failed`
- `publish.failed`
- `budget.stopped`
- `review.overdue`
- `webhook.delivery.dead`
- `seo.sync.failed`
- `seo.auth.expired`
- `seo.opportunity.ready`
- `seo.brief.ready`
- `seo.action.measurement_ready`
- `seo.indexing.anomaly`
- `site_fix_pr.awaiting_merge`

If the implementation baseline has additional notification events, Email must support them automatically through the same subscription and delivery queue.

`webhook.delivery.dead` should be renamed in user-facing copy to `Notification delivery dead` or `Notification failed permanently` because Email failures are not webhooks.

## 8. Data Model

### 8.1 Channel Scope Migration

Current baseline:

```text
notification_channels.project_id -> projects.id
notification_subscriptions.project_id -> projects.id
notification_deliveries.project_id -> projects.id
```

Target V1:

```text
notification_channels.owner_id -> projects.owner_id
notification_subscriptions.project_id -> projects.id
notification_subscriptions.channel_id -> notification_channels.id
notification_deliveries.project_id -> projects.id
notification_deliveries.channel_id -> notification_channels.id
```

`notification_channels.project_id` should no longer be required for new channels. The migration should backfill `owner_id` from existing channels' projects and keep enough compatibility to avoid losing existing webhook channels.

Required invariant:

```text
subscription.project_id belongs to the same owner_id as subscription.channel_id
```

The database may enforce this with a trigger or the application may enforce it in every query path with contract tests. A channel from account A must never be subscribable from account B's project.

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

`encrypted_to` uses `NOTIFICATION_SECRET_KEY`. `address_hash` enables duplicate detection within an account without storing plaintext.

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
DELETE /api/projects/:projectID/notifications/channels/:channelID
POST   /api/projects/:projectID/notifications/channels/:channelID/test
GET    /api/projects/:projectID/notifications/subscriptions
PUT    /api/projects/:projectID/notifications/subscriptions
GET    /api/projects/:projectID/notifications/deliveries
POST   /api/projects/:projectID/notifications/deliveries/:deliveryID/retry
```

Their semantics change as follows:

- Channel list returns account-level channels for the owner of `projectID`.
- Channel create creates an account-level channel for the owner of `projectID`.
- Channel delete soft-deletes the account channel and stops future deliveries from any project subscriptions using it.
- Channel test sends through the correct destination type and marks the channel verified only after success.
- Subscription list remains scoped to `projectID`.
- Subscription upsert must reject channels whose `owner_id` does not match the project owner.
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

The existing delivery table already deduplicates `(event_id, channel_id)`. The Resend send call should additionally pass an idempotency key derived from the delivery ID or `event_id + channel_id` so worker retries do not create duplicate outbound emails when Resend receives a request but the worker times out.

## 11. Error Handling

- Missing `NOTIFICATION_SECRET_KEY`: channel creation and worker processing fail with existing secret guidance.
- Missing `RESEND_API_KEY`: Email test sends and Email deliveries fail with a clear `RESEND_API_KEY is required` error.
- Missing `NOTIFICATION_EMAIL_FROM`: Email test sends and Email deliveries fail with a clear sender configuration error.
- Invalid Email address: channel creation returns 400 and does not store a channel.
- Resend non-2xx/API error: delivery remains pending until retry policy marks it `dead`.
- Permanent suppression/bounce handling is out of scope unless Resend returns a synchronous suppression error during send.

## 12. Migration And Rollout

### 12.1 Phase 1: Account-Level Foundation

- Add account ownership to notification channels using `projects.owner_id`.
- Backfill existing Slack and Discord channels.
- Update channel queries to list and create by owner.
- Preserve project-scoped subscriptions and deliveries.
- Add cross-owner subscription protection.

### 12.2 Phase 2: Email Channel

- Add `email` channel kind.
- Add email config preparation, encryption, redaction, validation, and duplicate detection.
- Add Resend-backed sender.
- Update Settings UI for Email channel creation and destination display.
- Add tests for create, list, test, worker send, retry, and response redaction.

### 12.3 Phase 3: Production Verification

- Connect Resend.
- Verify the sender domain.
- Configure production environment variables.
- Create an Email channel in production Settings.
- Send and receive a test email.
- Subscribe the Email channel to a low-risk event.
- Trigger or enqueue that event and verify delivery history reaches `sent`.

## 13. Acceptance Criteria

1. A user can open `/projects/:id/settings#notifications` and add an Email channel with a recipient address.
2. The Email channel appears as an account channel in every project owned by the same account.
3. The Email channel does not appear in projects owned by another account.
4. The browser never receives the raw recipient address after channel creation.
5. A successful test email marks the channel verified.
6. A failed test email leaves the channel unverified and shows a clear error.
7. A user can subscribe an Email channel to events for project A without subscribing it to project B.
8. A project event enqueues one delivery per enabled subscription, including Email subscriptions.
9. The notification worker sends Email deliveries through Resend.
10. Delivery retry and `dead` behavior works for Email the same way it works for webhooks.
11. Delivery history on a project shows that project's Email notification attempts.
12. Existing Slack and Discord channels continue to work after the account-scope migration.
13. Existing Slack and Discord subscriptions are preserved during migration.
14. Production verification confirms a real Email is received through the configured Resend sender.

## 14. Test Plan

### 14.1 Backend

- Migration contract test: `notification_channels` supports account ownership and `email`.
- Query contract test: channel list is owner-scoped, subscription list remains project-scoped.
- Authorization test: project B cannot subscribe to project A owner's channel.
- Config test: Email addresses are encrypted, redacted, hashed, and validated.
- API test: Email channel response omits plaintext recipient.
- API test: Email test requires Resend sender config and marks verified only after successful send.
- Worker test: Email pending delivery calls the Email sender and marks sent.
- Worker test: Email sender failure follows retry/dead behavior.
- Regression test: Slack and Discord sender behavior remains unchanged.

### 14.2 Frontend

- API client type includes `email`.
- Settings channel kind selector includes Email.
- Email selected state uses email input copy and type.
- Channel table uses `Destination`, not `Webhook`.
- Event subscription modal uses project-scoped wording.
- Delivery table remains project-scoped.

### 14.3 Production

- Confirm Resend domain verification.
- Confirm `RESEND_API_KEY`, `NOTIFICATION_EMAIL_FROM`, and `NOTIFICATION_SECRET_KEY` are configured.
- Add a production Email channel.
- Receive a test email.
- Subscribe to one low-risk event.
- Trigger or enqueue the event.
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

