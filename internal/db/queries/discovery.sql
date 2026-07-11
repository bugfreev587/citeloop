-- name: CreateDiscoveryShadowRun :one
insert into discovery_shadow_runs
  (project_id, candidate_schema_version, signature_version)
values
  (sqlc.arg(project_id), sqlc.arg(candidate_schema_version), sqlc.arg(signature_version))
returning *;

-- name: EnsureCanonicalDiscoveryRun :one
insert into discovery_shadow_runs
  (id, project_id, mode, status, candidate_schema_version, signature_version,
   doctor_candidates, identity_ready)
values
  (sqlc.arg(id), sqlc.arg(project_id), 'canonical', 'running',
   sqlc.arg(candidate_schema_version), sqlc.arg(signature_version), 1, 1)
on conflict (id) do update set
  updated_at = now()
where discovery_shadow_runs.project_id = excluded.project_id
  and discovery_shadow_runs.mode = 'canonical'
  and discovery_shadow_runs.candidate_schema_version = excluded.candidate_schema_version
  and discovery_shadow_runs.signature_version = excluded.signature_version
returning *;

-- name: CompleteCanonicalDiscoveryRun :one
update discovery_shadow_runs
set status = 'completed',
    error = null,
    finished_at = coalesce(finished_at, now()),
    updated_at = now()
where id = sqlc.arg(id)
  and project_id = sqlc.arg(project_id)
  and mode = 'canonical'
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
  candidate_version = discovery_candidates.candidate_version + case when (
    discovery_candidates.target_kind,
    discovery_candidates.normalized_target_set,
    discovery_candidates.issue_or_hypothesis_family,
    discovery_candidates.change_family,
    discovery_candidates.proposed_mutations,
    discovery_candidates.artifact_intent,
    discovery_candidates.intended_slug_or_canonical,
    discovery_candidates.topic_entity_identity,
    discovery_candidates.audience_identity,
    discovery_candidates.primary_success_metric,
    discovery_candidates.verification_mode,
    discovery_candidates.evidence_ids,
    discovery_candidates.evidence_fingerprint,
    discovery_candidates.suggested_owner,
    discovery_candidates.confidence,
    discovery_candidates.status,
    discovery_candidates.hold_reason,
    discovery_candidates.exact_signature_hash,
    discovery_candidates.signature_payload,
    discovery_candidates.conflict_bucket_keys
  ) is distinct from (
    excluded.target_kind,
    excluded.normalized_target_set,
    excluded.issue_or_hypothesis_family,
    excluded.change_family,
    excluded.proposed_mutations,
    excluded.artifact_intent,
    excluded.intended_slug_or_canonical,
    excluded.topic_entity_identity,
    excluded.audience_identity,
    excluded.primary_success_metric,
    excluded.verification_mode,
    excluded.evidence_ids,
    excluded.evidence_fingerprint,
    excluded.suggested_owner,
    excluded.confidence,
    excluded.status,
    excluded.hold_reason,
    excluded.exact_signature_hash,
    excluded.signature_payload,
    excluded.conflict_bucket_keys
  ) then 1 else 0 end,
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
on conflict (candidate_id) where mode in ('shadow') do update set
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
select seo_opportunities.* from seo_opportunities
where seo_opportunities.project_id = $1
  and seo_opportunities.status in ('open','accepted','converted','dismissed','snoozed','watching')
  and not exists (
    select 1 from growth_opportunity_work_aliases alias
    where alias.project_id = seo_opportunities.project_id
      and alias.legacy_opportunity_id = seo_opportunities.id
      and alias.disposition in ('duplicate','doctor_merge')
  )
order by seo_opportunities.created_at asc;

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
  provider = coalesce(sqlc.narg(resolved_provider), provider),
  model = coalesce(sqlc.narg(resolved_model), model),
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

-- name: FinishAICallRecordIfRunning :one
update ai_call_records set
  status = 'failed', error_code = sqlc.arg(error_code), finished_at = now(), updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id) and status = 'running'
returning *;

-- name: FinishCanonicalAICallFenced :one
update ai_call_records set
  status = case when status = 'running' then sqlc.arg(status) else status end,
  error_code = case when status = 'running' then sqlc.narg(error_code) else error_code end,
  provider = coalesce(sqlc.narg(resolved_provider), provider),
  model = coalesce(sqlc.narg(resolved_model), model),
  prompt_tokens = sqlc.arg(prompt_tokens),
  completion_tokens = sqlc.arg(completion_tokens),
  total_tokens = sqlc.arg(total_tokens),
  cost_usd = sqlc.arg(cost_usd),
  finished_at = coalesce(finished_at, now()),
  updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;

-- name: GetAICallRecord :one
select * from ai_call_records where id = sqlc.arg(id) and project_id = sqlc.arg(project_id);

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

-- name: ListSnapshotReviewAliases :many
select a.* from work_signature_aliases a
join work_review_memory m on m.id = a.review_memory_id
where a.project_id = sqlc.arg(project_id)
  and m.project_id = sqlc.arg(project_id)
  and m.active = true
  and m.conflict_bucket_keys ?| sqlc.arg(bucket_keys)::text[]
order by a.id asc;

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

-- name: GetArbitrationDecision :one
select * from discovery_arbitration_decisions
where project_id = sqlc.arg(project_id)
  and id = sqlc.arg(id);

-- name: LockArbitrationDecisionForReserve :one
select * from discovery_arbitration_decisions
where project_id = sqlc.arg(project_id)
  and id = sqlc.arg(id)
for update;

-- name: LockDiscoveryCandidateForReserve :one
select candidate.*
from discovery_candidates candidate
join discovery_shadow_runs run
  on run.id = candidate.shadow_run_id
 and run.project_id = candidate.project_id
where candidate.project_id = sqlc.arg(project_id)
  and candidate.id = sqlc.arg(candidate_id)
  and run.mode = 'canonical'
  and run.status = 'completed'
for update of candidate, run;

-- name: GetSEODoctorFindingForSiteFixForUpdate :one
select * from seo_doctor_findings
where project_id = sqlc.arg(project_id)
  and id = sqlc.arg(finding_id)
for update;

-- name: GetDiscoveryCandidateForArbitration :one
select candidate.*
from discovery_candidates candidate
join discovery_shadow_runs run
  on run.id = candidate.shadow_run_id
 and run.project_id = candidate.project_id
where candidate.project_id = sqlc.arg(project_id)
  and candidate.id = sqlc.arg(candidate_id)
  and run.mode = 'canonical'
  and run.status = 'completed';

-- name: LockConflictBucketsForReserve :many
select * from work_conflict_buckets
where project_id = sqlc.arg(project_id)
  and bucket_key = any(sqlc.arg(bucket_keys)::text[])
order by bucket_key asc
for update;

-- name: InsertEnforcedWorkSignature :one
insert into work_signature_registry
  (id, project_id, candidate_id, shadow_run_id, mode, status, active,
   exact_signature_hash, signature_payload, semantic_fingerprint,
   conflict_bucket_keys, signature_version, owner,
   source_object_type, source_object_id, arbitration_decision_id,
   reserved_work_type, reserved_work_id, evidence_fingerprint)
values
  (sqlc.arg(id), sqlc.arg(project_id), sqlc.arg(candidate_id), sqlc.arg(shadow_run_id),
   'enforced', sqlc.arg(status), true, sqlc.arg(exact_signature_hash),
   sqlc.arg(signature_payload)::jsonb, sqlc.arg(semantic_fingerprint),
   sqlc.arg(conflict_bucket_keys)::jsonb, sqlc.arg(signature_version),
   sqlc.arg(owner), sqlc.arg(source_object_type), sqlc.arg(source_object_id),
   sqlc.arg(arbitration_decision_id), sqlc.arg(reserved_work_type),
   sqlc.arg(reserved_work_id), sqlc.arg(evidence_fingerprint))
returning *;

-- name: IncrementConflictBucketVersions :many
update work_conflict_buckets
set bucket_version = bucket_version + 1,
    updated_at = now()
where project_id = sqlc.arg(project_id)
  and bucket_key = any(sqlc.arg(bucket_keys)::text[])
returning *;

-- name: UpsertWorkRelationship :one
insert into work_relationships
  (project_id, dependent_candidate_id, dependent_work_signature_id,
   dependent_work_type, dependent_work_id, blocking_work_signature_id,
   relationship_type, dependency_class, reason,
   overlapping_mutation_fields, reassessment_trigger, active)
values
  (sqlc.arg(project_id), sqlc.arg(dependent_candidate_id), sqlc.arg(dependent_work_signature_id),
   sqlc.arg(dependent_work_type), sqlc.arg(dependent_work_id), sqlc.arg(blocking_work_signature_id),
   sqlc.arg(relationship_type), sqlc.arg(dependency_class), sqlc.arg(reason),
   sqlc.arg(overlapping_mutation_fields)::jsonb, sqlc.arg(reassessment_trigger), true)
on conflict (project_id, dependent_work_signature_id, blocking_work_signature_id, relationship_type)
do update set
  dependency_class = excluded.dependency_class,
  reason = excluded.reason,
  overlapping_mutation_fields = excluded.overlapping_mutation_fields,
  reassessment_trigger = excluded.reassessment_trigger,
  active = true,
  resolved_at = null,
  updated_at = now()
returning *;

-- name: GrowthOpportunityExecutionGuard :one
select case when exists (
  select 1 from product_writer_authority authority
  where authority.project_id = sqlc.arg(project_id)
    and authority.product = 'opportunities'
    and authority.writer_authority = 'legacy'
    and authority.write_fenced = false
) then true else exists (
  select 1
  from work_signature_registry signature
  where signature.project_id = sqlc.arg(project_id)
    and signature.mode = 'enforced'
    and signature.active = true
    and signature.owner = 'opportunities'
    and signature.reserved_work_type = 'seo_opportunity'
    and signature.reserved_work_id = sqlc.arg(opportunity_id)
    and not exists (
      select 1 from growth_opportunity_work_aliases alias
      where alias.project_id = signature.project_id
        and alias.legacy_opportunity_id = sqlc.arg(opportunity_id)
        and alias.disposition in ('duplicate','doctor_merge')
    )
    and not exists (
      select 1
      from work_relationships relationship
      where relationship.project_id = signature.project_id
        and relationship.dependent_work_signature_id = signature.id
        and relationship.dependency_class = 'hard_blocker'
        and relationship.active = true
    )
) end::boolean as can_execute;

-- name: ListWorkSignaturesForRelationship :many
select * from work_signature_registry
where project_id = sqlc.arg(project_id)
  and id = any(sqlc.arg(signature_ids)::uuid[])
  and mode = 'enforced'
  and active = true
order by id;

-- name: MarkArbitrationDecisionReserved :one
update discovery_arbitration_decisions
set status = 'reserved',
    updated_at = now()
where project_id = sqlc.arg(project_id)
  and id = sqlc.arg(id)
  and status = 'prepared'
returning *;

-- name: MarkArbitrationDecisionResolved :one
update discovery_arbitration_decisions
set status = 'resolved', updated_at = now()
where project_id = sqlc.arg(project_id)
  and id = sqlc.arg(id)
  and status = 'prepared'
returning *;

-- name: GetDiscoveryArbitrationConfig :one
select * from discovery_arbitration_configs
where project_id = sqlc.arg(project_id);

-- name: LockDiscoveryReviewItemForResolve :one
select * from discovery_review_items
where project_id = sqlc.arg(project_id)
  and candidate_id = sqlc.arg(candidate_id)
for update;

-- name: ResolveDiscoveryReviewItem :one
update discovery_review_items set
  state = 'resolved',
  resolution = sqlc.arg(resolution)::jsonb,
  resolved_by = sqlc.arg(resolved_by),
  resolved_at = now(),
  arbitration_decision_id = sqlc.arg(arbitration_decision_id),
  updated_at = now()
where project_id = sqlc.arg(project_id)
  and candidate_id = sqlc.arg(candidate_id)
  and state <> 'resolved'
returning *;

-- name: DeactivateWorkReviewMemory :one
update work_review_memory set
  active = false,
  updated_at = now()
where project_id = sqlc.arg(project_id)
  and id = sqlc.arg(id)
  and active = true
returning *;

-- name: ListDiscoverySemanticGoldCases :many
select * from discovery_semantic_gold_cases
where project_id = sqlc.arg(project_id)
  and dataset_version = sqlc.arg(dataset_version)
order by id asc;

-- name: CreateDiscoverySemanticEvaluation :one
insert into discovery_semantic_evaluations
  (project_id, dataset_version, confidence_threshold,
   duplicate_safety_recall_target, false_suppression_rate_target,
   total_cases, duplicate_safety_cases, distinct_cases,
   duplicate_safety_recall, false_suppression_rate, comparator_coverage,
   automated_disposition_coverage, hold_rate, threshold_backlog,
   weekly_ops_capacity, launch_ready, automatic_suppression_enabled,
   blockers, evaluated_by)
values
  (sqlc.arg(project_id), sqlc.arg(dataset_version), sqlc.arg(confidence_threshold),
   sqlc.arg(duplicate_safety_recall_target), sqlc.arg(false_suppression_rate_target),
   sqlc.arg(total_cases), sqlc.arg(duplicate_safety_cases), sqlc.arg(distinct_cases),
   sqlc.arg(duplicate_safety_recall), sqlc.arg(false_suppression_rate), sqlc.arg(comparator_coverage),
   sqlc.arg(automated_disposition_coverage), sqlc.arg(hold_rate), sqlc.arg(threshold_backlog),
   sqlc.arg(weekly_ops_capacity), sqlc.arg(launch_ready), sqlc.arg(automatic_suppression_enabled),
   sqlc.arg(blockers)::jsonb, sqlc.arg(evaluated_by))
returning *;

-- name: UpsertDiscoveryArbitrationEvaluationConfig :one
insert into discovery_arbitration_configs
  (project_id, confidence_threshold, duplicate_safety_recall_target,
   false_suppression_rate_target, gold_dataset_version, weekly_ops_capacity,
   launch_ready, automatic_suppression_enabled, evaluated_at)
values
  (sqlc.arg(project_id), sqlc.arg(confidence_threshold),
   sqlc.arg(duplicate_safety_recall_target), sqlc.arg(false_suppression_rate_target),
   sqlc.arg(gold_dataset_version), sqlc.arg(weekly_ops_capacity),
   sqlc.arg(launch_ready), sqlc.arg(automatic_suppression_enabled), now())
on conflict (project_id) do update set
  confidence_threshold = excluded.confidence_threshold,
  duplicate_safety_recall_target = excluded.duplicate_safety_recall_target,
  false_suppression_rate_target = excluded.false_suppression_rate_target,
  gold_dataset_version = excluded.gold_dataset_version,
  weekly_ops_capacity = excluded.weekly_ops_capacity,
  launch_ready = excluded.launch_ready,
  automatic_suppression_enabled = excluded.automatic_suppression_enabled,
  evaluated_at = excluded.evaluated_at,
  updated_at = now()
returning *;

-- name: GetLatestDiscoverySemanticEvaluation :one
select * from discovery_semantic_evaluations
where project_id = sqlc.arg(project_id)
order by created_at desc
limit 1;

-- name: GetEnforcedWorkSignatureForReservedWork :one
select * from work_signature_registry
where project_id = sqlc.arg(project_id)
  and mode = 'enforced'
  and reserved_work_type = sqlc.arg(reserved_work_type)
  and reserved_work_id = sqlc.arg(reserved_work_id)
order by created_at desc
limit 1;

-- name: MarkCanonicalWorkSignatureMigrationRolledBack :one
with signature_snapshot as materialized (
  select w.id, w.project_id, w.status as expected_status,
         w.conflict_bucket_keys
  from work_signature_registry w
  where w.project_id = sqlc.arg(project_id)
    and w.id = sqlc.arg(id)
    and w.mode = 'enforced'
    and w.active = true
    and w.reserved_work_type = 'site_fix'
    and w.status in ('reserved','proposed','approved','preparing','executing','awaiting_deploy','failed_retryable')
), expected_keys as materialized (
  select distinct keys.bucket_key
  from signature_snapshot s
  cross join lateral jsonb_array_elements_text(s.conflict_bucket_keys) keys(bucket_key)
  order by keys.bucket_key
), locked_buckets as materialized (
  select b.id, b.bucket_key
  from work_conflict_buckets b
  join expected_keys keys on keys.bucket_key = b.bucket_key
  where b.project_id = sqlc.arg(project_id)
  order by b.bucket_key
  for update of b
), locked_signature as materialized (
  select w.id, w.project_id
  from work_signature_registry w
  join signature_snapshot s on s.id = w.id and s.project_id = w.project_id
  where w.mode = 'enforced'
    and w.active = true
    and w.status = s.expected_status
    and w.reserved_work_type = 'site_fix'
    and w.conflict_bucket_keys = s.conflict_bucket_keys
    and jsonb_array_length(s.conflict_bucket_keys) > 0
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  for update of w
), bumped as (
  update work_conflict_buckets b
  set bucket_version = bucket_version + 1, updated_at = now()
  from locked_buckets locked
  where b.id = locked.id
    and exists (select 1 from locked_signature)
    and (select count(*) from locked_buckets) =
        (select count(*) from expected_keys)
  returning b.id
)
update work_signature_registry w
set status = 'cancelled', active = false, updated_at = now()
from locked_signature locked
where w.id = locked.id
  and w.project_id = locked.project_id
  and w.active = true
  and (select count(*) from bumped) =
      (select count(*) from expected_keys)
returning w.*;
