-- Growth Radar prompt rotation, source evidence cache, deterministic funnel,
-- and replayable candidate dispositions.

alter table geo_prompts
  add column if not exists cluster_key text not null default '',
  add column if not exists last_observed_at timestamptz,
  add column if not exists next_observed_at timestamptz,
  add column if not exists observation_count int not null default 0,
  add column if not exists targeted_reason text not null default '',
  add column if not exists archived_reason text not null default '';

create index if not exists idx_geo_prompts_rotation
  on geo_prompts(project_id, status, next_observed_at, last_observed_at, id);

update geo_prompts
set cluster_key = lower(regexp_replace(coalesce(nullif(btrim(target_topic), ''), nullif(btrim(intent_type), ''), id::text), '[^a-z0-9]+', '-', 'g'))
where cluster_key = '';

with ranked as (
  select id, project_id, priority, created_at,
         row_number() over (partition by project_id, cluster_key order by priority desc, created_at, id) cluster_rank,
         row_number() over (partition by project_id, intent_type, target_persona order by priority desc, created_at, id) pair_rank
  from geo_prompts where status = 'active'
), eligible as (
  select id, project_id,
         row_number() over (partition by project_id order by priority desc, created_at, id) project_rank
  from ranked where cluster_rank <= 6 and pair_rank <= 2
), keepers as (
  select id from eligible where project_rank <= 60
)
update geo_prompts prompt
set status = 'archived', archived_reason = 'portfolio_cap_migration', updated_at = now()
where prompt.status = 'active' and not exists (select 1 from keepers where keepers.id = prompt.id);
