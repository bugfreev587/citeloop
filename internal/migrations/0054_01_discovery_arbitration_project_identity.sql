-- citeloop:migration-mode=nontransactional
-- citeloop:index=discovery_arbitration_decisions_project_id_id_key
create unique index concurrently if not exists discovery_arbitration_decisions_project_id_id_key
  on discovery_arbitration_decisions (project_id, id);
