-- Keep legacy migration selection and database writer fencing aligned with
-- every immediate repair/optimization family emitted by Doctor.
create or replace function is_legacy_doctor_technical_opportunity(opportunity_type text, evidence jsonb)
returns boolean language sql immutable as $$
  select lower(coalesce(opportunity_type, '')) in (
    'schema_gap','structured_data_missing','json_ld_missing','schema_missing',
    'indexing_anomaly','technical_visibility_issue','robots_blocked','robots_conflict',
    'geo_crawler_access_blocked','noindex','noindex_conflict',
    'canonical_missing','canonical_mismatch','canonical_invalid','canonical_multiple',
    'broken_url','soft_404','redirect_loop','redirect_chain',
    'title_missing','missing_title','metadata_title','meta_description_missing',
    'metadata_description','h1_missing','internal_link_gap','zero_internal_links',
    'broken_internal_link','orphan_page','important_page_missing_from_sitemap',
    'sitemap_update','unsafe_mdx_detected','metadata_readability',
    'duplicate_metadata_template','supported_fact_extractability',
    'source_association','entity_naming_consistency'
  ) or (
    lower(coalesce(opportunity_type, '')) in ('direct_patch','metadata_rewrite')
    and lower(coalesce(evidence->>'work_type', evidence->>'owner', '')) in ('fix_site_issue','doctor')
    and case when jsonb_typeof(coalesce(evidence->'added_propositions', '[]'::jsonb)) = 'array'
      then jsonb_array_length(coalesce(evidence->'added_propositions', '[]'::jsonb)) = 0 else false end
  );
$$;
