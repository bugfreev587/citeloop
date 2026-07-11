set local lock_timeout = '5s';
set local statement_timeout = '30s';

create unique index if not exists work_signature_registry_project_candidate_id_key
  on work_signature_registry (project_id, candidate_id, id);

reset statement_timeout;
reset lock_timeout;
