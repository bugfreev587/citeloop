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
