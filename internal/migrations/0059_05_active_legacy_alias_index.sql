-- citeloop:migration-mode=nontransactional
-- citeloop:index=uniq_active_legacy_object_alias
create unique index concurrently if not exists uniq_active_legacy_object_alias
  on legacy_object_aliases (project_id, legacy_object_type, legacy_object_id)
  where alias_state in ('active');
