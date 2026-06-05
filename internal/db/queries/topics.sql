-- name: CreateTopic :one
insert into topics
  (project_id, channel, title, target_keyword, target_prompt, angle, format, priority, internal_links, status, scheduled_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
returning *;

-- name: GetTopic :one
select * from topics where id = $1;

-- name: ListTopics :many
select * from topics
where project_id = $1
order by priority desc, created_at desc;

-- name: UpdateTopicStatus :one
update topics set status = $2 where id = $1
returning *;

-- name: SetTopicScheduledAt :one
update topics set scheduled_at = $2 where id = $1
returning *;

-- Scheduler candidate selection: backlog/scheduled topics with a slot inside the
-- buffer window, locked to avoid concurrent double-generation (§5.4).
-- name: SelectGenerationCandidates :many
select * from topics
where project_id = $1
  and status in ('backlog','scheduled')
order by priority desc, created_at asc
limit $2
for update skip locked;

-- name: CountNonRejectedArticlesForTopic :one
select count(*) from articles
where topic_id = $1 and status <> 'rejected';
