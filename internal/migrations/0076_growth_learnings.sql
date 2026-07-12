-- Immutable terminal Growth outcomes. Directional learnings and measurement
-- quality records are deliberately separate so insufficient data never scores.

create table if not exists growth_terminal_outcomes (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  candidate_id uuid references discovery_candidates(id) on delete no action deferrable initially deferred,
  opportunity_id uuid not null references seo_opportunities(id) on delete no action deferrable initially deferred,
  content_action_id uuid not null references content_actions(id) on delete no action deferrable initially deferred,
  article_id uuid references articles(id) on delete no action deferrable initially deferred,
  artifact_url text not null default '',
  action_family text not null,
  target_identity jsonb not null default '{}'::jsonb check (jsonb_typeof(target_identity) = 'object'),
  audience jsonb not null default '[]'::jsonb check (jsonb_typeof(audience) = 'array'),
  primary_metric text not null,
  outcome_label text not null check (outcome_label in ('positive','negative','mixed','inconclusive','insufficient_data')),
  record_kind text not null check (record_kind in ('directional_learning','measurement_quality')),
  terminal_reason text not null,
  measurement_policy_version text not null,
  baseline_snapshot jsonb not null check (jsonb_typeof(baseline_snapshot) = 'object'),
  checkpoint_snapshot jsonb not null check (jsonb_typeof(checkpoint_snapshot) = 'object'),
  outcome_snapshot jsonb not null check (jsonb_typeof(outcome_snapshot) = 'object'),
  created_at timestamptz not null default now(),
  unique (project_id, content_action_id),
  check (
    (record_kind = 'directional_learning' and outcome_label in ('positive','negative','mixed','inconclusive'))
    or (record_kind = 'measurement_quality' and outcome_label = 'insufficient_data')
  )
);

create table if not exists growth_learnings (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  terminal_outcome_id uuid not null unique references growth_terminal_outcomes(id) on delete cascade,
  content_action_id uuid not null,
  learning_summary text not null,
  applicability jsonb not null default '{}'::jsonb check (jsonb_typeof(applicability) = 'object'),
  scoring_eligible boolean not null default true check (scoring_eligible = true),
  learning_version text not null default 'growth-learning-v1',
  created_at timestamptz not null default now(),
  unique (project_id, content_action_id)
);

create table if not exists measurement_quality_records (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  terminal_outcome_id uuid not null unique references growth_terminal_outcomes(id) on delete cascade,
  content_action_id uuid not null,
  data_quality_state text not null,
  quality_gaps jsonb not null default '[]'::jsonb check (jsonb_typeof(quality_gaps) = 'array'),
  recommendation text not null,
  scoring_eligible boolean not null default false,
  quality_version text not null default 'measurement-quality-v1',
  created_at timestamptz not null default now(),
  unique (project_id, content_action_id),
  check (scoring_eligible = false)
);

create or replace function enforce_growth_terminal_project_scope()
returns trigger language plpgsql as $$
begin
  if not exists (
    select 1 from seo_opportunities opportunity
    where opportunity.id = new.opportunity_id and opportunity.project_id = new.project_id
  ) then
    raise exception 'Growth terminal opportunity is outside the project scope' using errcode = '23514';
  end if;

  if not exists (
    select 1 from content_actions action
    where action.id = new.content_action_id and action.project_id = new.project_id
      and action.opportunity_id = new.opportunity_id
  ) then
    raise exception 'Growth terminal action is outside the project or opportunity scope' using errcode = '23514';
  end if;

  if new.candidate_id is not null and not exists (
    select 1 from discovery_candidates candidate
    where candidate.id = new.candidate_id and candidate.project_id = new.project_id
  ) then
    raise exception 'Growth terminal candidate is outside the project scope' using errcode = '23514';
  end if;

  if new.article_id is not null and not exists (
    select 1 from articles article
    where article.id = new.article_id and article.project_id = new.project_id
  ) then
    raise exception 'Growth terminal article is outside the project scope' using errcode = '23514';
  end if;

  return new;
end;
$$;

create trigger growth_terminal_outcomes_project_scope
before insert on growth_terminal_outcomes
for each row execute function enforce_growth_terminal_project_scope();

create or replace function prevent_growth_terminal_mutation()
returns trigger language plpgsql as $$
begin
  if tg_op = 'DELETE' and pg_trigger_depth() > 1 then
    return old;
  end if;
  raise exception 'Growth terminal records are immutable' using errcode = '55000';
end;
$$;

create trigger growth_terminal_outcomes_immutable
before update or delete on growth_terminal_outcomes
for each row execute function prevent_growth_terminal_mutation();
create trigger growth_learnings_immutable
before update or delete on growth_learnings
for each row execute function prevent_growth_terminal_mutation();
create trigger measurement_quality_records_immutable
before update or delete on measurement_quality_records
for each row execute function prevent_growth_terminal_mutation();

create or replace function enforce_growth_terminal_child_kind()
returns trigger language plpgsql as $$
declare expected_kind text;
begin
  expected_kind := case when tg_table_name = 'growth_learnings' then 'directional_learning' else 'measurement_quality' end;
  if not exists (
    select 1 from growth_terminal_outcomes terminal
    where terminal.id = new.terminal_outcome_id and terminal.project_id = new.project_id
      and terminal.content_action_id = new.content_action_id and terminal.record_kind = expected_kind
  ) then
    raise exception 'Growth terminal child kind does not match its outcome' using errcode = '23514';
  end if;
  return new;
end;
$$;

create trigger growth_learnings_kind_guard
before insert on growth_learnings
for each row execute function enforce_growth_terminal_child_kind();
create trigger measurement_quality_records_kind_guard
before insert on measurement_quality_records
for each row execute function enforce_growth_terminal_child_kind();

create or replace function enforce_growth_terminal_child_exists()
returns trigger language plpgsql as $$
begin
  if new.record_kind = 'directional_learning' and not exists (
    select 1 from growth_learnings learning where learning.terminal_outcome_id = new.id
  ) then
    raise exception 'Directional Growth outcome requires one learning' using errcode = '23514';
  end if;
  if new.record_kind = 'measurement_quality' and not exists (
    select 1 from measurement_quality_records quality where quality.terminal_outcome_id = new.id
  ) then
    raise exception 'Insufficient Growth outcome requires one measurement-quality record' using errcode = '23514';
  end if;
  return null;
end;
$$;

create constraint trigger growth_terminal_outcomes_child_required
after insert on growth_terminal_outcomes
deferrable initially deferred
for each row execute function enforce_growth_terminal_child_exists();
