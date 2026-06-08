-- name: StartGEORun :one
insert into geo_runs (project_id, agent, status, provider, started_at, input)
values ($1, $2, 'degraded', $3, $4, $5)
returning *;

-- name: FinishGEORun :one
update geo_runs set
  status = $3,
  finished_at = $4,
  output = $5,
  error = $6,
  cost_usd = $7
where id = $1 and project_id = $2
returning *;

-- name: ListGEORuns :many
select * from geo_runs
where project_id = sqlc.arg(project_id)
  and (sqlc.arg(agent)::text = '' or agent = sqlc.arg(agent))
  and (sqlc.arg(status)::text = '' or status = sqlc.arg(status))
  and (sqlc.arg(cursor_started_at)::timestamptz is null or started_at < sqlc.arg(cursor_started_at))
order by started_at desc
limit sqlc.arg(limit_rows);

-- name: UpsertAICrawlerAccessSnapshot :one
insert into ai_crawler_access_snapshots
  (project_id, run_id, page_url, normalized_page_url, target_user_agent, probe_user_agent,
   evidence_type, robots_state, http_status, access_state, confidence, inferred,
   meta_robots_state, sitemap_state, body_extractable, raw_details, checked_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
on conflict (project_id, run_id, normalized_page_url, target_user_agent, evidence_type) do update set
  page_url = excluded.page_url,
  probe_user_agent = excluded.probe_user_agent,
  robots_state = excluded.robots_state,
  http_status = excluded.http_status,
  access_state = excluded.access_state,
  confidence = excluded.confidence,
  inferred = excluded.inferred,
  meta_robots_state = excluded.meta_robots_state,
  sitemap_state = excluded.sitemap_state,
  body_extractable = excluded.body_extractable,
  raw_details = excluded.raw_details,
  checked_at = excluded.checked_at
returning *;

-- name: ListLatestAICrawlerAccessSnapshots :many
select s.*
from ai_crawler_access_snapshots s
join geo_runs r on r.id = s.run_id
where s.project_id = $1
  and r.started_at = (
    select max(started_at)
    from geo_runs
    where project_id = $1 and agent = 'geo_crawler_audit'
  )
order by s.normalized_page_url asc, s.target_user_agent asc, s.evidence_type asc;

-- name: UpsertCrawlerAccessOpportunity :one
with updated as (
  update seo_opportunities so set
    priority_score = $4,
    confidence = $5,
    page_url = $6,
    normalized_page_url = $7,
    evidence = so.evidence || $8,
    recommended_action = $9,
    expected_impact = $10,
    effort = $11,
    risk_level = $12,
    updated_at = now()
  where so.project_id = $1
    and so.type = $2
    and so.status in ('open','accepted','converted')
    and so.normalized_page_url = $7
    and coalesce(so.query, '') = ''
  returning *
), inserted as (
  insert into seo_opportunities
    (project_id, type, status, priority_score, confidence, page_url, normalized_page_url,
     query, evidence, recommended_action, expected_impact, effort, risk_level, created_by_run_id)
  select $1, $2, $3, $4, $5, $6, $7, null, $8, $9, $10, $11, $12, null
  where not exists (select 1 from updated)
  returning *
)
select * from updated
union all
select * from inserted;
