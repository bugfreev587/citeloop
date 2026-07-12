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

-- name: ListDoctorPagePriorityInputs :many
select
  max(page_url)::text as page_url,
  normalized_page_url,
  coalesce(sum(clicks), 0)::numeric as gsc_clicks_28d,
  coalesce(sum(impressions), 0)::numeric as gsc_impressions_28d,
  coalesce(sum(ga4_sessions), 0)::numeric as ga4_sessions_28d,
  coalesce(sum(ga4_engaged_sessions), 0)::numeric as ga4_engaged_sessions_28d,
  coalesce(sum(ga4_conversions), 0)::numeric as ga4_key_events_28d,
  max(date)::date as evidence_fresh_through
from page_performance_daily
where project_id = sqlc.arg(project_id)
  and date >= current_date - 28
  and (
    clicks is not null or impressions is not null or ga4_sessions is not null
    or ga4_engaged_sessions is not null or ga4_conversions is not null
  )
group by normalized_page_url
order by
  coalesce(sum(impressions), 0) + coalesce(sum(ga4_sessions), 0) desc,
  normalized_page_url asc
limit least(greatest(sqlc.arg(limit_rows)::int, 1), 50);

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

-- name: CreateSEODoctorRun :one
insert into seo_doctor_runs
  (project_id, trigger, status, stage, progress_percent, message, input_snapshot, created_by_user_id, started_at)
values (
  sqlc.arg(project_id),
  sqlc.arg(trigger),
  sqlc.arg(status),
  sqlc.arg(stage),
  sqlc.arg(progress_percent),
  sqlc.arg(message),
  sqlc.arg(input_snapshot)::jsonb,
  sqlc.narg(created_by_user_id),
  sqlc.narg(started_at)
)
returning *;

-- name: GetSEODoctorRun :one
select * from seo_doctor_runs
where id = $1 and project_id = $2;

-- name: GetActiveSEODoctorRun :one
select * from seo_doctor_runs
where project_id = $1
  and status in ('queued','running')
order by created_at desc
limit 1;

-- name: UpdateSEODoctorRunProgress :one
update seo_doctor_runs set
  status = sqlc.arg(status),
  stage = sqlc.arg(stage),
  progress_percent = sqlc.arg(progress_percent),
  message = sqlc.arg(message),
  block_reason = sqlc.narg(block_reason),
  pages_discovered = sqlc.arg(pages_discovered),
  pages_fetched = sqlc.arg(pages_fetched),
  pages_checked = sqlc.arg(pages_checked),
  issues_found = sqlc.arg(issues_found),
  started_at = coalesce(started_at, sqlc.narg(started_at)),
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: CompleteSEODoctorRun :one
update seo_doctor_runs set
  status = 'completed',
  stage = 'completed',
  progress_percent = 100,
  message = sqlc.arg(message),
  block_reason = null,
  pages_discovered = sqlc.arg(pages_discovered),
  pages_fetched = sqlc.arg(pages_fetched),
  pages_checked = sqlc.arg(pages_checked),
  issues_found = sqlc.arg(issues_found),
  health_score = sqlc.arg(health_score),
  output_summary = sqlc.arg(output_summary)::jsonb,
  healthy_coverage = sqlc.arg(healthy_coverage)::jsonb,
  error = null,
  updated_at = now(),
  finished_at = sqlc.arg(finished_at)
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: FailSEODoctorRun :one
update seo_doctor_runs set
  status = sqlc.arg(status),
  stage = sqlc.arg(stage),
  progress_percent = sqlc.arg(progress_percent),
  message = sqlc.arg(message),
  block_reason = sqlc.narg(block_reason),
  error = sqlc.narg(error),
  output_summary = sqlc.arg(output_summary)::jsonb,
  updated_at = now(),
  finished_at = sqlc.arg(finished_at)
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: LatestSEODoctorRun :one
select * from seo_doctor_runs
where project_id = $1
order by created_at desc
limit 1;

-- name: LatestCompletedSEODoctorRun :one
select * from seo_doctor_runs
where project_id = $1
  and status = 'completed'
order by finished_at desc nulls last, updated_at desc
limit 1;

-- name: CountManualSEODoctorRunsSince :one
select count(*)::bigint from seo_doctor_runs
where project_id = $1
  and trigger = 'manual'
  and created_at >= $2;

-- name: ListSEODoctorRunsDueWeekly :many
select p.* from projects p
where not exists (
  select 1
  from seo_doctor_runs r
  where r.project_id = p.id
    and r.status = 'completed'
    and r.trigger in ('onboarding','manual','weekly','post_publish')
    and coalesce(r.finished_at, r.updated_at, r.created_at) >= now() - interval '6 days'
)
order by p.created_at asc;

-- name: UpsertSEODoctorFinding :one
insert into seo_doctor_findings
  (project_id, run_id, finding_key, severity, category, issue_type, status,
   affected_urls, normalized_urls, evidence, why_it_matters, fix_intent,
   developer_instructions, likely_files_or_surfaces, acceptance_tests,
   risk_level, review_required, autofix_eligible, linked_opportunity_id,
   linked_content_action_id, first_seen_at, last_seen_at, finding_kind)
values (
  sqlc.arg(project_id),
  sqlc.arg(run_id),
  sqlc.arg(finding_key),
  sqlc.arg(severity),
  sqlc.arg(category),
  sqlc.arg(issue_type),
  'active',
  sqlc.arg(affected_urls)::jsonb,
  sqlc.arg(normalized_urls)::jsonb,
  sqlc.arg(evidence)::jsonb,
  sqlc.arg(why_it_matters),
  sqlc.arg(fix_intent),
  sqlc.arg(developer_instructions),
  sqlc.arg(likely_files_or_surfaces)::jsonb,
  sqlc.arg(acceptance_tests)::jsonb,
  sqlc.arg(risk_level),
  sqlc.arg(review_required),
  sqlc.arg(autofix_eligible),
  sqlc.narg(linked_opportunity_id),
  sqlc.narg(linked_content_action_id),
  sqlc.arg(seen_at),
  sqlc.arg(seen_at),
  sqlc.arg(finding_kind)
)
on conflict (project_id, finding_key) where status = 'active' do update set
  run_id = excluded.run_id,
  severity = excluded.severity,
  category = excluded.category,
  issue_type = excluded.issue_type,
  finding_kind = excluded.finding_kind,
  affected_urls = excluded.affected_urls,
  normalized_urls = excluded.normalized_urls,
  evidence = excluded.evidence,
  why_it_matters = excluded.why_it_matters,
  fix_intent = excluded.fix_intent,
  developer_instructions = excluded.developer_instructions,
  likely_files_or_surfaces = excluded.likely_files_or_surfaces,
  acceptance_tests = excluded.acceptance_tests,
  risk_level = excluded.risk_level,
  review_required = excluded.review_required,
  autofix_eligible = excluded.autofix_eligible,
  linked_opportunity_id = coalesce(excluded.linked_opportunity_id, seo_doctor_findings.linked_opportunity_id),
  linked_content_action_id = coalesce(excluded.linked_content_action_id, seo_doctor_findings.linked_content_action_id),
  last_seen_at = excluded.last_seen_at,
  resolved_at = null,
  updated_at = now()
returning *;

-- name: ResolveMissingSEODoctorFindings :exec
update seo_doctor_findings set
  status = 'resolved',
  resolved_at = sqlc.arg(resolved_at),
  updated_at = now()
where project_id = sqlc.arg(project_id)
  and status = 'active'
  and run_id <> sqlc.arg(run_id)
  and not (finding_key = any(sqlc.arg(active_keys)::text[]))
  and (
    issue_type not in ('important_page_missing_from_sitemap', 'geo_crawler_access_blocked')
    or (
      issue_type = 'important_page_missing_from_sitemap'
      and exists (
        select 1 from jsonb_array_elements_text(normalized_urls) scoped_url
        where scoped_url.value = any(sqlc.arg(assessed_sitemap_urls)::text[])
      )
    )
    or (
      issue_type = 'geo_crawler_access_blocked'
      and exists (
        select 1 from jsonb_array_elements_text(normalized_urls) scoped_url
        where scoped_url.value || chr(10) || lower(btrim(coalesce(evidence->>'target_user_agent', ''))) = any(sqlc.arg(assessed_geo_scopes)::text[])
      )
    )
  );

-- name: ListSEODoctorFindingsForRun :many
select * from seo_doctor_findings
where project_id = $1
  and run_id = $2
  and issue_type <> 'no_active_technical_blockers'
order by
  case severity
    when 'P0' then 0
    when 'P1' then 1
    when 'P2' then 2
    else 3
  end,
  updated_at desc;

-- name: ListCurrentSEODoctorFindings :many
select * from seo_doctor_findings
where project_id = $1
  and (run_id = $2 or status = 'active')
  and issue_type <> 'no_active_technical_blockers'
order by
  case severity
    when 'P0' then 0
    when 'P1' then 1
    when 'P2' then 2
    else 3
  end,
  updated_at desc;

-- name: GetSEODoctorFinding :one
select * from seo_doctor_findings
where id = $1 and project_id = $2;

-- name: DismissSEODoctorFinding :one
update seo_doctor_findings set
  status = 'dismissed',
  updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: LinkSEODoctorFindingToAction :one
update seo_doctor_findings set
  status = 'converted',
  linked_opportunity_id = sqlc.arg(linked_opportunity_id),
  linked_content_action_id = sqlc.arg(linked_content_action_id),
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: ListLatestTechnicalChecks :many
select tc.*
from technical_checks tc
join seo_runs sr on sr.id = tc.run_id
where tc.project_id = sqlc.arg(project_id)
  and sr.agent = 'seo_sync'
  and sr.started_at = (
    select max(started_at)
    from seo_runs
    where project_id = sqlc.arg(project_id)
      and agent = 'seo_sync'
  )
order by tc.checked_at desc
limit sqlc.arg(limit_rows);

-- name: CountNewestTechnicalCheckRun :one
with latest as (
  select latest_run.id
  from seo_runs latest_run
  where latest_run.project_id = sqlc.arg(scoped_project_id) and latest_run.agent = 'seo_sync'
  order by latest_run.started_at desc, latest_run.id desc
  limit 1
)
select coalesce((select id::text from latest), '')::text as run_id,
       count(tc.id)::bigint as check_count,
       count(tc.id) filter (where
         tc.http_status is null
         or tc.raw_details ? 'error'
         or coalesce(tc.raw_details->>'crawl_status', '') in ('partial', 'unchecked', 'skipped')
       )::bigint as incomplete_check_count,
       coalesce(max(sr.status), '')::text as run_status
from seo_runs sr
left join technical_checks tc on tc.run_id = sr.id and tc.project_id = sr.project_id
where sr.id = (select id from latest);

-- name: ListNewestTechnicalCheckRunPage :many
select tc.* from technical_checks tc
where tc.project_id = sqlc.arg(project_id)
  and tc.run_id = sqlc.arg(run_id)
order by tc.normalized_page_url, tc.id
limit sqlc.arg(limit_rows) offset sqlc.arg(offset_rows);

-- name: UpsertSEOOpportunity :one
with opportunity_input as (
  select
    sqlc.arg(project_id)::uuid as project_id,
    sqlc.arg(type)::text as type,
    sqlc.arg(status)::text as status,
    sqlc.arg(priority_score)::numeric as priority_score,
    sqlc.arg(confidence)::numeric as confidence,
    sqlc.narg(page_url)::text as page_url,
    sqlc.arg(normalized_page_url)::text as normalized_page_url,
    sqlc.narg(article_id)::uuid as article_id,
    sqlc.narg(topic_id)::uuid as topic_id,
    sqlc.narg(query)::text as query,
    sqlc.arg(evidence)::jsonb as evidence,
    sqlc.narg(recommended_action)::text as recommended_action,
    sqlc.narg(expected_impact)::text as expected_impact,
    sqlc.arg(effort)::int as effort,
    sqlc.arg(risk_level)::text as risk_level,
    sqlc.narg(created_by_run_id)::uuid as created_by_run_id,
    encode(digest(
      sqlc.arg(project_id)::uuid::text || '|' ||
      sqlc.arg(type)::text || '|' ||
      coalesce(sqlc.arg(normalized_page_url)::text, '') || '|' ||
      coalesce(sqlc.narg(query)::text, '') || '|' ||
      coalesce((sqlc.arg(evidence)::jsonb)->>'intent_type', '') || '|' ||
      coalesce((sqlc.arg(evidence)::jsonb)->>'engine', ''),
      'sha256'
    ), 'hex') as opportunity_identity_key,
    encode(digest(
      coalesce((sqlc.arg(evidence)::jsonb)->>'evidence_window', '') || '|' ||
      coalesce((sqlc.arg(evidence)::jsonb)->>'reason', '') || '|' ||
      coalesce((sqlc.arg(evidence)::jsonb)->>'severity', '') || '|' ||
      coalesce((sqlc.arg(evidence)::jsonb)->>'issue_type', '') || '|' ||
      coalesce((sqlc.arg(evidence)::jsonb)->>'status', '') || '|' ||
      coalesce((sqlc.arg(evidence)::jsonb)->>'title_status', '') || '|' ||
      coalesce((sqlc.arg(evidence)::jsonb)->>'meta_description_status', '') || '|' ||
      coalesce(sqlc.arg(priority_score)::numeric::text, '') || '|' ||
      coalesce(sqlc.arg(confidence)::numeric::text, '') || '|' ||
      coalesce(sqlc.arg(risk_level)::text, ''),
      'sha256'
    ), 'hex') as evidence_fingerprint
),
upserted as (
  insert into seo_opportunities
    (project_id, type, status, priority_score, confidence, page_url, normalized_page_url,
     article_id, topic_id, query, evidence, recommended_action, expected_impact, effort,
     risk_level, created_by_run_id, opportunity_key, opportunity_identity_key, evidence_fingerprint)
  select
    project_id, type, status, priority_score, confidence, page_url, normalized_page_url,
    article_id, topic_id, query, evidence, recommended_action, expected_impact, effort,
    risk_level, created_by_run_id, opportunity_identity_key, opportunity_identity_key, evidence_fingerprint
  from opportunity_input
  on conflict (project_id, opportunity_identity_key) where status in ('open','accepted','converted','dismissed','snoozed','watching') do update set
    status = case
      when seo_opportunities.status in ('accepted','converted') then seo_opportunities.status
      when seo_opportunities.status in ('dismissed','snoozed','watching')
        and seo_opportunities.evidence_fingerprint = excluded.evidence_fingerprint then seo_opportunities.status
      else excluded.status
    end,
    priority_score = excluded.priority_score,
    confidence = excluded.confidence,
    page_url = excluded.page_url,
    normalized_page_url = excluded.normalized_page_url,
    article_id = excluded.article_id,
    topic_id = excluded.topic_id,
    query = excluded.query,
    evidence = case
      when seo_opportunities.status in ('dismissed','snoozed','watching')
        and seo_opportunities.evidence_fingerprint <> excluded.evidence_fingerprint
      then excluded.evidence || jsonb_build_object(
        'previously_reviewed_status', seo_opportunities.status,
        'previous_evidence_fingerprint', seo_opportunities.evidence_fingerprint,
        'reopened_reason', 'Evidence fingerprint changed since the opportunity was reviewed'
      )
      else excluded.evidence
    end,
    recommended_action = excluded.recommended_action,
    expected_impact = excluded.expected_impact,
    effort = excluded.effort,
    risk_level = excluded.risk_level,
    created_by_run_id = excluded.created_by_run_id,
    opportunity_key = excluded.opportunity_key,
    evidence_fingerprint = excluded.evidence_fingerprint,
    snoozed_until = case
      when seo_opportunities.status in ('dismissed','snoozed','watching')
        and seo_opportunities.evidence_fingerprint <> excluded.evidence_fingerprint then null
      else seo_opportunities.snoozed_until
    end,
    snooze_reason = case
      when seo_opportunities.status in ('dismissed','snoozed','watching')
        and seo_opportunities.evidence_fingerprint <> excluded.evidence_fingerprint then null
      else seo_opportunities.snooze_reason
    end,
    unsnoozed_at = case
      when seo_opportunities.status = 'snoozed'
        and seo_opportunities.evidence_fingerprint <> excluded.evidence_fingerprint then now()
      else seo_opportunities.unsnoozed_at
    end,
    updated_at = now()
  returning *
),
reopened_review_state as (
  update seo_opportunity_review_states rs set
    reopened_at = now(),
    reopened_reason = 'Evidence fingerprint changed since the opportunity was reviewed',
    material_change_metadata = jsonb_build_object(
      'previous_fingerprint', rs.evidence_fingerprint,
      'current_fingerprint', upserted.evidence_fingerprint
    ),
    updated_at = now()
  from upserted
  where rs.project_id = upserted.project_id
    and rs.opportunity_identity_key = upserted.opportunity_identity_key
    and rs.review_status in ('dismissed','snoozed','watching')
    and rs.evidence_fingerprint <> upserted.evidence_fingerprint
    and upserted.status = 'open'
  returning rs.id
)
select * from upserted;

-- name: CreateCanonicalGrowthOpportunity :one
insert into seo_opportunities
  (id, project_id, type, status, priority_score, confidence, page_url,
   normalized_page_url, article_id, topic_id, query, evidence,
   recommended_action, expected_impact, effort, risk_level, created_by_run_id,
   opportunity_key, opportunity_identity_key, evidence_fingerprint, canonical_growth,
   growth_spec_state, growth_spec_version, growth_spec_origin, growth_spec,
   growth_spec_missing, decision_ready_at)
values
  (sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(type), 'open',
   sqlc.arg(priority_score), sqlc.arg(confidence), sqlc.narg(page_url),
   sqlc.arg(normalized_page_url), sqlc.narg(article_id), sqlc.narg(topic_id),
   sqlc.narg(query), sqlc.arg(evidence)::jsonb, sqlc.narg(recommended_action),
   sqlc.narg(expected_impact), sqlc.arg(effort), sqlc.arg(risk_level),
   sqlc.narg(created_by_run_id), sqlc.arg(exact_signature_hash),
   sqlc.arg(exact_signature_hash), sqlc.arg(evidence_fingerprint), true,
   sqlc.arg(growth_spec_state), sqlc.arg(growth_spec_version), 'forward',
   sqlc.arg(growth_spec)::jsonb, sqlc.arg(growth_spec_missing)::jsonb,
   sqlc.arg(decision_ready_at))
returning *;

-- name: MergeCanonicalGrowthOpportunityEvidence :one
with locked_signature as (
  select signature.id, signature.reserved_work_id
  from work_signature_registry signature
  where signature.project_id = sqlc.arg(project_id)
    and signature.id = sqlc.arg(work_signature_id)
    and signature.mode = 'enforced' and signature.active = true
    and signature.owner = 'opportunities'
    and signature.reserved_work_type = 'seo_opportunity'
  for update
), merged as (
  update seo_opportunities opportunity set
    evidence = opportunity.evidence || jsonb_build_object(
      'merged_cross_source_evidence',
      coalesce(opportunity.evidence->'merged_cross_source_evidence', '[]'::jsonb)
        || jsonb_build_array(sqlc.arg(evidence)::jsonb)
    ),
    priority_score = greatest(opportunity.priority_score, sqlc.arg(incoming_priority_score)::numeric),
    confidence = greatest(opportunity.confidence, sqlc.arg(incoming_confidence)::numeric),
    evidence_fingerprint = sqlc.arg(evidence_fingerprint),
    growth_spec_state = case when sqlc.arg(growth_spec_state)::text = 'decision_ready' then sqlc.arg(growth_spec_state)::text else opportunity.growth_spec_state end,
    growth_spec_version = case when sqlc.arg(growth_spec_state)::text = 'decision_ready' then sqlc.arg(growth_spec_version)::text else opportunity.growth_spec_version end,
    growth_spec_origin = case when sqlc.arg(growth_spec_state)::text = 'decision_ready' then 'forward' else opportunity.growth_spec_origin end,
    growth_spec = case when sqlc.arg(growth_spec_state)::text = 'decision_ready' then sqlc.arg(growth_spec)::jsonb else opportunity.growth_spec end,
    growth_spec_missing = case when sqlc.arg(growth_spec_state)::text = 'decision_ready' then sqlc.arg(growth_spec_missing)::jsonb else opportunity.growth_spec_missing end,
    decision_ready_at = case when sqlc.arg(growth_spec_state)::text = 'decision_ready'
      then coalesce(opportunity.decision_ready_at, sqlc.arg(decision_ready_at)::timestamptz)
      else opportunity.decision_ready_at end,
    updated_at = now()
  from locked_signature signature
  where opportunity.project_id = sqlc.arg(project_id)
    and opportunity.id = signature.reserved_work_id
  returning opportunity.*
), updated_signature as (
  update work_signature_registry signature set
    evidence_fingerprint = sqlc.arg(evidence_fingerprint), updated_at = now()
  from locked_signature
  where signature.project_id = sqlc.arg(project_id)
    and signature.id = locked_signature.id
  returning signature.id
)
select merged.* from merged, updated_signature;

-- name: GetCanonicalGrowthOpportunityByWorkSignatureForUpdate :one
select opportunity.* from work_signature_registry signature
join seo_opportunities opportunity
  on opportunity.project_id = signature.project_id and opportunity.id = signature.reserved_work_id
where signature.project_id = sqlc.arg(project_id)
  and signature.id = sqlc.arg(work_signature_id)
  and signature.mode = 'enforced' and signature.active = true
  and signature.owner = 'opportunities' and signature.reserved_work_type = 'seo_opportunity'
for update of opportunity;

-- name: GetWorkSignatureForGrowthEvidenceMergeForUpdate :one
select * from work_signature_registry
where project_id = sqlc.arg(project_id) and id = sqlc.arg(work_signature_id)
  and mode = 'enforced' and active = true
for update;

-- name: ListSEOOpportunities :many
select seo_opportunities.* from seo_opportunities
where seo_opportunities.project_id = sqlc.arg(project_id)
  and not exists (
    select 1 from growth_opportunity_work_aliases alias
    where alias.project_id = seo_opportunities.project_id
      and alias.legacy_opportunity_id = seo_opportunities.id
      and alias.disposition in ('duplicate','doctor_merge')
  )
  and (sqlc.arg(type)::text = '' or seo_opportunities.type = sqlc.arg(type))
  and (sqlc.arg(status)::text = '' or seo_opportunities.status = sqlc.arg(status))
  and (sqlc.arg(cursor_created_at)::timestamptz is null or seo_opportunities.created_at < sqlc.arg(cursor_created_at))
order by seo_opportunities.priority_score desc, seo_opportunities.created_at desc
limit sqlc.arg(limit_rows);

-- name: GetSEOOpportunity :one
select seo_opportunities.* from seo_opportunities
where seo_opportunities.id = $1 and seo_opportunities.project_id = $2
  and not exists (
    select 1 from growth_opportunity_work_aliases alias
    where alias.project_id = seo_opportunities.project_id
      and alias.legacy_opportunity_id = seo_opportunities.id
      and alias.disposition in ('duplicate','doctor_merge')
  );

-- name: LockSEOOpportunityForGrowthReserve :one
select * from seo_opportunities
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
  and status in ('open','accepted','converted','snoozed','watching')
for update;

-- name: ListActiveLegacyGrowthOpportunities :many
select opportunity.* from seo_opportunities opportunity
where opportunity.project_id = sqlc.arg(project_id)
  and opportunity.canonical_growth = false
  and opportunity.canonical_read_only = false
  and not is_legacy_doctor_technical_opportunity(opportunity.type, opportunity.evidence)
  and not exists (
    select 1 from growth_opportunity_work_aliases alias
    where alias.project_id = opportunity.project_id
      and alias.legacy_opportunity_id = opportunity.id
      and alias.disposition in ('canonicalized','duplicate','doctor_merge')
  )
  and opportunity.status in ('open','accepted','converted','snoozed','watching')
order by opportunity.created_at, opportunity.id
limit sqlc.arg(limit_rows);

-- name: MarkLegacyGrowthOpportunityCanonical :one
update seo_opportunities set
  canonical_growth = true,
  evidence = sqlc.arg(evidence)::jsonb,
  evidence_fingerprint = sqlc.arg(evidence_fingerprint),
  updated_at = now()
where project_id = sqlc.arg(project_id) and id = sqlc.arg(id)
  and canonical_growth = false and canonical_read_only = false
returning *;

-- name: GetLegacyGrowthIntendedTarget :one
select
  coalesce(opportunity.evidence->>'intended_slug_or_canonical', ''::text)::text as evidence_intended,
  opportunity.page_url as opportunity_page_url,
  coalesce(action.id, '00000000-0000-0000-0000-000000000000'::uuid) as action_id,
  action.updated_at as action_updated_at,
  action.target_url as action_target_url,
  action.normalized_target_url as action_normalized_target_url,
  coalesce(article.id, '00000000-0000-0000-0000-000000000000'::uuid) as article_id,
  article.content_hash as article_content_hash,
  article.published_at as article_published_at,
  article.canonical_url as article_canonical_url,
  article.external_url as article_external_url,
  coalesce(article.seo_meta->>'canonical_url', ''::text)::text as seo_canonical_url,
  coalesce(article.seo_meta->>'slug', ''::text)::text as seo_slug
from seo_opportunities opportunity
left join content_actions action
  on action.project_id = opportunity.project_id and action.opportunity_id = opportunity.id
left join articles article
  on article.project_id = opportunity.project_id and article.id = action.draft_article_id
where opportunity.project_id = sqlc.arg(project_id)
  and opportunity.id = sqlc.arg(opportunity_id)
order by
  (article.canonical_url is not null or article.external_url is not null) desc,
  article.published_at desc nulls last,
  action.updated_at desc nulls last,
  action.id
limit 1;

-- name: LockLegacyGrowthIntendedTarget :one
with locked_opportunity as materialized (
  select opportunity.*
  from seo_opportunities opportunity
  where opportunity.project_id = sqlc.arg(project_id)
    and opportunity.id = sqlc.arg(opportunity_id)
  for update
), locked_actions as materialized (
  select selected.*
  from content_actions selected
  join locked_opportunity opportunity on opportunity.project_id = selected.project_id
    and opportunity.id = selected.opportunity_id
  order by selected.id
  for update of selected
), locked_articles as materialized (
  select selected_article.*
  from articles selected_article
  join locked_actions action on action.project_id = selected_article.project_id
    and action.draft_article_id = selected_article.id
  order by selected_article.id
  for update of selected_article
), selected_target as (
  select action.*, article.id as selected_article_id
  from locked_actions action
  left join locked_articles article on article.project_id = action.project_id and article.id = action.draft_article_id
  order by
    (article.canonical_url is not null or article.external_url is not null) desc,
    article.published_at desc nulls last,
    action.updated_at desc nulls last,
    action.id
  limit 1
)
select
  coalesce(opportunity.evidence->>'intended_slug_or_canonical', ''::text)::text as evidence_intended,
  opportunity.page_url as opportunity_page_url,
  coalesce(action.id, '00000000-0000-0000-0000-000000000000'::uuid) as action_id,
  action.updated_at as action_updated_at,
  action.target_url as action_target_url,
  action.normalized_target_url as action_normalized_target_url,
  coalesce(article.id, '00000000-0000-0000-0000-000000000000'::uuid) as article_id,
  article.content_hash as article_content_hash,
  article.published_at as article_published_at,
  article.canonical_url as article_canonical_url,
  article.external_url as article_external_url,
  coalesce(article.seo_meta->>'canonical_url', ''::text)::text as seo_canonical_url,
  coalesce(article.seo_meta->>'slug', ''::text)::text as seo_slug
from locked_opportunity opportunity
left join selected_target action on true
left join locked_articles article
  on article.project_id = action.project_id and article.id = action.selected_article_id;

-- name: CreateDuplicateGrowthOpportunityAlias :one
insert into growth_opportunity_work_aliases
  (project_id, legacy_opportunity_id, canonical_object_type, canonical_opportunity_id, work_signature_id, disposition)
select sqlc.arg(project_id), sqlc.arg(legacy_opportunity_id), 'seo_opportunity', signature.reserved_work_id,
       signature.id, 'duplicate'
from work_signature_registry signature
where signature.project_id = sqlc.arg(project_id)
  and signature.id = sqlc.arg(work_signature_id)
  and signature.mode = 'enforced' and signature.active = true
  and signature.owner = 'opportunities'
  and signature.reserved_work_type = 'seo_opportunity'
  and signature.reserved_work_id is not null
  and signature.reserved_work_id <> sqlc.arg(legacy_opportunity_id)
on conflict (project_id, legacy_opportunity_id) do update set
  canonical_opportunity_id = excluded.canonical_opportunity_id,
  canonical_object_type = excluded.canonical_object_type,
  canonical_site_fix_id = null,
  work_signature_id = excluded.work_signature_id,
  disposition = 'duplicate'
returning *;

-- name: CreateCanonicalizedGrowthOpportunityAlias :one
insert into growth_opportunity_work_aliases
  (project_id, legacy_opportunity_id, canonical_object_type, canonical_opportunity_id, work_signature_id, disposition)
values (sqlc.arg(project_id), sqlc.arg(opportunity_id), 'seo_opportunity', sqlc.arg(opportunity_id),
        sqlc.arg(work_signature_id), 'canonicalized')
on conflict (project_id, legacy_opportunity_id) do update set
  canonical_object_type = excluded.canonical_object_type,
  canonical_opportunity_id = excluded.canonical_opportunity_id,
  canonical_site_fix_id = null,
  work_signature_id = excluded.work_signature_id,
  disposition = excluded.disposition
returning *;

-- name: CreateDoctorGrowthEvidenceAlias :one
insert into growth_opportunity_work_aliases
  (project_id, legacy_opportunity_id, canonical_object_type, canonical_site_fix_id, work_signature_id, disposition)
select sqlc.arg(project_id), sqlc.arg(legacy_opportunity_id), 'site_fix', fix.id,
       signature.id, 'doctor_merge'
from work_signature_registry signature
join site_fixes fix on fix.project_id = signature.project_id and fix.work_signature_id = signature.id
where signature.project_id = sqlc.arg(project_id)
  and signature.id = sqlc.arg(work_signature_id)
  and signature.mode = 'enforced' and signature.active = true
  and signature.owner = 'doctor' and signature.reserved_work_type = 'site_fix'
on conflict (project_id, legacy_opportunity_id) do update set
  canonical_object_type = excluded.canonical_object_type,
  canonical_opportunity_id = null,
  canonical_site_fix_id = excluded.canonical_site_fix_id,
  work_signature_id = excluded.work_signature_id,
  disposition = excluded.disposition
returning *;

-- name: GetGrowthOpportunityWorkAlias :one
select * from growth_opportunity_work_aliases
where project_id = sqlc.arg(project_id)
  and legacy_opportunity_id = sqlc.arg(legacy_opportunity_id);

-- name: GetGrowthExecutionChainForUpdate :one
with locked_actions as materialized (
  select action.* from content_actions action
  where action.project_id = sqlc.arg(project_id)
    and action.opportunity_id = sqlc.arg(source_opportunity_id)
  order by action.id
  for update of action
), locked_topics as materialized (
  select topic.* from topics topic
  join locked_actions action on action.id = topic.source_content_action_id
  where topic.project_id = sqlc.arg(project_id)
  order by topic.id
  for update of topic
), locked_articles as materialized (
  select article.* from articles article
  where article.project_id = sqlc.arg(project_id)
    and (
      exists (select 1 from locked_topics topic where topic.id = article.topic_id)
      or exists (select 1 from locked_actions action where action.target_article_id = article.id or action.draft_article_id = article.id)
    )
  order by article.id
  for update of article
), locked_applications as materialized (
  select application.* from site_change_applications application
  join locked_actions action on action.id = application.content_action_id
  where application.project_id = sqlc.arg(project_id)
  order by application.id
  for update of application
), locked_drafts as materialized (
  select draft.* from page_update_drafts draft
  join locked_actions action on action.id = draft.content_action_id
  where draft.project_id = sqlc.arg(project_id)
  order by draft.id
  for update of draft
), locked_measurements as materialized (
  select measurement.* from action_measurements measurement
  join locked_actions action on action.id = measurement.content_action_id
  where measurement.project_id = sqlc.arg(project_id)
  order by measurement.id
  for update of measurement
), locked_runs as materialized (
  select run.* from generation_runs run
  where run.project_id = sqlc.arg(project_id)
    and run.input->>'topic' = any(
      coalesce((select array_agg(topic.id::text) from locked_topics topic), array[]::text[])
    )
  order by run.id
  for update of run
)
select
  (select count(*)::bigint from locked_actions) as action_count,
  (select count(*)::bigint from locked_actions source
    join content_actions canonical
      on canonical.project_id = source.project_id
     and canonical.opportunity_id = sqlc.narg(canonical_opportunity_id)
     and canonical.action_type = source.action_type
     and canonical.id <> source.id) as conflicting_action_count,
  jsonb_build_object(
    'content_actions', coalesce((select jsonb_agg(to_jsonb(action) order by action.id) from locked_actions action), '[]'::jsonb),
    'topics', coalesce((select jsonb_agg(to_jsonb(topic) order by topic.id) from locked_topics topic), '[]'::jsonb),
    'articles', coalesce((select jsonb_agg(to_jsonb(article) order by article.id) from locked_articles article), '[]'::jsonb),
    'site_change_applications', coalesce((select jsonb_agg(to_jsonb(application) order by application.id) from locked_applications application), '[]'::jsonb),
    'page_update_drafts', coalesce((select jsonb_agg(to_jsonb(draft) order by draft.id) from locked_drafts draft), '[]'::jsonb),
    'action_measurements', coalesce((select jsonb_agg(to_jsonb(measurement) order by measurement.id) from locked_measurements measurement), '[]'::jsonb),
    'generation_runs', coalesce((select jsonb_agg(to_jsonb(run) order by run.id) from locked_runs run), '[]'::jsonb)
  ) as execution_snapshot;

-- name: RepointDuplicateGrowthContentActions :one
with repointed as (
  update content_actions action set opportunity_id = sqlc.arg(canonical_opportunity_id), updated_at = now()
  where action.project_id = sqlc.arg(project_id)
    and action.opportunity_id = sqlc.arg(source_opportunity_id)
    and not exists (
      select 1 from content_actions canonical
      where canonical.project_id = action.project_id
        and canonical.opportunity_id = sqlc.arg(canonical_opportunity_id)
        and canonical.action_type = action.action_type
        and canonical.id <> action.id
    )
  returning action.id
)
select count(*)::bigint as repointed_count,
       coalesce(array_agg(repointed.id order by repointed.id), array[]::uuid[])::uuid[] as repointed_content_action_ids
from repointed;

-- name: RestoreGrowthContentActionRepoints :execrows
update content_actions action set opportunity_id = sqlc.arg(source_opportunity_id), updated_at = now()
where action.project_id = sqlc.arg(project_id)
  and action.opportunity_id = sqlc.arg(canonical_opportunity_id)
  and action.id = any(sqlc.arg(content_action_ids)::uuid[]);

-- name: StartGrowthCutoverSession :one
insert into growth_cutover_sessions
  (batch_id, project_id, fence_token, status)
values (sqlc.arg(batch_id), sqlc.arg(project_id), sqlc.arg(fence_token), 'applying')
returning *;

-- name: GetActiveGrowthCutoverSession :one
select * from growth_cutover_sessions
where project_id = sqlc.arg(project_id) and status = 'applying'
order by started_at desc limit 1;

-- name: AppendGrowthCutoverSessionEntry :one
insert into growth_cutover_session_entries
  (batch_id, project_id, sequence_number, opportunity_id, run_id, candidate_id,
   arbitration_decision_id, ai_call_id, work_signature_id, disposition,
   before_snapshot, after_snapshot, inverse_operation)
values
  (sqlc.arg(batch_id), sqlc.arg(project_id), sqlc.arg(sequence_number),
   sqlc.arg(opportunity_id), sqlc.arg(run_id), sqlc.arg(candidate_id),
   sqlc.narg(arbitration_decision_id), sqlc.narg(ai_call_id), sqlc.narg(work_signature_id),
   sqlc.arg(disposition), sqlc.arg(before_snapshot)::jsonb,
   sqlc.arg(after_snapshot)::jsonb, sqlc.arg(inverse_operation)::jsonb)
returning *;

-- name: UpdateGrowthCutoverSessionEntryDecision :one
update growth_cutover_session_entries set
  arbitration_decision_id = coalesce(sqlc.narg(arbitration_decision_id), arbitration_decision_id),
  ai_call_id = coalesce(sqlc.narg(ai_call_id), ai_call_id),
  work_signature_id = coalesce(sqlc.narg(work_signature_id), work_signature_id),
  disposition = sqlc.arg(disposition),
  entry_status = sqlc.arg(entry_status),
  before_snapshot = coalesce(sqlc.narg(before_snapshot)::jsonb, before_snapshot),
  after_snapshot = sqlc.arg(after_snapshot)::jsonb,
  inverse_operation = sqlc.arg(inverse_operation)::jsonb
where project_id = sqlc.arg(project_id) and batch_id = sqlc.arg(batch_id)
  and opportunity_id = sqlc.arg(opportunity_id)
returning *;

-- name: ListGrowthCutoverSessionEntries :many
select * from growth_cutover_session_entries
where project_id = sqlc.arg(project_id) and batch_id = sqlc.arg(batch_id)
order by sequence_number desc;

-- name: FinishGrowthCutoverSession :one
update growth_cutover_sessions
set status = sqlc.arg(status), error = sqlc.narg(error), finished_at = now()
where project_id = sqlc.arg(project_id) and batch_id = sqlc.arg(batch_id)
  and status = 'applying'
returning *;

-- name: SetGrowthCutoverSessionReviewRequired :one
update growth_cutover_sessions set error = sqlc.arg(error)
where project_id = sqlc.arg(project_id) and batch_id = sqlc.arg(batch_id)
  and status = 'applying'
returning *;

-- name: MarkGrowthCutoverSessionEntriesRolledBack :execrows
update growth_cutover_session_entries set entry_status = 'rolled_back'
where project_id = sqlc.arg(project_id) and batch_id = sqlc.arg(batch_id)
  and entry_status <> 'rolled_back';

-- name: CountUnrepresentedActiveLegacyGrowth :one
select count(*)::bigint from seo_opportunities opportunity
where opportunity.project_id = sqlc.arg(project_id)
  and opportunity.canonical_read_only = false
  and not is_legacy_doctor_technical_opportunity(opportunity.type, opportunity.evidence)
  and opportunity.status in ('open','accepted','converted','snoozed','watching')
  and not exists (
    select 1 from work_signature_registry owned_signature
    where owned_signature.project_id = opportunity.project_id
      and owned_signature.mode = 'enforced' and owned_signature.active = true
      and owned_signature.owner = 'opportunities'
      and owned_signature.reserved_work_type = 'seo_opportunity'
      and owned_signature.reserved_work_id = opportunity.id
  )
  and not exists (
    select 1 from growth_opportunity_work_aliases alias
    join work_signature_registry alias_signature
      on alias_signature.project_id = alias.project_id
     and alias_signature.id = alias.work_signature_id
     and alias_signature.mode = 'enforced' and alias_signature.active = true
    where alias.project_id = opportunity.project_id
      and alias.legacy_opportunity_id = opportunity.id
  );

-- name: RollbackGrowthCutoverDuplicate :one
with tombstoned_alias as (
  update growth_opportunity_work_aliases alias set disposition = 'rolled_back'
  where alias.project_id = sqlc.arg(project_id)
    and alias.legacy_opportunity_id = sqlc.arg(opportunity_id)
    and alias.disposition in ('duplicate','doctor_merge')
  returning alias.legacy_opportunity_id, alias.work_signature_id
), restored_growth as (
  update seo_opportunities opportunity set
    evidence = sqlc.narg(canonical_evidence)::jsonb,
    priority_score = coalesce(sqlc.narg(canonical_priority_score)::numeric, opportunity.priority_score),
    confidence = coalesce(sqlc.narg(canonical_confidence)::numeric, opportunity.confidence),
    evidence_fingerprint = coalesce(sqlc.narg(canonical_evidence_fingerprint)::text, opportunity.evidence_fingerprint),
    updated_at = now()
  where opportunity.project_id = sqlc.arg(project_id)
    and opportunity.id = sqlc.narg(canonical_opportunity_id)
    and exists (select 1 from tombstoned_alias)
  returning opportunity.id
), restored_doctor as (
  update site_fixes fix set evidence_snapshot = sqlc.narg(canonical_evidence)::jsonb, updated_at = now()
  where fix.project_id = sqlc.arg(project_id)
    and fix.id = sqlc.narg(canonical_site_fix_id)
    and exists (select 1 from tombstoned_alias)
  returning fix.id
), restored_signature as (
  update work_signature_registry signature set
    evidence_fingerprint = coalesce(sqlc.narg(canonical_evidence_fingerprint)::text, signature.evidence_fingerprint),
    updated_at = now()
  from tombstoned_alias alias
  where signature.project_id = sqlc.arg(project_id) and signature.id = alias.work_signature_id
  returning signature.id
), tombstoned_candidate as (
  update discovery_candidates candidate set status = 'migration_rolled_back', updated_at = now()
  where candidate.project_id = sqlc.arg(project_id)
    and candidate.id = sqlc.arg(candidate_id)
    and exists (select 1 from tombstoned_alias)
  returning candidate.id
), superseded_decisions as (
  update discovery_arbitration_decisions decision set status = 'superseded', updated_at = now()
  where decision.project_id = sqlc.arg(project_id) and decision.candidate_id = sqlc.arg(candidate_id)
  returning decision.id
), terminal_ai_calls as (
  update ai_call_records call set
    status = case when call.status in ('queued','running') then 'failed' else call.status end,
    error_code = case when call.status in ('queued','running') then 'growth_cutover_rolled_back' else call.error_code end,
    finished_at = case when call.status in ('queued','running') then coalesce(call.finished_at, now()) else call.finished_at end,
    updated_at = now()
  where call.project_id = sqlc.arg(project_id)
    and call.linked_object_type = 'discovery_candidate'
    and call.linked_object_id = sqlc.arg(candidate_id)
  returning call.id
), failed_run as (
  update discovery_shadow_runs run set status = 'failed', error = 'growth cutover rolled back',
    finished_at = coalesce(finished_at, now()), updated_at = now()
  where run.project_id = sqlc.arg(project_id) and run.id = sqlc.arg(run_id)
  returning run.id
)
select count(*)::bigint as removed_count from tombstoned_alias;

-- name: RollbackGrowthCutoverMaterialized :one
with tombstoned_candidate as (
  update discovery_candidates candidate set status = 'migration_rolled_back', updated_at = now()
  where candidate.project_id = sqlc.arg(project_id) and candidate.id = sqlc.arg(candidate_id)
  returning candidate.id
), superseded_decisions as (
  update discovery_arbitration_decisions decision set status = 'superseded', updated_at = now()
  where decision.project_id = sqlc.arg(project_id) and decision.candidate_id = sqlc.arg(candidate_id)
  returning decision.id
), terminal_ai_calls as (
  update ai_call_records call set
    status = case when call.status in ('queued','running') then 'failed' else call.status end,
    error_code = case when call.status in ('queued','running') then 'growth_cutover_rolled_back' else call.error_code end,
    finished_at = case when call.status in ('queued','running') then coalesce(call.finished_at, now()) else call.finished_at end,
    updated_at = now()
  where call.project_id = sqlc.arg(project_id)
    and call.linked_object_type = 'discovery_candidate'
    and call.linked_object_id = sqlc.arg(candidate_id)
  returning call.id
), failed_run as (
  update discovery_shadow_runs run set status = 'failed', error = 'growth cutover rolled back',
    finished_at = coalesce(finished_at, now()), updated_at = now()
  where run.project_id = sqlc.arg(project_id) and run.id = sqlc.arg(run_id)
  returning run.id
)
select count(*)::bigint as tombstoned_count from tombstoned_candidate;

-- name: RollbackGrowthCutoverCanonical :one
with source_signature as materialized (
  select signature.id, signature.candidate_id, signature.conflict_bucket_keys
  from work_signature_registry signature
  where signature.project_id = sqlc.arg(project_id)
    and signature.id = sqlc.arg(work_signature_id)
    and signature.candidate_id = sqlc.arg(candidate_id)
    and signature.owner = 'opportunities'
    and signature.reserved_work_type = 'seo_opportunity'
    and signature.reserved_work_id = sqlc.arg(opportunity_id)
), expected_keys as materialized (
  select distinct key.bucket_key
  from source_signature signature
  cross join lateral jsonb_array_elements_text(signature.conflict_bucket_keys) key(bucket_key)
), locked_buckets as materialized (
  select bucket.id
  from work_conflict_buckets bucket
  join expected_keys expected on expected.bucket_key = bucket.bucket_key
  where bucket.project_id = sqlc.arg(project_id)
  order by bucket.bucket_key
  for update of bucket
), tombstoned_relationships as (
  update work_relationships relationship set active = false, resolved_at = now(), updated_at = now()
  from source_signature signature
  where relationship.project_id = sqlc.arg(project_id)
    and (relationship.dependent_work_signature_id = signature.id
      or relationship.blocking_work_signature_id = signature.id)
  returning relationship.id
), tombstoned_aliases as (
  update growth_opportunity_work_aliases alias set disposition = 'rolled_back'
  from source_signature signature
  where alias.project_id = sqlc.arg(project_id)
    and alias.work_signature_id = signature.id
  returning alias.legacy_opportunity_id
), reverted_opportunity as (
  update seo_opportunities opportunity
  set canonical_growth = false,
      evidence = sqlc.arg(original_evidence)::jsonb,
      evidence_fingerprint = sqlc.arg(original_evidence_fingerprint),
      updated_at = now()
  from source_signature signature
  where opportunity.project_id = sqlc.arg(project_id)
    and opportunity.id = sqlc.arg(opportunity_id)
    and opportunity.canonical_growth = true
    and (select count(*) from locked_buckets) = (select count(*) from expected_keys)
  returning opportunity.id
), tombstoned_signature as (
  update work_signature_registry signature set active = false, status = 'migration_rolled_back', updated_at = now()
  from source_signature source
  where signature.project_id = sqlc.arg(project_id)
    and signature.id = source.id
    and exists (select 1 from reverted_opportunity)
  returning signature.candidate_id
), tombstoned_candidate as (
  update discovery_candidates candidate set status = 'migration_rolled_back', updated_at = now()
  from tombstoned_signature signature
  where candidate.project_id = sqlc.arg(project_id)
    and candidate.id = signature.candidate_id
  returning candidate.id
), superseded_decisions as (
  update discovery_arbitration_decisions decision set status = 'superseded', updated_at = now()
  where decision.project_id = sqlc.arg(project_id) and decision.candidate_id = sqlc.arg(candidate_id)
  returning decision.id
), terminal_ai_calls as (
  update ai_call_records call set
    status = case when call.status in ('queued','running') then 'failed' else call.status end,
    error_code = case when call.status in ('queued','running') then 'growth_cutover_rolled_back' else call.error_code end,
    finished_at = case when call.status in ('queued','running') then coalesce(call.finished_at, now()) else call.finished_at end,
    updated_at = now()
  where call.project_id = sqlc.arg(project_id)
    and call.linked_object_type = 'discovery_candidate'
    and call.linked_object_id = sqlc.arg(candidate_id)
  returning call.id
), failed_run as (
  update discovery_shadow_runs run set status = 'failed', error = 'growth cutover rolled back',
    finished_at = coalesce(finished_at, now()), updated_at = now()
  where run.project_id = sqlc.arg(project_id) and run.id = sqlc.arg(run_id)
  returning run.id
), bumped as (
  update work_conflict_buckets bucket
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where bucket.id = locked.id and exists (select 1 from tombstoned_signature)
  returning bucket.id
)
select count(*)::bigint as removed_count from tombstoned_signature;

-- name: UpdateSEOOpportunityStatus :one
update seo_opportunities set
  status = $3,
  updated_at = now()
where seo_opportunities.id = $1 and seo_opportunities.project_id = $2
  and not exists (
    select 1 from growth_opportunity_work_aliases alias
    where alias.project_id = seo_opportunities.project_id
      and alias.legacy_opportunity_id = seo_opportunities.id
      and alias.disposition in ('duplicate','doctor_merge')
  )
returning *;

-- name: CountOpenSEOOpportunities :one
select count(*)::bigint from seo_opportunities
where seo_opportunities.project_id = $1
  and seo_opportunities.status = 'open'
  and not exists (
    select 1 from growth_opportunity_work_aliases alias
    where alias.project_id = seo_opportunities.project_id
      and alias.legacy_opportunity_id = seo_opportunities.id
      and alias.disposition in ('duplicate','doctor_merge')
  );

-- name: CreateContentAction :one
insert into content_actions
  (project_id, opportunity_id, action_type, status, target_article_id, target_url,
   normalized_target_url, target_content_hash_before, baseline_window, measurement_window,
   approval_source, routing_source, work_type)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
on conflict (project_id, opportunity_id, action_type) do update set
  status = excluded.status,
  status_reason = null,
  target_article_id = excluded.target_article_id,
  target_url = excluded.target_url,
  normalized_target_url = excluded.normalized_target_url,
  target_content_hash_before = excluded.target_content_hash_before,
  baseline_window = excluded.baseline_window,
  measurement_window = excluded.measurement_window,
  approval_source = excluded.approval_source,
  routing_source = excluded.routing_source,
  work_type = excluded.work_type,
  updated_at = now()
returning *;

-- name: SnoozeSEOOpportunity :one
update seo_opportunities set
  status = 'snoozed',
  snoozed_until = sqlc.arg(snoozed_until),
  snooze_reason = sqlc.narg(snooze_reason),
  updated_at = now()
where seo_opportunities.id = sqlc.arg(id) and seo_opportunities.project_id = sqlc.arg(project_id) and seo_opportunities.status in ('open','snoozed')
  and not exists (
    select 1 from growth_opportunity_work_aliases alias
    where alias.project_id = seo_opportunities.project_id
      and alias.legacy_opportunity_id = seo_opportunities.id
      and alias.disposition in ('duplicate','doctor_merge')
  )
returning *;

-- name: UnsnoozeSEOOpportunity :one
update seo_opportunities set
  status = 'open',
  snoozed_until = null,
  unsnoozed_at = now(),
  updated_at = now()
where seo_opportunities.id = sqlc.arg(id) and seo_opportunities.project_id = sqlc.arg(project_id) and seo_opportunities.status = 'snoozed'
  and not exists (
    select 1 from growth_opportunity_work_aliases alias
    where alias.project_id = seo_opportunities.project_id
      and alias.legacy_opportunity_id = seo_opportunities.id
      and alias.disposition in ('duplicate','doctor_merge')
  )
returning *;

-- name: WakeDueSnoozedSEOOpportunities :execrows
update seo_opportunities set
  status = 'open',
  unsnoozed_at = now(),
  updated_at = now()
where seo_opportunities.project_id = $1
  and seo_opportunities.status = 'snoozed'
  and seo_opportunities.snoozed_until is not null
  and seo_opportunities.snoozed_until <= now()
  and not exists (
    select 1 from growth_opportunity_work_aliases alias
    where alias.project_id = seo_opportunities.project_id
      and alias.legacy_opportunity_id = seo_opportunities.id
      and alias.disposition in ('duplicate','doctor_merge')
  );

-- name: CreateSEOWatchlistItem :one
insert into seo_watchlist_items
  (project_id, source_opportunity_id, observation_window_days, due_at)
values ($1, $2, $3, $4)
on conflict (project_id, source_opportunity_id) do update set
  status = 'watching',
  observation_window_days = excluded.observation_window_days,
  due_at = excluded.due_at,
  closed_at = null,
  updated_at = now()
returning *;

-- name: ListSEOWatchlistItems :many
select w.*,
  coalesce(so.type, '')::text as opportunity_type,
  so.page_url as opportunity_page_url,
  so.query as opportunity_query,
  so.recommended_action as opportunity_recommended_action,
  so.expected_impact as opportunity_expected_impact
from seo_watchlist_items w
left join seo_opportunities so
  on so.id = w.source_opportunity_id
  and so.project_id = w.project_id
where w.project_id = sqlc.arg(project_id)
  and (sqlc.arg(status)::text = '' or w.status = sqlc.arg(status))
order by w.due_at asc, w.created_at desc
limit sqlc.arg(limit_rows);

-- name: MarkDueSEOWatchlistItems :execrows
update seo_watchlist_items set
  status = 'due_for_review',
  updated_at = now()
where project_id = $1
  and status = 'watching'
  and due_at <= now();

-- name: CloseSEOWatchlistItem :one
update seo_watchlist_items set
  status = sqlc.arg(status),
  closed_at = now(),
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: ListContentActions :many
select * from content_actions
where project_id = sqlc.arg(project_id)
  and (sqlc.arg(status)::text = '' or status = sqlc.arg(status))
  and (sqlc.arg(cursor_created_at)::timestamptz is null or created_at < sqlc.arg(cursor_created_at))
order by created_at desc
limit sqlc.arg(limit_rows);

-- name: ListVisibilityActionRows :many
select
  ca.id,
  ca.project_id,
  ca.opportunity_id,
  ca.action_type,
  ca.status,
  ca.target_article_id,
  ca.target_url,
  ca.normalized_target_url,
  ca.draft_article_id,
  ca.baseline_window,
  ca.measurement_window,
  ca.measurement_policy_version,
  ca.measurement_policy,
  ca.measuring_started_at,
  ca.absolute_terminal_at,
  ca.measurement_terminal_reason,
  ca.published_at,
  ca.outcome_summary,
  ca.created_at,
  ca.updated_at,
  ca.asset_type,
  ca.risk_reasons,
  ca.evidence_snapshot,
  ca.output_snapshot,
  ca.diff_snapshot,
  ca.review_required,
  ca.verified_at,
  ca.verification_snapshot,
  ca.approval_source,
  ca.routing_source,
  ca.work_type,
  coalesce(so.status, '')::text as opportunity_status,
  coalesce(so.type, '')::text as opportunity_type,
  so.page_url as opportunity_page_url,
  so.normalized_page_url as opportunity_normalized_page_url,
  so.query as opportunity_query,
  so.recommended_action as opportunity_recommended_action,
  so.expected_impact as opportunity_expected_impact,
  coalesce(so.risk_level, '')::text as opportunity_risk_level,
  so.priority_score as opportunity_priority_score,
  t.id as topic_id,
  t.status as topic_status,
  t.title as topic_title,
  a.id as draft_article_joined_id,
  a.status as draft_article_status,
  a.canonical_url as draft_article_canonical_url
from content_actions ca
left join seo_opportunities so
  on so.id = ca.opportunity_id
  and so.project_id = ca.project_id
left join topics t
  on t.source_content_action_id = ca.id
  and t.project_id = ca.project_id
left join articles a
  on a.id = ca.draft_article_id
  and a.project_id = ca.project_id
where ca.project_id = $1
order by ca.updated_at desc, ca.created_at desc
limit $2;

-- name: GetContentAction :one
select * from content_actions
where id = $1 and project_id = $2;

-- name: UpdateContentActionStatus :one
update content_actions set
  status = $3,
  status_reason = null,
  updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: MarkContentActionReturnedToOpportunity :one
with candidate as (
  select content_actions.id, content_actions.project_id, content_actions.opportunity_id,
    content_actions.draft_article_id
  from content_actions
  where content_actions.id = sqlc.arg(action_id)
    and content_actions.project_id = sqlc.arg(project_id)
    and published_at is null
    and status in (
      'drafting','ready_for_review','approved','failed','verification_failed',
      'recovery_required','verification_pending','manual_apply_required','needs_follow_up'
    )
  for update
),
updated_action as (
  update content_actions ca set
    status = 'returned',
    status_reason = 'returned_to_opportunities',
    updated_at = now()
  from candidate
  where ca.id = candidate.id
    and ca.project_id = candidate.project_id
  returning ca.*
),
updated_opportunity as (
  update seo_opportunities so set
    status = 'open',
    snoozed_until = null,
    snooze_reason = null,
    updated_at = now()
  from candidate
  where so.id = candidate.opportunity_id
    and so.project_id = candidate.project_id
  returning so.id
),
-- Withdraw the in-progress draft so a returned opportunity does not leave an
-- approved article stranded in the Publish "Ready to post" queue.
withdrawn_article as (
  update articles a set
    status = 'rejected',
    updated_at = now()
  from candidate
  where a.id = candidate.draft_article_id
    and a.project_id = candidate.project_id
    and a.status in ('generating','pending_review','approved')
  returning a.id
)
select * from updated_action;

-- name: DismissSEOContentActionAndOpportunity :one
with candidate as (
  select content_actions.id, content_actions.project_id, content_actions.opportunity_id
  from content_actions
  where content_actions.id = sqlc.arg(action_id)
    and content_actions.project_id = sqlc.arg(project_id)
    and published_at is null
    and status in (
      'drafting','ready_for_review','approved','failed','verification_failed',
      'recovery_required','verification_pending','manual_apply_required','needs_follow_up'
    )
  for update
),
updated_action as (
  update content_actions ca set
    status = 'dismissed',
    status_reason = 'opportunity_dismissed',
    updated_at = now()
  from candidate
  where ca.id = candidate.id
    and ca.project_id = candidate.project_id
  returning ca.*
),
updated_opportunity as (
  update seo_opportunities so set
    status = 'dismissed',
    updated_at = now()
  from candidate
  where so.id = candidate.opportunity_id
    and so.project_id = candidate.project_id
  returning so.*
)
select * from updated_action;

-- name: CreateOrUpdateSEOOpportunityReviewState :one
with source_opportunity as (
  select *
  from seo_opportunities
  where seo_opportunities.id = sqlc.arg(source_opportunity_id)
    and seo_opportunities.project_id = sqlc.arg(project_id)
)
insert into seo_opportunity_review_states
  (project_id, opportunity_identity_key, source_opportunity_id, content_action_id,
   review_status, evidence_fingerprint, reviewed_by, snoozed_until, reviewed_at)
select
  source_opportunity.project_id,
  source_opportunity.opportunity_identity_key,
  source_opportunity.id,
  sqlc.narg(content_action_id)::uuid,
  sqlc.arg(review_status)::text,
  source_opportunity.evidence_fingerprint,
  sqlc.narg(reviewed_by)::text,
  coalesce(sqlc.narg(snoozed_until)::timestamptz, source_opportunity.snoozed_until),
  now()
from source_opportunity
where source_opportunity.opportunity_identity_key <> ''
on conflict (project_id, opportunity_identity_key) do update set
  source_opportunity_id = excluded.source_opportunity_id,
  content_action_id = coalesce(excluded.content_action_id, seo_opportunity_review_states.content_action_id),
  review_status = excluded.review_status,
  evidence_fingerprint = excluded.evidence_fingerprint,
  reviewed_by = excluded.reviewed_by,
  snoozed_until = excluded.snoozed_until,
  reviewed_at = excluded.reviewed_at,
  reopened_at = null,
  reopened_reason = null,
  material_change_metadata = '{}'::jsonb,
  updated_at = now()
returning *;

-- name: MarkContentActionVerification :one
update content_actions set
  status = sqlc.arg(status)::text,
  verified_at = sqlc.narg(verified_at)::timestamptz,
  verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: MarkContentActionSiteFixPRResult :one
update content_actions set
  status = 'verification_pending',
  output_snapshot = coalesce(output_snapshot, '{}'::jsonb) || jsonb_build_object('publisher_result', sqlc.arg(publisher_result)::jsonb),
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: MarkSiteChangeApplicationAndContentActionVerified :one
with verified_application as (
  update site_change_applications set
    status = 'verified',
    deployment_snapshot = sqlc.arg(deployment_snapshot)::jsonb,
    verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
    failure_reason = null,
    deployed_at = coalesce(deployed_at, sqlc.arg(verified_at)::timestamptz),
    verified_at = coalesce(verified_at, sqlc.arg(verified_at)::timestamptz),
    updated_at = now()
  where site_change_applications.id = sqlc.arg(application_id)
    and site_change_applications.project_id = sqlc.arg(project_id)
    and site_change_applications.content_action_id is not null
    and site_change_applications.site_fix_id is null
    and exists (select 1 from content_actions action where action.project_id=site_change_applications.project_id and action.id=site_change_applications.content_action_id and action.canonical_read_only=false)
  returning content_action_id
)
update content_actions set
  status = 'measuring',
  verified_at = sqlc.arg(verified_at)::timestamptz,
  verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
  output_snapshot = coalesce(output_snapshot, '{}'::jsonb) ||
    jsonb_build_object('publisher_result', sqlc.arg(publisher_result)::jsonb),
  updated_at = now()
from verified_application
where content_actions.id = verified_application.content_action_id
  and content_actions.project_id = sqlc.arg(project_id)
returning content_actions.*;

-- name: MarkContentActionDraftReady :one
update content_actions set
  status = 'ready_for_review',
  draft_article_id = $3,
  status_reason = null,
  updated_at = now()
where id = $1 and project_id = $2
  and status not in ('returned','dismissed','published','measuring','completed')
returning *;

-- name: CreateOrReusePageUpdateDraft :one
insert into page_update_drafts
  (project_id, content_action_id, target_url, normalized_target_url, opportunity_key,
   target_article_id, source_file_path, base_content_hash, resolution_criteria, original_source_snapshot)
values (
  sqlc.arg(project_id),
  sqlc.arg(content_action_id),
  sqlc.arg(target_url),
  sqlc.arg(normalized_target_url),
  sqlc.arg(opportunity_key),
  sqlc.narg(target_article_id),
  sqlc.narg(source_file_path),
  sqlc.narg(base_content_hash),
  sqlc.arg(resolution_criteria)::jsonb,
  sqlc.arg(original_source_snapshot)::jsonb
)
on conflict (project_id, content_action_id) do update set
  target_url = excluded.target_url,
  normalized_target_url = excluded.normalized_target_url,
  opportunity_key = excluded.opportunity_key,
  target_article_id = excluded.target_article_id,
  source_file_path = coalesce(excluded.source_file_path, page_update_drafts.source_file_path),
  base_content_hash = coalesce(excluded.base_content_hash, page_update_drafts.base_content_hash),
  resolution_criteria = excluded.resolution_criteria,
  original_source_snapshot = excluded.original_source_snapshot,
  updated_at = now()
returning *;

-- name: GetPageUpdateDraftForProject :one
select * from page_update_drafts
where id = $1 and project_id = $2;

-- name: GetPageUpdateDraftForContentAction :one
select * from page_update_drafts
where project_id = $1 and content_action_id = $2;

-- name: UpdatePageUpdateDraftContent :one
update page_update_drafts set
  proposed_content_md = sqlc.arg(proposed_content_md),
  patch = sqlc.arg(patch)::jsonb,
  diff_snapshot = sqlc.arg(diff_snapshot)::jsonb,
  qa_feedback = sqlc.arg(qa_feedback)::jsonb,
  status = 'ready_for_review',
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: UpdatePageUpdateDraftStatus :one
update page_update_drafts set
  status = sqlc.arg(status),
  approved_at = case when sqlc.arg(status) = 'approved' then now() else approved_at end,
  applied_at = case when sqlc.arg(status) in ('applied','verification_pending','manual_apply_required') then now() else applied_at end,
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: ClaimPageUpdateDraftForApply :one
with candidate as (
  select page_update_drafts.id from page_update_drafts
  where page_update_drafts.id = sqlc.arg(id)
    and page_update_drafts.project_id = sqlc.arg(project_id)
    and page_update_drafts.status = 'approved'
  for update skip locked
)
update page_update_drafts set
  status = 'applying',
  updated_at = now()
from candidate
where page_update_drafts.id = candidate.id
returning page_update_drafts.*;

-- name: MarkPageUpdateDraftApplyResult :one
update page_update_drafts set
  status = sqlc.arg(status),
  publisher_result = sqlc.arg(publisher_result)::jsonb,
  applied_at = now(),
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: MarkPageUpdateDraftVerification :one
update page_update_drafts set
  status = sqlc.arg(status),
  verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
  verified_at = case when sqlc.arg(status) = 'verified' then now() else verified_at end,
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: ListStalePageUpdateDrafts :many
select * from page_update_drafts
where project_id = sqlc.arg(project_id)
  and status in ('drafting','applying','verification_pending')
  and updated_at < now() - interval '30 minutes'
order by updated_at asc
limit sqlc.arg(limit_rows)
for update skip locked;

-- name: CreateOrReuseSiteChangeApplication :one
insert into site_change_applications
  (project_id, source_opportunity_id, content_action_id, page_update_draft_id,
   application_kind, target_url, normalized_target_url, opportunity_key,
   publisher_connection_id, repo_full_name, base_branch, working_branch,
   base_commit_sha, head_commit_sha, source_file_path, source_file_paths,
   source_mapping_confidence, source_mapping_reason, base_file_sha,
   base_content_hash, proposed_content_hash, patch_snapshot, diff_snapshot,
   resolution_criteria, status)
select
  sqlc.arg(project_id),
  sqlc.narg(source_opportunity_id),
  sqlc.arg(content_action_id),
  sqlc.narg(page_update_draft_id),
  sqlc.arg(application_kind),
  sqlc.arg(target_url),
  sqlc.arg(normalized_target_url),
  sqlc.arg(opportunity_key),
  sqlc.narg(publisher_connection_id),
  sqlc.narg(repo_full_name),
  sqlc.narg(base_branch),
  sqlc.narg(working_branch),
  sqlc.narg(base_commit_sha),
  sqlc.narg(head_commit_sha),
  sqlc.narg(source_file_path),
  sqlc.arg(source_file_paths)::jsonb,
  sqlc.arg(source_mapping_confidence),
  sqlc.arg(source_mapping_reason),
  sqlc.narg(base_file_sha),
  sqlc.narg(base_content_hash),
  sqlc.narg(proposed_content_hash),
  sqlc.arg(patch_snapshot)::jsonb,
  sqlc.arg(diff_snapshot)::jsonb,
  sqlc.arg(resolution_criteria)::jsonb,
  sqlc.arg(status)
where sqlc.arg(content_action_id)::uuid is not null
on conflict (project_id, opportunity_key)
where status in (
  'draft_ready',
  'source_mapping_required',
  'ready_for_pr',
  'creating_pr',
  'github_pr_open',
  'github_pr_closed',
  'github_pr_merged',
  'deployment_pending',
  'verification_pending',
  'needs_follow_up',
  'conflict',
  'manual_apply_required'
) do update set
  source_opportunity_id = coalesce(excluded.source_opportunity_id, site_change_applications.source_opportunity_id),
  content_action_id = excluded.content_action_id,
  page_update_draft_id = coalesce(excluded.page_update_draft_id, site_change_applications.page_update_draft_id),
  application_kind = excluded.application_kind,
  target_url = excluded.target_url,
  normalized_target_url = excluded.normalized_target_url,
  publisher_connection_id = coalesce(excluded.publisher_connection_id, site_change_applications.publisher_connection_id),
  repo_full_name = coalesce(excluded.repo_full_name, site_change_applications.repo_full_name),
  base_branch = case
    when site_change_applications.status in ('github_pr_open','github_pr_merged','deployment_pending','verification_pending') then site_change_applications.base_branch
    else coalesce(excluded.base_branch, site_change_applications.base_branch)
  end,
  working_branch = case
    when site_change_applications.status in ('github_pr_open','github_pr_merged','deployment_pending','verification_pending') then site_change_applications.working_branch
    else coalesce(excluded.working_branch, site_change_applications.working_branch)
  end,
  base_commit_sha = case
    when site_change_applications.status in ('github_pr_open','github_pr_merged','deployment_pending','verification_pending') then site_change_applications.base_commit_sha
    else coalesce(excluded.base_commit_sha, site_change_applications.base_commit_sha)
  end,
  head_commit_sha = coalesce(excluded.head_commit_sha, site_change_applications.head_commit_sha),
  source_file_path = case
    when site_change_applications.status in ('github_pr_open','github_pr_merged','deployment_pending','verification_pending') then site_change_applications.source_file_path
    else coalesce(excluded.source_file_path, site_change_applications.source_file_path)
  end,
  source_file_paths = case
    when site_change_applications.status in ('github_pr_open','github_pr_merged','deployment_pending','verification_pending') then site_change_applications.source_file_paths
    when excluded.source_file_paths <> '[]'::jsonb then excluded.source_file_paths
    else site_change_applications.source_file_paths
  end,
  source_mapping_confidence = excluded.source_mapping_confidence,
  source_mapping_reason = excluded.source_mapping_reason,
  base_file_sha = coalesce(excluded.base_file_sha, site_change_applications.base_file_sha),
  base_content_hash = coalesce(excluded.base_content_hash, site_change_applications.base_content_hash),
  proposed_content_hash = coalesce(excluded.proposed_content_hash, site_change_applications.proposed_content_hash),
  patch_snapshot = excluded.patch_snapshot,
  diff_snapshot = excluded.diff_snapshot,
  resolution_criteria = excluded.resolution_criteria,
  status = case
    when site_change_applications.status in ('github_pr_open','github_pr_merged','deployment_pending','verification_pending','verified') then site_change_applications.status
    else excluded.status
  end,
  updated_at = now()
returning *;

-- name: GetSiteChangeApplicationForProject :one
select * from site_change_applications
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id);

-- name: ListOpenSiteChangePRApplications :many
select application.* from site_change_applications application
where application.project_id = sqlc.arg(project_id)
  and application.status = 'github_pr_open'
  and application.github_pr_number is not null
  and application.content_action_id is not null
  and application.site_fix_id is null
  and exists (select 1 from content_actions action where action.project_id=application.project_id and action.id=application.content_action_id and action.canonical_read_only=false)
order by application.updated_at asc;

-- name: GetActiveSiteChangeApplicationByOpportunityKey :one
select application.* from site_change_applications application
where application.project_id = sqlc.arg(project_id)
  and application.opportunity_key = sqlc.arg(opportunity_key)
  and application.status in (
    'draft_ready',
    'source_mapping_required',
    'ready_for_pr',
    'creating_pr',
    'github_pr_open',
    'github_pr_closed',
    'github_pr_merged',
    'deployment_pending',
    'verification_pending',
    'needs_follow_up',
    'conflict',
    'manual_apply_required'
  )
  and application.content_action_id is not null and application.site_fix_id is null
  and exists (select 1 from content_actions action where action.project_id=application.project_id and action.id=application.content_action_id and action.canonical_read_only=false)
order by application.updated_at desc
limit 1;

-- name: MarkSiteChangeApplicationGitHubPR :one
update site_change_applications application set
  status = 'github_pr_open',
  working_branch = sqlc.arg(working_branch),
  head_commit_sha = sqlc.narg(head_commit_sha),
  base_file_sha = coalesce(sqlc.narg(base_file_sha), base_file_sha),
  github_pr_number = sqlc.arg(github_pr_number),
  github_pr_url = sqlc.arg(github_pr_url),
  github_pr_state = sqlc.arg(github_pr_state),
  pr_created_at = coalesce(pr_created_at, now()),
  next_poll_at = now() + interval '5 minutes',
  next_notify_at = now() + interval '12 hours',
  updated_at = now()
where application.id = sqlc.arg(id) and application.project_id = sqlc.arg(project_id)
  and application.content_action_id is not null and application.site_fix_id is null
  and exists (select 1 from content_actions action where action.project_id=application.project_id and action.id=application.content_action_id and action.canonical_read_only=false)
returning *;

-- name: ListMergedSiteChangeApplicationsForVerification :many
select application.* from site_change_applications application
where application.project_id = sqlc.arg(project_id)
  and application.status = 'github_pr_merged'
  and application.content_action_id is not null
  and application.site_fix_id is null
  and exists (select 1 from content_actions action where action.project_id=application.project_id and action.id=application.content_action_id and action.canonical_read_only=false)
order by application.merged_at asc nulls first;

-- name: SetSiteChangePRNextPollAt :exec
update site_change_applications application set
  next_poll_at = sqlc.arg(next_poll_at),
  updated_at = now()
where application.id = sqlc.arg(id) and application.project_id = sqlc.arg(project_id)
  and application.content_action_id is not null and application.site_fix_id is null
  and exists (select 1 from content_actions action where action.project_id=application.project_id and action.id=application.content_action_id and action.canonical_read_only=false);

-- name: SetSiteChangePRNextNotifyAt :exec
update site_change_applications application set
  next_notify_at = sqlc.arg(next_notify_at),
  updated_at = now()
where application.id = sqlc.arg(id) and application.project_id = sqlc.arg(project_id)
  and application.content_action_id is not null and application.site_fix_id is null
  and exists (select 1 from content_actions action where action.project_id=application.project_id and action.id=application.content_action_id and action.canonical_read_only=false);

-- name: MarkSiteChangeApplicationStatus :one
update site_change_applications application set
  status = sqlc.arg(status),
  github_pr_state = coalesce(sqlc.narg(github_pr_state), github_pr_state),
  deployment_snapshot = sqlc.arg(deployment_snapshot)::jsonb,
  verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
  failure_reason = sqlc.narg(failure_reason),
  merged_at = case when sqlc.arg(status) = 'github_pr_merged' then coalesce(merged_at, now()) else merged_at end,
  deployed_at = case when sqlc.arg(status) in ('verification_pending','verified') then coalesce(deployed_at, now()) else deployed_at end,
  verified_at = case when sqlc.arg(status) = 'verified' then coalesce(verified_at, now()) else verified_at end,
  updated_at = now()
where application.id = sqlc.arg(id) and application.project_id = sqlc.arg(project_id)
  and application.content_action_id is not null and application.site_fix_id is null
  and exists (select 1 from content_actions action where action.project_id=application.project_id and action.id=application.content_action_id and action.canonical_read_only=false)
returning *;

-- name: MarkContentActionMeasuringForDraftArticle :one
update content_actions set
  status = 'measuring',
  published_at = now(),
  updated_at = now()
where project_id = $1 and draft_article_id = $2
returning *;

-- name: ListDueMeasuringContentActions :many
select ca.* from content_actions ca
where ca.project_id = sqlc.arg(project_id)
  and ca.status = 'measuring'
  and ca.canonical_read_only = false
  and not exists (
    select 1 from product_writer_authority authority
    where authority.project_id = ca.project_id and authority.product = 'opportunities' and authority.write_fenced = true
  )
  and not exists (
    select 1 from growth_opportunity_work_aliases alias
    where alias.project_id = ca.project_id and alias.legacy_opportunity_id = ca.opportunity_id
      and alias.disposition in ('duplicate','doctor_merge')
  )
  and (
    coalesce(ca.measuring_started_at, ca.published_at) is null
    or ca.measurement_window = '{}'::jsonb
    or ca.absolute_terminal_at <= sqlc.arg(now_at)::timestamptz
    or not exists (
      select 1 from action_measurements baseline
      where baseline.project_id = ca.project_id
        and baseline.content_action_id = ca.id
        and baseline.checkpoint_role = 'baseline'
        and baseline.measurement_policy_version = ca.measurement_policy_version
    )
    or exists (
      select 1
      from jsonb_array_elements(coalesce(ca.measurement_window->'checkpoints', '[]'::jsonb)) checkpoint
      where coalesce(checkpoint->>'status', 'scheduled') = 'scheduled'
        and coalesce(ca.measuring_started_at, ca.published_at)
          + (coalesce(nullif(checkpoint->>'day', '')::int, 0) * interval '1 day') <= sqlc.arg(now_at)::timestamptz
    )
  )
order by ca.published_at asc nulls first, ca.updated_at asc
limit sqlc.arg(limit_rows)
for update of ca skip locked;

-- name: BindLegacyMeasuringContentActionPolicy :one
update content_actions set
  status = status,
  updated_at = updated_at
where id = sqlc.arg(id)
  and project_id = sqlc.arg(project_id)
  and status = 'measuring'
  and canonical_read_only = false
  and measuring_started_at is null
returning *;

-- name: UpdateContentActionOutcomeSummary :one
update content_actions set
  status = sqlc.arg(status)::text,
  outcome_summary = sqlc.arg(outcome_summary)::jsonb,
  measurement_window = sqlc.arg(measurement_window)::jsonb,
  measurement_terminal_reason = coalesce(sqlc.narg(measurement_terminal_reason), measurement_terminal_reason),
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: InsertActionMeasurementCheckpoint :exec
insert into action_measurements
  (project_id, content_action_id, article_id, checkpoint_day, window_start, window_end,
   seo_metrics, ga4_metrics, geo_metrics, execution_metrics, outcome_label, outcome_reason,
   attribution_confidence, confounders, computed_at, checkpoint_role,
   measurement_policy_version, checkpoint_attempt, data_quality_state, source_freshness)
values (
  sqlc.arg(project_id),
  sqlc.arg(content_action_id),
  sqlc.narg(article_id),
  sqlc.arg(checkpoint_day),
  sqlc.narg(window_start),
  sqlc.narg(window_end),
  sqlc.arg(seo_metrics)::jsonb,
  sqlc.arg(ga4_metrics)::jsonb,
  sqlc.arg(geo_metrics)::jsonb,
  sqlc.arg(execution_metrics)::jsonb,
  sqlc.arg(outcome_label),
  sqlc.arg(outcome_reason),
  sqlc.arg(attribution_confidence),
  sqlc.arg(confounders)::jsonb,
  sqlc.arg(computed_at),
  sqlc.arg(checkpoint_role),
  sqlc.arg(measurement_policy_version),
  sqlc.arg(checkpoint_attempt),
  sqlc.arg(data_quality_state),
  sqlc.arg(source_freshness)::jsonb
)
on conflict (project_id, content_action_id, checkpoint_day) do nothing;

-- name: ListActionMeasurementsForProject :many
select * from action_measurements
where project_id = sqlc.arg(project_id)
  and (sqlc.narg(content_action_id)::uuid is null or content_action_id = sqlc.narg(content_action_id))
order by computed_at desc, checkpoint_day desc
limit sqlc.arg(limit_rows);

-- name: ListActionMeasurementsForAction :many
select * from action_measurements
where project_id = sqlc.arg(project_id)
  and content_action_id = sqlc.arg(content_action_id)
order by checkpoint_day asc, computed_at asc;

-- name: ListResultsActionRows :many
select
  ca.id,
  ca.project_id,
  ca.opportunity_id,
  ca.action_type,
  ca.status,
  ca.target_article_id,
  ca.target_url,
  ca.normalized_target_url,
  ca.target_content_hash_before,
  ca.target_content_hash_after,
  ca.draft_article_id,
  ca.baseline_window,
  ca.measurement_window,
  ca.measurement_policy_version,
  ca.measurement_policy,
  ca.measuring_started_at,
  ca.absolute_terminal_at,
  ca.measurement_terminal_reason,
  ca.published_at,
  ca.outcome_summary,
  ca.created_at,
  ca.updated_at,
  ca.asset_type,
  ca.target_surface_id,
  ca.risk_reasons,
  ca.evidence_snapshot,
  ca.input_snapshot,
  ca.output_snapshot,
  ca.diff_snapshot,
  ca.review_required,
  ca.approved_by,
  ca.approved_at,
  ca.verified_at,
  ca.verification_snapshot,
  ca.approval_source,
  ca.routing_source,
  ca.work_type,
  coalesce(so.type, '')::text as opportunity_type,
  so.query as opportunity_query,
  so.page_url as opportunity_page_url,
  so.normalized_page_url as opportunity_normalized_page_url,
  so.recommended_action as opportunity_recommended_action,
  so.expected_impact as opportunity_expected_impact,
  t.title as topic_title,
  a.status as draft_article_status,
  a.canonical_url as draft_article_canonical_url
from content_actions ca
left join seo_opportunities so
  on so.id = ca.opportunity_id
  and so.project_id = ca.project_id
left join topics t
  on t.source_content_action_id = ca.id
  and t.project_id = ca.project_id
left join articles a
  on a.id = ca.draft_article_id
  and a.project_id = ca.project_id
where ca.project_id = sqlc.arg(project_id)
  and (sqlc.arg(status)::text = '' or ca.status = sqlc.arg(status))
  and (sqlc.arg(cursor_updated_at)::timestamptz is null or ca.updated_at < sqlc.arg(cursor_updated_at))
  and (
    ca.status in ('published','measuring','completed','verification_failed','recovery_required')
    or ca.published_at is not null
    or ca.verified_at is not null
    or exists (
      select 1
      from action_measurements am
      where am.project_id = ca.project_id
        and am.content_action_id = ca.id
    )
  )
order by coalesce(ca.published_at, ca.verified_at, ca.updated_at) desc, ca.created_at desc
limit sqlc.arg(limit_rows);

-- name: GetResultsActionRow :one
select
  ca.id,
  ca.project_id,
  ca.opportunity_id,
  ca.action_type,
  ca.status,
  ca.target_article_id,
  ca.target_url,
  ca.normalized_target_url,
  ca.target_content_hash_before,
  ca.target_content_hash_after,
  ca.draft_article_id,
  ca.baseline_window,
  ca.measurement_window,
  ca.measurement_policy_version,
  ca.measurement_policy,
  ca.measuring_started_at,
  ca.absolute_terminal_at,
  ca.measurement_terminal_reason,
  ca.published_at,
  ca.outcome_summary,
  ca.created_at,
  ca.updated_at,
  ca.asset_type,
  ca.target_surface_id,
  ca.risk_reasons,
  ca.evidence_snapshot,
  ca.input_snapshot,
  ca.output_snapshot,
  ca.diff_snapshot,
  ca.review_required,
  ca.approved_by,
  ca.approved_at,
  ca.verified_at,
  ca.verification_snapshot,
  ca.approval_source,
  ca.routing_source,
  ca.work_type,
  coalesce(so.type, '')::text as opportunity_type,
  so.query as opportunity_query,
  so.page_url as opportunity_page_url,
  so.normalized_page_url as opportunity_normalized_page_url,
  so.recommended_action as opportunity_recommended_action,
  so.expected_impact as opportunity_expected_impact,
  t.title as topic_title,
  a.status as draft_article_status,
  a.canonical_url as draft_article_canonical_url
from content_actions ca
left join seo_opportunities so
  on so.id = ca.opportunity_id
  and so.project_id = ca.project_id
left join topics t
  on t.source_content_action_id = ca.id
  and t.project_id = ca.project_id
left join articles a
  on a.id = ca.draft_article_id
  and a.project_id = ca.project_id
where ca.id = sqlc.arg(id)
  and ca.project_id = sqlc.arg(project_id);

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
  and not exists (
    select 1 from product_writer_authority authority
    where authority.project_id = ca.project_id and authority.product = 'opportunities' and authority.write_fenced = true
  )
  and not exists (
    select 1 from growth_opportunity_work_aliases alias
    where alias.project_id = ca.project_id and alias.legacy_opportunity_id = ca.opportunity_id
      and alias.disposition in ('duplicate','doctor_merge')
  )
  and t.id is null
  and coalesce(ca.work_type, '') <> 'fix_site_issue'
  and lower(coalesce(ca.asset_type, '') || ' ' || coalesce(ca.action_type, '')) not like any (
    array[
      '%internal_link_patch%',
      '%schema_patch%',
      '%sitemap_update%',
      '%technical_fix%',
      '%technical seo%',
      '%internal link%',
      '%robots%',
      '%canonical%',
      '%crawler%'
    ]
  )
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
select seo_opportunities.type, seo_opportunities.status, count(*)::bigint as count
from seo_opportunities
where seo_opportunities.project_id = $1
  and not exists (
    select 1 from growth_opportunity_work_aliases alias
    where alias.project_id = seo_opportunities.project_id
      and alias.legacy_opportunity_id = seo_opportunities.id
      and alias.disposition in ('duplicate','doctor_merge')
  )
group by seo_opportunities.type, seo_opportunities.status
order by seo_opportunities.type, seo_opportunities.status;

-- name: ContentActionCounts :many
select status, count(*)::bigint as count
from content_actions
where project_id = $1
group by status
order by status;
