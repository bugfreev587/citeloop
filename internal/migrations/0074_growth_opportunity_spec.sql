-- Phase 3: make every forward canonical Growth Opportunity decision-ready.
-- Existing rows remain explicit legacy rows; this migration never invents a
-- baseline or changes a user's prior decision.

alter table seo_opportunities
  add column if not exists growth_spec_state text not null default 'legacy',
  add column if not exists growth_spec_version text not null default 'legacy-v0',
  add column if not exists growth_spec_origin text not null default 'legacy_migration',
  add column if not exists growth_spec jsonb not null default '{}'::jsonb,
  add column if not exists growth_spec_missing jsonb not null default '[]'::jsonb,
  add column if not exists decision_ready_at timestamptz;

alter table seo_opportunities
  drop constraint if exists seo_opportunities_growth_spec_state_check,
  add constraint seo_opportunities_growth_spec_state_check
    check (growth_spec_state in ('legacy','needs_specification','needs_evidence','decision_ready')) not valid,
  drop constraint if exists seo_opportunities_growth_spec_origin_check,
  add constraint seo_opportunities_growth_spec_origin_check
    check (growth_spec_origin in ('legacy_migration','forward')) not valid,
  drop constraint if exists seo_opportunities_growth_spec_json_check,
  add constraint seo_opportunities_growth_spec_json_check
    check (jsonb_typeof(growth_spec) = 'object') not valid,
  drop constraint if exists seo_opportunities_growth_spec_missing_json_check,
  add constraint seo_opportunities_growth_spec_missing_json_check
    check (jsonb_typeof(growth_spec_missing) = 'array') not valid;

-- NOT VALID preserves historical rows. PostgreSQL still enforces the contract
-- for new/updated forward rows, which is the Phase 3 cutover boundary.
alter table seo_opportunities
  drop constraint if exists growth_opportunity_forward_spec_required,
  add constraint growth_opportunity_forward_spec_required check (
    not (canonical_growth and growth_spec_origin = 'forward' and status = 'open')
    or coalesce((
      growth_spec_state = 'decision_ready'
      and growth_spec_version = 'growth-opportunity-v1'
      and decision_ready_at is not null
      and nullif(btrim(growth_spec->>'hypothesis'), '') is not null
      and jsonb_typeof(growth_spec->'audience') = 'array'
      and jsonb_array_length(growth_spec->'audience') > 0
      and jsonb_typeof(growth_spec->'baseline') = 'object'
      and nullif(btrim(growth_spec->'baseline'->>'source'), '') is not null
      and nullif(btrim(growth_spec->'baseline'->>'metric'), '') is not null
      and jsonb_typeof(growth_spec->'baseline'->'value') = 'number'
      and nullif(btrim(growth_spec->'baseline'->>'window_start'), '') is not null
      and nullif(btrim(growth_spec->'baseline'->>'window_end'), '') is not null
      and nullif(btrim(growth_spec->>'primary_metric'), '') is not null
      and jsonb_typeof(growth_spec->'expected_change') = 'object'
      and (growth_spec->'expected_change'->>'direction') in ('increase','decrease','maintain')
      and jsonb_typeof(growth_spec->'expected_change'->'decision_threshold') = 'object'
      and jsonb_typeof(growth_spec->'measurement_policy') = 'object'
      and nullif(btrim(growth_spec->'measurement_policy'->>'policy_version'), '') is not null
      and case when jsonb_typeof(growth_spec->'measurement_policy'->'primary_checkpoint_offset_days') = 'number'
        then (growth_spec->'measurement_policy'->>'primary_checkpoint_offset_days')::int > 0 else false end
      and case when jsonb_typeof(growth_spec->'measurement_policy'->'max_measuring_duration_days') = 'number'
        then (growth_spec->'measurement_policy'->>'max_measuring_duration_days')::int > 0 else false end
      and case when jsonb_typeof(growth_spec->'measurement_policy'->'terminalization_grace_period_days') = 'number'
        then (growth_spec->'measurement_policy'->>'terminalization_grace_period_days')::int >= 0 else false end
      and nullif(btrim(growth_spec->>'attribution_model'), '') is not null
      and jsonb_typeof(growth_spec->'stop_conditions') = 'array'
      and jsonb_array_length(growth_spec->'stop_conditions') > 0
      and jsonb_typeof(growth_spec->'reconsider_conditions') = 'array'
      and jsonb_array_length(growth_spec->'reconsider_conditions') > 0
      and jsonb_array_length(growth_spec_missing) = 0
    ), false)
  ) not valid;
