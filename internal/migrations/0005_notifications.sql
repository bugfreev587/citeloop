do $$
declare
  legacy_suffix text := to_char(clock_timestamp(), 'YYYYMMDDHH24MISSMS');
begin
  if to_regclass('public.notification_deliveries') is not null
     and not exists (
       select 1 from information_schema.columns
       where table_schema = 'public'
         and table_name = 'notification_deliveries'
         and column_name = 'project_id'
     ) then
    execute format('alter table public.notification_deliveries rename to %I', 'notification_deliveries_legacy_' || legacy_suffix);
  end if;

  if to_regclass('public.notification_subscriptions') is not null
     and not exists (
       select 1 from information_schema.columns
       where table_schema = 'public'
         and table_name = 'notification_subscriptions'
         and column_name = 'project_id'
     ) then
    execute format('alter table public.notification_subscriptions rename to %I', 'notification_subscriptions_legacy_' || legacy_suffix);
  end if;

  if to_regclass('public.notification_channels') is not null
     and not exists (
       select 1 from information_schema.columns
       where table_schema = 'public'
         and table_name = 'notification_channels'
         and column_name = 'project_id'
     ) then
    execute format('alter table public.notification_channels rename to %I', 'notification_channels_legacy_' || legacy_suffix);
  end if;
end $$;

create table if not exists notification_channels (
  id          uuid primary key default gen_random_uuid(),
  project_id  uuid not null references projects(id),
  kind text not null check (kind in ('slack_webhook','discord_webhook')),
  config jsonb not null,
  label       text not null default '',
  verified_at timestamptz,
  created_at  timestamptz not null default now(),
  deleted_at  timestamptz
);

create table if not exists notification_subscriptions (
  id          uuid primary key default gen_random_uuid(),
  project_id  uuid not null references projects(id),
  event_type  text not null,
  channel_id  uuid not null references notification_channels(id),
  enabled     boolean not null default true,
  filter      jsonb,
  created_at  timestamptz not null default now(),
  unique (project_id, event_type, channel_id)
);

create table if not exists notification_deliveries (
  id              uuid primary key default gen_random_uuid(),
  project_id      uuid not null references projects(id),
  subscription_id uuid references notification_subscriptions(id),
  channel_id      uuid not null references notification_channels(id),
  event_type      text not null,
  event_id        text not null,
  payload         jsonb not null,
  status text not null default 'pending' check (status in ('pending','sent','dead')),
  attempts        int not null default 0,
  next_retry_at   timestamptz,
  last_error      text,
  delivered_at    timestamptz,
  created_at      timestamptz not null default now(),
  unique (event_id, channel_id)
);

create index if not exists idx_notification_channels_project
  on notification_channels (project_id)
  where deleted_at is null;

create index if not exists idx_notification_deliveries_pending
  on notification_deliveries (status, next_retry_at, created_at)
  where status = 'pending';
