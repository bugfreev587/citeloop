-- name: InsertGenerationRun :one
insert into generation_runs
  (project_id, agent, input, output, model, tokens, cost_usd, status, error)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
returning *;

-- name: GetGenerationRun :one
select * from generation_runs
where id = $1
  and project_id = $2;

-- name: ListGenerationRuns :many
select * from generation_runs
where project_id = sqlc.arg(project_id)
  and (sqlc.arg(agent)::text = '' or agent = sqlc.arg(agent))
  and (sqlc.arg(status)::text = '' or status = sqlc.arg(status))
  and (sqlc.arg(cursor_created_at)::timestamptz is null or created_at < sqlc.arg(cursor_created_at))
order by created_at desc
limit sqlc.arg(limit_rows);

-- name: MonthlySpend :one
-- Cumulative cost for the current calendar month (cost breaker basis, §5.4).
-- ai_call_records is authoritative for new binaries. generation_runs remains
-- in the sum so rolling old binaries and pre-ledger history are not lost; new
-- binaries intentionally write zero cost to generation_runs.
select (
  coalesce((
    select sum(call.cost_usd) from ai_call_records call
    where call.project_id = sqlc.arg(project_id)
      and call.status not in ('queued', 'skipped')
      and call.started_at >= date_trunc('month', now())
  ), 0)
  + coalesce((
    select sum(run.cost_usd) from generation_runs run
    where run.project_id = sqlc.arg(project_id)
      and run.created_at >= date_trunc('month', now())
  ), 0)
)::numeric;

-- name: RecentRunFailures :one
-- Consecutive failures heuristic for alerting (§5.2/§5.4).
select count(*) from (
  select status from generation_runs
  where project_id = $1 and agent = $2
  order by created_at desc
  limit $3
) recent
where status = 'error';
