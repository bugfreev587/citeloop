-- name: CreateSiteFixMeasurement :one
insert into site_fix_measurements (
  id, project_id, site_fix_id, measurement_generation,
  target_url, normalized_target_url, target_query, target_identity,
  fix_type, impact_mode, classifier_version, decision_origin, decision_confidence,
  prospective_observation, growth_hypothesis, primary_metric, secondary_metrics,
  measurement_policy_version, measurement_policy_snapshot,
  baseline_window, baseline_snapshot, baseline_status,
  absolute_terminal_at, status, attribution_confidence, results_deep_link
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(site_fix_id), sqlc.arg(measurement_generation),
  sqlc.arg(target_url), sqlc.arg(normalized_target_url), sqlc.narg(target_query), sqlc.arg(target_identity),
  sqlc.arg(fix_type), sqlc.arg(impact_mode), sqlc.arg(classifier_version), sqlc.arg(decision_origin), sqlc.arg(decision_confidence),
  sqlc.arg(prospective_observation), sqlc.arg(growth_hypothesis), sqlc.arg(primary_metric), sqlc.arg(secondary_metrics),
  sqlc.arg(measurement_policy_version), sqlc.arg(measurement_policy_snapshot),
  sqlc.arg(baseline_window), sqlc.arg(baseline_snapshot), sqlc.arg(baseline_status),
  sqlc.arg(absolute_terminal_at), sqlc.arg(status), sqlc.arg(attribution_confidence), sqlc.narg(results_deep_link)
)
on conflict (project_id, site_fix_id, measurement_generation) do update
set measurement_generation = excluded.measurement_generation
returning *;

-- name: GetSiteFixMeasurement :one
select * from site_fix_measurements
where project_id = sqlc.arg(project_id) and id = sqlc.arg(id);

-- name: GetLatestSiteFixMeasurementForFix :one
select * from site_fix_measurements
where project_id = sqlc.arg(project_id) and site_fix_id = sqlc.arg(site_fix_id)
order by measurement_generation desc
limit 1;

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
    results_deep_link = coalesce(results_deep_link, sqlc.arg(results_deep_link)),
    updated_at = now()
where project_id = sqlc.arg(project_id)
  and site_fix_id = sqlc.arg(site_fix_id)
  and measurement_generation = sqlc.arg(measurement_generation)
  and status in ('ready','planned','baseline_blocked','failed_retryable')
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
        and checkpoint.computed_at is null
    )
  )
order by measurement.absolute_terminal_at, measurement.created_at
limit 1
for update skip locked;

-- name: InsertSiteFixMeasurementCheckpoint :one
insert into site_fix_measurement_checkpoints (
  id, project_id, measurement_id, checkpoint_key, checkpoint_role,
  scheduled_at, window_start, window_end, attempt_number,
  required_data_sources, data_availability, minimum_sample,
  seo_metrics, ga4_metrics, geo_metrics, execution_metrics, guardrail_results,
  outcome_label, outcome_reason, attribution_confidence,
  computed_at, failure_reason, retry_classification
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(measurement_id), sqlc.arg(checkpoint_key), sqlc.arg(checkpoint_role),
  sqlc.arg(scheduled_at), sqlc.arg(window_start), sqlc.arg(window_end), sqlc.arg(attempt_number),
  sqlc.arg(required_data_sources), sqlc.arg(data_availability), sqlc.arg(minimum_sample),
  sqlc.arg(seo_metrics), sqlc.arg(ga4_metrics), sqlc.arg(geo_metrics), sqlc.arg(execution_metrics), sqlc.arg(guardrail_results),
  sqlc.narg(outcome_label), sqlc.narg(outcome_reason), sqlc.arg(attribution_confidence),
  sqlc.narg(computed_at), sqlc.narg(failure_reason), sqlc.arg(retry_classification)
)
on conflict (measurement_id, checkpoint_key, attempt_number) do nothing
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

-- name: CreateSiteFixMeasurementTerminalOutcome :one
insert into site_fix_measurement_terminal_outcomes (
  id, project_id, measurement_id, outcome_label, record_kind, terminal_reason,
  measurement_policy_version, baseline_snapshot, checkpoint_snapshot, outcome_snapshot
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(measurement_id), sqlc.arg(outcome_label), sqlc.arg(record_kind), sqlc.arg(terminal_reason),
  sqlc.arg(measurement_policy_version), sqlc.arg(baseline_snapshot), sqlc.arg(checkpoint_snapshot), sqlc.arg(outcome_snapshot)
)
on conflict (project_id, measurement_id) do nothing
returning *;

-- name: CreateSiteFixMeasurementLearning :one
insert into site_fix_measurement_learnings (
  id, project_id, terminal_outcome_id, measurement_id, learning_summary, applicability, learning_version
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(terminal_outcome_id), sqlc.arg(measurement_id),
  sqlc.arg(learning_summary), sqlc.arg(applicability), sqlc.arg(learning_version)
)
on conflict (project_id, measurement_id) do nothing
returning *;

-- name: CreateSiteFixMeasurementQualityRecord :one
insert into site_fix_measurement_quality_records (
  id, project_id, terminal_outcome_id, measurement_id,
  data_quality_state, quality_gaps, recommendation, quality_version
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(terminal_outcome_id), sqlc.arg(measurement_id),
  sqlc.arg(data_quality_state), sqlc.arg(quality_gaps), sqlc.arg(recommendation), sqlc.arg(quality_version)
)
on conflict (project_id, measurement_id) do nothing
returning *;

-- name: EnqueueSiteFixMeasurementHandoff :one
insert into site_fix_measurement_handoff_outbox (
  id, project_id, site_fix_id, measurement_generation, idempotency_key,
  max_attempts, next_attempt_at
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(site_fix_id), sqlc.arg(measurement_generation), sqlc.arg(idempotency_key),
  sqlc.arg(max_attempts), sqlc.arg(next_attempt_at)
)
on conflict (project_id, idempotency_key) do update
set idempotency_key = excluded.idempotency_key
returning *;

-- name: ClaimSiteFixMeasurementHandoff :one
with due as (
  select candidate.id
  from site_fix_measurement_handoff_outbox candidate
  where candidate.status in ('pending','failed_retryable')
    and candidate.next_attempt_at <= sqlc.arg(now_at)
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
order by measurement.updated_at desc, measurement.id;
