alter table seo_policies
  add column if not exists recovery_plan_acknowledged_at timestamptz,
  add column if not exists recovery_plan_acknowledged_by text;
