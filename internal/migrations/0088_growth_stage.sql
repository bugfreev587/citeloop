-- Manual, versioned project growth stages for stage-aware Opportunity discovery.

create table if not exists growth_stage_settings (
  project_id uuid primary key references projects(id) on delete cascade,
  stage text not null check (stage in ('foundation','traction','scale','optimize')),
  stage_profile_version text not null,
  setting_version bigint not null default 1 check (setting_version > 0),
  is_default_unconfirmed boolean not null default false,
  selected_by text not null default '',
  selected_at timestamptz not null default now(),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists growth_stage_events (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  previous_stage text not null check (previous_stage in ('foundation','traction','scale','optimize')),
  new_stage text not null check (new_stage in ('foundation','traction','scale','optimize')),
  previous_profile_version text not null,
  new_profile_version text not null,
  expected_setting_version bigint not null check (expected_setting_version >= 0),
  committed_setting_version bigint not null check (committed_setting_version > 0),
  actor text not null default '',
  reason text not null default '',
  affected_watchlist_count int not null default 0 check (affected_watchlist_count >= 0),
  rescore_status text not null default 'pending' check (rescore_status in ('pending','running','complete','failed')),
  failure_code text not null default '',
  failure_detail text not null default '',
  started_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz not null default now()
);

create index if not exists idx_growth_stage_events_project_created
  on growth_stage_events(project_id, created_at desc);
create index if not exists idx_growth_stage_events_pending
  on growth_stage_events(rescore_status, created_at)
  where rescore_status in ('pending','running');

-- Re-apply the public-prompt safety boundary for installations that generated
-- prompts after the original Growth Radar migration but before runtime input
-- sanitization was added. ListActiveGEOPrompts also excludes these rows so a
-- migration delay can never make them observable.
update geo_prompts
set status = 'archived', archived_reason = 'internal_sensitive_term', updated_at = now()
where status = 'active' and lower(prompt_text) ~
  '(api[ _-]?key|access[ _-]?token|secret|credential|password|database|postgres|mysql|redis|migration|deploy(ment)?|railway|vercel|github[ _-]?token|aes|encrypt|private[ _-]?key|private[ _-]?repo|token[ _-]?gate|kubernetes|docker|internal[ _-]?diagnostic)';
