-- Rewrite legacy UniPost preview-domain publish URLs to the production domain.
-- The GitHub publisher now uses https://unipost.dev, and this backfills rows
-- already written while the publisher connection pointed at dev.unipost.dev.
update articles
set canonical_url = case
      when canonical_url ~* '^https?://dev\.unipost\.dev'
      then regexp_replace(canonical_url, '^https?://dev\.unipost\.dev', 'https://unipost.dev', 'i')
      else canonical_url
    end,
    publish_result = case
      when publish_result ? 'url'
        and publish_result->>'url' ~* '^https?://dev\.unipost\.dev'
      then jsonb_set(
        publish_result,
        '{url}',
        to_jsonb(regexp_replace(publish_result->>'url', '^https?://dev\.unipost\.dev', 'https://unipost.dev', 'i'))
      )
      else publish_result
    end
where canonical_url ~* '^https?://dev\.unipost\.dev'
   or (
    publish_result ? 'url'
    and publish_result->>'url' ~* '^https?://dev\.unipost\.dev'
   );

update publisher_connections
set config = jsonb_set(
  config,
  '{base_url}',
  to_jsonb(regexp_replace(config->>'base_url', '^https?://dev\.unipost\.dev', 'https://unipost.dev', 'i'))
)
where kind = 'github_nextjs'
  and config ? 'base_url'
  and config->>'base_url' ~* '^https?://dev\.unipost\.dev';
