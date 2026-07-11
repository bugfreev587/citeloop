-- citeloop:migration-mode=nontransactional
-- citeloop:index=idx_rollback_records_site_fix_id
create index concurrently if not exists idx_rollback_records_site_fix_id
  on rollback_records (project_id, site_fix_id)
  where site_fix_id is not null;
