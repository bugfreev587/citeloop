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

-- name: ListProjectsByOwner :many
select * from projects
where owner_id = $1
order by created_at desc;

-- name: UpdateProjectConfig :one
update projects set config = $2 where id = $1
returning *;

-- name: UpdateProjectConfigForOwner :one
update projects set config = $2
where id = $1
  and owner_id = $3
returning *;
