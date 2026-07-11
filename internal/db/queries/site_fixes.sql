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

-- name: LockCanonicalSiteFixForUpdate :one
select * from site_fixes
where id = sqlc.arg(id)
  and project_id = sqlc.arg(project_id)
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
insert into site_change_applications (
  id, project_id, source_opportunity_id, content_action_id, site_fix_id,
  page_update_draft_id, application_kind, target_url,
  normalized_target_url, opportunity_key, source_file_paths,
  source_mapping_confidence, source_mapping_reason, patch_snapshot,
  diff_snapshot, resolution_criteria, status
) values (
  sqlc.arg(id), sqlc.arg(project_id), null, null, sqlc.arg(site_fix_id),
  null, 'site_fix', sqlc.arg(target_url), sqlc.arg(normalized_target_url),
  sqlc.arg(opportunity_key), sqlc.arg(source_file_paths)::jsonb,
  sqlc.arg(source_mapping_confidence), sqlc.arg(source_mapping_reason),
  sqlc.arg(patch_snapshot)::jsonb, sqlc.arg(diff_snapshot)::jsonb,
  sqlc.arg(resolution_criteria)::jsonb, sqlc.arg(status)
)
returning *;

-- name: RepointApplicationToCanonicalSiteFix :one
update site_change_applications
set site_fix_id = sqlc.arg(site_fix_id),
    content_action_id = null,
    application_kind = 'site_fix',
    updated_at = now()
where project_id = sqlc.arg(project_id)
  and id = sqlc.arg(application_id)
  and content_action_id = sqlc.arg(content_action_id)
  and site_fix_id is null
returning *;

-- name: RestoreApplicationToLegacyContentAction :one
update site_change_applications
set content_action_id = sqlc.arg(content_action_id),
    site_fix_id = null,
    application_kind = 'site_fix',
    updated_at = now()
where project_id = sqlc.arg(project_id)
  and id = sqlc.arg(application_id)
  and site_fix_id = sqlc.arg(site_fix_id)
  and content_action_id is null
returning *;

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
    and sf.status = 'proposed'
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
    and sf.status in ('verifying','failed_retryable','reopened')
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
         null::uuid as application_id,
         null::text as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and sf.status in ('verifying','reopened')
    and sf.retry_count < sf.max_retries
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
  set status = 'failed_retryable',
      retry_count = retry_count + 1,
      verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
      failure_reason = sqlc.arg(failure_reason),
      updated_at = now()
  from locked_work e
  where sf.id = e.id
    and sf.project_id = sqlc.arg(project_id)
    and sf.status in ('verifying','reopened')
    and sf.retry_count < sf.max_retries
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning sf.*
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
         null::uuid as application_id,
         null::text as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and sf.status = 'failed_retryable'
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
  set status = 'reopened', failure_reason = null, updated_at = now()
  from locked_work e
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
         null::uuid as application_id,
         null::text as expected_application_status
  from site_fixes sf
  join work_signature_registry w
    on w.id = sf.work_signature_id and w.project_id = sf.project_id
  where sf.id = sqlc.arg(site_fix_id)
    and sf.project_id = sqlc.arg(project_id)
    and sf.status in ('verifying','failed_retryable','reopened')
    and (sf.retry_count >= sf.max_retries or sqlc.arg(force_terminal)::boolean)
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
  set status = 'failed_terminal',
      verification_snapshot = sqlc.arg(verification_snapshot)::jsonb,
      failure_reason = sqlc.arg(failure_reason),
      updated_at = now()
  from locked_work e
  where sf.id = e.id
    and sf.status in ('verifying','failed_retryable','reopened')
    and (sf.retry_count >= sf.max_retries or sqlc.arg(force_terminal)::boolean)
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning sf.*
), signature_transition as (
  update work_signature_registry w
  set status = 'failed_terminal', active = false, updated_at = now()
  from transitioned sf
  where w.id = sf.work_signature_id and w.project_id = sf.project_id
  returning w.id
)
select transitioned.* from transitioned
cross join signature_transition;

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
    and a.status in (
      'ready_for_pr','creating_pr','github_pr_open','github_pr_merged',
      'manual_apply_required'
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
    and a.status in (
      'ready_for_pr','creating_pr','github_pr_open','github_pr_merged',
      'manual_apply_required'
    )
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
  returning a.site_fix_id
), transitioned as (
  update site_fixes sf
  set status = 'awaiting_deploy',
      applied_at = coalesce(sf.applied_at, sqlc.arg(applied_at)),
      failure_reason = null,
      updated_at = now()
  from applied_application a
  where sf.id = a.site_fix_id
    and sf.project_id = sqlc.arg(project_id)
    and sf.status = 'applying'
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

-- name: MarkCanonicalSiteFixAwaitingDeploy :one
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
    and sf.status in ('ready_to_apply','applying')
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
  set status = 'awaiting_deploy',
      applied_at = coalesce(applied_at, sqlc.arg(applied_at)),
      failure_reason = null,
      updated_at = now()
  from locked_work e
  where sf.id = e.id
    and sf.status in ('ready_to_apply','applying')
    and (select count(*) from bumped) =
        (select count(*) from expected_keys)
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
    and sf.status = 'awaiting_deploy'
    and a.status in ('github_pr_merged','deployment_pending','verification_pending')
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
    and a.status in ('github_pr_merged','deployment_pending','verification_pending')
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
    and sf.status = 'awaiting_deploy'
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
