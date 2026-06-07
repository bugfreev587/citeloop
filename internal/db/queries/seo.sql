-- name: UpsertSEOProperty :one
insert into seo_properties
  (project_id, site_url, gsc_site_url, ga4_property_id, url_normalization_config, default_country, default_language)
values ($1, $2, $3, $4, $5, $6, $7)
on conflict (project_id, site_url) do update set
  gsc_site_url = excluded.gsc_site_url,
  ga4_property_id = excluded.ga4_property_id,
  url_normalization_config = excluded.url_normalization_config,
  default_country = excluded.default_country,
  default_language = excluded.default_language,
  updated_at = now()
returning *;

-- name: GetSEOPropertyForProject :one
select * from seo_properties
where project_id = $1
order by created_at asc
limit 1;

-- name: UpsertSEOIntegration :one
insert into seo_integrations
  (project_id, provider, status, credential_ref, last_verified_at, last_error)
values ($1, $2, $3, $4, $5, $6)
on conflict (project_id, provider) do update set
  status = excluded.status,
  credential_ref = excluded.credential_ref,
  last_verified_at = excluded.last_verified_at,
  last_error = excluded.last_error,
  updated_at = now()
returning *;

-- name: ListSEOIntegrations :many
select * from seo_integrations
where project_id = $1
order by provider asc;

-- name: InsertSEORun :one
insert into seo_runs
  (project_id, agent, status, started_at, finished_at, cost_usd, input, output, error)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
returning *;

-- name: StartSEORun :one
insert into seo_runs
  (project_id, agent, status, started_at, input)
values ($1, $2, 'degraded', $3, $4)
returning *;

-- name: FinishSEORun :one
update seo_runs set
  status = $3,
  finished_at = $4,
  cost_usd = $5,
  output = $6,
  error = $7
where id = $1 and project_id = $2
returning *;

-- name: ListSEORuns :many
select * from seo_runs
where project_id = sqlc.arg(project_id)
  and (sqlc.arg(agent)::text = '' or agent = sqlc.arg(agent))
  and (sqlc.arg(status)::text = '' or status = sqlc.arg(status))
  and (sqlc.arg(cursor_started_at)::timestamptz is null or started_at < sqlc.arg(cursor_started_at))
order by started_at desc
limit sqlc.arg(limit_rows);

-- name: UpsertPagePerformanceDaily :one
insert into page_performance_daily
  (project_id, property_id, date, page_url, normalized_page_url, article_id, topic_id,
   clicks, impressions, weighted_position, ctr, ga4_sessions, ga4_engaged_sessions,
   ga4_conversions, indexed_state, technical_status, data_source_notes)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
on conflict (project_id, property_id, date, normalized_page_url) do update set
  page_url = excluded.page_url,
  article_id = excluded.article_id,
  topic_id = excluded.topic_id,
  clicks = excluded.clicks,
  impressions = excluded.impressions,
  weighted_position = excluded.weighted_position,
  ctr = excluded.ctr,
  ga4_sessions = excluded.ga4_sessions,
  ga4_engaged_sessions = excluded.ga4_engaged_sessions,
  ga4_conversions = excluded.ga4_conversions,
  indexed_state = excluded.indexed_state,
  technical_status = excluded.technical_status,
  data_source_notes = excluded.data_source_notes,
  updated_at = now()
returning *;

-- name: UpsertTechnicalCheck :one
insert into technical_checks
  (project_id, run_id, page_url, normalized_page_url, article_id, http_status, canonical_status,
   robots_status, title_status, meta_description_status, h1_status, structured_data_status,
   sitemap_status, internal_link_count, outbound_link_count, content_hash, unsafe_mdx_detected, raw_details)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
on conflict (project_id, run_id, normalized_page_url) do update set
  page_url = excluded.page_url,
  article_id = excluded.article_id,
  http_status = excluded.http_status,
  canonical_status = excluded.canonical_status,
  robots_status = excluded.robots_status,
  title_status = excluded.title_status,
  meta_description_status = excluded.meta_description_status,
  h1_status = excluded.h1_status,
  structured_data_status = excluded.structured_data_status,
  sitemap_status = excluded.sitemap_status,
  internal_link_count = excluded.internal_link_count,
  outbound_link_count = excluded.outbound_link_count,
  content_hash = excluded.content_hash,
  unsafe_mdx_detected = excluded.unsafe_mdx_detected,
  raw_details = excluded.raw_details,
  checked_at = now()
returning *;

-- name: UpsertSEOOpportunity :one
insert into seo_opportunities
  (project_id, type, status, priority_score, confidence, page_url, normalized_page_url,
   article_id, topic_id, query, evidence, recommended_action, expected_impact, effort,
   risk_level, created_by_run_id)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
on conflict (project_id, type, normalized_page_url, query, created_by_run_id) do update set
  status = excluded.status,
  priority_score = excluded.priority_score,
  confidence = excluded.confidence,
  page_url = excluded.page_url,
  article_id = excluded.article_id,
  topic_id = excluded.topic_id,
  evidence = excluded.evidence,
  recommended_action = excluded.recommended_action,
  expected_impact = excluded.expected_impact,
  effort = excluded.effort,
  risk_level = excluded.risk_level,
  updated_at = now()
returning *;

-- name: ListSEOOpportunities :many
select * from seo_opportunities
where project_id = sqlc.arg(project_id)
  and (sqlc.arg(type)::text = '' or type = sqlc.arg(type))
  and (sqlc.arg(status)::text = '' or status = sqlc.arg(status))
  and (sqlc.arg(cursor_created_at)::timestamptz is null or created_at < sqlc.arg(cursor_created_at))
order by priority_score desc, created_at desc
limit sqlc.arg(limit_rows);

-- name: GetSEOOpportunity :one
select * from seo_opportunities
where id = $1 and project_id = $2;

-- name: UpdateSEOOpportunityStatus :one
update seo_opportunities set
  status = $3,
  updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: CreateContentAction :one
insert into content_actions
  (project_id, opportunity_id, action_type, status, target_article_id, target_url,
   normalized_target_url, target_content_hash_before, baseline_window, measurement_window)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
on conflict (project_id, opportunity_id, action_type) do update set
  status = excluded.status,
  target_article_id = excluded.target_article_id,
  target_url = excluded.target_url,
  normalized_target_url = excluded.normalized_target_url,
  target_content_hash_before = excluded.target_content_hash_before,
  baseline_window = excluded.baseline_window,
  measurement_window = excluded.measurement_window,
  updated_at = now()
returning *;

-- name: ListContentActions :many
select * from content_actions
where project_id = sqlc.arg(project_id)
  and (sqlc.arg(status)::text = '' or status = sqlc.arg(status))
  and (sqlc.arg(cursor_created_at)::timestamptz is null or created_at < sqlc.arg(cursor_created_at))
order by created_at desc
limit sqlc.arg(limit_rows);

-- name: GetContentAction :one
select * from content_actions
where id = $1 and project_id = $2;

-- name: UpdateContentActionStatus :one
update content_actions set
  status = $3,
  updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: ListPublishedCanonicalArticlesForSEO :many
select * from articles
where project_id = $1
  and kind = 'canonical'
  and status in ('published','pending_url_verification','publish_failed')
  and canonical_url is not null
order by published_at desc nulls last, created_at desc;

-- name: SEOOverviewStats :one
select
  coalesce(sum(clicks) filter (where date >= current_date - interval '28 days'), 0)::numeric as clicks_28d,
  coalesce(sum(impressions) filter (where date >= current_date - interval '28 days'), 0)::numeric as impressions_28d,
  coalesce(avg(ctr) filter (where date >= current_date - interval '28 days'), 0)::numeric as ctr_28d,
  coalesce(avg(weighted_position) filter (where date >= current_date - interval '28 days'), 0)::numeric as position_28d,
  count(distinct date) filter (
    where date >= current_date - interval '28 days'
      and (clicks is not null or impressions is not null)
  )::bigint as gsc_days_28d
from page_performance_daily
where project_id = $1;

-- name: SEOTechnicalSummary :one
select
  count(*)::bigint as checked_urls,
  count(*) filter (where http_status between 200 and 299)::bigint as ok_urls,
  count(*) filter (where http_status is null or http_status < 200 or http_status >= 300)::bigint as anomaly_urls
from technical_checks tc
join seo_runs sr on sr.id = tc.run_id
where tc.project_id = $1
  and sr.started_at = (
    select max(started_at) from seo_runs where project_id = $1 and agent = 'seo_sync'
  );

-- name: SEOOpportunityCounts :many
select type, status, count(*)::bigint as count
from seo_opportunities
where project_id = $1
group by type, status
order by type, status;

-- name: ContentActionCounts :many
select status, count(*)::bigint as count
from content_actions
where project_id = $1
group by status
order by status;
