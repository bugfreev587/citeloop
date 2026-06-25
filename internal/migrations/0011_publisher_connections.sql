create table if not exists publisher_connections (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  kind text not null check (kind in ('github_nextjs','webhook','wordpress')),
  label text not null default '',
  status text not null default 'missing' check (status in ('missing','connected','error','revoked')),
  is_default boolean not null default false,
  enabled boolean not null default false,
  capabilities jsonb not null default '{}',
  capability_schema_version int not null default 1,
  credential_ref text,
  config jsonb not null default '{}',
  oauth_access_expires_at timestamptz,
  oauth_refresh_status text,
  revoked_at timestamptz,
  last_verified_at timestamptz,
  last_error text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index if not exists publisher_connections_default_kind_key
  on publisher_connections (project_id, kind)
  where is_default;

create index if not exists publisher_connections_project_idx
  on publisher_connections (project_id, kind, status, enabled);
