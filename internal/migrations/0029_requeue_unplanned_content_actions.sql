-- Requeue accepted Analysis actions that were created while the workflow waited
-- for every open recommendation to be reviewed. Those actions have no source
-- topic yet, so Content Plan can otherwise remain empty indefinitely.
with unplanned_actions as (
  select
    ca.project_id,
    count(*) as action_count,
    min(ca.id::text) as first_action_id
  from content_actions ca
  left join topics t
    on t.project_id = ca.project_id
    and t.source_content_action_id = ca.id
  where ca.status = 'ready_for_review'
    and t.id is null
  group by ca.project_id
)
insert into workflow_events
  (project_id, event_type, entity_type, entity_id, dedupe_key, payload)
select
  project_id,
  'opportunity.batch_completed',
  'project',
  project_id,
  'opportunity.batch_completed:' || project_id::text || ':migration-0029:' || first_action_id,
  jsonb_build_object(
    'source', 'migration_0029',
    'unplanned_actions', action_count
  )
from unplanned_actions
on conflict (dedupe_key) do nothing;
