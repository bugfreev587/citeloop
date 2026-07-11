create table if not exists discovery_shadow_runs (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  mode text not null default 'shadow' check (mode in ('shadow')),
  status text not null default 'running' check (status in ('running','completed','failed')),
  candidate_schema_version text not null,
  signature_version text not null,
  doctor_candidates int not null default 0 check (doctor_candidates >= 0),
  opportunity_candidates int not null default 0 check (opportunity_candidates >= 0),
  identity_ready int not null default 0 check (identity_ready >= 0),
  needs_specification int not null default 0 check (needs_specification >= 0),
  exact_duplicate_groups int not null default 0 check (exact_duplicate_groups >= 0),
  possible_conflict_groups int not null default 0 check (possible_conflict_groups >= 0),
  error text,
  started_at timestamptz not null default now(),
  finished_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists discovery_candidates (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  shadow_run_id uuid not null references discovery_shadow_runs(id) on delete cascade,
  source_kind text not null check (source_kind in ('doctor','signal_scan','ai_discovery','migration')),
  source_object_type text not null,
  source_object_id text not null,
  target_kind text not null check (target_kind in ('page','query','prompt','entity','site','integration')),
  normalized_target_set jsonb not null default '[]'::jsonb check (jsonb_typeof(normalized_target_set) = 'array'),
  issue_or_hypothesis_family text not null,
  change_family text not null,
  proposed_mutations jsonb not null default '[]'::jsonb check (jsonb_typeof(proposed_mutations) = 'array'),
  artifact_intent text not null check (artifact_intent in ('repair_existing_surface','update_existing_content','create_new_asset','consolidate_assets','measurement_only')),
  intended_slug_or_canonical text,
  topic_entity_identity jsonb not null default '[]'::jsonb check (jsonb_typeof(topic_entity_identity) = 'array'),
  audience_identity jsonb not null default '[]'::jsonb check (jsonb_typeof(audience_identity) = 'array'),
  primary_success_metric text not null default '',
  verification_mode text not null check (verification_mode in ('immediate','delayed')),
  evidence_ids jsonb not null default '[]'::jsonb check (jsonb_typeof(evidence_ids) = 'array'),
  evidence_fingerprint text not null default '',
  suggested_owner text not null check (suggested_owner in ('doctor','opportunities')),
  confidence numeric(5,4) not null default 0 check (confidence >= 0 and confidence <= 1),
  candidate_schema_version text not null,
  status text not null check (status in ('identity_ready','needs_specification','needs_evidence','needs_arbitration_review')),
  hold_reason text,
  exact_signature_hash text,
  signature_payload jsonb check (signature_payload is null or jsonb_typeof(signature_payload) = 'object'),
  conflict_bucket_keys jsonb not null default '[]'::jsonb check (jsonb_typeof(conflict_bucket_keys) = 'array'),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index if not exists uniq_discovery_candidate_source_version
  on discovery_candidates
    (project_id, source_kind, source_object_type, source_object_id, candidate_schema_version);

create index if not exists idx_discovery_candidates_project_run_status
  on discovery_candidates (project_id, shadow_run_id, status, created_at desc);

create index if not exists idx_discovery_candidates_exact_signature
  on discovery_candidates (project_id, exact_signature_hash)
  where exact_signature_hash is not null;

create table if not exists work_conflict_buckets (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  bucket_key text not null,
  bucket_version bigint not null default 0 check (bucket_version >= 0),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, bucket_key)
);

create table if not exists work_signature_registry (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  candidate_id uuid not null references discovery_candidates(id) on delete cascade,
  shadow_run_id uuid not null references discovery_shadow_runs(id) on delete cascade,
  mode text not null default 'shadow' check (mode in ('shadow','enforced')),
  status text not null default 'shadow_observed' check (status in ('shadow_observed','reserved','proposed','approved','preparing','executing','awaiting_deploy','verifying','measuring','blocked','watching','snoozed','failed_retryable','reopened','verified','learned','dismissed','superseded','cancelled','failed_terminal')),
  active boolean not null default false,
  exact_signature_hash text not null,
  signature_payload jsonb not null check (jsonb_typeof(signature_payload) = 'object'),
  semantic_fingerprint text,
  conflict_bucket_keys jsonb not null default '[]'::jsonb check (jsonb_typeof(conflict_bucket_keys) = 'array'),
  signature_version text not null,
  owner text check (owner is null or owner in ('doctor','opportunities')),
  source_object_type text not null,
  source_object_id text not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (candidate_id)
);

create unique index if not exists uniq_enforced_active_work_signature
  on work_signature_registry (project_id, exact_signature_hash)
  where mode = 'enforced' and active = true;

create index if not exists idx_work_signature_registry_project_run
  on work_signature_registry (project_id, shadow_run_id, created_at desc);

create index if not exists idx_work_signature_registry_exact_hash
  on work_signature_registry (project_id, exact_signature_hash);

create index if not exists idx_work_signature_registry_conflict_buckets
  on work_signature_registry using gin (conflict_bucket_keys);

create table if not exists discovery_review_items (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  candidate_id uuid not null references discovery_candidates(id) on delete cascade,
  state text not null check (state in ('needs_specification','needs_evidence','needs_arbitration_review','resolved')),
  reason text not null,
  assignee text,
  expected_bucket_versions jsonb not null default '{}'::jsonb check (jsonb_typeof(expected_bucket_versions) = 'object'),
  resolution jsonb check (resolution is null or jsonb_typeof(resolution) = 'object'),
  resolved_by text,
  resolved_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (candidate_id)
);

create index if not exists idx_discovery_review_items_project_state_age
  on discovery_review_items (project_id, state, created_at);
