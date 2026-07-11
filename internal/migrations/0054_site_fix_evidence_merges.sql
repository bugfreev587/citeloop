set local lock_timeout = '5s';
set local statement_timeout = '30s';

create table if not exists site_fix_evidence_merges (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  candidate_id uuid not null,
  arbitration_decision_id uuid not null,
  site_fix_id uuid not null,
  doctor_finding_id uuid not null,
  finding_kind text not null check (finding_kind in ('broken','optimization')),
  evidence_fingerprint text not null check (length(btrim(evidence_fingerprint)) > 0),
  evidence_snapshot jsonb not null check (jsonb_typeof(evidence_snapshot) = 'object'),
  created_at timestamptz not null default now(),
  unique (project_id, candidate_id, site_fix_id, evidence_fingerprint),
  constraint site_fix_evidence_merges_candidate_fk
    foreign key (project_id, candidate_id)
    references discovery_candidates(project_id, id)
    on delete no action deferrable initially deferred,
  constraint site_fix_evidence_merges_decision_fk
    foreign key (project_id, arbitration_decision_id)
    references discovery_arbitration_decisions(project_id, id)
    on delete no action deferrable initially deferred,
  constraint site_fix_evidence_merges_site_fix_fk
    foreign key (project_id, site_fix_id)
    references site_fixes(project_id, id)
    on delete no action deferrable initially deferred,
  constraint site_fix_evidence_merges_finding_fk
    foreign key (project_id, doctor_finding_id, finding_kind)
    references seo_doctor_findings(project_id, id, finding_kind)
    on delete no action deferrable initially deferred
);

drop trigger if exists site_fix_evidence_merges_immutable on site_fix_evidence_merges;
create trigger site_fix_evidence_merges_immutable
before update or delete on site_fix_evidence_merges
for each row execute function reject_doctor_append_only_mutation();
