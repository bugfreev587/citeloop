-- Search Console activation states after OAuth property selection.

alter table seo_integrations drop constraint if exists seo_integrations_status_check;
alter table seo_integrations
  add constraint seo_integrations_status_check
  check (status in (
    'missing',
    'connected',
    'property_selection_required',
    'backfilling',
    'stale',
    'mismatch',
    'expired',
    'error',
    'revoked'
  ));
