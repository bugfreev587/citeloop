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

-- name: GetGitHubPRReadinessForProject :one
select * from publisher_connections
where project_id = sqlc.arg(project_id)
  and kind = 'github_nextjs'
  and is_default
limit 1;

-- name: SetGitHubPRReadinessIfUnchanged :one
update publisher_connections
set pr_readiness_status = sqlc.arg(pr_readiness_status),
    pr_readiness_checked_at = sqlc.narg(pr_readiness_checked_at),
    pr_readiness_detail = sqlc.narg(pr_readiness_detail),
    updated_at = now()
where id = sqlc.arg(connection_id)
  and project_id = sqlc.arg(project_id)
  and kind = 'github_nextjs'
  and is_default
  and updated_at = sqlc.arg(expected_updated_at)
returning *;

-- name: GetEnabledPublisherConnectionForProject :one
select * from publisher_connections
where project_id = $1
  and kind = $2
  and is_default
  and enabled = true
  and status = 'connected'
limit 1;

-- name: SetPublisherConnectionEnabled :one
update publisher_connections
set enabled = $3,
    pr_readiness_status = case
      when kind = 'github_nextjs' and status = 'connected' and $3 then 'not_checked'
      else 'not_connected'
    end,
    pr_readiness_checked_at = null,
    pr_readiness_detail = null,
    updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: DeletePublisherConnectionForProject :one
delete from publisher_connections
where id = $1 and project_id = $2
returning *;

-- FindReusableGitHubInstallation returns a GitHub App installation_id already
-- linked by ANOTHER project of the SAME owner. A GitHub App installs once per
-- account, so a second project can reuse the existing installation instead of
-- re-running the install flow (which dead-ends on GitHub's "already installed"
-- page). Scoped to the current project's owner so it never crosses tenants.
-- name: FindReusableGitHubInstallation :one
select coalesce(pc.config ->> 'installation_id', '')::text as installation_id
from publisher_connections pc
join projects p on p.id = pc.project_id
where p.owner_id = (select owner.owner_id from projects owner where owner.id = $1)
  and pc.project_id <> $1
  and pc.kind = 'github_nextjs'
  and coalesce(pc.config ->> 'installation_id', '') <> ''
order by pc.updated_at desc
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
  pr_readiness_status = case
    when publisher_connections.kind = 'github_nextjs'
      and excluded.status = 'connected'
      and publisher_connections.enabled then 'not_checked'
    else 'not_connected'
  end,
  pr_readiness_checked_at = null,
  pr_readiness_detail = null,
  updated_at = now()
returning *;

-- name: MarkPublisherConnectionVerified :one
update publisher_connections
set status = 'connected',
    last_verified_at = now(),
    last_error = null,
    pr_readiness_status = case
      when kind = 'github_nextjs' and enabled then 'not_checked'
      else 'not_connected'
    end,
    pr_readiness_checked_at = null,
    pr_readiness_detail = null,
    updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: MarkPublisherConnectionError :one
update publisher_connections
set status = 'error',
    last_error = $3,
    pr_readiness_status = 'not_connected',
    pr_readiness_checked_at = null,
    pr_readiness_detail = null,
    updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: SetPublisherConnectionCredentialRef :one
update publisher_connections
set credential_ref = $3,
    status = 'missing',
    last_error = null,
    pr_readiness_status = 'not_connected',
    pr_readiness_checked_at = null,
    pr_readiness_detail = null,
    updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: ClearPublisherConnectionCredentialRef :one
update publisher_connections
set credential_ref = null,
    status = 'missing',
    last_error = null,
    pr_readiness_status = 'not_connected',
    pr_readiness_checked_at = null,
    pr_readiness_detail = null,
    updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: UpsertPublisherCredential :one
with invalidated_connection as (
  update publisher_connections
  set pr_readiness_status = case
        when kind = 'github_nextjs' and status = 'connected' and enabled then 'not_checked'
        else 'not_connected'
      end,
      pr_readiness_checked_at = null,
      pr_readiness_detail = null,
      updated_at = now()
  where id = sqlc.arg(connection_id)
    and project_id = sqlc.arg(project_id)
  returning id, project_id
)
insert into publisher_credentials
  (project_id, connection_id, kind, encrypted_value, redacted_value)
select
  sqlc.arg(project_id),
  sqlc.arg(connection_id),
  sqlc.arg(kind),
  sqlc.arg(encrypted_value),
  sqlc.arg(redacted_value)
from invalidated_connection
where true
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
with invalidated_connection as (
  update publisher_connections connection
  set pr_readiness_status = case
        when connection.kind = 'github_nextjs'
          and connection.status = 'connected'
          and connection.enabled then 'not_checked'
        else 'not_connected'
      end,
      pr_readiness_checked_at = null,
      pr_readiness_detail = null,
      updated_at = now()
  where connection.id = sqlc.arg(connection_id)
    and connection.project_id = sqlc.arg(project_id)
    and exists (
      select 1
      from publisher_credentials credential
      where credential.project_id = connection.project_id
        and credential.connection_id = connection.id
        and credential.kind = sqlc.arg(kind)
        and credential.revoked_at is null
    )
  returning connection.id, connection.project_id
)
update publisher_credentials credential
set revoked_at = now(),
    updated_at = now()
from invalidated_connection connection
where credential.project_id = connection.project_id
  and credential.connection_id = connection.id
  and credential.kind = sqlc.arg(kind)
  and credential.revoked_at is null
returning credential.*;
