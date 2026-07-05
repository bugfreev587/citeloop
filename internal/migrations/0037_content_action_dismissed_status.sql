alter table content_actions drop constraint if exists content_actions_status_check;
alter table content_actions
  add constraint content_actions_status_check
  check (status in (
    'drafting','ready_for_review','approved','published','measuring','completed','failed',
    'verification_failed','recovery_required','dismissed'
  ));
