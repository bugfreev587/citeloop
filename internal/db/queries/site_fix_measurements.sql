-- name: CreateSiteFixMeasurement :one
select * from create_site_fix_measurement_idempotently(
  sqlc.arg(id)::uuid,
  sqlc.arg(project_id)::uuid,
  sqlc.arg(site_fix_id)::uuid,
  sqlc.arg(creation_idempotency_key)::text,
  sqlc.arg(target_url)::text,
  sqlc.arg(normalized_target_url)::text,
  sqlc.narg(target_query)::text,
  sqlc.arg(target_identity)::jsonb,
  sqlc.arg(fix_type)::text,
  sqlc.arg(impact_mode)::text,
  sqlc.arg(classifier_version)::text,
  sqlc.arg(decision_origin)::text,
  sqlc.arg(decision_confidence)::text,
  sqlc.arg(prospective_observation)::boolean,
  sqlc.arg(growth_hypothesis)::text,
  sqlc.arg(primary_metric)::text,
  sqlc.arg(secondary_metrics)::jsonb,
  sqlc.arg(measurement_policy_version)::text,
  sqlc.arg(measurement_policy_snapshot)::jsonb,
  sqlc.arg(baseline_window)::jsonb,
  sqlc.arg(baseline_snapshot)::jsonb,
  sqlc.arg(baseline_status)::text,
  sqlc.arg(status)::text,
  sqlc.arg(attribution_confidence)::text,
  sqlc.narg(results_deep_link)::text
);

-- name: GetSiteFixMeasurement :one
select * from site_fix_measurements
where project_id = sqlc.arg(project_id) and id = sqlc.arg(id);

-- name: GetLatestSiteFixMeasurementForFix :one
select * from site_fix_measurements
where project_id = sqlc.arg(project_id) and site_fix_id = sqlc.arg(site_fix_id)
order by measurement_generation desc
limit 1;

-- name: GetSiteFixMeasurementGeneration :one
select * from site_fix_measurements
where project_id = sqlc.arg(project_id)
  and site_fix_id = sqlc.arg(site_fix_id)
  and measurement_generation = sqlc.arg(measurement_generation);

-- name: UpdateSiteFixMeasurementBaseline :one
update site_fix_measurements
set baseline_snapshot = sqlc.arg(baseline_snapshot),
    baseline_status = sqlc.arg(baseline_status),
    status = sqlc.arg(status),
    updated_at = now()
where project_id = sqlc.arg(project_id)
  and id = sqlc.arg(id)
  and baseline_status in ('planned','collecting')
returning *;

-- name: ActivateSiteFixMeasurement :one
update site_fix_measurements
set status = 'observing',
    started_at = coalesce(started_at, sqlc.arg(started_at)),
    absolute_terminal_at = coalesce(
      absolute_terminal_at,
      sqlc.arg(started_at) + (
        ((measurement_policy_snapshot->>'max_measuring_duration_days')::int
          + (measurement_policy_snapshot->>'terminalization_grace_period_days')::int) * interval '1 day'
      )
    ),
    results_deep_link = coalesce(results_deep_link, sqlc.arg(results_deep_link)),
    updated_at = now()
where project_id = sqlc.arg(project_id)
  and site_fix_id = sqlc.arg(site_fix_id)
  and measurement_generation = sqlc.arg(measurement_generation)
  and status = 'ready'
returning *;

-- name: ClaimDueSiteFixMeasurement :one
select measurement.*
from site_fix_measurements measurement
where measurement.status in ('ready','observing')
  and (
    measurement.absolute_terminal_at <= sqlc.arg(now_at)
    or exists (
      select 1
      from site_fix_measurement_checkpoints checkpoint
      where checkpoint.project_id = measurement.project_id
        and checkpoint.measurement_id = measurement.id
        and checkpoint.scheduled_at <= sqlc.arg(now_at)
        and checkpoint.next_attempt_at <= sqlc.arg(now_at)
        and checkpoint.computed_at is null
    )
  )
order by measurement.absolute_terminal_at, measurement.created_at
limit 1
for update skip locked;

-- name: GetOrCreateSiteFixMeasurementCheckpoint :one
insert into site_fix_measurement_checkpoints (
  id, project_id, measurement_id, checkpoint_key, checkpoint_role,
  scheduled_at, window_start, window_end, attempt_number,
  required_data_sources, data_availability, minimum_sample,
  seo_metrics, ga4_metrics, geo_metrics, execution_metrics, guardrail_results,
  outcome_label, outcome_reason, attribution_confidence,
  computed_at, failure_reason, retry_classification,
  evaluation_attempt_count, next_attempt_at
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(measurement_id), sqlc.arg(checkpoint_key), sqlc.arg(checkpoint_role),
  sqlc.arg(scheduled_at), sqlc.arg(window_start), sqlc.arg(window_end), sqlc.arg(attempt_number),
  sqlc.arg(required_data_sources), sqlc.arg(data_availability), sqlc.arg(minimum_sample),
  sqlc.arg(seo_metrics), sqlc.arg(ga4_metrics), sqlc.arg(geo_metrics), sqlc.arg(execution_metrics), sqlc.arg(guardrail_results),
  sqlc.narg(outcome_label), sqlc.narg(outcome_reason), sqlc.arg(attribution_confidence),
  sqlc.narg(computed_at), sqlc.narg(failure_reason), sqlc.arg(retry_classification),
  sqlc.arg(evaluation_attempt_count), sqlc.arg(next_attempt_at)
)
on conflict (measurement_id, checkpoint_key, attempt_number) do update
set id = site_fix_measurement_checkpoints.id
returning *;

-- name: CompleteSiteFixMeasurementCheckpoint :one
select * from complete_site_fix_measurement_checkpoint_idempotently(
  sqlc.arg(project_id), sqlc.arg(measurement_id), sqlc.arg(checkpoint_key), sqlc.arg(attempt_number),
  sqlc.arg(data_availability), sqlc.arg(seo_metrics), sqlc.arg(ga4_metrics), sqlc.arg(geo_metrics),
  sqlc.arg(execution_metrics), sqlc.arg(guardrail_results), sqlc.narg(outcome_label),
  sqlc.narg(outcome_reason), sqlc.arg(attribution_confidence), sqlc.arg(computed_at),
  sqlc.narg(failure_reason), sqlc.arg(retry_classification)
);

-- name: DeferSiteFixMeasurementCheckpoint :one
update site_fix_measurement_checkpoints checkpoint
set evaluation_attempt_count = evaluation_attempt_count + 1,
    next_attempt_at = sqlc.arg(next_attempt_at),
    failure_reason = sqlc.arg(failure_reason),
    retry_classification = 'retryable'
where checkpoint.project_id=sqlc.arg(project_id)
  and checkpoint.measurement_id=sqlc.arg(measurement_id)
  and checkpoint.checkpoint_key=sqlc.arg(checkpoint_key)
  and checkpoint.attempt_number=sqlc.arg(attempt_number)
  and checkpoint.computed_at is null
  and checkpoint.next_attempt_at < sqlc.arg(next_attempt_at)
returning *;

-- name: CloseSiteFixMeasurementOpenCheckpoints :many
update site_fix_measurement_checkpoints checkpoint
set computed_at=sqlc.arg(computed_at),
    outcome_label=null,
    outcome_reason='skipped_after_terminal_outcome',
    attribution_confidence='none',
    failure_reason='closed_after_terminal_outcome',
    retry_classification='terminal',
    evaluation_attempt_count=evaluation_attempt_count + 1
where checkpoint.project_id=sqlc.arg(project_id)
  and checkpoint.measurement_id=sqlc.arg(measurement_id)
  and checkpoint.computed_at is null
returning *;

-- name: ListSiteFixMeasurementCheckpoints :many
select * from site_fix_measurement_checkpoints
where project_id = sqlc.arg(project_id) and measurement_id = sqlc.arg(measurement_id)
order by scheduled_at, checkpoint_key, attempt_number;

-- name: TerminalizeSiteFixMeasurement :one
update site_fix_measurements
set status = 'terminal',
    terminal_outcome = sqlc.arg(terminal_outcome),
    outcome_reason = sqlc.arg(outcome_reason),
    attribution_confidence = sqlc.arg(attribution_confidence),
    confounders = sqlc.arg(confounders),
    updated_at = now()
where project_id = sqlc.arg(project_id)
  and id = sqlc.arg(id)
  and status <> 'terminal'
returning *;

-- name: GetOrCreateSiteFixMeasurementTerminalOutcome :one
insert into site_fix_measurement_terminal_outcomes (
  id, project_id, measurement_id, outcome_label, record_kind, terminal_reason,
  measurement_policy_version, baseline_snapshot, checkpoint_snapshot, outcome_snapshot
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(measurement_id), sqlc.arg(outcome_label), sqlc.arg(record_kind), sqlc.arg(terminal_reason),
  sqlc.arg(measurement_policy_version), sqlc.arg(baseline_snapshot), sqlc.arg(checkpoint_snapshot), sqlc.arg(outcome_snapshot)
)
on conflict (project_id, measurement_id) do update
set id = site_fix_measurement_terminal_outcomes.id
returning *;

-- name: GetOrCreateSiteFixMeasurementLearning :one
insert into site_fix_measurement_learnings (
  id, project_id, terminal_outcome_id, measurement_id, learning_summary, applicability, learning_version
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(terminal_outcome_id), sqlc.arg(measurement_id),
  sqlc.arg(learning_summary), sqlc.arg(applicability), sqlc.arg(learning_version)
)
on conflict (project_id, measurement_id) do update
set id = site_fix_measurement_learnings.id
returning *;

-- name: GetOrCreateSiteFixMeasurementQualityRecord :one
insert into site_fix_measurement_quality_records (
  id, project_id, terminal_outcome_id, measurement_id,
  data_quality_state, quality_gaps, recommendation, quality_version
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(terminal_outcome_id), sqlc.arg(measurement_id),
  sqlc.arg(data_quality_state), sqlc.arg(quality_gaps), sqlc.arg(recommendation), sqlc.arg(quality_version)
)
on conflict (project_id, measurement_id) do update
set id = site_fix_measurement_quality_records.id
returning *;

-- name: EnqueueSiteFixMeasurementHandoff :one
insert into site_fix_measurement_handoff_outbox (
  id, project_id, site_fix_id, measurement_generation, idempotency_key,
  max_attempts, next_attempt_at, occurred_at
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(site_fix_id), sqlc.arg(measurement_generation), sqlc.arg(idempotency_key),
  sqlc.arg(max_attempts), sqlc.arg(next_attempt_at), sqlc.arg(occurred_at)
)
on conflict (project_id, site_fix_id, measurement_generation) do update
set idempotency_key = site_fix_measurement_handoff_outbox.idempotency_key
returning *;

-- name: ClaimSiteFixMeasurementHandoff :one
with due as (
  select candidate.id
  from site_fix_measurement_handoff_outbox candidate
  where (
      (candidate.status in ('pending','failed_retryable') and candidate.next_attempt_at <= sqlc.arg(now_at))
      or (candidate.status = 'processing' and candidate.locked_until <= sqlc.arg(now_at))
    )
    and candidate.attempt_count < candidate.max_attempts
  order by candidate.next_attempt_at, candidate.created_at
  limit 1
  for update skip locked
)
update site_fix_measurement_handoff_outbox outbox
set status = 'processing',
    attempt_count = attempt_count + 1,
    lock_token = sqlc.arg(lock_token),
    locked_until = sqlc.arg(locked_until),
    updated_at = now()
from due
where outbox.id = due.id
returning outbox.*;

-- name: CompleteSiteFixMeasurementHandoff :one
update site_fix_measurement_handoff_outbox
set status = 'completed',
    lock_token = null,
    locked_until = null,
    completed_at = now(),
    last_error_classification = null,
    last_error = null,
    updated_at = now()
where id = sqlc.arg(id)
  and project_id = sqlc.arg(project_id)
  and status = 'processing'
  and lock_token = sqlc.arg(lock_token)
returning *;

-- name: TerminalizeExpiredSiteFixMeasurementHandoffs :many
update site_fix_measurement_handoff_outbox
set status = 'failed_terminal',
    lock_token = null,
    locked_until = null,
    last_error_classification = 'lease_expired_after_attempt_limit',
    last_error = 'processing lease expired after the finite handoff attempt limit',
    updated_at = now()
where status = 'processing'
  and locked_until <= sqlc.arg(now_at)
  and attempt_count >= max_attempts
returning *;

-- name: ReconcileVerifiedSiteFixMeasurementHandoffs :many
with candidates as (
  select fix.project_id, fix.id as site_fix_id, measurement.measurement_generation,
         case when measurement.prospective_observation then measurement.created_at
              else coalesce(fix.verified_at, measurement.created_at) end as occurred_at
  from site_fixes fix
  join site_fix_measurements measurement
    on measurement.project_id = fix.project_id
   and measurement.site_fix_id = fix.id
   and measurement.status <> 'terminal'
  where fix.status = 'verified'
    and not exists (
      select 1 from site_fix_measurement_handoff_outbox existing
      where existing.project_id = fix.project_id
        and existing.site_fix_id = fix.id
        and existing.measurement_generation = measurement.measurement_generation
    )
  order by fix.project_id, fix.id
  limit least(greatest(sqlc.arg(limit_rows)::int, 1), 100)
)
insert into site_fix_measurement_handoff_outbox (
  id, project_id, site_fix_id, measurement_generation, idempotency_key,
  max_attempts, next_attempt_at, occurred_at
)
select gen_random_uuid(), candidate.project_id, candidate.site_fix_id,
       candidate.measurement_generation,
       'reconcile:' || candidate.site_fix_id::text || ':' || candidate.measurement_generation::text,
       8, sqlc.arg(now_at), candidate.occurred_at
from candidates candidate
on conflict (project_id, site_fix_id, measurement_generation) do nothing
returning *;

-- name: ClaimFailedTerminalSiteFixMeasurementHandoffAlert :one
with candidate as (
  select id
  from site_fix_measurement_handoff_outbox
  where site_fix_measurement_handoff_outbox.status='failed_terminal'
    and site_fix_measurement_handoff_outbox.alert_notified_at is null
    and (site_fix_measurement_handoff_outbox.alert_lock_token is null or site_fix_measurement_handoff_outbox.alert_locked_until <= sqlc.arg(now_at))
  order by updated_at, id
  for update skip locked
  limit 1
)
update site_fix_measurement_handoff_outbox outbox
set alert_lock_token=sqlc.arg(alert_lock_token),
    alert_locked_until=sqlc.arg(alert_locked_until),
    updated_at=now()
from candidate
where outbox.id=candidate.id
returning outbox.*;

-- name: CompleteFailedTerminalSiteFixMeasurementHandoffAlert :one
update site_fix_measurement_handoff_outbox
set alert_notified_at=sqlc.arg(alert_notified_at),
    alert_lock_token=null,
    alert_locked_until=null,
    updated_at=now()
where id=sqlc.arg(id)
  and project_id=sqlc.arg(project_id)
  and status='failed_terminal'
  and alert_notified_at is null
  and alert_lock_token=sqlc.arg(alert_lock_token)
returning *;

-- name: ListVerifiedRequiredSiteFixesMissingMeasurement :many
select fix.*
from site_fixes fix
where fix.status = 'verified'
  and fix.measurement_policy = 'measurement_required'
  and not exists (
    select 1 from site_fix_measurements measurement
    where measurement.project_id = fix.project_id
      and measurement.site_fix_id = fix.id
      and measurement.status <> 'terminal'
  )
order by fix.updated_at, fix.id
limit least(greatest(sqlc.arg(limit_rows)::int, 1), 100);

-- name: RetrySiteFixMeasurementHandoff :one
update site_fix_measurement_handoff_outbox
set status = case when attempt_count >= max_attempts then 'failed_terminal' else 'failed_retryable' end,
    lock_token = null,
    locked_until = null,
    next_attempt_at = sqlc.arg(next_attempt_at),
    last_error_classification = sqlc.arg(last_error_classification),
    last_error = sqlc.arg(last_error),
    updated_at = now()
where id = sqlc.arg(id)
  and project_id = sqlc.arg(project_id)
  and status = 'processing'
  and lock_token = sqlc.arg(lock_token)
returning *;

-- name: ListSiteFixMeasurementsForResults :many
select measurement.*,
  'site_fix'::text as source_type,
  measurement.site_fix_id as source_id
from site_fix_measurements measurement
where measurement.project_id = sqlc.arg(project_id)
order by measurement.updated_at desc, measurement.id
limit least(greatest(sqlc.arg(page_limit)::int, 1), 100)
offset greatest(sqlc.arg(page_offset)::int, 0);

-- name: ListResultsFeedRows :many
with feed as (
  select
    'content_action'::text as source_type,
    action.id,
    action.project_id,
    action.status,
    coalesce(action.published_at, action.verified_at, action.updated_at) as activity_at,
    to_jsonb(action) || jsonb_build_object(
      'source_type', 'content_action',
      'opportunity_type', coalesce(opportunity.type, ''),
      'opportunity_query', opportunity.query,
      'opportunity_page_url', opportunity.page_url,
      'opportunity_normalized_page_url', opportunity.normalized_page_url,
      'opportunity_recommended_action', opportunity.recommended_action,
      'opportunity_expected_impact', opportunity.expected_impact,
      'topic_title', topic.title,
      'draft_article_status', article.status,
      'draft_article_canonical_url', article.canonical_url
    ) as payload
  from content_actions action
  left join seo_opportunities opportunity
    on opportunity.id=action.opportunity_id and opportunity.project_id=action.project_id
  left join topics topic
    on topic.source_content_action_id=action.id and topic.project_id=action.project_id
  left join articles article
    on article.id=action.draft_article_id and article.project_id=action.project_id
  where action.project_id=sqlc.arg(project_id)
    and (
      action.status in ('published','measuring','completed','verification_failed','recovery_required')
      or action.published_at is not null
      or action.verified_at is not null
      or exists (
        select 1 from action_measurements action_measurement
        where action_measurement.project_id=action.project_id
          and action_measurement.content_action_id=action.id
      )
    )

  union all

  select
    'site_fix'::text as source_type,
    measurement.id,
    measurement.project_id,
    measurement.status,
    measurement.updated_at as activity_at,
    jsonb_build_object(
      'source_type', 'site_fix',
      'id', measurement.id,
      'project_id', measurement.project_id,
      'site_fix_id', measurement.site_fix_id,
      'measurement_generation', measurement.measurement_generation,
      'status', measurement.status,
      'target_url', measurement.target_url,
      'fix_type', measurement.fix_type,
      'impact_mode', measurement.impact_mode,
      'prospective_observation', measurement.prospective_observation,
      'growth_hypothesis', measurement.growth_hypothesis,
      'primary_metric', measurement.primary_metric,
      'secondary_metrics', measurement.secondary_metrics,
      'baseline_status', measurement.baseline_status,
      'started_at', measurement.started_at,
      'absolute_terminal_at', measurement.absolute_terminal_at,
      'terminal_outcome', measurement.terminal_outcome,
      'outcome_reason', measurement.outcome_reason,
      'attribution_confidence', measurement.attribution_confidence,
      'results_deep_link', '/projects/' || measurement.project_id::text || '/results?source_type=site_fix&measurement=' || measurement.id::text,
      'site_fix_status', fix.status,
      'verified_at', fix.verified_at,
      'created_at', measurement.created_at,
      'updated_at', measurement.updated_at
    ) as payload
  from site_fix_measurements measurement
  join site_fixes fix
    on fix.project_id=measurement.project_id and fix.id=measurement.site_fix_id
  where measurement.project_id=sqlc.arg(project_id)
), filtered as (
  select * from feed
  where (sqlc.arg(status)::text='' or feed.status=sqlc.arg(status))
    and (
      sqlc.narg(legacy_cursor_at)::timestamptz is null
      or feed.activity_at < sqlc.narg(legacy_cursor_at)
    )
    and (
      sqlc.narg(cursor_activity_at)::timestamptz is null
      or feed.activity_at < sqlc.narg(cursor_activity_at)
      or (
        feed.activity_at=sqlc.narg(cursor_activity_at)
        and (
          feed.source_type > sqlc.arg(cursor_source_type)::text
          or (feed.source_type=sqlc.arg(cursor_source_type)::text and feed.id < sqlc.narg(cursor_id)::uuid)
        )
      )
    )
)
select source_type, id, project_id, status, activity_at, payload
from filtered
order by activity_at desc, source_type asc, id desc
limit least(greatest(sqlc.arg(limit_rows)::int, 1), 101);

-- name: GetSiteFixMeasurementResultsDetail :one
select
  measurement.id,
  measurement.project_id,
  measurement.site_fix_id,
  measurement.measurement_generation,
  measurement.status,
  measurement.target_url,
  measurement.fix_type,
  measurement.impact_mode,
  measurement.prospective_observation,
  measurement.growth_hypothesis,
  measurement.primary_metric,
  measurement.secondary_metrics,
  measurement.measurement_policy_version,
  measurement.baseline_status,
  measurement.started_at,
  measurement.absolute_terminal_at,
  measurement.terminal_outcome,
  measurement.outcome_reason,
  measurement.attribution_confidence,
  ('/projects/' || measurement.project_id::text || '/results?source_type=site_fix&measurement=' || measurement.id::text)::text as results_deep_link,
  measurement.created_at,
  measurement.updated_at,
  fix.status as site_fix_status,
  fix.finding_kind,
  fix.target_urls,
  fix.measurement_policy,
  fix.verified_at
from site_fix_measurements measurement
join site_fixes fix
  on fix.project_id=measurement.project_id and fix.id=measurement.site_fix_id
where measurement.project_id=sqlc.arg(project_id)
  and measurement.id=sqlc.arg(measurement_id);

-- name: GetSiteFixMeasurementTerminalOutcome :one
select id, project_id, measurement_id, outcome_label, record_kind, terminal_reason, created_at
from site_fix_measurement_terminal_outcomes
where project_id=sqlc.arg(project_id) and measurement_id=sqlc.arg(measurement_id);

-- name: GetLatestSiteFixMeasurementHandoff :one
select handoff.*
from site_fix_measurement_handoff_outbox handoff
join site_fix_measurements measurement
  on measurement.project_id=handoff.project_id
 and measurement.site_fix_id=handoff.site_fix_id
 and measurement.measurement_generation=handoff.measurement_generation
where handoff.project_id=sqlc.arg(project_id)
  and measurement.id=sqlc.arg(measurement_id)
order by handoff.created_at desc, handoff.id desc
limit 1;

-- name: ListLatestSiteFixMeasurementStatesForFixes :many
select distinct on (measurement.site_fix_id)
  measurement.*,
  coalesce(handoff.status, '')::text as handoff_status
from site_fix_measurements measurement
left join lateral (
  select candidate.status
  from site_fix_measurement_handoff_outbox candidate
  where candidate.project_id=measurement.project_id
    and candidate.site_fix_id=measurement.site_fix_id
    and candidate.measurement_generation=measurement.measurement_generation
  order by candidate.created_at desc, candidate.id desc
  limit 1
) handoff on true
where measurement.project_id=sqlc.arg(project_id)
order by measurement.site_fix_id, measurement.measurement_generation desc, measurement.created_at desc, measurement.id desc;
