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
