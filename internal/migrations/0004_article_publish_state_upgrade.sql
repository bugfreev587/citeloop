-- Production databases that already applied 0001 need the publish-state fields
-- added forward-only; editing 0001 only helps fresh databases.

alter table articles
  add column if not exists last_publish_error text;

alter table articles
  add column if not exists publish_attempts int not null default 0;

alter table articles
  add column if not exists next_publish_retry_at timestamptz;

alter table articles
  add column if not exists publish_phase text;

alter table articles
  add column if not exists resolved_slug text;

alter table articles
  add column if not exists publish_path text;

alter table articles
  add column if not exists canonical_url_verified_at timestamptz;

alter table articles
  add column if not exists last_publish_run_id uuid;

alter table articles
  drop constraint if exists articles_status_check;

alter table articles
  add constraint articles_status_check
  check (status in (
    'generating',
    'pending_review',
    'approved',
    'scheduled',
    'pending_url_verification',
    'published',
    'publish_failed',
    'ready_to_distribute',
    'distributed',
    'rejected'
  ));

alter table generation_runs
  drop constraint if exists generation_runs_agent_check;

alter table generation_runs
  add constraint generation_runs_agent_check
  check (agent in (
    'insight',
    'strategist',
    'writer',
    'qa',
    'publisher',
    'notification'
  ));
