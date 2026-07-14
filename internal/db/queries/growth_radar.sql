-- name: GetCachedGrowthSearchEvidence :one
select * from growth_search_evidence
where project_id = sqlc.arg(project_id)
  and request_hash = sqlc.arg(request_hash)
  and expires_at > sqlc.arg(now_at)
order by fetched_at desc
limit 1;

-- name: GetGrowthSearchUsage :one
select
  count(*) filter (where project_id = sqlc.arg(project_id) and fetched_at >= sqlc.arg(now_at) - interval '1 day')::int daily_requests,
  count(*) filter (where project_id = sqlc.arg(project_id) and trigger_kind = 'weekly_rebuild' and fetched_at >= sqlc.arg(now_at) - interval '7 days')::int weekly_rebuild_requests,
  count(*) filter (where project_id = sqlc.arg(project_id) and fetched_at >= sqlc.arg(now_at) - interval '30 days')::int rolling_requests,
  coalesce(sum(request_cost_usd) filter (where project_id = sqlc.arg(project_id) and fetched_at >= sqlc.arg(now_at) - interval '30 days'), 0)::numeric rolling_cost_usd,
  coalesce(sum(request_cost_usd) filter (where fetched_at >= sqlc.arg(now_at) - interval '30 days'), 0)::numeric installation_cost_usd
from growth_search_evidence;

-- name: CreateGrowthSearchEvidence :one
insert into growth_search_evidence (
  project_id, normalized_query, request_hash, result_set_hash, provider,
  provider_order_not_rank, results, synthetic, trigger_kind, request_cost_usd,
  fetched_at, expires_at
) values (
  sqlc.arg(project_id), sqlc.arg(normalized_query), sqlc.arg(request_hash), sqlc.arg(result_set_hash), sqlc.arg(provider),
  true, sqlc.arg(results), sqlc.arg(synthetic), sqlc.arg(trigger_kind), sqlc.arg(request_cost_usd),
  sqlc.arg(fetched_at), sqlc.arg(expires_at)
)
returning *;

-- name: CreateGrowthRadarRun :one
insert into growth_radar_runs (project_id, phase, status, funnel, cost_usd)
values (sqlc.arg(project_id), sqlc.arg(phase), sqlc.arg(status), sqlc.arg(funnel), sqlc.arg(cost_usd))
returning *;

-- name: ListRecentGrowthRadarRuns :many
select * from growth_radar_runs
where project_id = sqlc.arg(project_id)
order by created_at desc
limit sqlc.arg(limit_rows);

-- name: CreateGrowthRadarItem :one
insert into growth_radar_items (run_id, project_id, candidate_identity, disposition, reason, score, scoring_snapshot)
values (sqlc.arg(run_id), sqlc.arg(project_id), sqlc.arg(candidate_identity), sqlc.arg(disposition), sqlc.arg(reason), sqlc.arg(score), sqlc.arg(scoring_snapshot))
on conflict (run_id, candidate_identity) do update set
  disposition = excluded.disposition, reason = excluded.reason, score = excluded.score, scoring_snapshot = excluded.scoring_snapshot
returning *;
