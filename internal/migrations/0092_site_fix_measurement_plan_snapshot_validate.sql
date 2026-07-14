set local lock_timeout = '5s';
set local statement_timeout = '30s';

-- Rows classified before the dedicated snapshot column existed must not make
-- online validation fail. Preserve a required plan only when the exact legacy
-- evidence plan is structurally complete and agrees with every denormalized
-- measurement field. An explicit override has the same precedence here as it
-- did in the classifier; an invalid override never falls through to the
-- regular finding plan.
with legacy_required_plans as (
  select
    project_id,
    id,
    case
      when evidence_snapshot #> '{finding,site_fix_policy_override}' is not null
        and evidence_snapshot #> '{finding,site_fix_policy_override}' <> 'null'::jsonb
      then evidence_snapshot #> '{finding,site_fix_policy_override,measurement_plan}'
      else evidence_snapshot #> '{finding,measurement_plan}'
    end as plan_snapshot
  from site_fixes
  where measurement_policy = 'measurement_required'
    and measurement_plan_snapshot = '{}'::jsonb
), canonical_legacy_required_plans as (
  select
    project_id,
    id,
    jsonb_set(
      plan_snapshot,
      '{growth_hypothesis}',
      to_jsonb(btrim(plan_snapshot->>'growth_hypothesis')),
      false
    ) as plan_snapshot
  from legacy_required_plans
  where (
    jsonb_typeof(plan_snapshot) = 'object'
    and nullif(btrim(plan_snapshot->>'growth_hypothesis'), '') is not null
  ) is true
)
update site_fixes as sf
set measurement_plan_snapshot = canonical.plan_snapshot
from canonical_legacy_required_plans as canonical
where sf.project_id = canonical.project_id
  and sf.id = canonical.id
  and (
    canonical.plan_snapshot <> '{}'::jsonb
    and nullif(btrim(canonical.plan_snapshot->>'primary_metric'), '') is not null
    and jsonb_typeof(canonical.plan_snapshot->'secondary_metrics') = 'array'
    and jsonb_typeof(canonical.plan_snapshot->'policy_snapshot') = 'object'
    and sf.growth_hypothesis = canonical.plan_snapshot->>'growth_hypothesis'
    and sf.primary_metric = canonical.plan_snapshot->>'primary_metric'
    and sf.secondary_metrics = canonical.plan_snapshot->'secondary_metrics'
    and sf.measurement_policy_version = canonical.plan_snapshot->'policy_snapshot'->>'policy_version'
    and sf.measurement_policy_snapshot = canonical.plan_snapshot->'policy_snapshot'
  ) is true;

-- Historical rows without an exactly reconstructable plan remain useful
-- verified fixes, but must not claim growth measurement readiness.
update site_fixes
set measurement_policy = 'verification_only'
where measurement_policy = 'measurement_required'
  and measurement_plan_snapshot = '{}'::jsonb;

-- site_fixes has deferrable project-scoped foreign keys. Flush their trigger
-- events before ALTER TABLE validation in this same migration transaction.
set constraints all immediate;

alter table site_fixes
  validate constraint site_fixes_measurement_plan_snapshot_json_check,
  validate constraint site_fixes_measurement_plan_alignment_check;
