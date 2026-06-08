-- name: CreateSEOObjective :one
insert into seo_objectives
  (project_id, name, status, primary_metric, secondary_metrics, target_pages,
   target_topics, target_queries, time_horizon_days, budget_usd)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
returning *;

-- name: ListSEOObjectives :many
select * from seo_objectives
where project_id = $1
  and ($2::text = '' or status = $2)
order by created_at desc;

-- name: UpdateSEOObjectiveStatus :one
update seo_objectives set
  status = $3,
  updated_at = now()
where id = $1 and project_id = $2
returning *;

-- name: UpsertSEOPolicy :one
insert into seo_policies
  (project_id, autopilot_level, weekly_action_limit, monthly_budget_limit,
   allowed_action_types, blocked_url_patterns, requires_review_action_types,
   max_auto_changes_per_page_per_month, low_traffic_clicks_28d_threshold,
   low_traffic_impressions_28d_threshold, min_confidence_for_auto_publish,
   quiet_hours_start, quiet_hours_end, quiet_hours_timezone, quiet_hours_behavior,
   kill_switch_enabled, safe_mode_enabled, risk_classifier_version)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
on conflict (project_id) do update set
  autopilot_level = excluded.autopilot_level,
  weekly_action_limit = excluded.weekly_action_limit,
  monthly_budget_limit = excluded.monthly_budget_limit,
  allowed_action_types = excluded.allowed_action_types,
  blocked_url_patterns = excluded.blocked_url_patterns,
  requires_review_action_types = excluded.requires_review_action_types,
  max_auto_changes_per_page_per_month = excluded.max_auto_changes_per_page_per_month,
  low_traffic_clicks_28d_threshold = excluded.low_traffic_clicks_28d_threshold,
  low_traffic_impressions_28d_threshold = excluded.low_traffic_impressions_28d_threshold,
  min_confidence_for_auto_publish = excluded.min_confidence_for_auto_publish,
  quiet_hours_start = excluded.quiet_hours_start,
  quiet_hours_end = excluded.quiet_hours_end,
  quiet_hours_timezone = excluded.quiet_hours_timezone,
  quiet_hours_behavior = excluded.quiet_hours_behavior,
  kill_switch_enabled = excluded.kill_switch_enabled,
  safe_mode_enabled = excluded.safe_mode_enabled,
  risk_classifier_version = excluded.risk_classifier_version,
  updated_at = now()
returning *;

-- name: GetSEOPolicy :one
select * from seo_policies
where project_id = $1;

-- name: UpsertRiskClassificationRule :one
insert into risk_classification_rules
  (project_id, version, rules, created_by, retired_at)
values ($1, $2, $3, $4, $5)
on conflict (project_id, version) do update set
  rules = excluded.rules,
  retired_at = excluded.retired_at
returning *;

-- name: GetRiskClassificationRule :one
select * from risk_classification_rules
where project_id = $1 and version = $2;

-- name: InsertAutopilotRun :one
insert into autopilot_runs
  (project_id, objective_id, status, autopilot_level_snapshot, derived_mode,
   started_at, finished_at, input_snapshot, selected_actions, rejected_actions,
   guardrail_results, published_changes, cost_usd, error)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
returning *;

-- name: ListAutopilotRuns :many
select * from autopilot_runs
where project_id = $1
order by started_at desc
limit $2;

-- name: CreateSEOActionPlan :one
insert into seo_action_plans
  (project_id, autopilot_run_id, objective_id, plan_window_start, plan_window_end,
   status, actions, expected_impact, expected_effort, aggregate_risk,
   risk_classifier_version, approval_required)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
returning *;

-- name: ListSEOActionPlans :many
select * from seo_action_plans
where project_id = $1
order by created_at desc
limit $2;

-- name: EnterSafeMode :one
insert into safe_mode_events
  (project_id, reason, trigger_source, entered_by, related_run_id, related_action_id)
values ($1, $2, $3, $4, $5, $6)
on conflict (project_id) where exited_at is null do update set
  reason = excluded.reason,
  trigger_source = excluded.trigger_source,
  related_run_id = excluded.related_run_id,
  related_action_id = excluded.related_action_id
returning *;

-- name: ExitSafeMode :one
update safe_mode_events set
  exited_at = now(),
  exited_by = $3,
  exit_reason = $4
where id = $1 and project_id = $2 and exited_at is null
returning *;

-- name: GetOpenSafeModeEvent :one
select * from safe_mode_events
where project_id = $1 and exited_at is null
limit 1;

-- name: ListSafeModeEvents :many
select * from safe_mode_events
where project_id = $1
order by entered_at desc
limit $2;

-- name: InsertAutopilotAuditEvent :one
insert into autopilot_audit_events
  (project_id, actor, event_type, entity_type, entity_id, before_snapshot, after_snapshot)
values ($1, $2, $3, $4, $5, $6, $7)
returning *;
