set local lock_timeout = '5s';
set local statement_timeout = '30s';

alter table site_fixes
  validate constraint site_fixes_measurement_plan_snapshot_json_check,
  validate constraint site_fixes_measurement_plan_alignment_check;
