-- name: AcquireOpportunityFindingStage :one
insert into opportunity_finding_stage_checkpoints
  (id, project_id, workflow_event_id, stage, stage_order, request_fingerprint,
   status, attempt_number, owner_token, lease_expires_at, started_at)
values
  (sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(workflow_event_id), sqlc.arg(stage),
   sqlc.arg(stage_order), sqlc.arg(request_fingerprint), 'running', 1,
   sqlc.arg(owner_token), sqlc.arg(lease_expires_at), now())
on conflict (workflow_event_id, stage) do update set
  status = case
    when opportunity_finding_stage_checkpoints.status = 'running'
      and opportunity_finding_stage_checkpoints.lease_expires_at <= now()
    then 'running'
    else opportunity_finding_stage_checkpoints.status
  end,
  attempt_number = case
    when opportunity_finding_stage_checkpoints.status = 'running'
      and opportunity_finding_stage_checkpoints.lease_expires_at <= now()
    then opportunity_finding_stage_checkpoints.attempt_number + 1
    else opportunity_finding_stage_checkpoints.attempt_number
  end,
  owner_token = case
    when opportunity_finding_stage_checkpoints.status = 'running'
      and opportunity_finding_stage_checkpoints.lease_expires_at <= now()
    then excluded.owner_token
    else opportunity_finding_stage_checkpoints.owner_token
  end,
  lease_expires_at = case
    when opportunity_finding_stage_checkpoints.status = 'running'
      and opportunity_finding_stage_checkpoints.lease_expires_at <= now()
    then excluded.lease_expires_at
    else opportunity_finding_stage_checkpoints.lease_expires_at
  end,
  started_at = case
    when opportunity_finding_stage_checkpoints.status = 'running'
      and opportunity_finding_stage_checkpoints.lease_expires_at <= now()
    then now()
    else opportunity_finding_stage_checkpoints.started_at
  end,
  output_summary = case
    when opportunity_finding_stage_checkpoints.status = 'running'
      and opportunity_finding_stage_checkpoints.lease_expires_at <= now()
    then '{}'::jsonb
    else opportunity_finding_stage_checkpoints.output_summary
  end,
  error = case
    when opportunity_finding_stage_checkpoints.status = 'running'
      and opportunity_finding_stage_checkpoints.lease_expires_at <= now()
    then null
    else opportunity_finding_stage_checkpoints.error
  end,
  updated_at = case
    when opportunity_finding_stage_checkpoints.status = 'running'
      and opportunity_finding_stage_checkpoints.lease_expires_at <= now()
    then now()
    else opportunity_finding_stage_checkpoints.updated_at
  end
returning *;

-- name: FinishOpportunityFindingStage :one
with finished as (
  update opportunity_finding_stage_checkpoints set
    status = sqlc.arg(status),
    output_summary = sqlc.arg(output_summary)::jsonb,
    error = sqlc.narg(error),
    lease_expires_at = null,
    finished_at = now(),
    updated_at = now()
  where opportunity_finding_stage_checkpoints.id = sqlc.arg(id)
    and opportunity_finding_stage_checkpoints.project_id = sqlc.arg(project_id)
    and opportunity_finding_stage_checkpoints.workflow_event_id = sqlc.arg(workflow_event_id)
    and opportunity_finding_stage_checkpoints.stage = sqlc.arg(stage)
    and opportunity_finding_stage_checkpoints.status = 'running'
    and opportunity_finding_stage_checkpoints.owner_token = sqlc.arg(owner_token)
    and sqlc.arg(status)::text in ('succeeded','partial','failed','skipped')
  returning *
),
heartbeat as (
  update workflow_events set
    locked_at = now(),
    updated_at = now()
  where id = sqlc.arg(workflow_event_id)
    and project_id = sqlc.arg(project_id)
    and status = 'running'
    and exists (select 1 from finished)
  returning 1
)
select finished.* from finished;

-- name: ListOpportunityFindingStages :many
select * from opportunity_finding_stage_checkpoints
where project_id = sqlc.arg(project_id)
  and workflow_event_id = sqlc.arg(workflow_event_id)
order by stage_order, created_at, id;
