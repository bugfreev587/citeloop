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
order by updated_at desc, id asc;

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
