set local lock_timeout = '5s';
set local statement_timeout = '30s';

create table if not exists doctor_site_fix_preparation_leases (
  project_id uuid not null references projects(id) on delete cascade,
  exact_signature_hash text not null check (length(btrim(exact_signature_hash)) > 0),
  lease_token uuid not null,
  runtime_authority_fingerprint text not null check (length(btrim(runtime_authority_fingerprint)) > 0),
  leader_candidate_id uuid not null,
  arbitration_decision_id uuid,
  resolved_provider text,
  resolved_model text,
  status text not null check (status in ('preparing','completed','failed')),
  lease_expires_at timestamptz not null,
  result_expires_at timestamptz,
  attempt_count bigint not null default 1 check (attempt_count >= 1),
  last_error_code text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  completed_at timestamptz,
  primary key (project_id, exact_signature_hash),
  unique (project_id, lease_token),
  constraint doctor_site_fix_preparation_candidate_fk
    foreign key (project_id, leader_candidate_id)
    references discovery_candidates(project_id, id)
    on delete no action deferrable initially deferred,
  constraint doctor_site_fix_preparation_decision_fk
    foreign key (project_id, arbitration_decision_id)
    references discovery_arbitration_decisions(project_id, id)
    on delete no action deferrable initially deferred,
  check (
    (status = 'preparing' and arbitration_decision_id is null and resolved_provider is null and resolved_model is null and result_expires_at is null and completed_at is null)
    or (status = 'completed' and arbitration_decision_id is not null and resolved_provider is not null and resolved_model is not null and result_expires_at is not null and completed_at is not null)
    or (status = 'failed' and result_expires_at is null)
  )
);
