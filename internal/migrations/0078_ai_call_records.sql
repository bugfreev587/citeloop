set local lock_timeout = '5s';
set local statement_timeout = '30s';

alter table ai_call_records
  add column if not exists attempt_number integer,
  add column if not exists provider_called boolean,
  add column if not exists provider_started_at timestamptz,
  add column if not exists caused_by_call_id uuid;

-- Quarantine any legacy cross-project link before adding project-scoped FKs.
update ai_call_records child
set parent_call_id = null
from ai_call_records parent
where child.parent_call_id = parent.id
  and child.project_id <> parent.project_id;

-- The pre-0078 parent column also represented causal generation→verification
-- links. Move same-project causal links to a dedicated column before parent
-- becomes retry-only.
update ai_call_records child
set caused_by_call_id = child.parent_call_id,
    parent_call_id = null
from ai_call_records parent
where child.parent_call_id = parent.id
  and (
    child.stage <> parent.stage
    or child.linked_object_type <> parent.linked_object_type
    or child.linked_object_id <> parent.linked_object_id
    or child.request_fingerprint <> parent.request_fingerprint
  )
  and child.project_id = parent.project_id;

-- Known historical preflight failures never reached a provider. Reclassifying
-- them as skipped keeps provider-call counts and spend honest.
update ai_call_records
set status = 'skipped'
where status = 'failed'
  and (
    provider = 'none'
    or error_code in (
      'provider_unavailable', 'invalid_target', 'invalid_snapshot',
      'missing_grounding_context', 'invalid_input',
      'doctor_ai_not_authorized', 'deterministic_verification'
    )
  );

update ai_call_records
set attempt_number = coalesce(attempt_number, 1),
    provider_called = coalesce(provider_called, status not in ('queued', 'skipped')),
    provider_started_at = case
      when coalesce(provider_called, status not in ('queued', 'skipped')) then coalesce(provider_started_at, started_at)
      else null
    end
where attempt_number is null
   or provider_called is null
   or (provider_called and provider_started_at is null)
   or (not provider_called and provider_started_at is not null);

update ai_call_records
set prompt_tokens = 0,
    completion_tokens = 0,
    total_tokens = 0,
    cost_usd = 0,
    error_code = coalesce(error_code, 'unspecified_skip'),
    finished_at = coalesce(finished_at, updated_at, started_at)
where status = 'skipped';

update ai_call_records
set finished_at = coalesce(finished_at, updated_at, started_at)
where status in ('ok', 'partial', 'failed') and finished_at is null;

update ai_call_records
set error_code = case when status = 'partial' then 'partial_result' else 'unknown_failure' end
where status in ('failed', 'partial') and error_code is null;

alter table ai_call_records
  alter column attempt_number set default 1,
  alter column attempt_number set not null,
  alter column provider_called set default true,
  alter column provider_called set not null,
  alter column provider_started_at set default now();

-- This trigger is intentionally rolling-deploy compatible. Old instances omit
-- the new columns and still use parent_call_id for causal links; defaults plus
-- this normalizer keep their writes valid until they drain.
create or replace function normalize_ai_call_accounting()
returns trigger language plpgsql as $$
declare
  parent_row ai_call_records%rowtype;
	late_usage_fill boolean;
begin
	if tg_op = 'UPDATE' and old.status not in ('queued', 'running') then
		-- Cleanup may terminalize a still-running physical call with zero usage.
		-- Its eventual provider result may fill that missing accounting once;
		-- nonzero terminal accounting is immutable against duplicate late writes.
		late_usage_fill := old.prompt_tokens = 0
			and old.completion_tokens = 0
			and old.total_tokens = 0
			and old.cost_usd = 0
			and (
				new.prompt_tokens > 0 or new.completion_tokens > 0
				or new.total_tokens > 0 or new.cost_usd > 0
				or new.provider is distinct from old.provider
				or new.model is distinct from old.model
			);
		if not late_usage_fill then
			new.provider := old.provider;
			new.model := old.model;
			new.prompt_tokens := old.prompt_tokens;
			new.completion_tokens := old.completion_tokens;
			new.total_tokens := old.total_tokens;
			new.cost_usd := old.cost_usd;
		end if;
		new.provider_called := old.provider_called;
		new.provider_started_at := old.provider_started_at;
		if not (
			old.status in ('ok', 'partial')
			and new.status = 'failed'
			and new.error_code in ('invalid_response', 'invalid_output')
		) then
			new.status := old.status;
			new.error_code := old.error_code;
		end if;
		new.finished_at := old.finished_at;
	end if;

	if new.status = 'failed' and (
		new.provider = 'none'
		or new.error_code in (
			'provider_unavailable', 'invalid_target', 'invalid_snapshot',
			'missing_grounding_context', 'invalid_input',
			'doctor_ai_not_authorized', 'deterministic_verification',
			'deterministic_snapshot_mismatch', 'invalid_evidence'
		)
	) then
		new.status := 'skipped';
	end if;

  if new.parent_call_id is not null then
    select * into parent_row from ai_call_records
    where id = new.parent_call_id and project_id = new.project_id;
    if not found then
      raise exception 'AI retry parent must belong to the same project';
    end if;
    if new.stage <> parent_row.stage
       or new.linked_object_type <> parent_row.linked_object_type
       or new.linked_object_id <> parent_row.linked_object_id
       or new.request_fingerprint <> parent_row.request_fingerprint then
      if new.caused_by_call_id is null then
        new.caused_by_call_id := new.parent_call_id;
      end if;
      new.parent_call_id := null;
      new.attempt_number := 1;
    else
      new.attempt_number := parent_row.attempt_number + 1;
    end if;
  elsif new.attempt_number is null then
    new.attempt_number := 1;
  end if;

  if new.caused_by_call_id is not null and not exists (
    select 1 from ai_call_records cause
    where cause.id = new.caused_by_call_id and cause.project_id = new.project_id
  ) then
    raise exception 'AI causal parent must belong to the same project';
  end if;

  if new.status in ('queued', 'skipped') then
    new.provider_called := false;
    new.provider_started_at := null;
  else
    new.provider_called := true;
    new.provider_started_at := coalesce(new.provider_started_at, new.started_at, now());
  end if;
  if new.status = 'skipped' then
    new.prompt_tokens := 0;
    new.completion_tokens := 0;
    new.total_tokens := 0;
    new.cost_usd := 0;
    new.error_code := coalesce(new.error_code, 'unspecified_skip');
  end if;
	if new.status in ('failed', 'partial') and new.error_code is null then
		new.error_code := case when new.status = 'partial' then 'partial_result' else 'unknown_failure' end;
	elsif new.status = 'ok' then
		new.error_code := null;
	end if;
  if new.status in ('queued', 'running') then
    new.finished_at := null;
  else
    new.finished_at := coalesce(new.finished_at, now());
  end if;
  return new;
end $$;

drop trigger if exists ai_call_records_accounting_compat on ai_call_records;
create trigger ai_call_records_accounting_compat
before insert or update on ai_call_records
for each row execute function normalize_ai_call_accounting();

alter table ai_call_records
  drop constraint if exists ai_call_records_attempt_number_check,
  drop constraint if exists ai_call_records_provider_invocation_check,
  drop constraint if exists ai_call_records_skipped_usage_check,
  drop constraint if exists ai_call_records_terminal_time_check,
  drop constraint if exists ai_call_records_terminal_error_check;

alter table ai_call_records
  add constraint ai_call_records_attempt_number_check
    check (attempt_number > 0) not valid,
  add constraint ai_call_records_provider_invocation_check
    check (
      (status = 'skipped' and provider_called = false and provider_started_at is null)
      or
      (status = 'queued' and provider_called = false and provider_started_at is null)
      or
      (status not in ('queued', 'skipped') and provider_called = true and provider_started_at is not null)
    ) not valid,
  add constraint ai_call_records_skipped_usage_check
    check (
      status <> 'skipped'
      or (error_code is not null and prompt_tokens = 0 and completion_tokens = 0 and total_tokens = 0 and cost_usd = 0)
    ) not valid,
  add constraint ai_call_records_terminal_time_check
    check (
      (status in ('queued', 'running') and finished_at is null)
      or (status not in ('queued', 'running') and finished_at is not null)
    ) not valid,
  add constraint ai_call_records_terminal_error_check
    check (
      (status in ('failed', 'partial', 'skipped') and error_code is not null)
      or (status = 'ok' and error_code is null)
      or status in ('queued', 'running')
    ) not valid;

create index if not exists idx_ai_call_records_retry_chain
  on ai_call_records (project_id, request_fingerprint, attempt_number, created_at);

create index if not exists idx_ai_call_records_parent_call_id
  on ai_call_records (project_id, parent_call_id)
  where parent_call_id is not null;

create index if not exists idx_ai_call_records_caused_by_call_id
  on ai_call_records (project_id, caused_by_call_id)
  where caused_by_call_id is not null;

create unique index if not exists idx_ai_call_records_id_project
  on ai_call_records (id, project_id);

alter table ai_call_records
  drop constraint if exists ai_call_records_parent_call_id_fkey,
  drop constraint if exists ai_call_records_parent_project_fkey,
  drop constraint if exists ai_call_records_cause_project_fkey;

alter table ai_call_records
  add constraint ai_call_records_parent_project_fkey
    foreign key (parent_call_id, project_id)
    references ai_call_records (id, project_id) not valid,
  add constraint ai_call_records_cause_project_fkey
    foreign key (caused_by_call_id, project_id)
    references ai_call_records (id, project_id) not valid;

alter table ai_call_records validate constraint ai_call_records_attempt_number_check;
alter table ai_call_records validate constraint ai_call_records_provider_invocation_check;
alter table ai_call_records validate constraint ai_call_records_skipped_usage_check;
alter table ai_call_records validate constraint ai_call_records_terminal_time_check;
alter table ai_call_records validate constraint ai_call_records_terminal_error_check;
alter table ai_call_records validate constraint ai_call_records_parent_project_fkey;
alter table ai_call_records validate constraint ai_call_records_cause_project_fkey;

reset statement_timeout;
reset lock_timeout;
