-- citeloop:migration-mode=nontransactional
-- citeloop:index=idx_work_review_memory_legacy_state
create index concurrently if not exists idx_work_review_memory_legacy_state
  on work_review_memory (project_id, legacy_review_state_id)
  where legacy_review_state_id is not null;
