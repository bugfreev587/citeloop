-- name: CreateAICallRecord :one
insert into ai_call_records
  (project_id, run_id, stage, linked_object_type, linked_object_id,
   provider, model, prompt_version, request_fingerprint, status, parent_call_id, caused_by_call_id,
   attempt_number, provider_called, provider_started_at, finished_at)
select
   sqlc.arg(project_id), sqlc.narg(run_id), sqlc.arg(stage),
   sqlc.arg(linked_object_type), sqlc.arg(linked_object_id),
   sqlc.arg(provider), sqlc.arg(model), sqlc.arg(prompt_version),
   sqlc.arg(request_fingerprint), sqlc.arg(status), sqlc.narg(parent_call_id), sqlc.narg(caused_by_call_id),
   coalesce((
     select parent.attempt_number + 1
     from ai_call_records parent
     where parent.id = sqlc.narg(parent_call_id)
       and parent.project_id = sqlc.arg(project_id)
       and parent.stage = sqlc.arg(stage)
       and parent.linked_object_type = sqlc.arg(linked_object_type)
       and parent.linked_object_id = sqlc.arg(linked_object_id)
       and parent.request_fingerprint = sqlc.arg(request_fingerprint)
   ), 1),
   sqlc.arg(status) not in ('queued', 'skipped'),
   case when sqlc.arg(status) not in ('queued', 'skipped') then now() else null end,
   case when sqlc.arg(status) not in ('queued', 'running') then now() else null end
where sqlc.narg(parent_call_id)::uuid is null or exists (
  select 1 from ai_call_records parent
  where parent.id = sqlc.narg(parent_call_id)
    and parent.project_id = sqlc.arg(project_id)
    and parent.stage = sqlc.arg(stage)
    and parent.linked_object_type = sqlc.arg(linked_object_type)
    and parent.linked_object_id = sqlc.arg(linked_object_id)
    and parent.request_fingerprint = sqlc.arg(request_fingerprint)
)
returning *;

-- name: CreateSkippedAICallRecord :one
insert into ai_call_records
  (project_id, run_id, stage, linked_object_type, linked_object_id,
   provider, model, prompt_version, request_fingerprint, status, error_code,
   parent_call_id, caused_by_call_id, attempt_number, provider_called, provider_started_at, finished_at)
select
   sqlc.arg(project_id), sqlc.narg(run_id), sqlc.arg(stage),
   sqlc.arg(linked_object_type), sqlc.arg(linked_object_id),
   sqlc.arg(provider), sqlc.arg(model), sqlc.arg(prompt_version),
   sqlc.arg(request_fingerprint), 'skipped', sqlc.arg(error_code),
   sqlc.narg(parent_call_id), sqlc.narg(caused_by_call_id),
   coalesce((
     select parent.attempt_number + 1
     from ai_call_records parent
     where parent.id = sqlc.narg(parent_call_id)
       and parent.project_id = sqlc.arg(project_id)
       and parent.stage = sqlc.arg(stage)
       and parent.linked_object_type = sqlc.arg(linked_object_type)
       and parent.linked_object_id = sqlc.arg(linked_object_id)
       and parent.request_fingerprint = sqlc.arg(request_fingerprint)
   ), 1),
   false, null, now()
where sqlc.narg(parent_call_id)::uuid is null or exists (
  select 1 from ai_call_records parent
  where parent.id = sqlc.narg(parent_call_id)
    and parent.project_id = sqlc.arg(project_id)
    and parent.stage = sqlc.arg(stage)
    and parent.linked_object_type = sqlc.arg(linked_object_type)
    and parent.linked_object_id = sqlc.arg(linked_object_id)
    and parent.request_fingerprint = sqlc.arg(request_fingerprint)
)
returning *;

-- name: FinishAICallRecord :one
update ai_call_records set
  status = sqlc.arg(status),
  provider = coalesce(sqlc.narg(resolved_provider), provider),
  model = coalesce(sqlc.narg(resolved_model), model),
  error_code = sqlc.narg(error_code),
  prompt_tokens = case when sqlc.arg(status) = 'skipped' then 0 else sqlc.arg(prompt_tokens) end,
  completion_tokens = case when sqlc.arg(status) = 'skipped' then 0 else sqlc.arg(completion_tokens) end,
  total_tokens = case when sqlc.arg(status) = 'skipped' then 0 else sqlc.arg(total_tokens) end,
  cost_usd = case when sqlc.arg(status) = 'skipped' then 0::numeric else sqlc.arg(cost_usd)::numeric end,
  provider_called = sqlc.arg(status) <> 'skipped',
  provider_started_at = case when sqlc.arg(status) = 'skipped' then null else coalesce(provider_started_at, started_at) end,
  finished_at = now(),
  updated_at = now()
where id = sqlc.arg(id)
  and project_id = sqlc.arg(project_id)
  and status = 'running'
returning *;

-- name: FinishAICallRecordIfRunning :one
update ai_call_records set
  status = 'failed', error_code = sqlc.arg(error_code), finished_at = now(), updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id) and status = 'running'
returning *;

-- name: MarkAICallProviderStarted :one
update ai_call_records set
  status = 'running',
  model = coalesce(sqlc.narg(resolved_model), model),
  provider_called = true,
  provider_started_at = now(),
  updated_at = now()
where id = sqlc.arg(id)
  and project_id = sqlc.arg(project_id)
  and status = 'queued'
returning *;

-- name: FinishQueuedAICallSkipped :one
update ai_call_records set
  status = 'skipped',
  error_code = sqlc.arg(error_code),
  prompt_tokens = 0,
  completion_tokens = 0,
  total_tokens = 0,
  cost_usd = 0,
  provider_called = false,
  provider_started_at = null,
  finished_at = now(),
  updated_at = now()
where id = sqlc.arg(id)
  and project_id = sqlc.arg(project_id)
  and status = 'queued'
returning *;

-- name: ReclassifyAICallRecordOutputFailure :one
update ai_call_records set
  status = 'failed', error_code = sqlc.arg(error_code), updated_at = now()
where id = sqlc.arg(id)
  and project_id = sqlc.arg(project_id)
  and status in ('ok', 'partial')
returning *;

-- name: FinishCanonicalAICallFenced :one
update ai_call_records set
  status = case when status = 'running' or (status = 'queued' and sqlc.arg(status)::text = 'skipped') then sqlc.arg(status)::text else status end,
  error_code = case when status = 'running' or (status = 'queued' and sqlc.arg(status)::text = 'skipped') then sqlc.narg(error_code) else error_code end,
  provider = coalesce(sqlc.narg(resolved_provider), provider),
  model = coalesce(sqlc.narg(resolved_model), model),
  prompt_tokens = case when status = 'skipped' or (status in ('queued', 'running') and sqlc.arg(status)::text = 'skipped') then 0 else sqlc.arg(prompt_tokens) end,
  completion_tokens = case when status = 'skipped' or (status in ('queued', 'running') and sqlc.arg(status)::text = 'skipped') then 0 else sqlc.arg(completion_tokens) end,
  total_tokens = case when status = 'skipped' or (status in ('queued', 'running') and sqlc.arg(status)::text = 'skipped') then 0 else sqlc.arg(total_tokens) end,
  cost_usd = case when status = 'skipped' or (status in ('queued', 'running') and sqlc.arg(status)::text = 'skipped') then 0::numeric else sqlc.arg(cost_usd)::numeric end,
  provider_called = case when status in ('queued', 'running') then sqlc.arg(status)::text <> 'skipped' else provider_called end,
  provider_started_at = case when status in ('queued', 'running') and sqlc.arg(status)::text = 'skipped' then null else provider_started_at end,
  finished_at = coalesce(finished_at, now()),
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: GetAICallRecord :one
select * from ai_call_records where id = sqlc.arg(id) and project_id = sqlc.arg(project_id);

-- name: GetLatestAICallForRequest :one
select * from ai_call_records
where project_id = sqlc.arg(project_id)
  and stage = sqlc.arg(stage)
  and linked_object_type = sqlc.arg(linked_object_type)
  and linked_object_id = sqlc.arg(linked_object_id)
  and request_fingerprint = sqlc.arg(request_fingerprint)
order by attempt_number desc, created_at desc, id desc
limit 1;

-- name: ListAICallsForObject :many
select * from ai_call_records
where project_id = sqlc.arg(project_id)
  and linked_object_type = sqlc.arg(linked_object_type)
  and linked_object_id = sqlc.arg(linked_object_id)
order by created_at, id;

-- name: AggregateAICallsForObject :one
select
  count(*)::bigint as call_count,
  count(*) filter (where status = 'skipped')::bigint as skipped_count,
  count(*) filter (where status in ('failed', 'partial'))::bigint as unsuccessful_count,
  coalesce(sum(prompt_tokens), 0)::bigint as prompt_tokens,
  coalesce(sum(completion_tokens), 0)::bigint as completion_tokens,
  coalesce(sum(total_tokens), 0)::bigint as total_tokens,
  coalesce(sum(cost_usd), 0)::numeric as cost_usd
from ai_call_records
where project_id = sqlc.arg(project_id)
  and linked_object_type = sqlc.arg(linked_object_type)
  and linked_object_id = sqlc.arg(linked_object_id);
