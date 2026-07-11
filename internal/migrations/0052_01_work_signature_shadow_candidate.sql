-- citeloop:migration-mode=nontransactional
-- citeloop:index=uniq_work_signature_registry_shadow_candidate
create unique index concurrently if not exists uniq_work_signature_registry_shadow_candidate
  on work_signature_registry (candidate_id)
  where mode in ('shadow');
