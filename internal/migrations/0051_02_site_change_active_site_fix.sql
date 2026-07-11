-- citeloop:migration-mode=nontransactional
-- citeloop:index=uniq_active_site_change_application_site_fix
create unique index concurrently if not exists uniq_active_site_change_application_site_fix
  on site_change_applications (project_id, site_fix_id)
  where site_fix_id is not null and status in (
    'draft_ready','source_mapping_required','ready_for_pr','creating_pr','github_pr_open',
    'github_pr_closed','github_pr_merged','deployment_pending','verification_pending',
    'needs_follow_up','conflict','manual_apply_required'
  );
