alter table projects
  add column if not exists updated_at timestamptz;

update projects
set updated_at = created_at
where updated_at is null;

alter table projects
  alter column updated_at set default now(),
  alter column updated_at set not null;

create index if not exists idx_projects_updated_at
  on projects (updated_at desc, created_at desc);
