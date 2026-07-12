-- This constraint trigger serves two tables with different record shapes.
-- PL/pgSQL can resolve both branches of a CASE record-field expression, so
-- select the table branch before reading its table-specific NEW fields.
create or replace function enforce_canonical_growth_reservation()
returns trigger language plpgsql as $$
declare
  authority text;
  fenced boolean;
  fence_token uuid;
  canonical_growth boolean;
  opportunity_id uuid;
  signature_id uuid;
begin
  select writer.writer_authority, writer.write_fenced, writer.fence_token
    into authority, fenced, fence_token
  from product_writer_authority writer
  where writer.project_id = new.project_id and writer.product = 'opportunities'
  for share;

  if fenced and fence_token::text is distinct from current_setting('citeloop.growth_cutover_fence_token', true) then
    raise exception 'Growth writer authority is fenced' using errcode = '55000';
  end if;
  if authority is distinct from 'canonical' then
    return new;
  end if;

  if tg_table_name = 'seo_opportunities' then
    opportunity_id := new.id;
    canonical_growth := new.canonical_growth;
  elsif tg_table_name = 'content_actions' then
    opportunity_id := new.opportunity_id;
    select opportunity.canonical_growth into canonical_growth
    from seo_opportunities opportunity
    where opportunity.project_id = new.project_id and opportunity.id = opportunity_id;
  else
    raise exception 'unsupported canonical Growth reservation trigger table: %', tg_table_name using errcode = '55000';
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
