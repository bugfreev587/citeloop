-- citeloop:migration-mode=nontransactional
-- citeloop:index=idx_discovery_candidates_shadow_run_fk
create index concurrently if not exists idx_discovery_candidates_shadow_run_fk
  on discovery_candidates (shadow_run_id);
