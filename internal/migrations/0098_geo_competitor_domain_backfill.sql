-- Backfill active GEO competitor domains when legacy rows stored a URL or
-- bare domain in the competitor name or aliases. This intentionally only
-- fills empty domain arrays; manual edits and newer profile/search-result
-- writes with domains are preserved.

with competitor_text as (
  select
    competitor.id,
    lower(
      competitor.name || ' ' || coalesce((
        select string_agg(alias.value, ' ')
        from jsonb_array_elements_text(competitor.aliases) as alias(value)
      ), '')
    ) as raw_text
  from geo_competitors competitor
  where competitor.status = 'active'
    and competitor.domains = '[]'::jsonb
),
matched_domains as (
  select
    id,
    (regexp_match(
      raw_text,
      '(?:https?://)?(?:www\.)?([a-z0-9][a-z0-9-]*(?:\.[a-z0-9][a-z0-9-]*)+)(?:[/:?#\s]|$)'
    ))[1] as domain
  from competitor_text
)
update geo_competitors
set
  domains = jsonb_build_array(matched_domains.domain),
  updated_at = now()
from matched_domains
where geo_competitors.id = matched_domains.id
  and geo_competitors.status = 'active'
  and geo_competitors.domains = '[]'::jsonb
  and matched_domains.domain is not null;
