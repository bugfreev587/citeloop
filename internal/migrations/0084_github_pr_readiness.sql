set local lock_timeout = '5s';
set local statement_timeout = '4min';

alter table publisher_connections
  add column if not exists pr_readiness_status text not null default 'not_connected',
  add column if not exists pr_readiness_checked_at timestamptz,
  add column if not exists pr_readiness_detail text;

alter table publisher_connections
  drop constraint if exists publisher_connections_pr_readiness_status_check;

alter table publisher_connections
  add constraint publisher_connections_pr_readiness_status_check
  check (pr_readiness_status in ('not_connected', 'not_checked', 'ready', 'permission_missing', 'repository_unavailable', 'error')) not valid;

update publisher_connections
set pr_readiness_status = case
  when kind = 'github_nextjs' and status = 'connected' and enabled then 'not_checked'
  else 'not_connected'
end;

alter table publisher_connections
  validate constraint publisher_connections_pr_readiness_status_check;
