set local lock_timeout = '5s';
set local statement_timeout = '30s';

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'seo_doctor_findings_status_check_v2'
      and conrelid = 'seo_doctor_findings'::regclass
  ) then
    alter table seo_doctor_findings
      add constraint seo_doctor_findings_status_check_v2
      check (status in ('active','resolved','dismissed','converted','migration_rolled_back')) not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'seo_doctor_runs_trigger_check_v2'
      and conrelid = 'seo_doctor_runs'::regclass
  ) then
    alter table seo_doctor_runs
      add constraint seo_doctor_runs_trigger_check_v2
      check (trigger in ('onboarding','manual','weekly','post_publish','migration')) not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'discovery_shadow_runs_mode_check_v2'
      and conrelid = 'discovery_shadow_runs'::regclass
  ) then
    alter table discovery_shadow_runs
      add constraint discovery_shadow_runs_mode_check_v2
      check (mode in ('shadow','canonical','migration')) not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'site_fixes_doctor_finding_project_fk'
      and conrelid = 'site_fixes'::regclass
  ) then
    alter table site_fixes
      add constraint site_fixes_doctor_finding_project_fk
      foreign key (project_id, doctor_finding_id, finding_kind)
      references seo_doctor_findings(project_id, id, finding_kind)
      on delete no action deferrable initially deferred not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'site_fixes_candidate_project_fk'
      and conrelid = 'site_fixes'::regclass
  ) then
    alter table site_fixes
      add constraint site_fixes_candidate_project_fk
      foreign key (project_id, candidate_id)
      references discovery_candidates(project_id, id)
      on delete no action deferrable initially deferred not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'site_fixes_work_signature_project_fk'
      and conrelid = 'site_fixes'::regclass
  ) then
    alter table site_fixes
      add constraint site_fixes_work_signature_project_fk
      foreign key (project_id, candidate_id, work_signature_id)
      references work_signature_registry(project_id, candidate_id, id)
      on delete no action deferrable initially deferred not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'site_fixes_supersedes_project_fk'
      and conrelid = 'site_fixes'::regclass
  ) then
    alter table site_fixes
      add constraint site_fixes_supersedes_project_fk
      foreign key (project_id, candidate_id, supersedes_site_fix_id)
      references site_fixes(project_id, candidate_id, id)
      on delete no action deferrable initially deferred not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'site_fix_verifications_site_fix_project_fk'
      and conrelid = 'site_fix_verifications'::regclass
  ) then
    alter table site_fix_verifications
      add constraint site_fix_verifications_site_fix_project_fk
      foreign key (project_id, site_fix_id)
      references site_fixes(project_id, id)
      on delete cascade not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'site_change_applications_site_fix_project_fk'
      and conrelid = 'site_change_applications'::regclass
  ) then
    alter table site_change_applications
      add constraint site_change_applications_site_fix_project_fk
      foreign key (project_id, site_fix_id)
      references site_fixes(project_id, id)
      on delete cascade not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'site_fixes_migration_batch_project_fk'
      and conrelid = 'site_fixes'::regclass
  ) then
    alter table site_fixes
      add constraint site_fixes_migration_batch_project_fk
      foreign key (project_id, migration_batch_id)
      references migration_batches(project_id, id)
      on delete cascade not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'migration_ledger_batch_project_fk'
      and conrelid = 'migration_ledger'::regclass
  ) then
    alter table migration_ledger
      add constraint migration_ledger_batch_project_fk
      foreign key (project_id, migration_batch_id)
      references migration_batches(project_id, id)
      on delete cascade not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'migration_review_items_batch_project_fk'
      and conrelid = 'migration_review_items'::regclass
  ) then
    alter table migration_review_items
      add constraint migration_review_items_batch_project_fk
      foreign key (project_id, migration_batch_id)
      references migration_batches(project_id, id)
      on delete cascade not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'legacy_object_aliases_batch_project_fk'
      and conrelid = 'legacy_object_aliases'::regclass
  ) then
    alter table legacy_object_aliases
      add constraint legacy_object_aliases_batch_project_fk
      foreign key (project_id, migration_batch_id)
      references migration_batches(project_id, id)
      on delete cascade not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'migration_rollback_events_batch_project_fk'
      and conrelid = 'migration_rollback_events'::regclass
  ) then
    alter table migration_rollback_events
      add constraint migration_rollback_events_batch_project_fk
      foreign key (project_id, migration_batch_id)
      references migration_batches(project_id, id)
      on delete cascade not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'migration_rollback_events_ledger_project_fk'
      and conrelid = 'migration_rollback_events'::regclass
  ) then
    alter table migration_rollback_events
      add constraint migration_rollback_events_ledger_project_fk
      foreign key (project_id, migration_batch_id, migration_ledger_id)
      references migration_ledger(project_id, migration_batch_id, id)
      on delete cascade not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'discovery_candidates_shadow_run_restrict_fk'
      and conrelid = 'discovery_candidates'::regclass
  ) then
    alter table discovery_candidates
      add constraint discovery_candidates_shadow_run_restrict_fk
      foreign key (shadow_run_id) references discovery_shadow_runs(id)
      on delete no action deferrable initially deferred not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'work_signature_registry_shadow_run_restrict_fk'
      and conrelid = 'work_signature_registry'::regclass
  ) then
    alter table work_signature_registry
      add constraint work_signature_registry_shadow_run_restrict_fk
      foreign key (shadow_run_id) references discovery_shadow_runs(id)
      on delete no action deferrable initially deferred not valid;
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'work_signature_registry_candidate_project_fk'
      and conrelid = 'work_signature_registry'::regclass
  ) then
    alter table work_signature_registry
      add constraint work_signature_registry_candidate_project_fk
      foreign key (project_id, candidate_id)
      references discovery_candidates(project_id, id)
      on delete no action deferrable initially deferred not valid;
  end if;
end;
$$;

reset statement_timeout;
reset lock_timeout;
