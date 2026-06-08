create table if not exists publisher_credentials (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  connection_id uuid not null references publisher_connections(id) on delete cascade,
  kind text not null check (kind in ('github_token','webhook_secret','oauth_refresh_token')),
  encrypted_value text not null,
  redacted_value text not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  revoked_at timestamptz,
  unique (project_id, connection_id, kind)
);

create index if not exists publisher_credentials_connection_idx
  on publisher_credentials (project_id, connection_id, kind)
  where revoked_at is null;
