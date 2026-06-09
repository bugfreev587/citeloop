alter table projects
  add column if not exists status text not null default 'active';

alter table projects
  drop constraint if exists projects_status_check;

alter table projects
  add constraint projects_status_check
  check (status in ('active', 'archived'));

create index if not exists projects_owner_status_created_at_idx
  on projects (owner_id, status, created_at desc);
