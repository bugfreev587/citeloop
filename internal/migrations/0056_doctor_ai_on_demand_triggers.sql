set local lock_timeout = '5s';
set local statement_timeout = '30s';

create table if not exists doctor_ai_on_demand_triggers (
  request_id uuid primary key,
  project_id uuid not null references projects(id) on delete cascade,
  site_fix_id uuid not null,
  trigger_kind text not null default 'verification_user'
    check (trigger_kind = 'verification_user'),
  requested_policy text not null
    check (requested_policy in ('manual_only','on_demand','automatic')),
	status text not null default 'pending'
	check (status in ('pending','processing','consumed','rejected','superseded')),
  processing_token uuid,
  processing_expires_at timestamptz,
  ai_call_id uuid references ai_call_records(id) on delete restrict,
  result_snapshot jsonb check (result_snapshot is null or jsonb_typeof(result_snapshot) = 'object'),
	rejection_reason text,
  requested_at timestamptz not null default now(),
  consumed_at timestamptz,
  lifecycle_applied_at timestamptz,
  constraint doctor_ai_on_demand_triggers_site_fix_fk
    foreign key (project_id, site_fix_id)
    references site_fixes(project_id, id)
    on delete cascade,
  constraint doctor_ai_on_demand_triggers_consumption_check
	check (
	  (status = 'pending' and processing_token is null and processing_expires_at is null
	    and ai_call_id is null and result_snapshot is null and rejection_reason is null and consumed_at is null and lifecycle_applied_at is null)
	  or (status = 'processing' and processing_token is not null and processing_expires_at is not null
	    and result_snapshot is null and rejection_reason is null and consumed_at is null and lifecycle_applied_at is null)
	  or (status = 'consumed' and processing_token is null and processing_expires_at is null
	    and ai_call_id is not null and result_snapshot is not null and rejection_reason is null and consumed_at is not null)
	  or (status in ('rejected','superseded') and processing_token is null and processing_expires_at is null
	    and result_snapshot is not null and length(btrim(rejection_reason)) > 0
	    and consumed_at is not null and lifecycle_applied_at is not null)
	)
);

create index if not exists doctor_ai_on_demand_triggers_pending_idx
  on doctor_ai_on_demand_triggers(project_id, site_fix_id, requested_at)
  where status = 'pending';

create index if not exists doctor_ai_on_demand_triggers_processing_expiry_idx
  on doctor_ai_on_demand_triggers(project_id, processing_expires_at)
  where status = 'processing';

create index if not exists doctor_ai_on_demand_triggers_consumed_unapplied_idx
  on doctor_ai_on_demand_triggers(project_id, site_fix_id, consumed_at)
  where status = 'consumed' and lifecycle_applied_at is null;
