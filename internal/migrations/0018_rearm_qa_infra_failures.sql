-- Re-arm drafts that were escalated to the operator because of a QA
-- infrastructure failure (a truncated/unparseable model response — e.g.
-- "parse qa: unexpected EOF" or "missing claims"), not a real content problem.
-- With the larger QA token budget these now re-check cleanly, so clear the human
-- flag and reset repair/recovery state to let the recovery tick re-run QA.
update articles
set requires_human_decision = false,
    repair_status = 'idle',
    repair_attempts = 0,
    recovery_attempts = 0,
    repair_failure_reason = null
where status = 'pending_review'
  and qa_blocking = true
  and requires_human_decision = true
  and (
    qa_issues::text ilike '%parse qa%'
    or qa_issues::text ilike '%unexpected EOF%'
    or qa_issues::text ilike '%qa re-check failed%'
    or qa_issues::text ilike '%qa step failed%'
    or qa_issues::text ilike '%missing claims%'
    or qa_issues::text ilike '%compact fallback%'
  );
