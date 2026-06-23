-- Multi-Surface SEO Growth Layer foundation.

create table if not exists seo_asset_types (
  id uuid primary key default gen_random_uuid(),
  key text not null unique,
  name text not null,
  description text not null default '',
  default_risk_level text not null default 'medium'
    check (default_risk_level in ('low','medium','high')),
  default_measurement_window_days int not null default 28,
  supported_publication_surfaces jsonb not null default '[]',
  requires_evidence boolean not null default true,
  requires_review_by_default boolean not null default true,
  default_generation_path text not null default 'topic_article'
    check (default_generation_path in ('topic_article','direct_patch','external_draft','technical_task')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

insert into seo_asset_types
  (key, name, description, default_risk_level, default_measurement_window_days,
   supported_publication_surfaces, requires_evidence, requires_review_by_default, default_generation_path)
values
  ('blog_post', 'Blog post', 'Canonical article or supporting article.', 'medium', 56, '["blog"]', true, true, 'topic_article'),
  ('comparison_page', 'Comparison page', 'Competitor or product comparison page.', 'medium', 90, '["blog","landing","docs"]', true, true, 'topic_article'),
  ('alternative_page', 'Alternative page', 'Alternative-to page for buyer-intent searches.', 'medium', 90, '["blog","landing"]', true, true, 'topic_article'),
  ('use_case_page', 'Use-case page', 'Workflow, persona, or problem-oriented page.', 'medium', 90, '["blog","landing","docs"]', true, true, 'topic_article'),
  ('integration_page', 'Integration page', 'Integration page for a third-party platform or workflow.', 'medium', 90, '["docs","landing"]', true, true, 'topic_article'),
  ('template_or_checklist', 'Template or checklist', 'Reusable template, checklist, or downloadable asset.', 'medium', 56, '["blog","docs","hosted_asset"]', true, true, 'topic_article'),
  ('glossary_definition', 'Glossary definition', 'Definition or terminology page/block.', 'low', 56, '["blog","docs","landing"]', true, false, 'topic_article'),
  ('benchmark_report', 'Benchmark report', 'Small data report, benchmark, or stats page.', 'medium', 90, '["blog","hosted_asset"]', true, true, 'topic_article'),
  ('metadata_rewrite', 'Metadata rewrite', 'Title/meta rewrite for an existing page.', 'low', 28, '["owned_site"]', true, false, 'direct_patch'),
  ('internal_link_patch', 'Internal link patch', 'Internal link additions or edits.', 'low', 56, '["owned_site"]', true, false, 'direct_patch'),
  ('schema_patch', 'Structured data patch', 'JSON-LD or structured data update.', 'medium', 28, '["owned_site"]', true, true, 'direct_patch'),
  ('sitemap_update', 'Sitemap update', 'Sitemap creation, update, or submit action.', 'low', 14, '["owned_site"]', false, false, 'technical_task')
on conflict (key) do update set
  name = excluded.name,
  description = excluded.description,
  default_risk_level = excluded.default_risk_level,
  default_measurement_window_days = excluded.default_measurement_window_days,
  supported_publication_surfaces = excluded.supported_publication_surfaces,
  requires_evidence = excluded.requires_evidence,
  requires_review_by_default = excluded.requires_review_by_default,
  default_generation_path = excluded.default_generation_path,
  updated_at = now();

alter table content_actions
  add column if not exists asset_type text,
  add column if not exists target_surface_id uuid,
  add column if not exists risk_reasons jsonb not null default '[]',
  add column if not exists evidence_snapshot jsonb not null default '{}',
  add column if not exists input_snapshot jsonb not null default '{}',
  add column if not exists output_snapshot jsonb not null default '{}',
  add column if not exists diff_snapshot jsonb not null default '{}',
  add column if not exists review_required boolean not null default true,
  add column if not exists approved_by text,
  add column if not exists approved_at timestamptz,
  add column if not exists verified_at timestamptz,
  add column if not exists verification_snapshot jsonb not null default '{}';

alter table content_actions drop constraint if exists content_actions_status_check;
alter table content_actions
  add constraint content_actions_status_check
  check (status in (
    'drafting','ready_for_review','approved','published','measuring','completed','failed',
    'verification_failed','recovery_required'
  ));

alter table geo_external_surfaces
  add column if not exists source_url text,
  add column if not exists canonical_status text not null default 'unknown',
  add column if not exists indexability_status text not null default 'unknown',
  add column if not exists publication_status text not null default 'unknown',
  add column if not exists owner_confidence text not null default 'medium'
    check (owner_confidence in ('high','medium','low')),
  add column if not exists last_verified_at timestamptz,
  add column if not exists verification_snapshot jsonb not null default '{}',
  add column if not exists related_action_ids jsonb not null default '[]';

alter table geo_external_surfaces drop constraint if exists geo_external_surfaces_owner_type_check;
alter table geo_external_surfaces
  add constraint geo_external_surfaces_owner_type_check
  check (owner_type in ('project','user','owned','managed_external','third_party'));

alter table geo_asset_briefs
  add column if not exists target_queries jsonb not null default '[]',
  add column if not exists target_personas jsonb not null default '[]',
  add column if not exists expected_citation_mechanism text not null default '',
  add column if not exists source_type text not null default 'geo'
    check (source_type in ('geo','seo','distribution','technical'));

alter table seo_opportunities
  add column if not exists opportunity_key text;

update seo_opportunities
set opportunity_key = encode(digest(
  project_id::text || '|' || type || '|' || coalesce(normalized_page_url, '') || '|' ||
  coalesce(query, '') || '|' || coalesce((evidence->>'intent_type'), '') || '|' ||
  coalesce((evidence->>'engine'), '') || '|' || coalesce((evidence->>'evidence_window'), ''),
  'sha256'
), 'hex')
where opportunity_key is null or opportunity_key = '';

with ranked as (
  select
    id,
    first_value(id) over (
      partition by project_id, opportunity_key
      order by
        case status
          when 'converted' then 1
          when 'accepted' then 2
          when 'open' then 3
          else 4
        end,
        updated_at desc,
        created_at desc,
        id
    ) as winner_id,
    row_number() over (
      partition by project_id, opportunity_key
      order by
        case status
          when 'converted' then 1
          when 'accepted' then 2
          when 'open' then 3
          else 4
        end,
        updated_at desc,
        created_at desc,
        id
    ) as rn
  from seo_opportunities
  where status in ('open','accepted','converted')
)
update seo_opportunities so
set
  status = 'stale',
  evidence = so.evidence || jsonb_build_object(
    'staled_by_multi_surface_dedupe', true,
    'dedupe_winner_id', ranked.winner_id
  ),
  updated_at = now()
from ranked
where so.id = ranked.id
  and ranked.rn > 1;

alter table seo_opportunities
  alter column opportunity_key set default '',
  alter column opportunity_key set not null;

create unique index if not exists uniq_open_seo_opportunity_key
  on seo_opportunities (project_id, opportunity_key)
  where status in ('open','accepted','converted');

alter table articles
  add column if not exists publication_mode text not null default 'auto'
    check (publication_mode in ('auto','draft_only','gated_publish','auto_allowed')),
  add column if not exists source_url text,
  add column if not exists external_url text,
  add column if not exists verification_status text not null default 'unknown'
    check (verification_status in ('unknown','pending','passed','failed')),
  add column if not exists external_surface_id uuid;
