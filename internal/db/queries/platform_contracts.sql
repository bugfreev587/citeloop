-- name: ListActivePlatformContentContracts :many
select * from platform_content_contracts
where status = 'active'
order by platform;

-- name: GetActivePlatformContentContract :one
select * from platform_content_contracts
where platform = $1 and status = 'active';

-- name: ListPlatformTargetContexts :many
select * from platform_target_contexts
where project_id = sqlc.arg(project_id)
  and (sqlc.arg(platform)::text = '' or platform = sqlc.arg(platform))
order by platform, target_key, version desc;

-- name: GetPlatformTargetContextForProject :one
select * from platform_target_contexts
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id);

-- name: GetCurrentPlatformTargetContext :one
select * from platform_target_contexts
where project_id = sqlc.arg(project_id)
  and platform = sqlc.arg(platform)
  and target_key = sqlc.arg(target_key)
  and status = 'confirmed'
  and expires_at > now();

-- name: NextPlatformTargetContextVersion :one
select coalesce(max(version), 0)::int + 1
from platform_target_contexts
where project_id = sqlc.arg(project_id)
  and platform = sqlc.arg(platform)
  and target_key = sqlc.arg(target_key);

-- name: CreatePlatformTargetContext :one
insert into platform_target_contexts (
  project_id, platform, target_key, version, status, source_kind,
  source_url, rules_url, rules_text, allowed_post_types, required_flair,
  link_policy, self_promotion_policy, disclosure_requirements, notes,
  content_hash, supersedes_context_id
) values (
  sqlc.arg(project_id), sqlc.arg(platform), sqlc.arg(target_key), sqlc.arg(version),
  'draft', sqlc.arg(source_kind), sqlc.narg(source_url), sqlc.narg(rules_url),
  sqlc.arg(rules_text), sqlc.arg(allowed_post_types), sqlc.narg(required_flair),
  sqlc.arg(link_policy), sqlc.arg(self_promotion_policy), sqlc.arg(disclosure_requirements),
  sqlc.arg(notes), sqlc.arg(content_hash), sqlc.narg(supersedes_context_id)
)
returning *;

-- name: SupersedeCurrentPlatformTargetContext :exec
update platform_target_contexts
set status = 'superseded', updated_at = now()
where project_id = sqlc.arg(project_id)
  and platform = sqlc.arg(platform)
  and target_key = sqlc.arg(target_key)
  and status = 'confirmed';

-- name: ConfirmPlatformTargetContext :one
update platform_target_contexts
set status = 'confirmed', confirmed_by = sqlc.arg(confirmed_by),
    confirmed_at = sqlc.arg(confirmed_at), expires_at = sqlc.arg(expires_at),
    updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id) and status = 'draft'
returning *;

-- name: ExpirePlatformTargetContexts :many
update platform_target_contexts
set status = 'expired', updated_at = now()
where status = 'confirmed' and expires_at <= sqlc.arg(now_at)
returning *;

-- name: CreateContentTargetPlan :one
insert into content_target_plans (
  project_id, opportunity_id, content_action_id, asset_type,
  canonical_target, selection_mode, status, capability_snapshot
) values (
  sqlc.arg(project_id), sqlc.narg(opportunity_id), sqlc.narg(content_action_id),
  sqlc.arg(asset_type), sqlc.arg(canonical_target), sqlc.arg(selection_mode),
  sqlc.arg(status), sqlc.arg(capability_snapshot)
)
returning *;

-- name: GetContentTargetPlanForProject :one
select * from content_target_plans
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id);

-- name: GetActiveContentTargetPlanByOpportunity :one
select * from content_target_plans
where project_id = sqlc.arg(project_id)
  and opportunity_id = sqlc.arg(opportunity_id)
  and status <> 'archived';

-- name: CreateContentTargetPlanItem :one
insert into content_target_plan_items (
  plan_id, ordinal, platform, target_key, output_type, is_canonical,
  platform_contract_id, platform_contract_version, target_context_id,
  target_context_version, rationale, status
) values (
  sqlc.arg(plan_id), sqlc.arg(ordinal), sqlc.arg(platform), sqlc.arg(target_key),
  sqlc.arg(output_type), sqlc.arg(is_canonical), sqlc.narg(platform_contract_id),
  sqlc.arg(platform_contract_version), sqlc.narg(target_context_id),
  sqlc.narg(target_context_version), sqlc.arg(rationale), sqlc.arg(status)
)
returning *;

-- name: ListContentTargetPlanItems :many
select * from content_target_plan_items
where plan_id = $1
order by ordinal, id;

-- name: UpdateContentTargetPlanStatus :one
update content_target_plans
set status = sqlc.arg(status), updated_at = now()
where id = sqlc.arg(id) and project_id = sqlc.arg(project_id)
returning *;
