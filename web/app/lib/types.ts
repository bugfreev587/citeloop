export type DoctorFindingKind = "broken" | "optimization" | "healthy";

export type DoctorHealthyCoverage = {
  check: string;
  checked_urls: string[];
  passed_urls: string[];
  failed_urls: string[];
  skipped_urls: string[];
};

export type SiteFixStatus =
  | "proposed"
  | "approved"
  | "preparing"
  | "ready_to_apply"
  | "applying"
  | "awaiting_deploy"
  | "verifying"
  | "failed_retryable"
  | "reopened"
  | "verified"
  | "failed_terminal"
  | "superseded"
  | "migration_rolled_back";

export type SiteFixType =
  | "title_readability"
  | "metadata_format"
  | "metadata_ctr_optimization"
  | "search_title_keyword_optimization"
  | "canonical_repair"
  | "robots_repair"
  | "sitemap_repair"
  | "redirect_or_http_repair"
  | "schema_validity_repair"
  | "schema_entity_optimization"
  | "internal_link_patch"
  | "internal_link_authority_optimization"
  | "geo_entity_clarity"
  | "geo_citation_optimization"
  | "geo_content_clarity"
  | "content_typo_or_clarity"
  | "content_rewrite_for_search"
  | "content_demand_expansion"
  | "external_distribution"
  | "conversion_or_cta_optimization"
  | "metadata_rewrite"
  | "schema_patch"
  | "technical_fix"
  | "unknown";

export type SiteFixImpactMode =
  | "unclassified"
  | "presentation_only"
  | "technical_reliability"
  | "search_visibility"
  | "geo_visibility"
  | "content_demand"
  | "conversion_or_ctr";

export type SiteFixMeasurementPolicy = "verification_only" | "measurement_required" | "measurement_optional";
export type SiteFixMeasurementHandoffStatus = "not_applicable" | "not_started" | "pending" | "started" | "failed";
export type SiteFixMeasurementStatus =
  | "planned"
  | "baseline_blocked"
  | "ready"
  | "observing"
  | "terminal"
  | "failed_retryable"
  | "failed_terminal";
export type SiteFixMeasurementOutcome = "positive" | "negative" | "mixed" | "inconclusive" | "insufficient_data";
export type SiteFixAttributionConfidence = "high" | "medium" | "low" | "none";

export type SiteFixMeasurementSummary = {
  source_type: "site_fix";
  id: string;
  project_id: string;
  site_fix_id: string;
  measurement_generation: number;
  status: SiteFixMeasurementStatus;
  target_url: string;
  fix_type: SiteFixType;
  impact_mode: SiteFixImpactMode;
  prospective_observation: boolean;
  growth_hypothesis: string;
  primary_metric: string;
  secondary_metrics: string[];
  baseline_status: string;
  started_at?: string | null;
  absolute_terminal_at?: string | null;
  terminal_outcome?: SiteFixMeasurementOutcome | null;
  outcome_reason?: string | null;
  attribution_confidence: SiteFixAttributionConfidence;
  results_deep_link: string;
  site_fix_status: SiteFixStatus;
  verified_at?: string | null;
  created_at?: string | null;
  updated_at?: string | null;
};

export type ResultsSiteFixCheckpoint = {
  id: string;
  checkpoint_key: string;
  checkpoint_role: string;
  scheduled_at?: string | null;
  window_start?: string | null;
  window_end?: string | null;
  attempt_number: number;
  outcome_label?: SiteFixMeasurementOutcome | null;
  outcome_reason?: string | null;
  attribution_confidence: SiteFixAttributionConfidence;
  computed_at?: string | null;
  retry_classification: string;
};

export type ResultsSiteFixTerminal = {
  id: string;
  outcome_label: SiteFixMeasurementOutcome;
  record_kind: string;
  terminal_reason: string;
  created_at?: string | null;
};

export type ResultsSiteFixPublic = {
  id: string;
  status: SiteFixStatus;
  finding_kind: Exclude<DoctorFindingKind, "healthy">;
  target_urls: string[];
  fix_type: SiteFixType;
  impact_mode: SiteFixImpactMode;
  measurement_policy: SiteFixMeasurementPolicy;
  verified_at?: string | null;
};

export type ResultsSiteFixMeasurementDetail = {
  source_type: "site_fix";
  measurement: SiteFixMeasurementSummary;
  site_fix: ResultsSiteFixPublic;
  checkpoints: ResultsSiteFixCheckpoint[];
  terminal?: ResultsSiteFixTerminal | null;
  measurement_summary: SiteFixMeasurementSummary;
  measurement_handoff_status: SiteFixMeasurementHandoffStatus;
};

export type SiteFixVerification = {
  id: string;
  project_id: string;
  site_fix_id: string;
  attempt_number: number;
  evidence_read: unknown;
  acceptance_results: unknown;
  ai_call_id?: string | null;
  result: string;
  retry_classification: string;
  failure_reason?: string | null;
  attempted_at?: string | null;
  created_at?: string | null;
};

export type SiteFixLegacyAlias = {
  object_type: string;
  object_id: string;
};

export type SiteChangeApplication = {
  id: string;
  project_id?: string;
  site_fix_id: string;
  application_kind: string;
  target_url: string;
  normalized_target_url: string;
  source_file_paths: string[];
  source_mapping_confidence: string;
  source_mapping_reason: string;
  repo_full_name?: string | null;
  base_branch?: string | null;
  working_branch?: string | null;
  base_commit_sha?: string | null;
  head_commit_sha?: string | null;
  patch_snapshot: unknown;
  diff_snapshot: unknown;
  resolution_criteria: unknown;
  github_pr_number?: number | null;
  github_pr_url?: string | null;
  github_pr_state?: string | null;
  deployment_snapshot: unknown;
  verification_snapshot: unknown;
  failure_reason?: string | null;
  status: string;
  created_at?: string | null;
  updated_at?: string | null;
  pr_created_at?: string | null;
  merged_at?: string | null;
  deployed_at?: string | null;
  verified_at?: string | null;
};

export type SiteFix = {
  id: string;
  project_id: string;
  doctor_finding_id: string;
  candidate_id: string;
  work_signature_id: string;
  supersedes_site_fix_id?: string | null;
  status: SiteFixStatus;
  fix_type: SiteFixType;
  impact_mode: SiteFixImpactMode;
  measurement_policy: SiteFixMeasurementPolicy;
  classifier_version: string;
  decision_origin: string;
  decision_confidence: "high" | "medium" | "low";
  growth_hypothesis?: string | null;
  primary_metric?: string | null;
  secondary_metrics: string[];
  measurement_policy_version?: string | null;
  measurement_summary?: SiteFixMeasurementSummary | null;
  measurement_handoff_status: SiteFixMeasurementHandoffStatus;
  finding_kind: Exclude<DoctorFindingKind, "healthy">;
  target_urls: string[];
  evidence_snapshot: unknown;
  proposed_fix: unknown;
  acceptance_tests: unknown[];
  verification_snapshot: unknown;
  failure_reason?: string | null;
  retry_count: number;
  max_retries: number;
  legacy_opportunity_id?: string | null;
  legacy_content_action_id?: string | null;
  migration_batch_id?: string | null;
  legacy_aliases?: SiteFixLegacyAlias[];
  approved_at?: string | null;
  applied_at?: string | null;
  deployed_at?: string | null;
  verified_at?: string | null;
  doctor_link_dismissed_at?: string | null;
  doctor_link_dismissed_by?: string | null;
  created_at?: string | null;
  updated_at?: string | null;
  application?: SiteChangeApplication | null;
  verifications?: SiteFixVerification[];
};

export type SiteFixLifecycleResult = {
  site_fix: SiteFix;
  application: SiteChangeApplication;
};
