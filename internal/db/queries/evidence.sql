-- name: AcquireEvidenceRun :one
with acquired as (
insert into evidence_runs (
  id, project_id, source, normalized_target, target_kind, window_start, window_end,
  collection_spec, collection_spec_fingerprint, collection_owner_token, requested_by,
  attempt_number, lease_expires_at, status, started_at
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(source), sqlc.arg(normalized_target),
  sqlc.arg(target_kind), sqlc.narg(window_start)::date, sqlc.narg(window_end)::date,
  sqlc.arg(collection_spec)::jsonb, sqlc.arg(collection_spec_fingerprint),
  sqlc.arg(collection_owner_token), sqlc.arg(requested_by)::jsonb,
  sqlc.arg(attempt_number), sqlc.arg(lease_expires_at), 'running', sqlc.arg(started_at)
)
on conflict (
  project_id, source, normalized_target, target_kind,
  (coalesce(window_start, '-infinity'::date)),
  (coalesce(window_end, 'infinity'::date)),
  collection_spec_fingerprint
) do update set
  requested_by = (
    select coalesce(jsonb_agg(requester order by requester), '[]'::jsonb)
    from (
      select distinct jsonb_array_elements_text(evidence_runs.requested_by || excluded.requested_by) as requester
    ) requesters
  ),
  collection_owner_token = case
    when evidence_runs.status in ('failed','partial') or (evidence_runs.status = 'running' and evidence_runs.lease_expires_at <= now())
      then excluded.collection_owner_token else evidence_runs.collection_owner_token end,
  attempt_number = case
    when evidence_runs.status in ('failed','partial') or (evidence_runs.status = 'running' and evidence_runs.lease_expires_at <= now())
      then evidence_runs.attempt_number + 1 else evidence_runs.attempt_number end,
  lease_expires_at = case
    when evidence_runs.status in ('failed','partial') or (evidence_runs.status = 'running' and evidence_runs.lease_expires_at <= now())
      then excluded.lease_expires_at else evidence_runs.lease_expires_at end,
  error_history = case
    when (evidence_runs.status in ('failed','partial') or (evidence_runs.status = 'running' and evidence_runs.lease_expires_at <= now()))
      and evidence_runs.error_summary is not null
      then evidence_runs.error_history || jsonb_build_array(jsonb_build_object('attempt', evidence_runs.attempt_number, 'status', evidence_runs.status, 'error', evidence_runs.error_summary, 'finished_at', evidence_runs.finished_at))
    else evidence_runs.error_history end,
  status = case
    when evidence_runs.status in ('failed','partial') or (evidence_runs.status = 'running' and evidence_runs.lease_expires_at <= now())
      then 'running' else evidence_runs.status end,
  error_summary = case
    when evidence_runs.status in ('failed','partial') or (evidence_runs.status = 'running' and evidence_runs.lease_expires_at <= now())
      then null else evidence_runs.error_summary end,
  finished_at = case
    when evidence_runs.status in ('failed','partial') or (evidence_runs.status = 'running' and evidence_runs.lease_expires_at <= now())
      then null else evidence_runs.finished_at end,
  started_at = case
    when evidence_runs.status in ('failed','partial') or (evidence_runs.status = 'running' and evidence_runs.lease_expires_at <= now())
      then excluded.started_at else evidence_runs.started_at end,
  updated_at = now()
returning evidence_runs.*
), expired_attempt as (
  update evidence_run_attempts attempt set
    status = 'failed', error_summary = 'collection lease expired before terminalization',
    finished_at = now()
  from acquired
  where attempt.run_id = acquired.id and attempt.project_id = acquired.project_id
    and attempt.attempt_number < acquired.attempt_number and attempt.status = 'running'
  returning attempt.run_id
), ensured_attempt as (
  insert into evidence_run_attempts (
    run_id, project_id, attempt_number, collection_owner_token, status,
    started_at, lease_expires_at
  )
  select id, project_id, attempt_number, collection_owner_token, 'running', started_at, lease_expires_at
  from acquired
  on conflict (run_id, attempt_number) do nothing
  returning run_id
)
select acquired.* from acquired
left join ensured_attempt on ensured_attempt.run_id = acquired.id
left join expired_attempt on expired_attempt.run_id = acquired.id
limit 1;

-- name: CreateEvidenceObservation :one
insert into evidence_observations (
  id, project_id, run_id, attempt_number, source, source_observation_key, normalized_target, target_kind,
  evidence_state, facts, raw_snapshot, confidence, completeness, provider, model,
  provider_version, prompt_version, call_status, prompt_tokens, completion_tokens, total_tokens, cost_usd,
  privacy_state, permission_state, error_code, observed_at, window_start, window_end
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(run_id), sqlc.arg(attempt_number), sqlc.arg(source),
  sqlc.arg(source_observation_key), sqlc.arg(normalized_target), sqlc.arg(target_kind),
  sqlc.arg(evidence_state), sqlc.arg(facts)::jsonb, sqlc.arg(raw_snapshot)::jsonb,
  sqlc.arg(confidence), sqlc.arg(completeness), sqlc.narg(provider), sqlc.narg(model),
  sqlc.narg(provider_version), sqlc.narg(prompt_version), sqlc.narg(call_status), sqlc.arg(prompt_tokens),
  sqlc.arg(completion_tokens), sqlc.arg(total_tokens), sqlc.arg(cost_usd),
  sqlc.arg(privacy_state), sqlc.arg(permission_state), sqlc.narg(error_code),
  sqlc.arg(observed_at), sqlc.narg(window_start)::date, sqlc.narg(window_end)::date
)
on conflict do nothing
returning *;

-- name: GetEvidenceObservation :one
select * from evidence_observations
where project_id = sqlc.arg(project_id) and run_id = sqlc.arg(run_id)
  and attempt_number = sqlc.arg(attempt_number)
  and source_observation_key = sqlc.arg(source_observation_key);

-- name: ListEvidenceObservations :many
select * from evidence_observations
where project_id = sqlc.arg(project_id) and run_id = sqlc.arg(run_id)
  and attempt_number = sqlc.arg(attempt_number)
order by created_at, id;

-- name: FinishEvidenceRun :one
with locked_run as (
  select run.* from evidence_runs run
  where run.id = sqlc.arg(id) and run.project_id = sqlc.arg(project_id)
  for update
), finished_attempt as (
  update evidence_run_attempts attempt set
    status = sqlc.arg(status), error_summary = sqlc.narg(error_summary),
    finished_at = sqlc.arg(finished_at)
  from locked_run
  where attempt.run_id = locked_run.id and attempt.project_id = locked_run.project_id
    and attempt.attempt_number = sqlc.arg(attempt_number)
    and attempt.collection_owner_token = sqlc.arg(collection_owner_token)
    and attempt.status = 'running'
  returning *
), finished_run as (
  update evidence_runs run set
    status = finished_attempt.status, error_summary = finished_attempt.error_summary,
    finished_at = finished_attempt.finished_at, updated_at = now()
  from finished_attempt
  where run.id = finished_attempt.run_id and run.project_id = finished_attempt.project_id
    and run.attempt_number = finished_attempt.attempt_number
    and run.collection_owner_token = finished_attempt.collection_owner_token
  returning run.*
)
select * from finished_run;

-- name: GetEvidenceRun :one
select * from evidence_runs where id = sqlc.arg(id) and project_id = sqlc.arg(project_id);

-- name: ListEvidenceRuns :many
select * from evidence_runs where project_id = sqlc.arg(project_id)
order by created_at desc limit sqlc.arg(limit_rows);

-- name: LinkEvidenceConsumption :one
insert into evidence_consumptions (project_id, evidence_run_id, attempt_number, consumer_type, consumer_id)
values (sqlc.arg(project_id), sqlc.arg(evidence_run_id), sqlc.arg(attempt_number), sqlc.arg(consumer_type), sqlc.arg(consumer_id))
on conflict (project_id, evidence_run_id, attempt_number, consumer_type, consumer_id) do update
set consumer_id = excluded.consumer_id
returning *;

-- name: ListEvidenceRunsForConsumers :many
select distinct run.*, attempt.attempt_number as consumed_attempt_number,
  attempt.status as consumed_status, attempt.error_summary as consumed_error_summary,
  attempt.started_at as consumed_started_at, attempt.finished_at as consumed_finished_at
from evidence_consumptions consumption
join evidence_runs run on run.id = consumption.evidence_run_id and run.project_id = consumption.project_id
join evidence_run_attempts attempt
  on attempt.run_id = consumption.evidence_run_id and attempt.project_id = consumption.project_id
 and attempt.attempt_number = consumption.attempt_number
where consumption.project_id = sqlc.arg(project_id)
  and consumption.consumer_type = sqlc.arg(consumer_type)
  and consumption.consumer_id = any(sqlc.arg(consumer_ids)::uuid[])
order by run.created_at, run.id;
