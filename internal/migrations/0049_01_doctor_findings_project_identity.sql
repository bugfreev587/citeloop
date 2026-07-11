set local lock_timeout = '5s';
set local statement_timeout = '30s';

create unique index if not exists seo_doctor_findings_project_id_kind_key
  on seo_doctor_findings (project_id, id, finding_kind);

reset statement_timeout;
reset lock_timeout;
