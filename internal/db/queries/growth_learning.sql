-- name: RecordGrowthTerminalOutcome :exec
with inserted_terminal as (
  insert into growth_terminal_outcomes
    (project_id, candidate_id, opportunity_id, content_action_id, article_id, artifact_url,
     action_family, target_identity, audience, primary_metric, outcome_label, record_kind,
     terminal_reason, measurement_policy_version, baseline_snapshot, checkpoint_snapshot, outcome_snapshot)
  values (
    sqlc.arg(project_id),
    (select signature.candidate_id from work_signature_registry signature
     where signature.project_id = sqlc.arg(project_id)
       and signature.reserved_work_type = 'seo_opportunity'
       and signature.reserved_work_id = sqlc.arg(opportunity_id)
       and signature.owner = 'opportunities' and signature.mode = 'enforced'
     order by signature.active desc, signature.created_at desc limit 1),
    sqlc.arg(opportunity_id), sqlc.arg(content_action_id), sqlc.narg(article_id),
    coalesce((select article.canonical_url from articles article where article.id = sqlc.narg(article_id) and article.project_id = sqlc.arg(project_id)), sqlc.arg(artifact_url)),
    sqlc.arg(action_family), sqlc.arg(target_identity)::jsonb,
    sqlc.arg(audience)::jsonb, sqlc.arg(primary_metric), sqlc.arg(outcome_label),
    sqlc.arg(record_kind), sqlc.arg(terminal_reason), sqlc.arg(measurement_policy_version),
    sqlc.arg(baseline_snapshot)::jsonb, sqlc.arg(checkpoint_snapshot)::jsonb,
    sqlc.arg(outcome_snapshot)::jsonb
  )
  on conflict (project_id, content_action_id) do nothing
  returning *
), inserted_learning as (
  insert into growth_learnings
    (project_id, terminal_outcome_id, content_action_id, learning_summary, applicability)
  select project_id, id, content_action_id, sqlc.arg(learning_summary), sqlc.arg(applicability)::jsonb
  from inserted_terminal where record_kind = 'directional_learning'
  returning id
)
insert into measurement_quality_records
  (project_id, terminal_outcome_id, content_action_id, data_quality_state, quality_gaps, recommendation)
select project_id, id, content_action_id, sqlc.arg(data_quality_state),
  sqlc.arg(quality_gaps)::jsonb, sqlc.arg(quality_recommendation)
from inserted_terminal where record_kind = 'measurement_quality';

-- name: ListGrowthLearnings :many
select learning.*, terminal.opportunity_id, terminal.candidate_id, terminal.article_id,
  terminal.artifact_url, terminal.action_family, terminal.target_identity, terminal.audience,
  terminal.primary_metric, terminal.outcome_label, terminal.terminal_reason,
  terminal.measurement_policy_version, terminal.baseline_snapshot, terminal.checkpoint_snapshot,
  terminal.outcome_snapshot
from growth_learnings learning
join growth_terminal_outcomes terminal on terminal.id = learning.terminal_outcome_id
where learning.project_id = sqlc.arg(project_id)
order by learning.created_at desc
limit sqlc.arg(limit_rows);

-- name: ListApplicableGrowthLearnings :many
select learning.*, terminal.opportunity_id, terminal.candidate_id, terminal.article_id,
  terminal.artifact_url, terminal.action_family, terminal.target_identity, terminal.audience,
  terminal.primary_metric, terminal.outcome_label, terminal.terminal_reason,
  terminal.measurement_policy_version, terminal.baseline_snapshot, terminal.checkpoint_snapshot,
  terminal.outcome_snapshot
from growth_learnings learning
join growth_terminal_outcomes terminal on terminal.id = learning.terminal_outcome_id
where learning.project_id = sqlc.arg(project_id)
  and learning.scoring_eligible = true
  and lower(btrim(terminal.action_family)) = lower(btrim(sqlc.arg(action_family)::text))
  and lower(btrim(terminal.primary_metric)) = lower(btrim(sqlc.arg(primary_metric)::text))
  and exists (
    select 1
    from jsonb_each_text(terminal.target_identity) historical(key, value)
    join jsonb_each_text(sqlc.arg(target_identity)::jsonb) candidate(key, value)
      on lower(rtrim(btrim(historical.value), '/')) = lower(rtrim(btrim(candidate.value), '/'))
  )
  and exists (
    select 1
    from jsonb_array_elements_text(terminal.audience) historical(value)
    join jsonb_array_elements_text(sqlc.arg(audience)::jsonb) candidate(value)
      on lower(btrim(historical.value)) = lower(btrim(candidate.value))
  )
order by learning.created_at desc
limit sqlc.arg(limit_rows);

-- name: ListMeasurementQualityRecords :many
select quality.*, terminal.opportunity_id, terminal.candidate_id, terminal.article_id,
  terminal.artifact_url, terminal.action_family, terminal.target_identity, terminal.audience,
  terminal.primary_metric, terminal.outcome_label, terminal.terminal_reason,
  terminal.measurement_policy_version, terminal.baseline_snapshot, terminal.checkpoint_snapshot,
  terminal.outcome_snapshot
from measurement_quality_records quality
join growth_terminal_outcomes terminal on terminal.id = quality.terminal_outcome_id
where quality.project_id = sqlc.arg(project_id)
order by quality.created_at desc
limit sqlc.arg(limit_rows);
