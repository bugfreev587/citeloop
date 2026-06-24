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

-- name: UpsertSEOOAuthToken :one
insert into seo_oauth_tokens
  (project_id, provider, encrypted_refresh_token, token_type, scope, access_token_expires_at,
   account_email, authorized_properties, last_error)
values (
  sqlc.arg(project_id),
  sqlc.arg(provider),
  sqlc.arg(encrypted_refresh_token),
  sqlc.arg(token_type),
  sqlc.arg(scope),
  sqlc.narg(access_token_expires_at),
  sqlc.narg(account_email),
  sqlc.arg(authorized_properties)::jsonb,
  sqlc.narg(last_error)
)
on conflict (project_id, provider) do update set
  encrypted_refresh_token = excluded.encrypted_refresh_token,
  token_type = excluded.token_type,
  scope = excluded.scope,
  access_token_expires_at = excluded.access_token_expires_at,
  account_email = excluded.account_email,
  authorized_properties = excluded.authorized_properties,
  last_error = excluded.last_error,
  revoked_at = null,
  updated_at = now()
returning *;

-- name: GetActiveSEOOAuthToken :one
select * from seo_oauth_tokens
where project_id = $1
  and provider = $2
  and revoked_at is null;

-- name: UpdateSEOOAuthSelectedProperty :one
update seo_oauth_tokens set
  selected_property = $3,
  last_error = null,
  updated_at = now()
where project_id = $1
  and provider = $2
  and revoked_at is null
returning *;

-- name: RevokeSEOOAuthToken :one
update seo_oauth_tokens set
  revoked_at = now(),
  updated_at = now()
where project_id = $1
  and provider = $2
  and revoked_at is null
returning *;

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
  article_id = coalesce(excluded.article_id, page_performance_daily.article_id),
  topic_id = coalesce(excluded.topic_id, page_performance_daily.topic_id),
  clicks = coalesce(excluded.clicks, page_performance_daily.clicks),
  impressions = coalesce(excluded.impressions, page_performance_daily.impressions),
  weighted_position = coalesce(excluded.weighted_position, page_performance_daily.weighted_position),
  ctr = coalesce(excluded.ctr, page_performance_daily.ctr),
  ga4_sessions = coalesce(excluded.ga4_sessions, page_performance_daily.ga4_sessions),
  ga4_engaged_sessions = coalesce(excluded.ga4_engaged_sessions, page_performance_daily.ga4_engaged_sessions),
  ga4_conversions = coalesce(excluded.ga4_conversions, page_performance_daily.ga4_conversions),
  indexed_state = coalesce(excluded.indexed_state, page_performance_daily.indexed_state),
  technical_status = coalesce(excluded.technical_status, page_performance_daily.technical_status),
  data_source_notes = page_performance_daily.data_source_notes || excluded.data_source_notes,
  updated_at = now()
returning *;

-- name: UpsertSearchPerformanceDaily :one
insert into search_performance_daily
  (project_id, property_id, date, page_url, normalized_page_url, query, country, device,
   clicks, impressions, ctr, position, query_data_partial, source)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
on conflict (project_id, property_id, date, normalized_page_url, query, country, device) do update set
  page_url = excluded.page_url,
  clicks = excluded.clicks,
  impressions = excluded.impressions,
  ctr = excluded.ctr,
  position = excluded.position,
  query_data_partial = excluded.query_data_partial,
  source = excluded.source,
  updated_at = now()
returning *;

-- name: UpsertSearchAppearanceDaily :one
insert into search_appearance_daily
  (project_id, property_id, date, search_appearance, clicks, impressions, ctr, position, source)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
on conflict (project_id, property_id, date, search_appearance) do update set
  clicks = excluded.clicks,
  impressions = excluded.impressions,
  ctr = excluded.ctr,
  position = excluded.position,
  source = excluded.source,
  updated_at = now()
returning *;

-- name: SEODataDayCount :one
select count(distinct date)::bigint
from page_performance_daily
where project_id = $1
  and property_id = $2
  and (clicks is not null or impressions is not null);

-- name: ListSearchQueryOpportunityRollups :many
select
  max(page_url)::text as page_url,
  normalized_page_url,
  query,
  coalesce(sum(clicks), 0)::numeric as clicks_28d,
  coalesce(sum(impressions), 0)::numeric as impressions_28d,
  case
    when coalesce(sum(impressions), 0) > 0 then coalesce(sum(clicks), 0) / nullif(sum(impressions), 0)
    else 0
  end::numeric as ctr_28d,
  coalesce(avg(position), 0)::numeric as position_28d,
  min(date)::date as window_start,
  max(date)::date as window_end
from search_performance_daily
where project_id = $1
  and property_id = $2
  and date >= current_date - 28
group by normalized_page_url, query
having coalesce(sum(impressions), 0) >= 100
order by impressions_28d desc
limit $3;

-- name: ListPageDecayOpportunityRollups :many
select
  max(page_url)::text as page_url,
  normalized_page_url,
  coalesce(sum(clicks) filter (where date >= current_date - 28), 0)::numeric as current_clicks_28d,
  coalesce(sum(clicks) filter (where date < current_date - 28 and date >= current_date - 56), 0)::numeric as previous_clicks_28d,
  coalesce(sum(impressions) filter (where date >= current_date - 28), 0)::numeric as current_impressions_28d,
  coalesce(sum(impressions) filter (where date < current_date - 28 and date >= current_date - 56), 0)::numeric as previous_impressions_28d,
  (current_date - 28)::date as window_start,
  (current_date - 1)::date as window_end
from page_performance_daily
where project_id = $1
  and property_id = $2
  and date >= current_date - 56
  and clicks is not null
group by normalized_page_url
having coalesce(sum(clicks) filter (where date < current_date - 28 and date >= current_date - 56), 0) >= 10
   and coalesce(sum(clicks) filter (where date >= current_date - 28), 0) <
     coalesce(sum(clicks) filter (where date < current_date - 28 and date >= current_date - 56), 0) * 0.7
order by
  coalesce(sum(clicks) filter (where date < current_date - 28 and date >= current_date - 56), 0) -
  coalesce(sum(clicks) filter (where date >= current_date - 28), 0) desc
limit $3;

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
   risk_level, created_by_run_id, opportunity_key)
values (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
  encode(digest(
    $1::text || '|' || $2 || '|' || coalesce($7, '') || '|' || coalesce($10, '') || '|' ||
    coalesce(($11::jsonb)->>'intent_type', '') || '|' ||
    coalesce(($11::jsonb)->>'engine', '') || '|' ||
    coalesce(($11::jsonb)->>'evidence_window', '') || '|' ||
    coalesce(($11::jsonb)->>'reason', ''),
    'sha256'
  ), 'hex')
)
on conflict (project_id, opportunity_key) where status in ('open','accepted','converted') do update set
  status = case
    when seo_opportunities.status in ('accepted','converted') then seo_opportunities.status
    else excluded.status
  end,
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

-- name: CountOpenSEOOpportunities :one
select count(*)::bigint from seo_opportunities
where project_id = $1
  and status = 'open';

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

-- name: MarkContentActionVerification :one
update content_actions set
  status = sqlc.arg(status)::text,
  verified_at = sqlc.narg(verified_at)::timestamptz,
  verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: MarkContentActionDraftReady :one
update content_actions set
  status = 'ready_for_review',
  draft_article_id = $3,
  updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: MarkContentActionMeasuringForDraftArticle :one
update content_actions set
  status = 'measuring',
  published_at = now(),
  updated_at = now()
where project_id = $1 and draft_article_id = $2
returning *;

-- name: UpdateContentActionExecutionMetadata :one
update content_actions set
  asset_type = coalesce(sqlc.narg(asset_type)::text, asset_type),
  target_surface_id = coalesce(sqlc.narg(target_surface_id)::uuid, target_surface_id),
  risk_reasons = sqlc.arg(risk_reasons)::jsonb,
  evidence_snapshot = sqlc.arg(evidence_snapshot)::jsonb,
  input_snapshot = sqlc.arg(input_snapshot)::jsonb,
  output_snapshot = sqlc.arg(output_snapshot)::jsonb,
  diff_snapshot = sqlc.arg(diff_snapshot)::jsonb,
  review_required = sqlc.arg(review_required)::boolean,
  approved_by = coalesce(sqlc.narg(approved_by)::text, approved_by),
  approved_at = coalesce(sqlc.narg(approved_at)::timestamptz, approved_at),
  verified_at = coalesce(sqlc.narg(verified_at)::timestamptz, verified_at),
  verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: ListUnplannedContentActions :many
select ca.* from content_actions ca
left join topics t
  on t.source_content_action_id = ca.id
where ca.project_id = $1
  and ca.status = 'ready_for_review'
  and t.id is null
order by ca.created_at asc
limit $2
for update of ca skip locked;

-- name: UpsertSEOAssetType :one
insert into seo_asset_types
  (key, name, description, default_risk_level, default_measurement_window_days,
   supported_publication_surfaces, requires_evidence, requires_review_by_default, default_generation_path)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
on conflict (key) do update set
  name = excluded.name,
  description = excluded.description,
  default_risk_level = excluded.default_risk_level,
  default_measurement_window_days = excluded.default_measurement_window_days,
  supported_publication_surfaces = excluded.supported_publication_surfaces,
  requires_evidence = excluded.requires_evidence,
  requires_review_by_default = excluded.requires_review_by_default,
  default_generation_path = excluded.default_generation_path,
  updated_at = now()
returning *;

-- name: ListSEOAssetTypes :many
select * from seo_asset_types
order by key asc;

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
