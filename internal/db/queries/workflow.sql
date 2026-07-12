-- name: EnqueueWorkflowEvent :one
insert into workflow_events
  (project_id, event_type, entity_type, entity_id, dedupe_key, payload, run_after)
values ($1, $2, sqlc.narg(entity_type), sqlc.narg(entity_id), $3, $4, coalesce(sqlc.narg(run_after), now()))
on conflict (dedupe_key) do update set
  payload = workflow_events.payload,
  updated_at = workflow_events.updated_at
returning *;

-- name: ClaimPendingWorkflowEvents :many
with candidates as (
  select id
  from workflow_events
  where status = 'pending'
    and run_after <= now()
  order by created_at asc
  limit $1
  for update skip locked
)
update workflow_events set
  status = 'running',
  attempts = attempts + 1,
  locked_at = now(),
  updated_at = now(),
  error = null
where id in (select id from candidates)
returning *;

-- name: ReclaimStuckWorkflowEvents :many
with candidates as (
  select id
  from workflow_events
  where status = 'running'
    and locked_at < now() - interval '30 minutes'
  order by locked_at asc
  limit $1
  for update skip locked
)
update workflow_events set
  status = 'pending',
  run_after = now(),
  locked_at = null,
  error = coalesce(error, 'reclaimed after worker timeout'),
  updated_at = now()
where id in (select id from candidates)
returning *;

-- name: MarkWorkflowEventSucceeded :one
update workflow_events set
  status = 'succeeded',
  processed_at = now(),
  locked_at = null,
  error = null,
  updated_at = now()
where id = sqlc.arg(id)
  and status = 'running'
  and attempts = sqlc.arg(expected_attempts)
returning *;

-- name: MarkWorkflowEventFailed :one
update workflow_events set
  status = case when attempts >= 4 then 'dead' else 'pending' end,
  run_after = case
    when attempts >= 4 then run_after
    when attempts = 1 then now() + interval '1 minute'
    when attempts = 2 then now() + interval '5 minutes'
    else now() + interval '30 minutes'
  end,
  locked_at = null,
  error = sqlc.arg(error),
  updated_at = now()
where id = sqlc.arg(id)
  and status = 'running'
  and attempts = sqlc.arg(expected_attempts)
returning *;

-- name: MarkWorkflowEventDead :one
update workflow_events set
  status = 'dead',
  locked_at = null,
  processed_at = now(),
  error = sqlc.arg(error),
  updated_at = now()
where id = sqlc.arg(id)
  and status = 'running'
  and attempts = sqlc.arg(expected_attempts)
returning *;

-- name: RetryWorkflowEvent :one
update workflow_events set
  status = 'pending',
  run_after = now(),
  locked_at = null,
  error = null,
  updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: ListDeadWorkflowEventsForProject :many
select * from workflow_events
where project_id = $1
  and status = 'dead'
order by updated_at desc
limit $2;

-- name: LatestOpportunityFindingWorkflowEvent :one
select * from workflow_events
where project_id = $1
  and event_type = 'opportunity_finding.requested'
order by created_at desc, id desc
limit 1;

-- name: ActiveOpportunityFindingWorkflowEvent :one
select * from workflow_events
where project_id = $1
  and event_type = 'opportunity_finding.requested'
  and status in ('pending','running')
order by created_at desc, id desc
limit 1;
