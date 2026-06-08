-- SEO Autopilot observe-only foundation.

create table if not exists seo_objectives (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  name text not null,
  status text not null default 'active' check (status in ('active','paused','archived')),
  primary_metric text not null default 'clicks'
    check (primary_metric in ('clicks','impressions','conversions','qualified_sessions','ai_visibility')),
  secondary_metrics jsonb not null default '[]',
  target_pages jsonb not null default '[]',
  target_topics jsonb not null default '[]',
  target_queries jsonb not null default '[]',
  time_horizon_days int not null default 90,
  budget_usd numeric,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists idx_seo_objectives_project_status
  on seo_objectives (project_id, status, created_at desc);

create table if not exists seo_policies (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade unique,
  autopilot_level int not null default 0 check (autopilot_level between 0 and 4),
  weekly_action_limit int not null default 5,
  monthly_budget_limit numeric not null default 25,
  allowed_action_types jsonb not null default '["submit sitemap","metadata rewrite","technical SEO fix task"]',
  blocked_url_patterns jsonb not null default '[]',
  requires_review_action_types jsonb not null default '["refresh paragraph","create supporting article","merge pages","noindex/prune/delete","change robots/canonical rules"]',
  max_auto_changes_per_page_per_month int not null default 1,
  low_traffic_clicks_28d_threshold int not null default 10,
  low_traffic_impressions_28d_threshold int not null default 500,
  min_confidence_for_auto_publish numeric not null default 80,
  quiet_hours_start text,
  quiet_hours_end text,
  quiet_hours_timezone text not null default 'America/Los_Angeles',
  quiet_hours_behavior text not null default 'defer_to_next_window'
    check (quiet_hours_behavior in ('defer_to_next_window','skip_cycle')),
  kill_switch_enabled boolean not null default false,
  safe_mode_enabled boolean not null default false,
  risk_classifier_version text not null default 'v1',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists risk_classification_rules (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  version text not null,
  rules jsonb not null default '{}',
  created_by text not null default 'system',
  created_at timestamptz not null default now(),
  retired_at timestamptz,
  unique (project_id, version)
);

create table if not exists autopilot_runs (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  objective_id uuid references seo_objectives(id) on delete set null,
  status text not null check (status in ('ok','error','degraded')),
  autopilot_level_snapshot int not null default 0,
  derived_mode text not null default 'observe',
  started_at timestamptz not null default now(),
  finished_at timestamptz,
  input_snapshot jsonb not null default '{}',
  selected_actions jsonb not null default '[]',
  rejected_actions jsonb not null default '[]',
  guardrail_results jsonb not null default '[]',
  published_changes jsonb not null default '[]',
  cost_usd numeric,
  error text
);

create index if not exists idx_autopilot_runs_project_started
  on autopilot_runs (project_id, started_at desc);

create table if not exists seo_action_plans (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  autopilot_run_id uuid references autopilot_runs(id) on delete set null,
  objective_id uuid references seo_objectives(id) on delete set null,
  plan_window_start date not null,
  plan_window_end date not null,
  status text not null default 'draft' check (status in ('draft','ready_for_review','approved','executing','completed','blocked')),
  actions jsonb not null default '[]',
  expected_impact jsonb not null default '{}',
  expected_effort int not null default 0,
  aggregate_risk text not null default 'low' check (aggregate_risk in ('low','medium','high')),
  risk_classifier_version text not null default 'v1',
  approval_required boolean not null default true,
  approved_by text,
  approved_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists seo_experiments (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  action_id uuid references content_actions(id) on delete cascade,
  hypothesis text,
  baseline_start date,
  baseline_end date,
  measurement_start date,
  measurement_end date,
  primary_metric text,
  secondary_metrics jsonb not null default '[]',
  control_pages jsonb not null default '[]',
  evidence_level text not null default 'no_control'
    check (evidence_level in ('matched_control','site_trend_normalized','no_control')),
  result text check (result in ('improved','neutral','worsened','inconclusive')),
  confidence numeric,
  notes text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists guardrail_checks (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  action_id uuid references content_actions(id) on delete cascade,
  check_type text not null,
  status text not null check (status in ('passed','warning','blocked')),
  severity text not null default 'info' check (severity in ('info','warning','critical')),
  details jsonb not null default '{}',
  reported_false_positive_at timestamptz,
  reported_false_negative_at timestamptz,
  reported_by text,
  created_at timestamptz not null default now()
);

create table if not exists rollback_records (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  action_id uuid references content_actions(id) on delete set null,
  rollback_type text not null,
  source_commit_sha text,
  rollback_commit_sha text,
  reason text,
  performed_by text,
  created_at timestamptz not null default now()
);

create table if not exists safe_mode_events (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  reason text not null,
  trigger_source text not null
    check (trigger_source in ('manual','publish_failure','auth','notification','guardrail_report','traffic_anomaly','budget')),
  entered_at timestamptz not null default now(),
  entered_by text not null default 'system',
  exited_at timestamptz,
  exited_by text,
  exit_reason text,
  related_run_id uuid,
  related_action_id uuid,
  unique (project_id, exited_at)
);

create unique index if not exists uniq_open_safe_mode_event
  on safe_mode_events (project_id)
  where exited_at is null;

create table if not exists autopilot_audit_events (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  actor text not null check (actor in ('autopilot','human','system')),
  event_type text not null,
  entity_type text not null,
  entity_id uuid,
  before_snapshot jsonb not null default '{}',
  after_snapshot jsonb not null default '{}',
  created_at timestamptz not null default now()
);
