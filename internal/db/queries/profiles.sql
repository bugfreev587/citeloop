-- name: GetActiveProfile :one
select * from product_profiles
where project_id = $1 and is_active
limit 1;

-- name: ListProfileVersions :many
select * from product_profiles
where project_id = $1
order by version desc;

-- name: DeactivateProfiles :exec
update product_profiles set is_active = false, updated_at = now()
where project_id = $1 and is_active;

-- name: NextProfileVersion :one
select coalesce(max(version), 0) + 1 from product_profiles
where project_id = $1;

-- name: InsertProfile :one
insert into product_profiles (project_id, source_urls, profile, version, is_active)
values ($1, $2, $3, $4, true)
returning *;

-- name: UpdateProfile :one
update product_profiles set profile = $2, source_urls = $3, updated_at = now()
where id = $1
returning *;
