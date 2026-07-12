do $$
begin
  if exists (
    select 1
    from projects
    where (config ? 'opportunity_finding_source_mix' or config ? 'ai_discovery_automation')
      and (
        coalesce((config->>'capability_policy_version')::int, 0) < 1
        or not config ? 'growth_signal_enabled'
        or not config ? 'growth_ai_enabled'
        or not config ? 'growth_ai_run_policy'
      )
  ) then
    raise exception 'cannot retire legacy discovery settings before capability authority is complete';
  end if;
end $$;

update projects
set config = config - 'opportunity_finding_source_mix' - 'ai_discovery_automation',
    updated_at = now()
where config ? 'opportunity_finding_source_mix'
   or config ? 'ai_discovery_automation';
