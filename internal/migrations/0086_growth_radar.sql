-- Growth Radar prompt rotation, source evidence cache, deterministic funnel,
-- and replayable candidate dispositions.

alter table geo_prompts
  add column if not exists cluster_key text not null default '',
  add column if not exists last_observed_at timestamptz,
  add column if not exists next_observed_at timestamptz,
  add column if not exists observation_count int not null default 0,
  add column if not exists targeted_reason text not null default '',
  add column if not exists archived_reason text not null default '';

create index if not exists idx_geo_prompts_rotation
  on geo_prompts(project_id, status, next_observed_at, last_observed_at, id);

update geo_prompts
set cluster_key = lower(regexp_replace(coalesce(nullif(btrim(target_topic), ''), nullif(btrim(intent_type), ''), id::text), '[^a-z0-9]+', '-', 'g'))
where cluster_key = '';

with ranked as (
  select id, project_id, priority, created_at,
         row_number() over (partition by project_id, cluster_key order by priority desc, created_at, id) cluster_rank,
         row_number() over (partition by project_id, intent_type, target_persona order by priority desc, created_at, id) pair_rank
  from geo_prompts where status = 'active'
), eligible as (
  select id, project_id,
         row_number() over (partition by project_id order by priority desc, created_at, id) project_rank
  from ranked where cluster_rank <= 6 and pair_rank <= 2
), keepers as (
  select id from eligible where project_rank <= 60
)
update geo_prompts prompt
set status = 'archived', archived_reason = 'portfolio_cap_migration', updated_at = now()
where prompt.status = 'active' and not exists (select 1 from keepers where keepers.id = prompt.id);

create table if not exists growth_search_evidence (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  normalized_query text not null,
  request_hash text not null,
  result_set_hash text not null,
  provider text not null,
  provider_order_not_rank boolean not null default true,
  results jsonb not null default '[]'::jsonb,
  synthetic boolean not null default false,
  trigger_kind text not null default 'daily',
  request_cost_usd numeric(12,6) not null default 0,
  fetched_at timestamptz not null,
  expires_at timestamptz not null,
  created_at timestamptz not null default now()
);

create index if not exists idx_growth_search_evidence_budget
  on growth_search_evidence(project_id, fetched_at desc);
create index if not exists idx_growth_search_evidence_cache
  on growth_search_evidence(project_id, request_hash, expires_at desc);

create table if not exists growth_radar_runs (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  phase text not null,
  status text not null check (status in ('ok','degraded','failed')),
  funnel jsonb not null default '{}'::jsonb,
  cost_usd numeric(12,6) not null default 0,
  created_at timestamptz not null default now()
);

create index if not exists idx_growth_radar_runs_project_created
  on growth_radar_runs(project_id, created_at desc);

create table if not exists growth_radar_items (
  id uuid primary key default gen_random_uuid(),
  run_id uuid not null references growth_radar_runs(id) on delete cascade,
  project_id uuid not null references projects(id) on delete cascade,
  candidate_identity text not null,
  disposition text not null,
  reason text not null default '',
  score jsonb not null default '{}'::jsonb,
  scoring_snapshot jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  unique(run_id, candidate_identity)
);
