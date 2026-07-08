-- PRD-CiteLoop-Existing-Page-Update-Flow:
-- existing-page improvements create patch/diff artifacts instead of new Topics.

alter table content_actions drop constraint if exists content_actions_status_check;
alter table content_actions
  add constraint content_actions_status_check
  check (status in (
    'drafting','ready_for_review','approved','published','measuring','completed','failed',
    'verification_failed','recovery_required','dismissed',
    'verification_pending','manual_apply_required','needs_follow_up'
  ));

create table if not exists page_update_drafts (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  content_action_id uuid not null references content_actions(id) on delete cascade,
  target_url text not null,
  normalized_target_url text not null,
  opportunity_key text not null default '',
  target_article_id uuid references articles(id) on delete set null,
  source_file_path text,
  base_content_hash text,
  proposed_content_md text not null default '',
  patch jsonb not null default '{}',
  diff_snapshot jsonb not null default '{}',
  qa_feedback jsonb not null default '{}',
  resolution_criteria jsonb not null default '{}',
  publisher_result jsonb not null default '{}',
  verification_snapshot jsonb not null default '{}',
  original_source_snapshot jsonb not null default '{}',
  status text not null default 'drafting',
  approved_at timestamptz,
  applied_at timestamptz,
  verified_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, content_action_id),
  check (status in (
    'drafting','ready_for_review','approved','applying','applied','verification_pending',
    'manual_apply_required','verified','needs_follow_up','rejected','verification_failed'
  ))
);

create index if not exists idx_page_update_drafts_project_status
  on page_update_drafts (project_id, status, updated_at desc);

create unique index if not exists uniq_active_page_update_draft_target_key
  on page_update_drafts (project_id, normalized_target_url, opportunity_key)
  where status in ('drafting','ready_for_review','approved','applying','verification_pending','manual_apply_required');
