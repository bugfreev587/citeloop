-- Site Fix classification and its Results-owned, finite measurement aggregate.
-- Existing Site Fixes remain verification-only; no populated table is rewritten.

set local lock_timeout = '5s';
set local statement_timeout = '30s';

create or replace function site_fix_measurement_data_source_is_supported_v1(source_name text)
returns boolean
language sql
immutable
as $$
  select case
    when lower(btrim(coalesce(source_name, ''))) not in ('gsc','ga4','geo') then false
    else true
  end;
$$;

create or replace function site_fix_measurement_policy_is_finite(policy jsonb)
returns boolean
language plpgsql
immutable
as $$
declare
  early_days numeric;
  primary_days numeric;
  duration_days numeric;
  grace_days numeric;
  max_follow_ups numeric;
  follow_up_value jsonb;
  follow_up_day numeric;
  previous_day numeric;
  threshold_value numeric;
  minimum_after_periods numeric;
  minimum_after_sample numeric;
  source_value jsonb;
  source_name text;
  source_names text[] := array[]::text[];
  guardrail_value jsonb;
  guardrail_name text;
  guardrail_names text[] := array[]::text[];
  max_adverse_relative numeric;
begin
  if jsonb_typeof(policy) <> 'object'
    or nullif(btrim(policy->>'policy_version'), '') is null
    or jsonb_typeof(policy->'early_signal_offset_days') is distinct from 'number'
    or jsonb_typeof(policy->'primary_checkpoint_offset_days') is distinct from 'number'
    or jsonb_typeof(policy->'follow_up_offsets_days') is distinct from 'array'
    or jsonb_typeof(policy->'max_follow_up_attempts') is distinct from 'number'
    or jsonb_typeof(policy->'max_measuring_duration_days') is distinct from 'number'
    or jsonb_typeof(policy->'terminalization_grace_period_days') is distinct from 'number'
    or jsonb_typeof(policy->'metric_thresholds') is distinct from 'object'
    or jsonb_typeof(policy->'guardrails') is distinct from 'array'
    or jsonb_typeof(policy->'required_data_sources') is distinct from 'array'
    or jsonb_array_length(policy->'required_data_sources') = 0
    or not coalesce(jsonb_typeof(policy->'minimum_sample') = 'object', false) then
    return false;
  end if;

  if jsonb_typeof(policy->'metric_thresholds'->'direction') is distinct from 'string'
    or policy->'metric_thresholds'->>'direction' not in ('increase','decrease')
    or jsonb_typeof(policy->'metric_thresholds'->'kind') is distinct from 'string'
    or policy->'metric_thresholds'->>'kind' not in ('absolute','relative')
    or jsonb_typeof(policy->'metric_thresholds'->'value') is distinct from 'number' then
    return false;
  end if;
  threshold_value := (policy->'metric_thresholds'->>'value')::numeric;
  if threshold_value < 0 or threshold_value > 1000000000 then
    return false;
  end if;

  if not (policy->'minimum_sample' ? 'minimum_after_periods')
    and not (policy->'minimum_sample' ? 'minimum_after_sample') then
    return false;
  end if;
  if policy->'minimum_sample' ? 'minimum_after_periods' then
    if jsonb_typeof(policy->'minimum_sample'->'minimum_after_periods') is distinct from 'number' then
      return false;
    end if;
    minimum_after_periods := (policy->'minimum_sample'->>'minimum_after_periods')::numeric;
    if minimum_after_periods <> trunc(minimum_after_periods) or minimum_after_periods not between 1 and 365 then
      return false;
    end if;
  end if;
  if policy->'minimum_sample' ? 'minimum_after_sample' then
    if jsonb_typeof(policy->'minimum_sample'->'minimum_after_sample') is distinct from 'number' then
      return false;
    end if;
    minimum_after_sample := (policy->'minimum_sample'->>'minimum_after_sample')::numeric;
    if minimum_after_sample <= 0 or minimum_after_sample > 1000000000000 then
      return false;
    end if;
  end if;

  for source_value in select value from jsonb_array_elements(policy->'required_data_sources') loop
    if jsonb_typeof(source_value) <> 'string' or nullif(btrim(source_value #>> '{}'), '') is null then
      return false;
    end if;
    source_name := lower(btrim(source_value #>> '{}'));
    if not site_fix_measurement_data_source_is_supported_v1(source_name)
      or source_name = any(source_names) then
      return false;
    end if;
    source_names := array_append(source_names, source_name);
  end loop;

  for guardrail_value in select value from jsonb_array_elements(policy->'guardrails') loop
    if jsonb_typeof(guardrail_value) <> 'object'
      or jsonb_typeof(guardrail_value->'metric') is distinct from 'string'
      or nullif(btrim(guardrail_value->>'metric'), '') is null
      or jsonb_typeof(guardrail_value->'max_adverse_relative') is distinct from 'number' then
      return false;
    end if;
    guardrail_name := lower(btrim(guardrail_value->>'metric'));
    max_adverse_relative := (guardrail_value->>'max_adverse_relative')::numeric;
    if guardrail_name = any(guardrail_names) or max_adverse_relative <= 0 or max_adverse_relative > 1 then
      return false;
    end if;
    guardrail_names := array_append(guardrail_names, guardrail_name);
  end loop;

  early_days := (policy->>'early_signal_offset_days')::numeric;
  primary_days := (policy->>'primary_checkpoint_offset_days')::numeric;
  max_follow_ups := (policy->>'max_follow_up_attempts')::numeric;
  duration_days := (policy->>'max_measuring_duration_days')::numeric;
  grace_days := (policy->>'terminalization_grace_period_days')::numeric;

  if early_days <> trunc(early_days) or early_days not between 1 and 365
    or primary_days <> trunc(primary_days) or primary_days <= early_days
    or duration_days <> trunc(duration_days) or duration_days not between 1 and 365
    or primary_days > duration_days
    or grace_days <> trunc(grace_days) or grace_days not between 0 and 30
    or max_follow_ups <> trunc(max_follow_ups) or max_follow_ups not between 0 and 4
    or jsonb_array_length(policy->'follow_up_offsets_days') > max_follow_ups then
    return false;
  end if;

  previous_day := primary_days;
  for follow_up_value in select value from jsonb_array_elements(policy->'follow_up_offsets_days') loop
    if jsonb_typeof(follow_up_value) <> 'number' then
      return false;
    end if;
    follow_up_day := (follow_up_value #>> '{}')::numeric;
    if follow_up_day <> trunc(follow_up_day)
      or follow_up_day <= previous_day
      or follow_up_day > duration_days then
      return false;
    end if;
    previous_day := follow_up_day;
  end loop;

  return true;
exception when others then
  return false;
end;
$$;

alter table site_fixes
  add column if not exists fix_type text not null default 'unknown',
  add column if not exists impact_mode text not null default 'unclassified',
  add column if not exists measurement_policy text not null default 'verification_only',
  add column if not exists classifier_version text not null default 'legacy-unclassified-v0',
  add column if not exists decision_origin text not null default 'imported_policy',
  add column if not exists decision_confidence text not null default 'low',
  add column if not exists growth_hypothesis text,
  add column if not exists primary_metric text,
  add column if not exists secondary_metrics jsonb not null default '[]'::jsonb,
  add column if not exists measurement_policy_version text,
  add column if not exists measurement_policy_snapshot jsonb not null default '{}'::jsonb;

alter table site_fixes
  drop constraint if exists site_fixes_fix_type_check,
  add constraint site_fixes_fix_type_check check (fix_type in (
    'title_readability','metadata_format','metadata_ctr_optimization',
    'search_title_keyword_optimization','canonical_repair','robots_repair',
    'sitemap_repair','redirect_or_http_repair','schema_validity_repair',
    'schema_entity_optimization','internal_link_patch',
    'internal_link_authority_optimization','geo_entity_clarity',
    'geo_citation_optimization','geo_content_clarity','content_typo_or_clarity',
    'content_rewrite_for_search','content_demand_expansion','external_distribution',
    'conversion_or_cta_optimization','metadata_rewrite','schema_patch',
    'technical_fix','unknown'
  )) not valid,
  drop constraint if exists site_fixes_impact_mode_check,
  add constraint site_fixes_impact_mode_check check (impact_mode in (
    'unclassified','presentation_only','technical_reliability','search_visibility',
    'geo_visibility','content_demand','conversion_or_ctr'
  )) not valid,
  drop constraint if exists site_fixes_measurement_policy_check,
  add constraint site_fixes_measurement_policy_check check (measurement_policy in (
    'verification_only','measurement_required','measurement_optional'
  )) not valid,
  drop constraint if exists site_fixes_decision_origin_check,
  add constraint site_fixes_decision_origin_check check (decision_origin in (
    'system_rule','user_override','imported_policy'
  )) not valid,
  drop constraint if exists site_fixes_decision_confidence_check,
  add constraint site_fixes_decision_confidence_check check (
    decision_confidence in ('high','medium','low')
  ) not valid,
  drop constraint if exists site_fixes_secondary_metrics_json_check,
  add constraint site_fixes_secondary_metrics_json_check check (
    jsonb_typeof(secondary_metrics) = 'array'
  ) not valid,
  drop constraint if exists site_fixes_measurement_policy_snapshot_json_check,
  add constraint site_fixes_measurement_policy_snapshot_json_check check (
    jsonb_typeof(measurement_policy_snapshot) = 'object'
  ) not valid,
  drop constraint if exists site_fixes_measurement_readiness_check,
  add constraint site_fixes_measurement_readiness_check check (
    measurement_policy <> 'measurement_required'
    or (
      nullif(btrim(coalesce(growth_hypothesis, '')), '') is not null
      and nullif(btrim(coalesce(primary_metric, '')), '') is not null
      and nullif(btrim(coalesce(measurement_policy_version, '')), '') is not null
      and measurement_policy_snapshot <> '{}'::jsonb
      and measurement_policy_version = measurement_policy_snapshot->>'policy_version'
      and site_fix_measurement_policy_is_finite(measurement_policy_snapshot)
    )
  ) not valid;

create table if not exists site_fix_measurements (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  site_fix_id uuid not null,
  measurement_generation int not null check (measurement_generation >= 1),
  creation_idempotency_key text not null check (nullif(btrim(creation_idempotency_key), '') is not null),
  target_url text not null check (nullif(btrim(target_url), '') is not null),
  normalized_target_url text not null check (nullif(btrim(normalized_target_url), '') is not null),
  target_query text,
  target_identity jsonb not null default '{}'::jsonb check (jsonb_typeof(target_identity) = 'object'),
  fix_type text not null,
  impact_mode text not null,
  classifier_version text not null,
  decision_origin text not null,
  decision_confidence text not null check (decision_confidence in ('high','medium','low')),
  prospective_observation boolean not null default false,
  growth_hypothesis text not null check (nullif(btrim(growth_hypothesis), '') is not null),
  primary_metric text not null check (nullif(btrim(primary_metric), '') is not null),
  secondary_metrics jsonb not null default '[]'::jsonb check (jsonb_typeof(secondary_metrics) = 'array'),
  measurement_policy_version text not null check (nullif(btrim(measurement_policy_version), '') is not null),
  measurement_policy_snapshot jsonb not null check (jsonb_typeof(measurement_policy_snapshot) = 'object'),
  baseline_window jsonb not null check (jsonb_typeof(baseline_window) = 'object'),
  baseline_snapshot jsonb not null default '{}'::jsonb check (jsonb_typeof(baseline_snapshot) = 'object'),
  baseline_status text not null default 'planned' check (baseline_status in (
    'planned','collecting','ready','unavailable','blocked','insufficient_data'
  )),
  started_at timestamptz,
  absolute_terminal_at timestamptz,
  status text not null default 'planned' check (status in (
    'planned','baseline_blocked','ready','observing','terminal',
    'failed_retryable','failed_terminal'
  )),
  terminal_outcome text check (terminal_outcome is null or terminal_outcome in (
    'positive','negative','mixed','inconclusive','insufficient_data'
  )),
  outcome_reason text,
  attribution_confidence text not null default 'none' check (
    attribution_confidence in ('high','medium','low','none')
  ),
  confounders jsonb not null default '[]'::jsonb check (jsonb_typeof(confounders) = 'array'),
  results_deep_link text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, id),
  unique (project_id, site_fix_id, measurement_generation),
  unique (project_id, site_fix_id, creation_idempotency_key),
  check (
    (started_at is null and absolute_terminal_at is null)
    or (
      started_at is not null
      and absolute_terminal_at = started_at + (
        ((measurement_policy_snapshot->>'max_measuring_duration_days')::int
          + (measurement_policy_snapshot->>'terminalization_grace_period_days')::int) * interval '1 day'
      )
    )
  ),
  check (status not in ('observing','terminal') or started_at is not null),
  check (measurement_policy_version = measurement_policy_snapshot->>'policy_version'),
  check (site_fix_measurement_policy_is_finite(measurement_policy_snapshot)),
  check (
    (status = 'terminal' and terminal_outcome is not null and outcome_reason is not null)
    or (status <> 'terminal' and terminal_outcome is null)
  ),
  check (not prospective_observation or baseline_status in ('unavailable','insufficient_data')),
  check (not prospective_observation or attribution_confidence in ('low','none'))
);

alter table site_fix_measurements
  add constraint site_fix_measurements_site_fix_project_fk
  foreign key (project_id, site_fix_id)
  references site_fixes(project_id, id)
  on delete cascade not valid;

create table if not exists site_fix_measurement_generation_counters (
  project_id uuid not null references projects(id) on delete cascade,
  site_fix_id uuid not null,
  last_generation int not null check (last_generation >= 1),
  updated_at timestamptz not null default now(),
  primary key (project_id, site_fix_id)
);

alter table site_fix_measurement_generation_counters
  add constraint site_fix_measurement_generation_counters_site_fix_project_fk
  foreign key (project_id, site_fix_id)
  references site_fixes(project_id, id)
  on delete cascade not valid;

create or replace function create_site_fix_measurement_idempotently(
  requested_id uuid,
  requested_project_id uuid,
  requested_site_fix_id uuid,
  requested_creation_idempotency_key text,
  requested_target_url text,
  requested_normalized_target_url text,
  requested_target_query text,
  requested_target_identity jsonb,
  requested_fix_type text,
  requested_impact_mode text,
  requested_classifier_version text,
  requested_decision_origin text,
  requested_decision_confidence text,
  requested_prospective_observation boolean,
  requested_growth_hypothesis text,
  requested_primary_metric text,
  requested_secondary_metrics jsonb,
  requested_measurement_policy_version text,
  requested_measurement_policy_snapshot jsonb,
  requested_baseline_window jsonb,
  requested_baseline_snapshot jsonb,
  requested_baseline_status text,
  requested_status text,
  requested_attribution_confidence text,
  requested_results_deep_link text
)
returns site_fix_measurements
language plpgsql
volatile
as $$
declare
  existing_measurement site_fix_measurements%rowtype;
  next_generation int;
begin
  select measurement.* into existing_measurement
  from site_fix_measurements measurement
  where measurement.project_id = requested_project_id
    and measurement.site_fix_id = requested_site_fix_id
    and measurement.creation_idempotency_key = requested_creation_idempotency_key;
  if found then
    return existing_measurement;
  end if;

  perform fix.id
  from site_fixes fix
  where fix.project_id = requested_project_id and fix.id = requested_site_fix_id
  for update;
  if not found then
    raise exception 'Site Fix measurement requires its project-scoped Site Fix'
      using errcode = '23503';
  end if;

  select measurement.* into existing_measurement
  from site_fix_measurements measurement
  where measurement.project_id = requested_project_id
    and measurement.site_fix_id = requested_site_fix_id
    and measurement.creation_idempotency_key = requested_creation_idempotency_key;
  if found then
    return existing_measurement;
  end if;

  insert into site_fix_measurement_generation_counters (
    project_id, site_fix_id, last_generation
  ) values (
    requested_project_id, requested_site_fix_id, 1
  )
  on conflict (project_id, site_fix_id) do update
  set last_generation = site_fix_measurement_generation_counters.last_generation + 1,
      updated_at = now()
  returning last_generation into next_generation;

  insert into site_fix_measurements (
    id, project_id, site_fix_id, measurement_generation, creation_idempotency_key,
    target_url, normalized_target_url, target_query, target_identity,
    fix_type, impact_mode, classifier_version, decision_origin, decision_confidence,
    prospective_observation, growth_hypothesis, primary_metric, secondary_metrics,
    measurement_policy_version, measurement_policy_snapshot,
    baseline_window, baseline_snapshot, baseline_status,
    status, attribution_confidence, results_deep_link
  ) values (
    requested_id, requested_project_id, requested_site_fix_id, next_generation, requested_creation_idempotency_key,
    requested_target_url, requested_normalized_target_url, requested_target_query, requested_target_identity,
    requested_fix_type, requested_impact_mode, requested_classifier_version, requested_decision_origin, requested_decision_confidence,
    requested_prospective_observation, requested_growth_hypothesis, requested_primary_metric, requested_secondary_metrics,
    requested_measurement_policy_version, requested_measurement_policy_snapshot,
    requested_baseline_window, requested_baseline_snapshot, requested_baseline_status,
    requested_status, requested_attribution_confidence, requested_results_deep_link
  )
  returning * into existing_measurement;

  return existing_measurement;
end;
$$;

create table if not exists site_fix_measurement_checkpoints (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  measurement_id uuid not null,
  checkpoint_key text not null check (nullif(btrim(checkpoint_key), '') is not null),
  checkpoint_role text not null check (checkpoint_role in ('baseline','early_signal','primary','follow_up')),
  scheduled_at timestamptz not null,
  window_start timestamptz not null,
  window_end timestamptz not null,
  attempt_number int not null check (attempt_number between 1 and 5),
  required_data_sources jsonb not null default '[]'::jsonb check (jsonb_typeof(required_data_sources) = 'array'),
  data_availability jsonb not null default '{}'::jsonb check (jsonb_typeof(data_availability) = 'object'),
  minimum_sample jsonb not null default '{}'::jsonb check (jsonb_typeof(minimum_sample) = 'object'),
  seo_metrics jsonb not null default '{}'::jsonb check (jsonb_typeof(seo_metrics) = 'object'),
  ga4_metrics jsonb not null default '{}'::jsonb check (jsonb_typeof(ga4_metrics) = 'object'),
  geo_metrics jsonb not null default '{}'::jsonb check (jsonb_typeof(geo_metrics) = 'object'),
  execution_metrics jsonb not null default '{}'::jsonb check (jsonb_typeof(execution_metrics) = 'object'),
  guardrail_results jsonb not null default '{}'::jsonb check (jsonb_typeof(guardrail_results) = 'object'),
  outcome_label text check (outcome_label is null or outcome_label in (
    'positive','negative','mixed','inconclusive','insufficient_data'
  )),
  outcome_reason text,
  attribution_confidence text not null default 'none' check (
    attribution_confidence in ('high','medium','low','none')
  ),
  computed_at timestamptz,
  failure_reason text,
  retry_classification text not null default 'not_applicable' check (
    retry_classification in ('not_applicable','retryable','retry_exhausted','terminal')
  ),
  created_at timestamptz not null default now(),
  unique (measurement_id, checkpoint_key, attempt_number),
  unique (project_id, id),
  check (window_end >= window_start)
);

alter table site_fix_measurement_checkpoints
  add constraint site_fix_measurement_checkpoints_measurement_project_fk
  foreign key (project_id, measurement_id)
  references site_fix_measurements(project_id, id)
  on delete cascade not valid;

create table if not exists site_fix_measurement_terminal_outcomes (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  measurement_id uuid not null,
  outcome_label text not null check (outcome_label in (
    'positive','negative','mixed','inconclusive','insufficient_data'
  )),
  record_kind text not null check (record_kind in ('directional_learning','measurement_quality')),
  terminal_reason text not null,
  measurement_policy_version text not null,
  baseline_snapshot jsonb not null check (jsonb_typeof(baseline_snapshot) = 'object'),
  checkpoint_snapshot jsonb not null check (jsonb_typeof(checkpoint_snapshot) = 'object'),
  outcome_snapshot jsonb not null check (jsonb_typeof(outcome_snapshot) = 'object'),
  created_at timestamptz not null default now(),
  unique (project_id, id),
  unique (project_id, measurement_id),
  unique (project_id, id, measurement_id),
  check (
    (record_kind = 'directional_learning' and outcome_label in ('positive','negative','mixed','inconclusive'))
    or (record_kind = 'measurement_quality' and outcome_label = 'insufficient_data')
  )
);

alter table site_fix_measurement_terminal_outcomes
  add constraint site_fix_measurement_terminal_outcomes_measurement_project_fk
  foreign key (project_id, measurement_id)
  references site_fix_measurements(project_id, id)
  on delete cascade not valid;

create table if not exists site_fix_measurement_learnings (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  terminal_outcome_id uuid not null,
  measurement_id uuid not null,
  learning_summary text not null,
  applicability jsonb not null default '{}'::jsonb check (jsonb_typeof(applicability) = 'object'),
  scoring_eligible boolean not null default true check (scoring_eligible = true),
  learning_version text not null default 'site-fix-learning-v1',
  created_at timestamptz not null default now(),
  unique (project_id, terminal_outcome_id),
  unique (project_id, measurement_id)
);

alter table site_fix_measurement_learnings
  add constraint site_fix_measurement_learnings_terminal_project_fk
  foreign key (project_id, terminal_outcome_id, measurement_id)
  references site_fix_measurement_terminal_outcomes(project_id, id, measurement_id)
  on delete cascade not valid;

create table if not exists site_fix_measurement_quality_records (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  terminal_outcome_id uuid not null,
  measurement_id uuid not null,
  data_quality_state text not null,
  quality_gaps jsonb not null default '[]'::jsonb check (jsonb_typeof(quality_gaps) = 'array'),
  recommendation text not null,
  scoring_eligible boolean not null default false check (scoring_eligible = false),
  quality_version text not null default 'site-fix-measurement-quality-v1',
  created_at timestamptz not null default now(),
  unique (project_id, terminal_outcome_id),
  unique (project_id, measurement_id)
);

alter table site_fix_measurement_quality_records
  add constraint site_fix_measurement_quality_records_terminal_project_fk
  foreign key (project_id, terminal_outcome_id, measurement_id)
  references site_fix_measurement_terminal_outcomes(project_id, id, measurement_id)
  on delete cascade not valid;

create table if not exists site_fix_measurement_handoff_outbox (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  site_fix_id uuid not null,
  measurement_generation int not null check (measurement_generation >= 1),
  event_type text not null default 'activate_measurement' check (event_type = 'activate_measurement'),
  idempotency_key text not null check (nullif(btrim(idempotency_key), '') is not null),
  status text not null default 'pending' check (status in (
    'pending','processing','completed','failed_retryable','failed_terminal'
  )),
  attempt_count int not null default 0 check (attempt_count >= 0),
  max_attempts int not null default 8 check (max_attempts between 1 and 20),
  next_attempt_at timestamptz not null default now(),
  lock_token uuid,
  locked_until timestamptz,
  last_error_classification text,
  last_error text,
  completed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, idempotency_key),
  unique (project_id, site_fix_id, measurement_generation),
  check ((status = 'processing') = (lock_token is not null and locked_until is not null)),
  check ((status = 'completed') = (completed_at is not null))
);

alter table site_fix_measurement_handoff_outbox
  add constraint site_fix_measurement_handoff_measurement_project_fk
  foreign key (project_id, site_fix_id, measurement_generation)
  references site_fix_measurements(project_id, site_fix_id, measurement_generation)
  on delete cascade not valid;

create index if not exists idx_site_fix_measurements_results
  on site_fix_measurements (project_id, status, updated_at desc);
create index if not exists idx_site_fix_measurements_due
  on site_fix_measurements (status, absolute_terminal_at);
create index if not exists idx_site_fix_measurement_checkpoints_due
  on site_fix_measurement_checkpoints (scheduled_at, computed_at);
create index if not exists idx_site_fix_measurement_handoff_due
  on site_fix_measurement_handoff_outbox (next_attempt_at, created_at)
  where status in ('pending','failed_retryable');
create index if not exists idx_site_fix_measurement_handoff_processing_lease
  on site_fix_measurement_handoff_outbox (locked_until, created_at)
  where status = 'processing';

create or replace function prevent_site_fix_measurement_policy_mutation()
returns trigger language plpgsql as $$
begin
  if new.project_id is distinct from old.project_id
    or new.site_fix_id is distinct from old.site_fix_id
    or new.measurement_generation is distinct from old.measurement_generation
    or new.creation_idempotency_key is distinct from old.creation_idempotency_key
    or new.target_url is distinct from old.target_url
    or new.normalized_target_url is distinct from old.normalized_target_url
    or new.target_query is distinct from old.target_query
    or new.target_identity is distinct from old.target_identity
    or new.fix_type is distinct from old.fix_type
    or new.impact_mode is distinct from old.impact_mode
    or new.classifier_version is distinct from old.classifier_version
    or new.decision_origin is distinct from old.decision_origin
    or new.decision_confidence is distinct from old.decision_confidence
    or new.prospective_observation is distinct from old.prospective_observation
    or new.growth_hypothesis is distinct from old.growth_hypothesis
    or new.primary_metric is distinct from old.primary_metric
    or new.secondary_metrics is distinct from old.secondary_metrics
    or new.measurement_policy_version is distinct from old.measurement_policy_version
    or new.measurement_policy_snapshot is distinct from old.measurement_policy_snapshot
    or new.baseline_window is distinct from old.baseline_window then
    raise exception 'Site Fix measurement plan is immutable' using errcode = '23514';
  end if;
  if old.started_at is not null and (
    new.started_at is distinct from old.started_at
    or new.absolute_terminal_at is distinct from old.absolute_terminal_at
  ) then
    raise exception 'Site Fix measurement activation deadline is immutable' using errcode = '23514';
  end if;
  if old.started_at is null and new.started_at is null
    and new.absolute_terminal_at is distinct from old.absolute_terminal_at then
    raise exception 'Site Fix measurement deadline requires activation' using errcode = '23514';
  end if;
  if old.baseline_status in ('ready','unavailable','blocked','insufficient_data') and (
    new.baseline_status is distinct from old.baseline_status
    or new.baseline_snapshot is distinct from old.baseline_snapshot
  ) then
    raise exception 'Site Fix measurement baseline is immutable after freezing' using errcode = '23514';
  end if;
  return new;
end;
$$;

create or replace function reject_site_fix_measurement_append_only_mutation()
returns trigger language plpgsql as $$
begin
  if tg_op = 'UPDATE' and to_jsonb(new) = to_jsonb(old) then
    return new;
  end if;
  raise exception 'Site Fix measurement evidence is append-only' using errcode = '23514';
end;
$$;

drop trigger if exists site_fix_measurements_immutable_plan on site_fix_measurements;
create trigger site_fix_measurements_immutable_plan
before update on site_fix_measurements
for each row execute function prevent_site_fix_measurement_policy_mutation();

drop trigger if exists site_fix_measurement_checkpoints_immutable on site_fix_measurement_checkpoints;
create trigger site_fix_measurement_checkpoints_immutable
before update or delete on site_fix_measurement_checkpoints
for each row execute function reject_site_fix_measurement_append_only_mutation();

drop trigger if exists site_fix_measurement_terminal_outcomes_immutable on site_fix_measurement_terminal_outcomes;
create trigger site_fix_measurement_terminal_outcomes_immutable
before update or delete on site_fix_measurement_terminal_outcomes
for each row execute function reject_site_fix_measurement_append_only_mutation();

drop trigger if exists site_fix_measurement_learnings_immutable on site_fix_measurement_learnings;
create trigger site_fix_measurement_learnings_immutable
before update or delete on site_fix_measurement_learnings
for each row execute function reject_site_fix_measurement_append_only_mutation();

drop trigger if exists site_fix_measurement_quality_records_immutable on site_fix_measurement_quality_records;
create trigger site_fix_measurement_quality_records_immutable
before update or delete on site_fix_measurement_quality_records
for each row execute function reject_site_fix_measurement_append_only_mutation();

reset statement_timeout;
reset lock_timeout;
