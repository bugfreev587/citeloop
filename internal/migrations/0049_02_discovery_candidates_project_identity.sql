set local lock_timeout = '5s';
set local statement_timeout = '30s';

create unique index if not exists discovery_candidates_project_id_id_key
  on discovery_candidates (project_id, id);

reset statement_timeout;
reset lock_timeout;
