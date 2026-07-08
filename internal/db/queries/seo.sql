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
   linked_content_action_id, first_seen_at, last_seen_at)
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
  sqlc.arg(seen_at)
)
on conflict (project_id, finding_key) where status = 'active' do update set
  run_id = excluded.run_id,
  severity = excluded.severity,
  category = excluded.category,
  issue_type = excluded.issue_type,
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
  and not (finding_key = any(sqlc.arg(active_keys)::text[]));

-- name: ListSEODoctorFindingsForRun :many
select * from seo_doctor_findings
where project_id = $1
  and run_id = $2
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

-- name: UpsertSEOOpportunity :one
insert into seo_opportunities
  (project_id, type, status, priority_score, confidence, page_url, normalized_page_url,
   article_id, topic_id, query, evidence, recommended_action, expected_impact, effort,
   risk_level, created_by_run_id, opportunity_key)
values (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
  encode(digest(
    $1::uuid::text || '|' || $2 || '|' || coalesce($7, '') || '|' || coalesce($10, '') || '|' ||
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
   normalized_target_url, target_content_hash_before, baseline_window, measurement_window,
   approval_source, routing_source, work_type)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
on conflict (project_id, opportunity_id, action_type) do update set
  status = excluded.status,
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
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id) and status in ('open','snoozed')
returning *;

-- name: UnsnoozeSEOOpportunity :one
update seo_opportunities set
  status = 'open',
  snoozed_until = null,
  unsnoozed_at = now(),
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id) and status = 'snoozed'
returning *;

-- name: WakeDueSnoozedSEOOpportunities :execrows
update seo_opportunities set
  status = 'open',
  unsnoozed_at = now(),
  updated_at = now()
where project_id = $1
  and status = 'snoozed'
  and snoozed_until is not null
  and snoozed_until <= now();

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
  and (
    ca.published_at is null
    or ca.measurement_window = '{}'::jsonb
    or exists (
      select 1
      from jsonb_array_elements(coalesce(ca.measurement_window->'checkpoints', '[]'::jsonb)) checkpoint
      where coalesce(checkpoint->>'status', 'scheduled') = 'scheduled'
        and ca.published_at + (coalesce(nullif(checkpoint->>'day', '')::int, 0) * interval '1 day') <= sqlc.arg(now_at)::timestamptz
    )
  )
order by ca.published_at asc nulls first, ca.updated_at asc
limit sqlc.arg(limit_rows)
for update of ca skip locked;

-- name: UpdateContentActionOutcomeSummary :one
update content_actions set
  status = sqlc.arg(status)::text,
  outcome_summary = sqlc.arg(outcome_summary)::jsonb,
  measurement_window = sqlc.arg(measurement_window)::jsonb,
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: UpsertActionMeasurement :one
insert into action_measurements
  (project_id, content_action_id, article_id, checkpoint_day, window_start, window_end,
   seo_metrics, ga4_metrics, geo_metrics, execution_metrics, outcome_label, outcome_reason,
   attribution_confidence, confounders, computed_at)
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
  sqlc.arg(computed_at)
)
on conflict (project_id, content_action_id, checkpoint_day) do update set
  article_id = coalesce(excluded.article_id, action_measurements.article_id),
  window_start = excluded.window_start,
  window_end = excluded.window_end,
  seo_metrics = excluded.seo_metrics,
  ga4_metrics = excluded.ga4_metrics,
  geo_metrics = excluded.geo_metrics,
  execution_metrics = excluded.execution_metrics,
  outcome_label = excluded.outcome_label,
  outcome_reason = excluded.outcome_reason,
  attribution_confidence = excluded.attribution_confidence,
  confounders = excluded.confounders,
  computed_at = excluded.computed_at,
  updated_at = now()
returning *;

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
