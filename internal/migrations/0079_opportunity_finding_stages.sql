set local lock_timeout = '5s';
set local statement_timeout = '30s';

create table if not exists opportunity_finding_stage_checkpoints (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  workflow_event_id uuid not null references workflow_events(id) on delete cascade,
  stage text not null check (stage in (
    'evidence_refresh',
    'deterministic_signals',
    'ai_hypotheses',
    'arbitration',
    'materialization',
    'summary'
  )),
  stage_order smallint not null check (stage_order between 1 and 6),
  request_fingerprint text not null check (request_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
  status text not null check (status in ('running','succeeded','partial','failed','skipped')),
  attempt_number integer not null default 1 check (attempt_number > 0),
  owner_token uuid not null,
  lease_expires_at timestamptz,
  output_summary jsonb not null default '{}'::jsonb check (jsonb_typeof(output_summary) = 'object'),
  error text,
  started_at timestamptz not null default now(),
  finished_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (workflow_event_id, stage),
  constraint opportunity_finding_stage_terminal_shape check (
    (status = 'running' and finished_at is null and lease_expires_at is not null)
    or
    (status in ('succeeded','partial','failed','skipped') and finished_at is not null and lease_expires_at is null)
  )
);

create or replace function enforce_opportunity_finding_stage_project()
returns trigger language plpgsql as $$
begin
  if not exists (
    select 1 from workflow_events event
    where event.id = new.workflow_event_id
      and event.project_id = new.project_id
      and event.event_type = 'opportunity_finding.requested'
  ) then
    raise exception 'workflow event project mismatch for Opportunity Finding checkpoint';
  end if;
  return new;
end $$;

drop trigger if exists opportunity_finding_stage_project_guard on opportunity_finding_stage_checkpoints;
create trigger opportunity_finding_stage_project_guard
before insert or update of project_id, workflow_event_id
on opportunity_finding_stage_checkpoints
for each row execute function enforce_opportunity_finding_stage_project();

reset statement_timeout;
reset lock_timeout;
