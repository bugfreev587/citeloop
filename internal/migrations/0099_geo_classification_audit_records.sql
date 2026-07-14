-- Persist derived entity/citation classifications so GEO attribution decisions
-- can be inspected and replayed when classifier logic changes.

create table if not exists geo_classification_audit_records (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  run_id uuid not null references geo_runs(id) on delete cascade,
  observation_id uuid references geo_observations(id) on delete set null,
  classifier_type text not null,
  input jsonb not null default '{}',
  output jsonb not null default '{}',
  reason_codes jsonb not null default '[]',
  created_at timestamptz not null default now()
);

create index if not exists idx_geo_classification_audit_project_run
  on geo_classification_audit_records (project_id, run_id, created_at desc);

create index if not exists idx_geo_classification_audit_observation
  on geo_classification_audit_records (observation_id)
  where observation_id is not null;
