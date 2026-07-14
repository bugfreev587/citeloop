-- name: GetCachedGrowthSearchEvidence :one
select * from growth_search_evidence
where project_id = sqlc.arg(project_id)
  and request_hash = sqlc.arg(request_hash)
  and expires_at > sqlc.arg(now_at)
order by fetched_at desc
limit 1;

-- name: GetGrowthSearchUsage :one
select
  count(*) filter (where project_id = sqlc.arg(project_id) and fetched_at >= sqlc.arg(now_at)::timestamptz - interval '1 day')::int daily_requests,
  count(*) filter (where project_id = sqlc.arg(project_id) and trigger_kind = 'weekly_rebuild' and fetched_at >= sqlc.arg(now_at)::timestamptz - interval '7 days')::int weekly_rebuild_requests,
  count(*) filter (where project_id = sqlc.arg(project_id) and fetched_at >= sqlc.arg(now_at)::timestamptz - interval '30 days')::int rolling_requests,
  coalesce(sum(request_cost_usd) filter (where project_id = sqlc.arg(project_id) and fetched_at >= sqlc.arg(now_at)::timestamptz - interval '30 days'), 0)::numeric rolling_cost_usd,
  coalesce(sum(request_cost_usd) filter (where fetched_at >= sqlc.arg(now_at)::timestamptz - interval '30 days'), 0)::numeric installation_cost_usd
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

-- name: UpdateGrowthRadarRun :one
update growth_radar_runs
set status = sqlc.arg(status), funnel = sqlc.arg(funnel), cost_usd = sqlc.arg(cost_usd)
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: CreateGrowthRadarItem :one
insert into growth_radar_items (run_id, project_id, candidate_identity, disposition, reason, score, scoring_snapshot, evidence)
values (sqlc.arg(run_id), sqlc.arg(project_id), sqlc.arg(candidate_identity), sqlc.arg(disposition), sqlc.arg(reason), sqlc.arg(score), sqlc.arg(scoring_snapshot), sqlc.arg(evidence))
on conflict (run_id, candidate_identity) do update set
  disposition = excluded.disposition, reason = excluded.reason, score = excluded.score, scoring_snapshot = excluded.scoring_snapshot, evidence = excluded.evidence
returning *;

-- name: UpsertGrowthRadarWatchlistItem :one
insert into growth_radar_watchlist (
  project_id, candidate_identity, status, reason, score, scoring_snapshot, evidence,
  evidence_fingerprint, expires_at, last_run_id
) values (
  sqlc.arg(project_id), sqlc.arg(candidate_identity), 'active', sqlc.arg(reason),
  sqlc.arg(score), sqlc.arg(scoring_snapshot), sqlc.arg(evidence), sqlc.arg(evidence_fingerprint),
  sqlc.arg(expires_at), sqlc.arg(last_run_id)
)
on conflict (project_id, candidate_identity) do update set
  status = case
    when growth_radar_watchlist.evidence_fingerprint <> excluded.evidence_fingerprint then 'active'
    else growth_radar_watchlist.status
  end,
  reason = excluded.reason,
  score = excluded.score,
  scoring_snapshot = excluded.scoring_snapshot,
  evidence = excluded.evidence,
  last_seen_at = now(),
  last_evidence_changed_at = case
    when growth_radar_watchlist.evidence_fingerprint <> excluded.evidence_fingerprint then now()
    else growth_radar_watchlist.last_evidence_changed_at
  end,
  expires_at = case
    when growth_radar_watchlist.evidence_fingerprint <> excluded.evidence_fingerprint then excluded.expires_at
    else growth_radar_watchlist.expires_at
  end,
  reopened_count = growth_radar_watchlist.reopened_count + case
    when growth_radar_watchlist.status in ('expired','resolved','dismissed')
     and growth_radar_watchlist.evidence_fingerprint <> excluded.evidence_fingerprint then 1 else 0 end,
  evidence_fingerprint = excluded.evidence_fingerprint,
  last_run_id = excluded.last_run_id
returning *;

-- name: ResolveGrowthRadarWatchlistItem :exec
update growth_radar_watchlist
set status = 'resolved', reason = sqlc.arg(reason), last_seen_at = now(), last_run_id = sqlc.arg(last_run_id)
where project_id = sqlc.arg(project_id) and candidate_identity = sqlc.arg(candidate_identity) and status = 'active';

-- name: ExpireGrowthRadarWatchlist :many
update growth_radar_watchlist
set status = 'expired'
where project_id = sqlc.arg(project_id) and status = 'active' and expires_at <= sqlc.arg(now_at)
returning *;

-- name: ListActiveGrowthRadarWatchlist :many
select * from growth_radar_watchlist
where project_id = sqlc.arg(project_id) and status = 'active' and expires_at > sqlc.arg(now_at)
order by last_evidence_changed_at desc, candidate_identity;

-- name: GetGrowthRadarDemandSnapshot :one
select
  coalesce(sum(impressions) filter (where date >= current_date - 28), 0)::bigint current_impressions,
  coalesce(sum(impressions) filter (where date < current_date - 28 and date >= current_date - 56), 0)::bigint previous_impressions
from search_performance_daily
where project_id = sqlc.arg(project_id)
  and lower(btrim(query)) = lower(btrim(sqlc.arg(query)))
  and date >= current_date - 56;

-- name: CountRecentGrowthSearchEvidenceForQuery :one
select count(*) from growth_search_evidence
where project_id = sqlc.arg(project_id)
  and normalized_query = lower(regexp_replace(btrim(sqlc.arg(query)), '[[:space:]]+', ' ', 'g'))
  and synthetic = false
  and fetched_at >= sqlc.arg(since_at);

-- name: GetGrowthStageSetting :one
select * from growth_stage_settings
where project_id = sqlc.arg(project_id);

-- name: UpsertGrowthStageSetting :one
insert into growth_stage_settings (
  project_id, stage, stage_profile_version, setting_version,
  is_default_unconfirmed, selected_by, selected_at
) values (
  sqlc.arg(project_id), sqlc.arg(stage), sqlc.arg(stage_profile_version), 1,
  false, sqlc.arg(selected_by), now()
)
on conflict (project_id) do update set
  stage = excluded.stage,
  stage_profile_version = excluded.stage_profile_version,
  setting_version = growth_stage_settings.setting_version + 1,
  is_default_unconfirmed = false,
  selected_by = excluded.selected_by,
  selected_at = now(),
  updated_at = now()
where growth_stage_settings.setting_version = sqlc.arg(expected_version)
returning *;

-- name: CreateGrowthStageEvent :one
insert into growth_stage_events (
  project_id, previous_stage, new_stage, previous_profile_version,
  new_profile_version, expected_setting_version, committed_setting_version,
  actor, reason, affected_watchlist_count, rescore_status
) values (
  sqlc.arg(project_id), sqlc.arg(previous_stage), sqlc.arg(new_stage),
  sqlc.arg(previous_profile_version), sqlc.arg(new_profile_version),
  sqlc.arg(expected_setting_version), sqlc.arg(committed_setting_version),
  sqlc.arg(actor), sqlc.arg(reason), sqlc.arg(affected_watchlist_count), 'pending'
)
returning *;

-- name: GetLatestGrowthStageEvent :one
select * from growth_stage_events
where project_id = sqlc.arg(project_id)
order by created_at desc
limit 1;

-- name: UpdateGrowthStageEventStatus :one
update growth_stage_events
set rescore_status = sqlc.arg(rescore_status),
    failure_code = sqlc.arg(failure_code),
    failure_detail = sqlc.arg(failure_detail),
    started_at = case when sqlc.arg(rescore_status) = 'running' and started_at is null then now() else started_at end,
    completed_at = case when sqlc.arg(rescore_status) in ('complete','failed') then now() else completed_at end
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: CountActiveGrowthRadarWatchlist :one
select count(*) from growth_radar_watchlist
where project_id = sqlc.arg(project_id)
  and status = 'active'
  and expires_at > now();

-- name: UpdateGrowthRadarWatchlistStageScore :one
update growth_radar_watchlist
set score = sqlc.arg(score),
    scoring_snapshot = sqlc.arg(scoring_snapshot),
    reason = sqlc.arg(reason),
    last_seen_at = now()
where growth_radar_watchlist.project_id = sqlc.arg(project_id)
  and growth_radar_watchlist.candidate_identity = sqlc.arg(candidate_identity)
  and growth_radar_watchlist.status = 'active'
  and exists (
    select 1 from growth_stage_settings setting
    where setting.project_id = growth_radar_watchlist.project_id
      and setting.setting_version = sqlc.arg(expected_setting_version)
  )
returning *;
