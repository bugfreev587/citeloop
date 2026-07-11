set local lock_timeout = '5s';
set local statement_timeout = '30s';

alter table discovery_candidates
  drop constraint if exists discovery_candidates_status_check;
alter table discovery_candidates
  add constraint discovery_candidates_status_check
  check (status in ('identity_ready','needs_specification','needs_evidence','needs_arbitration_review','migration_rolled_back')) not valid;

alter table work_signature_registry
  drop constraint if exists work_signature_registry_status_check;
alter table work_signature_registry
  add constraint work_signature_registry_status_check
  check (status in ('shadow_observed','reserved','proposed','approved','preparing','executing','awaiting_deploy','verifying','measuring','blocked','watching','snoozed','failed_retryable','reopened','verified','learned','dismissed','superseded','cancelled','failed_terminal','migration_rolled_back')) not valid;

reset statement_timeout;
reset lock_timeout;
