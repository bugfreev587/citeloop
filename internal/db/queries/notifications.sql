-- name: CreateNotificationChannel :one
insert into notification_channels (project_id, kind, config, label)
values ($1, $2, $3, $4)
returning *;

-- name: ListNotificationChannels :many
select * from notification_channels
where project_id = $1
  and deleted_at is null
order by created_at desc;

-- name: GetNotificationChannel :one
select * from notification_channels
where id = $1
  and project_id = $2
  and deleted_at is null;

-- name: MarkNotificationChannelVerified :one
update notification_channels
set verified_at = now()
where id = $1
  and project_id = $2
  and deleted_at is null
returning *;

-- name: SoftDeleteNotificationChannel :one
update notification_channels
set deleted_at = now()
where id = $1
  and project_id = $2
returning *;

-- name: UpsertNotificationSubscription :one
insert into notification_subscriptions (project_id, event_type, channel_id, enabled, filter)
values ($1, $2, $3, $4, $5)
on conflict (project_id, event_type, channel_id) do update
set enabled = excluded.enabled,
    filter = excluded.filter
returning *;

-- name: ListNotificationSubscriptions :many
select * from notification_subscriptions
where project_id = $1
order by event_type, created_at desc;

-- name: ListEnabledNotificationSubscriptionsForEvent :many
select s.* from notification_subscriptions s
join notification_channels c
  on c.id = s.channel_id
where s.project_id = $1
  and s.event_type = $2
  and s.enabled = true
  and c.deleted_at is null
order by s.created_at asc;

-- name: CreateNotificationDelivery :one
insert into notification_deliveries (project_id, subscription_id, channel_id, event_type, event_id, payload, next_retry_at)
values ($1, $2, $3, $4, $5, $6, now())
on conflict (event_id, channel_id) do nothing
returning *;

-- name: ListNotificationDeliveries :many
select * from notification_deliveries
where project_id = sqlc.arg(project_id)
  and (sqlc.arg(status)::text = '' or status = sqlc.arg(status))
order by created_at desc
limit sqlc.arg(limit_rows);

-- name: ListPendingNotificationDeliveries :many
select
  d.id,
  d.project_id,
  d.subscription_id,
  d.channel_id,
  d.event_type,
  d.event_id,
  d.payload,
  d.status,
  d.attempts,
  d.next_retry_at,
  d.last_error,
  d.delivered_at,
  d.created_at,
  c.kind as channel_kind,
  c.config as channel_config
from notification_deliveries d
join notification_channels c
  on c.id = d.channel_id
where d.status = 'pending'
  and (d.next_retry_at is null or d.next_retry_at <= now())
  and c.deleted_at is null
order by d.created_at asc
for update skip locked
limit $1;

-- name: MarkNotificationDeliverySent :one
update notification_deliveries
set status = 'sent',
    delivered_at = now(),
    last_error = null
where id = $1
returning *;

-- name: MarkNotificationDeliveryFailed :one
update notification_deliveries
set attempts = attempts + 1,
    status = case when attempts + 1 >= 4 then 'dead' else 'pending' end,
    next_retry_at = case
      when attempts + 1 >= 4 then null
      when attempts + 1 = 1 then now() + interval '1 minute'
      when attempts + 1 = 2 then now() + interval '5 minutes'
      else now() + interval '30 minutes'
    end,
    last_error = $2
where id = $1
returning *;

-- name: RetryNotificationDelivery :one
update notification_deliveries
set status = 'pending',
    next_retry_at = now(),
    last_error = null
where id = $1
  and project_id = $2
returning *;
