-- Ledger-backed legacy technical-work cutover. Legacy technical rows are canonical provenance
-- after cutover: they are never deleted and cannot be
-- used as writers while canonical_read_only is true.

set local lock_timeout = '5s';
set local statement_timeout = '4min';

alter table content_actions
  add column if not exists canonical_site_fix_id uuid,
  add column if not exists canonical_read_only boolean not null default false,
  add column if not exists legacy_migration_batch_id uuid,
  add column if not exists legacy_migration_disposition text not null default 'none'
    check (legacy_migration_disposition in ('none','migrated','duplicate','review','rolled_back'));

alter table content_actions
  drop constraint if exists content_actions_canonical_cutover_state;
alter table content_actions
  add constraint content_actions_canonical_cutover_state check (
    (canonical_read_only = false and canonical_site_fix_id is null and legacy_migration_batch_id is null
      and legacy_migration_disposition in ('none','rolled_back'))
    or
    (canonical_read_only = true and canonical_site_fix_id is not null and legacy_migration_batch_id is not null
      and legacy_migration_disposition in ('migrated','duplicate'))
    or
    (canonical_read_only = true and canonical_site_fix_id is null and legacy_migration_batch_id is not null
      and legacy_migration_disposition = 'review')
  ) not valid;

alter table seo_opportunities
  add column if not exists canonical_site_fix_id uuid,
  add column if not exists canonical_read_only boolean not null default false,
  add column if not exists legacy_migration_batch_id uuid,
  add column if not exists legacy_migration_disposition text not null default 'none'
    check (legacy_migration_disposition in ('none','migrated','duplicate','review','rolled_back'));

alter table seo_opportunities
  drop constraint if exists seo_opportunities_canonical_cutover_state;
alter table seo_opportunities
  add constraint seo_opportunities_canonical_cutover_state check (
    (canonical_read_only = false and canonical_site_fix_id is null and legacy_migration_batch_id is null
      and legacy_migration_disposition in ('none','rolled_back'))
    or
    (canonical_read_only = true and legacy_migration_batch_id is not null
      and legacy_migration_disposition in ('migrated','duplicate','review'))
  ) not valid;

alter table work_review_memory
  add column if not exists migration_batch_id uuid,
  add column if not exists legacy_review_state_id uuid;

-- A rolled-back alias is immutable history, not the active resolution. Replace
-- the original all-history uniqueness rule with a concurrent active-only index
-- in 0059_05 so rollback -> apply can create a new batch-scoped alias.
do $$
declare legacy_alias_unique text;
begin
  select constraint_name.conname into legacy_alias_unique
  from pg_constraint constraint_name
  where constraint_name.conrelid = 'legacy_object_aliases'::regclass
    and constraint_name.contype = 'u'
    and pg_get_constraintdef(constraint_name.oid) = 'UNIQUE (project_id, legacy_object_type, legacy_object_id)'
  limit 1;
  if legacy_alias_unique is not null then
    execute format('alter table legacy_object_aliases drop constraint %I', legacy_alias_unique);
  end if;
end $$;

alter table content_actions drop constraint if exists content_actions_canonical_site_fix_id_fkey;
alter table content_actions drop constraint if exists content_actions_legacy_migration_batch_id_fkey;
alter table content_actions drop constraint if exists content_actions_canonical_site_fix_project_fk;
alter table content_actions add constraint content_actions_canonical_site_fix_project_fk
  foreign key (project_id, canonical_site_fix_id) references site_fixes(project_id, id)
  on delete restrict not valid;
alter table content_actions drop constraint if exists content_actions_migration_batch_project_fk;
alter table content_actions add constraint content_actions_migration_batch_project_fk
  foreign key (project_id, legacy_migration_batch_id) references migration_batches(project_id, id)
  on delete restrict not valid;

alter table seo_opportunities drop constraint if exists seo_opportunities_canonical_site_fix_id_fkey;
alter table seo_opportunities drop constraint if exists seo_opportunities_legacy_migration_batch_id_fkey;
alter table seo_opportunities drop constraint if exists seo_opportunities_canonical_site_fix_project_fk;
alter table seo_opportunities add constraint seo_opportunities_canonical_site_fix_project_fk
  foreign key (project_id, canonical_site_fix_id) references site_fixes(project_id, id)
  on delete restrict not valid;
alter table seo_opportunities drop constraint if exists seo_opportunities_migration_batch_project_fk;
alter table seo_opportunities add constraint seo_opportunities_migration_batch_project_fk
  foreign key (project_id, legacy_migration_batch_id) references migration_batches(project_id, id)
  on delete restrict not valid;

alter table work_review_memory drop constraint if exists work_review_memory_migration_batch_id_fkey;
alter table work_review_memory drop constraint if exists work_review_memory_legacy_review_state_id_fkey;
alter table work_review_memory drop constraint if exists work_review_memory_migration_batch_project_fk;
alter table work_review_memory add constraint work_review_memory_migration_batch_project_fk
  foreign key (project_id, migration_batch_id) references migration_batches(project_id, id)
  on delete restrict not valid;

alter table content_actions validate constraint content_actions_canonical_cutover_state;
alter table seo_opportunities validate constraint seo_opportunities_canonical_cutover_state;
alter table content_actions validate constraint content_actions_canonical_site_fix_project_fk;
alter table content_actions validate constraint content_actions_migration_batch_project_fk;
alter table seo_opportunities validate constraint seo_opportunities_canonical_site_fix_project_fk;
alter table seo_opportunities validate constraint seo_opportunities_migration_batch_project_fk;
alter table work_review_memory validate constraint work_review_memory_migration_batch_project_fk;

create or replace function enforce_legacy_canonical_read_only()
returns trigger language plpgsql as $$
begin
  if tg_op = 'DELETE' then
    if not exists (select 1 from projects where id = old.project_id) then return old; end if;
    if old.canonical_read_only then
      raise exception 'legacy technical rows are canonical-read-only' using errcode = '55000';
    end if;
    return old;
  end if;
  if old.canonical_read_only and new.canonical_read_only and new is distinct from old then
    raise exception 'legacy technical rows are canonical-read-only' using errcode = '55000';
  end if;
  if old.canonical_read_only and not new.canonical_read_only and not exists (
    select 1 from product_writer_authority pwa where pwa.project_id = old.project_id
      and pwa.product = 'doctor' and pwa.writer_authority = 'canonical' and pwa.write_fenced
      and pwa.fence_token::text = current_setting('citeloop.migration_fence_token', true)
  ) then
    raise exception 'legacy rollback requires the canonical writer fence' using errcode = '55000';
  end if;
  if not old.canonical_read_only and new.canonical_read_only and not exists (
    select 1 from product_writer_authority pwa where pwa.project_id = old.project_id
      and pwa.product = 'doctor' and pwa.writer_authority = 'legacy' and pwa.write_fenced
      and pwa.fence_token::text = current_setting('citeloop.migration_fence_token', true)
  ) then
    raise exception 'legacy cutover requires the legacy writer fence' using errcode = '55000';
  end if;
  return new;
end;
$$;

drop trigger if exists content_actions_canonical_read_only on content_actions;
create trigger content_actions_canonical_read_only
before update or delete on content_actions
for each row execute function enforce_legacy_canonical_read_only();

create or replace function is_legacy_doctor_technical_opportunity(opportunity_type text, evidence jsonb)
returns boolean language sql immutable as $$
  select lower(coalesce(opportunity_type, '')) in (
    'schema_gap','structured_data_missing','json_ld_missing','schema_missing',
    'indexing_anomaly','technical_visibility_issue','robots_blocked','robots_conflict',
    'noindex','noindex_conflict','canonical_missing','canonical_mismatch','canonical_invalid',
    'canonical_multiple','broken_url','soft_404','redirect_loop','redirect_chain',
    'title_missing','missing_title','metadata_title','internal_link_gap','zero_internal_links',
    'broken_internal_link','orphan_page','important_page_missing_from_sitemap','sitemap_update'
  ) or (
    lower(coalesce(opportunity_type, '')) in ('direct_patch','metadata_rewrite')
    and lower(coalesce(evidence->>'work_type', evidence->>'owner', '')) in ('fix_site_issue','doctor')
    and case when jsonb_typeof(coalesce(evidence->'added_propositions', '[]'::jsonb)) = 'array'
      then jsonb_array_length(coalesce(evidence->'added_propositions', '[]'::jsonb)) = 0 else false end
  );
$$;

create or replace function enforce_legacy_technical_writer_authority()
returns trigger language plpgsql as $$
declare
  scoped_project uuid;
  technical boolean := false;
  transition_to_read_only boolean := false;
  transition_to_legacy boolean := false;
  authority text;
  authority_fenced boolean;
  authority_fence_token uuid;
begin
  scoped_project := case when tg_op = 'DELETE' then old.project_id else new.project_id end;
  if tg_op = 'DELETE' and not exists (select 1 from projects where id = scoped_project) then return old; end if;

  if tg_table_name = 'seo_opportunities' then
    technical := case when tg_op = 'DELETE'
      then is_legacy_doctor_technical_opportunity(old.type, old.evidence)
      else is_legacy_doctor_technical_opportunity(new.type, new.evidence)
        or (tg_op = 'UPDATE' and is_legacy_doctor_technical_opportunity(old.type, old.evidence)) end;
  else
    if tg_op = 'DELETE' then
      technical := lower(coalesce(old.asset_type, '')) in ('technical_fix','sitemap_update','schema_patch')
        or lower(coalesce(old.action_type, '')) in ('technical_fix','sitemap_update','schema_patch','technical seo fix task')
        or ((lower(coalesce(old.asset_type, '')) in ('internal_link_patch','metadata_rewrite') or lower(coalesce(old.action_type, '')) in ('internal_link_patch','metadata_rewrite'))
          and coalesce(old.work_type, 'fix_site_issue') = 'fix_site_issue')
        or lower(coalesce(old.output_snapshot->>'output_type', '')) = 'technical_task'
        or lower(coalesce(old.diff_snapshot->>'output_type', '')) = 'technical_task'
        or (lower(coalesce(old.output_snapshot->>'output_type', old.diff_snapshot->>'output_type', '')) = 'direct_patch'
          and coalesce(old.work_type, '') in ('','fix_site_issue')
          and case when jsonb_typeof(coalesce(old.output_snapshot->'added_propositions', old.diff_snapshot->'added_propositions', '[]'::jsonb)) = 'array'
            then jsonb_array_length(coalesce(old.output_snapshot->'added_propositions', old.diff_snapshot->'added_propositions', '[]'::jsonb)) = 0 else false end)
        or exists (select 1 from seo_opportunities opportunity where opportunity.id = old.opportunity_id
          and opportunity.project_id = old.project_id
          and is_legacy_doctor_technical_opportunity(opportunity.type, opportunity.evidence));
    else
      technical := lower(coalesce(new.asset_type, '')) in ('technical_fix','sitemap_update','schema_patch')
        or lower(coalesce(new.action_type, '')) in ('technical_fix','sitemap_update','schema_patch','technical seo fix task')
        or ((lower(coalesce(new.asset_type, '')) in ('internal_link_patch','metadata_rewrite') or lower(coalesce(new.action_type, '')) in ('internal_link_patch','metadata_rewrite'))
          and coalesce(new.work_type, 'fix_site_issue') = 'fix_site_issue')
        or lower(coalesce(new.output_snapshot->>'output_type', '')) = 'technical_task'
        or lower(coalesce(new.diff_snapshot->>'output_type', '')) = 'technical_task'
        or (lower(coalesce(new.output_snapshot->>'output_type', new.diff_snapshot->>'output_type', '')) = 'direct_patch'
          and coalesce(new.work_type, '') in ('','fix_site_issue')
          and case when jsonb_typeof(coalesce(new.output_snapshot->'added_propositions', new.diff_snapshot->'added_propositions', '[]'::jsonb)) = 'array'
            then jsonb_array_length(coalesce(new.output_snapshot->'added_propositions', new.diff_snapshot->'added_propositions', '[]'::jsonb)) = 0 else false end)
        or exists (select 1 from seo_opportunities opportunity where opportunity.id = new.opportunity_id
          and opportunity.project_id = new.project_id
          and is_legacy_doctor_technical_opportunity(opportunity.type, opportunity.evidence));
      if tg_op = 'UPDATE' then
        technical := technical or lower(coalesce(old.asset_type, '')) in ('technical_fix','sitemap_update','schema_patch')
          or lower(coalesce(old.action_type, '')) in ('technical_fix','sitemap_update','schema_patch','technical seo fix task')
          or ((lower(coalesce(old.asset_type, '')) in ('internal_link_patch','metadata_rewrite') or lower(coalesce(old.action_type, '')) in ('internal_link_patch','metadata_rewrite'))
            and coalesce(old.work_type, 'fix_site_issue') = 'fix_site_issue');
      end if;
    end if;
  end if;
  if not technical then
    if tg_op = 'DELETE' then return old; end if;
    return new;
  end if;

  -- SHARE conflicts with the migration transaction's NO KEY UPDATE row lock.
  -- A legacy write that raced the cutover waits, then observes canonical
  -- authority and is rejected instead of escaping the migration snapshot.
  select pwa.writer_authority, pwa.write_fenced, pwa.fence_token
    into authority, authority_fenced, authority_fence_token
  from product_writer_authority pwa
  where pwa.project_id = scoped_project and pwa.product = 'doctor'
  for share;

  if tg_op = 'UPDATE' then
    transition_to_read_only := not old.canonical_read_only and new.canonical_read_only;
    transition_to_legacy := old.canonical_read_only and not new.canonical_read_only;
  end if;
  if transition_to_read_only and authority = 'legacy' and authority_fenced
    and authority_fence_token::text = current_setting('citeloop.migration_fence_token', true)
  then return new; end if;
  if transition_to_legacy and authority = 'canonical' and authority_fenced
    and authority_fence_token::text = current_setting('citeloop.migration_fence_token', true)
  then return new; end if;
  if authority is distinct from 'legacy' or authority_fenced is distinct from false then
    raise exception 'legacy technical writer is not authoritative' using errcode = '55000';
  end if;
  if tg_op = 'DELETE' then return old; end if;
  return new;
end;
$$;

drop trigger if exists content_actions_technical_writer_authority on content_actions;
create trigger content_actions_technical_writer_authority
before insert or update or delete on content_actions
for each row execute function enforce_legacy_technical_writer_authority();

drop trigger if exists seo_opportunities_technical_writer_authority on seo_opportunities;
create trigger seo_opportunities_technical_writer_authority
before insert or update or delete on seo_opportunities
for each row execute function enforce_legacy_technical_writer_authority();

create or replace function enforce_legacy_application_writer_authority()
returns trigger language plpgsql as $$
declare
  old_action_id uuid;
  new_action_id uuid;
  scoped_project uuid;
  action_read_only boolean;
  new_action_read_only boolean;
  authority text;
  authority_fenced boolean;
  authority_fence_token uuid;
begin
  scoped_project := case when tg_op = 'DELETE' then old.project_id else new.project_id end;
  old_action_id := case when tg_op in ('UPDATE','DELETE') then old.content_action_id else null end;
  new_action_id := case when tg_op in ('UPDATE','INSERT') then new.content_action_id else null end;
  if old_action_id is null and new_action_id is null then
    if tg_op = 'DELETE' then return old; end if;
    return new;
  end if;
  select writer.writer_authority, writer.write_fenced, writer.fence_token
    into authority, authority_fenced, authority_fence_token
  from product_writer_authority writer
  where writer.project_id = scoped_project and writer.product = 'doctor'
  for share;
  action_read_only := false;
  if old_action_id is not null then
    select action.canonical_read_only into action_read_only
    from content_actions action
    where action.project_id = scoped_project and action.id = old_action_id
    for share;
  end if;
  if new_action_id is not null and new_action_id is distinct from old_action_id then
    select action.canonical_read_only into new_action_read_only
    from content_actions action
    where action.project_id = scoped_project and action.id = new_action_id
    for share;
    action_read_only := action_read_only or coalesce(new_action_read_only, false);
  end if;
  if action_read_only is distinct from true then
    if tg_op = 'DELETE' then return old; end if;
    return new;
  end if;
  if authority in ('legacy','canonical') and authority_fenced
    and authority_fence_token::text = current_setting('citeloop.migration_fence_token', true)
  then
    if tg_op = 'DELETE' then return old; end if;
    return new;
  end if;
  raise exception 'legacy technical application writer is not authoritative' using errcode = '55000';
end;
$$;

drop trigger if exists site_change_applications_legacy_writer_authority on site_change_applications;
create trigger site_change_applications_legacy_writer_authority
before insert or update or delete on site_change_applications
for each row execute function enforce_legacy_application_writer_authority();

drop trigger if exists seo_opportunities_canonical_read_only on seo_opportunities;
create trigger seo_opportunities_canonical_read_only
before update or delete on seo_opportunities
for each row execute function enforce_legacy_canonical_read_only();

-- Rollback retains canonical site_fixes with status='migration_rolled_back';
-- aliases similarly remain as rolled_back_tombstone provenance.

reset statement_timeout;
reset lock_timeout;
