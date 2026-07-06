alter table publisher_connections
  drop constraint if exists publisher_connections_kind_check;

alter table publisher_connections
  add constraint publisher_connections_kind_check
  check (kind in ('github_nextjs','webhook','wordpress','dev_to'));

alter table publisher_credentials
  drop constraint if exists publisher_credentials_kind_check;

alter table publisher_credentials
  add constraint publisher_credentials_kind_check
  check (kind in ('github_token','webhook_secret','oauth_refresh_token','dev_to_api_key'));
