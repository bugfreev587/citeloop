-- Replace source-operation uniqueness with source-effect uniqueness. One legacy
-- source intentionally creates several canonical artifacts in the same batch.
-- The replacement index is built concurrently in 0071; this transaction only
-- takes the bounded metadata lock needed to swap constraints.

set local lock_timeout = '5s';
set local statement_timeout = '30s';

do $$
declare
  narrow_constraint name;
begin
  if not exists (
    select 1
    from pg_class replacement
    join pg_index index_state on index_state.indexrelid = replacement.oid
    where replacement.relnamespace = current_schema()::regnamespace
      and replacement.relname = 'migration_ledger_effect_identity_key'
      and index_state.indrelid = 'migration_ledger'::regclass
      and index_state.indisunique
      and index_state.indisvalid
      and index_state.indisready
      and index_state.indnullsnotdistinct
  ) then
    raise exception 'valid migration ledger effect identity index is required'
      using errcode = '55000';
  end if;

  select constraint_state.conname
    into narrow_constraint
  from pg_constraint constraint_state
  where constraint_state.conrelid = 'migration_ledger'::regclass
    and constraint_state.contype = 'u'
    and regexp_replace(lower(pg_get_constraintdef(constraint_state.oid)), '\s+', ' ', 'g') =
      'unique (migration_batch_id, source_object_type, source_object_id, operation)'
  limit 1;

  if narrow_constraint is not null then
    execute format('alter table migration_ledger drop constraint %I', narrow_constraint);
  end if;

  if not exists (
    select 1 from pg_constraint constraint_state
    where constraint_state.conrelid = 'migration_ledger'::regclass
      and constraint_state.conname = 'migration_ledger_effect_identity_key'
      and constraint_state.contype = 'u'
  ) then
    alter table migration_ledger
      add constraint migration_ledger_effect_identity_key unique using index migration_ledger_effect_identity_key;
  end if;
end $$;
