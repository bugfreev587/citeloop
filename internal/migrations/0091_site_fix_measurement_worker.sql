-- Permit one atomic null -> completed checkpoint transition while retaining an
-- immutable schedule and append-only computed evidence.

set local lock_timeout = '5s';
set local statement_timeout = '30s';

create or replace function site_fix_measurement_guardrails_supported_v1(primary_metric text, policy jsonb)
returns boolean language plpgsql immutable as $$
declare guardrail jsonb; metric text; allowed text;
begin
  case lower(btrim(primary_metric))
    when 'ctr' then allowed := 'gsc_impressions';
    when 'gsc_ctr' then allowed := 'gsc_impressions';
    when 'clicks' then allowed := 'gsc_impressions';
    when 'gsc_clicks' then allowed := 'gsc_impressions';
    when 'position' then allowed := 'gsc_impressions';
    when 'gsc_position' then allowed := 'gsc_impressions';
    when 'conversion_rate' then allowed := 'ga4_sessions';
    when 'ga4_conversion_rate' then allowed := 'ga4_sessions';
    when 'qualified_actions' then allowed := 'ga4_sessions';
    when 'ga4_key_events' then allowed := 'ga4_sessions';
    when 'citations' then allowed := 'ai_brand_mention_rate';
    when 'ai_citation_count' then allowed := 'ai_brand_mention_rate';
    else allowed := null;
  end case;
  for guardrail in select value from jsonb_array_elements(coalesce(policy->'guardrails', '[]'::jsonb)) loop
    metric := lower(btrim(guardrail->>'metric'));
    if metric = 'impressions' then metric := 'gsc_impressions'; end if;
    if metric = 'referral_sessions' then metric := 'ga4_sessions'; end if;
    if metric = 'brand_mentions' then metric := 'ai_brand_mention_rate'; end if;
    if allowed is null or metric <> allowed then return false; end if;
  end loop;
  return true;
exception when others then return false;
end;
$$;

alter table site_fixes
  drop constraint if exists site_fixes_measurement_guardrails_supported_check,
  add constraint site_fixes_measurement_guardrails_supported_check check (
    measurement_policy <> 'measurement_required'
    or site_fix_measurement_guardrails_supported_v1(primary_metric, measurement_policy_snapshot)
  ) not valid;

alter table site_fix_measurements
  drop constraint if exists site_fix_measurements_guardrails_supported_check,
  add constraint site_fix_measurements_guardrails_supported_check check (
    site_fix_measurement_guardrails_supported_v1(primary_metric, measurement_policy_snapshot)
  ) not valid;

create or replace function prevent_site_fix_measurement_checkpoint_mutation()
returns trigger language plpgsql as $$
begin
  if tg_op = 'DELETE' then
    raise exception 'Site Fix measurement checkpoint result is immutable after completion' using errcode = '23514';
  end if;
  if new.id is distinct from old.id
    or new.created_at is distinct from old.created_at
    or new.project_id is distinct from old.project_id
    or new.measurement_id is distinct from old.measurement_id
    or new.checkpoint_key is distinct from old.checkpoint_key
    or new.checkpoint_role is distinct from old.checkpoint_role
    or new.scheduled_at is distinct from old.scheduled_at
    or new.window_start is distinct from old.window_start
    or new.window_end is distinct from old.window_end
    or new.attempt_number is distinct from old.attempt_number
    or new.required_data_sources is distinct from old.required_data_sources
    or new.minimum_sample is distinct from old.minimum_sample then
    raise exception 'Site Fix measurement checkpoint schedule is immutable' using errcode = '23514';
  end if;
  if old.computed_at is null and new.computed_at is not null then
    return new;
  end if;
  if to_jsonb(new) = to_jsonb(old) then
    return new;
  end if;
  raise exception 'Site Fix measurement checkpoint result is immutable after completion' using errcode = '23514';
end;
$$;

drop trigger if exists site_fix_measurement_checkpoints_immutable on site_fix_measurement_checkpoints;
create trigger site_fix_measurement_checkpoints_immutable
before update or delete on site_fix_measurement_checkpoints
for each row execute function prevent_site_fix_measurement_checkpoint_mutation();

reset statement_timeout;
reset lock_timeout;
