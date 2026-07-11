set local lock_timeout = '5s';
set local statement_timeout = '30s';

alter table migration_ledger
  drop constraint if exists migration_ledger_operation_check;

alter table migration_ledger
  add constraint migration_ledger_operation_check check (operation in (
    'create','map','decision_migrate','repoint','archive_duplicate',
    'authority_switch','rollback','tombstone','mark_canonical_read_only',
    'migration_review','migration_bucket_mutation','validate_conservation'
  )) not valid;
