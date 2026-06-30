-- Phase 4 Results attribution checkpoint ledger.

create table if not exists action_measurements (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  content_action_id uuid not null references content_actions(id) on delete cascade,
  article_id uuid references articles(id) on delete set null,
  checkpoint_day int not null default 0,
  window_start date,
  window_end date,
  seo_metrics jsonb not null default '{}',
  ga4_metrics jsonb not null default '{}',
  geo_metrics jsonb not null default '{}',
  execution_metrics jsonb not null default '{}',
  outcome_label text not null default 'insufficient_data'
    check (outcome_label in ('insufficient_data','positive','negative','mixed','inconclusive')),
  outcome_reason text not null default '',
  attribution_confidence text not null default 'low'
    check (attribution_confidence in ('high','medium','low','none')),
  confounders jsonb not null default '[]',
  computed_at timestamptz not null default now(),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, content_action_id, checkpoint_day)
);

create index if not exists idx_action_measurements_project_computed
  on action_measurements (project_id, computed_at desc);

create index if not exists idx_action_measurements_action
  on action_measurements (content_action_id, checkpoint_day);

create index if not exists idx_action_measurements_article_id
  on action_measurements (article_id);
