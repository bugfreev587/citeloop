-- Persist the classifier-validated structured measurement plan independently
-- from the human-facing evidence/proposed-fix envelopes.

set local lock_timeout = '5s';
set local statement_timeout = '30s';

alter table site_fixes
  add column if not exists measurement_plan_snapshot jsonb not null default '{}'::jsonb;

alter table site_fixes
  drop constraint if exists site_fixes_measurement_plan_snapshot_json_check,
  add constraint site_fixes_measurement_plan_snapshot_json_check check (
    jsonb_typeof(measurement_plan_snapshot) = 'object'
  ) not valid,
  drop constraint if exists site_fixes_measurement_plan_alignment_check,
  add constraint site_fixes_measurement_plan_alignment_check check (
    (
      measurement_plan_snapshot = '{}'::jsonb
      or (
        nullif(btrim(measurement_plan_snapshot->>'growth_hypothesis'), '') is not null
        and nullif(btrim(measurement_plan_snapshot->>'primary_metric'), '') is not null
        and jsonb_typeof(measurement_plan_snapshot->'secondary_metrics') = 'array'
        and jsonb_typeof(measurement_plan_snapshot->'policy_snapshot') = 'object'
        and growth_hypothesis = measurement_plan_snapshot->>'growth_hypothesis'
        and primary_metric = measurement_plan_snapshot->>'primary_metric'
        and secondary_metrics = measurement_plan_snapshot->'secondary_metrics'
        and measurement_policy_version = measurement_plan_snapshot->'policy_snapshot'->>'policy_version'
        and measurement_policy_snapshot = measurement_plan_snapshot->'policy_snapshot'
      )
    )
    and (
      measurement_policy <> 'measurement_required'
      or measurement_plan_snapshot <> '{}'::jsonb
    )
  ) not valid;
