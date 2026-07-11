alter table discovery_candidates
  add column if not exists candidate_version bigint not null default 1
  check (candidate_version >= 1);

create table if not exists ai_call_records (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  run_id uuid,
  stage text not null check (stage in (
    'evidence','doctor_diagnosis','arbitration','fix_generation','verification',
    'growth_hypothesis','brief','content_generation','qa','outcome_learning'
  )),
  linked_object_type text not null,
  linked_object_id uuid not null,
  provider text not null,
  model text not null,
  prompt_version text not null,
  request_fingerprint text not null,
  status text not null check (status in ('queued','running','ok','partial','failed','skipped')),
  error_code text,
  prompt_tokens int not null default 0 check (prompt_tokens >= 0),
  completion_tokens int not null default 0 check (completion_tokens >= 0),
  total_tokens int not null default 0 check (total_tokens >= 0),
  cost_usd numeric(14,8) not null default 0 check (cost_usd >= 0),
  parent_call_id uuid references ai_call_records(id) on delete set null,
  started_at timestamptz not null default now(),
  finished_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists idx_ai_call_records_project_stage_age
  on ai_call_records (project_id, stage, created_at desc);

create index if not exists idx_ai_call_records_linked_object
  on ai_call_records (project_id, linked_object_type, linked_object_id, created_at desc);

create index if not exists idx_ai_call_records_request_fingerprint
  on ai_call_records (project_id, request_fingerprint, created_at desc);

create table if not exists discovery_arbitration_decisions (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  candidate_id uuid not null references discovery_candidates(id) on delete cascade,
  candidate_version bigint not null check (candidate_version >= 1),
  ai_call_id uuid references ai_call_records(id) on delete set null,
  disposition text not null check (disposition in (
    'deterministic_safe','exact_merge','semantic_comparison','review_memory',
    'provider_failure','incomplete_specification','manual_resolution'
  )),
  decision text not null check (decision in (
    'create','merge_evidence','suppress','block_on_other_line','hold'
  )),
  owner text check (owner is null or owner in ('doctor','opportunities')),
  overlap_work_ids jsonb not null default '[]'::jsonb check (jsonb_typeof(overlap_work_ids) = 'array'),
  reason text not null,
  confidence numeric(5,4) not null check (confidence >= 0 and confidence <= 1),
  semantic_fingerprint text not null,
  compared_work_ids jsonb not null default '[]'::jsonb check (jsonb_typeof(compared_work_ids) = 'array'),
  expected_bucket_versions jsonb not null default '{}'::jsonb check (jsonb_typeof(expected_bucket_versions) = 'object'),
  snapshot_fingerprint text not null,
  exact_signature_hash text not null,
  signature_version text not null,
  evidence_fingerprint text not null default '',
  rules_version text not null,
  prompt_version text not null,
  provider text not null,
  model text not null,
  status text not null default 'prepared' check (status in ('prepared','held','reserved','stale','resolved','superseded')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists idx_discovery_arbitration_candidate_age
  on discovery_arbitration_decisions (project_id, candidate_id, created_at desc);

create index if not exists idx_discovery_arbitration_status_age
  on discovery_arbitration_decisions (project_id, status, created_at);

alter table work_signature_registry
  add column if not exists arbitration_decision_id uuid
    references discovery_arbitration_decisions(id) on delete set null,
  add column if not exists reserved_work_type text,
  add column if not exists reserved_work_id uuid,
  add column if not exists evidence_fingerprint text not null default '';

create index if not exists idx_work_signature_registry_reserved_work
  on work_signature_registry (project_id, reserved_work_type, reserved_work_id)
  where reserved_work_id is not null;

create table if not exists work_review_memory (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  candidate_id uuid references discovery_candidates(id) on delete set null,
  work_signature_id uuid references work_signature_registry(id) on delete set null,
  exact_signature_hash_at_decision text not null,
  semantic_fingerprint_at_decision text not null,
  signature_payload jsonb not null check (jsonb_typeof(signature_payload) = 'object'),
  conflict_bucket_keys jsonb not null default '[]'::jsonb check (jsonb_typeof(conflict_bucket_keys) = 'array'),
  signature_version text not null,
  decision text not null check (decision in ('dismissed','snoozed','watching')),
  decision_scope jsonb not null default '{}'::jsonb check (jsonb_typeof(decision_scope) = 'object'),
  evidence_fingerprint_at_decision text not null default '',
  snoozed_until timestamptz,
  material_change_policy_version text not null,
  decided_by text not null,
  decided_at timestamptz not null default now(),
  active boolean not null default true,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index if not exists uniq_active_review_memory_exact_signature
  on work_review_memory (project_id, exact_signature_hash_at_decision)
  where active = true;

create index if not exists idx_work_review_memory_project_decision
  on work_review_memory (project_id, decision, decided_at desc)
  where active = true;

create index if not exists idx_work_review_memory_conflict_buckets
  on work_review_memory using gin (conflict_bucket_keys);

create table if not exists work_signature_aliases (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  review_memory_id uuid not null references work_review_memory(id) on delete cascade,
  alias_exact_signature_hash text not null,
  alias_semantic_fingerprint text not null,
  alias_signature_version text not null,
  created_at timestamptz not null default now(),
  unique (project_id, alias_exact_signature_hash, alias_signature_version)
);

create index if not exists idx_work_signature_aliases_semantic
  on work_signature_aliases (project_id, alias_semantic_fingerprint);

alter table discovery_review_items
  add column if not exists expected_candidate_version bigint not null default 1
    check (expected_candidate_version >= 1),
  add column if not exists internal_owner text not null default 'discovery_ops',
  add column if not exists due_at timestamptz,
  add column if not exists arbitration_decision_id uuid
    references discovery_arbitration_decisions(id) on delete set null;

create table if not exists discovery_semantic_gold_cases (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  left_candidate_id uuid not null references discovery_candidates(id) on delete cascade,
  right_candidate_id uuid not null references discovery_candidates(id) on delete cascade,
  dataset_version text not null,
  label text not null check (label in ('equivalent','distinct','conflict')),
  expected_decision text not null check (expected_decision in ('merge_evidence','suppress','block_on_other_line','create')),
  actual_decision text check (actual_decision is null or actual_decision in ('merge_evidence','suppress','block_on_other_line','create','hold')),
  actual_confidence numeric(5,4) check (actual_confidence is null or (actual_confidence >= 0 and actual_confidence <= 1)),
  reviewed_by text not null,
  reviewed_at timestamptz not null default now(),
  notes text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, dataset_version, left_candidate_id, right_candidate_id),
  check (left_candidate_id <> right_candidate_id)
);

create index if not exists idx_discovery_semantic_gold_dataset
  on discovery_semantic_gold_cases (project_id, dataset_version, label);

create table if not exists discovery_arbitration_configs (
  project_id uuid primary key references projects(id) on delete cascade,
  confidence_threshold numeric(5,4) not null default 0.8000
    check (confidence_threshold >= 0 and confidence_threshold <= 1),
  duplicate_safety_recall_target numeric(5,4) not null default 0.9500
    check (duplicate_safety_recall_target >= 0 and duplicate_safety_recall_target <= 1),
  false_suppression_rate_target numeric(5,4) not null default 0.0200
    check (false_suppression_rate_target >= 0 and false_suppression_rate_target <= 1),
  gold_dataset_version text not null default '',
  weekly_ops_capacity int not null default 0 check (weekly_ops_capacity >= 0),
  launch_ready boolean not null default false,
  automatic_suppression_enabled boolean not null default false,
  rules_version text not null default 'discovery-arbitration-v1',
  prompt_version text not null default 'discovery-semantic-v1',
  semantic_fingerprint_version text not null default 'discovery-semantic-fingerprint-v1',
  evaluated_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  check (not automatic_suppression_enabled or launch_ready)
);
