alter table articles
  add column if not exists qa_status text not null default 'pending';

alter table articles
  add column if not exists qa_failure_kind text;

alter table articles
  add column if not exists qa_failure_message text;

alter table articles
  add column if not exists qa_failure_fingerprint text;

alter table articles
  add column if not exists qa_attempt_count int not null default 0;

alter table articles
  add column if not exists qa_last_checked_at timestamptz;

alter table articles
  add column if not exists qa_human_options jsonb not null default '[]';

alter table articles
  drop constraint if exists articles_qa_status_check;

alter table articles
  add constraint articles_qa_status_check
  check (qa_status in (
    'pending',
    'passed',
    'blocking',
    'parse_failed',
    'needs_human_decision'
  ));

update articles
set qa_status = case
    when qa_status = 'pending' and qa_blocking then 'blocking'
    when qa_status = 'pending' then 'passed'
    else qa_status
  end,
  qa_last_checked_at = coalesce(qa_last_checked_at, created_at)
where qa_status = 'pending' or qa_last_checked_at is null;
