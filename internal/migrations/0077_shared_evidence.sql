create table if not exists evidence_runs (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  source text not null check (source in ('crawl','render','gsc','ga4','ai_answer','context','publisher')),
  normalized_target text not null,
  target_kind text not null check (target_kind in ('page','query','prompt','entity','site','integration')),
  window_start date,
  window_end date,
  collection_spec jsonb not null check (jsonb_typeof(collection_spec) = 'object'),
  collection_spec_fingerprint text not null check (length(collection_spec_fingerprint) >= 16),
  collection_owner_token uuid not null,
  attempt_number int not null default 1 check (attempt_number > 0),
  lease_expires_at timestamptz not null,
  error_history jsonb not null default '[]'::jsonb check (jsonb_typeof(error_history) = 'array'),
  requested_by jsonb not null default '[]'::jsonb check (jsonb_typeof(requested_by) = 'array'),
  status text not null default 'running' check (status in ('running','completed','partial','failed')),
  error_summary text,
  started_at timestamptz not null default now(),
  finished_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (id, project_id),
  check (window_end is null or window_start is null or window_end >= window_start),
  check ((status = 'running' and finished_at is null) or status <> 'running')
);

create unique index evidence_runs_collection_identity_unique
  on evidence_runs (
    project_id, source, normalized_target, target_kind,
    coalesce(window_start, '-infinity'::date),
    coalesce(window_end, 'infinity'::date),
    collection_spec_fingerprint
  );

create index evidence_runs_project_source_created_idx
  on evidence_runs (project_id, source, created_at desc);

create table if not exists evidence_run_attempts (
  run_id uuid not null,
  project_id uuid not null,
  attempt_number int not null check (attempt_number > 0),
  collection_owner_token uuid not null,
  status text not null check (status in ('running','completed','partial','failed')),
  error_summary text,
  started_at timestamptz not null,
  lease_expires_at timestamptz not null,
  finished_at timestamptz,
  created_at timestamptz not null default now(),
  primary key (run_id, attempt_number),
  unique (run_id, project_id, attempt_number),
  foreign key (run_id, project_id) references evidence_runs(id, project_id) on delete cascade
);

create table if not exists evidence_observations (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  run_id uuid not null,
  attempt_number int not null check (attempt_number > 0),
  source text not null,
  source_observation_key text not null,
  normalized_target text not null,
  target_kind text not null,
  evidence_state text not null check (evidence_state in ('observed','inferred','model_assisted','missing','provider_unavailable')),
  facts jsonb not null default '{}'::jsonb check (jsonb_typeof(facts) = 'object'),
  raw_snapshot jsonb not null default '{}'::jsonb,
  confidence numeric(6,5) not null default 0 check (confidence >= 0 and confidence <= 1),
  completeness numeric(6,5) not null default 0 check (completeness >= 0 and completeness <= 1),
  provider text,
  model text,
  provider_version text,
  prompt_version text,
  call_status text check (call_status is null or call_status in ('ok','partial','failed','skipped')),
  prompt_tokens bigint not null default 0 check (prompt_tokens >= 0),
  completion_tokens bigint not null default 0 check (completion_tokens >= 0),
  total_tokens bigint not null default 0 check (total_tokens >= 0),
  cost_usd numeric not null default 0 check (cost_usd >= 0),
  privacy_state text not null default 'not_applicable',
  permission_state text not null default 'not_applicable',
  error_code text,
  observed_at timestamptz not null,
  window_start date,
  window_end date,
  created_at timestamptz not null default now(),
  foreign key (run_id, project_id, attempt_number) references evidence_run_attempts(run_id, project_id, attempt_number) on delete cascade,
  unique (run_id, attempt_number, source_observation_key)
);

create index evidence_observations_project_target_idx
  on evidence_observations (project_id, source, normalized_target, observed_at desc);

create or replace function enforce_evidence_observation_run_scope()
returns trigger language plpgsql as $$
begin
  if not exists (
    select 1 from evidence_runs run
    where run.id = new.run_id and run.project_id = new.project_id
      and run.source = new.source
      and run.normalized_target = new.normalized_target
      and run.target_kind = new.target_kind
      and run.attempt_number = new.attempt_number
      and run.window_start is not distinct from new.window_start
      and run.window_end is not distinct from new.window_end
  ) then
    raise exception 'Evidence observation does not match its collection run scope' using errcode = '23514';
  end if;
  return new;
end;
$$;

create trigger evidence_observations_run_scope
before insert on evidence_observations
for each row execute function enforce_evidence_observation_run_scope();

create or replace function prevent_evidence_observation_mutation()
returns trigger language plpgsql as $$
begin
  if tg_op = 'DELETE' and pg_trigger_depth() > 1 then
    return old;
  end if;
  raise exception 'Evidence observations are immutable' using errcode = '55000';
end;
$$;

create trigger evidence_observations_immutable
before update or delete on evidence_observations
for each row execute function prevent_evidence_observation_mutation();

create table if not exists evidence_consumptions (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  evidence_run_id uuid not null,
  attempt_number int not null check (attempt_number > 0),
  consumer_type text not null check (consumer_type in ('seo_run','geo_run','doctor_run','growth_action')),
  consumer_id uuid not null,
  created_at timestamptz not null default now(),
  foreign key (evidence_run_id, project_id, attempt_number) references evidence_run_attempts(run_id, project_id, attempt_number) on delete cascade,
  unique (project_id, evidence_run_id, attempt_number, consumer_type, consumer_id)
);

create index evidence_consumptions_consumer_idx
  on evidence_consumptions (project_id, consumer_type, consumer_id, created_at);
