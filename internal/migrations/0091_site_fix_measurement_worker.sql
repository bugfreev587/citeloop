-- Permit one atomic null -> completed checkpoint transition while retaining an
-- immutable schedule and append-only computed evidence.

set local lock_timeout = '5s';
set local statement_timeout = '30s';

create or replace function site_fix_measurement_guardrails_supported_v1(primary_metric text, policy jsonb)
returns boolean language plpgsql immutable as $$
declare guardrail jsonb; metric text; allowed text;
begin
  case lower(btrim(primary_metric))
    when 'ctr' then allowed := 'gsc_impressions';
    when 'gsc_ctr' then allowed := 'gsc_impressions';
    when 'clicks' then allowed := 'gsc_impressions';
    when 'gsc_clicks' then allowed := 'gsc_impressions';
    when 'position' then allowed := 'gsc_impressions';
    when 'gsc_position' then allowed := 'gsc_impressions';
    when 'conversion_rate' then allowed := 'ga4_sessions';
    when 'ga4_conversion_rate' then allowed := 'ga4_sessions';
    when 'qualified_actions' then allowed := 'ga4_sessions';
    when 'ga4_key_events' then allowed := 'ga4_sessions';
    when 'citations' then allowed := 'ai_brand_mention_rate';
    when 'ai_citation_count' then allowed := 'ai_brand_mention_rate';
    else allowed := null;
  end case;
  for guardrail in select value from jsonb_array_elements(coalesce(policy->'guardrails', '[]'::jsonb)) loop
    metric := lower(btrim(guardrail->>'metric'));
    if metric = 'impressions' then metric := 'gsc_impressions'; end if;
    if metric = 'referral_sessions' then metric := 'ga4_sessions'; end if;
    if metric = 'brand_mentions' then metric := 'ai_brand_mention_rate'; end if;
    if allowed is null or metric <> allowed then return false; end if;
  end loop;
  return true;
exception when others then return false;
end;
$$;

alter table site_fixes
  drop constraint if exists site_fixes_measurement_guardrails_supported_check,
  add constraint site_fixes_measurement_guardrails_supported_check check (
    measurement_policy <> 'measurement_required'
    or site_fix_measurement_guardrails_supported_v1(primary_metric, measurement_policy_snapshot)
  ) not valid;

alter table site_fix_measurements
  drop constraint if exists site_fix_measurements_guardrails_supported_check,
  add constraint site_fix_measurements_guardrails_supported_check check (
    site_fix_measurement_guardrails_supported_v1(primary_metric, measurement_policy_snapshot)
  ) not valid;

alter table site_fix_measurement_handoff_outbox
  add column if not exists occurred_at timestamptz;

update site_fix_measurement_handoff_outbox
set occurred_at = least(next_attempt_at, created_at)
where occurred_at is null;

alter table site_fix_measurement_checkpoints
  add column if not exists evaluation_attempt_count int not null default 0,
  add column if not exists next_attempt_at timestamptz;

update site_fix_measurement_checkpoints
set next_attempt_at = scheduled_at
where next_attempt_at is null;

create or replace function prevent_site_fix_measurement_checkpoint_mutation()
returns trigger language plpgsql as $$
begin
  if tg_op = 'DELETE' then
    raise exception 'Site Fix measurement checkpoint result is immutable after completion' using errcode = '23514';
  end if;
  if new.id is distinct from old.id
    or new.created_at is distinct from old.created_at
    or new.project_id is distinct from old.project_id
    or new.measurement_id is distinct from old.measurement_id
    or new.checkpoint_key is distinct from old.checkpoint_key
    or new.checkpoint_role is distinct from old.checkpoint_role
    or new.scheduled_at is distinct from old.scheduled_at
    or new.window_start is distinct from old.window_start
    or new.window_end is distinct from old.window_end
    or new.attempt_number is distinct from old.attempt_number
    or new.required_data_sources is distinct from old.required_data_sources
    or new.minimum_sample is distinct from old.minimum_sample then
    raise exception 'Site Fix measurement checkpoint schedule is immutable' using errcode = '23514';
  end if;
  if old.computed_at is null and new.computed_at is not null then
    return new;
  end if;
  if old.computed_at is null and new.computed_at is null
    and new.evaluation_attempt_count = old.evaluation_attempt_count + 1
    and new.next_attempt_at > old.next_attempt_at
    and new.data_availability is not distinct from old.data_availability
    and new.seo_metrics is not distinct from old.seo_metrics
    and new.ga4_metrics is not distinct from old.ga4_metrics
    and new.geo_metrics is not distinct from old.geo_metrics
    and new.execution_metrics is not distinct from old.execution_metrics
    and new.guardrail_results is not distinct from old.guardrail_results
    and new.outcome_label is not distinct from old.outcome_label
    and new.outcome_reason is not distinct from old.outcome_reason
    and new.attribution_confidence is not distinct from old.attribution_confidence then
    return new;
  end if;
  if to_jsonb(new) = to_jsonb(old) then
    return new;
  end if;
  raise exception 'Site Fix measurement checkpoint result is immutable after completion' using errcode = '23514';
end;
$$;

drop trigger if exists site_fix_measurement_checkpoints_immutable on site_fix_measurement_checkpoints;
create trigger site_fix_measurement_checkpoints_immutable
before update or delete on site_fix_measurement_checkpoints
for each row execute function prevent_site_fix_measurement_checkpoint_mutation();

create or replace function complete_site_fix_measurement_checkpoint_idempotently(
  requested_project_id uuid, requested_measurement_id uuid, requested_checkpoint_key text, requested_attempt_number int,
  requested_data_availability jsonb, requested_seo_metrics jsonb, requested_ga4_metrics jsonb,
  requested_geo_metrics jsonb, requested_execution_metrics jsonb, requested_guardrail_results jsonb,
  requested_outcome_label text, requested_outcome_reason text, requested_attribution_confidence text,
  requested_computed_at timestamptz, requested_failure_reason text, requested_retry_classification text
)
returns site_fix_measurement_checkpoints language plpgsql volatile as $$
declare checkpoint site_fix_measurement_checkpoints%rowtype;
begin
  select candidate.* into checkpoint
  from site_fix_measurement_checkpoints candidate
  where candidate.project_id=requested_project_id and candidate.measurement_id=requested_measurement_id
    and candidate.checkpoint_key=requested_checkpoint_key and candidate.attempt_number=requested_attempt_number
  for update;
  if not found then return null; end if;
  if checkpoint.computed_at is null then
    update site_fix_measurement_checkpoints candidate set
      data_availability=requested_data_availability, seo_metrics=requested_seo_metrics,
      ga4_metrics=requested_ga4_metrics, geo_metrics=requested_geo_metrics,
      execution_metrics=requested_execution_metrics, guardrail_results=requested_guardrail_results,
      outcome_label=requested_outcome_label, outcome_reason=requested_outcome_reason,
      attribution_confidence=requested_attribution_confidence, computed_at=requested_computed_at,
      failure_reason=requested_failure_reason, retry_classification=requested_retry_classification,
      evaluation_attempt_count=candidate.evaluation_attempt_count+1
    where candidate.id=checkpoint.id returning * into checkpoint;
    return checkpoint;
  end if;
  if checkpoint.data_availability is distinct from requested_data_availability
    or checkpoint.seo_metrics is distinct from requested_seo_metrics
    or checkpoint.ga4_metrics is distinct from requested_ga4_metrics
    or checkpoint.geo_metrics is distinct from requested_geo_metrics
    or checkpoint.execution_metrics is distinct from requested_execution_metrics
    or checkpoint.guardrail_results is distinct from requested_guardrail_results
    or checkpoint.outcome_label is distinct from requested_outcome_label
    or checkpoint.outcome_reason is distinct from requested_outcome_reason
    or checkpoint.attribution_confidence is distinct from requested_attribution_confidence
    or checkpoint.computed_at is distinct from requested_computed_at
    or checkpoint.failure_reason is distinct from requested_failure_reason
    or checkpoint.retry_classification is distinct from requested_retry_classification then
    raise exception 'conflicting Site Fix measurement checkpoint completion replay' using errcode='23514';
  end if;
  return checkpoint;
end;
$$;

create or replace function prevent_site_fix_measurement_handoff_occurrence_mutation()
returns trigger language plpgsql as $$
begin
  if new.occurred_at is distinct from old.occurred_at then
    raise exception 'Site Fix measurement handoff occurrence is immutable' using errcode='23514';
  end if;
  return new;
end;
$$;

drop trigger if exists site_fix_measurement_handoff_occurrence_immutable on site_fix_measurement_handoff_outbox;
create trigger site_fix_measurement_handoff_occurrence_immutable
before update on site_fix_measurement_handoff_outbox
for each row execute function prevent_site_fix_measurement_handoff_occurrence_mutation();

reset statement_timeout;
reset lock_timeout;
