-- citeloop:migration-mode=nontransactional
-- citeloop:index=site_change_applications_pr_claim_expiry_idx
create index concurrently if not exists site_change_applications_pr_claim_expiry_idx
  on site_change_applications (project_id, pr_claim_expires_at)
  where status = 'creating_pr';
