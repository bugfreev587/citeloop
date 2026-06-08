-- name: ListPublisherConnections :many
select * from publisher_connections
where project_id = $1
order by is_default desc, created_at desc;

-- name: GetPublisherConnectionForProject :one
select * from publisher_connections
where id = $1 and project_id = $2;

-- name: GetDefaultPublisherConnectionForProject :one
select * from publisher_connections
where project_id = $1 and kind = $2 and is_default
limit 1;

-- name: UpsertDefaultPublisherConnection :one
insert into publisher_connections
  (project_id, kind, label, status, is_default, capabilities, capability_schema_version, credential_ref, config, last_verified_at, last_error)
values
  ($1, $2, $3, $4, true, $5, $6, $7, $8, $9, $10)
on conflict (project_id, kind) where is_default
do update set
  label = excluded.label,
  status = excluded.status,
  capabilities = excluded.capabilities,
  capability_schema_version = excluded.capability_schema_version,
  credential_ref = excluded.credential_ref,
  config = excluded.config,
  last_verified_at = excluded.last_verified_at,
  last_error = excluded.last_error,
  updated_at = now()
returning *;

-- name: MarkPublisherConnectionVerified :one
update publisher_connections
set status = 'connected',
    last_verified_at = now(),
    last_error = null,
    updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: MarkPublisherConnectionError :one
update publisher_connections
set status = 'error',
    last_error = $3,
    updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: SetPublisherConnectionCredentialRef :one
update publisher_connections
set credential_ref = $3,
    status = 'missing',
    last_error = null,
    updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: ClearPublisherConnectionCredentialRef :one
update publisher_connections
set credential_ref = null,
    status = 'missing',
    last_error = null,
    updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: UpsertPublisherCredential :one
insert into publisher_credentials
  (project_id, connection_id, kind, encrypted_value, redacted_value)
values
  ($1, $2, $3, $4, $5)
on conflict (project_id, connection_id, kind)
do update set
  encrypted_value = excluded.encrypted_value,
  redacted_value = excluded.redacted_value,
  revoked_at = null,
  updated_at = now()
returning *;

-- name: GetActivePublisherCredential :one
select * from publisher_credentials
where id = $1
  and project_id = $2
  and connection_id = $3
  and revoked_at is null;

-- name: GetActivePublisherCredentialForConnection :one
select * from publisher_credentials
where project_id = $1
  and connection_id = $2
  and kind = $3
  and revoked_at is null
limit 1;

-- name: RevokePublisherCredentialForConnection :one
update publisher_credentials
set revoked_at = now(),
    updated_at = now()
where project_id = $1
  and connection_id = $2
  and kind = $3
  and revoked_at is null
returning *;
