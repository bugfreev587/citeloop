-- GEO Visibility Layer PR3: analyzer asset briefs.

create table if not exists geo_asset_briefs (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  opportunity_id uuid not null references seo_opportunities(id) on delete cascade,
  asset_type text not null,
  status text not null check (status in ('draft','ready_for_review','accepted','converted','dismissed')) default 'draft',
  target_prompts jsonb not null default '[]',
  required_evidence jsonb not null default '[]',
  recommended_outline jsonb not null default '[]',
  internal_link_plan jsonb not null default '[]',
  publication_surface text not null default 'blog',
  created_by_run_id uuid references geo_runs(id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, opportunity_id)
);

create index if not exists idx_geo_asset_briefs_project_status
  on geo_asset_briefs (project_id, status, updated_at desc);
