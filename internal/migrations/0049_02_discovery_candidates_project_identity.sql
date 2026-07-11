-- citeloop:migration-mode=nontransactional
-- citeloop:index=discovery_candidates_project_id_id_key
create unique index concurrently if not exists discovery_candidates_project_id_id_key
  on discovery_candidates (project_id, id);
