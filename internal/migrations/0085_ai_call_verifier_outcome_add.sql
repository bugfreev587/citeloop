set local lock_timeout = '5s';
set local statement_timeout = '30s';

alter table ai_call_records
  add column if not exists verifier_outcome jsonb;

alter table ai_call_records
  drop constraint if exists ai_call_records_verifier_outcome_check;

alter table ai_call_records
  add constraint ai_call_records_verifier_outcome_check check (
    verifier_outcome is null
    or (
      stage = 'fix_grounding_verification'
      and jsonb_typeof(verifier_outcome) = 'object'
    )
  ) not valid;

reset statement_timeout;
reset lock_timeout;
