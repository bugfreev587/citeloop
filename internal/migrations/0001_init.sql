-- CiteLoop initial schema (PRD §3). All business entities are project-scoped.
-- Targets PostgreSQL; uses gen_random_uuid() from pgcrypto (built-in on PG13+).

create extension if not exists pgcrypto;

create table projects (
  id          uuid primary key default gen_random_uuid(),
  owner_id    text not null,
  name        text not null,
  slug        text not null unique,
  config      jsonb not null default '{}',   -- see projects.config schema (§3)
  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now()
);

create table product_profiles (
  id          uuid primary key default gen_random_uuid(),
  project_id  uuid not null references projects(id),
  source_urls jsonb not null default '[]',
  profile     jsonb not null,
  version     int not null default 1,
  is_active   boolean not null default true,
  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now()
);
-- single active profile per project
create unique index one_active_profile_per_project
  on product_profiles (project_id) where is_active;

create table content_inventory (
  id             uuid primary key default gen_random_uuid(),
  project_id     uuid not null references projects(id),
  url            text not null,            -- canonical-normalized (§5.1)
  title          text,
  target_keyword text,
  topics         jsonb not null default '[]',
  summary        text,
  evidence_snippets jsonb not null default '[]', -- supports QA evidence mapping (§5.3)
  source         text not null default 'existing'
                 check (source in ('existing','generated')),
  captured_at    timestamptz not null default now(),
  unique (project_id, url)                 -- dedupe on normalized url
);

create table topics (
  id              uuid primary key default gen_random_uuid(),
  project_id      uuid not null references projects(id),
  channel         text not null check (channel in ('blog','syndication','both')),
  title           text not null,
  target_keyword  text,
  target_prompt   text,
  angle           text,
  format          text,
  priority        int  not null default 0,
  internal_links  jsonb not null default '[]',
  status          text not null default 'backlog'
                 check (status in ('backlog','scheduled','generating','drafted','done','archived')),
  scheduled_at    timestamptz,             -- planned publish time (scheduling intent)
  created_at      timestamptz not null default now()
);

create table articles (
  id             uuid primary key default gen_random_uuid(),
  project_id     uuid not null references projects(id),
  topic_id       uuid not null references topics(id),
  kind           text not null check (kind in ('canonical','syndication_variant')),
  platform       text,                     -- null for canonical
  content_md     text not null,
  seo_meta       jsonb not null default '{}',
  geo_score      numeric,
  seo_score      numeric,
  qa_issues      jsonb not null default '[]',
  qa_blocking    boolean not null default false,  -- unresolved blocking evidence issue
  canonical_url  text,                     -- real URL, backfilled only after canonical publish
  status         text not null default 'generating'
                 check (status in ('generating','pending_review','approved',
                                    'scheduled','pending_url_verification',
                                    'published','publish_failed',
                                    'ready_to_distribute','distributed','rejected')),
  scheduled_at   timestamptz,              -- this artifact's publish time (source of truth, §3 state machine)
  reviewed_by    text,
  reviewed_at    timestamptz,
  published_at   timestamptz,
  publish_result jsonb,
  last_publish_error text,
  publish_attempts int not null default 0,
  next_publish_retry_at timestamptz,
  publish_phase text,
  resolved_slug text,
  publish_path text,
  canonical_url_verified_at timestamptz,
  last_publish_run_id uuid,
  created_at     timestamptz not null default now(),
  -- canonical must have no platform; variant must have a platform
  check ((kind='canonical' and platform is null)
      or (kind='syndication_variant' and platform is not null))
);
-- at most one (kind, platform) per topic (platform-less canonical uses empty string)
create unique index uniq_article_topic_kind_platform
  on articles (topic_id, kind, coalesce(platform,''));

create table generation_runs (   -- observability + cost audit (breaker basis)
  id          uuid primary key default gen_random_uuid(),
  project_id  uuid not null references projects(id),
  agent       text not null check (agent in ('insight','strategist','writer','qa','publisher','notification')),
  input       jsonb,
  output      jsonb,        -- includes search result snapshot (§5.2)
  model       text,
  tokens      int,
  cost_usd    numeric,
  status      text not null check (status in ('ok','error')),
  error       text,
  created_at  timestamptz not null default now()
);
