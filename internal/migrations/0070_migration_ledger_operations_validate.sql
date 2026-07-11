set local lock_timeout = '5s';
set local statement_timeout = '5min';

alter table migration_ledger
  validate constraint migration_ledger_operation_check;
