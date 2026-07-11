set local lock_timeout = '5s';
set local statement_timeout = '30s';

alter table seo_doctor_findings
  validate constraint seo_doctor_findings_status_check_v2;
alter table seo_doctor_runs
  validate constraint seo_doctor_runs_trigger_check_v2;
alter table discovery_shadow_runs
  validate constraint discovery_shadow_runs_mode_check_v2;

alter table site_fixes
  validate constraint site_fixes_doctor_finding_project_fk;
alter table site_fixes
  validate constraint site_fixes_candidate_project_fk;
alter table site_fixes
  validate constraint site_fixes_work_signature_project_fk;
alter table site_fixes
  validate constraint site_fixes_supersedes_project_fk;
alter table site_fixes
  validate constraint site_fixes_migration_batch_project_fk;
alter table site_fix_verifications
  validate constraint site_fix_verifications_site_fix_project_fk;
alter table site_change_applications
  validate constraint site_change_applications_site_fix_project_fk;
alter table rollback_records
  validate constraint rollback_records_site_fix_project_fk;

alter table migration_ledger
  validate constraint migration_ledger_batch_project_fk;
alter table migration_review_items
  validate constraint migration_review_items_batch_project_fk;
alter table legacy_object_aliases
  validate constraint legacy_object_aliases_batch_project_fk;
alter table migration_rollback_events
  validate constraint migration_rollback_events_batch_project_fk;
alter table migration_rollback_events
  validate constraint migration_rollback_events_ledger_project_fk;

alter table discovery_candidates
  validate constraint discovery_candidates_shadow_run_restrict_fk;
alter table work_signature_registry
  validate constraint work_signature_registry_shadow_run_restrict_fk;
alter table work_signature_registry
  validate constraint work_signature_registry_candidate_project_fk;

alter table site_change_applications
  validate constraint site_change_applications_exactly_one_source;
alter table site_change_applications
  validate constraint site_change_applications_kind_source_consistency;
alter table rollback_records
  validate constraint rollback_records_at_most_one_source;

reset statement_timeout;
reset lock_timeout;
