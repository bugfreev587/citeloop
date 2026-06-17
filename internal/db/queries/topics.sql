-- name: CreateTopic :one
insert into topics
  (project_id, channel, title, target_keyword, target_prompt, angle, format, priority, internal_links, status, scheduled_at, source_content_action_id)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, sqlc.narg(source_content_action_id))
returning *;

-- name: GetTopic :one
select * from topics where id = $1;

-- name: GetTopicForProject :one
select * from topics
where id = $1 and project_id = $2;

-- name: ListTopics :many
select * from topics
where project_id = $1
order by priority asc, created_at desc;

-- name: UpdateTopic :one
update topics set
  channel = $3,
  title = $4,
  target_keyword = $5,
  target_prompt = $6,
  angle = $7,
  format = $8,
  priority = $9,
  internal_links = $10,
  status = $11,
  scheduled_at = $12,
  source_content_action_id = sqlc.narg(source_content_action_id)
where id = $1 and project_id = $2
returning *;

-- name: UpdateTopicStatus :one
update topics set status = $2 where id = $1
returning *;

-- name: UpdateTopicStatusForProject :one
update topics set status = $3
where id = $1 and project_id = $2
returning *;

-- name: StartTopicGenerationForProject :one
update topics set status = 'generating'
where id = $1 and project_id = $2 and status in ('backlog','scheduled')
returning *;

-- name: SetTopicScheduledAt :one
update topics set scheduled_at = $2 where id = $1
returning *;

-- name: SetTopicScheduledAtForProject :one
update topics set
  scheduled_at = $3,
  status = case when $3::timestamptz is null then 'backlog' else 'scheduled' end
where id = $1 and project_id = $2
returning *;

-- name: ArchiveTopicForProject :one
update topics set status = 'archived', scheduled_at = null
where id = $1 and project_id = $2
returning *;

-- Scheduler candidate selection: backlog/scheduled topics with a slot inside the
-- buffer window, locked to avoid concurrent double-generation (§5.4).
-- name: SelectGenerationCandidates :many
select * from topics
where project_id = $1
  and status in ('backlog','scheduled')
order by
  case when source_content_action_id is not null then 0 else 1 end,
  priority asc,
  created_at asc
limit $2
for update skip locked;

-- Scheduler: topics whose operator-set scheduled_at slot has arrived. Unlike
-- SelectGenerationCandidates this is time-driven and ignores buffer/priority,
-- since the operator explicitly scheduled the slot (§5.4). Locked to avoid
-- concurrent double-generation.
-- name: SelectDueScheduledTopics :many
select * from topics
where project_id = $1
  and status = 'scheduled'
  and scheduled_at is not null
  and scheduled_at <= now()
order by scheduled_at asc
for update skip locked;

-- name: CountNonRejectedArticlesForTopic :one
select count(*) from articles
where topic_id = $1 and status <> 'rejected';

-- name: ListArticlesByTopicForProject :many
select * from articles
where topic_id = $1 and project_id = $2
order by kind, platform;

-- name: IncrementTopicRecoveryAttempt :one
update topics set recovery_attempts = recovery_attempts + 1
where id = $1 and project_id = $2
returning *;
