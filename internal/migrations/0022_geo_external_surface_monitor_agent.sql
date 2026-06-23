-- Allow the PRD multi-surface monitor to record external-surface health runs.

alter table geo_runs drop constraint if exists geo_runs_agent_check;
alter table geo_runs add constraint geo_runs_agent_check check (
  agent in (
    'geo_crawler_audit',
    'geo_prompt_builder',
    'geo_observer',
    'geo_analyzer',
    'geo_asset_brief',
    'geo_external_surface_monitor'
  )
);
