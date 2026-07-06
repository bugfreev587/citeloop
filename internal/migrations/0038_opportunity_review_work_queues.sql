-- PRD-CiteLoop-Opportunity-Review-and-Work-Queues:
-- approval provenance on execution items, snooze semantics on opportunities,
-- and a Results Watchlist for watch-only decisions.

alter table content_actions add column if not exists approval_source text not null default 'human_review';
alter table content_actions add column if not exists routing_source text not null default 'system_recommendation';
alter table content_actions add column if not exists work_type text;

alter table content_actions drop constraint if exists content_actions_approval_source_check;
alter table content_actions
  add constraint content_actions_approval_source_check
  check (approval_source in ('human_review','autopilot_policy','manual','retry_recovery','admin_import'));

alter table content_actions drop constraint if exists content_actions_routing_source_check;
alter table content_actions
  add constraint content_actions_routing_source_check
  check (routing_source in ('system_recommendation','user_override','policy'));

alter table content_actions drop constraint if exists content_actions_work_type_check;
alter table content_actions
  add constraint content_actions_work_type_check
  check (work_type is null or work_type in ('create_content','improve_page','fix_site_issue'));

update content_actions
set approval_source = 'autopilot_policy', routing_source = 'policy'
where approved_by = 'autopilot' and approval_source = 'human_review';

alter table seo_opportunities add column if not exists snoozed_until timestamptz;
alter table seo_opportunities add column if not exists snooze_reason text;
alter table seo_opportunities add column if not exists unsnoozed_at timestamptz;

alter table seo_opportunities drop constraint if exists seo_opportunities_status_check;
alter table seo_opportunities
  add constraint seo_opportunities_status_check
  check (status in ('open','accepted','dismissed','converted','done','stale','snoozed','watching'));

create table if not exists seo_watchlist_items (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  source_opportunity_id uuid not null references seo_opportunities(id) on delete cascade,
  status text not null default 'watching' check (status in ('watching','due_for_review','learned','closed')),
  observation_window_days int not null default 28,
  due_at timestamptz not null,
  closed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, source_opportunity_id)
);

create index if not exists idx_seo_watchlist_project_status
  on seo_watchlist_items (project_id, status, due_at);
