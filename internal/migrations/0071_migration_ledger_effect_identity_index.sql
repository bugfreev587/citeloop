-- citeloop:migration-mode=nontransactional
-- citeloop:index=migration_ledger_effect_identity_key
create unique index concurrently if not exists migration_ledger_effect_identity_key
  on migration_ledger (migration_batch_id, source_object_type, source_object_id, operation, canonical_object_type, canonical_object_id)
  nulls not distinct;
