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
  candidate_version = discovery_candidates.candidate_version + 1,
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

-- name: CreateAICallRecord :one
insert into ai_call_records
  (project_id, run_id, stage, linked_object_type, linked_object_id,
   provider, model, prompt_version, request_fingerprint, status, parent_call_id)
values
  (sqlc.arg(project_id), sqlc.narg(run_id), sqlc.arg(stage),
   sqlc.arg(linked_object_type), sqlc.arg(linked_object_id),
   sqlc.arg(provider), sqlc.arg(model), sqlc.arg(prompt_version),
   sqlc.arg(request_fingerprint), sqlc.arg(status), sqlc.narg(parent_call_id))
returning *;

-- name: FinishAICallRecord :one
update ai_call_records set
  status = sqlc.arg(status),
  error_code = sqlc.narg(error_code),
  prompt_tokens = sqlc.arg(prompt_tokens),
  completion_tokens = sqlc.arg(completion_tokens),
  total_tokens = sqlc.arg(total_tokens),
  cost_usd = sqlc.arg(cost_usd),
  finished_at = now(),
  updated_at = now()
where id = sqlc.arg(id)
  and project_id = sqlc.arg(project_id)
returning *;

-- name: CreateArbitrationDecision :one
insert into discovery_arbitration_decisions
  (project_id, candidate_id, candidate_version, ai_call_id, disposition,
   decision, owner, overlap_work_ids, reason, confidence,
   semantic_fingerprint, compared_work_ids, expected_bucket_versions,
   snapshot_fingerprint, exact_signature_hash, signature_version,
   evidence_fingerprint, rules_version, prompt_version, provider, model, status)
values
  (sqlc.arg(project_id), sqlc.arg(candidate_id), sqlc.arg(candidate_version),
   sqlc.narg(ai_call_id), sqlc.arg(disposition), sqlc.arg(decision),
   sqlc.narg(owner), sqlc.arg(overlap_work_ids)::jsonb, sqlc.arg(reason),
   sqlc.arg(confidence), sqlc.arg(semantic_fingerprint),
   sqlc.arg(compared_work_ids)::jsonb, sqlc.arg(expected_bucket_versions)::jsonb,
   sqlc.arg(snapshot_fingerprint), sqlc.arg(exact_signature_hash),
   sqlc.arg(signature_version), sqlc.arg(evidence_fingerprint),
   sqlc.arg(rules_version), sqlc.arg(prompt_version), sqlc.arg(provider),
   sqlc.arg(model), sqlc.arg(status))
returning *;

-- name: UpdateArbitrationDecisionStatus :one
update discovery_arbitration_decisions set
  status = sqlc.arg(status),
  updated_at = now()
where id = sqlc.arg(id)
  and project_id = sqlc.arg(project_id)
returning *;

-- name: MaterializeConflictBuckets :many
insert into work_conflict_buckets (project_id, bucket_key, bucket_version)
select sqlc.arg(project_id), keys.bucket_key, 0
from unnest(sqlc.arg(bucket_keys)::text[]) as keys(bucket_key)
on conflict (project_id, bucket_key) do update set
  updated_at = work_conflict_buckets.updated_at
returning *;

-- name: GetConflictBucketSnapshot :many
select * from work_conflict_buckets
where project_id = sqlc.arg(project_id)
  and bucket_key = any(sqlc.arg(bucket_keys)::text[])
order by bucket_key asc;

-- name: ListSnapshotActiveSignatures :many
select * from work_signature_registry
where project_id = sqlc.arg(project_id)
  and mode = 'enforced'
  and active = true
  and conflict_bucket_keys ?| sqlc.arg(bucket_keys)::text[]
order by id asc;

-- name: ListSnapshotReviewMemory :many
select * from work_review_memory
where project_id = sqlc.arg(project_id)
  and active = true
  and conflict_bucket_keys ?| sqlc.arg(bucket_keys)::text[]
order by id asc;

-- name: GetDiscoveryCandidateForReview :one
select * from discovery_candidates
where project_id = sqlc.arg(project_id)
  and id = sqlc.arg(candidate_id);

-- name: UpsertDiscoveryReviewItem :one
insert into discovery_review_items
  (project_id, candidate_id, state, reason, assignee,
   expected_bucket_versions, expected_candidate_version,
   internal_owner, due_at, arbitration_decision_id)
values
  (sqlc.arg(project_id), sqlc.arg(candidate_id), sqlc.arg(state),
   sqlc.arg(reason), sqlc.narg(assignee),
   sqlc.arg(expected_bucket_versions)::jsonb, sqlc.arg(expected_candidate_version),
   sqlc.arg(internal_owner), sqlc.narg(due_at), sqlc.narg(arbitration_decision_id))
on conflict (candidate_id) do update set
  state = excluded.state,
  reason = excluded.reason,
  assignee = excluded.assignee,
  expected_bucket_versions = excluded.expected_bucket_versions,
  expected_candidate_version = excluded.expected_candidate_version,
  internal_owner = excluded.internal_owner,
  due_at = excluded.due_at,
  arbitration_decision_id = excluded.arbitration_decision_id,
  resolution = null,
  resolved_by = null,
  resolved_at = null,
  updated_at = now()
returning *;

-- name: ListDiscoveryReviewItems :many
select * from discovery_review_items
where project_id = sqlc.arg(project_id)
  and (sqlc.narg(state)::text is null or state = sqlc.narg(state)::text)
  and (sqlc.narg(assignee)::text is null or assignee = sqlc.narg(assignee)::text)
  and created_at <= now() - (sqlc.arg(min_age_seconds)::bigint * interval '1 second')
order by created_at asc;

-- name: GetDiscoveryReviewItem :one
select * from discovery_review_items
where project_id = sqlc.arg(project_id)
  and candidate_id = sqlc.arg(candidate_id);

-- name: UpsertWorkReviewMemory :one
insert into work_review_memory
  (project_id, candidate_id, work_signature_id,
   exact_signature_hash_at_decision, semantic_fingerprint_at_decision,
   signature_payload, conflict_bucket_keys, signature_version, decision, decision_scope,
   evidence_fingerprint_at_decision, snoozed_until,
   material_change_policy_version, decided_by, decided_at, active)
values
  (sqlc.arg(project_id), sqlc.narg(candidate_id), sqlc.narg(work_signature_id),
   sqlc.arg(exact_signature_hash_at_decision),
   sqlc.arg(semantic_fingerprint_at_decision),
   sqlc.arg(signature_payload)::jsonb, sqlc.arg(conflict_bucket_keys)::jsonb, sqlc.arg(signature_version),
   sqlc.arg(decision), sqlc.arg(decision_scope)::jsonb,
   sqlc.arg(evidence_fingerprint_at_decision), sqlc.narg(snoozed_until),
   sqlc.arg(material_change_policy_version), sqlc.arg(decided_by),
   sqlc.arg(decided_at), sqlc.arg(active))
on conflict (project_id, exact_signature_hash_at_decision) where active = true
do update set
  candidate_id = excluded.candidate_id,
  work_signature_id = excluded.work_signature_id,
  semantic_fingerprint_at_decision = excluded.semantic_fingerprint_at_decision,
  signature_payload = excluded.signature_payload,
  conflict_bucket_keys = excluded.conflict_bucket_keys,
  signature_version = excluded.signature_version,
  decision = excluded.decision,
  decision_scope = excluded.decision_scope,
  evidence_fingerprint_at_decision = excluded.evidence_fingerprint_at_decision,
  snoozed_until = excluded.snoozed_until,
  material_change_policy_version = excluded.material_change_policy_version,
  decided_by = excluded.decided_by,
  decided_at = excluded.decided_at,
  updated_at = now()
returning *;

-- name: UpsertWorkSignatureAlias :one
insert into work_signature_aliases
  (project_id, review_memory_id, alias_exact_signature_hash,
   alias_semantic_fingerprint, alias_signature_version)
values
  (sqlc.arg(project_id), sqlc.arg(review_memory_id),
   sqlc.arg(alias_exact_signature_hash), sqlc.arg(alias_semantic_fingerprint),
   sqlc.arg(alias_signature_version))
on conflict (project_id, alias_exact_signature_hash, alias_signature_version)
do update set
  review_memory_id = excluded.review_memory_id,
  alias_semantic_fingerprint = excluded.alias_semantic_fingerprint
returning *;
