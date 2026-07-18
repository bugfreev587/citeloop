-- citeloop:migration-mode=nontransactional
-- citeloop:index=idx_site_fix_evidence_merges_project_fix_finding
create index concurrently if not exists idx_site_fix_evidence_merges_project_fix_finding
  on site_fix_evidence_merges (project_id, site_fix_id, doctor_finding_id);
