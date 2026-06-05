-- name: CreateProject :one
insert into projects (owner_id, name, slug, config)
values ($1, $2, $3, $4)
returning *;

-- name: GetProject :one
select * from projects where id = $1;

-- name: GetProjectBySlug :one
select * from projects where slug = $1;

-- name: ListProjects :many
select * from projects order by created_at desc;

-- name: UpdateProjectConfig :one
update projects set config = $2 where id = $1
returning *;
