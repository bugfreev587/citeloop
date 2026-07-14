alter table geo_prompts
  drop constraint if exists geo_prompts_source_check;

alter table geo_prompts
  add constraint geo_prompts_source_check
  check (source in (
    'profile',
    'topic',
    'competitor',
    'manual',
    'search_result',
    'ai_growth_planner'
  ));
