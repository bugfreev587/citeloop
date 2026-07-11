alter table seo_opportunities
  add column if not exists canonical_growth boolean not null default false;

create table if not exists growth_opportunity_work_aliases (
  project_id uuid not null references projects(id) on delete cascade,
  legacy_opportunity_id uuid not null,
  canonical_object_type text not null default 'seo_opportunity'
    check (canonical_object_type in ('seo_opportunity','site_fix')),
  canonical_opportunity_id uuid,
  canonical_site_fix_id uuid,
  work_signature_id uuid not null,
  disposition text not null check (disposition in ('canonicalized','duplicate','doctor_merge','rolled_back')),
  created_at timestamptz not null default now(),
  primary key (project_id, legacy_opportunity_id),
  foreign key (project_id, legacy_opportunity_id)
    references seo_opportunities(project_id, id) on delete restrict,
  foreign key (project_id, canonical_opportunity_id)
    references seo_opportunities(project_id, id) on delete restrict,
  foreign key (project_id, canonical_site_fix_id)
    references site_fixes(project_id, id) on delete restrict,
  foreign key (project_id, work_signature_id)
    references work_signature_registry(project_id, id) on delete restrict,
  check ((canonical_object_type = 'seo_opportunity' and canonical_opportunity_id is not null and canonical_site_fix_id is null)
      or (canonical_object_type = 'site_fix' and canonical_opportunity_id is null and canonical_site_fix_id is not null)),
  check (disposition <> 'canonicalized' or legacy_opportunity_id = canonical_opportunity_id),
  check (disposition <> 'duplicate' or legacy_opportunity_id <> canonical_opportunity_id),
  check (disposition <> 'doctor_merge' or canonical_object_type = 'site_fix')
);

create table if not exists growth_cutover_sessions (
  batch_id uuid primary key,
  project_id uuid not null references projects(id) on delete cascade,
  fence_token uuid not null,
  status text not null check (status in ('applying','completed','rolled_back')),
  started_at timestamptz not null default now(),
  finished_at timestamptz,
  error text
);

create unique index if not exists uniq_growth_cutover_active_project
  on growth_cutover_sessions (project_id) where status = 'applying';

create table if not exists growth_cutover_session_entries (
  batch_id uuid not null references growth_cutover_sessions(batch_id) on delete cascade,
  project_id uuid not null references projects(id) on delete cascade,
  sequence_number int not null check (sequence_number > 0),
  opportunity_id uuid not null,
  run_id uuid not null,
  candidate_id uuid not null,
  arbitration_decision_id uuid,
  ai_call_id uuid,
  work_signature_id uuid,
  disposition text not null check (disposition in ('materialized','canonicalized','duplicate','doctor_merge')),
  entry_status text not null default 'applying' check (entry_status in ('applying','applied','rolled_back')),
  before_snapshot jsonb not null check (jsonb_typeof(before_snapshot) = 'object'),
  after_snapshot jsonb not null check (jsonb_typeof(after_snapshot) = 'object'),
  inverse_operation jsonb not null check (jsonb_typeof(inverse_operation) = 'object'),
  created_at timestamptz not null default now(),
  primary key (batch_id, sequence_number),
  unique (batch_id, opportunity_id)
);

create or replace function enforce_legacy_growth_read_only_after_cutover()
returns trigger language plpgsql as $$
declare
  authority text;
  authority_fenced boolean;
  authority_fence_token uuid;
begin
  select writer.writer_authority, writer.write_fenced, writer.fence_token
    into authority, authority_fenced, authority_fence_token
  from product_writer_authority writer
  where writer.project_id = new.project_id and writer.product = 'opportunities'
  for share;
  if authority_fenced then
    if authority_fence_token::text is distinct from current_setting('citeloop.growth_cutover_fence_token', true) then
      raise exception 'Growth writer authority is fenced' using errcode = '55000';
    end if;
  elsif authority is distinct from 'canonical' then
    return new;
  end if;
  if is_legacy_doctor_technical_opportunity(new.type, new.evidence) then return new; end if;
  if tg_op = 'INSERT' and new.canonical_growth = false then
    raise exception 'legacy Growth creation is disabled after canonical cutover' using errcode = '55000';
  end if;
  if tg_op = 'UPDATE' and old.canonical_growth = false and new.canonical_growth = false then
    raise exception 'legacy Growth evidence/work identity is read-only after canonical cutover' using errcode = '55000';
  end if;
  return new;
end;
$$;

drop trigger if exists seo_opportunities_legacy_growth_read_only on seo_opportunities;
create trigger seo_opportunities_legacy_growth_read_only
before insert or update on seo_opportunities
for each row execute function enforce_legacy_growth_read_only_after_cutover();

create or replace function reject_duplicate_growth_content_action()
returns trigger language plpgsql as $$
begin
  if exists (
    select 1 from growth_opportunity_work_aliases alias
    where alias.project_id = new.project_id
      and alias.legacy_opportunity_id = new.opportunity_id
      and alias.disposition in ('duplicate','doctor_merge')
  ) then
    raise exception 'content action cannot execute a duplicate Growth opportunity' using errcode = '23514';
  end if;
  return new;
end;
$$;

drop trigger if exists content_actions_reject_duplicate_growth on content_actions;
create trigger content_actions_reject_duplicate_growth
before insert or update of opportunity_id on content_actions
for each row execute function reject_duplicate_growth_content_action();

-- Authority remains legacy until a fenced application cutover has represented
-- every active Growth row, written ledgers, and passed conservation.
