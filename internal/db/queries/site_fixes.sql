-- Canonical Doctor Site Fix persistence. Every lifecycle mutation is scoped to
-- one project and advances the arbitration conflict-bucket snapshot in the
-- same statement as the Site Fix and enforced work-signature transition.

-- name: CreateCanonicalSiteFix :one
insert into site_fixes (
  id, project_id, doctor_finding_id, candidate_id, work_signature_id,
  supersedes_site_fix_id, status, finding_kind, target_urls,
  evidence_snapshot, proposed_fix, acceptance_tests, verification_snapshot,
  failure_reason, retry_count, max_retries, legacy_opportunity_id,
  legacy_content_action_id, migration_batch_id, approved_at, applied_at,
  deployed_at, verified_at, created_at, updated_at
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(doctor_finding_id),
  sqlc.arg(candidate_id), sqlc.arg(work_signature_id),
  sqlc.narg(supersedes_site_fix_id), sqlc.arg(status), sqlc.arg(finding_kind),
  sqlc.arg(target_urls)::jsonb, sqlc.arg(evidence_snapshot)::jsonb,
  sqlc.arg(proposed_fix)::jsonb, sqlc.arg(acceptance_tests)::jsonb,
  sqlc.arg(verification_snapshot)::jsonb, sqlc.narg(failure_reason),
  sqlc.arg(retry_count), sqlc.arg(max_retries),
  sqlc.narg(legacy_opportunity_id), sqlc.narg(legacy_content_action_id),
  sqlc.narg(migration_batch_id), sqlc.narg(approved_at), sqlc.narg(applied_at),
  sqlc.narg(deployed_at), sqlc.narg(verified_at),
  sqlc.arg(created_at), sqlc.arg(updated_at)
)
returning *;

-- name: GetCanonicalSiteFix :one
select * from site_fixes
where id = sqlc.arg(id)
  and project_id = sqlc.arg(project_id);

-- name: ListCanonicalSiteFixes :many
select * from site_fixes
where project_id = sqlc.arg(project_id)
  and (sqlc.narg(status)::text is null or status = sqlc.narg(status)::text)
order by updated_at desc, id asc
limit 250;

-- name: ListLatestCanonicalSiteFixApplications :many
with listed_site_fixes as (
  select id
  from site_fixes
  where project_id = sqlc.arg(project_id)
    and (sqlc.narg(status)::text is null or status = sqlc.narg(status)::text)
  order by updated_at desc, id asc
  limit 250
)
select distinct on (application.site_fix_id) application.*
from site_change_applications application
join listed_site_fixes listed on listed.id = application.site_fix_id
where application.project_id = sqlc.arg(project_id)
  and application.site_fix_id is not null
  and application.content_action_id is null
order by application.site_fix_id, application.updated_at desc, application.id asc;

-- name: ListCanonicalSiteFixVerificationsForList :many
with listed_site_fixes as (
  select id
  from site_fixes
  where project_id = sqlc.arg(project_id)
    and (sqlc.narg(status)::text is null or status = sqlc.narg(status)::text)
  order by updated_at desc, id asc
  limit 250
)
select verification.*
from site_fix_verifications verification
join listed_site_fixes listed on listed.id = verification.site_fix_id
where verification.project_id = sqlc.arg(project_id)
order by verification.site_fix_id, verification.attempt_number asc, verification.id asc;

-- name: GetCanonicalSiteFixByWorkSignature :one
select * from site_fixes
where project_id = sqlc.arg(project_id)
  and work_signature_id = sqlc.arg(work_signature_id);

-- name: ClaimDoctorSiteFixPreparationLease :one
with claimed as (
  insert into doctor_site_fix_preparation_leases (
    project_id, exact_signature_hash, lease_token, runtime_authority_fingerprint, leader_candidate_id,
    status, lease_expires_at, attempt_count
  ) values (
    sqlc.arg(project_id), sqlc.arg(exact_signature_hash), sqlc.arg(lease_token), sqlc.arg(runtime_authority_fingerprint),
    sqlc.arg(leader_candidate_id), 'preparing',
    clock_timestamp() + make_interval(secs => sqlc.arg(lease_ttl_seconds)::int), 1
  )
  on conflict (project_id, exact_signature_hash) do update set
    lease_token = excluded.lease_token,
    runtime_authority_fingerprint = excluded.runtime_authority_fingerprint,
    leader_candidate_id = excluded.leader_candidate_id,
    arbitration_decision_id = null,
    resolved_provider = null,
    resolved_model = null,
    status = 'preparing',
    lease_expires_at = excluded.lease_expires_at,
    result_expires_at = null,
    attempt_count = doctor_site_fix_preparation_leases.attempt_count + 1,
    last_error_code = null,
    updated_at = now(),
    completed_at = null
  where doctor_site_fix_preparation_leases.status = 'failed'
     or doctor_site_fix_preparation_leases.runtime_authority_fingerprint <> excluded.runtime_authority_fingerprint
     or (
       doctor_site_fix_preparation_leases.status = 'preparing'
       and doctor_site_fix_preparation_leases.lease_expires_at <= clock_timestamp()
     )
     or (
       doctor_site_fix_preparation_leases.status = 'completed'
       and doctor_site_fix_preparation_leases.result_expires_at <= clock_timestamp()
     )
  returning doctor_site_fix_preparation_leases.*
)
select claimed.*, true as is_leader from claimed
union all
select current.*, false as is_leader
from doctor_site_fix_preparation_leases current
where current.project_id = sqlc.arg(project_id)
  and current.exact_signature_hash = sqlc.arg(exact_signature_hash)
  and not exists (select 1 from claimed)
limit 1;

-- name: GetDoctorSiteFixPreparationLease :one
select * from doctor_site_fix_preparation_leases
where project_id = sqlc.arg(project_id)
  and exact_signature_hash = sqlc.arg(exact_signature_hash);

-- name: ValidateDoctorSiteFixPreparationLease :one
update doctor_site_fix_preparation_leases
set lease_expires_at = clock_timestamp() + make_interval(secs => sqlc.arg(lease_ttl_seconds)::int),
    updated_at = now()
where project_id = sqlc.arg(project_id)
  and exact_signature_hash = sqlc.arg(exact_signature_hash)
  and lease_token = sqlc.arg(lease_token)
  and status = 'preparing'
  and lease_expires_at > clock_timestamp()
returning *;

-- name: CompleteDoctorSiteFixPreparationLease :one
update doctor_site_fix_preparation_leases
set arbitration_decision_id = sqlc.arg(arbitration_decision_id),
    resolved_provider = sqlc.arg(resolved_provider),
    resolved_model = sqlc.arg(resolved_model),
    status = 'completed',
    result_expires_at = clock_timestamp() + make_interval(secs => sqlc.arg(result_ttl_seconds)::int),
    completed_at = clock_timestamp(),
    updated_at = now(),
    last_error_code = null
where project_id = sqlc.arg(project_id)
  and exact_signature_hash = sqlc.arg(exact_signature_hash)
  and lease_token = sqlc.arg(lease_token)
  and status = 'preparing'
  and lease_expires_at > clock_timestamp()
returning *;

-- name: FailDoctorSiteFixPreparationLease :one
update doctor_site_fix_preparation_leases
set status = 'failed', result_expires_at = null, completed_at = null,
    last_error_code = sqlc.arg(error_code), updated_at = now()
where project_id = sqlc.arg(project_id)
  and exact_signature_hash = sqlc.arg(exact_signature_hash)
  and lease_token = sqlc.arg(lease_token)
  and status = 'preparing'
returning *;

-- name: InvalidateDoctorSiteFixPreparationLease :one
update doctor_site_fix_preparation_leases
set status = 'failed', result_expires_at = null, completed_at = null,
    last_error_code = 'stale_result', updated_at = now()
where project_id = sqlc.arg(project_id)
  and exact_signature_hash = sqlc.arg(exact_signature_hash)
  and lease_token = sqlc.arg(observed_lease_token)
  and status = 'completed'
returning *;

-- name: LockDoctorSiteFixPreparationLeaseForReserve :one
update doctor_site_fix_preparation_leases
set lease_expires_at = clock_timestamp() + make_interval(secs => sqlc.arg(lease_ttl_seconds)::int),
    updated_at = now()
from discovery_candidates candidate
where doctor_site_fix_preparation_leases.project_id = sqlc.arg(project_id)
  and doctor_site_fix_preparation_leases.exact_signature_hash = sqlc.arg(exact_signature_hash)
  and doctor_site_fix_preparation_leases.lease_token = sqlc.arg(lease_token)
  and doctor_site_fix_preparation_leases.leader_candidate_id = sqlc.arg(leader_candidate_id)
  and doctor_site_fix_preparation_leases.status = 'preparing'
  and doctor_site_fix_preparation_leases.lease_expires_at > clock_timestamp()
  and candidate.project_id = doctor_site_fix_preparation_leases.project_id
  and candidate.id = doctor_site_fix_preparation_leases.leader_candidate_id
  and candidate.status = 'identity_ready'
  and candidate.exact_signature_hash = doctor_site_fix_preparation_leases.exact_signature_hash
returning doctor_site_fix_preparation_leases.*;

-- name: GetCompletedDoctorSiteFixPreparationDecision :one
select decision.*
from doctor_site_fix_preparation_leases preparation
join discovery_arbitration_decisions decision
  on decision.project_id = preparation.project_id
 and decision.id = preparation.arbitration_decision_id
join discovery_candidates candidate
  on candidate.project_id = decision.project_id
 and candidate.id = decision.candidate_id
where preparation.project_id = sqlc.arg(project_id)
  and preparation.exact_signature_hash = sqlc.arg(exact_signature_hash)
  and preparation.runtime_authority_fingerprint = sqlc.arg(runtime_authority_fingerprint)
  and preparation.status = 'completed'
  and preparation.result_expires_at > clock_timestamp()
  and decision.exact_signature_hash = preparation.exact_signature_hash
  and decision.candidate_id = sqlc.arg(candidate_id)
  and decision.evidence_fingerprint = sqlc.arg(evidence_fingerprint)
  and decision.candidate_version = candidate.candidate_version
  and preparation.resolved_provider = decision.provider
  and preparation.resolved_model = decision.model
  and decision.rules_version = sqlc.arg(rules_version)
  and decision.prompt_version = sqlc.arg(prompt_version)
  and not exists (
    select 1
    from jsonb_each_text(decision.expected_bucket_versions) expected(bucket_key, bucket_version)
    left join work_conflict_buckets bucket
      on bucket.project_id = decision.project_id
     and bucket.bucket_key = expected.bucket_key
    where bucket.id is null
       or bucket.bucket_version <> expected.bucket_version::bigint
  );

-- name: GetCanonicalSiteFixEvidenceMerge :one
select evidence_merge.*
from site_fix_evidence_merges evidence_merge
join site_fixes sf
  on sf.project_id = evidence_merge.project_id
 and sf.id = evidence_merge.site_fix_id
join work_signature_registry registry
  on registry.project_id = sf.project_id
 and registry.id = sf.work_signature_id
where evidence_merge.project_id = sqlc.arg(project_id)
  and evidence_merge.candidate_id = sqlc.arg(candidate_id)
  and evidence_merge.evidence_fingerprint = sqlc.arg(evidence_fingerprint)
  and registry.mode = 'enforced'
  and registry.active = true
  and registry.owner = 'doctor'
  and registry.reserved_work_type = 'site_fix'
  and registry.reserved_work_id = sf.id
  and sf.status in (
    'proposed','approved','preparing','ready_to_apply','applying',
    'awaiting_deploy','verifying','failed_retryable','reopened'
  )
order by evidence_merge.created_at desc, evidence_merge.id asc
limit 1;

-- name: CreateCanonicalSiteFixEvidenceMerge :one
with existing_link as materialized (
  select evidence_merge.*
  from site_fix_evidence_merges evidence_merge
  where evidence_merge.project_id = sqlc.arg(project_id)
    and evidence_merge.candidate_id = sqlc.arg(candidate_id)
    and evidence_merge.site_fix_id = sqlc.arg(site_fix_id)
    and evidence_merge.doctor_finding_id = sqlc.arg(doctor_finding_id)
    and evidence_merge.finding_kind = sqlc.arg(finding_kind)
    and evidence_merge.evidence_fingerprint = sqlc.arg(evidence_fingerprint)
), authority as materialized (
  select product_writer_authority.project_id
  from product_writer_authority
  where product_writer_authority.project_id = sqlc.arg(project_id)
    and product_writer_authority.product = 'doctor'
    and product_writer_authority.writer_authority = 'canonical'
    and product_writer_authority.write_fenced = false
  for update
), source_snapshot as materialized (
  select decision.id, decision.expected_bucket_versions
  from discovery_arbitration_decisions decision
  join authority on authority.project_id = decision.project_id
  where decision.project_id = sqlc.arg(project_id)
    and decision.id = sqlc.arg(arbitration_decision_id)
    and not exists (select 1 from existing_link)
), expected_keys as materialized (
  select expected.bucket_key, expected.bucket_version::bigint as bucket_version
  from source_snapshot snapshot
  cross join lateral jsonb_each_text(snapshot.expected_bucket_versions)
    expected(bucket_key, bucket_version)
  order by expected.bucket_key
), locked_buckets as materialized (
  select bucket.id, bucket.bucket_key
  from work_conflict_buckets bucket
  join expected_keys expected
    on expected.bucket_key = bucket.bucket_key
   and expected.bucket_version = bucket.bucket_version
  where bucket.project_id = sqlc.arg(project_id)
  order by bucket.bucket_key
  for update of bucket
), locked_work as materialized (
  select decision.id as decision_id
  from discovery_arbitration_decisions decision
  join discovery_candidates candidate
    on candidate.project_id = decision.project_id
   and candidate.id = decision.candidate_id
  join seo_doctor_findings finding
    on finding.project_id = candidate.project_id
   and finding.id = sqlc.arg(doctor_finding_id)
   and finding.finding_kind = sqlc.arg(finding_kind)
  join site_fixes sf
    on sf.project_id = candidate.project_id
   and sf.id = sqlc.arg(site_fix_id)
  join work_signature_registry registry
    on registry.project_id = sf.project_id
   and registry.id = sf.work_signature_id
  join doctor_site_fix_preparation_leases preparation
    on preparation.project_id = candidate.project_id
   and preparation.exact_signature_hash = sqlc.arg(exact_signature_hash)
  join authority on authority.project_id = decision.project_id
  where decision.project_id = sqlc.arg(project_id)
    and decision.id = sqlc.arg(arbitration_decision_id)
    and decision.candidate_id = sqlc.arg(candidate_id)
    and decision.decision = 'merge_evidence'
    and decision.owner = 'doctor'
    and decision.status = 'prepared'
    and decision.candidate_version = candidate.candidate_version
    and decision.exact_signature_hash = sqlc.arg(exact_signature_hash)
    and decision.exact_signature_hash = candidate.exact_signature_hash
    and decision.evidence_fingerprint = sqlc.arg(evidence_fingerprint)
    and candidate.status = 'identity_ready'
    and candidate.source_kind = 'doctor'
    and candidate.source_object_type = 'seo_doctor_finding'
    and candidate.source_object_id = sqlc.arg(doctor_finding_id)::text
    and candidate.evidence_fingerprint = sqlc.arg(evidence_fingerprint)
    and finding.status = 'active'
    and finding.updated_at = sqlc.arg(expected_finding_updated_at)
    and registry.mode = 'enforced'
    and registry.active = true
    and registry.owner = 'doctor'
    and registry.reserved_work_type = 'site_fix'
    and registry.reserved_work_id = sf.id
    and registry.exact_signature_hash = decision.exact_signature_hash
    and decision.overlap_work_ids @> jsonb_build_array(registry.id::text)
    and sf.status in (
      'proposed','approved','preparing','ready_to_apply','applying',
      'awaiting_deploy','verifying','failed_retryable','reopened'
    )
    and preparation.lease_token = sqlc.arg(lease_token)
    and preparation.leader_candidate_id = candidate.id
    and preparation.status = 'preparing'
    and preparation.lease_expires_at > clock_timestamp()
    and (select count(*) from locked_buckets) = (select count(*) from expected_keys)
  for update of decision, candidate, finding, sf, registry, preparation
), bumped_buckets as (
  update work_conflict_buckets bucket
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where bucket.id = locked.id
    and exists (select 1 from locked_work)
  returning bucket.id
), resolved_decision as (
  update discovery_arbitration_decisions decision
  set status = 'resolved', updated_at = now()
  from locked_work work
  where decision.project_id = sqlc.arg(project_id)
    and decision.id = work.decision_id
    and decision.status = 'prepared'
    and (select count(*) from bumped_buckets) = (select count(*) from expected_keys)
  returning decision.id
), inserted_link as (
insert into site_fix_evidence_merges (
  id, project_id, candidate_id, arbitration_decision_id, site_fix_id,
  doctor_finding_id, finding_kind, evidence_fingerprint, evidence_snapshot
)
select sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(candidate_id),
       decision.id, sqlc.arg(site_fix_id), sqlc.arg(doctor_finding_id),
       sqlc.arg(finding_kind), sqlc.arg(evidence_fingerprint),
       sqlc.arg(evidence_snapshot)::jsonb
from resolved_decision decision
returning site_fix_evidence_merges.*
)
select * from inserted_link
union all
select * from existing_link
limit 1;

-- name: LockCanonicalSiteFixForUpdate :one
select * from site_fixes
where id = sqlc.arg(id)
  and project_id = sqlc.arg(project_id)
for update;

-- name: GetLatestCanonicalSiteFixForFindingForUpdate :one
select * from site_fixes
where project_id = sqlc.arg(project_id)
  and doctor_finding_id = sqlc.arg(doctor_finding_id)
order by created_at desc, id desc
limit 1
for update;

-- name: GetActiveCanonicalSiteFixForFindingForUpdate :one
select * from site_fixes
where project_id = sqlc.arg(project_id)
  and doctor_finding_id = sqlc.arg(doctor_finding_id)
  and status in (
    'proposed','approved','preparing','ready_to_apply','applying',
    'awaiting_deploy','verifying','failed_retryable','reopened'
  )
order by created_at desc, id desc
limit 1
for update;

-- name: AppendCanonicalSiteFixVerification :one
insert into site_fix_verifications (
  id, project_id, site_fix_id, attempt_number, evidence_read,
  acceptance_results, ai_call_id, result, retry_classification,
  failure_reason, attempted_at
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(site_fix_id),
  sqlc.arg(attempt_number), sqlc.arg(evidence_read)::jsonb,
  sqlc.arg(acceptance_results)::jsonb, sqlc.narg(ai_call_id),
  sqlc.arg(result), sqlc.arg(retry_classification),
  sqlc.narg(failure_reason), sqlc.arg(attempted_at)
)
returning *;

-- name: ListCanonicalSiteFixVerifications :many
select * from site_fix_verifications
where project_id = sqlc.arg(project_id)
  and site_fix_id = sqlc.arg(site_fix_id)
order by attempt_number asc;

-- name: CreateCanonicalSiteFixApplication :one
with source_snapshot as materialized (
  select sf.id, sf.project_id, sf.work_signature_id,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         w.conflict_bucket_keys
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  where sf.project_id = sqlc.arg(project_id)
    and sf.id = sqlc.arg(site_fix_id)
    and sf.status in ('ready_to_apply','applying')
    and w.status = 'executing'
    and w.mode = 'enforced'
    and w.active = true
), expected_keys as materialized (
  select distinct keys.bucket_key
  from source_snapshot s
  cross join lateral jsonb_array_elements_text(s.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_work as materialized (
  select s.*
  from source_snapshot s
  join site_fixes sf on sf.id = s.id and sf.project_id = s.project_id
  join work_signature_registry w
    on w.id = s.work_signature_id and w.project_id = s.project_id
  where sf.status = s.expected_fix_status
    and sf.status in ('ready_to_apply','applying')
    and w.status = s.expected_signature_status
    and w.status = 'executing'
    and w.active = s.expected_signature_active
    and w.active = true
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = s.conflict_bucket_keys
    and jsonb_array_length(s.conflict_bucket_keys) > 0
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
)
insert into site_change_applications (
  id, project_id, source_opportunity_id, content_action_id, site_fix_id,
  page_update_draft_id, application_kind, target_url,
  normalized_target_url, opportunity_key, source_file_paths,
  source_mapping_confidence, source_mapping_reason, patch_snapshot,
  diff_snapshot, resolution_criteria, status
) select
  sqlc.arg(id), sqlc.arg(project_id), null, null, sqlc.arg(site_fix_id),
  null, 'site_fix', sqlc.arg(target_url), sqlc.arg(normalized_target_url),
  sqlc.arg(opportunity_key), sqlc.arg(source_file_paths)::jsonb,
  sqlc.arg(source_mapping_confidence), sqlc.arg(source_mapping_reason),
  sqlc.arg(patch_snapshot)::jsonb, sqlc.arg(diff_snapshot)::jsonb,
  sqlc.arg(resolution_criteria)::jsonb, sqlc.arg(status)
from locked_work work
where work.id = sqlc.arg(site_fix_id)
  and sqlc.arg(status)::text in (
    'draft_ready','source_mapping_required','ready_for_pr','manual_apply_required'
  )
  and (select count(*) from bumped) =
      (select count(*) from expected_keys)
returning *;

-- name: RepointApplicationToCanonicalSiteFix :one
with source_snapshot as materialized (
  select sf.id, sf.project_id, sf.work_signature_id,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         w.conflict_bucket_keys
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  where sf.project_id = sqlc.arg(project_id)
    and sf.id = sqlc.arg(site_fix_id)
    and w.mode = 'enforced'
    and (
      (w.active = true and (
        (sf.status = 'proposed' and w.status in ('reserved','proposed'))
        or (sf.status = 'approved' and w.status = 'approved')
        or (sf.status = 'preparing' and w.status = 'preparing')
        or (sf.status in ('ready_to_apply','applying') and w.status = 'executing')
        or (sf.status = 'awaiting_deploy' and w.status = 'awaiting_deploy')
        or (sf.status = 'verifying' and w.status = 'verifying')
        or (sf.status = 'failed_retryable' and w.status = 'failed_retryable')
        or (sf.status = 'reopened' and w.status = 'reopened')
      ))
      or (w.active = false and (
        (sf.status = 'verified' and w.status = 'verified')
        or (sf.status = 'failed_terminal' and w.status = 'failed_terminal')
        or (sf.status = 'superseded' and w.status = 'superseded')
      ))
    )
), expected_keys as materialized (
  select distinct keys.bucket_key
  from source_snapshot s
  cross join lateral jsonb_array_elements_text(s.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id, a.content_action_id
  from site_change_applications a
  where a.project_id = sqlc.arg(project_id)
    and a.id = sqlc.arg(application_id)
    and a.content_action_id = sqlc.arg(content_action_id)
    and a.site_fix_id is null
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of a
), locked_content_action as materialized (
  select ca.id
  from content_actions ca
  join locked_application a on a.content_action_id = ca.id
  where ca.project_id = sqlc.arg(project_id)
    and ca.id = sqlc.arg(content_action_id)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of ca
), locked_work as materialized (
  select s.*
  from source_snapshot s
  join site_fixes sf on sf.id = s.id and sf.project_id = s.project_id
  join work_signature_registry w
    on w.id = s.work_signature_id and w.project_id = s.project_id
  where sf.status = s.expected_fix_status
    and w.status = s.expected_signature_status
    and w.active = s.expected_signature_active
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = s.conflict_bucket_keys
    and jsonb_array_length(s.conflict_bucket_keys) > 0
    and exists (select 1 from locked_application)
    and exists (select 1 from locked_content_action)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
)
update site_change_applications a
set site_fix_id = sqlc.arg(site_fix_id),
    content_action_id = null,
    application_kind = 'site_fix',
    updated_at = now()
from locked_application locked
cross join locked_work work
where a.id = locked.id
  and a.project_id = sqlc.arg(project_id)
  and a.content_action_id = sqlc.arg(content_action_id)
  and a.site_fix_id is null
  and work.id = sqlc.arg(site_fix_id)
  and (select count(*) from bumped) =
      (select count(*) from expected_keys)
returning a.*;

-- name: RestoreApplicationToLegacyContentAction :one
with source_snapshot as materialized (
  select sf.id, sf.project_id, sf.work_signature_id,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         w.conflict_bucket_keys
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  where sf.project_id = sqlc.arg(project_id)
    and sf.id = sqlc.arg(site_fix_id)
    and w.mode = 'enforced'
    and (
      (w.active = true and (
        (sf.status = 'proposed' and w.status in ('reserved','proposed'))
        or (sf.status = 'approved' and w.status = 'approved')
        or (sf.status = 'preparing' and w.status = 'preparing')
        or (sf.status in ('ready_to_apply','applying') and w.status = 'executing')
        or (sf.status = 'awaiting_deploy' and w.status = 'awaiting_deploy')
        or (sf.status = 'verifying' and w.status = 'verifying')
        or (sf.status = 'failed_retryable' and w.status = 'failed_retryable')
        or (sf.status = 'reopened' and w.status = 'reopened')
      ))
      or (w.active = false and (
        (sf.status = 'verified' and w.status = 'verified')
        or (sf.status = 'failed_terminal' and w.status = 'failed_terminal')
        or (sf.status = 'superseded' and w.status = 'superseded')
        or (sf.status = 'migration_rolled_back' and w.status = 'cancelled')
      ))
    )
), expected_keys as materialized (
  select distinct keys.bucket_key
  from source_snapshot s
  cross join lateral jsonb_array_elements_text(s.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id
  from site_change_applications a
  where a.project_id = sqlc.arg(project_id)
    and a.id = sqlc.arg(application_id)
    and a.site_fix_id = sqlc.arg(site_fix_id)
    and a.content_action_id is null
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of a
), locked_content_action as materialized (
  select ca.id
  from content_actions ca
  join locked_application a on true
  where ca.project_id = sqlc.arg(project_id)
    and ca.id = sqlc.arg(content_action_id)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of ca
), locked_work as materialized (
  select s.*
  from source_snapshot s
  join site_fixes sf on sf.id = s.id and sf.project_id = s.project_id
  join work_signature_registry w
    on w.id = s.work_signature_id and w.project_id = s.project_id
  where sf.status = s.expected_fix_status
    and w.status = s.expected_signature_status
    and w.active = s.expected_signature_active
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = s.conflict_bucket_keys
    and jsonb_array_length(s.conflict_bucket_keys) > 0
    and exists (select 1 from locked_application)
    and exists (select 1 from locked_content_action)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
)
update site_change_applications a
set content_action_id = sqlc.arg(content_action_id),
    site_fix_id = null,
    application_kind = 'site_fix',
    updated_at = now()
from locked_application locked
cross join locked_content_action action
cross join locked_work work
where a.id = locked.id
  and a.project_id = sqlc.arg(project_id)
  and a.site_fix_id = sqlc.arg(site_fix_id)
  and a.content_action_id is null
  and action.id = sqlc.arg(content_action_id)
  and work.id = sqlc.arg(site_fix_id)
  and (select count(*) from bumped) =
      (select count(*) from expected_keys)
returning a.*;

-- name: GetProductWriterAuthority :one
select * from product_writer_authority
where project_id = sqlc.arg(project_id)
  and product = sqlc.arg(product);

-- name: LockProductWriterAuthority :one
select * from product_writer_authority
where project_id = sqlc.arg(project_id)
  and product = sqlc.arg(product)
for update;

-- name: FenceProductWriterAuthority :one
update product_writer_authority
set write_fenced = true,
    fence_token = sqlc.arg(fence_token),
    fenced_by = sqlc.arg(fenced_by),
    fenced_at = sqlc.arg(fenced_at),
    updated_at = now()
where project_id = sqlc.arg(project_id)
  and product = sqlc.arg(product)
  and write_fenced = false
returning *;

-- name: SwitchProductWriterAuthority :one
update product_writer_authority
set writer_authority = sqlc.arg(writer_authority),
    authority_changed_at = sqlc.arg(authority_changed_at),
    updated_at = now()
where project_id = sqlc.arg(project_id)
  and product = sqlc.arg(product)
  and write_fenced = true
  and fence_token = sqlc.arg(fence_token)
  and writer_authority = sqlc.arg(expected_writer_authority)
returning *;

-- name: ReleaseProductWriterFence :one
update product_writer_authority
set write_fenced = false,
    fence_token = null,
    fenced_by = null,
    fenced_at = null,
    updated_at = now()
where project_id = sqlc.arg(project_id)
  and product = sqlc.arg(product)
  and write_fenced = true
  and fence_token = sqlc.arg(fence_token)
returning *;

-- name: CreateMigrationBatch :one
insert into migration_batches (
  id, project_id, product, batch_kind, status, schema_version,
  source_count, migrated_count, archived_duplicate_count, review_count,
  writer_authority_before, writer_authority_after, source_snapshot,
  result_snapshot, initiated_by, started_at, finished_at
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(product), sqlc.arg(batch_kind),
  sqlc.arg(status), sqlc.arg(schema_version), sqlc.arg(source_count),
  sqlc.arg(migrated_count), sqlc.arg(archived_duplicate_count),
  sqlc.arg(review_count), sqlc.arg(writer_authority_before),
  sqlc.arg(writer_authority_after), sqlc.arg(source_snapshot)::jsonb,
  sqlc.arg(result_snapshot)::jsonb, sqlc.arg(initiated_by),
  sqlc.arg(started_at), sqlc.arg(finished_at)
)
returning *;

-- name: AppendMigrationLedger :one
insert into migration_ledger (
  id, project_id, migration_batch_id, sequence_number, source_object_type,
  source_object_id, canonical_object_type, canonical_object_id, operation,
  operation_version, cutover_point, rollback_eligibility, before_hash,
  after_hash, before_snapshot, after_snapshot, inverse_operation_version,
  inverse_operation, applied_by, applied_at
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(migration_batch_id),
  sqlc.arg(sequence_number), sqlc.arg(source_object_type),
  sqlc.arg(source_object_id), sqlc.arg(canonical_object_type),
  sqlc.narg(canonical_object_id), sqlc.arg(operation),
  sqlc.arg(operation_version), sqlc.arg(cutover_point),
  sqlc.arg(rollback_eligibility), sqlc.arg(before_hash), sqlc.arg(after_hash),
  sqlc.arg(before_snapshot)::jsonb, sqlc.arg(after_snapshot)::jsonb,
  sqlc.arg(inverse_operation_version), sqlc.arg(inverse_operation)::jsonb,
  sqlc.arg(applied_by), sqlc.arg(applied_at)
)
returning *;

-- name: CreateMigrationReviewItem :one
insert into migration_review_items (
  id, project_id, migration_batch_id, source_object_type, source_object_id,
  reason_code, reason, source_snapshot, proposed_resolution, status
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(migration_batch_id),
  sqlc.arg(source_object_type), sqlc.arg(source_object_id),
  sqlc.arg(reason_code), sqlc.arg(reason), sqlc.arg(source_snapshot)::jsonb,
  sqlc.arg(proposed_resolution)::jsonb, 'pending'
)
returning *;

-- name: ResolveMigrationReviewItem :one
update migration_review_items
set status = sqlc.arg(status),
    resolution_snapshot = sqlc.arg(resolution_snapshot)::jsonb,
    resolved_by = sqlc.arg(resolved_by),
    resolved_at = sqlc.arg(resolved_at),
    updated_at = now()
where project_id = sqlc.arg(project_id)
  and id = sqlc.arg(id)
  and status = 'pending'
returning *;

-- name: AppendMigrationRollbackEvent :one
insert into migration_rollback_events (
  id, project_id, migration_batch_id, migration_ledger_id, event_sequence,
  event_type, rollback_eligibility, cutover_point, reason,
  forward_fix_reference, event_snapshot, event_version, occurred_at,
  rolled_back_at
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(migration_batch_id),
  sqlc.narg(migration_ledger_id), sqlc.arg(event_sequence),
  sqlc.arg(event_type), sqlc.arg(rollback_eligibility),
  sqlc.arg(cutover_point), sqlc.arg(reason),
  sqlc.narg(forward_fix_reference), sqlc.arg(event_snapshot)::jsonb,
  sqlc.arg(event_version), sqlc.arg(occurred_at), sqlc.narg(rolled_back_at)
)
returning *;

-- name: CreateLegacyObjectAlias :one
insert into legacy_object_aliases (
  id, project_id, migration_batch_id, legacy_object_type, legacy_object_id,
  canonical_object_type, canonical_object_id, alias_state,
  provenance_snapshot
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(migration_batch_id),
  sqlc.arg(legacy_object_type), sqlc.arg(legacy_object_id),
  sqlc.arg(canonical_object_type), sqlc.arg(canonical_object_id),
  sqlc.arg(alias_state), sqlc.arg(provenance_snapshot)::jsonb
)
returning *;

-- name: ResolveLegacyObjectAlias :one
select * from legacy_object_aliases
where project_id = sqlc.arg(project_id)
  and legacy_object_type = sqlc.arg(legacy_object_type)
  and legacy_object_id = sqlc.arg(legacy_object_id)
  and alias_state = 'active';

-- name: ListLegacyTechnicalActionsForMigration :many
select
  a.project_id,
  o.id as opportunity_id,
  a.id as action_id,
  coalesce(a.normalized_target_url, a.target_url, o.normalized_page_url, o.page_url, '')::text as target_url,
  coalesce(nullif(a.asset_type, ''), nullif(a.action_type, ''), o.type)::text as change_family,
  o.type::text as opportunity_type,
  o.evidence::jsonb as opportunity_evidence,
  o.query,
  o.recommended_action,
  a.status,
  (case when a.evidence_snapshot <> '{}'::jsonb then a.evidence_snapshot else o.evidence end)::jsonb as evidence,
  coalesce(array_agg(app.id order by app.id) filter (where app.id is not null and app.status in (
    'draft_ready','source_mapping_required','ready_for_pr','creating_pr','github_pr_open',
    'github_pr_closed','github_pr_merged','deployment_pending','verification_pending',
    'needs_follow_up','conflict','manual_apply_required'
  )), '{}'::uuid[])::uuid[] as application_ids,
  coalesce(finding.id::text, '')::text as doctor_finding_id,
  coalesce(review.review_status, '')::text as review_decision,
  coalesce(review.id::text, '')::text as review_state_id,
  review.snoozed_until as review_snoozed_until,
  coalesce(review.reviewed_by, '')::text as review_decided_by,
  review.reviewed_at as review_decided_at,
  coalesce(review.evidence_fingerprint, '')::text as review_evidence_fingerprint,
  coalesce(review.material_change_metadata, '{}'::jsonb)::jsonb as review_material_change_metadata,
  coalesce((select jsonb_agg(jsonb_build_object('id', rs.id, 'decision', rs.review_status, 'decided_by', rs.reviewed_by, 'decided_at', rs.reviewed_at, 'snoozed_until', rs.snoozed_until, 'evidence_fingerprint', rs.evidence_fingerprint, 'material_change_metadata', rs.material_change_metadata) order by rs.created_at, rs.id)
    from seo_opportunity_review_states rs where rs.project_id = a.project_id and (rs.content_action_id = a.id or rs.source_opportunity_id = o.id)), '[]'::jsonb)::jsonb as review_history,
  a.canonical_site_fix_id,
  a.legacy_migration_disposition::text as action_legacy_disposition,
  o.legacy_migration_disposition::text as opportunity_legacy_disposition,
  a.created_at as original_created_at,
  a.updated_at as original_updated_at,
  a.approved_at
  ,coalesce(a.approved_by, '')::text as approved_by
  ,a.approval_source::text as approval_source
  ,a.input_snapshot::jsonb as input_snapshot
  ,a.output_snapshot::jsonb as output_snapshot
  ,a.diff_snapshot::jsonb as diff_snapshot
  ,coalesce(latest_app.status, '')::text as application_status
  ,(latest_app.github_pr_number is not null or latest_app.github_pr_url is not null)::boolean as has_pull_request
  ,(latest_app.deployed_at is not null or latest_app.status in ('verification_pending','verified'))::boolean as deployment_observed
  ,(latest_app.verified_at is not null or latest_app.status = 'verified')::boolean as verification_passed
from content_actions a
join seo_opportunities o on o.id = a.opportunity_id and o.project_id = a.project_id
left join site_change_applications app
  on app.project_id = a.project_id and app.content_action_id = a.id
left join lateral (
  select latest.* from site_change_applications latest
  where latest.project_id = a.project_id and latest.content_action_id = a.id
  order by (latest.status in (
    'draft_ready','source_mapping_required','ready_for_pr','creating_pr','github_pr_open',
    'github_pr_closed','github_pr_merged','deployment_pending','verification_pending',
    'needs_follow_up','conflict','manual_apply_required'
  )) desc, latest.updated_at desc, latest.id limit 1
) latest_app on true
left join lateral (
  select f.id
  from seo_doctor_findings f
  where f.project_id = a.project_id
    and (f.linked_content_action_id = a.id or f.linked_opportunity_id = o.id)
    and f.status <> 'migration_rolled_back'
  order by (f.linked_content_action_id = a.id) desc, f.created_at, f.id
  limit 1
) finding on true
left join lateral (
  select rs.id, rs.review_status, rs.snoozed_until, rs.reviewed_by, rs.reviewed_at,
         rs.evidence_fingerprint, rs.material_change_metadata
  from seo_opportunity_review_states rs
  where rs.project_id = a.project_id
    and (rs.content_action_id = a.id or rs.source_opportunity_id = o.id)
  order by rs.updated_at desc, rs.id
  limit 1
) review on true
where a.project_id = sqlc.arg(project_id)
  and a.canonical_read_only = false
  and a.legacy_migration_batch_id is null
  and a.legacy_migration_disposition in ('none','rolled_back')
  and (
    lower(coalesce(a.asset_type, '')) in ('technical_fix','sitemap_update','schema_patch')
    or lower(coalesce(a.action_type, '')) in ('technical_fix','sitemap_update','schema_patch','technical seo fix task')
    or ((lower(coalesce(a.asset_type, '')) in ('internal_link_patch','metadata_rewrite') or lower(coalesce(a.action_type, '')) in ('internal_link_patch','metadata_rewrite'))
      and coalesce(a.work_type, 'fix_site_issue') = 'fix_site_issue')
    or lower(coalesce(a.output_snapshot->>'output_type', '')) = 'technical_task'
    or lower(coalesce(a.diff_snapshot->>'output_type', '')) = 'technical_task'
    or (lower(coalesce(a.output_snapshot->>'output_type', a.diff_snapshot->>'output_type', '')) = 'direct_patch'
      and coalesce(a.work_type, '') in ('','fix_site_issue')
      and case when jsonb_typeof(coalesce(a.output_snapshot->'added_propositions', a.diff_snapshot->'added_propositions', '[]'::jsonb)) = 'array'
        then jsonb_array_length(coalesce(a.output_snapshot->'added_propositions', a.diff_snapshot->'added_propositions', '[]'::jsonb)) = 0 else false end)
    or is_legacy_doctor_technical_opportunity(o.type, o.evidence)
  )
group by a.project_id, o.id, a.id, finding.id, review.id, review.review_status,
         review.snoozed_until, review.reviewed_by, review.reviewed_at,
         review.evidence_fingerprint, review.material_change_metadata,
         latest_app.status, latest_app.github_pr_number, latest_app.github_pr_url,
         latest_app.deployed_at, latest_app.verified_at
order by a.created_at, a.id;

-- name: ListLegacyTechnicalOpportunitiesWithoutActions :many
select o.*, coalesce(review.review_status, '')::text as review_decision,
       coalesce(review.id::text, '')::text as review_state_id,
       review.snoozed_until as review_snoozed_until,
       coalesce(review.reviewed_by, '')::text as review_decided_by,
       review.reviewed_at as review_decided_at,
       coalesce(review.evidence_fingerprint, '')::text as review_evidence_fingerprint,
       coalesce(review.material_change_metadata, '{}'::jsonb)::jsonb as review_material_change_metadata,
       coalesce(finding.id::text, '')::text as doctor_finding_id,
       coalesce((select jsonb_agg(jsonb_build_object('id', rs.id, 'decision', rs.review_status, 'decided_by', rs.reviewed_by, 'decided_at', rs.reviewed_at, 'snoozed_until', rs.snoozed_until, 'evidence_fingerprint', rs.evidence_fingerprint, 'material_change_metadata', rs.material_change_metadata) order by rs.created_at, rs.id)
         from seo_opportunity_review_states rs where rs.project_id = o.project_id and rs.source_opportunity_id = o.id), '[]'::jsonb)::jsonb as review_history
from seo_opportunities o
left join lateral (
  select rs.* from seo_opportunity_review_states rs
  where rs.project_id = o.project_id and rs.source_opportunity_id = o.id
  order by rs.updated_at desc, rs.id limit 1
) review on true
left join lateral (
  select f.id from seo_doctor_findings f
  where f.project_id = o.project_id and f.linked_opportunity_id = o.id
    and f.status <> 'migration_rolled_back'
  order by f.created_at, f.id limit 1
) finding on true
where o.project_id = sqlc.arg(project_id)
  and o.canonical_read_only = false and o.legacy_migration_batch_id is null
  and o.legacy_migration_disposition in ('none','rolled_back')
  and not exists (select 1 from content_actions a where a.project_id = o.project_id and a.opportunity_id = o.id)
  and is_legacy_doctor_technical_opportunity(o.type, o.evidence)
order by o.created_at, o.id;

-- name: ListActiveWorkForLegacyMigration :many
select registry.id as work_signature_id, registry.exact_signature_hash,
       registry.conflict_bucket_keys, coalesce(registry.owner, '')::text as owner,
       coalesce(registry.reserved_work_type, '')::text as work_type,
       registry.reserved_work_id as work_id, registry.active,
       (select count(*)::int from site_change_applications app
        where app.project_id = registry.project_id and app.site_fix_id = registry.reserved_work_id
          and app.status in ('draft_ready','source_mapping_required','ready_for_pr','creating_pr','github_pr_open','github_pr_closed','github_pr_merged','deployment_pending','verification_pending','needs_follow_up','conflict','manual_apply_required'))::int as active_application_count
from work_signature_registry registry
where registry.project_id = sqlc.arg(project_id)
  and registry.mode = 'enforced' and registry.active = true
order by registry.exact_signature_hash, registry.id;

-- name: ListActiveReviewMemoryForLegacyMigration :many
select memory.id, memory.exact_signature_hash_at_decision as match_signature_hash,
       memory.semantic_fingerprint_at_decision as match_semantic_fingerprint,
       memory.conflict_bucket_keys, memory.decision,
       memory.evidence_fingerprint_at_decision, memory.snoozed_until, false as via_alias
from work_review_memory memory
where memory.project_id = sqlc.arg(project_id) and memory.active = true
union all
select memory.id, alias.alias_exact_signature_hash as match_signature_hash,
       alias.alias_semantic_fingerprint as match_semantic_fingerprint,
       memory.conflict_bucket_keys, memory.decision,
       memory.evidence_fingerprint_at_decision, memory.snoozed_until, true as via_alias
from work_review_memory memory
join work_signature_aliases alias on alias.project_id = memory.project_id and alias.review_memory_id = memory.id
where memory.project_id = sqlc.arg(project_id) and memory.active = true
order by match_signature_hash, id;

-- name: ListPlannedWorkForLegacyMigration :many
select candidate.id, coalesce(candidate.exact_signature_hash, '')::text as exact_signature_hash,
       candidate.conflict_bucket_keys, candidate.suggested_owner::text as owner
from discovery_candidates candidate
join discovery_shadow_runs run on run.id = candidate.shadow_run_id and run.project_id = candidate.project_id
where candidate.project_id = sqlc.arg(project_id)
  and candidate.status in ('identity_ready','needs_arbitration_review')
  and candidate.exact_signature_hash is not null
  and not exists (select 1 from work_signature_registry registry where registry.project_id = candidate.project_id and registry.candidate_id = candidate.id)
order by candidate.exact_signature_hash, candidate.id;

-- name: CreateMigrationDoctorArtifacts :one
with locked_authority as materialized (
  select pwa.project_id
  from product_writer_authority pwa
  where pwa.project_id = sqlc.arg(project_id)
    and pwa.product = 'doctor'
    and pwa.writer_authority = 'legacy'
    and pwa.write_fenced = true
    and pwa.fence_token = sqlc.arg(fence_token)
  for update
), batch as materialized (
  select b.id
  from migration_batches b
  join locked_authority authority on authority.project_id = b.project_id
  where b.project_id = sqlc.arg(project_id) and b.id = sqlc.arg(migration_batch_id)
), migration_run as (
  insert into seo_doctor_runs (
    id, project_id, trigger, status, stage, progress_percent, message,
    input_snapshot, output_summary, created_by_user_id, started_at, finished_at
  )
  select sqlc.arg(doctor_run_id), sqlc.arg(project_id), 'migration', 'completed',
         'completed', 100, 'Legacy technical work migration',
         sqlc.arg(finding_evidence)::jsonb, '{}'::jsonb, sqlc.arg(initiated_by),
         sqlc.arg(migrated_at), sqlc.arg(migrated_at)
  from batch
  where sqlc.narg(existing_doctor_finding_id)::uuid is null
  returning id
), migration_finding as (
  insert into seo_doctor_findings (
    id, project_id, run_id, finding_key, severity, category, issue_type, status,
    affected_urls, normalized_urls, evidence, why_it_matters, fix_intent,
    developer_instructions, likely_files_or_surfaces, acceptance_tests,
    risk_level, review_required, autofix_eligible, linked_opportunity_id,
    linked_content_action_id, first_seen_at, last_seen_at, finding_kind
  )
  select sqlc.arg(doctor_finding_id), sqlc.arg(project_id), migration_run.id,
         'migration:' || sqlc.arg(source_object_id)::text, 'P2', 'legacy_migration',
         sqlc.arg(change_family), 'active', sqlc.arg(target_urls)::jsonb,
         sqlc.arg(target_urls)::jsonb, sqlc.arg(finding_evidence)::jsonb,
         'Conserved legacy technical work', sqlc.arg(change_family),
         'Review the conserved legacy technical change and its original evidence.',
         '[]'::jsonb, sqlc.arg(acceptance_tests)::jsonb, 'medium', true, false,
         sqlc.arg(legacy_opportunity_id), sqlc.arg(legacy_action_id),
         sqlc.arg(original_created_at), sqlc.arg(original_updated_at), 'broken'
  from migration_run
  returning id, finding_kind
), chosen_finding as materialized (
  select f.id, f.finding_kind
  from seo_doctor_findings f
  join locked_authority authority on authority.project_id = f.project_id
  where f.project_id = sqlc.arg(project_id)
    and f.id = sqlc.narg(existing_doctor_finding_id)::uuid
    and f.status = 'active' and f.finding_kind in ('broken','optimization')
  union all
  select id, finding_kind from migration_finding
), migration_shadow_run as (
  insert into discovery_shadow_runs (
    id, project_id, mode, status, candidate_schema_version, signature_version,
    doctor_candidates, identity_ready, started_at, finished_at
  )
  select sqlc.arg(shadow_run_id), sqlc.arg(project_id), 'migration', 'completed',
         'discovery-candidate/v1', 'work-signature/v1', 1, 1,
         sqlc.arg(migrated_at), sqlc.arg(migrated_at)
  from chosen_finding
  returning id
), candidate as (
  insert into discovery_candidates (
    id, project_id, shadow_run_id, source_kind, source_object_type, source_object_id,
    target_kind, normalized_target_set, issue_or_hypothesis_family, change_family,
    proposed_mutations, artifact_intent, topic_entity_identity, audience_identity,
    primary_success_metric, verification_mode, evidence_ids, evidence_fingerprint,
    suggested_owner, confidence, candidate_schema_version, status,
    exact_signature_hash, signature_payload, conflict_bucket_keys, candidate_version
  )
  select sqlc.arg(candidate_id), sqlc.arg(project_id), migration_shadow_run.id,
         'migration', 'seo_doctor_finding', chosen_finding.id::text, 'page',
         sqlc.arg(target_urls)::jsonb, sqlc.arg(change_family), sqlc.arg(change_family),
         sqlc.arg(proposed_mutations)::jsonb, 'repair_existing_surface', '[]'::jsonb,
         '[]'::jsonb, 'acceptance_test_pass', 'immediate',
         jsonb_build_array(sqlc.arg(source_object_type) || ':' || sqlc.arg(source_object_id)::text),
         sqlc.arg(evidence_fingerprint), 'doctor', 1.0, 'discovery-candidate/v1',
         'identity_ready', sqlc.arg(exact_signature_hash), sqlc.arg(signature_payload)::jsonb,
         sqlc.arg(conflict_bucket_keys)::jsonb, 1
  from migration_shadow_run cross join chosen_finding
  returning id, shadow_run_id
), bumped_buckets as (
  insert into work_conflict_buckets (project_id, bucket_key, bucket_version)
  select sqlc.arg(project_id), keys.bucket_key, 1
  from jsonb_array_elements_text(sqlc.arg(conflict_bucket_keys)::jsonb) keys(bucket_key)
  on conflict (project_id, bucket_key) do update
    set bucket_version = work_conflict_buckets.bucket_version + 1, updated_at = now()
  returning id
), signature as (
  insert into work_signature_registry (
    id, project_id, candidate_id, shadow_run_id, mode, status, active,
    exact_signature_hash, signature_payload, conflict_bucket_keys, signature_version,
    owner, source_object_type, source_object_id, reserved_work_type, reserved_work_id,
    evidence_fingerprint
  )
  select sqlc.arg(work_signature_id), sqlc.arg(project_id), candidate.id,
         candidate.shadow_run_id, 'enforced', sqlc.arg(registry_status), sqlc.arg(registry_active),
         sqlc.arg(exact_signature_hash), sqlc.arg(signature_payload)::jsonb,
         sqlc.arg(conflict_bucket_keys)::jsonb, 'work-signature/v1', 'doctor',
         'site_fix', sqlc.arg(site_fix_id)::text, 'site_fix', sqlc.arg(site_fix_id),
         sqlc.arg(evidence_fingerprint)
  from candidate
  where (select count(*) from bumped_buckets) =
        jsonb_array_length(sqlc.arg(conflict_bucket_keys)::jsonb)
  returning id, candidate_id
), fix as (
  insert into site_fixes (
    id, project_id, doctor_finding_id, candidate_id, work_signature_id, status,
    finding_kind, target_urls, evidence_snapshot, proposed_fix, acceptance_tests,
    verification_snapshot, retry_count, max_retries, legacy_opportunity_id,
    legacy_content_action_id, migration_batch_id, approved_at, created_at, updated_at
  )
  select sqlc.arg(site_fix_id), sqlc.arg(project_id), chosen_finding.id,
         signature.candidate_id, signature.id, sqlc.arg(site_fix_status),
         chosen_finding.finding_kind, sqlc.arg(target_urls)::jsonb, sqlc.arg(finding_evidence)::jsonb,
         sqlc.arg(proposed_fix)::jsonb, sqlc.arg(acceptance_tests)::jsonb,
         '{}'::jsonb, 0, 3, sqlc.arg(legacy_opportunity_id), sqlc.arg(legacy_action_id),
         batch.id, sqlc.narg(approved_at), sqlc.arg(original_created_at),
         sqlc.arg(original_updated_at)
  from signature cross join chosen_finding cross join batch
  returning *
), archived_action as (
  update content_actions a
  set canonical_site_fix_id = fix.id, canonical_read_only = true,
      legacy_migration_batch_id = sqlc.arg(migration_batch_id),
      legacy_migration_disposition = 'migrated', updated_at = now()
  from fix
  where a.project_id = sqlc.arg(project_id) and a.id = sqlc.arg(legacy_action_id)
    and a.canonical_read_only = false and a.canonical_site_fix_id is null
  returning a.id
), archived_opportunity as (
  update seo_opportunities o
  set canonical_site_fix_id = fix.id, canonical_read_only = true,
      legacy_migration_batch_id = sqlc.arg(migration_batch_id),
      legacy_migration_disposition = 'migrated', updated_at = now()
  from fix
  where o.project_id = sqlc.arg(project_id) and o.id = sqlc.arg(legacy_opportunity_id)
    and o.canonical_read_only = false
  returning o.id
)
select fix.* from fix
where ((select count(*) from archived_action) = 1
   or (sqlc.narg(legacy_action_id)::uuid is null and (select count(*) from archived_action) = 0))
  and (select count(*) from archived_opportunity) = 1;

-- name: ListMigrationBucketsForUpdate :many
select bucket.*
from work_conflict_buckets bucket
where bucket.project_id = sqlc.arg(project_id)
  and bucket.bucket_key in (
    select jsonb_array_elements_text(sqlc.arg(conflict_bucket_keys)::jsonb)
  )
order by bucket.bucket_key
for update;

-- name: RepointLegacyApplicationsToCanonicalSiteFix :many
with locked_fix as materialized (
  select sf.id, sf.project_id, sf.work_signature_id
  from site_fixes sf
  join work_signature_registry w on w.id = sf.work_signature_id and w.project_id = sf.project_id
  join product_writer_authority pwa
    on pwa.project_id = sf.project_id and pwa.product = 'doctor'
  where sf.project_id = sqlc.arg(project_id) and sf.id = sqlc.arg(site_fix_id)
    and (sf.migration_batch_id = sqlc.arg(migration_batch_id)
      or (w.mode = 'enforced' and w.owner = 'doctor' and w.reserved_work_type = 'site_fix'
        and w.reserved_work_id = sf.id and w.active))
    and pwa.writer_authority = 'legacy' and pwa.write_fenced = true
    and pwa.fence_token = sqlc.arg(fence_token)
  for update of sf
), expected_keys as materialized (
  select keys.bucket_key
  from locked_fix f
  join work_signature_registry w on w.id = f.work_signature_id and w.project_id = f.project_id
  cross join lateral jsonb_array_elements_text(w.conflict_bucket_keys) keys(bucket_key)
), locked_buckets as materialized (
  select b.id
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key for update
), eligible_apps as materialized (
  select a.id from site_change_applications a
  where a.project_id = sqlc.arg(project_id)
    and a.content_action_id = sqlc.arg(legacy_action_id)
    and a.site_fix_id is null
    and num_nonnulls(a.content_action_id, a.site_fix_id) = 1
), bumped_buckets as (
  update work_conflict_buckets bucket
  set bucket_version = bucket.bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where bucket.id = locked.id and exists (select 1 from eligible_apps)
  returning bucket.id
)
update site_change_applications a
set content_action_id = null, site_fix_id = sqlc.arg(site_fix_id),
    application_kind = 'site_fix', updated_at = now()
where a.project_id = sqlc.arg(project_id)
  and a.id in (select id from eligible_apps)
  and (select count(*) from bumped_buckets) = (select count(*) from expected_keys)
returning a.*;

-- name: ListLegacyApplicationsForMigrationUpdate :many
select * from site_change_applications
where project_id = sqlc.arg(project_id)
  and content_action_id = sqlc.arg(legacy_action_id)
  and site_fix_id is null
order by id
for update;

-- name: MarkLegacyDuplicateCanonicalReadOnly :one
with marked_action as (
  update content_actions a
  set canonical_site_fix_id = sqlc.arg(site_fix_id), canonical_read_only = true,
      legacy_migration_batch_id = sqlc.arg(migration_batch_id),
      legacy_migration_disposition = 'duplicate', updated_at = now()
  where a.project_id = sqlc.arg(project_id) and a.id = sqlc.arg(legacy_action_id)
    and a.canonical_read_only = false and a.canonical_site_fix_id is null
  returning a.*
), marked_opportunity as (
  update seo_opportunities o
  set canonical_site_fix_id = sqlc.arg(site_fix_id), canonical_read_only = true,
      legacy_migration_batch_id = sqlc.arg(migration_batch_id),
      legacy_migration_disposition = 'duplicate', updated_at = now()
  from marked_action a
  where o.project_id = a.project_id and o.id = a.opportunity_id
    and o.canonical_read_only = false
  returning o.id
)
select marked_action.* from marked_action;

-- name: MarkLegacyMigrationReviewReadOnly :one
with marked_action as (
  update content_actions a
  set canonical_read_only = true, legacy_migration_batch_id = sqlc.arg(migration_batch_id),
      legacy_migration_disposition = 'review', updated_at = now()
  where a.project_id = sqlc.arg(project_id) and a.id = sqlc.arg(legacy_action_id)
    and a.canonical_read_only = false and a.canonical_site_fix_id is null
  returning a.*
), marked_opportunity as (
  update seo_opportunities o
  set canonical_read_only = true, legacy_migration_batch_id = sqlc.arg(migration_batch_id),
      legacy_migration_disposition = 'review', updated_at = now()
  from marked_action a
  where o.project_id = a.project_id and o.id = a.opportunity_id
    and o.canonical_read_only = false
  returning o.id
)
select marked_action.* from marked_action;

-- name: MarkLegacyOpportunityDuplicateReadOnly :one
update seo_opportunities opportunity
set canonical_site_fix_id = sqlc.arg(site_fix_id), canonical_read_only = true,
    legacy_migration_batch_id = sqlc.arg(migration_batch_id),
    legacy_migration_disposition = 'duplicate', updated_at = now()
where opportunity.project_id = sqlc.arg(project_id)
  and opportunity.id = sqlc.arg(legacy_opportunity_id)
  and opportunity.canonical_read_only = false
returning *;

-- name: MarkLegacyOpportunityMigrationReviewReadOnly :one
update seo_opportunities opportunity
set canonical_read_only = true, legacy_migration_batch_id = sqlc.arg(migration_batch_id),
    legacy_migration_disposition = 'review', updated_at = now()
where opportunity.project_id = sqlc.arg(project_id)
  and opportunity.id = sqlc.arg(legacy_opportunity_id)
  and opportunity.canonical_read_only = false
returning *;

-- name: CreateMigrationWorkReviewMemory :one
insert into work_review_memory (
  id, project_id, candidate_id, work_signature_id,
  exact_signature_hash_at_decision, semantic_fingerprint_at_decision,
  signature_payload, conflict_bucket_keys, signature_version, decision,
  decision_scope, evidence_fingerprint_at_decision, snoozed_until,
  material_change_policy_version, decided_by, decided_at, active,
  migration_batch_id, legacy_review_state_id
) values (
  sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(candidate_id), sqlc.arg(work_signature_id),
  sqlc.arg(exact_signature_hash_at_decision), sqlc.arg(semantic_fingerprint_at_decision),
  sqlc.arg(signature_payload)::jsonb, sqlc.arg(conflict_bucket_keys)::jsonb,
  'work-signature/v1', sqlc.arg(decision), sqlc.arg(decision_scope)::jsonb,
  sqlc.arg(evidence_fingerprint_at_decision), sqlc.narg(snoozed_until),
  'legacy-review-memory/v1', sqlc.arg(decided_by), sqlc.arg(decided_at), true,
  sqlc.arg(migration_batch_id), sqlc.arg(legacy_review_state_id)
)
on conflict (project_id, exact_signature_hash_at_decision) where active = true do nothing
returning *;

-- name: GetActiveWorkReviewMemoryByExactSignature :one
select * from work_review_memory
where project_id = sqlc.arg(project_id)
  and exact_signature_hash_at_decision = sqlc.arg(exact_signature_hash_at_decision)
  and active = true;

-- name: TombstoneLegacyObjectAliasesForBatch :execrows
update legacy_object_aliases
set alias_state = 'rolled_back_tombstone', updated_at = now()
where project_id = sqlc.arg(project_id)
  and migration_batch_id = sqlc.arg(migration_batch_id)
  and alias_state = 'active';

-- name: GetMigrationBatch :one
select * from migration_batches
where project_id = sqlc.arg(project_id) and id = sqlc.arg(migration_batch_id);

-- name: ListMigrationLedgerForBatch :many
select * from migration_ledger
where project_id = sqlc.arg(project_id) and migration_batch_id = sqlc.arg(migration_batch_id)
order by sequence_number;

-- name: LockMigrationBucketsForBatch :many
select bucket.*
from work_conflict_buckets bucket
join migration_ledger ledger
  on ledger.project_id = bucket.project_id
 and ledger.canonical_object_type = 'work_conflict_bucket'
 and ledger.canonical_object_id = bucket.id
where ledger.project_id = sqlc.arg(project_id)
  and ledger.migration_batch_id = sqlc.arg(migration_batch_id)
order by bucket.bucket_key
for update of bucket;

-- name: ListMigrationReviewItemsForBatch :many
select * from migration_review_items
where project_id = sqlc.arg(project_id) and migration_batch_id = sqlc.arg(migration_batch_id)
order by created_at, id;

-- name: ListMigrationRollbackEventsForBatch :many
select * from migration_rollback_events
where project_id = sqlc.arg(project_id) and migration_batch_id = sqlc.arg(migration_batch_id)
order by event_sequence;

-- name: GetMigrationCurrentSnapshot :one
select coalesce(case sqlc.arg(object_type)::text
  when 'migration_batch' then (select to_jsonb(b) from migration_batches b where b.project_id = sqlc.arg(project_id) and b.id = sqlc.arg(object_id))
  when 'seo_doctor_run' then (select to_jsonb(r) - 'updated_at' from seo_doctor_runs r where r.project_id = sqlc.arg(project_id) and r.id = sqlc.arg(object_id))
  when 'seo_doctor_finding' then (select to_jsonb(f) - 'updated_at' - 'last_seen_at' from seo_doctor_findings f where f.project_id = sqlc.arg(project_id) and f.id = sqlc.arg(object_id))
  when 'discovery_shadow_run' then (select to_jsonb(r) - 'updated_at' from discovery_shadow_runs r where r.project_id = sqlc.arg(project_id) and r.id = sqlc.arg(object_id))
  when 'discovery_candidate' then (select to_jsonb(c) - 'updated_at' from discovery_candidates c where c.project_id = sqlc.arg(project_id) and c.id = sqlc.arg(object_id))
  when 'work_conflict_bucket' then (select to_jsonb(b) - 'updated_at' from work_conflict_buckets b where b.project_id = sqlc.arg(project_id) and b.id = sqlc.arg(object_id))
  when 'work_signature_registry' then (select to_jsonb(w) - 'updated_at' from work_signature_registry w where w.project_id = sqlc.arg(project_id) and w.id = sqlc.arg(object_id))
  when 'site_fix' then (select to_jsonb(f) - 'updated_at' from site_fixes f where f.project_id = sqlc.arg(project_id) and f.id = sqlc.arg(object_id))
  when 'content_action' then (select to_jsonb(a) - 'updated_at' from content_actions a where a.project_id = sqlc.arg(project_id) and a.id = sqlc.arg(object_id))
  when 'seo_opportunity' then (select to_jsonb(o) - 'updated_at' from seo_opportunities o where o.project_id = sqlc.arg(project_id) and o.id = sqlc.arg(object_id))
  when 'site_change_application' then (select to_jsonb(a) - 'updated_at' from site_change_applications a where a.project_id = sqlc.arg(project_id) and a.id = sqlc.arg(object_id))
  when 'legacy_object_alias' then (select to_jsonb(a) - 'updated_at' from legacy_object_aliases a where a.project_id = sqlc.arg(project_id) and a.id = sqlc.arg(object_id))
  when 'migration_review_item' then (select to_jsonb(i) - 'updated_at' from migration_review_items i where i.project_id = sqlc.arg(project_id) and i.id = sqlc.arg(object_id))
  when 'work_review_memory' then (select to_jsonb(m) - 'updated_at' from work_review_memory m where m.project_id = sqlc.arg(project_id) and m.id = sqlc.arg(object_id))
  when 'product_writer_authority' then (select jsonb_build_object('project_id', p.project_id, 'product', p.product, 'writer_authority', p.writer_authority) from product_writer_authority p where p.project_id = sqlc.arg(project_id) and p.product = 'doctor')
end, '{"missing":true}'::jsonb)::jsonb as snapshot;

-- name: GetNextMigrationRollbackEventSequence :one
select (coalesce(max(event_sequence), 0) + 1)::int
from migration_rollback_events
where project_id = sqlc.arg(project_id) and migration_batch_id = sqlc.arg(migration_batch_id);

-- name: CountMigrationRollbackRelationBlockers :one
with batch as materialized (
  select migration_batch.* from migration_batches migration_batch
  where migration_batch.project_id = sqlc.arg(project_id) and migration_batch.id = sqlc.arg(migration_batch_id)
), migrated_fixes as materialized (
  select migrated.id, migrated.work_signature_id from site_fixes migrated
  where migrated.project_id = sqlc.arg(project_id) and migrated.migration_batch_id = sqlc.arg(migration_batch_id)
), blockers as (
  select revision.id from site_fixes revision join migrated_fixes original on revision.supersedes_site_fix_id = original.id
  where revision.migration_batch_id is distinct from sqlc.arg(migration_batch_id)
  union all
  select verification.id from site_fix_verifications verification join migrated_fixes fix on fix.id = verification.site_fix_id
  union all
  select application.id from site_change_applications application join migrated_fixes fix on fix.id = application.site_fix_id
  where not exists (select 1 from migration_ledger ledger where ledger.project_id = application.project_id and ledger.migration_batch_id = sqlc.arg(migration_batch_id) and ledger.source_object_type = 'site_change_application' and ledger.source_object_id = application.id and ledger.operation = 'repoint')
  union all
  select memory.id from work_review_memory memory join migrated_fixes fix on fix.work_signature_id = memory.work_signature_id
  where memory.migration_batch_id is distinct from sqlc.arg(migration_batch_id)
  union all
  select call.id from ai_call_records call join migrated_fixes fix on call.linked_object_type = 'site_fix' and call.linked_object_id = fix.id
  union all
  select rollback.id from rollback_records rollback join migrated_fixes fix on rollback.site_fix_id = fix.id
  union all
  select alias.id from work_signature_aliases alias
  join work_signature_registry signature on signature.project_id = alias.project_id and signature.exact_signature_hash = alias.alias_exact_signature_hash
  join migrated_fixes fix on fix.work_signature_id = signature.id
  cross join batch where alias.created_at > batch.finished_at
  union all
  select decision.id from discovery_arbitration_decisions decision
  join site_fixes fix on fix.candidate_id = decision.candidate_id and fix.project_id = decision.project_id
  join migrated_fixes migrated on migrated.id = fix.id
  cross join batch where decision.created_at > batch.finished_at
  union all
  select review.id from discovery_review_items review
  join site_fixes fix on fix.candidate_id = review.candidate_id and fix.project_id = review.project_id
  join migrated_fixes migrated on migrated.id = fix.id
  cross join batch where review.created_at > batch.finished_at
  union all
  select fix.id from site_fixes fix cross join batch
  where fix.project_id = sqlc.arg(project_id) and fix.created_at > batch.finished_at
    and fix.migration_batch_id is distinct from sqlc.arg(migration_batch_id)
  union all
  select application.id from site_change_applications application cross join batch
  where application.project_id = sqlc.arg(project_id) and application.site_fix_id is not null
    and application.created_at > batch.finished_at
    and not exists (select 1 from migration_ledger ledger where ledger.project_id = application.project_id and ledger.migration_batch_id = sqlc.arg(migration_batch_id) and ledger.source_object_type = 'site_change_application' and ledger.source_object_id = application.id)
)
select count(*)::int from blockers;

-- name: GetMigrationConservation :one
with batch_fixes as materialized (
  select * from site_fixes where project_id = sqlc.arg(project_id) and migration_batch_id = sqlc.arg(migration_batch_id)
), unledgered as (
  select fix.id from batch_fixes fix where not exists (select 1 from migration_ledger ledger where ledger.project_id=sqlc.arg(project_id) and ledger.migration_batch_id=sqlc.arg(migration_batch_id) and ledger.canonical_object_type='site_fix' and ledger.canonical_object_id=fix.id)
  union all select alias.id from legacy_object_aliases alias where alias.project_id=sqlc.arg(project_id) and alias.migration_batch_id=sqlc.arg(migration_batch_id) and not exists (select 1 from migration_ledger ledger where ledger.project_id=alias.project_id and ledger.migration_batch_id=alias.migration_batch_id and ledger.canonical_object_type='legacy_object_alias' and ledger.canonical_object_id=alias.id)
  union all select item.id from migration_review_items item where item.project_id=sqlc.arg(project_id) and item.migration_batch_id=sqlc.arg(migration_batch_id) and not exists (select 1 from migration_ledger ledger where ledger.project_id=item.project_id and ledger.migration_batch_id=item.migration_batch_id and ledger.canonical_object_type='migration_review_item' and ledger.canonical_object_id=item.id)
  union all select memory.id from work_review_memory memory where memory.project_id=sqlc.arg(project_id) and memory.migration_batch_id=sqlc.arg(migration_batch_id) and not exists (select 1 from migration_ledger ledger where ledger.project_id=memory.project_id and ledger.migration_batch_id=memory.migration_batch_id and ledger.canonical_object_type='work_review_memory' and ledger.canonical_object_id=memory.id)
  union all select action.id from content_actions action where action.project_id=sqlc.arg(project_id) and action.legacy_migration_batch_id=sqlc.arg(migration_batch_id) and not exists (select 1 from migration_ledger ledger where ledger.project_id=action.project_id and ledger.migration_batch_id=sqlc.arg(migration_batch_id) and ledger.canonical_object_type='content_action' and ledger.canonical_object_id=action.id)
  union all select opportunity.id from seo_opportunities opportunity where opportunity.project_id=sqlc.arg(project_id) and opportunity.legacy_migration_batch_id=sqlc.arg(migration_batch_id) and not exists (select 1 from migration_ledger ledger where ledger.project_id=opportunity.project_id and ledger.migration_batch_id=sqlc.arg(migration_batch_id) and ledger.canonical_object_type='seo_opportunity' and ledger.canonical_object_id=opportunity.id)
  union all select fix.candidate_id from batch_fixes fix where not exists (select 1 from migration_ledger ledger where ledger.project_id=fix.project_id and ledger.migration_batch_id=sqlc.arg(migration_batch_id) and ledger.canonical_object_type='discovery_candidate' and ledger.canonical_object_id=fix.candidate_id)
  union all select fix.work_signature_id from batch_fixes fix where not exists (select 1 from migration_ledger ledger where ledger.project_id=fix.project_id and ledger.migration_batch_id=sqlc.arg(migration_batch_id) and ledger.canonical_object_type='work_signature_registry' and ledger.canonical_object_id=fix.work_signature_id)
  union all select candidate.shadow_run_id from batch_fixes fix join discovery_candidates candidate on candidate.id=fix.candidate_id and candidate.project_id=fix.project_id where not exists (select 1 from migration_ledger ledger where ledger.project_id=fix.project_id and ledger.migration_batch_id=sqlc.arg(migration_batch_id) and ledger.canonical_object_type='discovery_shadow_run' and ledger.canonical_object_id=candidate.shadow_run_id)
  union all select finding.id from batch_fixes fix join seo_doctor_findings finding on finding.id=fix.doctor_finding_id and finding.project_id=fix.project_id join seo_doctor_runs run on run.id=finding.run_id and run.project_id=finding.project_id where run.trigger='migration' and not exists (select 1 from migration_ledger ledger where ledger.project_id=fix.project_id and ledger.migration_batch_id=sqlc.arg(migration_batch_id) and ledger.canonical_object_type='seo_doctor_finding' and ledger.canonical_object_id=finding.id)
  union all select run.id from batch_fixes fix join seo_doctor_findings finding on finding.id=fix.doctor_finding_id and finding.project_id=fix.project_id join seo_doctor_runs run on run.id=finding.run_id and run.project_id=finding.project_id where run.trigger='migration' and not exists (select 1 from migration_ledger ledger where ledger.project_id=fix.project_id and ledger.migration_batch_id=sqlc.arg(migration_batch_id) and ledger.canonical_object_type='seo_doctor_run' and ledger.canonical_object_id=run.id)
  union all select bucket.id from batch_fixes fix join work_signature_registry signature on signature.id=fix.work_signature_id and signature.project_id=fix.project_id cross join lateral jsonb_array_elements_text(signature.conflict_bucket_keys) keys(bucket_key) join work_conflict_buckets bucket on bucket.project_id=fix.project_id and bucket.bucket_key=keys.bucket_key where not exists (select 1 from migration_ledger ledger where ledger.project_id=fix.project_id and ledger.migration_batch_id=sqlc.arg(migration_batch_id) and ledger.canonical_object_type='work_conflict_bucket' and ledger.canonical_object_id=bucket.id)
)
select
  (select count(*)::int from legacy_object_aliases alias_count where alias_count.project_id=sqlc.arg(project_id) and alias_count.migration_batch_id=sqlc.arg(migration_batch_id) and alias_count.alias_state='active') as active_alias_count,
  (select count(*)::int from migration_ledger application_ledger where application_ledger.project_id=sqlc.arg(project_id) and application_ledger.migration_batch_id=sqlc.arg(migration_batch_id) and application_ledger.source_object_type='site_change_application' and application_ledger.operation='repoint') as repointed_application_count,
  (select count(*)::int from migration_ledger source_ledger where source_ledger.project_id=sqlc.arg(project_id) and source_ledger.migration_batch_id=sqlc.arg(migration_batch_id) and source_ledger.canonical_object_type in ('content_action','seo_opportunity')) as source_ledger_count,
  (select count(*)::int from batch_fixes) as site_fix_count,
  (select count(*)::int from migration_review_items review_count where review_count.project_id=sqlc.arg(project_id) and review_count.migration_batch_id=sqlc.arg(migration_batch_id)) as review_item_count,
  (select count(*)::int from work_review_memory memory_count where memory_count.project_id=sqlc.arg(project_id) and memory_count.migration_batch_id=sqlc.arg(migration_batch_id)) as review_memory_count,
  (select count(*)::int from site_change_applications invalid_application where invalid_application.project_id=sqlc.arg(project_id) and num_nonnulls(invalid_application.content_action_id,invalid_application.site_fix_id) <> 1) as invalid_application_source_count,
  (select count(*)::int from unledgered) as unledgered_object_count;

-- name: RestoreLegacyContentActionFromLedger :one
update content_actions
set canonical_site_fix_id = null, canonical_read_only = false,
    legacy_migration_batch_id = null,
    legacy_migration_disposition = coalesce(nullif(sqlc.arg(legacy_disposition)::text, ''), 'rolled_back'),
    updated_at = now()
where project_id = sqlc.arg(project_id) and id = sqlc.arg(object_id)
  and legacy_migration_batch_id = sqlc.arg(migration_batch_id)
returning *;

-- name: RestoreLegacyOpportunityFromLedger :one
update seo_opportunities
set canonical_site_fix_id = null, canonical_read_only = false,
    legacy_migration_batch_id = null,
    legacy_migration_disposition = coalesce(nullif(sqlc.arg(legacy_disposition)::text, ''), 'rolled_back'),
    updated_at = now()
where project_id = sqlc.arg(project_id) and id = sqlc.arg(object_id)
  and legacy_migration_batch_id = sqlc.arg(migration_batch_id)
returning *;

-- name: RestoreLegacyApplicationFromLedger :one
update site_change_applications
set content_action_id = sqlc.arg(content_action_id), site_fix_id = null,
    application_kind = sqlc.arg(application_kind), updated_at = now()
where project_id = sqlc.arg(project_id) and id = sqlc.arg(object_id)
  and site_fix_id = sqlc.arg(site_fix_id)
returning *;

-- name: TombstoneLegacyMigrationAlias :one
update legacy_object_aliases
set alias_state = 'rolled_back_tombstone', updated_at = now()
where project_id = sqlc.arg(project_id) and id = sqlc.arg(object_id)
  and migration_batch_id = sqlc.arg(migration_batch_id) and alias_state = 'active'
returning *;

-- name: TombstoneMigrationSiteFix :one
update site_fixes set status = 'migration_rolled_back', updated_at = now()
where project_id = sqlc.arg(project_id) and id = sqlc.arg(object_id)
  and migration_batch_id = sqlc.arg(migration_batch_id)
returning *;

-- name: TombstoneMigrationFinding :one
update seo_doctor_findings set status = 'migration_rolled_back', updated_at = now()
where project_id = sqlc.arg(project_id) and id = sqlc.arg(object_id)
returning *;

-- name: DeactivateMigrationSignature :one
update work_signature_registry set active = false, status = 'cancelled', updated_at = now()
where project_id = sqlc.arg(project_id) and id = sqlc.arg(object_id)
returning *;

-- name: RestoreMigrationBucketVersion :one
update work_conflict_buckets set bucket_version = sqlc.arg(bucket_version), updated_at = now()
where project_id = sqlc.arg(project_id) and id = sqlc.arg(object_id)
returning *;

-- name: DeleteCreatedMigrationBucket :execrows
delete from work_conflict_buckets
where project_id = sqlc.arg(project_id) and id = sqlc.arg(object_id)
  and bucket_version = sqlc.arg(expected_bucket_version);

-- name: DeactivateMigrationReviewMemory :one
update work_review_memory set active = false, updated_at = now()
where project_id = sqlc.arg(project_id) and id = sqlc.arg(object_id)
  and migration_batch_id = sqlc.arg(migration_batch_id) and active
returning *;

-- name: DismissMigrationReviewItem :one
update migration_review_items
set status = 'dismissed', resolution_snapshot = sqlc.arg(resolution_snapshot)::jsonb,
    resolved_by = sqlc.arg(resolved_by), resolved_at = sqlc.arg(resolved_at), updated_at = now()
where project_id = sqlc.arg(project_id) and id = sqlc.arg(object_id) and status = 'pending'
returning *;

-- name: RollbackLegacySiteFixMigration :one
with authority as materialized (
  select pwa.*
  from product_writer_authority pwa
  where pwa.project_id = sqlc.arg(project_id) and pwa.product = 'doctor'
    and pwa.writer_authority = 'canonical' and pwa.write_fenced = true
    and pwa.fence_token = sqlc.arg(fence_token)
  for update
), migrated_fixes as materialized (
  select sf.* from site_fixes sf join authority on authority.project_id = sf.project_id
  where sf.project_id = sqlc.arg(project_id)
    and sf.migration_batch_id = sqlc.arg(migration_batch_id)
  for update of sf
), eligibility as materialized (
  select not exists (
    select 1 from site_fixes revision join migrated_fixes original
      on revision.supersedes_site_fix_id = original.id
    where revision.migration_batch_id is distinct from sqlc.arg(migration_batch_id)
  ) and not exists (
    select 1 from site_change_applications app join migrated_fixes fix on fix.id = app.site_fix_id
    where not exists (
      select 1 from migration_ledger ledger
      where ledger.project_id = app.project_id
        and ledger.migration_batch_id = sqlc.arg(migration_batch_id)
        and ledger.source_object_type = 'site_change_application'
        and ledger.source_object_id = app.id and ledger.operation = 'repoint'
    )
  ) and not exists (
    select 1 from work_review_memory memory join migrated_fixes fix
      on fix.work_signature_id = memory.work_signature_id
    where memory.migration_batch_id is distinct from sqlc.arg(migration_batch_id)
      and memory.created_at > fix.created_at
  ) as eligible
), restored_applications as (
  update site_change_applications app
  set content_action_id = (ledger.before_snapshot->>'content_action_id')::uuid,
      site_fix_id = null, application_kind = 'site_fix', updated_at = now()
  from migration_ledger ledger, eligibility
  where eligibility.eligible
    and ledger.project_id = sqlc.arg(project_id)
    and ledger.migration_batch_id = sqlc.arg(migration_batch_id)
    and ledger.source_object_type = 'site_change_application'
    and ledger.operation = 'repoint' and app.id = ledger.source_object_id
    and app.site_fix_id = ledger.canonical_object_id
    and num_nonnulls(app.content_action_id, app.site_fix_id) = 1
  returning app.id
), restored_actions as (
  update content_actions action
  set canonical_site_fix_id = null, canonical_read_only = false,
      legacy_migration_batch_id = null, legacy_migration_disposition = 'rolled_back', updated_at = now()
  from eligibility
  where eligibility.eligible and action.project_id = sqlc.arg(project_id)
    and action.legacy_migration_batch_id = sqlc.arg(migration_batch_id)
  returning action.id
), restored_opportunities as (
  update seo_opportunities opportunity
  set canonical_site_fix_id = null, canonical_read_only = false,
      legacy_migration_batch_id = null, legacy_migration_disposition = 'rolled_back', updated_at = now()
  from eligibility
  where eligibility.eligible and opportunity.project_id = sqlc.arg(project_id)
    and opportunity.legacy_migration_batch_id = sqlc.arg(migration_batch_id)
  returning opportunity.id
), tombstoned_fixes as (
  update site_fixes fix
  set status = 'migration_rolled_back', updated_at = now()
  from eligibility
  where eligibility.eligible and fix.id in (select id from migrated_fixes)
  returning fix.id, fix.work_signature_id, fix.doctor_finding_id
), tombstoned_signatures as (
  update work_signature_registry registry
  set active = false, status = 'cancelled', updated_at = now()
  from tombstoned_fixes fix
  where registry.id = fix.work_signature_id
  returning registry.id, registry.conflict_bucket_keys
), bumped_buckets as (
  update work_conflict_buckets bucket
  set bucket_version = bucket.bucket_version + 1, updated_at = now()
  where bucket.project_id = sqlc.arg(project_id)
    and bucket.bucket_key in (
      select distinct keys.bucket_key from tombstoned_signatures signature
      cross join lateral jsonb_array_elements_text(signature.conflict_bucket_keys) keys(bucket_key)
    )
  returning bucket.id
), tombstoned_findings as (
  update seo_doctor_findings finding
  set status = 'migration_rolled_back', updated_at = now()
  from eligibility
  where eligibility.eligible and finding.id in (
    select ledger.canonical_object_id from migration_ledger ledger
    where ledger.project_id = sqlc.arg(project_id)
      and ledger.migration_batch_id = sqlc.arg(migration_batch_id)
      and ledger.canonical_object_type = 'seo_doctor_finding' and ledger.operation = 'create'
  )
  returning finding.id
), disabled_memory as (
  update work_review_memory memory set active = false, updated_at = now()
  from eligibility
  where eligibility.eligible and memory.project_id = sqlc.arg(project_id)
    and memory.migration_batch_id = sqlc.arg(migration_batch_id) and memory.active
  returning memory.id
), switched as (
  update product_writer_authority pwa
  set writer_authority = 'legacy', authority_changed_at = sqlc.arg(rolled_back_at), updated_at = now()
  from eligibility
  where eligibility.eligible and pwa.project_id = sqlc.arg(project_id)
    and pwa.product = 'doctor' and pwa.writer_authority = 'canonical'
    and pwa.write_fenced = true and pwa.fence_token = sqlc.arg(fence_token)
  returning pwa.writer_authority
)
select eligibility.eligible,
       coalesce((select count(*) from tombstoned_fixes), 0)::int as tombstoned_fixes,
       coalesce((select count(*) from restored_applications), 0)::int as restored_applications,
       coalesce((select writer_authority from switched), 'canonical')::text as writer_authority
from eligibility;

-- name: ApproveCanonicalSiteFix :one
with authority as materialized (
  select product_writer_authority.project_id
  from product_writer_authority
  where product_writer_authority.project_id = sqlc.arg(project_id)
    and product = 'doctor'
    and writer_authority = 'canonical'
    and write_fenced = false
  for update
), eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         null::uuid as application_id,
         null::text as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  join authority a on a.project_id = sf.project_id
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and sf.status = 'proposed'
    and w.status in ('reserved','proposed')
    and w.mode = 'enforced' and w.active = true
), expected_keys as materialized (
  select distinct keys.bucket_key
  from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id
  from site_change_applications a
  join eligible e on e.application_id = a.id
  where a.project_id = e.project_id
    and a.site_fix_id = e.id
    and a.content_action_id is null
    and a.status = e.expected_application_status
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of a
), locked_work as materialized (
  select e.*
  from eligible e
  join site_fixes sf
    on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w
    on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status
    and w.status = e.expected_signature_status
    and w.active = e.expected_signature_active
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = e.conflict_bucket_keys
    and jsonb_array_length(e.conflict_bucket_keys) > 0
    and (
      e.application_id is null
      or exists (
        select 1 from locked_application a
        where a.id = e.application_id
      )
    )
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
), transitioned as (
  update site_fixes sf
  set status = 'approved', approved_at = sqlc.arg(approved_at), updated_at = now()
  from locked_work e
  where sf.id = e.id
    and sf.status = 'proposed'
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning sf.*
), signature_transition as (
  update work_signature_registry w
  set status = 'approved', active = true, updated_at = now()
  from transitioned sf
  where w.id = sf.work_signature_id and w.project_id = sf.project_id
  returning w.id
)
select transitioned.* from transitioned
cross join signature_transition;

-- name: MarkCanonicalSiteFixVerified :one
with eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         a.id as application_id,
         a.status as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  join site_change_applications a
    on a.site_fix_id = sf.id and a.project_id = sf.project_id
   and a.content_action_id is null
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and a.id = sqlc.arg(application_id)
    and (
      (sf.status = 'verifying' and w.status = 'verifying')
      or (sf.status = 'failed_retryable' and w.status = 'failed_retryable')
      or (sf.status = 'reopened' and w.status = 'reopened')
    )
    and a.status in ('verification_pending','needs_follow_up')
    and w.mode = 'enforced' and w.active = true
), expected_keys as materialized (
  select distinct keys.bucket_key
  from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id
  from site_change_applications a
  join eligible e on e.application_id = a.id
  where a.project_id = e.project_id
    and a.site_fix_id = e.id
    and a.content_action_id is null
    and a.status = e.expected_application_status
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of a
), locked_work as materialized (
  select e.*
  from eligible e
  join site_fixes sf
    on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w
    on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status
    and w.status = e.expected_signature_status
    and w.active = e.expected_signature_active
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = e.conflict_bucket_keys
    and jsonb_array_length(e.conflict_bucket_keys) > 0
    and (
      e.application_id is null
      or exists (
        select 1 from locked_application a
        where a.id = e.application_id
      )
    )
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
), verified_application as (
  update site_change_applications a
  set status = 'verified',
      deployment_snapshot = sqlc.arg(deployment_snapshot)::jsonb,
      verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
      failure_reason = null,
      deployed_at = coalesce(deployed_at, sqlc.arg(verified_at)),
      verified_at = coalesce(verified_at, sqlc.arg(verified_at)),
      updated_at = now()
  from locked_work e
  where a.id = e.application_id
    and a.project_id = sqlc.arg(project_id)
    and a.site_fix_id = sqlc.arg(site_fix_id)
    and a.content_action_id is null
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning a.site_fix_id
), verified_fix as (
  update site_fixes sf
  set status = 'verified',
      verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
      failure_reason = null,
      deployed_at = coalesce(deployed_at, sqlc.arg(verified_at)),
      verified_at = coalesce(verified_at, sqlc.arg(verified_at)),
      updated_at = now()
  from verified_application va
  where sf.id = va.site_fix_id
    and sf.project_id = sqlc.arg(project_id)
    and sf.status in ('verifying','failed_retryable','reopened')
  returning sf.*
), rejected_markers as (
  update doctor_ai_on_demand_triggers marker
  set status = 'rejected',
      result_snapshot = case when marker.status = 'consumed' then marker.result_snapshot else jsonb_build_object('decision','rejected','reason','site fix verified without this AI result') end,
      rejection_reason = 'site fix verified without this AI result', consumed_at = coalesce(marker.consumed_at, now()), lifecycle_applied_at = now(),
      processing_token = null, processing_expires_at = null
  from verified_fix sf
  where marker.project_id = sf.project_id and marker.site_fix_id = sf.id
    and (marker.status in ('pending','processing') or (marker.status = 'consumed' and marker.lifecycle_applied_at is null))
  returning marker.ai_call_id, marker.project_id
), finished_ai_calls as (
  update ai_call_records call
  set status = 'failed', error_code = coalesce(call.error_code, 'doctor_ai_marker_rejected'),
      finished_at = coalesce(call.finished_at, now()), updated_at = now()
  from rejected_markers marker
  where call.project_id = marker.project_id and call.id = marker.ai_call_id and call.status = 'running'
  returning call.id
), signature_transition as (
  update work_signature_registry w
  set status = 'verified', active = false, updated_at = now()
  from verified_fix vf
  where w.id = vf.work_signature_id and w.project_id = vf.project_id
  returning w.id
)
select verified_fix.* from verified_fix
cross join signature_transition;

-- name: MarkCanonicalSiteFixRetryable :one
with eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         a.id as application_id,
         a.status as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  join site_change_applications a
    on a.site_fix_id = sf.id and a.project_id = sf.project_id
   and a.content_action_id is null
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and a.id = sqlc.arg(application_id)
    and (
      (sf.status = 'verifying' and w.status = 'verifying')
      or (sf.status = 'reopened' and w.status = 'reopened')
	  or (sf.status = 'awaiting_deploy' and w.status = 'awaiting_deploy')
    )
    and sf.retry_count < sf.max_retries
	and a.status in ('deployment_pending','verification_pending','needs_follow_up')
    and w.mode = 'enforced' and w.active = true
), expected_keys as materialized (
  select distinct keys.bucket_key
  from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id
  from site_change_applications a
  join eligible e on e.application_id = a.id
  where a.project_id = e.project_id
    and a.site_fix_id = e.id
    and a.content_action_id is null
    and a.status = e.expected_application_status
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of a
), locked_work as materialized (
  select e.*
  from eligible e
  join site_fixes sf
    on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w
    on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status
    and w.status = e.expected_signature_status
    and w.active = e.expected_signature_active
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = e.conflict_bucket_keys
    and jsonb_array_length(e.conflict_bucket_keys) > 0
    and (
      e.application_id is null
      or exists (
        select 1 from locked_application a
        where a.id = e.application_id
      )
    )
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
), retryable_application as (
  update site_change_applications a
  set status = 'needs_follow_up',
      verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
      failure_reason = sqlc.arg(failure_reason), updated_at = now()
  from locked_work e
  where a.id = e.application_id and a.project_id = sqlc.arg(project_id)
    and a.site_fix_id = sqlc.arg(site_fix_id) and a.content_action_id is null
    and (select count(*) from bumped) = (select count(*) from expected_keys)
  returning a.site_fix_id
), transitioned as (
  update site_fixes sf
  set status = 'failed_retryable',
      retry_count = retry_count + 1,
      verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
      failure_reason = sqlc.arg(failure_reason),
      updated_at = now()
  from locked_work e
  join retryable_application a on a.site_fix_id = e.id
  where sf.id = e.id
    and sf.project_id = sqlc.arg(project_id)
	and sf.status in ('awaiting_deploy','verifying','reopened')
    and sf.retry_count < sf.max_retries
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning sf.*
), rejected_markers as (
  update doctor_ai_on_demand_triggers marker
  set status = 'rejected',
      result_snapshot = case when marker.status = 'consumed' then marker.result_snapshot else jsonb_build_object('decision','rejected','reason','verification attempt completed without this AI result') end,
      rejection_reason = 'verification attempt completed without this AI result', consumed_at = coalesce(marker.consumed_at, now()), lifecycle_applied_at = now(),
      processing_token = null, processing_expires_at = null
  from transitioned sf
  where marker.project_id = sf.project_id and marker.site_fix_id = sf.id
    and (marker.status in ('pending','processing') or (marker.status = 'consumed' and marker.lifecycle_applied_at is null))
  returning marker.ai_call_id, marker.project_id
), finished_ai_calls as (
  update ai_call_records call
  set status = 'failed', error_code = coalesce(call.error_code, 'doctor_ai_marker_rejected'),
      finished_at = coalesce(call.finished_at, now()), updated_at = now()
  from rejected_markers marker
  where call.project_id = marker.project_id and call.id = marker.ai_call_id and call.status = 'running'
  returning call.id
), signature_transition as (
  update work_signature_registry w
  set status = 'failed_retryable', active = true, updated_at = now()
  from transitioned sf
  where w.id = sf.work_signature_id and w.project_id = sf.project_id
  returning w.id
)
select transitioned.* from transitioned
cross join signature_transition;

-- name: ReopenCanonicalSiteFix :one
with eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         a.id as application_id,
         a.status as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  join site_change_applications a
    on a.site_fix_id = sf.id and a.project_id = sf.project_id
   and a.content_action_id is null
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and a.id = sqlc.arg(application_id)
    and a.status = 'needs_follow_up'
    and sf.status = 'failed_retryable'
    and w.status = 'failed_retryable'
    and sf.retry_count <= sf.max_retries
    and w.mode = 'enforced' and w.active = true
), expected_keys as materialized (
  select distinct keys.bucket_key
  from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id
  from site_change_applications a
  join eligible e on e.application_id = a.id
  where a.project_id = e.project_id
    and a.site_fix_id = e.id
    and a.content_action_id is null
    and a.status = e.expected_application_status
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of a
), locked_work as materialized (
  select e.*
  from eligible e
  join site_fixes sf
    on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w
    on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status
    and w.status = e.expected_signature_status
    and w.active = e.expected_signature_active
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = e.conflict_bucket_keys
    and jsonb_array_length(e.conflict_bucket_keys) > 0
    and (
      e.application_id is null
      or exists (
        select 1 from locked_application a
        where a.id = e.application_id
      )
    )
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
), reopened_application as (
  update site_change_applications a
  set status = 'verification_pending', failure_reason = null, updated_at = now()
  from locked_work e
  where a.id = e.application_id and a.project_id = sqlc.arg(project_id)
    and a.site_fix_id = sqlc.arg(site_fix_id) and a.content_action_id is null
    and a.status = 'needs_follow_up'
    and (select count(*) from bumped) = (select count(*) from expected_keys)
  returning a.site_fix_id
), transitioned as (
  update site_fixes sf
  set status = 'reopened', failure_reason = null, updated_at = now()
  from locked_work e
  join reopened_application a on a.site_fix_id = e.id
  where sf.id = e.id
    and sf.status = 'failed_retryable'
    and sf.retry_count <= sf.max_retries
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning sf.*
), signature_transition as (
  update work_signature_registry w
  set status = 'reopened', active = true, updated_at = now()
  from transitioned sf
  where w.id = sf.work_signature_id and w.project_id = sf.project_id
  returning w.id
)
select transitioned.* from transitioned
cross join signature_transition;

-- name: TerminalizeCanonicalSiteFix :one
with eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         a.id as application_id,
         a.status as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  join site_change_applications a
    on a.site_fix_id = sf.id and a.project_id = sf.project_id
   and a.content_action_id is null
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and a.id = sqlc.arg(application_id)
    and (
      (sf.status = 'verifying' and w.status = 'verifying')
      or (sf.status = 'failed_retryable' and w.status = 'failed_retryable')
      or (sf.status = 'reopened' and w.status = 'reopened')
	  or (sf.status = 'awaiting_deploy' and w.status = 'awaiting_deploy')
	  or (sf.status = 'applying' and w.status = 'executing')
    )
    and (sf.retry_count >= sf.max_retries or sqlc.arg(force_terminal)::boolean)
	and a.status in ('draft_ready','source_mapping_required','ready_for_pr','creating_pr','github_pr_open','manual_apply_required','deployment_pending','verification_pending','needs_follow_up')
    and w.mode = 'enforced' and w.active = true
), expected_keys as materialized (
  select distinct keys.bucket_key
  from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id
  from site_change_applications a
  join eligible e on e.application_id = a.id
  where a.project_id = e.project_id
    and a.site_fix_id = e.id
    and a.content_action_id is null
    and a.status = e.expected_application_status
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of a
), locked_work as materialized (
  select e.*
  from eligible e
  join site_fixes sf
    on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w
    on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status
    and w.status = e.expected_signature_status
    and w.active = e.expected_signature_active
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = e.conflict_bucket_keys
    and jsonb_array_length(e.conflict_bucket_keys) > 0
    and (
      e.application_id is null
      or exists (
        select 1 from locked_application a
        where a.id = e.application_id
      )
    )
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
), terminal_application as (
  update site_change_applications a
  set status = 'failed', verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
      failure_reason = sqlc.arg(failure_reason),
	  pr_claim_token = null, pr_claim_expires_at = null, pr_claim_authority_fingerprint = null,
	  updated_at = now()
  from locked_work e
  where a.id = e.application_id and a.project_id = sqlc.arg(project_id)
    and a.site_fix_id = sqlc.arg(site_fix_id) and a.content_action_id is null
    and (select count(*) from bumped) = (select count(*) from expected_keys)
  returning a.site_fix_id
), transitioned as (
  update site_fixes sf
  set status = 'failed_terminal',
      verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
      failure_reason = sqlc.arg(failure_reason),
      updated_at = now()
  from locked_work e
  join terminal_application a on a.site_fix_id = e.id
  where sf.id = e.id
	and sf.status in ('applying','awaiting_deploy','verifying','failed_retryable','reopened')
    and (sf.retry_count >= sf.max_retries or sqlc.arg(force_terminal)::boolean)
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning sf.*
), rejected_markers as (
  update doctor_ai_on_demand_triggers marker
  set status = 'rejected',
      result_snapshot = case when marker.status = 'consumed' then marker.result_snapshot else jsonb_build_object('decision','rejected','reason','site fix reached terminal verification state') end,
      rejection_reason = 'site fix reached terminal verification state', consumed_at = coalesce(marker.consumed_at, now()), lifecycle_applied_at = now(),
      processing_token = null, processing_expires_at = null
  from transitioned sf
  where marker.project_id = sf.project_id and marker.site_fix_id = sf.id
    and (marker.status in ('pending','processing') or (marker.status = 'consumed' and marker.lifecycle_applied_at is null))
  returning marker.ai_call_id, marker.project_id
), finished_ai_calls as (
  update ai_call_records call
  set status = 'failed', error_code = coalesce(call.error_code, 'doctor_ai_marker_rejected'),
      finished_at = coalesce(call.finished_at, now()), updated_at = now()
  from rejected_markers marker
  where call.project_id = marker.project_id and call.id = marker.ai_call_id and call.status = 'running'
  returning call.id
), signature_transition as (
  update work_signature_registry w
  set status = 'failed_terminal', active = false, updated_at = now()
  from transitioned sf
  where w.id = sf.work_signature_id and w.project_id = sf.project_id
  returning w.id
)
select transitioned.* from transitioned
cross join signature_transition;

-- name: TerminateCanonicalSiteFixByUser :one
with authority as materialized (
  select pwa.project_id from product_writer_authority pwa
  where pwa.project_id = sqlc.arg(project_id) and pwa.product = 'doctor'
    and pwa.writer_authority = 'canonical' and pwa.write_fenced = false
  for update
), eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status, w.status as expected_signature_status,
         w.active as expected_signature_active, a.id as application_id
  from site_fixes sf
  join work_signature_registry w on w.id = sf.work_signature_id and w.project_id = sf.project_id
  left join lateral (
    select app.id from site_change_applications app
    where app.project_id = sf.project_id and app.site_fix_id = sf.id and app.content_action_id is null
    order by app.updated_at desc limit 1
  ) a on true
  where sf.project_id = sqlc.arg(project_id) and sf.id = sqlc.arg(site_fix_id)
    and (
      (sf.status = 'proposed' and w.status in ('reserved','proposed'))
      or (sf.status = 'approved' and w.status = 'approved')
      or (sf.status = 'preparing' and w.status = 'preparing')
      or (sf.status in ('ready_to_apply','applying') and w.status = 'executing')
      or (sf.status = 'awaiting_deploy' and w.status = 'awaiting_deploy')
      or (sf.status = 'verifying' and w.status = 'verifying')
      or (sf.status = 'failed_retryable' and w.status = 'failed_retryable')
      or (sf.status = 'reopened' and w.status = 'reopened')
    )
    and w.mode = 'enforced' and w.active = true and exists (select 1 from authority)
), expected_keys as materialized (
  select distinct keys.bucket_key from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id from work_conflict_buckets b join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id) order by b.bucket_key for update of b
), locked_application as materialized (
  select app.id from site_change_applications app join eligible e on e.application_id = app.id
  where app.project_id = e.project_id and app.site_fix_id = e.id and app.content_action_id is null
  for update of app
), locked_work as materialized (
  select e.* from eligible e
  join site_fixes sf on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status and w.status = e.expected_signature_status
    and w.active = e.expected_signature_active and w.mode = 'enforced'
    and w.conflict_bucket_keys = e.conflict_bucket_keys and jsonb_array_length(e.conflict_bucket_keys) > 0
    and (e.application_id is null or exists (select 1 from locked_application app where app.id = e.application_id))
    and (select count(*) from locked_buckets) = (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked where b.id = locked.id and exists (select 1 from locked_work)
  returning b.id
), failed_application as (
  update site_change_applications app
  set status = 'failed', failure_reason = sqlc.arg(failure_reason),
      pr_claim_token = null, pr_claim_expires_at = null, pr_claim_authority_fingerprint = null, updated_at = now()
  from locked_work e where e.application_id is not null and app.id = e.application_id
    and app.project_id = e.project_id and app.site_fix_id = e.id and app.content_action_id is null
    and (select count(*) from bumped) = (select count(*) from expected_keys)
  returning app.id
), rejected_markers as (
  update doctor_ai_on_demand_triggers marker
  set status = 'rejected', result_snapshot = case when marker.status = 'consumed' then marker.result_snapshot else jsonb_build_object('decision','rejected','reason','site fix terminated') end,
      rejection_reason = 'site fix terminated', consumed_at = coalesce(marker.consumed_at, now()), lifecycle_applied_at = now(),
      processing_token = null, processing_expires_at = null
  from locked_work e
  where marker.project_id = e.project_id and marker.site_fix_id = e.id
    and (marker.status in ('pending','processing') or (marker.status = 'consumed' and marker.lifecycle_applied_at is null))
  returning marker.request_id, marker.ai_call_id, marker.project_id
), finished_ai_calls as (
  update ai_call_records call
  set status = 'failed', error_code = coalesce(call.error_code, 'doctor_ai_marker_rejected'),
      finished_at = coalesce(call.finished_at, now()), updated_at = now()
  from rejected_markers marker
  where call.project_id = marker.project_id and call.id = marker.ai_call_id and call.status = 'running'
  returning call.id
), transitioned as (
  update site_fixes sf set status = 'failed_terminal', failure_reason = sqlc.arg(failure_reason),
      verification_snapshot = sqlc.arg(verification_snapshot)::jsonb, updated_at = now()
  from locked_work e where sf.id = e.id and sf.project_id = e.project_id
    and sf.status = e.expected_fix_status
    and (e.application_id is null or exists (select 1 from failed_application app where app.id = e.application_id))
    and (select count(*) from bumped) = (select count(*) from expected_keys)
  returning sf.*
), signature_transition as (
  update work_signature_registry w set status = 'failed_terminal', active = false, updated_at = now()
  from transitioned sf where w.id = sf.work_signature_id and w.project_id = sf.project_id returning w.id
)
select transitioned.* from transitioned cross join signature_transition;

-- name: SupersedeCanonicalSiteFix :one
with eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         null::uuid as application_id,
         null::text as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and sf.status in (
      'proposed','approved','preparing','ready_to_apply','applying',
      'awaiting_deploy','verifying','failed_retryable','reopened'
    )
    and (
      (sf.status = 'proposed' and w.status in ('reserved','proposed'))
      or (sf.status = 'approved' and w.status = 'approved')
      or (sf.status = 'preparing' and w.status = 'preparing')
      or (sf.status in ('ready_to_apply','applying') and w.status = 'executing')
      or (sf.status = 'awaiting_deploy' and w.status = 'awaiting_deploy')
      or (sf.status = 'verifying' and w.status = 'verifying')
      or (sf.status = 'failed_retryable' and w.status = 'failed_retryable')
      or (sf.status = 'reopened' and w.status = 'reopened')
    )
    and w.mode = 'enforced' and w.active = true
), expected_keys as materialized (
  select distinct keys.bucket_key
  from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id
  from site_change_applications a
  join eligible e on e.application_id = a.id
  where a.project_id = e.project_id
    and a.site_fix_id = e.id
    and a.content_action_id is null
    and a.status = e.expected_application_status
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of a
), locked_work as materialized (
  select e.*
  from eligible e
  join site_fixes sf
    on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w
    on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status
    and w.status = e.expected_signature_status
    and w.active = e.expected_signature_active
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = e.conflict_bucket_keys
    and jsonb_array_length(e.conflict_bucket_keys) > 0
    and (
      e.application_id is null
      or exists (
        select 1 from locked_application a
        where a.id = e.application_id
      )
    )
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
), transitioned as (
  update site_fixes sf
  set status = 'superseded', failure_reason = sqlc.narg(reason), updated_at = now()
  from locked_work e
  where sf.id = e.id
    and sf.status in (
      'proposed','approved','preparing','ready_to_apply','applying',
      'awaiting_deploy','verifying','failed_retryable','reopened'
    )
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning sf.*
), superseded_markers as (
  update doctor_ai_on_demand_triggers marker
  set status = 'superseded',
      result_snapshot = case when marker.status = 'consumed' then marker.result_snapshot else jsonb_build_object('decision','superseded','reason','site fix superseded') end,
      rejection_reason = 'site fix superseded', consumed_at = coalesce(marker.consumed_at, now()), lifecycle_applied_at = now(),
      processing_token = null, processing_expires_at = null
  from transitioned sf
  where marker.project_id = sf.project_id and marker.site_fix_id = sf.id
    and (marker.status in ('pending','processing') or (marker.status = 'consumed' and marker.lifecycle_applied_at is null))
  returning marker.ai_call_id, marker.project_id
), finished_ai_calls as (
  update ai_call_records call
  set status = 'failed', error_code = coalesce(call.error_code, 'doctor_ai_marker_superseded'),
      finished_at = coalesce(call.finished_at, now()), updated_at = now()
  from superseded_markers marker
  where call.project_id = marker.project_id and call.id = marker.ai_call_id and call.status = 'running'
  returning call.id
), signature_transition as (
  update work_signature_registry w
  set status = 'superseded', active = false, updated_at = now()
  from transitioned sf
  where w.id = sf.work_signature_id and w.project_id = sf.project_id
  returning w.id
)
select transitioned.* from transitioned
cross join signature_transition;

-- name: MarkCanonicalSiteFixMigrationRolledBack :one
with eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         null::uuid as application_id,
         null::text as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and sf.migration_batch_id = sqlc.arg(migration_batch_id)
    and sf.status in (
      'proposed','approved','preparing','ready_to_apply','applying',
      'awaiting_deploy','failed_retryable'
    )
    and (
      (sf.status = 'proposed' and w.status in ('reserved','proposed'))
      or (sf.status = 'approved' and w.status = 'approved')
      or (sf.status = 'preparing' and w.status = 'preparing')
      or (sf.status in ('ready_to_apply','applying') and w.status = 'executing')
      or (sf.status = 'awaiting_deploy' and w.status = 'awaiting_deploy')
      or (sf.status = 'failed_retryable' and w.status = 'failed_retryable')
    )
    and w.mode = 'enforced' and w.active = true
), expected_keys as materialized (
  select distinct keys.bucket_key
  from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id
  from site_change_applications a
  join eligible e on e.application_id = a.id
  where a.project_id = e.project_id
    and a.site_fix_id = e.id
    and a.content_action_id is null
    and a.status = e.expected_application_status
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of a
), locked_work as materialized (
  select e.*
  from eligible e
  join site_fixes sf
    on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w
    on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status
    and w.status = e.expected_signature_status
    and w.active = e.expected_signature_active
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = e.conflict_bucket_keys
    and jsonb_array_length(e.conflict_bucket_keys) > 0
    and (
      e.application_id is null
      or exists (
        select 1 from locked_application a
        where a.id = e.application_id
      )
    )
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
), transitioned as (
  update site_fixes sf
  set status = 'migration_rolled_back',
      failure_reason = sqlc.arg(reason),
      updated_at = now()
  from locked_work e
  where sf.id = e.id
    and sf.migration_batch_id = sqlc.arg(migration_batch_id)
    and sf.status in (
      'proposed','approved','preparing','ready_to_apply','applying',
      'awaiting_deploy','failed_retryable'
    )
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning sf.*
), superseded_markers as (
  update doctor_ai_on_demand_triggers marker
  set status = 'superseded',
      result_snapshot = case when marker.status = 'consumed' then marker.result_snapshot else jsonb_build_object('decision','superseded','reason','site fix migration rolled back') end,
      rejection_reason = 'site fix migration rolled back', consumed_at = coalesce(marker.consumed_at, now()), lifecycle_applied_at = now(),
      processing_token = null, processing_expires_at = null
  from transitioned sf
  where marker.project_id = sf.project_id and marker.site_fix_id = sf.id
    and (marker.status in ('pending','processing') or (marker.status = 'consumed' and marker.lifecycle_applied_at is null))
  returning marker.ai_call_id, marker.project_id
), finished_ai_calls as (
  update ai_call_records call
  set status = 'failed', error_code = coalesce(call.error_code, 'doctor_ai_marker_superseded'),
      finished_at = coalesce(call.finished_at, now()), updated_at = now()
  from superseded_markers marker
  where call.project_id = marker.project_id and call.id = marker.ai_call_id and call.status = 'running'
  returning call.id
), signature_transition as (
  update work_signature_registry w
  set status = 'cancelled', active = false, updated_at = now()
  from transitioned sf
  where w.id = sf.work_signature_id and w.project_id = sf.project_id
  returning w.id
)
select transitioned.* from transitioned
cross join signature_transition;

-- name: MarkCanonicalSiteFixApplied :one
with eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         a.id as application_id,
         a.status as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  join site_change_applications a
    on a.site_fix_id = sf.id and a.project_id = sf.project_id
   and a.content_action_id is null
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and a.id = sqlc.arg(application_id)
    and sf.status = 'applying'
    and w.status = 'executing'
    and a.status = 'github_pr_merged'
    and w.mode = 'enforced' and w.active = true
), expected_keys as materialized (
  select distinct keys.bucket_key
  from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id
  from site_change_applications a
  join eligible e on e.application_id = a.id
  where a.project_id = e.project_id
    and a.site_fix_id = e.id
    and a.content_action_id is null
    and a.status = e.expected_application_status
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of a
), locked_work as materialized (
  select e.*
  from eligible e
  join site_fixes sf
    on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w
    on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status
    and w.status = e.expected_signature_status
    and w.active = e.expected_signature_active
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = e.conflict_bucket_keys
    and jsonb_array_length(e.conflict_bucket_keys) > 0
    and (
      e.application_id is null
      or exists (
        select 1 from locked_application a
        where a.id = e.application_id
      )
    )
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
), applied_application as (
  update site_change_applications a
  set status = 'deployment_pending',
      updated_at = now()
  from locked_work e
  where a.id = e.application_id
    and a.project_id = sqlc.arg(project_id)
    and a.site_fix_id = sqlc.arg(site_fix_id)
    and a.content_action_id is null
    and a.status = 'github_pr_merged'
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning a.site_fix_id
), transitioned as (
  update site_fixes sf
  set applied_at = coalesce(sf.applied_at, sqlc.arg(applied_at)),
      failure_reason = null,
      updated_at = now()
  from applied_application a
  where sf.id = a.site_fix_id
    and sf.project_id = sqlc.arg(project_id)
    and sf.status = 'applying'
  returning sf.*
), signature_transition as (
  update work_signature_registry w
  set status = 'executing', active = true, updated_at = now()
  from transitioned sf
  where w.id = sf.work_signature_id and w.project_id = sf.project_id
  returning w.id
)
select transitioned.* from transitioned
cross join signature_transition;

-- name: MarkCanonicalSiteFixAwaitingDeploy :one
with eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         a.id as application_id,
         a.status as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  join site_change_applications a
    on a.site_fix_id = sf.id and a.project_id = sf.project_id
   and a.content_action_id is null
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and a.id = sqlc.arg(application_id)
    and sf.status = 'applying'
    and sf.applied_at is not null
    and w.status = 'executing'
    and a.status = 'deployment_pending'
    and w.mode = 'enforced' and w.active = true
), expected_keys as materialized (
  select distinct keys.bucket_key
  from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id
  from site_change_applications a
  join eligible e on e.application_id = a.id
  where a.project_id = e.project_id
    and a.site_fix_id = e.id
    and a.content_action_id is null
    and a.status = e.expected_application_status
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of a
), locked_work as materialized (
  select e.*
  from eligible e
  join site_fixes sf
    on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w
    on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status
    and w.status = e.expected_signature_status
    and w.active = e.expected_signature_active
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = e.conflict_bucket_keys
    and jsonb_array_length(e.conflict_bucket_keys) > 0
    and (
      e.application_id is null
      or exists (
        select 1 from locked_application a
        where a.id = e.application_id
      )
    )
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
), awaiting_application as (
  update site_change_applications a
  set status = 'deployment_pending', updated_at = now()
  from locked_work e
  where a.id = e.application_id
    and a.project_id = sqlc.arg(project_id)
    and a.site_fix_id = sqlc.arg(site_fix_id)
    and a.content_action_id is null
    and a.status = 'deployment_pending'
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning a.site_fix_id
), transitioned as (
  update site_fixes sf
  set status = 'awaiting_deploy',
      failure_reason = null,
      updated_at = now()
  from awaiting_application a
  where sf.id = a.site_fix_id
    and sf.project_id = sqlc.arg(project_id)
    and sf.status = 'applying'
    and sf.applied_at is not null
  returning sf.*
), signature_transition as (
  update work_signature_registry w
  set status = 'awaiting_deploy', active = true, updated_at = now()
  from transitioned sf
  where w.id = sf.work_signature_id and w.project_id = sf.project_id
  returning w.id
)
select transitioned.* from transitioned
cross join signature_transition;

-- name: MarkCanonicalSiteFixVerifying :one
with eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         a.id as application_id,
         a.status as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  join site_change_applications a
    on a.site_fix_id = sf.id and a.project_id = sf.project_id
   and a.content_action_id is null
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and a.id = sqlc.arg(application_id)
    and (
      (sf.status = 'awaiting_deploy' and w.status = 'awaiting_deploy' and a.status = 'deployment_pending')
      or (sf.status = 'reopened' and w.status = 'reopened' and a.status = 'verification_pending')
    )
    and w.mode = 'enforced' and w.active = true
), expected_keys as materialized (
  select distinct keys.bucket_key
  from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id
  from site_change_applications a
  join eligible e on e.application_id = a.id
  where a.project_id = e.project_id
    and a.site_fix_id = e.id
    and a.content_action_id is null
    and a.status = e.expected_application_status
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of a
), locked_work as materialized (
  select e.*
  from eligible e
  join site_fixes sf
    on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w
    on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status
    and w.status = e.expected_signature_status
    and w.active = e.expected_signature_active
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = e.conflict_bucket_keys
    and jsonb_array_length(e.conflict_bucket_keys) > 0
    and (
      e.application_id is null
      or exists (
        select 1 from locked_application a
        where a.id = e.application_id
      )
    )
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
), deployed_application as (
  update site_change_applications a
  set status = 'verification_pending',
      deployment_snapshot = sqlc.arg(deployment_snapshot)::jsonb,
      deployed_at = coalesce(deployed_at, sqlc.arg(deployed_at)),
      failure_reason = null,
      updated_at = now()
  from locked_work e
  where a.id = e.application_id
    and a.project_id = sqlc.arg(project_id)
    and a.site_fix_id = sqlc.arg(site_fix_id)
    and a.content_action_id is null
    and a.status in ('deployment_pending','verification_pending')
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning a.site_fix_id
), transitioned as (
  update site_fixes sf
  set status = 'verifying',
      deployed_at = coalesce(deployed_at, sqlc.arg(deployed_at)),
      failure_reason = null,
      updated_at = now()
  from deployed_application a
  where sf.id = a.site_fix_id
    and sf.project_id = sqlc.arg(project_id)
    and sf.status in ('awaiting_deploy','reopened')
  returning sf.*
), signature_transition as (
  update work_signature_registry w
  set status = 'verifying', active = true, updated_at = now()
  from transitioned sf
  where w.id = sf.work_signature_id and w.project_id = sf.project_id
  returning w.id
)
select transitioned.* from transitioned
cross join signature_transition;

-- name: MarkCanonicalSiteFixPreparing :one
with eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         null::uuid as application_id,
         null::text as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and sf.status = 'approved'
    and w.status = 'approved'
    and w.mode = 'enforced' and w.active = true
), expected_keys as materialized (
  select distinct keys.bucket_key
  from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id
  from site_change_applications a
  join eligible e on e.application_id = a.id
  where a.project_id = e.project_id
    and a.site_fix_id = e.id
    and a.content_action_id is null
    and a.status = e.expected_application_status
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of a
), locked_work as materialized (
  select e.*
  from eligible e
  join site_fixes sf
    on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w
    on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status
    and w.status = e.expected_signature_status
    and w.active = e.expected_signature_active
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = e.conflict_bucket_keys
    and jsonb_array_length(e.conflict_bucket_keys) > 0
    and (
      e.application_id is null
      or exists (
        select 1 from locked_application a
        where a.id = e.application_id
      )
    )
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
), transitioned as (
  update site_fixes sf
  set status = 'preparing', updated_at = now()
  from locked_work e
  where sf.id = e.id
    and sf.status = 'approved'
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning sf.*
), signature_transition as (
  update work_signature_registry w
  set status = 'preparing', active = true, updated_at = now()
  from transitioned sf
  where w.id = sf.work_signature_id and w.project_id = sf.project_id
  returning w.id
)
select transitioned.* from transitioned
cross join signature_transition;

-- name: MarkCanonicalSiteFixReadyToApply :one
with eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         null::uuid as application_id,
         null::text as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and sf.status = 'preparing'
    and w.status = 'preparing'
    and w.mode = 'enforced' and w.active = true
), expected_keys as materialized (
  select distinct keys.bucket_key
  from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id
  from site_change_applications a
  join eligible e on e.application_id = a.id
  where a.project_id = e.project_id
    and a.site_fix_id = e.id
    and a.content_action_id is null
    and a.status = e.expected_application_status
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of a
), locked_work as materialized (
  select e.*
  from eligible e
  join site_fixes sf
    on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w
    on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status
    and w.status = e.expected_signature_status
    and w.active = e.expected_signature_active
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = e.conflict_bucket_keys
    and jsonb_array_length(e.conflict_bucket_keys) > 0
    and (
      e.application_id is null
      or exists (
        select 1 from locked_application a
        where a.id = e.application_id
      )
    )
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
), transitioned as (
  update site_fixes sf
  set status = 'ready_to_apply', updated_at = now()
  from locked_work e
  where sf.id = e.id
    and sf.status = 'preparing'
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning sf.*
), signature_transition as (
  update work_signature_registry w
  set status = 'executing', active = true, updated_at = now()
  from transitioned sf
  where w.id = sf.work_signature_id and w.project_id = sf.project_id
  returning w.id
)
select transitioned.* from transitioned
cross join signature_transition;

-- name: ClaimCanonicalSiteFixApplying :one
with eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status,
         w.status as expected_signature_status,
         w.active as expected_signature_active,
         null::uuid as application_id,
         null::text as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and sf.status = 'ready_to_apply'
    and w.status = 'executing'
    and w.mode = 'enforced' and w.active = true
), expected_keys as materialized (
  select distinct keys.bucket_key
  from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id
  from site_change_applications a
  join eligible e on e.application_id = a.id
  where a.project_id = e.project_id
    and a.site_fix_id = e.id
    and a.content_action_id is null
    and a.status = e.expected_application_status
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of a
), locked_work as materialized (
  select e.*
  from eligible e
  join site_fixes sf
    on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w
    on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status
    and w.status = e.expected_signature_status
    and w.active = e.expected_signature_active
    and w.mode = 'enforced'
    and w.conflict_bucket_keys = e.conflict_bucket_keys
    and jsonb_array_length(e.conflict_bucket_keys) > 0
    and (
      e.application_id is null
      or exists (
        select 1 from locked_application a
        where a.id = e.application_id
      )
    )
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
), transitioned as (
  update site_fixes sf
  set status = 'applying', updated_at = now()
  from locked_work e
  where sf.id = e.id
    and sf.status = 'ready_to_apply'
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning sf.*
), signature_transition as (
  update work_signature_registry w
  set status = 'executing', active = true, updated_at = now()
  from transitioned sf
  where w.id = sf.work_signature_id and w.project_id = sf.project_id
  returning w.id
)
select transitioned.* from transitioned
cross join signature_transition;

-- name: GetCanonicalSiteFixApplication :one
select * from site_change_applications
where project_id = sqlc.arg(project_id)
  and id = sqlc.arg(application_id)
  and site_fix_id = sqlc.arg(site_fix_id)
  and site_fix_id is not null
  and content_action_id is null;

-- name: GetLatestCanonicalSiteFixApplication :one
select * from site_change_applications
where project_id = sqlc.arg(project_id)
  and site_fix_id = sqlc.arg(site_fix_id)
  and site_fix_id is not null
  and content_action_id is null
order by updated_at desc
limit 1;

-- name: SetCanonicalSiteFixNextPollAt :exec
update site_change_applications set next_poll_at = sqlc.arg(next_poll_at), updated_at = now()
where project_id = sqlc.arg(project_id) and id = sqlc.arg(application_id)
  and site_fix_id = sqlc.arg(site_fix_id) and site_fix_id is not null
  and content_action_id is null;

-- name: ClaimDoctorAIOnDemandTrigger :one
with authorized as materialized (
  select p.id,
         case
		   when p.config->>'doctor_ai_run_policy' in ('manual_only','on_demand','automatic')
             then p.config->>'doctor_ai_run_policy'
           else 'manual_only'
         end as run_policy
  from projects p
  join site_fixes sf on sf.project_id = p.id and sf.id = sqlc.arg(site_fix_id)
  where p.id = sqlc.arg(project_id)
    and lower(coalesce(p.config->>'doctor_ai_enabled', 'false')) = 'true'
	and p.config->>'doctor_ai_run_policy' in ('manual_only','on_demand','automatic')
	and (
	  sf.status in ('awaiting_deploy','verifying','reopened','failed_retryable')
	  or (sf.status = 'applying' and exists (
	    select 1 from site_change_applications app
	    where app.project_id = sf.project_id and app.site_fix_id = sf.id
	      and app.content_action_id is null and app.status = 'manual_apply_required'
	  ))
	)
), inserted as (
  insert into doctor_ai_on_demand_triggers (
    request_id, project_id, site_fix_id, trigger_kind, requested_policy
  )
  select sqlc.arg(request_id), authorized.id, sqlc.arg(site_fix_id),
         'verification_user', authorized.run_policy
  from authorized
  on conflict (request_id) do nothing
  returning *
)
select * from inserted
union all
select marker.* from doctor_ai_on_demand_triggers marker
join authorized on authorized.id = marker.project_id
where marker.request_id = sqlc.arg(request_id)
  and marker.project_id = sqlc.arg(project_id)
  and marker.site_fix_id = sqlc.arg(site_fix_id)
  and marker.trigger_kind = 'verification_user'
	and (marker.status in ('pending','processing') or (marker.status = 'consumed' and marker.lifecycle_applied_at is null))
limit 1;

-- name: GetDoctorAIOnDemandTrigger :one
select * from doctor_ai_on_demand_triggers marker
where marker.request_id = sqlc.arg(request_id)
  and marker.project_id = sqlc.arg(project_id) and marker.site_fix_id = sqlc.arg(site_fix_id);

-- name: ClaimDoctorAIOnDemandProcessing :one
with eligible as materialized (
  select marker.request_id
  from doctor_ai_on_demand_triggers marker
  join projects p on p.id = marker.project_id
  join site_fixes sf on sf.project_id = marker.project_id and sf.id = marker.site_fix_id
  where marker.project_id = sqlc.arg(project_id)
    and marker.site_fix_id = sqlc.arg(site_fix_id)
    and marker.trigger_kind = 'verification_user'
	and (marker.status = 'pending' or (marker.status = 'processing' and marker.processing_expires_at <= clock_timestamp()))
    and lower(coalesce(p.config->>'doctor_ai_enabled', 'false')) = 'true'
	and p.config->>'doctor_ai_run_policy' in ('manual_only','on_demand','automatic')
    and sf.status in ('awaiting_deploy','verifying','reopened')
  order by marker.requested_at, marker.request_id
  for update of marker skip locked
  limit 1
)
update doctor_ai_on_demand_triggers marker
set status = 'processing', processing_token = sqlc.arg(processing_token),
	processing_expires_at = clock_timestamp() + make_interval(secs => sqlc.arg(lease_ttl_seconds)::int)
from eligible
where marker.request_id = eligible.request_id
	and (marker.status = 'pending' or (marker.status = 'processing' and marker.processing_expires_at <= clock_timestamp()))
returning marker.*;

-- name: StartDoctorAIOnDemandCall :one
with eligible as materialized (
  select marker.request_id
  from doctor_ai_on_demand_triggers marker
  join projects p on p.id = marker.project_id
  join site_fixes sf on sf.project_id = marker.project_id and sf.id = marker.site_fix_id
  where marker.project_id = sqlc.arg(project_id) and marker.site_fix_id = sqlc.arg(site_fix_id)
    and marker.request_id = sqlc.arg(request_id) and marker.status = 'processing'
    and marker.processing_token = sqlc.arg(processing_token)
    and marker.processing_expires_at > clock_timestamp() and marker.ai_call_id is null
    and lower(coalesce(p.config->>'doctor_ai_enabled', 'false')) = 'true'
    and p.config->>'doctor_ai_run_policy' in ('manual_only','on_demand','automatic')
    and sf.status in ('awaiting_deploy','verifying','reopened')
  for update of marker
), started as (
  insert into ai_call_records(project_id,stage,linked_object_type,linked_object_id,provider,model,prompt_version,request_fingerprint,status)
  select sqlc.arg(project_id),'verification','site_fix',sqlc.arg(site_fix_id),
         sqlc.arg(provider),sqlc.arg(model),sqlc.arg(prompt_version),sqlc.arg(request_fingerprint),'running'
  from eligible
  returning id
)
update doctor_ai_on_demand_triggers marker
set ai_call_id = started.id
from eligible, started
where marker.request_id = eligible.request_id
returning marker.*;

-- name: ConsumeDoctorAIOnDemandProcessing :one
with authorized as materialized (
  select marker.request_id
  from doctor_ai_on_demand_triggers marker
  join projects p on p.id = marker.project_id
  join site_fixes sf on sf.project_id = marker.project_id and sf.id = marker.site_fix_id
  where marker.project_id = sqlc.arg(project_id) and marker.site_fix_id = sqlc.arg(site_fix_id)
    and marker.request_id = sqlc.arg(request_id) and marker.status = 'processing'
    and marker.processing_token = sqlc.arg(processing_token)
    and marker.ai_call_id = sqlc.arg(ai_call_id)
    and lower(coalesce(p.config->>'doctor_ai_enabled', 'false')) = 'true'
    and p.config->>'doctor_ai_run_policy' in ('manual_only','on_demand','automatic')
    and sf.status in ('awaiting_deploy','verifying','reopened')
  for update of marker
)
update doctor_ai_on_demand_triggers marker
set status = 'consumed', processing_token = null, processing_expires_at = null,
    result_snapshot = sqlc.arg(result_snapshot)::jsonb, consumed_at = now()
from authorized
where marker.request_id = authorized.request_id
returning marker.*;

-- name: GetDoctorAIOnDemandConsumedResult :one
select * from doctor_ai_on_demand_triggers marker
where marker.project_id = sqlc.arg(project_id) and marker.site_fix_id = sqlc.arg(site_fix_id)
  and marker.status = 'consumed' and marker.lifecycle_applied_at is null
order by marker.consumed_at, marker.request_id
limit 1;

-- name: GetDoctorAIOnDemandActive :one
select * from doctor_ai_on_demand_triggers marker
where marker.project_id = sqlc.arg(project_id) and marker.site_fix_id = sqlc.arg(site_fix_id)
  and marker.status in ('pending','processing')
order by marker.requested_at, marker.request_id
limit 1;

-- name: ListDoctorAIOnDemandActiveSiteFixes :many
select distinct marker.site_fix_id
from doctor_ai_on_demand_triggers marker
where marker.project_id = sqlc.arg(project_id)
  and (marker.status in ('pending','processing') or (marker.status = 'consumed' and marker.lifecycle_applied_at is null))
order by marker.site_fix_id;

-- name: MarkDoctorAIOnDemandLifecycleApplied :one
update doctor_ai_on_demand_triggers marker
set lifecycle_applied_at = now()
where marker.project_id = sqlc.arg(project_id) and marker.site_fix_id = sqlc.arg(site_fix_id)
  and marker.request_id = sqlc.arg(request_id) and marker.status = 'consumed'
  and marker.ai_call_id = sqlc.arg(ai_call_id) and marker.lifecycle_applied_at is null
returning marker.*;

-- name: RejectDoctorAIOnDemandTriggersForSiteFix :many
with locked_markers as materialized (
  select marker.request_id, marker.ai_call_id, marker.status
  from doctor_ai_on_demand_triggers marker
  where marker.project_id = sqlc.arg(project_id) and marker.site_fix_id = sqlc.arg(site_fix_id)
    and (marker.status in ('pending','processing') or (marker.status = 'consumed' and marker.lifecycle_applied_at is null))
  order by marker.request_id
  for update of marker
), finished_calls as (
  update ai_call_records call
  set status = 'failed', error_code = coalesce(call.error_code, 'doctor_ai_marker_rejected'),
      finished_at = coalesce(call.finished_at, now()), updated_at = now()
  from locked_markers marker
  where call.project_id = sqlc.arg(project_id) and call.id = marker.ai_call_id
    and call.status = 'running'
  returning call.id
)
update doctor_ai_on_demand_triggers marker
set status = 'rejected',
    result_snapshot = case when marker.status = 'consumed' then marker.result_snapshot else sqlc.arg(result_snapshot)::jsonb end,
    rejection_reason = sqlc.arg(rejection_reason), consumed_at = coalesce(marker.consumed_at, now()), lifecycle_applied_at = now(),
    processing_token = null, processing_expires_at = null
from locked_markers locked
where marker.request_id = locked.request_id
returning marker.*;

-- name: SupersedeDoctorAIOnDemandSiblingTriggers :many
with locked_markers as materialized (
  select marker.request_id, marker.ai_call_id, marker.status
  from doctor_ai_on_demand_triggers marker
  where marker.project_id = sqlc.arg(project_id) and marker.site_fix_id = sqlc.arg(site_fix_id)
    and (marker.status in ('pending','processing') or (marker.status = 'consumed' and marker.lifecycle_applied_at is null))
    and marker.request_id <> sqlc.arg(applied_request_id)
  order by marker.request_id
  for update of marker
), finished_calls as (
  update ai_call_records call
  set status = 'failed', error_code = coalesce(call.error_code, 'doctor_ai_marker_superseded'),
      finished_at = coalesce(call.finished_at, now()), updated_at = now()
  from locked_markers marker
  where call.project_id = sqlc.arg(project_id) and call.id = marker.ai_call_id
    and call.status = 'running'
  returning call.id
)
update doctor_ai_on_demand_triggers marker
set status = 'superseded',
    result_snapshot = case when marker.status = 'consumed' then marker.result_snapshot else jsonb_build_object('decision','superseded','reason',sqlc.arg(rejection_reason)::text) end,
    rejection_reason = sqlc.arg(rejection_reason), consumed_at = coalesce(marker.consumed_at, now()), lifecycle_applied_at = now(),
    processing_token = null, processing_expires_at = null
from locked_markers locked
where marker.request_id = locked.request_id
returning marker.*;

-- name: RejectUnauthorizedDoctorAIOnDemandTriggers :many
with locked_markers as materialized (
  select marker.request_id, marker.ai_call_id, marker.status
  from doctor_ai_on_demand_triggers marker
  join projects p on p.id = marker.project_id
  join site_fixes sf on sf.project_id = marker.project_id and sf.id = marker.site_fix_id
  where marker.project_id = sqlc.arg(project_id) and marker.site_fix_id = sqlc.arg(site_fix_id)
    and (marker.status in ('pending','processing') or (marker.status = 'consumed' and marker.lifecycle_applied_at is null))
    and (
      lower(coalesce(p.config->>'doctor_ai_enabled', 'false')) <> 'true'
      or coalesce(p.config->>'doctor_ai_run_policy', '') not in ('manual_only','on_demand','automatic')
      or sf.status not in ('awaiting_deploy','verifying','reopened')
    )
  order by marker.request_id
  for update of marker
), finished_calls as (
  update ai_call_records call
  set status = 'failed', error_code = coalesce(call.error_code, 'doctor_ai_authority_revoked'),
      finished_at = coalesce(call.finished_at, now()), updated_at = now()
  from locked_markers marker
  where call.project_id = sqlc.arg(project_id) and call.id = marker.ai_call_id
    and call.status = 'running'
  returning call.id
)
update doctor_ai_on_demand_triggers marker
set status = 'rejected',
    result_snapshot = case when marker.status = 'consumed' then marker.result_snapshot else jsonb_build_object('decision','rejected','reason','Doctor AI policy or lifecycle authority was revoked') end,
    rejection_reason = 'Doctor AI policy or lifecycle authority was revoked',
    consumed_at = coalesce(marker.consumed_at, now()), lifecycle_applied_at = now(), processing_token = null, processing_expires_at = null
from locked_markers locked
where marker.request_id = locked.request_id
returning marker.*;

-- name: ListDoctorAIOnDemandConsumedUnapplied :many
select marker.request_id, marker.project_id, marker.site_fix_id, marker.ai_call_id,
  exists (
    select 1 from site_fix_verifications verification
    where verification.project_id = marker.project_id
      and verification.site_fix_id = marker.site_fix_id
      and verification.ai_call_id = marker.ai_call_id
  ) as has_lifecycle_reference
from doctor_ai_on_demand_triggers marker
join site_fixes sf on sf.project_id = marker.project_id and sf.id = marker.site_fix_id
where marker.project_id = sqlc.arg(project_id) and marker.status = 'consumed'
  and marker.lifecycle_applied_at is null
  and sf.status in ('verified','failed_terminal','superseded','migration_rolled_back')
order by marker.consumed_at, marker.request_id;

-- name: RejectDoctorAIOnDemandConsumedWithoutLifecycleReference :one
update doctor_ai_on_demand_triggers marker
set status = 'rejected', rejection_reason = 'lifecycle_completed_without_this_ai_result',
    result_snapshot = jsonb_set(marker.result_snapshot, '{rejection_reason}', '"lifecycle_completed_without_this_ai_result"'::jsonb, true),
    lifecycle_applied_at = now()
where marker.project_id = sqlc.arg(project_id) and marker.site_fix_id = sqlc.arg(site_fix_id)
  and marker.request_id = sqlc.arg(request_id) and marker.ai_call_id = sqlc.arg(ai_call_id)
  and marker.status = 'consumed' and marker.lifecycle_applied_at is null
  and not exists (
    select 1 from site_fix_verifications verification
    where verification.project_id = marker.project_id
      and verification.site_fix_id = marker.site_fix_id
      and verification.ai_call_id = marker.ai_call_id
  )
returning marker.*;

-- name: ListRejectedDoctorAIRunningCalls :many
select marker.ai_call_id
from doctor_ai_on_demand_triggers marker
join ai_call_records call on call.project_id = marker.project_id and call.id = marker.ai_call_id
where marker.project_id = sqlc.arg(project_id)
  and marker.status in ('rejected','superseded') and call.status = 'running'
order by marker.request_id;

-- name: ListCanonicalSiteFixPRsForReconciliation :many
select a.* from site_change_applications a
join site_fixes sf
  on sf.id = a.site_fix_id and sf.project_id = a.project_id
where a.project_id = sqlc.arg(project_id)
  and a.site_fix_id is not null
  and a.content_action_id is null
  and a.status = 'github_pr_open'
  and sf.status = 'applying'
order by a.updated_at asc;

-- name: ListCanonicalSiteFixesForVerification :many
select a.* from site_change_applications a
join site_fixes sf
  on sf.id = a.site_fix_id and sf.project_id = a.project_id
where a.project_id = sqlc.arg(project_id)
  and a.site_fix_id is not null
  and a.content_action_id is null
  and a.status in ('deployment_pending','verification_pending','needs_follow_up')
  and sf.status in ('awaiting_deploy','verifying','failed_retryable','reopened')
order by a.updated_at asc;

-- name: MarkCanonicalSiteFixGitHubPR :one
with authority as materialized (
  select pwa.project_id,
         concat(pwa.writer_authority, ':', pwa.write_fenced::text, ':', pwa.authority_changed_at::text) as fingerprint
  from product_writer_authority pwa
  where pwa.project_id = sqlc.arg(project_id) and pwa.product = 'doctor'
    and pwa.writer_authority = 'canonical' and pwa.write_fenced = false
  for update
)
update site_change_applications app
set publisher_connection_id = sqlc.arg(publisher_connection_id),
    repo_full_name = sqlc.arg(repo_full_name),
    base_branch = sqlc.arg(base_branch),
    working_branch = sqlc.arg(working_branch),
    base_commit_sha = sqlc.narg(base_commit_sha),
    head_commit_sha = sqlc.arg(head_commit_sha),
    source_file_path = sqlc.arg(source_file_path),
    base_file_sha = sqlc.arg(base_file_sha),
    proposed_content_hash = sqlc.narg(proposed_content_hash),
    github_pr_number = sqlc.arg(github_pr_number),
    github_pr_url = sqlc.arg(github_pr_url),
    github_pr_state = sqlc.arg(github_pr_state),
    status = 'github_pr_open',
    pr_created_at = coalesce(pr_created_at, now()),
    failure_reason = null,
    pr_claim_token = null,
    pr_claim_expires_at = null,
    pr_claim_authority_fingerprint = null,
    updated_at = now()
where app.project_id = sqlc.arg(project_id)
  and app.id = sqlc.arg(application_id)
  and app.site_fix_id = sqlc.arg(site_fix_id)
  and app.site_fix_id is not null
  and app.content_action_id is null
  and app.status = 'creating_pr'
  and app.pr_claim_token = sqlc.arg(pr_claim_token)
  and app.pr_claim_expires_at > clock_timestamp()
  and app.pr_claim_authority_fingerprint = (select fingerprint from authority)
  and exists (select 1 from authority)
returning *;

-- name: ClaimCanonicalSiteFixGitHubPR :one
with authority as materialized (
  select pwa.project_id,
         concat(pwa.writer_authority, ':', pwa.write_fenced::text, ':', pwa.authority_changed_at::text) as fingerprint
  from product_writer_authority pwa
  where pwa.project_id = sqlc.arg(project_id) and pwa.product = 'doctor'
    and pwa.writer_authority = 'canonical' and pwa.write_fenced = false
  for update
)
update site_change_applications app
set status = 'creating_pr',
    pr_claim_token = sqlc.arg(pr_claim_token),
    pr_claim_expires_at = clock_timestamp() + make_interval(secs => sqlc.arg(lease_ttl_seconds)::int),
    pr_claim_authority_fingerprint = (select fingerprint from authority),
    failure_reason = null,
    updated_at = now()
where app.project_id = sqlc.arg(project_id)
  and app.id = sqlc.arg(application_id)
  and app.site_fix_id = sqlc.arg(site_fix_id)
  and app.site_fix_id is not null and app.content_action_id is null
  and (app.status = 'ready_for_pr' or (app.status = 'creating_pr' and app.pr_claim_expires_at <= clock_timestamp()))
  and exists (
    select 1 from site_fixes sf
    join work_signature_registry w on w.id = sf.work_signature_id and w.project_id = sf.project_id
    where sf.project_id = app.project_id and sf.id = app.site_fix_id
      and sf.status = 'applying' and w.status = 'executing' and w.active = true and w.mode = 'enforced'
  )
  and exists (select 1 from authority)
returning app.*;

-- name: FailCanonicalSiteFixGitHubPRClaim :one
with authority as materialized (
  select pwa.project_id,
         concat(pwa.writer_authority, ':', pwa.write_fenced::text, ':', pwa.authority_changed_at::text) as fingerprint
  from product_writer_authority pwa
  where pwa.project_id = sqlc.arg(project_id) and pwa.product = 'doctor'
    and pwa.writer_authority = 'canonical' and pwa.write_fenced = false
  for update
)
update site_change_applications app
set status = 'needs_follow_up', failure_reason = sqlc.arg(failure_reason),
    pr_claim_token = null, pr_claim_expires_at = null, pr_claim_authority_fingerprint = null,
    updated_at = now()
where app.project_id = sqlc.arg(project_id) and app.id = sqlc.arg(application_id)
  and app.site_fix_id = sqlc.arg(site_fix_id) and app.site_fix_id is not null and app.content_action_id is null
  and app.status = 'creating_pr' and app.pr_claim_token = sqlc.arg(pr_claim_token)
  and app.pr_claim_authority_fingerprint = (select fingerprint from authority)
  and exists (select 1 from authority)
returning app.*;

-- name: RenewCanonicalSiteFixGitHubPRClaim :one
with authority as materialized (
  select pwa.project_id,
         concat(pwa.writer_authority, ':', pwa.write_fenced::text, ':', pwa.authority_changed_at::text) as fingerprint
  from product_writer_authority pwa
  where pwa.project_id = sqlc.arg(project_id) and pwa.product = 'doctor'
    and pwa.writer_authority = 'canonical' and pwa.write_fenced = false
  for update
)
update site_change_applications app
set pr_claim_expires_at = clock_timestamp() + make_interval(secs => sqlc.arg(lease_ttl_seconds)::int),
    updated_at = now()
where app.project_id = sqlc.arg(project_id) and app.id = sqlc.arg(application_id)
  and app.site_fix_id = sqlc.arg(site_fix_id) and app.content_action_id is null
  and app.status = 'creating_pr' and app.pr_claim_token = sqlc.arg(pr_claim_token)
  and app.pr_claim_authority_fingerprint = (select fingerprint from authority)
  and exists (select 1 from authority)
returning app.*;

-- name: ReopenCanonicalSiteFixApply :one
with authority as materialized (
  select pwa.project_id from product_writer_authority pwa
  where pwa.project_id = sqlc.arg(project_id) and pwa.product = 'doctor'
    and pwa.writer_authority = 'canonical' and pwa.write_fenced = false
  for update
)
update site_change_applications app
set status = 'ready_for_pr', failure_reason = null,
    pr_claim_token = null, pr_claim_expires_at = null, pr_claim_authority_fingerprint = null,
    updated_at = now()
where app.project_id = sqlc.arg(project_id) and app.id = sqlc.arg(application_id)
  and app.site_fix_id = sqlc.arg(site_fix_id) and app.site_fix_id is not null and app.content_action_id is null
  and app.status = 'needs_follow_up'
  and exists (
    select 1 from site_fixes sf
    join work_signature_registry w on w.id = sf.work_signature_id and w.project_id = sf.project_id
    where sf.project_id = app.project_id and sf.id = app.site_fix_id
      and sf.status = 'applying' and w.status = 'executing' and w.active = true and w.mode = 'enforced'
  )
  and exists (select 1 from authority)
returning app.*;

-- name: MarkCanonicalSiteFixManualHandoff :one
with authority as materialized (
  select product_writer_authority.project_id from product_writer_authority
  where product_writer_authority.project_id = sqlc.arg(project_id)
    and product = 'doctor' and writer_authority = 'canonical' and write_fenced = false
  for update
)
update site_change_applications
set status = 'manual_apply_required', failure_reason = sqlc.arg(failure_reason), updated_at = now()
where site_change_applications.project_id = sqlc.arg(project_id) and site_change_applications.id = sqlc.arg(application_id)
  and site_change_applications.site_fix_id = sqlc.arg(site_fix_id) and site_change_applications.site_fix_id is not null
  and site_change_applications.content_action_id is null and site_change_applications.status = 'ready_for_pr'
  and exists (select 1 from authority)
returning site_change_applications.*;

-- name: MarkCanonicalSiteFixPRMerged :one
with authority as materialized (
  select product_writer_authority.project_id from product_writer_authority
  where product_writer_authority.project_id = sqlc.arg(project_id) and product = 'doctor'
    and writer_authority = 'canonical' and write_fenced = false
  for update
), eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status, w.status as expected_signature_status,
         w.active as expected_signature_active, a.id as application_id,
         a.status as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  join site_change_applications a
    on a.site_fix_id = sf.id and a.project_id = sf.project_id
   and a.content_action_id is null
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and a.id = sqlc.arg(application_id)
    and sf.status = 'applying'
    and w.status = 'executing' and w.mode = 'enforced' and w.active = true
    and a.status = 'github_pr_open'
    and exists (select 1 from authority)
), expected_keys as materialized (
  select distinct keys.bucket_key
  from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_application as materialized (
  select a.id
  from site_change_applications a
  join eligible e on e.application_id = a.id
  where a.project_id = e.project_id
    and a.site_fix_id = e.id and a.content_action_id is null
    and a.status = e.expected_application_status
    and (select count(*) from locked_buckets) = (select count(*) from expected_keys)
  for update of a
), locked_work as materialized (
  select e.*
  from eligible e
  join site_fixes sf on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w
    on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status
    and w.status = e.expected_signature_status and w.active = e.expected_signature_active
    and w.mode = 'enforced' and w.conflict_bucket_keys = e.conflict_bucket_keys
    and jsonb_array_length(e.conflict_bucket_keys) > 0
    and exists (select 1 from locked_application a where a.id = e.application_id)
    and (select count(*) from locked_buckets) = (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) = (select count(*) from expected_keys)
  returning b.id
), merged_application as (
  update site_change_applications
  set status = 'deployment_pending', github_pr_state = 'merged',
      merged_at = coalesce(site_change_applications.merged_at, now()),
      next_poll_at = now() + interval '3 minutes',
      failure_reason = null, updated_at = now()
  from locked_work e
  where site_change_applications.id = e.application_id and site_change_applications.project_id = sqlc.arg(project_id)
    and site_change_applications.site_fix_id = sqlc.arg(site_fix_id) and site_change_applications.content_action_id is null
    and site_change_applications.status = 'github_pr_open'
    and (select count(*) from bumped) = (select count(*) from expected_keys)
  returning site_change_applications.site_fix_id
), transitioned as (
  update site_fixes
  set status = 'awaiting_deploy', applied_at = coalesce(site_fixes.applied_at, sqlc.arg(observed_merged_at)::timestamptz),
      failure_reason = null, updated_at = now()
  from merged_application a
  where site_fixes.id = a.site_fix_id and site_fixes.project_id = sqlc.arg(project_id)
    and site_fixes.status = 'applying'
  returning site_fixes.*
), signature_transition as (
  update work_signature_registry w
  set status = 'awaiting_deploy', active = true, updated_at = now()
  from transitioned sf
  where w.id = sf.work_signature_id and w.project_id = sf.project_id
  returning w.id
)
select transitioned.* from transitioned cross join signature_transition;

-- name: MarkCanonicalSiteFixManualApplied :one
with authority as materialized (
  select product_writer_authority.project_id from product_writer_authority
  where product_writer_authority.project_id = sqlc.arg(project_id) and product = 'doctor'
    and writer_authority = 'canonical' and write_fenced = false
  for update
), eligible as materialized (
  select sf.id, sf.project_id, sf.work_signature_id, w.conflict_bucket_keys,
         sf.status as expected_fix_status, w.status as expected_signature_status,
         w.active as expected_signature_active, a.id as application_id,
         a.status as expected_application_status
  from site_fixes sf
  join work_signature_registry w on w.id = sf.work_signature_id and w.project_id = sf.project_id
  join site_change_applications a on a.site_fix_id = sf.id and a.project_id = sf.project_id
   and a.content_action_id is null
  where sf.id = sqlc.arg(site_fix_id) and sf.project_id = sqlc.arg(project_id)
    and a.id = sqlc.arg(application_id) and sf.status = 'applying'
    and w.status = 'executing' and w.mode = 'enforced' and w.active = true
    and a.status = 'manual_apply_required' and exists (select 1 from authority)
), expected_keys as materialized (
  select distinct keys.bucket_key from eligible e
  cross join lateral jsonb_array_elements_text(e.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id) order by b.bucket_key for update of b
), locked_application as materialized (
  select a.id from site_change_applications a join eligible e on e.application_id = a.id
  where a.project_id = e.project_id and a.site_fix_id = e.id and a.content_action_id is null
    and a.status = e.expected_application_status
    and (select count(*) from locked_buckets) = (select count(*) from expected_keys)
  for update of a
), locked_work as materialized (
  select e.* from eligible e
  join site_fixes sf on sf.id = e.id and sf.project_id = e.project_id
  join work_signature_registry w on w.id = e.work_signature_id and w.project_id = e.project_id
  where sf.status = e.expected_fix_status and w.status = e.expected_signature_status
    and w.active = e.expected_signature_active and w.mode = 'enforced'
    and w.conflict_bucket_keys = e.conflict_bucket_keys
    and jsonb_array_length(e.conflict_bucket_keys) > 0
    and exists (select 1 from locked_application a where a.id = e.application_id)
    and (select count(*) from locked_buckets) = (select count(*) from expected_keys)
  for update of sf, w
), bumped as (
  update work_conflict_buckets b set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked where b.id = locked.id and exists (select 1 from locked_work)
    and (select count(*) from locked_buckets) = (select count(*) from expected_keys)
  returning b.id
), applied_application as (
  update site_change_applications a
  set status = 'deployment_pending', deployment_snapshot = sqlc.arg(deployment_snapshot)::jsonb,
      merged_at = coalesce(a.merged_at, sqlc.arg(manual_applied_at)),
      next_poll_at = sqlc.arg(manual_applied_at), failure_reason = null, updated_at = now()
  from locked_work e where a.id = e.application_id and a.project_id = sqlc.arg(project_id)
    and a.site_fix_id = sqlc.arg(site_fix_id) and a.content_action_id is null
    and a.status = 'manual_apply_required'
    and (select count(*) from bumped) = (select count(*) from expected_keys)
  returning a.site_fix_id
), transitioned as (
  update site_fixes sf set status = 'awaiting_deploy',
      applied_at = coalesce(sf.applied_at, sqlc.arg(manual_applied_at)), failure_reason = null, updated_at = now()
  from applied_application a where sf.id = a.site_fix_id and sf.project_id = sqlc.arg(project_id)
    and sf.status = 'applying' returning sf.*
), signature_transition as (
  update work_signature_registry w set status = 'awaiting_deploy', active = true, updated_at = now()
  from transitioned sf where w.id = sf.work_signature_id and w.project_id = sf.project_id returning w.id
)
select transitioned.* from transitioned cross join signature_transition;

-- name: MarkCanonicalSiteFixApplyFailure :one
with authority as materialized (
  select product_writer_authority.project_id from product_writer_authority
  where product_writer_authority.project_id = sqlc.arg(project_id) and product = 'doctor'
    and writer_authority = 'canonical' and write_fenced = false
  for update
), eligible as materialized (
  select a.id
  from site_fixes sf
  join work_signature_registry w on w.id = sf.work_signature_id and w.project_id = sf.project_id
  join site_change_applications a on a.site_fix_id = sf.id and a.project_id = sf.project_id
   and a.content_action_id is null
  where sf.id = sqlc.arg(site_fix_id) and sf.project_id = sqlc.arg(project_id)
    and a.id = sqlc.arg(application_id) and sf.status = 'applying'
    and w.status = 'executing' and w.mode = 'enforced' and w.active = true
    and a.status = 'github_pr_open' and exists (select 1 from authority)
  for update of a
)
update site_change_applications a
set status = 'needs_follow_up',
    github_pr_state = coalesce(sqlc.narg(observed_github_pr_state)::text, a.github_pr_state),
    failure_reason = sqlc.arg(failure_reason), updated_at = now()
from eligible
where a.id = eligible.id and a.project_id = sqlc.arg(project_id)
  and a.site_fix_id = sqlc.arg(site_fix_id) and a.content_action_id is null
  and a.status = 'github_pr_open'
returning a.*;
