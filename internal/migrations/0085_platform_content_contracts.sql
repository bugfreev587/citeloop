-- Immutable platform content contracts, target context revisions, and exact
-- content target plans.

create table if not exists platform_content_contracts (
  id uuid primary key default gen_random_uuid(),
  platform text not null,
  version text not null,
  status text not null check (status in ('draft','active','deprecated','retired')),
  source_urls jsonb not null default '[]'::jsonb check (jsonb_typeof(source_urls) = 'array'),
  source_retrieved_at timestamptz,
  effective_at timestamptz,
  review_due_at timestamptz,
  generation_supported boolean not null default true,
  publish_mode text not null check (publish_mode in ('automatic','semi_manual','manual')),
  allowed_output_types jsonb not null default '[]'::jsonb check (jsonb_typeof(allowed_output_types) = 'array'),
  compatible_asset_types jsonb not null default '[]'::jsonb check (jsonb_typeof(compatible_asset_types) = 'array'),
  required_context_fields jsonb not null default '[]'::jsonb check (jsonb_typeof(required_context_fields) = 'array'),
  capabilities jsonb not null default '{}'::jsonb check (jsonb_typeof(capabilities) = 'object'),
  canonical_policy jsonb not null default '{}'::jsonb check (jsonb_typeof(canonical_policy) = 'object'),
  hard_rules jsonb not null default '{}'::jsonb check (jsonb_typeof(hard_rules) = 'object'),
  prompt_template text not null default '',
  semantic_rubric jsonb not null default '{}'::jsonb check (jsonb_typeof(semantic_rubric) = 'object'),
  preview_renderer_key text not null,
  supersedes_contract_id uuid references platform_content_contracts(id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (platform, version)
);

create unique index if not exists uniq_active_platform_content_contract
  on platform_content_contracts(platform)
  where status = 'active';

create table if not exists platform_target_contexts (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  platform text not null,
  target_key text not null,
  version int not null check (version > 0),
  status text not null check (status in ('draft','confirmed','expired','superseded')),
  source_kind text not null check (source_kind in ('user_pasted_rules','user_confirmed_rules','approved_provider')),
  source_url text,
  rules_url text,
  rules_text text not null default '',
  allowed_post_types jsonb not null default '[]'::jsonb check (jsonb_typeof(allowed_post_types) = 'array'),
  required_flair text,
  link_policy text not null default '',
  self_promotion_policy text not null default '',
  disclosure_requirements text not null default '',
  notes text not null default '',
  content_hash text not null,
  confirmed_by text,
  confirmed_at timestamptz,
  expires_at timestamptz,
  supersedes_context_id uuid references platform_target_contexts(id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (project_id, platform, target_key, version),
  check ((status = 'confirmed' and confirmed_by is not null and confirmed_at is not null and expires_at is not null)
      or status <> 'confirmed')
);

create unique index if not exists uniq_current_target_context
  on platform_target_contexts(project_id, platform, target_key)
  where status = 'confirmed';

create table if not exists content_target_plans (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  opportunity_id uuid references seo_opportunities(id) on delete set null,
  content_action_id uuid references content_actions(id) on delete set null,
  asset_type text not null,
  canonical_target text not null,
  selection_mode text not null check (selection_mode in ('contract_matrix','legacy_derived')),
  status text not null default 'planned' check (status in ('planned','generating','generated','archived')),
  capability_snapshot jsonb not null default '{}'::jsonb check (jsonb_typeof(capability_snapshot) = 'object'),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index if not exists uniq_active_opportunity_target_plan
  on content_target_plans(project_id, opportunity_id)
  where opportunity_id is not null and status <> 'archived';

create table if not exists content_target_plan_items (
  id uuid primary key default gen_random_uuid(),
  plan_id uuid not null references content_target_plans(id) on delete cascade,
  ordinal int not null check (ordinal >= 0),
  platform text not null,
  target_key text not null default '',
  output_type text not null,
  is_canonical boolean not null default false,
  platform_contract_id uuid references platform_content_contracts(id) on delete restrict,
  platform_contract_version text not null,
  target_context_id uuid references platform_target_contexts(id) on delete restrict,
  target_context_version int,
  rationale text not null default '',
  status text not null default 'planned' check (status in ('planned','generating','generated','failed','archived')),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create unique index if not exists uniq_target_plan_item
  on content_target_plan_items(plan_id, platform, target_key);

alter table topics
  add column if not exists asset_type text,
  add column if not exists target_plan_id uuid references content_target_plans(id) on delete set null;

alter table articles
  add column if not exists platform_contract_id uuid references platform_content_contracts(id) on delete restrict,
  add column if not exists platform_contract_version text,
  add column if not exists target_context_id uuid references platform_target_contexts(id) on delete restrict,
  add column if not exists output_type text not null default 'long_form_article',
  add column if not exists platform_metadata jsonb not null default '{}'::jsonb,
  add column if not exists contract_validation jsonb not null default '{}'::jsonb;

insert into seo_asset_types
  (key, name, description, default_risk_level, default_measurement_window_days,
   supported_publication_surfaces, requires_evidence, requires_review_by_default, default_generation_path)
values
  ('source_backed_evidence_page', 'Source-backed evidence page', 'Citation-ready evidence page backed by verified sources.', 'medium', 56, '["blog","docs","landing"]', true, true, 'topic_article'),
  ('faq_answer_block', 'FAQ answer block', 'Extractable question-and-answer content block.', 'low', 56, '["blog","docs","landing"]', true, false, 'topic_article')
on conflict (key) do update set
  name = excluded.name,
  description = excluded.description,
  supported_publication_surfaces = excluded.supported_publication_surfaces,
  requires_evidence = excluded.requires_evidence,
  requires_review_by_default = excluded.requires_review_by_default,
  default_generation_path = excluded.default_generation_path,
  updated_at = now();

with contract_seed(platform, publish_mode, output_types, context_fields, preview_key, source_urls, canonical_policy) as (
  values
    ('blog', 'automatic', '["long_form_article"]'::jsonb, '[]'::jsonb, 'blog_mdx', '["internal://github-nextjs"]'::jsonb, '{"mode":"canonical"}'::jsonb),
    ('dev_to', 'semi_manual', '["long_form_article"]'::jsonb, '[]'::jsonb, 'dev_to_article', '["https://developers.forem.com/api/v1"]'::jsonb, '{"mode":"rel_canonical"}'::jsonb),
    ('hashnode', 'semi_manual', '["long_form_article"]'::jsonb, '["publication"]'::jsonb, 'hashnode_article', '["https://apidocs.hashnode.com/"]'::jsonb, '{"mode":"rel_canonical"}'::jsonb),
    ('medium', 'semi_manual', '["long_form_article"]'::jsonb, '[]'::jsonb, 'medium_story', '["https://help.medium.com/hc/en-us/articles/360033930293-Set-a-canonical-link"]'::jsonb, '{"mode":"rel_canonical"}'::jsonb),
    ('linkedin', 'semi_manual', '["long_form_article"]'::jsonb, '[]'::jsonb, 'linkedin_article', '["https://www.linkedin.com/help/linkedin/answer/a522427/publish-articles-on-linkedin"]'::jsonb, '{"mode":"source_link"}'::jsonb),
    ('reddit', 'manual', '["community_post","link_submission"]'::jsonb, '["subreddit","rules"]'::jsonb, 'reddit_post', '["https://support.reddithelp.com/hc/en-us/articles/360060422572-How-do-I-post-and-comment-on-Reddit"]'::jsonb, '{"mode":"source_link"}'::jsonb),
    ('hacker_news', 'manual', '["link_submission"]'::jsonb, '[]'::jsonb, 'hacker_news_submission', '["https://news.ycombinator.com/newsguidelines.html"]'::jsonb, '{"mode":"source_link"}'::jsonb)
), asset_keys as (
  select '["blog_post","comparison_page","alternative_page","use_case_page","integration_page","template_or_checklist","glossary_definition","benchmark_report","source_backed_evidence_page","faq_answer_block"]'::jsonb value
)
insert into platform_content_contracts
  (platform, version, status, source_urls, source_retrieved_at, effective_at, review_due_at,
   publish_mode, allowed_output_types, compatible_asset_types, required_context_fields,
   capabilities, canonical_policy, hard_rules, prompt_template, semantic_rubric, preview_renderer_key)
select platform, 'platform-contract-v1', 'active', source_urls, now(), now(), now() + interval '90 days',
       publish_mode, output_types, asset_keys.value, context_fields,
       '{}'::jsonb, canonical_policy, '{}'::jsonb,
       'Generate a native artifact for {{platform}} using the resolved asset and target constraints.',
       '{"native_fit":true,"factual_support":true}'::jsonb, preview_key
from contract_seed cross join asset_keys
on conflict (platform, version) do nothing;

-- Backfill existing unregistered GEO asset types without overwriting a
-- conflicting non-empty forward value.
update content_actions ca
set asset_type = gab.asset_type
from geo_asset_briefs gab
where ca.opportunity_id = gab.opportunity_id
  and ca.asset_type is null
  and gab.asset_type in ('source_backed_evidence_page','faq_answer_block');

update topics t
set asset_type = gab.asset_type
from content_actions ca
join geo_asset_briefs gab on gab.opportunity_id = ca.opportunity_id
where t.source_content_action_id = ca.id
  and t.asset_type is null
  and gab.asset_type in ('source_backed_evidence_page','faq_answer_block');

update articles a
set seo_meta = jsonb_set(a.seo_meta, '{asset_type}', to_jsonb(t.asset_type), true)
from topics t
where a.topic_id = t.id
  and t.asset_type in ('source_backed_evidence_page','faq_answer_block')
  and nullif(btrim(coalesce(a.seo_meta->>'asset_type', '')), '') is null;
