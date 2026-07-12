set local lock_timeout = '5s';
set local statement_timeout = '30s';

alter table migration_review_items
  add column if not exists internal_owner text,
  add column if not exists due_at timestamptz;

update migration_review_items
set internal_owner = coalesce(nullif(btrim(internal_owner), ''), 'migration_ops'),
    due_at = coalesce(due_at, created_at + interval '7 days')
where internal_owner is null or btrim(internal_owner) = '' or due_at is null;

alter table migration_review_items
  alter column internal_owner set default 'migration_ops',
  alter column internal_owner set not null,
  alter column due_at set default (now() + interval '7 days'),
  alter column due_at set not null;

create index if not exists idx_migration_review_items_operations
  on migration_review_items (project_id, status, internal_owner, due_at, created_at);
