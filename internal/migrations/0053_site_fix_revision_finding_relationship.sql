set local lock_timeout = '5s';
set local statement_timeout = '30s';

-- A revision owns a new immutable candidate snapshot. Its predecessor must
-- remain in the same project and Doctor finding, but need not share candidate.
do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'site_fixes_project_finding_id_key'
      and conrelid = 'site_fixes'::regclass
  ) then
    alter table site_fixes
      add constraint site_fixes_project_finding_id_key unique (project_id, doctor_finding_id, id);
  end if;
end;
$$;

do $$
begin
  if not exists (
    select 1 from pg_constraint
    where conname = 'site_fixes_supersedes_finding_project_fk'
      and conrelid = 'site_fixes'::regclass
  ) then
    alter table site_fixes
      add constraint site_fixes_supersedes_finding_project_fk
      foreign key (project_id, doctor_finding_id, supersedes_site_fix_id)
      references site_fixes(project_id, doctor_finding_id, id)
      on delete no action deferrable initially deferred not valid;
  end if;
end;
$$;

alter table site_fixes
  validate constraint site_fixes_supersedes_finding_project_fk;

do $$
begin
  if exists (
    select 1 from pg_constraint
    where conname = 'site_fixes_supersedes_project_fk'
      and conrelid = 'site_fixes'::regclass
  ) then
    alter table site_fixes
      drop constraint if exists site_fixes_supersedes_project_fk;
  end if;
end;
$$;

reset statement_timeout;
reset lock_timeout;
