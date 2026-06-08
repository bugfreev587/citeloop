-- Persist AI repair loop state so QA/editor cannot bounce forever.

alter table articles
  add column repair_attempts int not null default 0,
  add column last_repair_at timestamptz,
  add column repair_status text not null default 'idle',
  add column repair_failure_reason text,
  add column requires_human_decision boolean not null default false,
  add column human_decision_options jsonb not null default '[]',
  add column qa_feedback jsonb not null default '{}';

alter table articles
  add constraint articles_repair_status_check
  check (repair_status in ('idle','repairing','repaired','exhausted','failed','human_decision'));

create index idx_articles_repair_queue
  on articles (project_id, status, qa_blocking, repair_status, repair_attempts)
  where status = 'pending_review';
