-- Historical cleanup: older measurement code could terminalize a parent
-- content_action even though its child article was still in Review/Publish.
-- That creates impossible workflow links such as a Results/Learned parent for
-- an article that is only approved and waiting in Publish. Reset only those
-- impossible parents; do not touch genuinely published/applied work.

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
  if tg_op = 'UPDATE'
    and new.status = 'ready_for_review'
    and old.draft_article_id is not null
    and new.status_reason like 'Reset stale parent handoff link%'
    and new.published_at is null
    and new.verified_at is null
    and new.verification_snapshot = '{}'::jsonb
    and new.outcome_summary = '{}'::jsonb
    and new.measuring_started_at is null
    and new.absolute_terminal_at is null
    and new.measurement_terminal_reason is null
    and new.measurement_policy = '{}'::jsonb
    and new.measurement_policy_version = 'legacy-v0' then
    return new;
  end if;

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

with stale_parent_actions as (
  select ca.id
  from content_actions ca
  join articles article
    on article.id = ca.draft_article_id
   and article.project_id = ca.project_id
  where ca.status in ('published','measuring','completed','failed','verification_failed','recovery_required')
    and article.status in (
      'generating',
      'pending_review',
      'approved',
      'scheduled',
      'pending_url_verification',
      'publish_failed',
      'ready_to_distribute',
      'distributed'
    )
    and article.published_at is null
)
delete from action_measurements measurement
using stale_parent_actions stale
where measurement.content_action_id = stale.id;

with stale_parent_actions as (
  select ca.id, ca.status as stale_status, article.status as article_status
  from content_actions ca
  join articles article
    on article.id = ca.draft_article_id
   and article.project_id = ca.project_id
  where ca.status in ('published','measuring','completed','failed','verification_failed','recovery_required')
    and article.status in (
      'generating',
      'pending_review',
      'approved',
      'scheduled',
      'pending_url_verification',
      'publish_failed',
      'ready_to_distribute',
      'distributed'
    )
    and article.published_at is null
)
update content_actions ca
set status = 'ready_for_review',
    published_at = null,
    verified_at = null,
    verification_snapshot = '{}'::jsonb,
    outcome_summary = '{}'::jsonb,
    measuring_started_at = null,
    absolute_terminal_at = null,
    measurement_terminal_reason = null,
    measurement_policy_version = 'legacy-v0',
    measurement_policy = '{}'::jsonb,
    status_reason = 'Reset stale parent handoff link from ' || stale.stale_status || ' because child article is still ' || stale.article_status,
    updated_at = now()
from stale_parent_actions stale
where ca.id = stale.id;
