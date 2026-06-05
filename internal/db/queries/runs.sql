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
select coalesce(sum(cost_usd), 0)::numeric from generation_runs
where project_id = $1
  and status = 'ok'
  and created_at >= date_trunc('month', now());

-- name: RecentRunFailures :one
-- Consecutive failures heuristic for alerting (§5.2/§5.4).
select count(*) from (
  select status from generation_runs
  where project_id = $1 and agent = $2
  order by created_at desc
  limit $3
) recent
where status = 'error';
