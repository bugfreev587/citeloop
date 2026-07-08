-- PRD-CiteLoop-Opportunity-Reconsideration-and-Dismissal:
-- split stable opportunity identity from evidence fingerprint, add a returned
-- action state, and persist user-reviewed opportunity states.

alter table content_actions drop constraint if exists content_actions_status_check;
alter table content_actions
  add constraint content_actions_status_check
  check (status in (
    'drafting','ready_for_review','approved','published','measuring','completed','failed',
    'verification_failed','recovery_required','dismissed',
    'verification_pending','manual_apply_required','needs_follow_up','returned'
  ));

alter table content_actions add column if not exists status_reason text;

alter table seo_opportunities add column if not exists opportunity_identity_key text not null default '';
alter table seo_opportunities add column if not exists evidence_fingerprint text not null default '';

update seo_opportunities
set
  opportunity_identity_key = encode(digest(
    project_id::text || '|' ||
    type || '|' ||
    coalesce(normalized_page_url, '') || '|' ||
    coalesce(query, '') || '|' ||
    coalesce(evidence->>'intent_type', '') || '|' ||
    coalesce(evidence->>'engine', ''),
    'sha256'
  ), 'hex'),
  evidence_fingerprint = encode(digest(
    coalesce(evidence->>'evidence_window', '') || '|' ||
    coalesce(evidence->>'reason', '') || '|' ||
    coalesce(evidence->>'severity', '') || '|' ||
    coalesce(evidence->>'issue_type', '') || '|' ||
    coalesce(evidence->>'status', '') || '|' ||
    coalesce(evidence->>'title_status', '') || '|' ||
    coalesce(evidence->>'meta_description_status', '') || '|' ||
    coalesce(priority_score::text, '') || '|' ||
    coalesce(confidence::text, '') || '|' ||
    coalesce(risk_level, ''),
    'sha256'
  ), 'hex')
where opportunity_identity_key = '' or evidence_fingerprint = '';

update seo_opportunities
set opportunity_key = opportunity_identity_key
where opportunity_key is distinct from opportunity_identity_key;

with ranked as (
  select
    id,
    first_value(id) over (
      partition by project_id, opportunity_identity_key
      order by
        case status
          when 'converted' then 1
          when 'accepted' then 2
          when 'dismissed' then 3
          when 'snoozed' then 4
          when 'watching' then 5
          when 'open' then 6
          else 7
        end,
        updated_at desc,
        created_at desc,
        id
    ) as winner_id,
    row_number() over (
      partition by project_id, opportunity_identity_key
      order by
        case status
          when 'converted' then 1
          when 'accepted' then 2
          when 'dismissed' then 3
          when 'snoozed' then 4
          when 'watching' then 5
          when 'open' then 6
          else 7
        end,
        updated_at desc,
        created_at desc,
        id
    ) as rn
  from seo_opportunities
  where status in ('open','accepted','converted','dismissed','snoozed','watching')
    and opportunity_identity_key <> ''
)
update seo_opportunities so
set
  status = 'stale',
  evidence = so.evidence || jsonb_build_object(
    'staled_by_opportunity_identity_dedupe', true,
    'dedupe_winner_id', ranked.winner_id
  ),
  updated_at = now()
from ranked
where so.id = ranked.id
  and ranked.rn > 1;

create table if not exists seo_opportunity_review_states (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  opportunity_identity_key text not null,
  source_opportunity_id uuid references seo_opportunities(id) on delete set null,
  content_action_id uuid references content_actions(id) on delete set null,
  review_status text not null check (review_status in ('dismissed','snoozed','watching')),
  evidence_fingerprint text not null default '',
  reviewed_by text,
  reviewed_at timestamptz not null default now(),
  snoozed_until timestamptz,
  material_change_metadata jsonb not null default '{}',
  reopened_at timestamptz,
  reopened_reason text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, opportunity_identity_key)
);

insert into seo_opportunity_review_states
  (project_id, opportunity_identity_key, source_opportunity_id, review_status,
   evidence_fingerprint, snoozed_until, reviewed_at, created_at, updated_at)
select
  project_id,
  opportunity_identity_key,
  id,
  status,
  evidence_fingerprint,
  snoozed_until,
  updated_at,
  created_at,
  updated_at
from seo_opportunities
where status in ('dismissed','snoozed','watching')
  and opportunity_identity_key <> ''
on conflict (project_id, opportunity_identity_key) do update set
  source_opportunity_id = excluded.source_opportunity_id,
  review_status = excluded.review_status,
  evidence_fingerprint = excluded.evidence_fingerprint,
  snoozed_until = excluded.snoozed_until,
  reviewed_at = excluded.reviewed_at,
  updated_at = now();

create index if not exists idx_seo_opportunity_review_states_project_status
  on seo_opportunity_review_states (project_id, review_status, updated_at desc);

drop index if exists uniq_open_seo_opportunity_key;

create unique index if not exists uniq_reviewable_seo_opportunity_identity
  on seo_opportunities (project_id, opportunity_identity_key)
  where status in ('open','accepted','converted','dismissed','snoozed','watching');
