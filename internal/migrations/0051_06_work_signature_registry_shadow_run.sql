-- citeloop:migration-mode=nontransactional
-- citeloop:index=idx_work_signature_registry_shadow_run_fk
create index concurrently if not exists idx_work_signature_registry_shadow_run_fk
  on work_signature_registry (shadow_run_id);
