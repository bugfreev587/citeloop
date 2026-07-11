-- citeloop:migration-mode=nontransactional
-- citeloop:index=idx_seo_opportunities_legacy_growth_migration
create index concurrently if not exists idx_seo_opportunities_legacy_growth_migration
  on seo_opportunities (project_id, created_at, id)
  where canonical_growth = false
    and status in ('open','accepted','converted','snoozed','watching');
