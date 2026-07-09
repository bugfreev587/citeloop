-- Fibonacci-backoff merge polling + awaiting-merge nag schedule for
-- source-backed PRs. next_poll_at gates the GitHub merge check; next_notify_at
-- gates the operator nag. Both are advanced by the scheduler reconcile tick.

alter table site_change_applications
  add column if not exists next_poll_at timestamptz,
  add column if not exists next_notify_at timestamptz;

-- Backfill applications already waiting on an open PR so the new tick picks them
-- up promptly and starts the nag clock from PR creation.
update site_change_applications
set next_poll_at = coalesce(next_poll_at, now()),
    next_notify_at = coalesce(next_notify_at, coalesce(pr_created_at, now()) + interval '12 hours')
where status = 'github_pr_open';
