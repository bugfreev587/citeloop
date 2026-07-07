update projects
set config =
  jsonb_set(
    jsonb_set(coalesce(config, '{}'::jsonb), '{publish_mode}', '"scheduled"'::jsonb, true),
    '{publish_interval_days}',
    to_jsonb(
      case
        when nullif(config->>'publish_interval_days', '') ~ '^[0-9]+$'
          and (config->>'publish_interval_days')::int > 0
          then (config->>'publish_interval_days')::int
        else 1
      end
    ),
    true
  )
where config->>'publish_mode' = 'auto';
