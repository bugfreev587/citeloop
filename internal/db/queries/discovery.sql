-- name: CreateDiscoveryShadowRun :one
insert into discovery_shadow_runs
  (project_id, candidate_schema_version, signature_version)
values
  (sqlc.arg(project_id), sqlc.arg(candidate_schema_version), sqlc.arg(signature_version))
returning *;

-- name: CompleteDiscoveryShadowRun :one
update discovery_shadow_runs set
  status = 'completed',
  doctor_candidates = sqlc.arg(doctor_candidates),
  opportunity_candidates = sqlc.arg(opportunity_candidates),
  identity_ready = sqlc.arg(identity_ready),
  needs_specification = sqlc.arg(needs_specification),
  exact_duplicate_groups = sqlc.arg(exact_duplicate_groups),
  possible_conflict_groups = sqlc.arg(possible_conflict_groups),
  error = null,
  finished_at = now(),
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: FailDiscoveryShadowRun :one
update discovery_shadow_runs set
  status = 'failed',
  error = sqlc.arg(error),
  finished_at = now(),
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: UpsertDiscoveryCandidate :one
insert into discovery_candidates
  (project_id, shadow_run_id, source_kind, source_object_type, source_object_id,
   target_kind, normalized_target_set, issue_or_hypothesis_family, change_family,
   proposed_mutations, artifact_intent, intended_slug_or_canonical,
   topic_entity_identity, audience_identity, primary_success_metric,
   verification_mode, evidence_ids, evidence_fingerprint, suggested_owner,
   confidence, candidate_schema_version, status, hold_reason,
   exact_signature_hash, signature_payload, conflict_bucket_keys)
values
  (sqlc.arg(project_id), sqlc.arg(shadow_run_id), sqlc.arg(source_kind),
   sqlc.arg(source_object_type), sqlc.arg(source_object_id), sqlc.arg(target_kind),
   sqlc.arg(normalized_target_set)::jsonb, sqlc.arg(issue_or_hypothesis_family),
   sqlc.arg(change_family), sqlc.arg(proposed_mutations)::jsonb,
   sqlc.arg(artifact_intent), sqlc.narg(intended_slug_or_canonical),
   sqlc.arg(topic_entity_identity)::jsonb, sqlc.arg(audience_identity)::jsonb,
   sqlc.arg(primary_success_metric), sqlc.arg(verification_mode),
   sqlc.arg(evidence_ids)::jsonb, sqlc.arg(evidence_fingerprint),
   sqlc.arg(suggested_owner), sqlc.arg(confidence),
   sqlc.arg(candidate_schema_version), sqlc.arg(status), sqlc.narg(hold_reason),
   sqlc.narg(exact_signature_hash), sqlc.narg(signature_payload)::jsonb,
   sqlc.arg(conflict_bucket_keys)::jsonb)
on conflict
  (shadow_run_id, project_id, source_kind, source_object_type, source_object_id, candidate_schema_version)
do update set
  target_kind = excluded.target_kind,
  normalized_target_set = excluded.normalized_target_set,
  issue_or_hypothesis_family = excluded.issue_or_hypothesis_family,
  change_family = excluded.change_family,
  proposed_mutations = excluded.proposed_mutations,
  artifact_intent = excluded.artifact_intent,
  intended_slug_or_canonical = excluded.intended_slug_or_canonical,
  topic_entity_identity = excluded.topic_entity_identity,
  audience_identity = excluded.audience_identity,
  primary_success_metric = excluded.primary_success_metric,
  verification_mode = excluded.verification_mode,
  evidence_ids = excluded.evidence_ids,
  evidence_fingerprint = excluded.evidence_fingerprint,
  suggested_owner = excluded.suggested_owner,
  confidence = excluded.confidence,
  status = excluded.status,
  hold_reason = excluded.hold_reason,
  exact_signature_hash = excluded.exact_signature_hash,
  signature_payload = excluded.signature_payload,
  conflict_bucket_keys = excluded.conflict_bucket_keys,
  updated_at = now()
returning *;

-- name: UpsertShadowWorkSignature :one
insert into work_signature_registry
  (project_id, candidate_id, shadow_run_id, mode, status, active,
   exact_signature_hash, signature_payload, conflict_bucket_keys,
   signature_version, owner, source_object_type, source_object_id)
values
  (sqlc.arg(project_id), sqlc.arg(candidate_id), sqlc.arg(shadow_run_id),
   'shadow', 'shadow_observed', false, sqlc.arg(exact_signature_hash),
   sqlc.arg(signature_payload)::jsonb, sqlc.arg(conflict_bucket_keys)::jsonb,
   sqlc.arg(signature_version), sqlc.narg(owner), sqlc.arg(source_object_type),
   sqlc.arg(source_object_id))
on conflict (candidate_id, mode) do update set
  shadow_run_id = excluded.shadow_run_id,
  status = 'shadow_observed',
  exact_signature_hash = excluded.exact_signature_hash,
  signature_payload = excluded.signature_payload,
  conflict_bucket_keys = excluded.conflict_bucket_keys,
  signature_version = excluded.signature_version,
  owner = excluded.owner,
  source_object_type = excluded.source_object_type,
  source_object_id = excluded.source_object_id,
  updated_at = now()
returning *;

-- name: DeleteShadowWorkSignatureForCandidate :exec
delete from work_signature_registry
where project_id = sqlc.arg(project_id)
  and candidate_id = sqlc.arg(candidate_id)
  and mode = 'shadow';

-- name: EnsureWorkConflictBucket :one
insert into work_conflict_buckets (project_id, bucket_key, bucket_version)
values (sqlc.arg(project_id), sqlc.arg(bucket_key), 0)
on conflict (project_id, bucket_key) do update set
  updated_at = work_conflict_buckets.updated_at
returning *;

-- name: ListActiveSEOOpportunitiesForDiscoveryShadow :many
select * from seo_opportunities
where project_id = $1
  and status in ('open','accepted','converted','dismissed','snoozed','watching')
order by created_at asc;

-- name: ListActiveDoctorFindingsForDiscoveryShadow :many
select * from seo_doctor_findings
where project_id = $1
  and status in ('active','dismissed','converted')
order by created_at asc;

-- name: GetLatestDiscoveryShadowRun :one
select * from discovery_shadow_runs
where project_id = $1
order by created_at desc
limit 1;

-- name: ListDiscoveryShadowSignaturesForRun :many
select
  r.id,
  r.project_id,
  r.candidate_id,
  r.exact_signature_hash,
  r.conflict_bucket_keys,
  r.signature_version,
  r.owner,
  r.source_object_type,
  r.source_object_id,
  c.source_kind,
  c.status as candidate_status
from work_signature_registry r
join discovery_candidates c on c.id = r.candidate_id
where r.project_id = sqlc.arg(project_id)
  and r.shadow_run_id = sqlc.arg(shadow_run_id)
  and r.mode = 'shadow'
order by r.created_at asc;
