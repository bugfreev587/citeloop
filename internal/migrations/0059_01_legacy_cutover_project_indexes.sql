-- citeloop:migration-mode=nontransactional
-- citeloop:index=seo_opportunity_review_states_project_id_id_key
create unique index concurrently if not exists seo_opportunity_review_states_project_id_id_key
  on seo_opportunity_review_states (project_id, id);
