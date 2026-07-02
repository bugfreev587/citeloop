insert into seo_asset_types
  (key, name, description, default_risk_level, default_measurement_window_days,
   supported_publication_surfaces, requires_evidence, requires_review_by_default, default_generation_path)
values
  (
    'technical_fix',
    'Technical fix',
    'Crawl, indexability, canonical, robots, rendering, or tracking task.',
    'medium',
    28,
    '["owned_site"]',
    false,
    true,
    'technical_task'
  )
on conflict (key) do update set
  name = excluded.name,
  description = excluded.description,
  default_risk_level = excluded.default_risk_level,
  default_measurement_window_days = excluded.default_measurement_window_days,
  supported_publication_surfaces = excluded.supported_publication_surfaces,
  requires_evidence = excluded.requires_evidence,
  requires_review_by_default = excluded.requires_review_by_default,
  default_generation_path = excluded.default_generation_path,
  updated_at = now();
