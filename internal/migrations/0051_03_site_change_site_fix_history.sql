-- citeloop:migration-mode=nontransactional
-- citeloop:index=idx_site_change_applications_site_fix
create index concurrently if not exists idx_site_change_applications_site_fix
  on site_change_applications (project_id, site_fix_id, updated_at desc)
  where site_fix_id is not null;
