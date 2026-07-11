-- Doctor-owned canonical Site Fixes and the audit-safe legacy cutover foundation.
-- Tenant audit is append-only while its project exists. The explicit project
-- hard-delete path erases the tenant and its audit rows through database cascades.

set local lock_timeout = '5s';
set local statement_timeout = '4min';

create or replace function reject_doctor_append_only_mutation()
returns trigger
language plpgsql
as $$
begin
  if tg_op = 'DELETE' and not exists (
    select 1 from projects where id = old.project_id
  ) then return old;
  end if;
  raise exception '% is append-only', tg_table_name using errcode = '55000';
end;
$$;

create table if not exists migration_batches (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  product text not null default 'doctor' check (product in ('doctor','opportunities','shared')),
  batch_kind text not null check (batch_kind in ('dry_run','forward','rollback')),
  status text not null check (status in ('completed','review_required','failed','rolled_back')),
  schema_version text not null,
  source_count int not null check (source_count >= 0),
  migrated_count int not null check (migrated_count >= 0),
  archived_duplicate_count int not null check (archived_duplicate_count >= 0),
  review_count int not null check (review_count >= 0),
  writer_authority_before text not null check (writer_authority_before in ('legacy','canonical')),
  writer_authority_after text not null check (writer_authority_after in ('legacy','canonical')),
  source_snapshot jsonb not null check (jsonb_typeof(source_snapshot) = 'object'),
  result_snapshot jsonb not null check (jsonb_typeof(result_snapshot) = 'object'),
  initiated_by text not null,
  started_at timestamptz not null,
  finished_at timestamptz not null,
  created_at timestamptz not null default now(),
  check (source_count = migrated_count + archived_duplicate_count + review_count),
  check (finished_at >= started_at),
  unique (project_id, id)
);

drop trigger if exists migration_batches_immutable on migration_batches;
create trigger migration_batches_immutable
before update or delete on migration_batches
for each row execute function reject_doctor_append_only_mutation();

create table if not exists product_writer_authority (
  project_id uuid not null references projects(id) on delete cascade,
  product text not null default 'doctor' check (product in ('doctor','opportunities')),
  writer_authority text not null default 'legacy' check (writer_authority in ('legacy','canonical')),
  write_fenced boolean not null default false,
  fence_token uuid,
  fenced_by text,
  fenced_at timestamptz,
  authority_changed_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key (project_id, product),
  check (
    (write_fenced = false and fence_token is null and fenced_by is null and fenced_at is null)
    or
    (write_fenced = true and fence_token is not null and fenced_by is not null and fenced_at is not null)
  )
);

insert into product_writer_authority (project_id, product, writer_authority, write_fenced)
select p.id, supported.product, 'legacy', false
from projects p
cross join (values ('doctor'), ('opportunities')) as supported(product)
on conflict (project_id, product) do nothing;

create or replace function seed_project_writer_authority()
returns trigger
language plpgsql
as $$
begin
  insert into product_writer_authority (project_id, product, writer_authority, write_fenced)
  values
    (new.id, 'doctor', 'legacy', false),
    (new.id, 'opportunities', 'legacy', false)
  on conflict (project_id, product) do nothing;
  return new;
end;
$$;

drop trigger if exists projects_seed_writer_authority on projects;
create trigger projects_seed_writer_authority
after insert on projects
for each row execute function seed_project_writer_authority();

create table if not exists site_fixes (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  doctor_finding_id uuid not null references seo_doctor_findings(id) on delete restrict,
  candidate_id uuid not null references discovery_candidates(id) on delete restrict,
  work_signature_id uuid not null,
  supersedes_site_fix_id uuid references site_fixes(id) on delete restrict,
  status text not null check (status in (
    'proposed','approved','preparing','ready_to_apply','applying',
    'awaiting_deploy','verifying','verified','failed_retryable',
    'reopened','failed_terminal','superseded','migration_rolled_back'
  )),
  finding_kind text not null check (finding_kind in ('broken','optimization')),
  target_urls jsonb not null default '[]'::jsonb check (jsonb_typeof(target_urls) = 'array'),
  evidence_snapshot jsonb not null default '{}'::jsonb check (jsonb_typeof(evidence_snapshot) = 'object'),
  proposed_fix jsonb not null default '{}'::jsonb check (jsonb_typeof(proposed_fix) = 'object'),
  acceptance_tests jsonb not null default '[]'::jsonb check (jsonb_typeof(acceptance_tests) = 'array'),
  verification_snapshot jsonb not null default '{}'::jsonb check (jsonb_typeof(verification_snapshot) = 'object'),
  failure_reason text,
  retry_count int not null default 0 check (retry_count >= 0),
  max_retries int not null default 3 check (max_retries >= 0),
  legacy_opportunity_id uuid references seo_opportunities(id) on delete set null,
  legacy_content_action_id uuid references content_actions(id) on delete set null,
  migration_batch_id uuid references migration_batches(id) on delete restrict,
  approved_at timestamptz,
  applied_at timestamptz,
  deployed_at timestamptz,
  verified_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  constraint fk_site_fixes_work_signature foreign key (work_signature_id)
    references work_signature_registry(id) on delete restrict deferrable initially deferred,
  unique (project_id, id),
  unique (project_id, candidate_id, id),
  check (id <> supersedes_site_fix_id)
);

create table if not exists site_fix_verifications (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  site_fix_id uuid not null references site_fixes(id) on delete restrict,
  attempt_number int not null check (attempt_number >= 1),
  evidence_read jsonb not null default '{}'::jsonb check (jsonb_typeof(evidence_read) = 'object'),
  acceptance_results jsonb not null default '[]'::jsonb check (jsonb_typeof(acceptance_results) = 'array'),
  ai_call_id uuid references ai_call_records(id) on delete cascade,
  result text not null check (result in ('passed','failed','inconclusive','error')),
  retry_classification text not null check (retry_classification in ('not_applicable','retryable','retry_exhausted','terminal')),
  failure_reason text,
  attempted_at timestamptz not null,
  created_at timestamptz not null default now(),
  unique (site_fix_id, attempt_number),
  check (
    (result = 'passed' and retry_classification = 'not_applicable' and failure_reason is null)
    or
    (result <> 'passed' and retry_classification <> 'not_applicable')
  )
);

drop trigger if exists site_fix_verifications_immutable on site_fix_verifications;
create trigger site_fix_verifications_immutable
before update or delete on site_fix_verifications
for each row execute function reject_doctor_append_only_mutation();

create table if not exists migration_ledger (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  migration_batch_id uuid not null references migration_batches(id) on delete cascade,
  sequence_number int not null check (sequence_number >= 1),
  source_object_type text not null,
  source_object_id uuid not null,
  canonical_object_type text not null,
  canonical_object_id uuid,
  operation text not null check (operation in (
    'create','map','decision_migrate','repoint','archive_duplicate',
    'authority_switch','rollback','tombstone'
  )),
  operation_version text not null,
  cutover_point text not null check (cutover_point in (
    'pre_cutover','writer_fenced','canonical_authority','rollback'
  )),
  rollback_eligibility text not null check (rollback_eligibility in (
    'eligible','not_eligible','not_applicable','blocked_forward_fix_required'
  )),
  before_hash text not null,
  after_hash text not null,
  before_snapshot jsonb not null check (jsonb_typeof(before_snapshot) = 'object'),
  after_snapshot jsonb not null check (jsonb_typeof(after_snapshot) = 'object'),
  inverse_operation_version text not null,
  inverse_operation jsonb not null check (jsonb_typeof(inverse_operation) = 'object'),
  applied_by text not null,
  applied_at timestamptz not null,
  created_at timestamptz not null default now(),
  unique (migration_batch_id, sequence_number),
  unique (migration_batch_id, source_object_type, source_object_id, operation),
  unique (project_id, migration_batch_id, id)
);

drop trigger if exists migration_ledger_immutable on migration_ledger;
create trigger migration_ledger_immutable
before update or delete on migration_ledger
for each row execute function reject_doctor_append_only_mutation();

create table if not exists migration_rollback_events (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  migration_batch_id uuid not null references migration_batches(id) on delete cascade,
  migration_ledger_id uuid references migration_ledger(id) on delete cascade,
  event_sequence int not null check (event_sequence >= 1),
  event_type text not null check (event_type in (
    'rollback_eligibility_assessed','rollback_started',
    'rollback_blocked_forward_fix_required','rollback_completed'
  )),
  rollback_eligibility text not null check (rollback_eligibility in (
    'eligible','not_eligible','not_applicable','blocked_forward_fix_required'
  )),
  cutover_point text not null check (cutover_point in (
    'pre_cutover','writer_fenced','canonical_authority','rollback'
  )),
  reason text not null default '',
  forward_fix_reference text,
  event_snapshot jsonb not null default '{}'::jsonb check (jsonb_typeof(event_snapshot) = 'object'),
  event_version text not null,
  occurred_at timestamptz not null,
  rolled_back_at timestamptz,
  created_at timestamptz not null default now(),
  unique (migration_batch_id, event_sequence),
  check ((event_type = 'rollback_completed') = (rolled_back_at is not null))
);

drop trigger if exists migration_rollback_events_immutable on migration_rollback_events;
create trigger migration_rollback_events_immutable
before update or delete on migration_rollback_events
for each row execute function reject_doctor_append_only_mutation();

create table if not exists migration_review_items (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  migration_batch_id uuid not null references migration_batches(id) on delete cascade,
  source_object_type text not null,
  source_object_id uuid not null,
  reason_code text not null,
  reason text not null,
  source_snapshot jsonb not null default '{}'::jsonb check (jsonb_typeof(source_snapshot) = 'object'),
  proposed_resolution jsonb not null default '{}'::jsonb check (jsonb_typeof(proposed_resolution) = 'object'),
  status text not null default 'pending' check (status in ('pending','resolved','dismissed')),
  resolution_snapshot jsonb check (resolution_snapshot is null or jsonb_typeof(resolution_snapshot) = 'object'),
  resolved_by text,
  resolved_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (migration_batch_id, source_object_type, source_object_id),
  check (
    (status = 'pending' and resolution_snapshot is null and resolved_by is null and resolved_at is null)
    or
    (status in ('resolved','dismissed') and resolution_snapshot is not null and resolved_by is not null and resolved_at is not null)
  )
);

create table if not exists legacy_object_aliases (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  migration_batch_id uuid not null references migration_batches(id) on delete cascade,
  legacy_object_type text not null,
  legacy_object_id uuid not null,
  canonical_object_type text not null,
  canonical_object_id uuid not null,
  alias_state text not null default 'active' check (alias_state in ('active','rolled_back_tombstone')),
  provenance_snapshot jsonb not null default '{}'::jsonb check (jsonb_typeof(provenance_snapshot) = 'object'),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, legacy_object_type, legacy_object_id)
);

-- Existing Page Update rows retain their Content Action source. Canonical Doctor
-- applications use site_fix_id; the XOR is safe to validate for all legacy rows.
alter table site_change_applications
  alter column content_action_id drop not null,
  add column if not exists site_fix_id uuid references site_fixes(id) on delete restrict;

alter table site_change_applications
  drop constraint if exists site_change_applications_exactly_one_source;
alter table site_change_applications
  add constraint site_change_applications_exactly_one_source
  check (num_nonnulls(site_fix_id, content_action_id) = 1) not valid;

-- Legacy site_fix applications may still point at their conserved Content Action;
-- canonical site_fix applications point only at site_fixes.
alter table site_change_applications
  drop constraint if exists site_change_applications_kind_source_consistency;
alter table site_change_applications
  add constraint site_change_applications_kind_source_consistency
  check (
    (application_kind = 'page_update' and content_action_id is not null and site_fix_id is null)
    or
    (application_kind = 'site_fix' and num_nonnulls(site_fix_id, content_action_id) = 1)
  ) not valid;

-- Rollback history predates a required source, so keep source-less legacy rows
-- while preventing a rollback from claiming both canonical source types.
alter table rollback_records
  add column if not exists site_fix_id uuid references site_fixes(id) on delete restrict;
alter table rollback_records
  drop constraint if exists rollback_records_at_most_one_source;
alter table rollback_records
  add constraint rollback_records_at_most_one_source
  check (num_nonnulls(action_id, site_fix_id) <= 1) not valid;

alter table seo_doctor_findings
  add column if not exists finding_kind text not null default 'broken'
    check (finding_kind in ('broken','optimization','healthy'));

alter table seo_doctor_runs
  add column if not exists healthy_coverage jsonb not null default '[]'::jsonb
    check (jsonb_typeof(healthy_coverage) = 'array');

reset statement_timeout;
reset lock_timeout;
