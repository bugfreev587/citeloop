-- name: CreateArticle :one
insert into articles
  (project_id, topic_id, kind, platform, content_md, seo_meta,
   geo_score, seo_score, qa_issues, qa_blocking, status, content_hash,
   repair_attempts, repair_status, requires_human_decision, human_decision_options, qa_feedback)
values (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
  encode(digest(coalesce($5::text, '') || coalesce($6::jsonb::text, ''), 'sha256'), 'hex'),
  $12, $13, $14, $15, $16
)
returning *;

-- name: GetArticle :one
select * from articles where id = $1;

-- name: GetArticleForProject :one
select * from articles
where id = $1 and project_id = $2;

-- name: ListArticlesByStatus :many
select * from articles
where project_id = $1 and status = $2
order by created_at desc;

-- name: ListArticlesByTopic :many
select * from articles where topic_id = $1 order by kind, platform;

-- name: ListPendingReview :many
select * from articles
where project_id = $1 and status = 'pending_review'
order by created_at asc;

-- name: ListOverdueReviewArticles :many
select * from articles
where status = 'pending_review'
  and created_at <= $1
order by created_at asc
limit $2;

-- name: UpdateArticleContent :one
update articles set
  content_md = $2,
  seo_meta = $3,
  content_hash = encode(digest(coalesce($2::text, '') || coalesce($3::jsonb::text, ''), 'sha256'), 'hex')
where id = $1
returning *;

-- name: UpdateArticleContentForProject :one
update articles set
  content_md = $2,
  seo_meta = $3,
  content_hash = encode(digest(coalesce($2::text, '') || coalesce($3::jsonb::text, ''), 'sha256'), 'hex')
where id = $1 and project_id = $4
returning *;

-- name: UpdateArticleDistributionMetadataForProject :one
update articles set
  publication_mode = $3,
  source_url = sqlc.narg(source_url),
  external_url = sqlc.narg(external_url),
  verification_status = $4,
  external_surface_id = sqlc.narg(external_surface_id)
where id = $1 and project_id = $2
returning *;

-- name: ApproveArticle :one
update articles set
  status = $2,
  scheduled_at = $3,
  reviewed_by = $4,
  reviewed_at = now()
where id = $1
returning *;

-- name: ApproveArticleForProject :one
update articles set
  status = $2,
  scheduled_at = $3,
  reviewed_by = $4,
  reviewed_at = now()
where id = $1 and project_id = $5
returning *;

-- name: RejectArticle :one
update articles set status = 'rejected', reviewed_by = $2, reviewed_at = now()
where id = $1
returning *;

-- name: RejectArticleForProject :one
update articles set status = 'rejected', reviewed_by = $2, reviewed_at = now()
where id = $1 and project_id = $3
returning *;

-- name: SetArticleQA :one
update articles set
  geo_score = $2, seo_score = $3, qa_issues = $4, qa_blocking = $5, status = $6, qa_feedback = $7
where id = $1
returning *;

-- name: StartArticleRepairForProject :one
update articles set
  repair_attempts = repair_attempts + 1,
  last_repair_at = now(),
  repair_status = 'repairing',
  repair_failure_reason = null,
  requires_human_decision = false,
  human_decision_options = '[]'::jsonb
where id = $1
  and project_id = $2
  and repair_attempts < $3
returning *;

-- name: FinishArticleRepairForProject :one
update articles set
  repair_status = $3,
  repair_failure_reason = $4,
  requires_human_decision = $5,
  human_decision_options = $6,
  qa_feedback = $7
where id = $1
  and project_id = $2
returning *;

-- Publisher: canonical articles due for auto-publish (§5.6).
-- name: SelectDueCanonical :many
select * from articles
where project_id = $1
  and kind = 'canonical'
  and (
    status = 'approved'
    or (status = 'publish_failed' and next_publish_retry_at is not null and next_publish_retry_at <= now())
  )
  and scheduled_at is not null
  and scheduled_at <= now()
for update skip locked;

-- name: PreparePublishAttempt :one
update articles set
  resolved_slug = $2,
  publish_path = $3,
  publish_phase = $4,
  publish_attempts = publish_attempts + 1,
  next_publish_retry_at = null,
  last_publish_error = null
where id = $1
returning *;

-- name: MarkPublished :one
update articles set
  status = 'published',
  published_at = now(),
  publish_result = $2,
  canonical_url = $3,
  resolved_slug = $4,
  publish_path = $5,
  publish_phase = 'published',
  canonical_url_verified_at = now(),
  last_publish_error = null,
  next_publish_retry_at = null
where id = $1
returning *;

-- name: RecordPublishAttemptResult :one
update articles set
  status = 'pending_url_verification',
  publish_result = $2,
  resolved_slug = $3,
  publish_path = $4,
  publish_phase = 'pending_url_verification',
  last_publish_error = null,
  next_publish_retry_at = $5
where id = $1
returning *;

-- name: MarkPublishFailed :one
update articles set
  status = 'publish_failed',
  last_publish_error = $2,
  next_publish_retry_at = $3,
  publish_phase = $4,
  canonical_url_verified_at = null
where id = $1
returning *;

-- name: RetryPublishArticle :one
update articles set
  next_publish_retry_at = now(),
  last_publish_error = null
where id = $1
  and project_id = $2
  and status = 'publish_failed'
returning *;

-- name: SelectPublishReconcileCandidates :many
select * from articles
where project_id = $1
  and kind = 'canonical'
  and (
    (status in ('approved','publish_failed') and publish_result is not null)
    or
    status = 'pending_url_verification'
    or
    (status = 'published' and (canonical_url is null or canonical_url_verified_at is null or publish_result is null))
  )
order by created_at asc;

-- syndication unlock: variants whose canonical is published (§5.6).
-- name: SelectUnlockableVariants :many
select v.* from articles v
join articles c
  on c.topic_id = v.topic_id and c.kind = 'canonical'
where v.kind = 'syndication_variant'
  and v.status = 'approved'
  and c.status = 'published'
  and c.canonical_url is not null
  and c.canonical_url_verified_at is not null;

-- name: UnlockVariant :one
update articles set
  status = 'ready_to_distribute',
  canonical_url = $2,
  content_md = $3,
  seo_meta = $4,          -- canonical placeholder backfilled in seo_meta too (§5.6)
  content_hash = encode(digest(coalesce($3::text, '') || coalesce($4::jsonb::text, ''), 'sha256'), 'hex')
where id = $1
returning *;

-- CountStockedCanonical counts canonical articles already in flight toward
-- publishing plus topics reserved for generation before their first article
-- exists. The scheduler uses this to fill only the buffer-window deficit
-- instead of regenerating every tick (§5.4).
-- name: CountStockedCanonical :one
select count(*) from (
  select articles.topic_id from articles
  where articles.project_id = $1
    and articles.kind = 'canonical'
    and articles.status in ('generating','pending_review','approved','scheduled','pending_url_verification')
  union
  select topics.id as topic_id from topics
  where topics.project_id = $1
    and topics.status = 'generating'
) stocked;

-- name: MarkDistributed :one
update articles set status = 'distributed' where id = $1
returning *;

-- name: MarkDistributedForProject :one
update articles set status = 'distributed'
where id = $1 and project_id = $2
returning *;

-- Review auto-recovery (§5.5): drafts CiteLoop can still resolve on its own —
-- blocked, not yet a genuine human decision. The recovery tick re-runs QA,
-- repairs, or regenerates these without involving a human.
-- name: ListRecoverableArticlesForProject :many
select * from articles
where project_id = $1
  and status = 'pending_review'
  and qa_blocking = true
order by requires_human_decision asc, created_at asc
limit $2;

-- ListApprovableForProject lists pending_review drafts QA has cleared, for
-- hands-off auto-approval when the project runs in auto-advance mode.
-- name: ListApprovableForProject :many
select * from articles
where project_id = $1
  and status = 'pending_review'
  and qa_blocking = false
order by created_at asc
limit $2;

-- name: IncrementArticleRecoveryAttempt :one
update articles set
  recovery_attempts = recovery_attempts + 1,
  last_repair_at = now()
where id = $1 and project_id = $2
returning *;

-- EscalateArticleToHumanForProject flips a draft into the genuine human-decision
-- state after automated recovery is exhausted or QA returned a real unmapped
-- claim a human must resolve.
-- name: EscalateArticleToHumanForProject :one
update articles set
  requires_human_decision = true,
  repair_status = 'human_decision',
  repair_failure_reason = $3,
  human_decision_options = $4
where id = $1 and project_id = $2
returning *;

-- DeleteRecoverableArticlesForTopic clears a topic's non-terminal drafts so the
-- recovery loop can regenerate a fresh canonical/variant without colliding with
-- the (topic, kind, platform) unique index. Published/approved rows are kept.
-- name: DeleteRecoverableArticlesForTopic :exec
delete from articles
where topic_id = $1 and project_id = $2
  and status in ('pending_review','rejected','generating');

-- LatestCanonicalPublishSlotForProject returns the latest publish slot already
-- taken by a project's canonical articles (scheduled or published), so a new
-- approval can be staggered after it instead of publishing immediately.
-- name: LatestCanonicalPublishSlotForProject :one
select coalesce(
  max(greatest(coalesce(scheduled_at, to_timestamp(0)), coalesce(published_at, to_timestamp(0)))),
  to_timestamp(0)
)::timestamptz as slot
from articles
where project_id = $1
  and kind = 'canonical'
  and status in ('approved','scheduled','pending_url_verification','published');

-- PublishArticleNowForProject brings an approved canonical's publish slot to now
-- so the next publish tick sends it out — the operator's "Publish now" override
-- (and the way manual-mode drafts get published).
-- name: PublishArticleNowForProject :one
update articles set scheduled_at = now()
where id = $1 and project_id = $2 and kind = 'canonical' and status = 'approved'
returning *;
