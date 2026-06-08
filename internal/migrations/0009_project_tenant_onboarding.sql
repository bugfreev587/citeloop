-- Project slugs are user-facing within a tenant. A globally unique slug leaks
-- tenant state and prevents two customers from onboarding the same product name.
alter table projects
  drop constraint if exists projects_slug_key;

create unique index if not exists projects_owner_slug_key
  on projects (owner_id, slug);
