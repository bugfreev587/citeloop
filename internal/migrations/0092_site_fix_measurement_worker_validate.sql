set local lock_timeout = '5s';
set local statement_timeout = '30s';

alter table site_fixes validate constraint site_fixes_measurement_guardrails_supported_check;
alter table site_fix_measurements validate constraint site_fix_measurements_guardrails_supported_check;

reset statement_timeout;
reset lock_timeout;
