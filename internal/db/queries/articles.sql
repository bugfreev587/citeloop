-- name: CreateArticle :one
insert into articles
  (project_id, topic_id, kind, platform, content_md, seo_meta,
   geo_score, seo_score, qa_issues, qa_blocking, status)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
returning *;

-- name: GetArticle :one
select * from articles where id = $1;

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

-- name: UpdateArticleContent :one
update articles set content_md = $2, seo_meta = $3 where id = $1
returning *;

-- name: ApproveArticle :one
update articles set
  status = $2,
  scheduled_at = $3,
  reviewed_by = $4,
  reviewed_at = now()
where id = $1
returning *;

-- name: RejectArticle :one
update articles set status = 'rejected', reviewed_by = $2, reviewed_at = now()
where id = $1
returning *;

-- name: SetArticleQA :one
update articles set
  geo_score = $2, seo_score = $3, qa_issues = $4, qa_blocking = $5, status = $6
where id = $1
returning *;

-- Publisher: canonical articles due for auto-publish (§5.6).
-- name: SelectDueCanonical :many
select * from articles
where project_id = $1
  and kind = 'canonical'
  and status = 'approved'
  and scheduled_at is not null
  and scheduled_at <= now()
for update skip locked;

-- name: MarkPublished :one
update articles set
  status = 'published',
  published_at = now(),
  publish_result = $2,
  canonical_url = $3
where id = $1
returning *;

-- syndication unlock: variants whose canonical is published (§5.6).
-- name: SelectUnlockableVariants :many
select v.* from articles v
join articles c
  on c.topic_id = v.topic_id and c.kind = 'canonical'
where v.kind = 'syndication_variant'
  and v.status = 'approved'
  and c.status = 'published'
  and c.canonical_url is not null;

-- name: UnlockVariant :one
update articles set
  status = 'ready_to_distribute',
  canonical_url = $2,
  content_md = $3,
  seo_meta = $4          -- canonical placeholder backfilled in seo_meta too (§5.6)
where id = $1
returning *;

-- CountStockedCanonical counts canonical articles already in flight toward
-- publishing (not backlog, not terminal). The scheduler uses this to fill only
-- the buffer-window deficit instead of regenerating every tick (§5.4).
-- name: CountStockedCanonical :one
select count(*) from articles
where project_id = $1
  and kind = 'canonical'
  and status in ('generating','pending_review','approved','scheduled');

-- name: MarkDistributed :one
update articles set status = 'distributed' where id = $1
returning *;
