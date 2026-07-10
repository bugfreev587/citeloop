alter table public.notification_channels
  add column if not exists owner_id text;

update public.notification_channels c
set owner_id = p.owner_id
from public.projects p
where c.project_id = p.id
  and (c.owner_id is null or c.owner_id = '');

alter table public.notification_channels
  alter column owner_id set not null;

alter table public.notification_channels
  alter column project_id drop not null;

alter table public.notification_channels
  drop constraint if exists notification_channels_kind_check;

alter table public.notification_channels
  add constraint notification_channels_kind_check
  check (kind in ('slack_webhook','discord_webhook','email'));

create index if not exists idx_notification_channels_owner
  on public.notification_channels (owner_id, created_at desc)
  where deleted_at is null;

create or replace function public.notification_subscription_owner_guard()
returns trigger as $$
begin
  if not exists (
    select 1
    from public.projects
    join public.notification_channels
      on notification_channels.id = new.channel_id
    where projects.id = new.project_id
      and notification_channels.owner_id = projects.owner_id
      and notification_channels.deleted_at is null
  ) then
    raise exception 'notification subscription channel owner mismatch';
  end if;
  return new;
end;
$$ language plpgsql;

drop trigger if exists trg_notification_subscription_owner_guard on public.notification_subscriptions;
create trigger trg_notification_subscription_owner_guard
before insert or update of project_id, channel_id on public.notification_subscriptions
for each row execute function public.notification_subscription_owner_guard();
