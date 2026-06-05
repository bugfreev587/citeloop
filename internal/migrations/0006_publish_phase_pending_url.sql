-- Existing databases may already have applied 0004 before publish_phase and
-- pending_url_verification existed. Add the forward-only upgrade separately.

alter table articles
  add column if not exists publish_phase text;

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
