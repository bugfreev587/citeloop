-- citeloop:migration-mode=nontransactional
-- citeloop:index=idx_active_site_change_application_content_action
create index concurrently if not exists idx_active_site_change_application_content_action
  on site_change_applications (project_id, content_action_id)
  where content_action_id is not null and status in (
    'draft_ready','source_mapping_required','ready_for_pr','creating_pr','github_pr_open',
    'github_pr_closed','github_pr_merged','deployment_pending','verification_pending',
    'needs_follow_up','conflict','manual_apply_required'
  );
