set local lock_timeout = '5s';
set local statement_timeout = '30s';

alter table ai_call_records
  drop constraint if exists ai_call_records_stage_check;

alter table ai_call_records
  add constraint ai_call_records_stage_check check (stage in (
    'evidence','doctor_diagnosis','arbitration','fix_generation','fix_grounding_verification','verification',
    'growth_hypothesis','brief','content_generation','qa','outcome_learning'
  )) not valid;

reset statement_timeout;
reset lock_timeout;
