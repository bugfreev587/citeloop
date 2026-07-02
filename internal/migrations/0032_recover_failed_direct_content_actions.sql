update content_actions
set
  status = 'ready_for_review',
  updated_at = now()
where status = 'failed'
  and approved_at is null
  and published_at is null
  and verified_at is null
  and coalesce(verification_snapshot, '{}'::jsonb) = '{}'::jsonb
  and coalesce(outcome_summary, '{}'::jsonb) = '{}'::jsonb
  and (
    lower(coalesce(asset_type, '')) in (
      'metadata_rewrite',
      'internal_link_patch',
      'schema_patch',
      'sitemap_update',
      'technical_fix'
    )
    or lower(coalesce(output_snapshot->>'output_type', '')) in ('direct_patch', 'technical_task')
    or lower(coalesce(diff_snapshot->>'output_type', '')) in ('direct_patch', 'technical_task')
    or lower(coalesce(action_type, '')) like any (
      array[
        '%metadata%',
        '%title%',
        '%meta description%',
        '%internal link%',
        '%schema%',
        '%sitemap%',
        '%technical seo%',
        '%robots%',
        '%canonical%',
        '%crawler%'
      ]
    )
  );
