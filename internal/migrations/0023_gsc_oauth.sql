-- Self-serve Google Search Console OAuth token storage.

alter table seo_integrations drop constraint if exists seo_integrations_status_check;
alter table seo_integrations
  add constraint seo_integrations_status_check
  check (status in ('missing','connected','property_selection_required','expired','error','revoked'));

create table if not exists seo_oauth_tokens (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  provider text not null check (provider in ('google_search_console')),
  encrypted_refresh_token text not null,
  token_type text not null default 'Bearer',
  scope text not null default '',
  access_token_expires_at timestamptz,
  account_email text,
  selected_property text,
  authorized_properties jsonb not null default '[]',
  last_error text,
  revoked_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, provider)
);

create index if not exists idx_seo_oauth_tokens_project_provider_active
  on seo_oauth_tokens (project_id, provider)
  where revoked_at is null;
