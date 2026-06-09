-- name: CreateProject :one
insert into projects (owner_id, name, slug, config)
values ($1, $2, $3, $4)
returning *;

-- name: GetProject :one
select * from projects where id = $1;

-- name: GetProjectForOwner :one
select * from projects
where id = $1
  and owner_id = $2;

-- name: GetProjectBySlug :one
select * from projects where slug = $1;

-- name: ListProjects :many
select * from projects
where (sqlc.arg(status)::text = '' or status = sqlc.arg(status))
order by created_at desc;

-- name: ListProjectsByOwner :many
select * from projects
where owner_id = $1
  and (sqlc.arg(status)::text = '' or status = sqlc.arg(status))
order by created_at desc;

-- name: ArchiveProjectForOwner :one
update projects set status = 'archived'
where id = $1
  and owner_id = $2
returning *;

-- name: RestoreProjectForOwner :one
update projects set status = 'active'
where id = $1
  and owner_id = $2
returning *;

-- name: CountProjectDeleteBlockers :one
select
  (
    select count(*) from articles a
    where a.project_id = sqlc.arg(project_id_arg)
      and a.status in ('pending_url_verification', 'published', 'publish_failed', 'ready_to_distribute', 'distributed')
  )::bigint as published_content_count,
  (
    (select count(*) from product_profiles pp where pp.project_id = sqlc.arg(project_id_arg)) +
    (select count(*) from content_inventory ci where ci.project_id = sqlc.arg(project_id_arg)) +
    (select count(*) from topics t where t.project_id = sqlc.arg(project_id_arg)) +
    (select count(*) from articles a where a.project_id = sqlc.arg(project_id_arg)) +
    (select count(*) from generation_runs gr where gr.project_id = sqlc.arg(project_id_arg)) +
    (select count(*) from notification_channels nc where nc.project_id = sqlc.arg(project_id_arg)) +
    (select count(*) from notification_subscriptions ns where ns.project_id = sqlc.arg(project_id_arg)) +
    (select count(*) from notification_deliveries nd where nd.project_id = sqlc.arg(project_id_arg))
  )::bigint as operational_record_count;

-- name: DeleteEmptyProjectForOwner :one
delete from projects
where projects.id = sqlc.arg(project_id_arg)
  and projects.owner_id = sqlc.arg(owner_id_arg)
  and not exists (select 1 from product_profiles pp where pp.project_id = sqlc.arg(project_id_arg))
  and not exists (select 1 from content_inventory ci where ci.project_id = sqlc.arg(project_id_arg))
  and not exists (select 1 from topics t where t.project_id = sqlc.arg(project_id_arg))
  and not exists (select 1 from articles a where a.project_id = sqlc.arg(project_id_arg))
  and not exists (select 1 from generation_runs gr where gr.project_id = sqlc.arg(project_id_arg))
  and not exists (select 1 from notification_channels nc where nc.project_id = sqlc.arg(project_id_arg))
  and not exists (select 1 from notification_subscriptions ns where ns.project_id = sqlc.arg(project_id_arg))
  and not exists (select 1 from notification_deliveries nd where nd.project_id = sqlc.arg(project_id_arg))
returning *;

-- name: UpdateProjectConfig :one
update projects set config = $2 where id = $1
returning *;

-- name: UpdateProjectConfigForOwner :one
update projects set config = $2
where id = $1
  and owner_id = $3
returning *;
