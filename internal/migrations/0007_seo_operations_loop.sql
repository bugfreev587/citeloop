-- SEO Operations Loop foundation.

alter table articles
  add column if not exists content_hash text;

update articles
set content_hash = encode(digest(coalesce(content_md, '') || coalesce(seo_meta::text, ''), 'sha256'), 'hex')
where content_hash is null;

alter table generation_runs
  drop constraint if exists generation_runs_agent_check;

alter table generation_runs
  add constraint generation_runs_agent_check
  check (agent in (
    'insight',
    'strategist',
    'writer',
    'qa',
    'publisher',
    'notification',
    'seo_sync',
    'seo_analyzer',
    'seo_brief',
    'seo_measurer',
    'reviser'
  ));

create table if not exists seo_properties (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  site_url text not null,
  gsc_site_url text,
  ga4_property_id text,
  url_normalization_config jsonb not null default '{}',
  default_country text,
  default_language text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, site_url)
);

create table if not exists seo_integrations (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  provider text not null check (provider in ('google_search_console','google_analytics','bigquery','serp_provider')),
  status text not null default 'missing' check (status in ('missing','connected','expired','error')),
  credential_ref text,
  last_verified_at timestamptz,
  last_error text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, provider)
);

create table if not exists seo_runs (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  agent text not null check (agent in ('seo_sync','seo_analyzer','seo_brief','seo_measurer')),
  status text not null check (status in ('ok','error','degraded')),
  started_at timestamptz not null default now(),
  finished_at timestamptz,
  cost_usd numeric,
  input jsonb not null default '{}',
  output jsonb not null default '{}',
  error text
);

create index if not exists idx_seo_runs_project_agent_started
  on seo_runs (project_id, agent, started_at desc);

create table if not exists search_performance_daily (
  project_id uuid not null references projects(id) on delete cascade,
  property_id uuid not null references seo_properties(id) on delete cascade,
  date date not null,
  page_url text not null,
  normalized_page_url text not null,
  query text not null,
  country text not null default '',
  device text not null default '',
  clicks numeric,
  impressions numeric,
  ctr numeric,
  position numeric,
  query_data_partial boolean not null default true,
  source text not null default 'gsc_api' check (source in ('gsc_api','gsc_bigquery','manual_fixture')),
  updated_at timestamptz not null default now(),
  primary key (project_id, property_id, date, normalized_page_url, query, country, device)
);

create table if not exists page_performance_daily (
  project_id uuid not null references projects(id) on delete cascade,
  property_id uuid not null references seo_properties(id) on delete cascade,
  date date not null,
  page_url text not null,
  normalized_page_url text not null,
  article_id uuid references articles(id) on delete set null,
  topic_id uuid references topics(id) on delete set null,
  clicks numeric,
  impressions numeric,
  weighted_position numeric,
  ctr numeric,
  ga4_sessions numeric,
  ga4_engaged_sessions numeric,
  ga4_conversions numeric,
  indexed_state text,
  technical_status text,
  data_source_notes jsonb not null default '{}',
  updated_at timestamptz not null default now(),
  primary key (project_id, property_id, date, normalized_page_url)
);

create table if not exists search_appearance_daily (
  project_id uuid not null references projects(id) on delete cascade,
  property_id uuid not null references seo_properties(id) on delete cascade,
  date date not null,
  search_appearance text not null,
  clicks numeric,
  impressions numeric,
  ctr numeric,
  position numeric,
  source text not null default 'gsc_api' check (source in ('gsc_api','gsc_bigquery','manual_fixture')),
  updated_at timestamptz not null default now(),
  primary key (project_id, property_id, date, search_appearance)
);

create table if not exists url_index_snapshots (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  run_id uuid not null references seo_runs(id) on delete cascade,
  page_url text not null,
  normalized_page_url text not null,
  article_id uuid references articles(id) on delete set null,
  inspection_status text,
  coverage_state text,
  google_canonical text,
  user_canonical text,
  last_crawl_time timestamptz,
  robots_txt_state text,
  page_fetch_state text,
  raw_summary jsonb not null default '{}',
  inspected_at timestamptz not null default now(),
  unique (project_id, run_id, normalized_page_url)
);

create table if not exists technical_checks (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  run_id uuid not null references seo_runs(id) on delete cascade,
  page_url text not null,
  normalized_page_url text not null,
  article_id uuid references articles(id) on delete set null,
  http_status int,
  canonical_status text,
  robots_status text,
  title_status text,
  meta_description_status text,
  h1_status text,
  structured_data_status text,
  sitemap_status text,
  internal_link_count int,
  outbound_link_count int,
  content_hash text,
  unsafe_mdx_detected boolean not null default false,
  raw_details jsonb not null default '{}',
  checked_at timestamptz not null default now(),
  unique (project_id, run_id, normalized_page_url)
);

create table if not exists seo_opportunities (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  type text not null,
  status text not null default 'open' check (status in ('open','accepted','dismissed','converted','done','stale')),
  priority_score numeric not null default 0,
  confidence numeric not null default 0,
  page_url text,
  normalized_page_url text not null default '',
  article_id uuid references articles(id) on delete set null,
  topic_id uuid references topics(id) on delete set null,
  query text,
  evidence jsonb not null default '{}',
  recommended_action text,
  expected_impact text,
  effort int not null default 1,
  risk_level text not null default 'low' check (risk_level in ('low','medium','high')),
  created_by_run_id uuid references seo_runs(id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, type, normalized_page_url, query, created_by_run_id)
);

create index if not exists idx_seo_opportunities_project_status_priority
  on seo_opportunities (project_id, status, priority_score desc, created_at desc);

create table if not exists content_actions (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  opportunity_id uuid not null references seo_opportunities(id) on delete cascade,
  action_type text not null,
  status text not null default 'drafting' check (status in ('drafting','ready_for_review','approved','published','measuring','completed','failed')),
  target_article_id uuid references articles(id) on delete set null,
  target_url text,
  normalized_target_url text,
  target_content_hash_before text,
  target_content_hash_after text,
  draft_article_id uuid references articles(id) on delete set null,
  baseline_window jsonb not null default '{}',
  measurement_window jsonb not null default '{}',
  published_at timestamptz,
  outcome_summary jsonb not null default '{}',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, opportunity_id, action_type)
);

create table if not exists internal_link_edges (
  project_id uuid not null references projects(id) on delete cascade,
  source_url text not null,
  normalized_source_url text not null,
  target_url text not null,
  normalized_target_url text not null,
  anchor_text text not null,
  link_context text,
  source_article_id uuid references articles(id) on delete set null,
  target_article_id uuid references articles(id) on delete set null,
  first_seen_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  primary key (project_id, normalized_source_url, normalized_target_url, anchor_text)
);
