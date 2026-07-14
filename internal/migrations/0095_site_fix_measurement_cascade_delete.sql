-- Keep measurement evidence immutable to direct writes while allowing the
-- database's audited parent-record cascades to honor their foreign keys.

set local lock_timeout = '5s';
set local statement_timeout = '30s';

create or replace function reject_direct_site_fix_measurement_evidence_delete()
returns trigger language plpgsql as $$
declare
  row_data jsonb := to_jsonb(old);
  row_project_id uuid := (row_data->>'project_id')::uuid;
begin
  -- Project deletion is the root hard-delete path for every aggregate table.
  if not exists (select 1 from public.projects project where project.id = row_project_id) then
    return old;
  end if;
  -- Each lower-level cascade is allowed only after its immediate owning row
  -- has actually been deleted. Trigger nesting depth alone is not authority.
  if tg_table_name = 'site_fix_measurements'
    and not exists (
      select 1 from public.site_fixes fix
      where fix.project_id = row_project_id and fix.id = (row_data->>'site_fix_id')::uuid
    ) then
    return old;
  end if;
  if tg_table_name in ('site_fix_measurement_checkpoints','site_fix_measurement_terminal_outcomes')
    and not exists (
      select 1 from public.site_fix_measurements measurement
      where measurement.project_id = row_project_id and measurement.id = (row_data->>'measurement_id')::uuid
    ) then
    return old;
  end if;
  if tg_table_name in ('site_fix_measurement_learnings','site_fix_measurement_quality_records')
    and not exists (
      select 1 from public.site_fix_measurement_terminal_outcomes outcome
      where outcome.project_id = row_project_id
        and outcome.id = (row_data->>'terminal_outcome_id')::uuid
        and outcome.measurement_id = (row_data->>'measurement_id')::uuid
    ) then
    return old;
  end if;
  raise exception 'Site Fix measurement evidence is append-only' using errcode = '23514';
end;
$$;

drop trigger if exists site_fix_measurements_direct_delete_guard on site_fix_measurements;
create trigger site_fix_measurements_direct_delete_guard
before delete on site_fix_measurements
for each row execute function reject_direct_site_fix_measurement_evidence_delete();

drop trigger if exists site_fix_measurement_checkpoints_immutable on site_fix_measurement_checkpoints;
drop trigger if exists site_fix_measurement_checkpoints_immutable_update on site_fix_measurement_checkpoints;
drop trigger if exists site_fix_measurement_checkpoints_direct_delete_guard on site_fix_measurement_checkpoints;
create trigger site_fix_measurement_checkpoints_immutable_update
before update on site_fix_measurement_checkpoints
for each row execute function prevent_site_fix_measurement_checkpoint_mutation();
create trigger site_fix_measurement_checkpoints_direct_delete_guard
before delete on site_fix_measurement_checkpoints
for each row execute function reject_direct_site_fix_measurement_evidence_delete();

drop trigger if exists site_fix_measurement_terminal_outcomes_immutable on site_fix_measurement_terminal_outcomes;
drop trigger if exists site_fix_measurement_terminal_outcomes_immutable_update on site_fix_measurement_terminal_outcomes;
drop trigger if exists site_fix_measurement_terminal_outcomes_direct_delete_guard on site_fix_measurement_terminal_outcomes;
create trigger site_fix_measurement_terminal_outcomes_immutable_update
before update on site_fix_measurement_terminal_outcomes
for each row execute function reject_site_fix_measurement_append_only_mutation();
create trigger site_fix_measurement_terminal_outcomes_direct_delete_guard
before delete on site_fix_measurement_terminal_outcomes
for each row execute function reject_direct_site_fix_measurement_evidence_delete();

drop trigger if exists site_fix_measurement_learnings_immutable on site_fix_measurement_learnings;
drop trigger if exists site_fix_measurement_learnings_immutable_update on site_fix_measurement_learnings;
drop trigger if exists site_fix_measurement_learnings_direct_delete_guard on site_fix_measurement_learnings;
create trigger site_fix_measurement_learnings_immutable_update
before update on site_fix_measurement_learnings
for each row execute function reject_site_fix_measurement_append_only_mutation();
create trigger site_fix_measurement_learnings_direct_delete_guard
before delete on site_fix_measurement_learnings
for each row execute function reject_direct_site_fix_measurement_evidence_delete();

drop trigger if exists site_fix_measurement_quality_records_immutable on site_fix_measurement_quality_records;
drop trigger if exists site_fix_measurement_quality_records_immutable_update on site_fix_measurement_quality_records;
drop trigger if exists site_fix_measurement_quality_records_direct_delete_guard on site_fix_measurement_quality_records;
create trigger site_fix_measurement_quality_records_immutable_update
before update on site_fix_measurement_quality_records
for each row execute function reject_site_fix_measurement_append_only_mutation();
create trigger site_fix_measurement_quality_records_direct_delete_guard
before delete on site_fix_measurement_quality_records
for each row execute function reject_direct_site_fix_measurement_evidence_delete();

reset statement_timeout;
reset lock_timeout;
