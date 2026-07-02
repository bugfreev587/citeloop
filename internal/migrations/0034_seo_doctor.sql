-- SEO Doctor user-facing technical health reports.

create table if not exists seo_doctor_runs (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  trigger text not null check (trigger in ('onboarding','manual','weekly','post_publish')),
  status text not null default 'queued' check (status in ('queued','running','completed','failed','blocked')),
  stage text not null default 'queued',
  progress_percent int not null default 0 check (progress_percent >= 0 and progress_percent <= 100),
  message text not null default '',
  block_reason text,
  pages_discovered int not null default 0,
  pages_fetched int not null default 0,
  pages_checked int not null default 0,
  issues_found int not null default 0,
  health_score int,
  input_snapshot jsonb not null default '{}',
  output_summary jsonb not null default '{}',
  error text,
  created_by_user_id text,
  started_at timestamptz,
  updated_at timestamptz not null default now(),
  finished_at timestamptz,
  created_at timestamptz not null default now()
);

create index if not exists idx_seo_doctor_runs_project_updated
  on seo_doctor_runs (project_id, updated_at desc);

create index if not exists idx_seo_doctor_runs_project_finished
  on seo_doctor_runs (project_id, finished_at desc);

create index if not exists idx_seo_doctor_runs_active
  on seo_doctor_runs (project_id, created_at desc)
  where status in ('queued','running');

create table if not exists seo_doctor_findings (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  run_id uuid not null references seo_doctor_runs(id) on delete cascade,
  finding_key text not null,
  severity text not null check (severity in ('P0','P1','P2','Info')),
  category text not null,
  issue_type text not null,
  status text not null default 'active' check (status in ('active','resolved','dismissed','converted')),
  affected_urls jsonb not null default '[]',
  normalized_urls jsonb not null default '[]',
  evidence jsonb not null default '{}',
  why_it_matters text not null default '',
  fix_intent text not null default '',
  developer_instructions text not null default '',
  likely_files_or_surfaces jsonb not null default '[]',
  acceptance_tests jsonb not null default '[]',
  risk_level text not null default 'low' check (risk_level in ('low','medium','high')),
  review_required boolean not null default true,
  autofix_eligible boolean not null default false,
  linked_opportunity_id uuid references seo_opportunities(id) on delete set null,
  linked_content_action_id uuid references content_actions(id) on delete set null,
  first_seen_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  resolved_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index if not exists uniq_active_seo_doctor_finding_key
  on seo_doctor_findings (project_id, finding_key)
  where status = 'active';

create index if not exists idx_seo_doctor_findings_run_severity
  on seo_doctor_findings (run_id, severity, updated_at desc);

create index if not exists idx_seo_doctor_findings_project_status
  on seo_doctor_findings (project_id, status, updated_at desc);
