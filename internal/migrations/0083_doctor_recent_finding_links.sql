-- Persist per-Site-Fix dismissal of Doctor's presentation-only forward link.
-- Canonical provenance and lifecycle state remain unchanged.

set local lock_timeout = '5s';
set local statement_timeout = '4min';

alter table site_fixes
  add column if not exists doctor_link_dismissed_at timestamptz,
  add column if not exists doctor_link_dismissed_by text;

alter table site_fixes
  drop constraint if exists site_fixes_doctor_link_dismissal_pair,
  add constraint site_fixes_doctor_link_dismissal_pair check (
    (doctor_link_dismissed_at is null and doctor_link_dismissed_by is null)
    or
    (doctor_link_dismissed_at is not null and doctor_link_dismissed_by is not null)
  ) not valid;

alter table site_fixes
  validate constraint site_fixes_doctor_link_dismissal_pair;
