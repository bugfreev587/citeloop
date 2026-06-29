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
select * from projects order by created_at desc;

-- name: ListAdminProjects :many
select * from projects order by updated_at desc, created_at desc;

-- name: ListAdminUsers :many
select
  owner_id,
  count(*)::bigint as project_count,
  min(created_at) as created_at,
  max(updated_at) as updated_at
from projects
group by owner_id
order by updated_at desc, created_at desc;

-- name: ListProjectsByOwner :many
select * from projects
where owner_id = $1
order by created_at desc;

-- name: UpdateProjectConfig :one
update projects set config = $2, updated_at = now() where id = $1
returning *;

-- name: UpdateProjectConfigForOwner :one
update projects set config = $2, updated_at = now()
where id = $1
  and owner_id = $3
returning *;

-- name: DeleteProjectForOwner :one
delete from projects
where id = $1
  and owner_id = $2
returning *;

-- name: DeleteProject :one
delete from projects
where id = $1
returning *;

-- name: DeleteProjectsByOwner :many
delete from projects
where owner_id = $1
returning *;
