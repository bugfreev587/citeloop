-- PRD-CiteLoop-GitHub-PR-Apply-Layer:
-- shared source-backed apply ledger for Page Updates and Site Fixes.

create table if not exists site_change_applications (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  source_opportunity_id uuid references seo_opportunities(id) on delete set null,
  content_action_id uuid not null references content_actions(id) on delete cascade,
  page_update_draft_id uuid references page_update_drafts(id) on delete cascade,
  application_kind text not null check (application_kind in ('page_update', 'site_fix')),
  target_url text not null,
  normalized_target_url text not null,
  opportunity_key text not null,
  publisher_connection_id uuid references publisher_connections(id) on delete set null,
  repo_full_name text,
  base_branch text,
  working_branch text,
  base_commit_sha text,
  head_commit_sha text,
  source_file_path text,
  source_file_paths jsonb not null default '[]',
  source_mapping_confidence text not null default 'low',
  source_mapping_reason text not null default '',
  base_file_sha text,
  base_content_hash text,
  proposed_content_hash text,
  patch_snapshot jsonb not null default '{}',
  diff_snapshot jsonb not null default '{}',
  resolution_criteria jsonb not null default '{}',
  github_pr_number integer,
  github_pr_url text,
  github_pr_state text,
  deployment_snapshot jsonb not null default '{}',
  verification_snapshot jsonb not null default '{}',
  failure_reason text,
  status text not null default 'draft_ready',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  pr_created_at timestamptz,
  merged_at timestamptz,
  deployed_at timestamptz,
  verified_at timestamptz,
  check (status in (
    'draft_ready',
    'source_mapping_required',
    'ready_for_pr',
    'creating_pr',
    'github_pr_open',
    'github_pr_closed',
    'github_pr_merged',
    'deployment_pending',
    'verification_pending',
    'verified',
    'needs_follow_up',
    'conflict',
    'manual_apply_required',
    'failed'
  ))
);

create unique index if not exists uniq_active_site_change_application
  on site_change_applications (project_id, opportunity_key)
  where status in (
    'draft_ready',
    'source_mapping_required',
    'ready_for_pr',
    'creating_pr',
    'github_pr_open',
    'github_pr_closed',
    'github_pr_merged',
    'deployment_pending',
    'verification_pending',
    'needs_follow_up',
    'conflict',
    'manual_apply_required'
  );

create index if not exists idx_site_change_applications_project_status
  on site_change_applications (project_id, status, updated_at desc);

create index if not exists idx_site_change_applications_content_action
  on site_change_applications (project_id, content_action_id, updated_at desc);
