set local lock_timeout = '5s';
set local statement_timeout = '30s';

alter table seo_doctor_findings
  drop constraint if exists seo_doctor_findings_status_check;
alter table seo_doctor_runs
  drop constraint if exists seo_doctor_runs_trigger_check;
alter table discovery_shadow_runs
  drop constraint if exists discovery_shadow_runs_mode_check;

alter table site_fixes
  drop constraint if exists site_fixes_doctor_finding_id_fkey,
  drop constraint if exists site_fixes_candidate_id_fkey,
  drop constraint if exists fk_site_fixes_work_signature,
  drop constraint if exists site_fixes_supersedes_site_fix_id_fkey,
  drop constraint if exists site_fixes_migration_batch_id_fkey;

alter table site_fix_verifications
  drop constraint if exists site_fix_verifications_site_fix_id_fkey;
alter table site_change_applications
  drop constraint if exists site_change_applications_site_fix_id_fkey;
alter table rollback_records
  drop constraint if exists rollback_records_site_fix_id_fkey;

alter table migration_ledger
  drop constraint if exists migration_ledger_migration_batch_id_fkey;
alter table migration_review_items
  drop constraint if exists migration_review_items_migration_batch_id_fkey;
alter table legacy_object_aliases
  drop constraint if exists legacy_object_aliases_migration_batch_id_fkey;
alter table migration_rollback_events
  drop constraint if exists migration_rollback_events_migration_batch_id_fkey,
  drop constraint if exists migration_rollback_events_migration_ledger_id_fkey;

alter table discovery_candidates
  drop constraint if exists discovery_candidates_shadow_run_id_fkey;
alter table work_signature_registry
  drop constraint if exists work_signature_registry_shadow_run_id_fkey,
  drop constraint if exists work_signature_registry_candidate_id_fkey;

reset statement_timeout;
reset lock_timeout;
