-- GEO Visibility Layer PR1: crawler access audit.

create table if not exists geo_runs (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  agent text not null check (agent in ('geo_crawler_audit')),
  status text not null check (status in ('ok','degraded','error')),
  provider text not null default 'citeloop_honest_probe',
  started_at timestamptz not null default now(),
  finished_at timestamptz,
  input jsonb not null default '{}',
  output jsonb not null default '{}',
  error text,
  cost_usd numeric
);

create index if not exists idx_geo_runs_project_agent_started
  on geo_runs (project_id, agent, started_at desc);

create table if not exists ai_crawler_access_snapshots (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  run_id uuid not null references geo_runs(id) on delete cascade,
  page_url text not null,
  normalized_page_url text not null,
  target_user_agent text not null,
  probe_user_agent text not null,
  evidence_type text not null check (evidence_type in ('robots_static','honest_probe','manual_confirmation')),
  robots_state text not null check (robots_state in ('allowed','disallowed','unknown')),
  http_status int,
  access_state text not null check (access_state in ('ok','blocked','challenge','rate_limited','timeout','error')),
  confidence text not null check (confidence in ('high','medium','low')),
  inferred boolean not null default false,
  meta_robots_state text,
  sitemap_state text,
  body_extractable boolean not null default false,
  raw_details jsonb not null default '{}',
  checked_at timestamptz not null default now(),
  unique (project_id, run_id, normalized_page_url, target_user_agent, evidence_type)
);

create index if not exists idx_ai_crawler_access_project_checked
  on ai_crawler_access_snapshots (project_id, checked_at desc);

create index if not exists idx_ai_crawler_access_project_agent
  on ai_crawler_access_snapshots (project_id, target_user_agent, checked_at desc);
