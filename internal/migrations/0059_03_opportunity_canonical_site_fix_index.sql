-- citeloop:migration-mode=nontransactional
-- citeloop:index=idx_seo_opportunities_canonical_site_fix
create index concurrently if not exists idx_seo_opportunities_canonical_site_fix
  on seo_opportunities (project_id, canonical_site_fix_id)
  where canonical_site_fix_id is not null;
