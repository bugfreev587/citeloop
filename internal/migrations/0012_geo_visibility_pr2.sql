-- GEO Visibility Layer PR2: prompt sets, manual fixture observations, score history.

alter table geo_runs drop constraint if exists geo_runs_agent_check;
alter table geo_runs add constraint geo_runs_agent_check check (
  agent in ('geo_crawler_audit','geo_prompt_builder','geo_observer','geo_analyzer','geo_asset_brief')
);

create table if not exists geo_prompt_sets (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  name text not null,
  status text not null check (status in ('draft','active','paused','archived')) default 'draft',
  locale text not null default 'en-US',
  created_by_run_id uuid references geo_runs(id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists idx_geo_prompt_sets_project_status
  on geo_prompt_sets (project_id, status, updated_at desc);

create table if not exists geo_competitors (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  name text not null,
  name_key text generated always as (lower(btrim(name))) stored,
  domains jsonb not null default '[]',
  aliases jsonb not null default '[]',
  source text not null check (source in ('profile','search_result','manual')) default 'manual',
  status text not null check (status in ('active','paused','archived')) default 'active',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, name_key)
);

create index if not exists idx_geo_competitors_project_status
  on geo_competitors (project_id, status, updated_at desc);

create table if not exists geo_prompts (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  prompt_set_id uuid not null references geo_prompt_sets(id) on delete cascade,
  prompt_text text not null,
  intent_type text not null,
  target_persona text not null default '',
  target_topic text not null default '',
  locale text not null default 'en-US',
  target_engines jsonb not null default '[]',
  priority int not null default 5 check (priority >= 0 and priority <= 10),
  source text not null check (source in ('profile','topic','competitor','manual','search_result')) default 'profile',
  status text not null check (status in ('active','paused','archived')) default 'active',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, prompt_set_id, prompt_text, locale)
);

create index if not exists idx_geo_prompts_project_set_status
  on geo_prompts (project_id, prompt_set_id, status, priority desc);

create table if not exists geo_external_surfaces (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  url text not null,
  normalized_url text not null,
  platform text not null default 'site',
  surface_type text not null default 'domain',
  owner_type text not null check (owner_type in ('project','user','third_party')) default 'project',
  canonical_target_url text,
  backlink_state text not null default 'unknown',
  last_http_status int,
  last_cited_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, normalized_url)
);

create index if not exists idx_geo_external_surfaces_project_owner
  on geo_external_surfaces (project_id, owner_type, updated_at desc);

create table if not exists geo_observations (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  run_id uuid not null references geo_runs(id) on delete cascade,
  prompt_id uuid references geo_prompts(id) on delete set null,
  engine text not null,
  locale text not null default 'en-US',
  source_type text not null check (source_type in ('answer_engine','serp_probe','manual_fixture','manual_required')),
  brand_mentioned boolean not null default false,
  brand_position int,
  project_citation_count int not null default 0,
  project_citation_rank_best int,
  project_cited_surface_ids jsonb not null default '[]',
  cited_urls jsonb not null default '[]',
  competitor_mentions jsonb not null default '[]',
  competitor_citations jsonb not null default '[]',
  observation_state text not null check (observation_state in ('observed','manual_required','provider_unavailable','budget_skipped','error')) default 'observed',
  answer_summary text not null default '',
  evidence_snippets jsonb not null default '[]',
  confidence text not null check (confidence in ('high','medium','low')) default 'medium',
  observed_at timestamptz not null default now()
);

create index if not exists idx_geo_observations_project_observed
  on geo_observations (project_id, observed_at desc);

create index if not exists idx_geo_observations_project_prompt
  on geo_observations (project_id, prompt_id, observed_at desc);

create table if not exists geo_visibility_scores (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  run_id uuid references geo_runs(id) on delete set null,
  score numeric not null default 0,
  coverage numeric not null default 0,
  confidence text not null check (confidence in ('high','medium','low','insufficient_data')),
  breakdown jsonb not null default '{}',
  prompt_count_total int not null default 0,
  prompt_count_observed int not null default 0,
  engine_count_observed int not null default 0,
  computed_at timestamptz not null default now()
);

create index if not exists idx_geo_visibility_scores_project_computed
  on geo_visibility_scores (project_id, computed_at desc);
