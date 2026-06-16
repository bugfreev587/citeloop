-- Review auto-recovery: CiteLoop drains the review queue itself (re-run QA →
-- AI repair → regenerate) so a human is only ever asked for a genuine evidence
-- or positioning decision. These counters bound the automated work so it can
-- never loop forever.
alter table articles add column if not exists recovery_attempts integer not null default 0;
alter table topics add column if not exists recovery_attempts integer not null default 0;

-- Re-arm existing "zombie" drafts: blocked drafts that QA never actually
-- evaluated (no claim map was returned) were previously surfaced to the human
-- as "needs decision" with a dead Resolve button. They are infrastructure
-- failures, not human decisions — clear the human flag and reset repair state so
-- the recovery tick picks them up and re-runs QA automatically.
update articles
set requires_human_decision = false,
    repair_status = 'idle',
    repair_attempts = 0,
    repair_failure_reason = null
where status = 'pending_review'
  and qa_blocking = true
  and (
    qa_feedback -> 'claims' is null
    or jsonb_typeof(qa_feedback -> 'claims') <> 'array'
    or jsonb_array_length(qa_feedback -> 'claims') = 0
  );
