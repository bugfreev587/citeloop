-- citeloop:migration-mode=nontransactional
-- citeloop:index=work_signature_registry_project_candidate_id_key
create unique index concurrently if not exists work_signature_registry_project_candidate_id_key
  on work_signature_registry (project_id, candidate_id, id);
