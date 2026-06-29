-- Speed up admin hard-delete paths. Deleting a user deletes their projects,
-- which cascades through project-scoped tables and channel/subscription links.

create index if not exists idx_projects_owner_id
  on projects (owner_id);

create index if not exists idx_product_profiles_project_id
  on product_profiles (project_id);

create index if not exists idx_topics_project_id
  on topics (project_id);

create index if not exists idx_articles_project_id
  on articles (project_id);

create index if not exists idx_generation_runs_project_id
  on generation_runs (project_id);

create index if not exists idx_notification_channels_project_id
  on notification_channels (project_id);

create index if not exists idx_notification_subscriptions_channel_id
  on notification_subscriptions (channel_id);

create index if not exists idx_notification_deliveries_project_id
  on notification_deliveries (project_id);

create index if not exists idx_notification_deliveries_subscription_id
  on notification_deliveries (subscription_id);

create index if not exists idx_notification_deliveries_channel_id
  on notification_deliveries (channel_id);

create index if not exists idx_content_actions_project_id
  on content_actions (project_id);

create index if not exists idx_seo_action_plans_project_id
  on seo_action_plans (project_id);

create index if not exists idx_seo_experiments_project_id
  on seo_experiments (project_id);

create index if not exists idx_guardrail_checks_project_id
  on guardrail_checks (project_id);

create index if not exists idx_rollback_records_project_id
  on rollback_records (project_id);

create index if not exists idx_autopilot_audit_events_project_id
  on autopilot_audit_events (project_id);

create index if not exists idx_geo_asset_briefs_opportunity_id
  on geo_asset_briefs (opportunity_id);

create index if not exists idx_workflow_events_project_id
  on workflow_events (project_id);
