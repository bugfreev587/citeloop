# Email Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `docs/PRD-CiteLoop-Email-Notifications.md` end to end: account-level notification channels, Email channels through Resend, project-scoped subscriptions/deliveries, Settings UI support, and production verification.

**Architecture:** Keep the existing project-scoped Settings and API URLs, but move channel ownership to `projects.owner_id` under the hood. Extend the notification config/sender boundary so webhook channels keep their current path while Email channels decrypt recipient config and call Resend. Preserve project-scoped subscriptions/deliveries and enforce cross-owner safety in both SQL and API code.

**Tech Stack:** Go, pgx/sqlc, PostgreSQL migrations, Next.js/React/TypeScript, Node test runner, Resend HTTP API.

---

## File Map

- Modify `internal/migrations/0005_notifications.sql`: include `email`, add `owner_id`, loosen `project_id`, add owner indexes and cross-owner trigger.
- Modify `internal/db/queries/notifications.sql`: owner-scoped channel queries, usage counts, channel update/delete/test helpers, subscription safeguards.
- Regenerate `internal/db/notifications.sql.go`, `internal/db/models.go`, and `internal/db/querier.go` with `sqlc generate`.
- Modify `internal/db/notifications_contract_test.go`: migration/query contract coverage for account scope and Email.
- Modify `internal/notification/config.go`: Email config validation, encryption, redaction, HMAC hash.
- Modify `internal/notification/sender.go`: split target sender, add Resend sender, render text/html email, use delivery ID idempotency key.
- Modify `internal/notification/worker.go` and tests: route by channel kind and preserve dead-delivery guard.
- Modify `internal/config/config.go` and tests: read `RESEND_API_KEY`, `NOTIFICATION_EMAIL_FROM`, and `NOTIFICATION_EMAIL_REPLY_TO`.
- Modify `internal/scheduler/scheduler.go`: pass email sender config into worker.
- Modify `internal/api/server.go`: register `PATCH /notifications/channels/{channelID}`.
- Modify `internal/api/handlers_notifications.go` and tests: create/list/update/delete/test/subscription semantics and redacted response.
- Modify `internal/api/handlers_autopilot.go`: readiness copy includes Email and test-accepted wording.
- Modify `web/app/lib/api.ts` and tests: add `email`, destination body, update endpoint, redacted email and usage count fields.
- Modify `web/app/projects/[id]/settings/settings-client.tsx` and contract tests: Email create/test/status, account-channel copy, usage/delete confirmation, unaccepted Email subscription guard.

---

### Task 1: Schema And Query Contract

**Files:**
- Modify: `internal/migrations/0005_notifications.sql`
- Modify: `internal/db/queries/notifications.sql`
- Modify: `internal/db/notifications_contract_test.go`
- Generate: `internal/db/notifications.sql.go`, `internal/db/models.go`, `internal/db/querier.go`

- [ ] **Step 1: Write failing migration/query contract tests**

Add assertions that:

```go
for _, want := range []string{
  "owner_id text",
  "kind text not null check (kind in ('slack_webhook','discord_webhook','email'))",
  "idx_notification_channels_owner",
  "notification_subscription_owner_guard",
  "projects.owner_id",
} {
  if !strings.Contains(schema, want) {
    t.Fatalf("notification migration missing %q", want)
  }
}
```

Add query assertions that:

```go
if !strings.Contains(listNotificationChannels, "p.owner_id") {
  t.Fatal("ListNotificationChannels must be owner scoped")
}
if !strings.Contains(listNotificationChannels, "project_subscription_count") {
  t.Fatal("ListNotificationChannels must expose usage counts")
}
if !strings.Contains(upsertNotificationSubscription, "verified_at is not null") {
  t.Fatal("Email subscriptions must require accepted test sends")
}
```

- [ ] **Step 2: Run tests to verify RED**

Run:

```bash
go test ./internal/db -run 'TestNotification'
```

Expected: FAIL because the migration and query strings are still project-scoped and do not mention Email/account-owner safeguards.

- [ ] **Step 3: Implement migration and queries**

Update schema so existing fresh installs create:

```sql
owner_id text,
project_id uuid references projects(id),
kind text not null check (kind in ('slack_webhook','discord_webhook','email'))
```

Add trigger function:

```sql
create or replace function notification_subscription_owner_guard()
returns trigger as $$
begin
  if not exists (
    select 1
    from projects p
    join notification_channels c
      on c.id = new.channel_id
    where p.id = new.project_id
      and c.owner_id = p.owner_id
      and c.deleted_at is null
  ) then
    raise exception 'notification subscription channel owner mismatch';
  end if;
  return new;
end;
$$ language plpgsql;
```

Revise queries so channel list/create/get/update/delete are scoped by `projects.owner_id`, while subscriptions and deliveries remain scoped by `project_id`.

- [ ] **Step 4: Generate sqlc**

Run:

```bash
sqlc generate
```

Expected: generated Go compiles with new params/rows.

- [ ] **Step 5: Run GREEN**

Run:

```bash
go test ./internal/db -run 'TestNotification'
```

Expected: PASS.

### Task 2: Notification Config And Resend Sender

**Files:**
- Modify: `internal/notification/config.go`
- Modify: `internal/notification/config_test.go`
- Modify: `internal/notification/sender.go`
- Modify: `internal/notification/sender_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing config tests**

Add tests proving:

```go
cfg, err := PrepareEmailConfig("Owner-1", " Ops@Example.COM ", secret)
// cfg.EncryptedTo does not contain plaintext
// cfg.RedactedTo == "o***@example.com"
// cfg.AddressHash is stable for owner+normalized email
// same email under another owner produces a different hash
```

Add config env test:

```go
t.Setenv("RESEND_API_KEY", "resend-key")
t.Setenv("NOTIFICATION_EMAIL_FROM", "CiteLoop <notifications@citeloop.app>")
t.Setenv("NOTIFICATION_EMAIL_REPLY_TO", "support@citeloop.app")
env := FromEnv()
```

- [ ] **Step 2: Verify RED**

Run:

```bash
go test ./internal/notification ./internal/config -run 'Email|Resend|Notification'
```

Expected: FAIL because Email config and Resend env fields do not exist.

- [ ] **Step 3: Implement Email config**

Add:

```go
const KindEmail = "email"

type EmailConfig struct {
  EncryptedTo string `json:"encrypted_to"`
  RedactedTo string `json:"redacted_to"`
  AddressHash string `json:"address_hash"`
}
```

Normalize with `mail.ParseAddress` plus lowercased address. Encrypt with the existing `encryptString`. Hash with HMAC-SHA256 over `ownerID + "\n" + normalizedEmail`.

- [ ] **Step 4: Implement Resend sender tests**

Use `httptest.Server` and assert:

```go
req.Header.Get("Authorization") == "Bearer resend-key"
req.Header.Get("Idempotency-Key") == deliveryID.String()
body["from"] == "CiteLoop <notifications@citeloop.app>"
body["to"] includes the decrypted recipient
```

Also assert non-2xx and synchronous error responses return errors.

- [ ] **Step 5: Implement Resend sender**

Add a sender request shape for `POST /emails`, text/html rendering from payload fields, and provider message ID parsing for future storage/logging.

- [ ] **Step 6: Run GREEN**

Run:

```bash
go test ./internal/notification ./internal/config -run 'Email|Resend|Notification'
```

Expected: PASS.

### Task 3: Worker Routing

**Files:**
- Modify: `internal/notification/worker.go`
- Modify: `internal/notification/worker_test.go`
- Modify: `internal/scheduler/scheduler.go`

- [ ] **Step 1: Write failing worker tests**

Add tests that an Email delivery row:

```go
row.ChannelKind = KindEmail
row.ChannelConfig = emailCfg.JSON()
row.ID = deliveryID
```

calls sender with decrypted recipient and `deliveryID` idempotency key, then marks sent. Also assert sender failure follows retry/dead behavior and the existing `webhook.delivery.dead` guard still prevents recursion.

- [ ] **Step 2: Verify RED**

Run:

```bash
go test ./internal/notification -run 'Worker'
```

Expected: FAIL because worker only knows webhook URL sending.

- [ ] **Step 3: Implement target-based sender interface**

Replace the old `Send(ctx, kind, webhookURL, payload)` boundary with a target struct containing kind, destination, payload, and delivery ID. Route webhook configs through existing URL decryption and Email configs through recipient decryption.

- [ ] **Step 4: Wire scheduler config**

Pass `RESEND_API_KEY`, `NOTIFICATION_EMAIL_FROM`, and optional reply-to from config into the scheduler worker sender.

- [ ] **Step 5: Run GREEN**

Run:

```bash
go test ./internal/notification ./internal/scheduler
```

Expected: PASS.

### Task 4: API Semantics

**Files:**
- Modify: `internal/api/handlers_notifications.go`
- Modify: `internal/api/notifications_routes_test.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/handlers_autopilot.go`

- [ ] **Step 1: Write failing API tests**

Add route registration for `PATCH /notifications/channels/{channelID}`.

Add DTO tests proving:

```go
notificationChannelResponse(emailChannel)
```

omits `encrypted_to`, includes `redacted_to`, includes `project_subscription_count`, and uses account `owner_id`.

Add handler-level tests where practical for missing `RESEND_API_KEY`/`NOTIFICATION_EMAIL_FROM` on Email test.

- [ ] **Step 2: Verify RED**

Run:

```bash
go test ./internal/api -run 'Notification'
```

Expected: FAIL because Email DTO, PATCH route, and config behavior are absent.

- [ ] **Step 3: Implement API changes**

Create channels with account owner loaded from `projectID`. Support body:

```json
{"kind":"email","label":"Ops","destination":"ops@example.com"}
```

Keep backward-compatible `url` for Slack/Discord. Add `PATCH` for label update and explicit destination update rejection unless destination update is implemented. For V1, implement label update and reject destination changes with 400 to satisfy PRD AC 16.

Email test should require notification secret, Resend key, and sender, send through Resend, then mark accepted in `verified_at`.

Subscription upsert should call owner-scoped get-channel and reject enabled Email subscriptions without `verified_at`.

- [ ] **Step 4: Run GREEN**

Run:

```bash
go test ./internal/api -run 'Notification|Autopilot'
```

Expected: PASS.

### Task 5: Frontend API And Settings UI

**Files:**
- Modify: `web/app/lib/api.ts`
- Modify: `web/app/lib/api.test.mjs`
- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
- Modify: `web/app/projects/[id]/settings/settings-client.tsx`

- [ ] **Step 1: Write failing web contract tests**

Update `api.test.mjs` to assert:

```js
await client.createNotificationChannel("project-1", {
  kind: "email",
  destination: "ops@example.com",
  label: "Ops",
});
```

posts `destination`, and `updateNotificationChannel` calls `PATCH`.

Update Settings contract tests to look for:

```js
"Account channels"
"Project subscriptions"
"Email address"
"Test accepted"
"Destination"
"Used by"
"Add Slack, Discord, or Email"
```

and to reject old copy like `Slack or Discord webhook` in notification empty states.

- [ ] **Step 2: Verify RED**

Run:

```bash
npm test -- app/lib/api.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs
```

Expected: FAIL because Email UI/API support is absent.

- [ ] **Step 3: Implement UI/API**

Add `email` to `NotificationChannelKind`, support `config.redacted_to`, `owner_id`, and `project_subscription_count`.

Change Settings copy and controls:

- Kind selector: Slack, Discord, Email.
- Email input: `type="email"`, placeholder `ops@example.com`.
- Table header: `Destination`.
- Email row status: `Test accepted` when `verified_at`.
- Delete confirmation includes usage count.
- Events button blocks unaccepted Email channels with a message prompting test first.

- [ ] **Step 4: Run GREEN**

Run:

```bash
npm test -- app/lib/api.test.mjs app/lib/dashboard-ux-phase1-contract.test.mjs
npm run typecheck
```

Expected: PASS.

### Task 6: Full Verification, PR, Merge, Production

**Files:**
- All touched files

- [ ] **Step 1: Run full local verification**

Run:

```bash
go test ./...
cd web && npm test && npm run typecheck
```

Expected: all pass.

- [ ] **Step 2: Commit and push**

Run:

```bash
git status --short
git add <touched files>
git commit -m "feat: add account email notifications"
git push -u origin codex/email-notifications-implementation
```

- [ ] **Step 3: Create PR and wait for checks**

Create PR to `main`, wait for Go/Web/Vercel/Claude review checks, and address any actionable review comments.

- [ ] **Step 4: Merge and verify production**

After merge, wait for Railway API and Vercel production deployment to be green. Verify production Settings at:

```text
https://citeloop.app/projects/1459b054-cdc3-4d9b-9dd4-18e12458c61a/settings#notifications
```

Confirm:

- Email is present as a channel kind.
- The destination field switches to Email copy and input type.
- Existing Slack/Discord channels still list.
- API responses do not expose raw Email recipient config.
- If Resend env is missing, production shows a clear error rather than accepting Email tests.
- If Resend env is configured, create/test/subscribe an Email channel and verify delivery history for an emitted event reaches `sent`.

---

## Plan Self-Review

- Spec coverage: The plan covers account-scope channels, Email config, Resend delivery, event/subscription safety, UI copy, tests, rollout, and production verification from the PRD.
- Placeholder scan: No `TBD` or open implementation slots remain; the only conditional path is the PRD-approved choice to reject destination changes explicitly in V1.
- Type consistency: The plan uses `destination` as the new client/API field while retaining `url` compatibility for existing Slack/Discord callers.
