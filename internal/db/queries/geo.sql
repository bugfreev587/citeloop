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

-- name: GetLatestGEOCrawlerAuditRun :one
select * from geo_runs
where project_id = $1 and agent = 'geo_crawler_audit'
order by started_at desc, id desc
limit 1;

-- name: ListAICrawlerAccessSnapshotsForRun :many
select * from ai_crawler_access_snapshots
where project_id = sqlc.arg(project_id) and run_id = sqlc.arg(run_id)
order by normalized_page_url asc, target_user_agent asc, evidence_type asc;

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

-- name: CreateGEOPromptSet :one
insert into geo_prompt_sets (project_id, name, status, locale, created_by_run_id)
values ($1, $2, $3, $4, $5)
returning *;

-- name: ListGEOPromptSets :many
select * from geo_prompt_sets
where project_id = sqlc.arg(project_id)
  and (sqlc.arg(status)::text = '' or status = sqlc.arg(status))
order by updated_at desc, created_at desc;

-- name: GetGEOPromptSetForProject :one
select * from geo_prompt_sets
where id = $1 and project_id = $2;

-- name: UpdateGEOPromptSet :one
update geo_prompt_sets set
  name = $3,
  status = $4,
  locale = $5,
  updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: CreateGEOPrompt :one
insert into geo_prompts
  (project_id, prompt_set_id, prompt_text, intent_type, target_persona, target_topic,
   locale, target_engines, priority, source, status)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
on conflict (project_id, prompt_set_id, prompt_text, locale) do update set
  intent_type = excluded.intent_type,
  target_persona = excluded.target_persona,
  target_topic = excluded.target_topic,
  target_engines = excluded.target_engines,
  priority = excluded.priority,
  source = excluded.source,
  status = excluded.status,
  updated_at = now()
returning *;

-- name: ListGEOPrompts :many
select * from geo_prompts
where project_id = sqlc.arg(project_id)
  and (sqlc.narg(prompt_set_id)::uuid is null or prompt_set_id = sqlc.narg(prompt_set_id))
  and (sqlc.arg(status)::text = '' or status = sqlc.arg(status))
order by priority desc, created_at asc;

-- name: ListActiveGEOPrompts :many
select p.*
from geo_prompts p
join geo_prompt_sets ps on ps.id = p.prompt_set_id
where p.project_id = $1
  and p.status = 'active'
  and ps.status = 'active'
  and lower(p.prompt_text) !~
    '(api[ _-]?key|access[ _-]?token|secret|credential|password|database|postgres|mysql|redis|migration|deploy(ment)?|railway|vercel|github[ _-]?token|aes|encrypt|private[ _-]?key|private[ _-]?repo|token[ _-]?gate|kubernetes|docker|internal[ _-]?diagnostic)'
order by p.priority desc, p.created_at asc;

-- name: MarkGEOPromptsObserved :many
update geo_prompts
set last_observed_at = sqlc.arg(observed_at),
    next_observed_at = sqlc.arg(observed_at) + interval '7 days',
    observation_count = observation_count + 1,
    targeted_reason = '',
    updated_at = now()
where project_id = sqlc.arg(project_id)
  and id = any(sqlc.arg(prompt_ids)::uuid[])
returning *;

-- name: ArchiveGEOPromptOverflow :many
update geo_prompts
set status = 'archived', archived_reason = sqlc.arg(archived_reason), updated_at = now()
where project_id = sqlc.arg(project_id)
  and id = any(sqlc.arg(prompt_ids)::uuid[])
returning *;

-- name: GetGEOPromptForProject :one
select * from geo_prompts
where id = $1 and project_id = $2;

-- name: UpdateGEOPrompt :one
update geo_prompts set
  prompt_text = $3,
  intent_type = $4,
  target_persona = $5,
  target_topic = $6,
  locale = $7,
  target_engines = $8,
  priority = $9,
  source = $10,
  status = $11,
  updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: UpsertGEOCompetitor :one
insert into geo_competitors (project_id, name, domains, aliases, source, status)
values ($1, $2, $3, $4, $5, $6)
on conflict (project_id, name_key) do update set
  domains = excluded.domains,
  aliases = excluded.aliases,
  source = excluded.source,
  status = excluded.status,
  updated_at = now()
returning *;

-- name: ListGEOCompetitors :many
select * from geo_competitors
where project_id = sqlc.arg(project_id)
  and (sqlc.arg(status)::text = '' or status = sqlc.arg(status))
order by status asc, name asc;

-- name: GetGEOCompetitorForProject :one
select * from geo_competitors
where id = $1 and project_id = $2;

-- name: UpdateGEOCompetitor :one
update geo_competitors set
  name = $3,
  domains = $4,
  aliases = $5,
  source = $6,
  status = $7,
  updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: UpsertGEOExternalSurface :one
insert into geo_external_surfaces
  (project_id, url, normalized_url, platform, surface_type, owner_type,
   canonical_target_url, backlink_state, last_http_status, last_cited_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
on conflict (project_id, normalized_url) do update set
  url = excluded.url,
  platform = excluded.platform,
  surface_type = excluded.surface_type,
  owner_type = excluded.owner_type,
  canonical_target_url = excluded.canonical_target_url,
  backlink_state = excluded.backlink_state,
  last_http_status = excluded.last_http_status,
  last_cited_at = coalesce(excluded.last_cited_at, geo_external_surfaces.last_cited_at),
  updated_at = now()
returning *;

-- name: ListGEOExternalSurfaces :many
select * from geo_external_surfaces
where project_id = sqlc.arg(project_id)
  and (sqlc.arg(owner_type)::text = '' or owner_type = sqlc.arg(owner_type))
order by owner_type asc, updated_at desc;

-- name: UpdateGEOExternalSurfaceMetadata :one
update geo_external_surfaces set
  source_url = coalesce(sqlc.narg(source_url)::text, source_url),
  canonical_status = sqlc.arg(canonical_status)::text,
  indexability_status = sqlc.arg(indexability_status)::text,
  publication_status = sqlc.arg(publication_status)::text,
  owner_confidence = sqlc.arg(owner_confidence)::text,
  last_verified_at = coalesce(sqlc.narg(last_verified_at)::timestamptz, last_verified_at),
  verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
  related_action_ids = sqlc.arg(related_action_ids)::jsonb,
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: ListProjectOwnedGEOExternalSurfaces :many
select * from geo_external_surfaces
where project_id = $1 and owner_type = 'project'
order by updated_at desc;

-- name: CreateGEOObservation :one
insert into geo_observations
  (project_id, run_id, prompt_id, engine, locale, source_type, brand_mentioned,
   brand_position, project_citation_count, project_citation_rank_best,
   project_cited_surface_ids, cited_urls, competitor_mentions, competitor_citations,
   observation_state, answer_summary, evidence_snippets, confidence, observed_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
returning *;

-- name: ListGEOObservations :many
select * from geo_observations
where project_id = sqlc.arg(project_id)
  and (sqlc.narg(prompt_id)::uuid is null or prompt_id = sqlc.narg(prompt_id))
  and (sqlc.arg(engine)::text = '' or engine = sqlc.arg(engine))
  and (sqlc.arg(source_type)::text = '' or source_type = sqlc.arg(source_type))
order by observed_at desc
limit sqlc.arg(limit_rows);

-- name: ListGEOObservationsForRun :many
select * from geo_observations
where project_id = $1 and run_id = $2
order by observed_at asc;

-- name: CreateGEOVisibilityScore :one
insert into geo_visibility_scores
  (project_id, run_id, score, coverage, confidence, breakdown,
   prompt_count_total, prompt_count_observed, engine_count_observed, computed_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
returning *;

-- name: GetLatestGEOVisibilityScore :one
select * from geo_visibility_scores
where project_id = $1
order by computed_at desc
limit 1;

-- name: ListGEOVisibilityScores :many
select * from geo_visibility_scores
where project_id = $1
order by computed_at desc
limit $2;

-- name: UpsertGEOObservationOpportunity :one
with updated as (
  update seo_opportunities so set
    priority_score = $4,
    confidence = $5,
    page_url = $6,
    normalized_page_url = $7,
    evidence = so.evidence || $9,
    recommended_action = $10,
    expected_impact = $11,
    effort = $12,
    risk_level = $13,
    updated_at = now()
  where so.project_id = $1
    and so.type = $2
    and so.status in ('open','accepted','converted')
    and so.normalized_page_url = $7
    and coalesce(so.query, '') = coalesce($8, '')
  returning *
), inserted as (
  insert into seo_opportunities
    (project_id, type, status, priority_score, confidence, page_url, normalized_page_url,
     query, evidence, recommended_action, expected_impact, effort, risk_level, created_by_run_id)
  select $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, null
  where not exists (select 1 from updated)
  returning *
)
select * from updated
union all
select * from inserted;

-- name: CreateGEOAssetBrief :one
insert into geo_asset_briefs
  (project_id, opportunity_id, asset_type, status, target_prompts, required_evidence,
   recommended_outline, internal_link_plan, publication_surface, created_by_run_id)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
on conflict (project_id, opportunity_id) do update set
  asset_type = excluded.asset_type,
  status = excluded.status,
  target_prompts = excluded.target_prompts,
  required_evidence = excluded.required_evidence,
  recommended_outline = excluded.recommended_outline,
  internal_link_plan = excluded.internal_link_plan,
  publication_surface = excluded.publication_surface,
  created_by_run_id = coalesce(geo_asset_briefs.created_by_run_id, excluded.created_by_run_id),
  updated_at = now()
returning *;

-- name: ListGEOAssetBriefs :many
select * from geo_asset_briefs
where project_id = sqlc.arg(project_id)
  and (sqlc.arg(status)::text = '' or status = sqlc.arg(status))
order by updated_at desc
limit sqlc.arg(limit_rows);

-- name: GetGEOAssetBriefForProject :one
select * from geo_asset_briefs
where id = $1 and project_id = $2;

-- name: UpdateGEOAssetBriefStatus :one
update geo_asset_briefs set
  status = $3,
  updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: UpdateGEOAssetBriefMultiSurfaceMetadata :one
update geo_asset_briefs set
  target_queries = sqlc.arg(target_queries)::jsonb,
  target_personas = sqlc.arg(target_personas)::jsonb,
  expected_citation_mechanism = sqlc.arg(expected_citation_mechanism)::text,
  source_type = sqlc.arg(source_type)::text,
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;
