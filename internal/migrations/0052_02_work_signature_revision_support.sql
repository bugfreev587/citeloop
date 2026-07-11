set local lock_timeout = '5s';
set local statement_timeout = '30s';

-- The replacement partial shadow index is created concurrently by 0052_01
-- before this short catalog change. Enforced rows remain protected by
-- uniq_enforced_active_work_signature while terminal work can now reserve a
-- later enforced revision for the same canonical candidate.
alter table work_signature_registry
  drop constraint if exists work_signature_registry_candidate_id_mode_key;

reset statement_timeout;
reset lock_timeout;
