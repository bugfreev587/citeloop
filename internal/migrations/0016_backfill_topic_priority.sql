-- Backfill topics that were created before Strategist priority normalization.
-- Action-sourced topics can recover the SEO opportunity score; older
-- Strategist-only rows get a deterministic positive fallback so the backlog no
-- longer renders as all-zero priority.

with action_priorities as (
  select
    t.id,
    least(100, greatest(1, round(o.priority_score)::int)) as priority
  from topics t
  join content_actions ca on ca.id = t.source_content_action_id
  join seo_opportunities o on o.id = ca.opportunity_id and o.project_id = t.project_id
  where t.priority <= 0
    and o.priority_score > 0
)
update topics t
set priority = action_priorities.priority
from action_priorities
where t.id = action_priorities.id;

with ranked as (
  select
    id,
    greatest(1, 10 - (((row_number() over (partition by project_id order by created_at asc, id)) - 1)::int % 10)) as priority
  from topics
  where priority <= 0
    and status in ('backlog','scheduled','generating')
)
update topics t
set priority = ranked.priority
from ranked
where t.id = ranked.id;
