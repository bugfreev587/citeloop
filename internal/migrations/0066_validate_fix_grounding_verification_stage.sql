set local lock_timeout = '5s';
set local statement_timeout = '30s';

alter table ai_call_records
  validate constraint ai_call_records_stage_check;

reset statement_timeout;
reset lock_timeout;
