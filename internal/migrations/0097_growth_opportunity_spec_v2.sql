-- Accept the versioned Platform Content Contract specification emitted by
-- Growth Radar while preserving the full v1 measurement contract. New v2
-- rows must carry an exact canonical platform target, pinned contract, score,
-- evidence, and success metric before they may enter the open decision queue.

alter table seo_opportunities
  drop constraint if exists growth_opportunity_forward_spec_required,
  add constraint growth_opportunity_forward_spec_required check (
    not (canonical_growth and growth_spec_origin = 'forward' and status = 'open')
    or coalesce((
      growth_spec_state = 'decision_ready'
      and decision_ready_at is not null
      and jsonb_array_length(growth_spec_missing) = 0
      and (
        (
          growth_spec_version = 'growth-opportunity-v1'
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
        )
        or (
          growth_spec_version = 'growth-opportunity-v2'
          and growth_spec->>'schema_version' = 'growth-opportunity-v2'
          and jsonb_typeof(growth_spec->'audience') = 'array'
          and jsonb_array_length(growth_spec->'audience') > 0
          and nullif(btrim(growth_spec->>'intent'), '') is not null
          and nullif(btrim(growth_spec->>'journey_stage'), '') is not null
          and nullif(btrim(growth_spec->>'topic_cluster_id'), '') is not null
          and nullif(btrim(growth_spec->>'normalized_topic'), '') is not null
          and nullif(btrim(growth_spec->>'asset_type'), '') is not null
          and nullif(btrim(growth_spec->>'recommended_action'), '') is not null
          and nullif(btrim(growth_spec->>'expected_user_value'), '') is not null
          and nullif(btrim(growth_spec->>'primary_metric'), '') is not null
          and jsonb_typeof(growth_spec->'targets') = 'object'
          and jsonb_typeof(growth_spec->'targets'->'canonical_target') = 'object'
          and nullif(btrim(growth_spec->'targets'->'canonical_target'->>'platform'), '') is not null
          and nullif(btrim(growth_spec->'targets'->'canonical_target'->>'platform_contract_id'), '') is not null
          and nullif(btrim(growth_spec->'targets'->'canonical_target'->>'platform_contract_version'), '') is not null
          and jsonb_typeof(growth_spec->'targets'->'target_platforms') = 'array'
          and jsonb_array_length(growth_spec->'targets'->'target_platforms') > 0
          and (growth_spec->'targets'->>'selection_mode') in ('contract_matrix','legacy_derived')
          and jsonb_typeof(growth_spec->'evidence') = 'object'
          and jsonb_typeof(growth_spec->'success_metric') = 'object'
          and nullif(btrim(growth_spec->'success_metric'->>'name'), '') is not null
          and case when jsonb_typeof(growth_spec->'success_metric'->'window_days') = 'number'
            then (growth_spec->'success_metric'->>'window_days')::int > 0 else false end
          and nullif(btrim(growth_spec->>'dedupe_identity'), '') is not null
          and jsonb_typeof(growth_spec->'score') = 'object'
          and jsonb_typeof(growth_spec->'source_versions') = 'object'
        )
      )
    ), false)
  ) not valid;
