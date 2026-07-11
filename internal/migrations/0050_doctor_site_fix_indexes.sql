set local lock_timeout = '5s';
set local statement_timeout = '30s';

create index if not exists idx_migration_batches_project_created
  on migration_batches (project_id, created_at desc);

create unique index if not exists uniq_active_site_fix_work_signature
  on site_fixes (project_id, work_signature_id)
  where status in (
    'proposed','approved','preparing','ready_to_apply','applying','awaiting_deploy',
    'verifying','failed_retryable','reopened'
  );

create unique index if not exists uniq_site_fixes_root_candidate
  on site_fixes (project_id, candidate_id)
  where supersedes_site_fix_id is null;

create unique index if not exists uniq_site_fixes_superseded_predecessor
  on site_fixes (project_id, supersedes_site_fix_id)
  where supersedes_site_fix_id is not null;

create index if not exists idx_site_fixes_project_status
  on site_fixes (project_id, status, updated_at desc);

create index if not exists idx_site_fixes_doctor_finding
  on site_fixes (project_id, doctor_finding_id, finding_kind, created_at desc);

create index if not exists idx_site_fixes_candidate
  on site_fixes (project_id, candidate_id);

create index if not exists idx_site_fixes_work_signature
  on site_fixes (project_id, candidate_id, work_signature_id);

create index if not exists idx_site_fixes_migration_batch
  on site_fixes (project_id, migration_batch_id)
  where migration_batch_id is not null;

create index if not exists idx_site_fix_verifications_fix_attempt
  on site_fix_verifications (project_id, site_fix_id, attempt_number desc);

create index if not exists idx_migration_ledger_project_source
  on migration_ledger (project_id, source_object_type, source_object_id, created_at desc);

create index if not exists idx_migration_ledger_canonical
  on migration_ledger (project_id, canonical_object_type, canonical_object_id)
  where canonical_object_id is not null;

create index if not exists idx_migration_ledger_batch
  on migration_ledger (project_id, migration_batch_id);

create index if not exists idx_migration_rollback_events_batch_age
  on migration_rollback_events (project_id, migration_batch_id, occurred_at);

create index if not exists idx_migration_rollback_events_ledger
  on migration_rollback_events (project_id, migration_batch_id, migration_ledger_id, occurred_at)
  where migration_ledger_id is not null;

create index if not exists idx_migration_review_items_project_status
  on migration_review_items (project_id, status, created_at);

create index if not exists idx_migration_review_items_batch
  on migration_review_items (project_id, migration_batch_id);

create index if not exists idx_legacy_object_aliases_canonical
  on legacy_object_aliases (project_id, canonical_object_type, canonical_object_id);

create index if not exists idx_legacy_object_aliases_batch
  on legacy_object_aliases (project_id, migration_batch_id);

reset statement_timeout;
reset lock_timeout;
