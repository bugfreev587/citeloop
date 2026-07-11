-- citeloop:migration-mode=nontransactional
-- citeloop:index=seo_opportunities_project_id_id_key
create unique index concurrently if not exists seo_opportunities_project_id_id_key
  on seo_opportunities (project_id, id);
