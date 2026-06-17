-- Re-arm automatic drafts that were left in topics.status='generating' without
-- any surviving draft rows. The scheduler only selects backlog/scheduled topics,
-- so these orphaned rows would otherwise stay stuck on "Drafting" forever.
with orphan_topics as (
  select t.id, t.project_id, t.source_content_action_id
  from topics t
  where t.status = 'generating'
    and not exists (
      select 1
      from articles a
      where a.topic_id = t.id
        and a.status <> 'rejected'
    )
),
rearmed_topics as (
  update topics t
  set status = case when scheduled_at is null then 'backlog' else 'scheduled' end
  from orphan_topics o
  where t.id = o.id
  returning t.id, t.project_id, t.source_content_action_id
)
update content_actions ca
set status = 'approved',
    updated_at = now()
from rearmed_topics t
where ca.id = t.source_content_action_id
  and ca.project_id = t.project_id
  and ca.status = 'drafting';

-- Old generated topics sometimes stored opportunity scores (e.g. 80) directly
-- in priority. Convert only out-of-range values to the P1..P10 scale, where P1
-- is highest priority.
update topics
set priority = least(10, greatest(1, ceil((100 - priority::numeric) / 10.0)::integer))
where priority > 10;
