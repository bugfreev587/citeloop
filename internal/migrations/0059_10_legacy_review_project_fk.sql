set local lock_timeout = '5s';
set local statement_timeout = '4min';

alter table work_review_memory drop constraint if exists work_review_memory_legacy_review_state_id_fkey;
alter table work_review_memory drop constraint if exists work_review_memory_legacy_review_state_project_fk;
alter table work_review_memory add constraint work_review_memory_legacy_review_state_project_fk
  foreign key (project_id, legacy_review_state_id)
  references seo_opportunity_review_states(project_id, id)
  on delete set null (legacy_review_state_id) not valid;
alter table work_review_memory validate constraint work_review_memory_legacy_review_state_project_fk;

reset statement_timeout;
reset lock_timeout;
