set local lock_timeout = '5s';
set local statement_timeout = '30s';

alter table site_fixes validate constraint site_fixes_measurement_guardrails_supported_check;
alter table site_fix_measurements validate constraint site_fix_measurements_guardrails_supported_check;
alter table site_fix_measurement_handoff_outbox validate constraint site_fix_measurement_handoff_alert_lease_check;

update site_fix_measurement_handoff_outbox set occurred_at=least(next_attempt_at,created_at) where occurred_at is null;
alter table site_fix_measurement_handoff_outbox alter column occurred_at set not null;

update site_fix_measurement_checkpoints set next_attempt_at=scheduled_at where next_attempt_at is null;
alter table site_fix_measurement_checkpoints alter column next_attempt_at set not null;

reset statement_timeout;
reset lock_timeout;
