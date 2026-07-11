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
  candidate_id uuid not null,
  work_signature_id uuid not null,
  disposition text not null check (disposition in ('canonicalized','duplicate')),
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

-- Authority remains legacy until a fenced application cutover has represented
-- every active Growth row, written ledgers, and passed conservation.
