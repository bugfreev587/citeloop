-- Bind a finite, immutable measurement lifecycle to every Growth Action.

alter table content_actions
  add column if not exists measurement_policy_version text not null default 'legacy-v0',
  add column if not exists measurement_policy jsonb not null default '{}'::jsonb,
  add column if not exists measuring_started_at timestamptz,
  add column if not exists absolute_terminal_at timestamptz,
  add column if not exists measurement_terminal_reason text;

alter table action_measurements
  add column if not exists checkpoint_role text not null default 'legacy_unknown',
  add column if not exists measurement_policy_version text not null default 'legacy-v0',
  add column if not exists checkpoint_attempt int not null default 1,
  add column if not exists data_quality_state text not null default 'insufficient',
  add column if not exists source_freshness jsonb not null default '{}'::jsonb;

alter table content_actions
  drop constraint if exists content_actions_measurement_policy_json_check,
  add constraint content_actions_measurement_policy_json_check
    check (jsonb_typeof(measurement_policy) = 'object') not valid,
  drop constraint if exists content_actions_finite_measurement_check,
  add constraint content_actions_finite_measurement_check check (
    status <> 'measuring' or (
      measurement_policy_version = 'legacy-v0'
      and measurement_policy = '{}'::jsonb
      and measuring_started_at is null
      and absolute_terminal_at is null
    ) or (
      measuring_started_at is not null
      and absolute_terminal_at is not null
      and absolute_terminal_at >= measuring_started_at
      and nullif(btrim(measurement_policy_version), '') is not null
      and measurement_policy_version = measurement_policy->>'policy_version'
      and jsonb_typeof(measurement_policy) = 'object'
      and measurement_policy <> '{}'::jsonb
      and case when jsonb_typeof(measurement_policy->'max_measuring_duration_days') = 'number'
        then (measurement_policy->>'max_measuring_duration_days')::int between 1 and 365 else false end
      and case when jsonb_typeof(measurement_policy->'early_signal_offset_days') = 'number'
        then (measurement_policy->>'early_signal_offset_days')::int > 0 else false end
      and case when jsonb_typeof(measurement_policy->'primary_checkpoint_offset_days') = 'number'
        then (measurement_policy->>'primary_checkpoint_offset_days')::int
          > (measurement_policy->>'early_signal_offset_days')::int else false end
      and case when jsonb_typeof(measurement_policy->'max_follow_up_attempts') = 'number'
        then (measurement_policy->>'max_follow_up_attempts')::int between 0 and 4 else false end
      and case when jsonb_typeof(measurement_policy->'follow_up_offsets_days') = 'array'
        then jsonb_array_length(measurement_policy->'follow_up_offsets_days')
          <= (measurement_policy->>'max_follow_up_attempts')::int else false end
      and case when jsonb_typeof(measurement_policy->'terminalization_grace_period_days') = 'number'
        then (measurement_policy->>'terminalization_grace_period_days')::int between 0 and 30 else false end
      and absolute_terminal_at = measuring_started_at + (
        ((measurement_policy->>'max_measuring_duration_days')::int
          + (measurement_policy->>'terminalization_grace_period_days')::int) * interval '1 day'
      )
    )
  ) not valid;

alter table action_measurements
  drop constraint if exists action_measurements_checkpoint_role_check,
  add constraint action_measurements_checkpoint_role_check
    check (checkpoint_role in ('legacy_unknown','baseline','early','primary','follow_up')) not valid,
  drop constraint if exists action_measurements_checkpoint_attempt_check,
  add constraint action_measurements_checkpoint_attempt_check
    check (checkpoint_attempt between 1 and 4) not valid,
  drop constraint if exists action_measurements_data_quality_state_check,
  add constraint action_measurements_data_quality_state_check
    check (data_quality_state in ('complete','partial','insufficient','provider_unavailable','stale')) not valid,
  drop constraint if exists action_measurements_source_freshness_json_check,
  add constraint action_measurements_source_freshness_json_check
    check (jsonb_typeof(source_freshness) = 'object') not valid;

create or replace function bind_immutable_growth_measurement_policy()
returns trigger language plpgsql as $$
declare
  selected_policy jsonb;
  started_at timestamptz;
  duration_days int;
  grace_days int;
  early_days int;
  primary_days int;
  max_follow_ups int;
  follow_up_day int;
  previous_day int;
  should_bind boolean := false;
  was_unbound boolean := true;
begin
  if tg_op = 'INSERT' then
    should_bind := new.status = 'measuring';
  elsif tg_op = 'UPDATE' then
    was_unbound := old.measuring_started_at is null;
    should_bind := new.status = 'measuring' and old.measuring_started_at is null;
  end if;

  if tg_op = 'UPDATE' and old.measuring_started_at is not null and not should_bind then
    if new.measurement_policy is distinct from old.measurement_policy
      or new.measurement_policy_version is distinct from old.measurement_policy_version then
      raise exception 'measurement policy is immutable after measuring starts' using errcode = '23514';
    end if;
    if new.measuring_started_at is distinct from old.measuring_started_at then
      raise exception 'measurement start is immutable' using errcode = '23514';
    end if;
    if new.absolute_terminal_at is distinct from old.absolute_terminal_at then
      raise exception 'absolute measurement deadline is immutable' using errcode = '23514';
    end if;
  end if;

  if not should_bind and was_unbound and new.status <> 'measuring' and (
    new.measuring_started_at is not null
    or new.absolute_terminal_at is not null
    or new.measurement_policy <> '{}'::jsonb
    or new.measurement_policy_version <> 'legacy-v0'
  ) then
    raise exception 'measurement policy cannot be bound before measuring starts' using errcode = '23514';
  end if;

  if should_bind then
    select case
      when jsonb_typeof(opportunity.growth_spec->'measurement_policy') = 'object'
        and nullif(btrim(opportunity.growth_spec->'measurement_policy'->>'policy_version'), '') is not null
      then opportunity.growth_spec->'measurement_policy'
      else jsonb_build_object(
        'policy_version', 'legacy-measurement-v1',
        'early_signal_offset_days', 14,
        'primary_checkpoint_offset_days', 28,
        'follow_up_offsets_days', jsonb_build_array(56, 90),
        'max_follow_up_attempts', 2,
        'max_measuring_duration_days', 90,
        'terminalization_grace_period_days', 7
      )
    end into selected_policy
    from seo_opportunities opportunity
    where opportunity.project_id = new.project_id and opportunity.id = new.opportunity_id;

    if selected_policy is null then
      raise exception 'Growth measurement policy requires its Opportunity' using errcode = '23514';
    end if;
    duration_days := (selected_policy->>'max_measuring_duration_days')::int;
    grace_days := (selected_policy->>'terminalization_grace_period_days')::int;
    early_days := (selected_policy->>'early_signal_offset_days')::int;
    primary_days := (selected_policy->>'primary_checkpoint_offset_days')::int;
    max_follow_ups := (selected_policy->>'max_follow_up_attempts')::int;
    if duration_days not between 1 and 365 or grace_days not between 0 and 30
      or early_days <= 0 or primary_days <= early_days or primary_days > duration_days
      or max_follow_ups not between 0 and 4
      or jsonb_typeof(selected_policy->'follow_up_offsets_days') <> 'array'
      or jsonb_array_length(selected_policy->'follow_up_offsets_days') > max_follow_ups then
      raise exception 'Growth measurement policy is not finitely bounded' using errcode = '23514';
    end if;
    previous_day := primary_days;
    for follow_up_day in select value::int from jsonb_array_elements_text(selected_policy->'follow_up_offsets_days') loop
      if follow_up_day <= previous_day or follow_up_day > duration_days then
        raise exception 'Growth measurement follow-ups are not finitely ordered' using errcode = '23514';
      end if;
      previous_day := follow_up_day;
    end loop;
    started_at := coalesce(new.published_at, new.verified_at, now());
    new.measurement_policy := selected_policy;
    new.measurement_policy_version := selected_policy->>'policy_version';
    new.measuring_started_at := started_at;
    new.absolute_terminal_at := started_at + ((duration_days + grace_days) * interval '1 day');
  end if;

  if new.status = 'completed' and nullif(btrim(coalesce(new.measurement_terminal_reason, '')), '') is null then
    new.measurement_terminal_reason := 'measurement_checkpoints_completed';
  end if;
  return new;
end;
$$;

drop trigger if exists content_actions_bind_immutable_measurement_policy on content_actions;
create trigger content_actions_bind_immutable_measurement_policy
before insert or update of status, measurement_policy, measurement_policy_version, measuring_started_at, absolute_terminal_at
on content_actions
for each row execute function bind_immutable_growth_measurement_policy();
