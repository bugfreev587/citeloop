alter table generation_runs
  drop constraint if exists generation_runs_agent_check;

alter table generation_runs
  add constraint generation_runs_agent_check
  check (agent in (
    'insight',
    'strategist',
    'writer',
    'qa',
    'publisher',
    'notification',
    'seo_sync',
    'seo_analyzer',
    'seo_brief',
    'seo_measurer',
    'reviser',
    'safe_mode'
  ));
