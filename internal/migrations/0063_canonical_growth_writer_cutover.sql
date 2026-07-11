alter table seo_opportunities
  add column if not exists canonical_growth boolean not null default false;

create unique index if not exists seo_opportunities_project_id_id_key
  on seo_opportunities (project_id, id);

create table if not exists growth_opportunity_work_aliases (
  project_id uuid not null references projects(id) on delete cascade,
  legacy_opportunity_id uuid not null,
  canonical_opportunity_id uuid not null,
  work_signature_id uuid not null,
  disposition text not null check (disposition in ('canonicalized','duplicate')),
  created_at timestamptz not null default now(),
  primary key (project_id, legacy_opportunity_id),
  foreign key (project_id, legacy_opportunity_id)
    references seo_opportunities(project_id, id) on delete restrict,
  foreign key (project_id, canonical_opportunity_id)
    references seo_opportunities(project_id, id) on delete restrict,
  foreign key (project_id, work_signature_id)
    references work_signature_registry(project_id, id) on delete restrict,
  check (disposition <> 'canonicalized' or legacy_opportunity_id = canonical_opportunity_id),
  check (disposition <> 'duplicate' or legacy_opportunity_id <> canonical_opportunity_id)
);

create index if not exists idx_seo_opportunities_legacy_growth_migration
  on seo_opportunities (project_id, created_at, id)
  where canonical_growth = false
    and status in ('open','accepted','converted','snoozed','watching');

create or replace function enforce_legacy_growth_read_only_after_cutover()
returns trigger language plpgsql as $$
declare
  authority text;
begin
  select writer.writer_authority into authority
  from product_writer_authority writer
  where writer.project_id = new.project_id and writer.product = 'opportunities'
  for share;
  if authority is distinct from 'canonical' then return new; end if;
  if is_legacy_doctor_technical_opportunity(new.type, new.evidence) then return new; end if;
  if tg_op = 'INSERT' and new.canonical_growth = false then
    raise exception 'legacy Growth creation is disabled after canonical cutover' using errcode = '55000';
  end if;
  if tg_op = 'UPDATE' and old.canonical_growth = false and new.canonical_growth = false
    and (to_jsonb(new) - array['status','snoozed_until','snooze_reason','unsnoozed_at','updated_at'])
      is distinct from
        (to_jsonb(old) - array['status','snoozed_until','snooze_reason','unsnoozed_at','updated_at'])
  then
    raise exception 'legacy Growth evidence/work identity is read-only after canonical cutover' using errcode = '55000';
  end if;
  return new;
end;
$$;

drop trigger if exists seo_opportunities_legacy_growth_read_only on seo_opportunities;
create trigger seo_opportunities_legacy_growth_read_only
before insert or update on seo_opportunities
for each row execute function enforce_legacy_growth_read_only_after_cutover();

-- Forward Growth generation is canonical after this release. Existing rows
-- remain readable and are lazily reserved through the same arbitration path
-- before they can be mutated as canonical work.
update product_writer_authority
set writer_authority = 'canonical', write_fenced = false,
    fence_token = null, fenced_at = null, updated_at = now()
where product = 'opportunities';
