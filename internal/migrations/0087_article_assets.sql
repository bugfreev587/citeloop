-- Non-blocking generated media attached to an article. Text review and
-- publication never depend on an asset reaching ready.
create table if not exists article_assets (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  article_id uuid not null references articles(id) on delete cascade,
  role text not null check (role in ('hero','inline_1','inline_2','benchmark_chart')),
  status text not null default 'planned' check (status in ('planned','generating','ready','failed')),
  brief jsonb not null default '{}'::jsonb,
  brief_hash text not null,
  revision integer not null default 1 check (revision > 0),
  prompt text not null default '',
  provider text not null default '',
  model text not null default '',
  mime_type text not null default '',
  storage_key text not null default '',
  stable_url text not null default '',
  alt_text text not null default '',
  caption text not null default '',
  width integer not null default 0,
  height integer not null default 0,
  error text not null default '',
  omitted boolean not null default false,
  generated_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(article_id, role, brief_hash, revision)
);

create index if not exists article_assets_project_article_idx on article_assets(project_id, article_id, created_at);
create index if not exists article_assets_status_idx on article_assets(status, updated_at);

create table if not exists article_asset_objects (
  storage_key text primary key,
  data bytea not null,
  mime_type text not null,
  created_at timestamptz not null default now()
);

create table if not exists admin_image_credentials (
  singleton boolean primary key default true check (singleton),
  provider text not null default 'openai' check (provider = 'openai'),
  encrypted_api_key text not null,
  base_url text not null default 'https://api.openai.com/v1',
  model text not null,
  enabled boolean not null default true,
  updated_at timestamptz not null default now()
);
