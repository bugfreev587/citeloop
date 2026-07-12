-- Reservation creates the canonical work object (and its migration alias)
-- before inserting the enforced signature in the same transaction. Defer this
-- one FK until commit so the atomic reservation can establish both rows.
alter table growth_opportunity_work_aliases
  alter constraint growth_opportunity_work_alias_project_id_work_signature_id_fkey
  deferrable initially deferred;
