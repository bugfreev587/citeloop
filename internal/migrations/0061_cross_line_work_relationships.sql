create unique index if not exists work_signature_registry_project_id_id_key
  on work_signature_registry (project_id, id);

create table if not exists work_relationships (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  dependent_candidate_id uuid not null,
  dependent_work_signature_id uuid not null,
  dependent_work_type text not null check (dependent_work_type in ('seo_opportunity','content_action','site_fix')),
  dependent_work_id uuid not null,
  blocking_work_signature_id uuid not null,
  relationship_type text not null check (relationship_type in (
    'blocked_by_doctor_finding','blocked_by_site_fix','provides_evidence_to_opportunity','unblocks_opportunity'
  )),
  dependency_class text not null check (dependency_class in ('hard_blocker','soft_dependency')),
  reason text not null,
  overlapping_mutation_fields jsonb not null default '[]'::jsonb
    check (jsonb_typeof(overlapping_mutation_fields) = 'array'),
  reassessment_trigger text not null,
  attribution_confounder boolean generated always as (dependency_class = 'soft_dependency') stored,
  active boolean not null default true,
  resolved_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint work_relationships_candidate_project_fk
    foreign key (project_id, dependent_candidate_id)
    references discovery_candidates(project_id, id) on delete restrict,
  constraint work_relationships_dependent_signature_project_fk
    foreign key (project_id, dependent_candidate_id, dependent_work_signature_id)
    references work_signature_registry(project_id, candidate_id, id)
    on delete restrict deferrable initially deferred,
  constraint work_relationships_blocking_signature_project_fk
    foreign key (project_id, blocking_work_signature_id)
    references work_signature_registry(project_id, id) on delete restrict,
  check (dependent_work_signature_id <> blocking_work_signature_id),
  check ((active and resolved_at is null) or (not active and resolved_at is not null)),
  unique nulls not distinct (
    project_id, dependent_work_signature_id, blocking_work_signature_id, relationship_type
  )
);

create index if not exists idx_work_relationships_growth_blockers
  on work_relationships (project_id, dependent_work_type, dependent_work_id, dependency_class)
  where active = true;

create index if not exists idx_work_relationships_blocking_signature
  on work_relationships (project_id, blocking_work_signature_id)
  where active = true;

create or replace function enforce_canonical_growth_reservation()
returns trigger language plpgsql as $$
declare
  authority text;
  fenced boolean;
  canonical_growth boolean;
  opportunity_id uuid;
  signature_id uuid;
begin
  select writer.writer_authority, writer.write_fenced
    into authority, fenced
  from product_writer_authority writer
  where writer.project_id = new.project_id and writer.product = 'opportunities'
  for share;

  if fenced then
    raise exception 'Growth writer authority is fenced' using errcode = '55000';
  end if;
  if authority is distinct from 'canonical' then
    return new;
  end if;

  opportunity_id := case when tg_table_name = 'seo_opportunities' then new.id else new.opportunity_id end;
  if tg_table_name = 'seo_opportunities' then
    canonical_growth := new.canonical_growth;
  else
    select opportunity.canonical_growth into canonical_growth
    from seo_opportunities opportunity
    where opportunity.project_id = new.project_id and opportunity.id = opportunity_id;
  end if;
  if canonical_growth is distinct from true then
    return new;
  end if;
  select signature.id into signature_id
  from work_signature_registry signature
  where signature.project_id = new.project_id
    and signature.mode = 'enforced'
    and signature.active = true
    and signature.owner = 'opportunities'
    and signature.reserved_work_type = 'seo_opportunity'
    and signature.reserved_work_id = opportunity_id
  limit 1;
  if signature_id is null then
    raise exception 'canonical Growth work requires an active owner-neutral reservation' using errcode = '23514';
  end if;

  if tg_table_name = 'content_actions' and exists (
    select 1 from work_relationships relationship
    where relationship.project_id = new.project_id
      and relationship.dependent_work_signature_id = signature_id
      and relationship.dependency_class = 'hard_blocker'
      and relationship.active = true
  ) then
    raise exception 'Growth action is blocked by unresolved Doctor work' using errcode = '23514';
  end if;
  return new;
end;
$$;

drop trigger if exists seo_opportunities_canonical_growth_reservation on seo_opportunities;
create constraint trigger seo_opportunities_canonical_growth_reservation
after insert or update on seo_opportunities
deferrable initially deferred
for each row execute function enforce_canonical_growth_reservation();

drop trigger if exists content_actions_canonical_growth_reservation on content_actions;
create constraint trigger content_actions_canonical_growth_reservation
after insert or update on content_actions
deferrable initially deferred
for each row execute function enforce_canonical_growth_reservation();
