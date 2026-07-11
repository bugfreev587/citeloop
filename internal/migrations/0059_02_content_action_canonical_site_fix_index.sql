-- citeloop:migration-mode=nontransactional
-- citeloop:index=idx_content_actions_canonical_site_fix
create index concurrently if not exists idx_content_actions_canonical_site_fix
  on content_actions (project_id, canonical_site_fix_id)
  where canonical_site_fix_id is not null;
