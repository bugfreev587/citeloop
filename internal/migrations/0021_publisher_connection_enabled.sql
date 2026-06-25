alter table publisher_connections
  add column if not exists enabled boolean not null default false;

drop index if exists publisher_connections_project_idx;

create index if not exists publisher_connections_project_idx
  on publisher_connections (project_id, kind, status, enabled);
