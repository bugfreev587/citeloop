alter table seo_policies
  add column if not exists automation_paused boolean not null default false;
