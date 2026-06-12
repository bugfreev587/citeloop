create table if not exists workflow_events (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  event_type text not null,
  entity_type text,
  entity_id uuid,
  dedupe_key text not null unique,
  payload jsonb not null default '{}',
  status text not null default 'pending'
    check (status in ('pending','running','succeeded','failed','dead')),
  attempts int not null default 0,
  run_after timestamptz not null default now(),
  locked_at timestamptz,
  processed_at timestamptz,
  error text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists idx_workflow_events_pending
  on workflow_events (status, run_after, created_at)
  where status = 'pending';

create index if not exists idx_workflow_events_reclaim
  on workflow_events (status, locked_at)
  where status = 'running';

alter table topics
  add column if not exists source_content_action_id uuid references content_actions(id) on delete set null;

create unique index if not exists uniq_topic_source_content_action
  on topics(project_id, source_content_action_id)
  where source_content_action_id is not null;
