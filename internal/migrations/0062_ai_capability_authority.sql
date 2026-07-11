-- Migrate the retired Signal Scan / AI Discovery product modes into internal
-- capability policy without adding any provider-call authority.
--
-- This is version-gated so an accidental replay cannot overwrite consent that
-- a user records after the migration.
update projects
set config = config || jsonb_build_object(
  'capability_policy_version', 1,
  'growth_signal_enabled', case lower(coalesce(config->>'opportunity_finding_source_mix', 'all'))
    when 'ai_discovery' then false
    else true
  end,
  'growth_ai_enabled', case lower(coalesce(config->>'opportunity_finding_source_mix', 'all'))
    when 'signal_scan' then false
    else true
  end,
  'growth_ai_run_policy', case lower(coalesce(config->>'ai_discovery_automation', 'semi_automatic'))
    when 'automatic' then 'scheduled_only'
    when 'manual' then 'manual_only'
    else 'on_demand_recommended'
  end,
  'doctor_ai_enabled', case
    when config ? 'doctor_ai_enabled' then
      lower(coalesce(config->>'doctor_ai_enabled', 'false')) = 'true'
      and lower(coalesce(config->>'doctor_ai_run_policy', 'manual_only')) in ('automatic', 'on_demand', 'manual_only')
    else false
  end,
  'doctor_ai_run_policy', case lower(coalesce(config->>'doctor_ai_run_policy', 'manual_only'))
    when 'automatic' then 'automatic'
    when 'on_demand' then 'on_demand'
    else 'manual_only'
  end
)
where not (config ? 'capability_policy_version');
