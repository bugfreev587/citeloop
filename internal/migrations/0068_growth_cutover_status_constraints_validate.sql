set local lock_timeout = '5s';
set local statement_timeout = '30s';

alter table discovery_candidates
  validate constraint discovery_candidates_status_check;
alter table work_signature_registry
  validate constraint work_signature_registry_status_check;

reset statement_timeout;
reset lock_timeout;
