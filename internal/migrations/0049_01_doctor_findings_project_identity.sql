-- citeloop:migration-mode=nontransactional
-- citeloop:index=seo_doctor_findings_project_id_kind_key
create unique index concurrently if not exists seo_doctor_findings_project_id_kind_key
  on seo_doctor_findings (project_id, id, finding_kind);
