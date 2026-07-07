import {
  Article,
  GenerationRun,
  InventoryItem,
  ProductProfile,
  RawPgNumeric,
  Topic,
  normalizeArticle,
  normalizeInventoryItem,
  normalizeProfile,
  normalizeRun,
  normalizeTopic,
} from "./normalize";

const BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
const MISSING_PROJECT_DETAIL = "Connect your domain to create your first project.";

export type { Article, GenerationRun, InventoryItem, ProductProfile, Topic };

export type AuthOptions = {
  token?: string | null;
  getToken?: () => Promise<string | null>;
  timeoutMs?: number;
};

const DEFAULT_API_TIMEOUT_MS = 8000;
const ADMIN_DESTRUCTIVE_DELETE_TIMEOUT_MS = 120_000;
const READ_TIMEOUT_RETRIES = 1;

function apiTimeoutMs(auth?: AuthOptions) {
  const configured = auth?.timeoutMs ?? Number(process.env.NEXT_PUBLIC_API_TIMEOUT_MS);
  return Number.isFinite(configured) && configured > 0 ? configured : DEFAULT_API_TIMEOUT_MS;
}

function withMinimumTimeout(auth: AuthOptions | undefined, timeoutMs: number): AuthOptions {
  if (apiTimeoutMs(auth) >= timeoutMs) return auth ?? {};
  return { ...auth, timeoutMs };
}

function isReadRequest(init?: RequestInit) {
  return (init?.method ?? "GET").toUpperCase() === "GET";
}

function parseErrorBody(body: string): Record<string, unknown> {
  try {
    const parsed = JSON.parse(body);
    return parsed && typeof parsed === "object" ? parsed : {};
  } catch {
    return {};
  }
}

export class ApiError extends Error {
  status: number;
  body: string;
  apiMessage: string;

  constructor(status: number, body: string) {
    const payload = parseErrorBody(body);
    const detail = typeof payload.error === "string" ? payload.error : typeof payload.last_error === "string" ? payload.last_error : "";
    const apiMessage = detail.trim() || body;
    super(`${status}: ${apiMessage}`);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
    this.apiMessage = apiMessage;
  }
}

export function isProjectMissingError(error: unknown) {
  if (!(error instanceof ApiError)) return false;
  const normalized = error.apiMessage.trim().toLowerCase();
  return (
    (error.status === 400 && normalized === "bad project id") ||
    (error.status === 404 && normalized === "project not found")
  );
}

export function friendlyApiError(error: unknown) {
  if (isProjectMissingError(error)) {
    return MISSING_PROJECT_DETAIL;
  }
  if (error instanceof ApiError) {
    if (error.status === 401 || error.status === 403) {
      return "Your session cannot access this project. Sign in again or switch accounts.";
    }
    if (error.status >= 500) {
      return "CiteLoop could not load this data. Try again in a moment.";
    }
    const detail = error.apiMessage.trim();
    if (detail && !detail.startsWith("{") && !detail.startsWith("[")) {
      return detail;
    }
    return "CiteLoop could not complete this request.";
  }
  return error instanceof Error && error.message
    ? error.message
    : "CiteLoop could not complete this request.";
}

export type ProjectConfig = {
  site_url?: string;
  cadence_per_week: number;
  buffer_days: number;
  channel_mix: { blog: number; syndication: number };
  brand_voice?: string;
  monthly_budget_usd: number;
  auto_advance_enabled: boolean;
  opportunity_finding_source_mix: OpportunityFindingSourceMix;
  ai_discovery_automation: AIDiscoveryAutomation;
  publish_mode?: "scheduled" | "manual";
  publish_interval_days?: number;
  crawl: {
    same_origin_only: boolean;
    max_pages: number;
    max_depth: number;
    request_timeout_ms: number;
    rate_limit_rps: number;
    respect_robots: boolean;
    sitemap_url_cap: number;
  };
};

export type OpportunityFindingSourceMix = "all" | "signal_scan" | "ai_discovery";
export type AIDiscoveryAutomation = "automatic" | "semi_automatic" | "manual";

export type Project = {
  id: string;
  owner_id?: string;
  name: string;
  slug: string;
  config: ProjectConfig;
  created_at?: any;
  updated_at?: any;
};

export type AdminProject = Project & {
  owner_id: string;
  owner_email: string;
};

export type AdminUser = {
  owner_id: string;
  owner_email: string;
  project_count: number;
  created_at?: any;
  updated_at?: any;
};

export type AdminUserDeleteResult = {
  owner_id: string;
  owner_email: string;
  deleted_projects: number;
};

export type ReviewGroup = { topic_id: string; articles: Article[] };

export type GenerateTopicResult = {
  status: "generating" | "ready";
  topic?: Topic;
  articles: Article[];
};

export type DistributeItem = {
  article: Article;
  compose_url: string;
  supports_canonical: boolean;
};

export type LLMProvider = "tokengate";
export type GEOProviderScope = "perplexity" | "openai" | "anthropic" | "gemini";

export type CrawlSummary = {
  landing_url?: string;
  discovered_count?: number;
  fetched_count?: number;
  inventory_count?: number;
  truncated?: boolean;
  errors?: string[];
  sample_urls?: string[];
};

export type LLMCredentialsStatus = {
  provider: LLMProvider;
  configured: boolean;
  key_tail?: string;
  base_url?: string;
  model?: string;
  writer_model?: string;
  qa_model?: string;
  updated_at?: string;
};

export type GEOCredentialsStatus = {
  scope: GEOProviderScope;
  provider: LLMProvider;
  configured: boolean;
  enabled: boolean;
  key_tail?: string;
  base_url?: string;
  model?: string;
  updated_at?: string;
};

export type GEOCredentialsUpdate = {
  provider?: LLMProvider;
  api_key?: string;
  base_url?: string;
  model?: string;
  enabled?: boolean;
};

export type ProviderTestResult = {
  ok: boolean;
  provider?: string;
  model?: string;
  latency_ms?: number;
  sample?: string;
  cost_usd?: number;
  error?: string;
};

export type InsightResult = {
  profile: any;
  inventory_count: number;
  crawl_summary?: CrawlSummary;
  background_crawl?: boolean;
};

export type RunListOptions = {
  agent?: string;
  status?: string;
  limit?: number;
  cursor?: string;
};

export type TopicUpdateInput = {
  channel?: string;
  title?: string;
  target_keyword?: string;
  target_prompt?: string;
  angle?: string;
  format?: string;
  priority?: number;
  internal_links?: any[];
};

export type TopicCreateInput = TopicUpdateInput & {
  scheduled_at?: string | null;
};

export type NotificationChannelKind = "slack_webhook" | "discord_webhook";

export type NotificationChannel = {
  id: string;
  project_id: string;
  kind: NotificationChannelKind;
  config: { redacted_url?: string };
  label: string;
  verified_at?: any;
  created_at?: any;
  deleted_at?: any;
};

export type NotificationEvent = {
  type: string;
};

export type NotificationSubscription = {
  id: string;
  project_id: string;
  event_type: string;
  channel_id: string;
  enabled: boolean;
  filter?: any;
  created_at?: any;
};

export type NotificationDelivery = {
  id: string;
  project_id: string;
  subscription_id?: any;
  channel_id: string;
  event_type: string;
  event_id: string;
  payload?: any;
  status: "pending" | "sent" | "dead";
  attempts: number;
  next_retry_at?: any;
  last_error?: string | null;
  delivered_at?: any;
  created_at?: any;
};

export type NotificationDeliveryListOptions = {
  status?: string;
  limit?: number;
};

export type PublisherConnection = {
  id: string;
  project_id: string;
  kind: "github_nextjs" | "webhook" | "wordpress" | "dev_to" | string;
  label: string;
  status: "missing" | "connected" | "error" | "revoked" | string;
  is_default: boolean;
  enabled: boolean;
  capabilities: Record<string, boolean>;
  capability_schema_version: number;
  credential_configured: boolean;
  config: {
    repo?: string;
    branch?: string;
    content_dir?: string;
    base_url?: string;
    publish_mode?: string;
    username?: string;
  };
  last_verified_at?: any;
  last_error?: string | null;
};

export type GitHubNextJSPublisherInput = {
  label?: string;
  repo: string;
  branch?: string;
  content_dir?: string;
  base_url: string;
  publish_mode?: string;
  credential_ref?: string;
};

export type DevToPublisherInput = {
  label?: string;
  username?: string;
};

export type PublisherCredentialInput = {
  kind: "github_token" | "dev_to_api_key";
  value: string;
};

export type GithubRepo = {
  full_name: string;
  default_branch: string;
  private: boolean;
};

export type GithubIntegrationStatus = {
  configured: boolean;
  connected: boolean;
  installation_id?: string;
  repo?: string;
  branch?: string;
  content_dir?: string;
  base_url?: string;
  install_url?: string;
  reusable_installation_id?: string;
};

export type SEOIntegration = {
  id: string;
  project_id: string;
  provider: string;
  status:
    | "missing"
    | "connected"
    | "property_selection_required"
    | "backfilling"
    | "stale"
    | "mismatch"
    | "expired"
    | "error"
    | "revoked"
    | string;
  credential_ref?: string | null;
  last_verified_at?: any;
  last_error?: string | null;
};

export type SEOProperty = {
  id: string;
  project_id: string;
  site_url: string;
  gsc_site_url?: string | null;
  ga4_property_id?: string | null;
  url_normalization_config?: any;
  default_country?: string | null;
  default_language?: string | null;
};

export type SetupChecklistItem = {
  key: string;
  label: string;
  status: "not_started" | "in_progress" | "connected" | "optional" | "blocked" | string;
  why_needed?: string;
  next_action?: string;
  capability_impact?: string;
};

export type SEOOverview = {
  property?: SEOProperty | null;
  integrations: SEOIntegration[];
  setup_checklist: SetupChecklistItem[];
  capability_mode: "public_only" | "managed_content_connected" | "customer_site_pending_verification" | "customer_site_connected" | string;
  last_28_days: {
    clicks_28d?: RawPgNumeric;
    impressions_28d?: RawPgNumeric;
    ctr_28d?: RawPgNumeric;
    position_28d?: RawPgNumeric;
    gsc_days_28d?: number;
  };
  technical: {
    checked_urls?: number;
    ok_urls?: number;
    anomaly_urls?: number;
  };
  opportunities_by_type: Array<{ type: string; status: string; count: number }>;
  actions_by_status: Array<{ status: string; count: number }>;
  cold_start: boolean;
  handoff_ready_for_autopilot: boolean;
  data_source_warnings?: string[];
};

export type OpportunityFindingSummaryItem = {
  label: string;
  detail: string;
  tone?: "green" | "amber" | "red" | "neutral" | string;
};

export type OpportunityFindingRun = {
  id: string;
  status: string;
  started_at?: any;
  finished_at?: any;
  duration_ms?: number;
  error?: string | null;
};

export type OpportunityFindingStatus = {
  source_mix: OpportunityFindingSourceMix;
  ai_discovery_automation: AIDiscoveryAutomation;
  manual_mode: boolean;
  last_run?: OpportunityFindingRun | null;
  next_finding_at?: any;
  summary: OpportunityFindingSummaryItem[];
  counts: {
    open: number;
    processed: number;
    in_loop: number;
    total: number;
    by_status: Record<string, number>;
  };
};

export type SEOOpportunity = {
  id: string;
  type: string;
  status: string;
  priority_score?: RawPgNumeric;
  confidence?: RawPgNumeric;
  page_url?: string | null;
  normalized_page_url?: string;
  query?: string | null;
  evidence?: any;
  recommended_action?: string | null;
  expected_impact?: string | null;
  effort?: number;
  risk_level?: string;
  snoozed_until?: any;
  snooze_reason?: string | null;
  unsnoozed_at?: any;
  created_at?: any;
};

export type SEOApprovalSource = "human_review" | "autopilot_policy" | "manual" | "retry_recovery" | "admin_import" | string;
export type SEORoutingSource = "system_recommendation" | "user_override" | "policy" | string;
export type SEOWorkTypeKey = "create_content" | "improve_page" | "fix_site_issue" | string;

export type SEOContentAction = {
  id: string;
  opportunity_id: string;
  action_type: string;
  status: string;
  draft_article_id?: string | null;
  asset_type?: string | null;
  target_surface_id?: string | null;
  target_url?: string | null;
  normalized_target_url?: string | null;
  target_content_hash_before?: string | null;
  risk_reasons?: any;
  evidence_snapshot?: any;
  input_snapshot?: any;
  output_snapshot?: any;
  diff_snapshot?: any;
  review_required?: boolean;
  approved_by?: string | null;
  approved_at?: any;
  verified_at?: any;
  verification_snapshot?: any;
  approval_source?: SEOApprovalSource;
  routing_source?: SEORoutingSource;
  work_type?: SEOWorkTypeKey | null;
  baseline_window?: any;
  measurement_window?: any;
  published_at?: any;
  outcome_summary?: any;
  created_at?: any;
};

export type SEOWatchlistItem = {
  id: string;
  project_id: string;
  source_opportunity_id: string;
  status: "watching" | "due_for_review" | "learned" | "closed" | string;
  observation_window_days: number;
  due_at?: any;
  closed_at?: any;
  created_at?: any;
  opportunity_type?: string;
  opportunity_page_url?: string | null;
  opportunity_query?: string | null;
  opportunity_recommended_action?: string | null;
  opportunity_expected_impact?: string | null;
};

export type SEODoctorRunStatus = "queued" | "running" | "completed" | "failed" | "blocked" | string;
export type SEODoctorStage =
  | "queued"
  | "discovering"
  | "crawling"
  | "checking"
  | "classifying"
  | "writing_report"
  | "handoff"
  | "completed"
  | string;
export type SEODoctorFindingSeverity = "P0" | "P1" | "P2" | "Info" | string;

export type SEODoctorRun = {
  id: string;
  project_id?: string;
  trigger: "onboarding" | "manual" | "weekly" | "post_publish" | string;
  status: SEODoctorRunStatus;
  stage: SEODoctorStage;
  progress_percent: number;
  message: string;
  block_reason?: string | null;
  pages_discovered: number;
  pages_fetched: number;
  pages_checked: number;
  issues_found: number;
  health_score?: number | null;
  input_snapshot?: any;
  output_summary?: any;
  error?: string | null;
  created_by_user_id?: string | null;
  started_at?: any;
  updated_at?: any;
  finished_at?: any;
  created_at?: any;
};

export type SEODoctorFinding = {
  id: string;
  project_id?: string;
  run_id?: string;
  finding_key: string;
  severity: SEODoctorFindingSeverity;
  category: string;
  issue_type: string;
  status: "active" | "resolved" | "dismissed" | "converted" | string;
  affected_urls: string[];
  normalized_urls: string[];
  evidence: any;
  why_it_matters: string;
  fix_intent: string;
  developer_instructions: string;
  likely_files_or_surfaces: string[];
  acceptance_tests: string[];
  risk_level: "low" | "medium" | "high" | string;
  review_required: boolean;
  autofix_eligible: boolean;
  linked_opportunity_id?: string | null;
  linked_content_action_id?: string | null;
  first_seen_at?: any;
  last_seen_at?: any;
  resolved_at?: any;
  updated_at?: any;
};

export type SEODoctorReport = {
  run?: SEODoctorRun | null;
  findings: SEODoctorFinding[];
  human_report?: {
    health_score?: number;
    status?: "healthy" | "needs_attention" | "blocked" | string;
    summary?: string;
    issue_counts?: Record<string, number>;
    checked_urls?: number;
  } | null;
};

export type ActionMeasurement = {
  id: string;
  project_id: string;
  content_action_id: string;
  article_id?: string | null;
  checkpoint_day: number;
  window_start?: any;
  window_end?: any;
  seo_metrics?: any;
  ga4_metrics?: any;
  geo_metrics?: any;
  execution_metrics?: any;
  outcome_label: "insufficient_data" | "positive" | "negative" | "mixed" | "inconclusive" | string;
  outcome_reason: string;
  attribution_confidence: "high" | "medium" | "low" | "none" | string;
  confounders: any[];
  computed_at?: any;
  created_at?: any;
  updated_at?: any;
};

export type ResultsAction = SEOContentAction & {
  opportunity_type?: string;
  opportunity_page_url?: string | null;
  opportunity_normalized_page_url?: string | null;
  opportunity_query?: string | null;
  opportunity_recommended_action?: string | null;
  opportunity_expected_impact?: string | null;
  topic_title?: string | null;
  draft_article_status?: string | null;
  draft_article_canonical_url?: string | null;
  latest_measurement?: ActionMeasurement | null;
  measurements: ActionMeasurement[];
};

export type VisibilityLifecycleStage =
  | "detected"
  | "added_to_plan"
  | "planned"
  | "drafting"
  | "ready_for_review"
  | "approved"
  | "published_or_applied"
  | "measuring"
  | "learned"
  | "blocked";

export type VisibilityLifecycleCounts = Record<VisibilityLifecycleStage, number>;

export type VisibilityActionInLoop = SEOContentAction & {
  lifecycle_stage: VisibilityLifecycleStage;
  draft_article_id?: string | null;
  opportunity_status?: string;
  opportunity_type?: string;
  opportunity_page_url?: string | null;
  opportunity_normalized_page_url?: string | null;
  opportunity_query?: string | null;
  opportunity_recommended_action?: string | null;
  opportunity_expected_impact?: string | null;
  opportunity_risk_level?: string;
  topic_id?: string | null;
  topic_status?: string | null;
  topic_title?: string | null;
  draft_article_status?: string | null;
  draft_article_canonical_url?: string | null;
};

export type VisibilityMeasurementUpdate = {
  action_id: string;
  status: VisibilityLifecycleStage | string;
  summary: string;
};

export type VisibilitySummary = {
  capability_mode: string;
  primary_status: string;
  setup_blockers: SetupChecklistItem[];
  open_opportunities: SEOOpportunity[];
  actions_in_loop: VisibilityActionInLoop[];
  lifecycle_counts: VisibilityLifecycleCounts;
  top_measurement_updates: VisibilityMeasurementUpdate[];
  diagnostics_health: Record<string, any>;
};

export type SEOBrief = {
  mode: "cold_start" | "opportunities" | string;
  title: string;
  generated_at: string;
  actions: SEOOpportunity[];
  blockers: string[];
  geo_blockers: string[];
  geo_opportunities: SEOOpportunity[];
  measurement_updates: string[];
};

export type GSCProperty = {
  site_url: string;
  permission_level: string;
  recommended?: boolean;
};

export type GSCConnection = {
  configured: boolean;
  status:
    | "missing"
    | "connected"
    | "property_selection_required"
    | "backfilling"
    | "stale"
    | "mismatch"
    | "expired"
    | "error"
    | "revoked"
    | string;
  selected_property?: string | null;
  recommended_property?: string | null;
  properties: GSCProperty[];
  account_email?: string | null;
  last_error?: string | null;
};

export type SEOListOptions = {
  type?: string;
  status?: string;
  limit?: number;
  cursor?: string;
};

export type SEOPolicy = {
  id: string;
  autopilot_level: number;
  automation_paused: boolean;
  weekly_action_limit: number;
  monthly_budget_limit?: RawPgNumeric;
  low_traffic_clicks_28d_threshold: number;
  low_traffic_impressions_28d_threshold: number;
  min_confidence_for_auto_publish?: RawPgNumeric;
  quiet_hours_timezone: string;
  quiet_hours_behavior: string;
  kill_switch_enabled: boolean;
  safe_mode_enabled: boolean;
  risk_classifier_version: string;
  recovery_plan_acknowledged_at?: any;
  recovery_plan_acknowledged_by?: string | null;
  recovery_plan_acknowledged?: boolean;
};

export type SEOPolicyUpdateInput = Partial<SEOPolicy> & {
  monthly_budget_limit?: RawPgNumeric | number;
  min_confidence_for_auto_publish?: RawPgNumeric | number;
};

export type SEOObjective = {
  id: string;
  name: string;
  status: string;
  primary_metric: string;
  time_horizon_days: number;
  budget_usd?: RawPgNumeric;
};

export type SEOActionPortfolioItem = {
  opportunity_id?: string;
  type: string;
  recommended_action?: string | null;
  action_bucket: string;
  asset_type?: string | null;
  risk_level: string;
  risk_reasons: string[];
  classifier_version?: string;
  auto_publish_allowed: boolean;
  review_required: boolean;
  measurement_schedule?: any;
};

export type SEOActionPortfolio = {
  selected_actions: SEOActionPortfolioItem[];
  deferred_actions: SEOActionPortfolioItem[];
  rejected_actions: SEOActionPortfolioItem[];
  reason_codes: Record<string, any>;
  policy_snapshot: Record<string, any>;
  budget_snapshot: Record<string, any>;
  risk_summary: Record<string, number>;
  required_approvals: any[];
  measurement_schedule: any[];
};

export type SEOActionPlan = {
  id: string;
  status: string;
  actions: any[];
  portfolio: SEOActionPortfolio;
  aggregate_risk: string;
  risk_classifier_version: string;
  approval_required: boolean;
  created_at?: any;
};

export type SafeModeEvent = {
  id: string;
  reason: string;
  trigger_source: string;
  entered_at?: any;
  entered_by?: string;
  exited_at?: any;
  exited_by?: string;
  exit_reason?: string;
};

export type AutopilotReadinessGate = {
  key:
    | "search_read"
    | "publisher_write"
    | "notification_write"
    | "autopilot_policy_confirmed"
    | "automation_pause_clear"
    | "monthly_budget_configured"
    | "safe_mode_clear"
    | "kill_switch_clear"
    | "rollback_or_recovery_ready"
    | string;
  label: string;
  status: "connected" | "in_progress" | "blocked" | "optional" | string;
  reason: string;
  next_action: string;
  blocking: boolean;
};

export type AutopilotReadiness = {
  ready_for_level_2: boolean;
  autopilot_level: number;
  derived_mode: string;
  automation_paused: boolean;
  safe_mode_active: boolean;
  kill_switch_enabled: boolean;
  failed_gates: string[];
  gates: AutopilotReadinessGate[];
  publisher_capabilities: Record<string, boolean>;
  low_risk_action_types: string[];
  generated_at?: any;
};

export type AutopilotExecuteResult = {
  plan: SEOActionPlan;
  executed_actions: SEOContentAction[];
  deferred_actions: Array<Record<string, any>>;
  readiness: AutopilotReadiness;
  guardrail_results: Array<Record<string, any>>;
  recovery_plans: Array<Record<string, any>>;
  generated_at?: any;
};

export type AICrawlerAccessSnapshot = {
  id: string;
  project_id: string;
  run_id: string;
  page_url: string;
  normalized_page_url: string;
  target_user_agent: string;
  probe_user_agent: string;
  evidence_type: string;
  robots_state: "allowed" | "disallowed" | "unknown" | string;
  http_status?: number | null;
  access_state: "ok" | "blocked" | "challenge" | "rate_limited" | "timeout" | "error" | string;
  confidence: "high" | "medium" | "low" | string;
  inferred: boolean;
  meta_robots_state?: string | null;
  sitemap_state?: string | null;
  body_extractable: boolean;
  raw_details?: any;
  checked_at?: any;
};

export type GEOCrawlerAuditRequest = {
  site_url?: string;
  urls?: string[];
  target_user_agents?: string[];
};

export type GEOCrawlerAuditResult = {
  run?: any;
  snapshots: AICrawlerAccessSnapshot[];
  checked_urls: number;
  created_blockers: number;
  data_source_notes: string[];
};

export type GEORun = {
  id: string;
  project_id?: string;
  agent: string;
  status: string;
  provider?: string;
  started_at?: any;
  finished_at?: any;
  input?: any;
  output?: any;
  error?: string | null;
  cost_usd?: RawPgNumeric;
};

export type GEOPromptSet = {
  id: string;
  project_id: string;
  name: string;
  status: "draft" | "active" | "paused" | "archived" | string;
  locale: string;
  created_by_run_id?: any;
  created_at?: any;
  updated_at?: any;
};

export type GEOPrompt = {
  id: string;
  project_id: string;
  prompt_set_id: string;
  prompt_text: string;
  intent_type: string;
  target_persona: string;
  target_topic: string;
  locale: string;
  target_engines: string[];
  priority: number;
  source: string;
  status: string;
  created_at?: any;
  updated_at?: any;
};

export type GEOPromptUpdateInput = {
  prompt_text?: string;
  intent_type?: string;
  target_persona?: string;
  target_topic?: string;
  locale?: string;
  target_engines?: string[];
  priority?: number;
  source?: string;
  status?: string;
};

export type GEOCompetitor = {
  id: string;
  project_id: string;
  name: string;
  domains: string[];
  aliases: string[];
  source: string;
  status: string;
};

export type GEOCompetitorUpdateInput = {
  name?: string;
  domains?: string[];
  aliases?: string[];
  source?: string;
  status?: string;
};

export type GEOExternalSurface = {
  id: string;
  project_id: string;
  url: string;
  normalized_url: string;
  platform: string;
  surface_type: string;
  owner_type: "project" | "user" | "third_party" | string;
  canonical_target_url?: string | null;
  backlink_state: string;
  last_http_status?: number | null;
  last_cited_at?: any;
  source_url?: string | null;
  canonical_status: string;
  indexability_status: string;
  publication_status: string;
  owner_confidence: string;
  last_verified_at?: any;
  verification_snapshot?: any;
  related_action_ids: string[];
  created_at?: any;
  updated_at?: any;
};

export type GEOObservation = {
  id: string;
  project_id: string;
  run_id: string;
  prompt_id?: any;
  engine: string;
  locale: string;
  source_type: string;
  brand_mentioned: boolean;
  brand_position?: number | null;
  project_citation_count: number;
  project_citation_rank_best?: number | null;
  project_cited_surface_ids: string[];
  cited_urls: string[];
  competitor_mentions: string[];
  competitor_citations: string[];
  observation_state: string;
  answer_summary: string;
  evidence_snippets: string[];
  confidence: string;
  observed_at?: any;
};

export type GEOVisibilityScore = {
  id: string;
  project_id: string;
  run_id?: any;
  score?: RawPgNumeric;
  coverage?: RawPgNumeric;
  confidence: "high" | "medium" | "low" | "insufficient_data" | string;
  breakdown: Record<string, any>;
  prompt_count_total: number;
  prompt_count_observed: number;
  engine_count_observed: number;
  computed_at?: any;
};

export type GEOAssetBrief = {
  id: string;
  project_id: string;
  opportunity_id: string;
  asset_type: string;
  status: "draft" | "ready_for_review" | "accepted" | "converted" | "dismissed" | string;
  target_prompts: string[];
  required_evidence: string[];
  recommended_outline: string[];
  internal_link_plan: string[];
  publication_surface: string;
  created_by_run_id?: any;
  created_at?: any;
  updated_at?: any;
};

export type GEOOverview = {
  score?: GEOVisibilityScore | null;
  prompt_sets: GEOPromptSet[];
  prompts: GEOPrompt[];
  competitors: GEOCompetitor[];
  external_surfaces: GEOExternalSurface[];
  observations: GEOObservation[];
};

export type GEOPromptSetBundle = {
  prompt_sets: GEOPromptSet[];
  prompts: GEOPrompt[];
  competitors: GEOCompetitor[];
};

export type ManualFixtureObservationInput = {
  prompt_id: string;
  answer_summary?: string;
  cited_urls?: string[];
  brand_mentioned: boolean;
  brand_position?: number | null;
  competitor_mentions?: string[];
  competitor_citations?: string[];
  evidence_snippets?: string[];
  project_citation_rank?: number;
  confidence?: string;
};

export type ManualFixtureRequest = {
  engine: string;
  locale?: string;
  observations: ManualFixtureObservationInput[];
};

export type GEOProviderObserveRequest = {
  engine?: string;
  locale?: string;
  max_prompts?: number;
  budget_usd?: number;
};

export type GEOProviderObserveResult = {
  run?: any;
  observations: GEOObservation[];
  score?: GEOVisibilityScore | null;
  cost_usd?: number;
  data_source_notes?: string[];
};

export type GEOExternalSurfaceMonitorRequest = {
  limit?: number;
};

export type GEOExternalSurfaceMonitorResult = {
  run?: any;
  surfaces: GEOExternalSurface[];
  checked: number;
  failed: number;
  data_source_notes?: string[];
};

export function defaultProjectConfig(): ProjectConfig {
  return {
    site_url: "",
    cadence_per_week: 3,
    buffer_days: 5,
    channel_mix: { blog: 0.6, syndication: 0.4 },
    brand_voice: "",
    monthly_budget_usd: 50,
    auto_advance_enabled: false,
    opportunity_finding_source_mix: "all",
    ai_discovery_automation: "semi_automatic",
    publish_mode: "manual",
    publish_interval_days: 2,
    crawl: {
      same_origin_only: true,
      max_pages: 200,
      max_depth: 3,
      request_timeout_ms: 8000,
      rate_limit_rps: 1,
      respect_robots: true,
      sitemap_url_cap: 2000,
    },
  };
}

function normalizeOpportunityFindingSourceMix(value: any): OpportunityFindingSourceMix {
  return value === "signal_scan" || value === "ai_discovery" || value === "all" ? value : "all";
}

function normalizeAIDiscoveryAutomation(value: any): AIDiscoveryAutomation {
  return value === "automatic" || value === "manual" || value === "semi_automatic" ? value : "semi_automatic";
}

function normalizeProjectConfig(raw: any): ProjectConfig {
  const defaults = defaultProjectConfig();
  const data = raw ?? {};
  return {
    ...defaults,
    ...data,
    channel_mix: { ...defaults.channel_mix, ...(data.channel_mix ?? {}) },
    crawl: { ...defaults.crawl, ...(data.crawl ?? {}) },
    opportunity_finding_source_mix: normalizeOpportunityFindingSourceMix(data.opportunity_finding_source_mix),
    ai_discovery_automation: normalizeAIDiscoveryAutomation(data.ai_discovery_automation),
  };
}

function normalizeProject(raw: any): Project {
  return {
    id: raw.id,
    owner_id: raw.owner_id,
    name: raw.name ?? "Untitled project",
    slug: raw.slug ?? raw.id,
    config: normalizeProjectConfig(raw.config),
    created_at: raw.created_at,
    updated_at: raw.updated_at,
  };
}

function normalizeAdminProject(raw: any): AdminProject {
  return {
    ...normalizeProject(raw),
    owner_id: raw.owner_id ?? "",
    owner_email: raw.owner_email ?? "",
  };
}

function normalizeAdminUser(raw: any): AdminUser {
  return {
    owner_id: raw.owner_id ?? "",
    owner_email: raw.owner_email ?? "",
    project_count: Number(raw.project_count ?? 0),
    created_at: raw.created_at,
    updated_at: raw.updated_at,
  };
}

function normalizeAdminUserDeleteResult(raw: any): AdminUserDeleteResult {
  return {
    owner_id: raw.owner_id ?? "",
    owner_email: raw.owner_email ?? "",
    deleted_projects: Number(raw.deleted_projects ?? 0),
  };
}

function normalizeReviewGroup(raw: any): ReviewGroup {
  return {
    topic_id: raw.topic_id,
    articles: Array.isArray(raw.articles) ? raw.articles.map(normalizeArticle) : [],
  };
}

function normalizeGenerateTopicResult(raw: any): GenerateTopicResult {
  if (Array.isArray(raw)) {
    return { status: "ready", articles: raw.map(normalizeArticle) };
  }
  return {
    status: raw?.status === "generating" ? "generating" : "ready",
    topic: raw?.topic ? normalizeTopic(raw.topic) : undefined,
    articles: arrayFrom(raw?.articles).map(normalizeArticle),
  };
}

function normalizeDistributeItem(raw: any): DistributeItem {
  return {
    article: normalizeArticle(raw.article),
    compose_url: raw.compose_url ?? "",
    supports_canonical: Boolean(raw.supports_canonical),
  };
}

function arrayFrom<T = any>(value: any): T[] {
  return Array.isArray(value) ? value : [];
}

function normalizeLLMCredentialsStatus(raw: any): LLMCredentialsStatus {
  return {
    provider: "tokengate",
    configured: Boolean(raw.configured),
    key_tail: raw.key_tail ?? undefined,
    base_url: raw.base_url ?? undefined,
    model: raw.model ?? undefined,
    writer_model: raw.writer_model ?? undefined,
    qa_model: raw.qa_model ?? undefined,
    updated_at: raw.updated_at ?? undefined,
  };
}

function normalizeGEOCredentialsStatus(raw: any): GEOCredentialsStatus {
  return {
    scope: raw?.scope,
    provider: "tokengate",
    configured: Boolean(raw?.configured),
    enabled: Boolean(raw?.enabled),
    key_tail: raw?.key_tail ?? undefined,
    base_url: raw?.base_url ?? undefined,
    model: raw?.model ?? undefined,
    updated_at: raw?.updated_at ?? undefined,
  };
}

function normalizeSEOOverview(raw: any): SEOOverview {
  const data = raw ?? {};
  return {
    property: data.property ?? null,
    integrations: arrayFrom<SEOIntegration>(data.integrations),
    setup_checklist: arrayFrom<SetupChecklistItem>(data.setup_checklist),
    capability_mode: data.capability_mode ?? "public_only",
    last_28_days: data.last_28_days ?? {},
    technical: data.technical ?? {},
    opportunities_by_type: arrayFrom(data.opportunities_by_type),
    actions_by_status: arrayFrom(data.actions_by_status),
    cold_start: Boolean(data.cold_start),
    handoff_ready_for_autopilot: Boolean(data.handoff_ready_for_autopilot),
    data_source_warnings: arrayFrom<string>(data.data_source_warnings).map(String),
  };
}

function normalizeSEOSettings(raw: any): { property?: SEOProperty | null; integrations: SEOIntegration[] } {
  return {
    property: raw?.property ?? null,
    integrations: arrayFrom<SEOIntegration>(raw?.integrations),
  };
}

function normalizeOpportunityFindingStatus(raw: any): OpportunityFindingStatus {
  const data = raw ?? {};
  const counts = data.counts ?? {};
  return {
    source_mix: normalizeOpportunityFindingSourceMix(data.source_mix),
    ai_discovery_automation: normalizeAIDiscoveryAutomation(data.ai_discovery_automation),
    manual_mode: Boolean(data.manual_mode),
    last_run: data.last_run
      ? {
          id: data.last_run.id ?? "",
          status: data.last_run.status ?? "",
          started_at: data.last_run.started_at ?? null,
          finished_at: data.last_run.finished_at ?? null,
          duration_ms: Number(data.last_run.duration_ms ?? 0),
          error: data.last_run.error ?? null,
        }
      : null,
    next_finding_at: data.next_finding_at ?? null,
    summary: arrayFrom<any>(data.summary).map((item) => ({
      label: item?.label ?? "Opportunity Finding",
      detail: item?.detail ?? "",
      tone: item?.tone ?? "neutral",
    })),
    counts: {
      open: Number(counts.open ?? 0),
      processed: Number(counts.processed ?? 0),
      in_loop: Number(counts.in_loop ?? 0),
      total: Number(counts.total ?? 0),
      by_status: counts.by_status ?? {},
    },
  };
}

function normalizeSEOBrief(raw: any): SEOBrief {
  const data = raw ?? {};
  return {
    mode: data.mode ?? "cold_start",
    title: data.title ?? "SEO operating brief",
    generated_at: data.generated_at ?? "",
    actions: arrayFrom<SEOOpportunity>(data.actions),
    blockers: arrayFrom<string>(data.blockers).map(String),
    geo_blockers: arrayFrom<string>(data.geo_blockers).map(String),
    geo_opportunities: arrayFrom<SEOOpportunity>(data.geo_opportunities),
    measurement_updates: arrayFrom<string>(data.measurement_updates).map(String),
  };
}

function normalizeSEOContentAction(raw: any): SEOContentAction {
  const data = raw ?? {};
  return {
    id: data.id ?? "",
    opportunity_id: data.opportunity_id ?? "",
    action_type: data.action_type ?? "",
    status: data.status ?? "",
    asset_type: data.asset_type ?? null,
    target_surface_id: data.target_surface_id ?? null,
    target_url: data.target_url ?? null,
    normalized_target_url: data.normalized_target_url ?? null,
    target_content_hash_before: data.target_content_hash_before ?? null,
    risk_reasons: data.risk_reasons ?? [],
    evidence_snapshot: data.evidence_snapshot ?? {},
    input_snapshot: data.input_snapshot ?? {},
    output_snapshot: data.output_snapshot ?? {},
    diff_snapshot: data.diff_snapshot ?? {},
    review_required: Boolean(data.review_required),
    approved_by: data.approved_by ?? null,
    approved_at: data.approved_at ?? undefined,
    verified_at: data.verified_at ?? undefined,
    verification_snapshot: data.verification_snapshot ?? null,
    baseline_window: data.baseline_window ?? {},
    measurement_window: data.measurement_window ?? {},
    published_at: data.published_at ?? undefined,
    outcome_summary: data.outcome_summary ?? {},
    created_at: data.created_at ?? undefined,
    draft_article_id: data.draft_article_id ?? null,
  };
}

function normalizeStringArray(raw: any): string[] {
  return arrayFrom(raw).map(String).filter((item) => item.trim() !== "");
}

function normalizeSEODoctorRun(raw: any): SEODoctorRun {
  const data = raw ?? {};
  return {
    id: data.id ?? "",
    project_id: data.project_id ?? undefined,
    trigger: data.trigger ?? "manual",
    status: data.status ?? "queued",
    stage: data.stage ?? "queued",
    progress_percent: Number(data.progress_percent ?? 0),
    message: data.message ?? "",
    block_reason: data.block_reason ?? null,
    pages_discovered: Number(data.pages_discovered ?? 0),
    pages_fetched: Number(data.pages_fetched ?? 0),
    pages_checked: Number(data.pages_checked ?? 0),
    issues_found: Number(data.issues_found ?? 0),
    health_score: data.health_score == null ? null : Number(data.health_score),
    input_snapshot: data.input_snapshot ?? {},
    output_summary: data.output_summary ?? {},
    error: data.error ?? null,
    created_by_user_id: data.created_by_user_id ?? null,
    started_at: data.started_at ?? undefined,
    updated_at: data.updated_at ?? undefined,
    finished_at: data.finished_at ?? undefined,
    created_at: data.created_at ?? undefined,
  };
}

function normalizeSEODoctorFinding(raw: any): SEODoctorFinding {
  const data = raw ?? {};
  return {
    id: data.id ?? "",
    project_id: data.project_id ?? undefined,
    run_id: data.run_id ?? undefined,
    finding_key: data.finding_key ?? "",
    severity: data.severity ?? "Info",
    category: data.category ?? "",
    issue_type: data.issue_type ?? "",
    status: data.status ?? "active",
    affected_urls: normalizeStringArray(data.affected_urls),
    normalized_urls: normalizeStringArray(data.normalized_urls),
    evidence: data.evidence ?? {},
    why_it_matters: data.why_it_matters ?? "",
    fix_intent: data.fix_intent ?? "",
    developer_instructions: data.developer_instructions ?? "",
    likely_files_or_surfaces: normalizeStringArray(data.likely_files_or_surfaces),
    acceptance_tests: normalizeStringArray(data.acceptance_tests),
    risk_level: data.risk_level ?? "low",
    review_required: Boolean(data.review_required),
    autofix_eligible: Boolean(data.autofix_eligible),
    linked_opportunity_id: data.linked_opportunity_id ?? null,
    linked_content_action_id: data.linked_content_action_id ?? null,
    first_seen_at: data.first_seen_at ?? undefined,
    last_seen_at: data.last_seen_at ?? undefined,
    resolved_at: data.resolved_at ?? undefined,
    updated_at: data.updated_at ?? undefined,
  };
}

function normalizeSEODoctorReport(raw: any): SEODoctorReport {
  const data = raw ?? {};
  return {
    run: data.run ? normalizeSEODoctorRun(data.run) : null,
    findings: arrayFrom(data.findings).map(normalizeSEODoctorFinding),
    human_report: data.human_report ?? null,
  };
}

function normalizeActionMeasurement(raw: any): ActionMeasurement {
  const data = raw ?? {};
  return {
    id: data.id ?? "",
    project_id: data.project_id ?? "",
    content_action_id: data.content_action_id ?? "",
    article_id: data.article_id ?? null,
    checkpoint_day: Number(data.checkpoint_day ?? 0),
    window_start: data.window_start ?? undefined,
    window_end: data.window_end ?? undefined,
    seo_metrics: data.seo_metrics ?? {},
    ga4_metrics: data.ga4_metrics ?? {},
    geo_metrics: data.geo_metrics ?? {},
    execution_metrics: data.execution_metrics ?? {},
    outcome_label: data.outcome_label ?? "insufficient_data",
    outcome_reason: data.outcome_reason ?? "No comparable before/after data is available yet.",
    attribution_confidence: data.attribution_confidence ?? "low",
    confounders: arrayFrom(data.confounders),
    computed_at: data.computed_at ?? undefined,
    created_at: data.created_at ?? undefined,
    updated_at: data.updated_at ?? undefined,
  };
}

function normalizeResultsAction(raw: any): ResultsAction {
  const data = raw ?? {};
  const measurements = arrayFrom(data.measurements).map(normalizeActionMeasurement);
  const latest = data.latest_measurement ? normalizeActionMeasurement(data.latest_measurement) : measurements[0] ?? null;
  return {
    ...normalizeSEOContentAction(data),
    opportunity_type: data.opportunity_type ?? "",
    opportunity_page_url: data.opportunity_page_url ?? null,
    opportunity_normalized_page_url: data.opportunity_normalized_page_url ?? null,
    opportunity_query: data.opportunity_query ?? null,
    opportunity_recommended_action: data.opportunity_recommended_action ?? null,
    opportunity_expected_impact: data.opportunity_expected_impact ?? null,
    topic_title: data.topic_title ?? null,
    draft_article_status: data.draft_article_status ?? null,
    draft_article_canonical_url: data.draft_article_canonical_url ?? null,
    latest_measurement: latest,
    measurements,
  };
}

const visibilityLifecycleStages: VisibilityLifecycleStage[] = [
  "detected",
  "added_to_plan",
  "planned",
  "drafting",
  "ready_for_review",
  "approved",
  "published_or_applied",
  "measuring",
  "learned",
  "blocked",
];

function normalizeVisibilityLifecycleCounts(raw: any): VisibilityLifecycleCounts {
  const source = raw && typeof raw === "object" && !Array.isArray(raw) ? raw : {};
  return visibilityLifecycleStages.reduce((counts, stage) => {
    counts[stage] = Number(source[stage] ?? 0);
    return counts;
  }, {} as VisibilityLifecycleCounts);
}

function normalizeVisibilityActionInLoop(raw: any): VisibilityActionInLoop {
  const data = raw ?? {};
  return {
    id: data.id ?? "",
    opportunity_id: data.opportunity_id ?? "",
    action_type: data.action_type ?? "",
    status: data.status ?? "",
    lifecycle_stage: visibilityLifecycleStages.includes(data.lifecycle_stage) ? data.lifecycle_stage : "added_to_plan",
    asset_type: data.asset_type ?? null,
    target_surface_id: data.target_surface_id ?? null,
    target_url: data.target_url ?? null,
    normalized_target_url: data.normalized_target_url ?? null,
    target_content_hash_before: data.target_content_hash_before ?? null,
    risk_reasons: data.risk_reasons ?? [],
    evidence_snapshot: data.evidence_snapshot ?? {},
    input_snapshot: data.input_snapshot ?? {},
    output_snapshot: data.output_snapshot ?? {},
    diff_snapshot: data.diff_snapshot ?? {},
    review_required: Boolean(data.review_required),
    approved_by: data.approved_by ?? null,
    approved_at: data.approved_at ?? undefined,
    verified_at: data.verified_at ?? undefined,
    verification_snapshot: data.verification_snapshot ?? null,
    baseline_window: data.baseline_window ?? {},
    measurement_window: data.measurement_window ?? {},
    published_at: data.published_at ?? undefined,
    outcome_summary: data.outcome_summary ?? {},
    created_at: data.created_at ?? undefined,
    draft_article_id: data.draft_article_id ?? null,
    opportunity_status: data.opportunity_status ?? "",
    opportunity_type: data.opportunity_type ?? "",
    opportunity_page_url: data.opportunity_page_url ?? null,
    opportunity_normalized_page_url: data.opportunity_normalized_page_url ?? null,
    opportunity_query: data.opportunity_query ?? null,
    opportunity_recommended_action: data.opportunity_recommended_action ?? null,
    opportunity_expected_impact: data.opportunity_expected_impact ?? null,
    opportunity_risk_level: data.opportunity_risk_level ?? "",
    topic_id: data.topic_id ?? null,
    topic_status: data.topic_status ?? null,
    topic_title: data.topic_title ?? null,
    draft_article_status: data.draft_article_status ?? null,
    draft_article_canonical_url: data.draft_article_canonical_url ?? null,
  };
}

function normalizeVisibilitySummary(raw: any): VisibilitySummary {
  const data = raw ?? {};
  return {
    capability_mode: data.capability_mode ?? "public_only",
    primary_status: data.primary_status ?? "steady",
    setup_blockers: arrayFrom(data.setup_blockers),
    open_opportunities: arrayFrom(data.open_opportunities),
    actions_in_loop: arrayFrom(data.actions_in_loop).map(normalizeVisibilityActionInLoop),
    lifecycle_counts: normalizeVisibilityLifecycleCounts(data.lifecycle_counts),
    top_measurement_updates: arrayFrom(data.top_measurement_updates).map((item: any) => ({
      action_id: item?.action_id ?? "",
      status: item?.status ?? "measuring",
      summary: item?.summary ?? "",
    })),
    diagnostics_health: data.diagnostics_health ?? {},
  };
}

function normalizeGSCConnection(raw: any): GSCConnection {
  const data = raw ?? {};
  return {
    configured: Boolean(data.configured),
    status: data.status ?? "missing",
    selected_property: data.selected_property ?? null,
    recommended_property: data.recommended_property ?? null,
    properties: arrayFrom(data.properties).map((property: any) => ({
      site_url: property?.site_url ?? "",
      permission_level: property?.permission_level ?? "",
      recommended: Boolean(property?.recommended),
    })),
    account_email: data.account_email ?? null,
    last_error: data.last_error ?? null,
  };
}

function normalizePortfolioItem(raw: any): SEOActionPortfolioItem {
  const data = raw ?? {};
  return {
    opportunity_id: data.opportunity_id ? String(data.opportunity_id) : undefined,
    type: data.type ?? "",
    recommended_action: data.recommended_action ?? null,
    action_bucket: data.action_bucket ?? "create new asset",
    asset_type: data.asset_type ?? null,
    risk_level: data.risk_level ?? "low",
    risk_reasons: arrayFrom<string>(data.risk_reasons).map(String),
    classifier_version: data.classifier_version ?? undefined,
    auto_publish_allowed: Boolean(data.auto_publish_allowed),
    review_required: Boolean(data.review_required ?? !data.auto_publish_allowed),
    measurement_schedule: data.measurement_schedule ?? undefined,
  };
}

function normalizeRiskSummary(raw: any): Record<string, number> {
  const data = raw && typeof raw === "object" && !Array.isArray(raw) ? raw : {};
  const out: Record<string, number> = { low: 0, medium: 0, high: 0 };
  for (const [key, value] of Object.entries(data)) {
    out[key] = Number(value ?? 0);
  }
  return out;
}

function normalizeSEOActionPortfolio(raw: any): SEOActionPortfolio {
  const data = Array.isArray(raw) ? { selected_actions: raw } : raw ?? {};
  return {
    selected_actions: arrayFrom(data.selected_actions).map(normalizePortfolioItem),
    deferred_actions: arrayFrom(data.deferred_actions).map(normalizePortfolioItem),
    rejected_actions: arrayFrom(data.rejected_actions).map(normalizePortfolioItem),
    reason_codes: data.reason_codes ?? {},
    policy_snapshot: data.policy_snapshot ?? {},
    budget_snapshot: data.budget_snapshot ?? {},
    risk_summary: normalizeRiskSummary(data.risk_summary),
    required_approvals: arrayFrom(data.required_approvals),
    measurement_schedule: arrayFrom(data.measurement_schedule),
  };
}

function normalizeSEOActionPlan(raw: any): SEOActionPlan {
  const data = raw ?? {};
  const portfolio = normalizeSEOActionPortfolio(data.actions);
  return {
    id: data.id ?? "",
    status: data.status ?? "",
    actions: portfolio.selected_actions,
    portfolio,
    aggregate_risk: data.aggregate_risk ?? "low",
    risk_classifier_version: data.risk_classifier_version ?? "",
    approval_required: Boolean(data.approval_required),
    created_at: data.created_at ?? undefined,
  };
}

function normalizeAICrawlerAccessSnapshot(raw: any): AICrawlerAccessSnapshot {
  const data = raw ?? {};
  return {
    id: data.id ?? "",
    project_id: data.project_id ?? "",
    run_id: data.run_id ?? "",
    page_url: data.page_url ?? "",
    normalized_page_url: data.normalized_page_url ?? "",
    target_user_agent: data.target_user_agent ?? "",
    probe_user_agent: data.probe_user_agent ?? "",
    evidence_type: data.evidence_type ?? "",
    robots_state: data.robots_state ?? "unknown",
    http_status: data.http_status ?? null,
    access_state: data.access_state ?? "unknown",
    confidence: data.confidence ?? "low",
    inferred: Boolean(data.inferred),
    meta_robots_state: data.meta_robots_state ?? null,
    sitemap_state: data.sitemap_state ?? null,
    body_extractable: Boolean(data.body_extractable),
    raw_details: data.raw_details ?? undefined,
    checked_at: data.checked_at ?? undefined,
  };
}

function normalizeGEOCrawlerAuditResult(raw: any): GEOCrawlerAuditResult {
  const data = raw ?? {};
  return {
    run: data.run ?? undefined,
    snapshots: arrayFrom(data.snapshots).map(normalizeAICrawlerAccessSnapshot),
    checked_urls: Number(data.checked_urls ?? 0),
    created_blockers: Number(data.created_blockers ?? 0),
    data_source_notes: arrayFrom<string>(data.data_source_notes).map(String),
  };
}

function normalizeGEOPromptSet(raw: any): GEOPromptSet {
  const data = raw ?? {};
  return {
    id: data.id ?? "",
    project_id: data.project_id ?? "",
    name: data.name ?? "",
    status: data.status ?? "draft",
    locale: data.locale ?? "en-US",
    created_by_run_id: data.created_by_run_id ?? undefined,
    created_at: data.created_at ?? undefined,
    updated_at: data.updated_at ?? undefined,
  };
}

function normalizeGEOPrompt(raw: any): GEOPrompt {
  const data = raw ?? {};
  return {
    id: data.id ?? "",
    project_id: data.project_id ?? "",
    prompt_set_id: data.prompt_set_id ?? "",
    prompt_text: data.prompt_text ?? "",
    intent_type: data.intent_type ?? "",
    target_persona: data.target_persona ?? "",
    target_topic: data.target_topic ?? "",
    locale: data.locale ?? "en-US",
    target_engines: arrayFrom<string>(data.target_engines).map(String),
    priority: Number(data.priority ?? 0),
    source: data.source ?? "",
    status: data.status ?? "active",
    created_at: data.created_at ?? undefined,
    updated_at: data.updated_at ?? undefined,
  };
}

function normalizeGEOCompetitor(raw: any): GEOCompetitor {
  const data = raw ?? {};
  return {
    id: data.id ?? "",
    project_id: data.project_id ?? "",
    name: data.name ?? "",
    domains: arrayFrom<string>(data.domains).map(String),
    aliases: arrayFrom<string>(data.aliases).map(String),
    source: data.source ?? "manual",
    status: data.status ?? "active",
  };
}

function normalizeGEOExternalSurface(raw: any): GEOExternalSurface {
  const data = raw ?? {};
  return {
    id: data.id ?? "",
    project_id: data.project_id ?? "",
    url: data.url ?? "",
    normalized_url: data.normalized_url ?? "",
    platform: data.platform ?? "site",
    surface_type: data.surface_type ?? "page",
    owner_type: data.owner_type ?? "project",
    canonical_target_url: data.canonical_target_url ?? null,
    backlink_state: data.backlink_state ?? "unknown",
    last_http_status: data.last_http_status ?? null,
    last_cited_at: data.last_cited_at ?? undefined,
    source_url: data.source_url ?? null,
    canonical_status: data.canonical_status ?? "unknown",
    indexability_status: data.indexability_status ?? "unknown",
    publication_status: data.publication_status ?? "unknown",
    owner_confidence: data.owner_confidence ?? "medium",
    last_verified_at: data.last_verified_at ?? undefined,
    verification_snapshot: data.verification_snapshot ?? undefined,
    related_action_ids: arrayFrom<string>(data.related_action_ids).map(String),
    created_at: data.created_at ?? undefined,
    updated_at: data.updated_at ?? undefined,
  };
}

function normalizeGEOObservation(raw: any): GEOObservation {
  const data = raw ?? {};
  return {
    id: data.id ?? "",
    project_id: data.project_id ?? "",
    run_id: data.run_id ?? "",
    prompt_id: data.prompt_id ?? undefined,
    engine: data.engine ?? "",
    locale: data.locale ?? "en-US",
    source_type: data.source_type ?? "",
    brand_mentioned: Boolean(data.brand_mentioned),
    brand_position: data.brand_position ?? null,
    project_citation_count: Number(data.project_citation_count ?? 0),
    project_citation_rank_best: data.project_citation_rank_best ?? null,
    project_cited_surface_ids: arrayFrom<string>(data.project_cited_surface_ids).map(String),
    cited_urls: arrayFrom<string>(data.cited_urls).map(String),
    competitor_mentions: arrayFrom<string>(data.competitor_mentions).map(String),
    competitor_citations: arrayFrom<string>(data.competitor_citations).map(String),
    observation_state: data.observation_state ?? "observed",
    answer_summary: data.answer_summary ?? "",
    evidence_snippets: arrayFrom<string>(data.evidence_snippets).map(String),
    confidence: data.confidence ?? "medium",
    observed_at: data.observed_at ?? undefined,
  };
}

function normalizeGEOVisibilityScore(raw: any): GEOVisibilityScore | null {
  if (!raw) return null;
  return {
    id: raw.id ?? "",
    project_id: raw.project_id ?? "",
    run_id: raw.run_id ?? undefined,
    score: raw.score,
    coverage: raw.coverage,
    confidence: raw.confidence ?? "insufficient_data",
    breakdown: raw.breakdown ?? {},
    prompt_count_total: Number(raw.prompt_count_total ?? 0),
    prompt_count_observed: Number(raw.prompt_count_observed ?? 0),
    engine_count_observed: Number(raw.engine_count_observed ?? 0),
    computed_at: raw.computed_at ?? undefined,
  };
}

function normalizeGEORun(raw: any): GEORun {
  const data = raw ?? {};
  return {
    id: data.id ?? "",
    project_id: data.project_id ?? undefined,
    agent: data.agent ?? "",
    status: data.status ?? "",
    provider: data.provider ?? undefined,
    started_at: data.started_at ?? undefined,
    finished_at: data.finished_at ?? undefined,
    input: data.input ?? undefined,
    output: data.output ?? undefined,
    error: data.error ?? null,
    cost_usd: data.cost_usd ?? undefined,
  };
}

function normalizeGEOAssetBrief(raw: any): GEOAssetBrief {
  const data = raw ?? {};
  return {
    id: data.id ?? "",
    project_id: data.project_id ?? "",
    opportunity_id: data.opportunity_id ?? "",
    asset_type: data.asset_type ?? "",
    status: data.status ?? "draft",
    target_prompts: arrayFrom<string>(data.target_prompts).map(String),
    required_evidence: arrayFrom<string>(data.required_evidence).map(String),
    recommended_outline: arrayFrom<string>(data.recommended_outline).map(String),
    internal_link_plan: arrayFrom<string>(data.internal_link_plan).map(String),
    publication_surface: data.publication_surface ?? "blog",
    created_by_run_id: data.created_by_run_id ?? undefined,
    created_at: data.created_at ?? undefined,
    updated_at: data.updated_at ?? undefined,
  };
}

function normalizeGEOOverview(raw: any): GEOOverview {
  const data = raw ?? {};
  return {
    score: normalizeGEOVisibilityScore(data.score),
    prompt_sets: arrayFrom(data.prompt_sets).map(normalizeGEOPromptSet),
    prompts: arrayFrom(data.prompts).map(normalizeGEOPrompt),
    competitors: arrayFrom(data.competitors).map(normalizeGEOCompetitor),
    external_surfaces: arrayFrom(data.external_surfaces).map(normalizeGEOExternalSurface),
    observations: arrayFrom(data.observations).map(normalizeGEOObservation),
  };
}

function normalizeGEOPromptSetBundle(raw: any): GEOPromptSetBundle {
  const data = raw ?? {};
  return {
    prompt_sets: arrayFrom(data.prompt_sets).map(normalizeGEOPromptSet),
    prompts: arrayFrom(data.prompts).map(normalizeGEOPrompt),
    competitors: arrayFrom(data.competitors).map(normalizeGEOCompetitor),
  };
}

function normalizePublisherConnection(raw: any): PublisherConnection {
  const data = raw ?? {};
  return {
    id: data.id ?? "",
    project_id: data.project_id ?? "",
    kind: data.kind ?? "github_nextjs",
    label: data.label ?? "",
    status: data.status ?? "missing",
    is_default: Boolean(data.is_default),
    enabled: Boolean(data.enabled),
    capabilities: data.capabilities ?? {},
    capability_schema_version: Number(data.capability_schema_version ?? 1),
    credential_configured: Boolean(data.credential_configured),
    config: data.config ?? {},
    last_verified_at: data.last_verified_at ?? undefined,
    last_error: data.last_error ?? null,
  };
}

function normalizeGithubRepo(raw: any): GithubRepo {
  const data = raw ?? {};
  return {
    full_name: data.full_name ?? "",
    default_branch: data.default_branch ?? "main",
    private: Boolean(data.private),
  };
}

function normalizeGithubIntegration(raw: any): GithubIntegrationStatus {
  const data = raw ?? {};
  return {
    configured: Boolean(data.configured),
    connected: Boolean(data.connected),
    installation_id: data.installation_id ?? undefined,
    repo: data.repo ?? undefined,
    branch: data.branch ?? undefined,
    content_dir: data.content_dir ?? undefined,
    base_url: data.base_url ?? undefined,
    install_url: data.install_url ?? undefined,
    reusable_installation_id: data.reusable_installation_id ?? undefined,
  };
}

async function bearerHeader(auth?: AuthOptions): Promise<Record<string, string>> {
  const token = auth?.token ?? (auth?.getToken ? await auth.getToken() : null);
  return token ? { Authorization: `Bearer ${token}` } : {};
}

async function req<T>(path: string, init?: RequestInit, auth?: AuthOptions): Promise<T> {
  const authHeader = await bearerHeader(auth);
  const headers = new Headers(init?.headers);
  if (!headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  if (authHeader.Authorization && !headers.has("Authorization")) {
    headers.set("Authorization", authHeader.Authorization);
  }
  const attempts = isReadRequest(init) ? READ_TIMEOUT_RETRIES + 1 : 1;
  let res: Response | null = null;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), apiTimeoutMs(auth));
    try {
      res = await fetch(`${BASE}/api${path}`, {
        ...init,
        headers,
        cache: "no-store",
        signal: controller.signal,
      });
      break;
    } catch (error) {
      if (controller.signal.aborted) {
        if (attempt < attempts - 1) continue;
        throw new Error("CiteLoop API request timed out");
      }
      throw error;
    } finally {
      clearTimeout(timeout);
    }
  }
  if (!res) throw new Error("CiteLoop API request failed");
  if (!res.ok) {
    const body = await res.text();
    throw new ApiError(res.status, body);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export function createApi(auth?: AuthOptions) {
  return {
  getLLMCredentials: async () => {
    const raw = await req<any>("/admin/llm-credentials", undefined, auth);
    return normalizeLLMCredentialsStatus(raw);
  },
  updateLLMCredentials: async (body: { provider?: LLMProvider; api_key?: string; base_url?: string; model?: string; writer_model?: string; qa_model?: string }) => {
    const raw = await req<any>("/admin/llm-credentials", { method: "PUT", body: JSON.stringify(body) }, auth);
    return normalizeLLMCredentialsStatus(raw);
  },
  testLLMCredentials: async () => {
    return req<{ ok: boolean; provider?: string; model?: string; latency_ms?: number; sample?: string; error?: string }>(
      "/admin/llm-credentials/test",
      { method: "POST" },
      auth,
    );
  },
  deleteLLMCredentials: async () => {
    const raw = await req<any>("/admin/llm-credentials", { method: "DELETE" }, auth);
    return normalizeLLMCredentialsStatus(raw);
  },
  listGEOCredentials: async () => {
    const raw = await req<any[]>("/admin/geo-credentials", undefined, auth);
    return arrayFrom(raw).map(normalizeGEOCredentialsStatus);
  },
  updateGEOCredentials: async (scope: GEOProviderScope, body: GEOCredentialsUpdate) => {
    const raw = await req<any>(`/admin/geo-credentials/${encodeURIComponent(scope)}`, { method: "PUT", body: JSON.stringify(body) }, auth);
    return normalizeGEOCredentialsStatus(raw);
  },
  testGEOCredentials: async (scope: GEOProviderScope) => {
    return req<ProviderTestResult>(
      `/admin/geo-credentials/${encodeURIComponent(scope)}/test`,
      { method: "POST" },
      auth,
    );
  },
  deleteGEOCredentials: async (scope: GEOProviderScope) => {
    const raw = await req<any>(`/admin/geo-credentials/${encodeURIComponent(scope)}`, { method: "DELETE" }, auth);
    return normalizeGEOCredentialsStatus(raw);
  },
  getMe: async () => {
    return req<{ user_id: string; email: string; is_admin: boolean }>("/me", undefined, auth);
  },
  listAdminProjects: async () => {
    const raw = await req<any[]>("/admin/projects", undefined, auth);
    return arrayFrom(raw).map(normalizeAdminProject);
  },
  deleteAdminProject: async (id: string) => {
    const raw = await req<any>(
      `/admin/projects/${id}`,
      { method: "DELETE" },
      withMinimumTimeout(auth, ADMIN_DESTRUCTIVE_DELETE_TIMEOUT_MS),
    );
    return normalizeAdminProject(raw);
  },
  listAdminUsers: async () => {
    const raw = await req<any[]>("/admin/users", undefined, auth);
    return arrayFrom(raw).map(normalizeAdminUser);
  },
  deleteAdminUser: async (ownerID: string) => {
    const raw = await req<any>(
      `/admin/users/${encodeURIComponent(ownerID)}`,
      { method: "DELETE" },
      withMinimumTimeout(auth, ADMIN_DESTRUCTIVE_DELETE_TIMEOUT_MS),
    );
    return normalizeAdminUserDeleteResult(raw);
  },
  listProjects: async () => {
    const raw = await req<any[]>("/projects", undefined, auth);
    return arrayFrom(raw).map(normalizeProject);
  },
  createProject: async (body: { name?: string; slug?: string; owner_id?: string; site_url?: string }) => {
    const raw = await req<any>("/projects", { method: "POST", body: JSON.stringify(body) }, auth);
    return normalizeProject(raw);
  },
  deleteProject: async (id: string) => {
    const raw = await req<any>(`/projects/${id}/`, { method: "DELETE" }, auth);
    return normalizeProject(raw);
  },
  getProject: async (id: string) => {
    const raw = await req<any>(`/projects/${id}/`, undefined, auth);
    return normalizeProject(raw);
  },
  updateConfig: async (id: string, config: ProjectConfig) => {
    const raw = await req<any>(`/projects/${id}/config`, { method: "PUT", body: JSON.stringify(config) }, auth);
    return normalizeProject(raw);
  },
  runInsight: (id: string, landingURL: string) =>
    req<InsightResult>(`/projects/${id}/insight`, {
      method: "POST",
      body: JSON.stringify({ landing_url: landingURL }),
    }, auth),
  refreshContext: async (id: string) => {
    const raw = await req<any>(`/projects/${id}/context/refresh`, { method: "POST" }, auth);
    return normalizeProfile(raw);
  },
  getProfile: async (id: string) => {
    const raw = await req<any>(`/projects/${id}/profile`, undefined, auth);
    return normalizeProfile(raw);
  },
  updateProfile: async (id: string, body: { profile: any; source_urls?: any[] }) => {
    const raw = await req<any>(`/projects/${id}/profile`, { method: "PUT", body: JSON.stringify(body) }, auth);
    return normalizeProfile(raw);
  },
  listInventory: async (id: string) => {
    const raw = await req<any[]>(`/projects/${id}/inventory`, undefined, auth);
    return arrayFrom(raw).map(normalizeInventoryItem);
  },
  updateInventory: async (
    id: string,
    itemID: string,
    body: { title?: string; target_keyword?: string; topics?: any[]; summary?: string; evidence_snippets?: any[] },
  ) => {
    const raw = await req<any>(`/projects/${id}/inventory/${itemID}`, { method: "PUT", body: JSON.stringify(body) }, auth);
    return normalizeInventoryItem(raw);
  },
  deleteInventory: (id: string, itemID: string) =>
    req<void>(`/projects/${id}/inventory/${itemID}`, { method: "DELETE" }, auth),
  runStrategist: async (id: string) => {
    const raw = await req<any[]>(`/projects/${id}/strategist`, { method: "POST" }, auth);
    return arrayFrom(raw).map(normalizeTopic);
  },
  listTopics: async (id: string) => {
    const raw = await req<any[]>(`/projects/${id}/topics`, undefined, auth);
    return arrayFrom(raw).map(normalizeTopic);
  },
  createTopic: async (id: string, body: TopicCreateInput) => {
    const raw = await req<any>(`/projects/${id}/topics`, { method: "POST", body: JSON.stringify(body) }, auth);
    return normalizeTopic(raw);
  },
  updateTopic: async (id: string, topicID: string, body: TopicUpdateInput) => {
    const raw = await req<any>(`/projects/${id}/topics/${topicID}`, { method: "PUT", body: JSON.stringify(body) }, auth);
    return normalizeTopic(raw);
  },
  scheduleTopic: async (id: string, topicID: string, scheduledAt: string | null) => {
    const raw = await req<any>(
      `/projects/${id}/topics/${topicID}/schedule`,
      { method: "POST", body: JSON.stringify({ scheduled_at: scheduledAt }) },
      auth,
    );
    return normalizeTopic(raw);
  },
  archiveTopic: async (id: string, topicID: string) => {
    const raw = await req<any>(`/projects/${id}/topics/${topicID}/archive`, { method: "POST" }, auth);
    return normalizeTopic(raw);
  },
  generateTopic: async (id: string, topicID: string) => {
    const raw = await req<any>(`/projects/${id}/topics/${topicID}/generate`, { method: "POST" }, auth);
    return normalizeGenerateTopicResult(raw);
  },
  listReview: async (id: string) => {
    const raw = await req<any[]>(`/projects/${id}/review`, undefined, auth);
    return arrayFrom(raw).map(normalizeReviewGroup);
  },
  listArticles: async (id: string, status: string) => {
    const raw = await req<any[]>(`/projects/${id}/articles?status=${status}`, undefined, auth);
    return arrayFrom(raw).map(normalizeArticle);
  },
  listDistribute: async (id: string) => {
    const raw = await req<any[]>(`/projects/${id}/distribute`, undefined, auth);
    return arrayFrom(raw).map(normalizeDistributeItem);
  },
  listRuns: async (id: string, options: RunListOptions = {}): Promise<GenerationRun[]> => {
    const params = new URLSearchParams();
    if (options.agent) params.set("agent", options.agent);
    if (options.status) params.set("status", options.status);
    if (options.limit) params.set("limit", String(options.limit));
    if (options.cursor) params.set("cursor", options.cursor);
    const suffix = params.toString() ? `?${params}` : "";
    const raw = await req<any[]>(`/projects/${id}/runs${suffix}`, undefined, auth);
    return arrayFrom(raw).map(normalizeRun);
  },
  listPublisherConnections: async (id: string): Promise<PublisherConnection[]> => {
    const raw = await req<any[]>(`/projects/${id}/publisher-connections`, undefined, auth);
    return arrayFrom(raw).map(normalizePublisherConnection);
  },
  upsertGitHubNextJSPublisherConnection: async (
    id: string,
    body: GitHubNextJSPublisherInput,
  ): Promise<PublisherConnection> => {
    const raw = await req<any>(
      `/projects/${id}/publisher-connections/github-nextjs`,
      { method: "PUT", body: JSON.stringify(body) },
      auth,
    );
    return normalizePublisherConnection(raw);
  },
  upsertDevToPublisherConnection: async (
    id: string,
    body: DevToPublisherInput,
  ): Promise<PublisherConnection> => {
    const raw = await req<any>(
      `/projects/${id}/publisher-connections/dev-to`,
      { method: "PUT", body: JSON.stringify(body) },
      auth,
    );
    return normalizePublisherConnection(raw);
  },
  upsertPublisherCredential: async (
    id: string,
    connectionID: string,
    body: PublisherCredentialInput,
  ): Promise<PublisherConnection> => {
    const raw = await req<any>(
      `/projects/${id}/publisher-connections/${connectionID}/credential`,
      { method: "PUT", body: JSON.stringify(body) },
      auth,
    );
    return normalizePublisherConnection(raw);
  },
  testPublisherConnection: async (id: string, connectionID: string): Promise<PublisherConnection> => {
    const raw = await req<any>(`/projects/${id}/publisher-connections/${connectionID}/test`, { method: "POST" }, auth);
    return normalizePublisherConnection(raw);
  },
  setPublisherConnectionEnabled: async (id: string, connectionID: string, enabled: boolean): Promise<PublisherConnection> => {
    const raw = await req<any>(
      `/projects/${id}/publisher-connections/${connectionID}/enabled`,
      { method: "PUT", body: JSON.stringify({ enabled }) },
      auth,
    );
    return normalizePublisherConnection(raw);
  },
  revokePublisherCredential: async (id: string, connectionID: string): Promise<PublisherConnection> => {
    const raw = await req<any>(
      `/projects/${id}/publisher-connections/${connectionID}/credential`,
      { method: "DELETE" },
      auth,
    );
    return normalizePublisherConnection(raw);
  },
  deletePublisherConnection: async (id: string, connectionID: string): Promise<PublisherConnection> => {
    const raw = await req<any>(
      `/projects/${id}/publisher-connections/${connectionID}`,
      { method: "DELETE" },
      auth,
    );
    return normalizePublisherConnection(raw);
  },
  getGithubIntegration: async (id: string): Promise<GithubIntegrationStatus> => {
    const raw = await req<any>(`/projects/${id}/integrations/github`, undefined, auth);
    return normalizeGithubIntegration(raw);
  },
  storeGithubInstallation: async (id: string, installationID: string): Promise<{ repositories: GithubRepo[] }> => {
    const raw = await req<any>(
      `/projects/${id}/integrations/github/installation`,
      { method: "POST", body: JSON.stringify({ installation_id: installationID }) },
      auth,
    );
    return { repositories: arrayFrom(raw?.repositories).map(normalizeGithubRepo) };
  },
  listGithubRepos: async (id: string): Promise<{ repositories: GithubRepo[] }> => {
    const raw = await req<any>(`/projects/${id}/integrations/github/repos`, undefined, auth);
    return { repositories: arrayFrom(raw?.repositories).map(normalizeGithubRepo) };
  },
  selectGithubRepo: async (
    id: string,
    body: { repo: string; branch: string; content_dir: string; base_url: string },
  ): Promise<PublisherConnection> => {
    const raw = await req<any>(
      `/projects/${id}/integrations/github/select-repo`,
      { method: "POST", body: JSON.stringify(body) },
      auth,
    );
    return normalizePublisherConnection(raw);
  },
  listResultsActions: async (id: string, options: SEOListOptions = {}): Promise<ResultsAction[]> => {
    const params = new URLSearchParams();
    if (options.status) params.set("status", options.status);
    if (options.limit) params.set("limit", String(options.limit));
    if (options.cursor) params.set("cursor", options.cursor);
    const suffix = params.toString();
    const endpoint = suffix ? `/projects/${id}/results/actions?${suffix}` : `/projects/${id}/results/actions`;
    const raw = await req<any[]>(endpoint, undefined, auth);
    return arrayFrom(raw).map(normalizeResultsAction);
  },
  getResultsAction: async (id: string, actionID: string): Promise<ResultsAction> => {
    const raw = await req<any>(`/projects/${id}/results/actions/${actionID}`, undefined, auth);
    return normalizeResultsAction(raw);
  },
  recomputeResults: async (id: string): Promise<{ status: string; actions: ResultsAction[] }> => {
    const raw = await req<any>(`/projects/${id}/results/recompute`, { method: "POST" }, auth);
    return {
      status: raw?.status ?? "recomputed",
      actions: arrayFrom(raw?.actions).map(normalizeResultsAction),
    };
  },
  getSEOOverview: async (id: string): Promise<SEOOverview> => {
    const raw = await req<any>(`/projects/${id}/seo/overview`, undefined, auth);
    return normalizeSEOOverview(raw);
  },
  getVisibilitySummary: async (id: string): Promise<VisibilitySummary> => {
    const raw = await req<any>(`/projects/${id}/seo/visibility/summary`, undefined, auth);
    return normalizeVisibilitySummary(raw);
  },
  getSEODoctor: async (id: string): Promise<SEODoctorReport> => {
    const raw = await req<any>(`/projects/${id}/doctor`, undefined, auth);
    return normalizeSEODoctorReport(raw);
  },
  getLatestSEODoctor: async (id: string): Promise<SEODoctorReport> => {
    const raw = await req<any>(`/projects/${id}/doctor/latest`, undefined, auth);
    return normalizeSEODoctorReport(raw);
  },
  startSEODoctorRun: async (id: string, body: { site_url?: string } = {}): Promise<SEODoctorRun> => {
    const raw = await req<any>(`/projects/${id}/doctor/runs`, { method: "POST", body: JSON.stringify(body) }, auth);
    return normalizeSEODoctorRun(raw);
  },
  getSEODoctorRun: async (id: string, runID: string): Promise<SEODoctorRun> => {
    const raw = await req<any>(`/projects/${id}/doctor/runs/${runID}`, undefined, auth);
    return normalizeSEODoctorRun(raw);
  },
  listSEODoctorRunFindings: async (id: string, runID: string): Promise<SEODoctorFinding[]> => {
    const raw = await req<any[]>(`/projects/${id}/doctor/runs/${runID}/findings`, undefined, auth);
    return arrayFrom(raw).map(normalizeSEODoctorFinding);
  },
  dismissSEODoctorFinding: async (id: string, findingID: string): Promise<SEODoctorFinding> => {
    const raw = await req<any>(`/projects/${id}/doctor/findings/${findingID}/dismiss`, { method: "POST" }, auth);
    return normalizeSEODoctorFinding(raw);
  },
  syncSEO: async (id: string, siteURL?: string) => {
    return req<any>(`/projects/${id}/seo/sync`, { method: "POST", body: JSON.stringify({ site_url: siteURL ?? "" }) }, auth);
  },
  analyzeSEO: async (id: string) => {
    return req<any>(`/projects/${id}/seo/analyze`, { method: "POST" }, auth);
  },
  getOpportunityFindingStatus: async (id: string): Promise<OpportunityFindingStatus> => {
    const raw = await req<any>(`/projects/${id}/seo/opportunity-finding/status`, undefined, auth);
    return normalizeOpportunityFindingStatus(raw);
  },
  runOpportunityFinding: async (id: string): Promise<{ status: OpportunityFindingStatus; sync?: any; analyze?: any }> => {
    const raw = await req<any>(`/projects/${id}/seo/opportunity-finding/run`, { method: "POST" }, auth);
    return { ...raw, status: normalizeOpportunityFindingStatus(raw?.status ?? raw) };
  },
  getSEOSettings: async (id: string): Promise<{ property?: SEOProperty | null; integrations: SEOIntegration[] }> => {
    const raw = await req<any>(`/projects/${id}/seo/settings`, undefined, auth);
    return normalizeSEOSettings(raw);
  },
  updateSEOSettings: async (
    id: string,
    body: {
      site_url: string;
      gsc_site_url?: string;
      ga4_property_id?: string;
      url_normalization_config?: any;
      default_country?: string;
      default_language?: string;
      gsc_credential_ref?: string;
    },
  ) => {
    return req<any>(`/projects/${id}/seo/settings`, { method: "PUT", body: JSON.stringify(body) }, auth);
  },
  getGSCConnection: async (id: string): Promise<GSCConnection> => {
    const raw = await req<any>(`/projects/${id}/seo/gsc/connection`, undefined, auth);
    return normalizeGSCConnection(raw);
  },
  startGSCOAuth: async (id: string): Promise<{ authorization_url: string }> => {
    return req<{ authorization_url: string }>(
      `/projects/${id}/seo/gsc/oauth/start`,
      { method: "POST" },
      auth,
    );
  },
  completeGSCOAuth: async (id: string, body: { code: string; state: string }): Promise<GSCConnection> => {
    const raw = await req<any>(
      `/projects/${id}/seo/gsc/oauth/complete`,
      { method: "POST", body: JSON.stringify(body) },
      auth,
    );
    return normalizeGSCConnection(raw);
  },
  selectGSCProperty: async (id: string, body: { site_url: string }): Promise<GSCConnection> => {
    const raw = await req<any>(
      `/projects/${id}/seo/gsc/property`,
      { method: "POST", body: JSON.stringify(body) },
      auth,
    );
    return normalizeGSCConnection(raw);
  },
  revokeGSCConnection: async (id: string): Promise<GSCConnection> => {
    const raw = await req<any>(`/projects/${id}/seo/gsc/revoke`, { method: "POST" }, auth);
    return normalizeGSCConnection(raw);
  },
  getSEOBrief: async (id: string): Promise<SEOBrief> => {
    const raw = await req<any>(`/projects/${id}/seo/briefs/latest`, undefined, auth);
    return normalizeSEOBrief(raw);
  },
  runGEOCrawlerAudit: async (id: string, body: GEOCrawlerAuditRequest = {}): Promise<GEOCrawlerAuditResult> => {
    const raw = await req<any>(`/projects/${id}/geo/crawler-audit`, { method: "POST", body: JSON.stringify(body) }, auth);
    return normalizeGEOCrawlerAuditResult(raw);
  },
  getLatestGEOCrawlerAudit: async (id: string): Promise<{ snapshots: AICrawlerAccessSnapshot[] }> => {
    const raw = await req<any>(`/projects/${id}/geo/crawler-audit/latest`, undefined, auth);
    return { snapshots: arrayFrom(raw?.snapshots).map(normalizeAICrawlerAccessSnapshot) };
  },
  getGEOOverview: async (id: string): Promise<GEOOverview> => {
    const raw = await req<any>(`/projects/${id}/geo/overview`, undefined, auth);
    return normalizeGEOOverview(raw);
  },
  listGEORuns: async (id: string, options: RunListOptions = {}): Promise<GEORun[]> => {
    const params = new URLSearchParams();
    if (options.agent) params.set("agent", options.agent);
    if (options.status) params.set("status", options.status);
    if (options.limit) params.set("limit", String(options.limit));
    if (options.cursor) params.set("cursor", options.cursor);
    const suffix = params.toString() ? `?${params}` : "";
    const raw = await req<any[]>(`/projects/${id}/geo/runs${suffix}`, undefined, auth);
    return arrayFrom(raw).map(normalizeGEORun);
  },
  generateGEOPromptSet: async (id: string, body: { name?: string; locale?: string; status?: string; target_engines?: string[] } = {}) => {
    return req<any>(`/projects/${id}/geo/prompt-sets/generate`, { method: "POST", body: JSON.stringify(body) }, auth);
  },
  listGEOPromptSets: async (id: string, options: { status?: string; prompt_status?: string } = {}): Promise<GEOPromptSetBundle> => {
    const params = new URLSearchParams();
    if (options.status) params.set("status", options.status);
    if (options.prompt_status) params.set("prompt_status", options.prompt_status);
    const suffix = params.toString() ? `?${params}` : "";
    const raw = await req<any>(`/projects/${id}/geo/prompt-sets${suffix}`, undefined, auth);
    return normalizeGEOPromptSetBundle(raw);
  },
  updateGEOPromptSet: async (id: string, promptSetID: string, body: { name?: string; status?: string; locale?: string }): Promise<GEOPromptSet> => {
    const raw = await req<any>(`/projects/${id}/geo/prompt-sets/${promptSetID}`, { method: "PUT", body: JSON.stringify(body) }, auth);
    return normalizeGEOPromptSet(raw);
  },
  updateGEOPrompt: async (id: string, promptID: string, body: GEOPromptUpdateInput): Promise<GEOPrompt> => {
    const raw = await req<any>(`/projects/${id}/geo/prompts/${promptID}`, { method: "PUT", body: JSON.stringify(body) }, auth);
    return normalizeGEOPrompt(raw);
  },
  updateGEOCompetitor: async (id: string, competitorID: string, body: GEOCompetitorUpdateInput): Promise<GEOCompetitor> => {
    const raw = await req<any>(`/projects/${id}/geo/competitors/${competitorID}`, { method: "PUT", body: JSON.stringify(body) }, auth);
    return normalizeGEOCompetitor(raw);
  },
  observeGEOManualFixtures: async (id: string, body: ManualFixtureRequest) => {
    return req<any>(`/projects/${id}/geo/runs/observe`, { method: "POST", body: JSON.stringify(body) }, auth);
  },
  observeGEOProvider: async (id: string, body: GEOProviderObserveRequest = {}): Promise<GEOProviderObserveResult> => {
    const raw = await req<any>(`/projects/${id}/geo/runs/observe-provider`, { method: "POST", body: JSON.stringify(body) }, auth);
    return {
      run: raw?.run,
      observations: arrayFrom(raw?.observations).map(normalizeGEOObservation),
      score: normalizeGEOVisibilityScore(raw?.score),
      cost_usd: Number(raw?.cost_usd ?? 0),
      data_source_notes: arrayFrom<string>(raw?.data_source_notes).map(String),
    };
  },
  listGEOObservations: async (id: string, options: { prompt_id?: string; engine?: string; source_type?: string; limit?: number } = {}): Promise<GEOObservation[]> => {
    const params = new URLSearchParams();
    if (options.prompt_id) params.set("prompt_id", options.prompt_id);
    if (options.engine) params.set("engine", options.engine);
    if (options.source_type) params.set("source_type", options.source_type);
    if (options.limit) params.set("limit", String(options.limit));
    const suffix = params.toString() ? `?${params}` : "";
    const raw = await req<any[]>(`/projects/${id}/geo/observations${suffix}`, undefined, auth);
    return arrayFrom(raw).map(normalizeGEOObservation);
  },
  listGEOExternalSurfaces: async (id: string, options: { owner_type?: string } = {}): Promise<GEOExternalSurface[]> => {
    const params = new URLSearchParams();
    if (options.owner_type) params.set("owner_type", options.owner_type);
    const suffix = params.toString() ? `?${params}` : "";
    const raw = await req<any[]>(`/projects/${id}/geo/external-surfaces${suffix}`, undefined, auth);
    return arrayFrom(raw).map(normalizeGEOExternalSurface);
  },
  createGEOExternalSurface: async (
    id: string,
    body: {
      url: string;
      normalized_url?: string;
      platform?: string;
      surface_type?: string;
      owner_type?: string;
      canonical_target_url?: string;
      backlink_state?: string;
      source_url?: string;
      canonical_status?: string;
      indexability_status?: string;
      publication_status?: string;
      owner_confidence?: string;
      verification_snapshot?: any;
      related_action_ids?: string[];
    },
  ): Promise<GEOExternalSurface> => {
    const raw = await req<any>(`/projects/${id}/geo/external-surfaces`, { method: "POST", body: JSON.stringify(body) }, auth);
    return normalizeGEOExternalSurface(raw);
  },
  monitorGEOExternalSurfaces: async (id: string, body: GEOExternalSurfaceMonitorRequest = {}): Promise<GEOExternalSurfaceMonitorResult> => {
    const raw = await req<any>(`/projects/${id}/geo/external-surfaces/monitor`, { method: "POST", body: JSON.stringify(body) }, auth);
    return {
      run: raw?.run,
      surfaces: arrayFrom(raw?.surfaces).map(normalizeGEOExternalSurface),
      checked: Number(raw?.checked ?? 0),
      failed: Number(raw?.failed ?? 0),
      data_source_notes: arrayFrom<string>(raw?.data_source_notes).map(String),
    };
  },
  analyzeGEOOpportunities: async (id: string, body: { limit?: number } = {}) => {
    return req<any>(`/projects/${id}/geo/opportunities/analyze`, { method: "POST", body: JSON.stringify(body) }, auth);
  },
  listGEOAssetBriefs: async (id: string, options: { status?: string; limit?: number } = {}): Promise<GEOAssetBrief[]> => {
    const params = new URLSearchParams();
    if (options.status) params.set("status", options.status);
    if (options.limit) params.set("limit", String(options.limit));
    const suffix = params.toString() ? `?${params}` : "";
    const raw = await req<any[]>(`/projects/${id}/geo/asset-briefs${suffix}`, undefined, auth);
    return arrayFrom(raw).map(normalizeGEOAssetBrief);
  },
  acceptGEOAssetBrief: async (id: string, briefID: string) => {
    return req<any>(`/projects/${id}/geo/asset-briefs/${briefID}/accept`, { method: "POST" }, auth);
  },
  listSEOOpportunities: async (id: string, options: SEOListOptions = {}): Promise<SEOOpportunity[]> => {
    const params = new URLSearchParams();
    if (options.type) params.set("type", options.type);
    if (options.status) params.set("status", options.status);
    if (options.limit) params.set("limit", String(options.limit));
    if (options.cursor) params.set("cursor", options.cursor);
    const suffix = params.toString() ? `?${params}` : "";
    const raw = await req<any[]>(`/projects/${id}/seo/opportunities${suffix}`, undefined, auth);
    return arrayFrom(raw);
  },
  acceptSEOOpportunity: async (id: string, opportunityID: string): Promise<SEOContentAction> => {
    return req<SEOContentAction>(`/projects/${id}/seo/opportunities/${opportunityID}/accept`, { method: "POST" }, auth);
  },
  dismissSEOOpportunity: async (id: string, opportunityID: string): Promise<SEOOpportunity> => {
    return req<SEOOpportunity>(`/projects/${id}/seo/opportunities/${opportunityID}/dismiss`, { method: "POST" }, auth);
  },
  createSEOContentAction: async (
    id: string,
    opportunityID: string,
    body: { action_type?: string; asset_type?: string; work_type?: string; review_required?: boolean } = {},
  ): Promise<SEOContentAction> => {
    return req<SEOContentAction>(
      `/projects/${id}/seo/opportunities/${opportunityID}/actions`,
      { method: "POST", body: JSON.stringify(body) },
      auth,
    );
  },
  snoozeSEOOpportunity: async (
    id: string,
    opportunityID: string,
    body: { days?: number; reason?: string } = {},
  ): Promise<SEOOpportunity> => {
    return req<SEOOpportunity>(
      `/projects/${id}/seo/opportunities/${opportunityID}/snooze`,
      { method: "POST", body: JSON.stringify(body) },
      auth,
    );
  },
  unsnoozeSEOOpportunity: async (id: string, opportunityID: string): Promise<SEOOpportunity> => {
    return req<SEOOpportunity>(`/projects/${id}/seo/opportunities/${opportunityID}/unsnooze`, { method: "POST" }, auth);
  },
  watchSEOOpportunity: async (
    id: string,
    opportunityID: string,
    body: { observation_window_days?: number } = {},
  ): Promise<SEOWatchlistItem> => {
    return req<SEOWatchlistItem>(
      `/projects/${id}/seo/opportunities/${opportunityID}/watch`,
      { method: "POST", body: JSON.stringify(body) },
      auth,
    );
  },
  listSEOWatchlist: async (id: string, options: { status?: string; limit?: number } = {}): Promise<SEOWatchlistItem[]> => {
    const params = new URLSearchParams();
    if (options.status) params.set("status", options.status);
    if (options.limit) params.set("limit", String(options.limit));
    const suffix = params.toString() ? `?${params}` : "";
    const raw = await req<any[]>(`/projects/${id}/seo/watchlist${suffix}`, undefined, auth);
    return arrayFrom(raw);
  },
  closeSEOWatchlistItem: async (
    id: string,
    watchlistItemID: string,
    body: { status?: "closed" | "learned" } = {},
  ): Promise<SEOWatchlistItem> => {
    return req<SEOWatchlistItem>(
      `/projects/${id}/seo/watchlist/${watchlistItemID}/close`,
      { method: "POST", body: JSON.stringify(body) },
      auth,
    );
  },
  listSEOContentActions: async (id: string, options: SEOListOptions = {}): Promise<SEOContentAction[]> => {
    const params = new URLSearchParams();
    if (options.status) params.set("status", options.status);
    if (options.limit) params.set("limit", String(options.limit));
    if (options.cursor) params.set("cursor", options.cursor);
    const suffix = params.toString() ? `?${params}` : "";
    const raw = await req<any[]>(`/projects/${id}/seo/actions${suffix}`, undefined, auth);
    return arrayFrom(raw);
  },
  planSEOContentAction: async (id: string, actionID: string, body: { publish_strategy?: string; publish_to?: string } = {}): Promise<Topic> => {
    const raw = await req<any>(
      `/projects/${id}/seo/actions/${actionID}/plan`,
      { method: "POST", body: JSON.stringify(body) },
      auth,
    );
    return normalizeTopic(raw);
  },
  verifySEOContentAction: async (
    id: string,
    actionID: string,
    body: { status: "verified" | "failed" | "recovery_required" | string; verification_snapshot?: any },
  ): Promise<SEOContentAction> => {
    return req<SEOContentAction>(
      `/projects/${id}/seo/actions/${actionID}/verify`,
      { method: "POST", body: JSON.stringify(body) },
      auth,
    );
  },
  dismissSEOContentAction: async (id: string, actionID: string): Promise<SEOContentAction> => {
    return req<SEOContentAction>(
      `/projects/${id}/seo/actions/${actionID}/dismiss`,
      { method: "POST" },
      auth,
    );
  },
  listSEOObjectives: async (id: string): Promise<SEOObjective[]> => {
    const raw = await req<any[]>(`/projects/${id}/seo/autopilot/objectives`, undefined, auth);
    return arrayFrom(raw);
  },
  createSEOObjective: async (
    id: string,
    body: { name: string; primary_metric?: string; time_horizon_days?: number; budget_usd?: number },
  ): Promise<SEOObjective> => {
    return req<SEOObjective>(`/projects/${id}/seo/autopilot/objectives`, { method: "POST", body: JSON.stringify(body) }, auth);
  },
  getSEOPolicy: async (id: string): Promise<SEOPolicy> => {
    return req<SEOPolicy>(`/projects/${id}/seo/autopilot/policy`, undefined, auth);
  },
  updateSEOPolicy: async (id: string, body: SEOPolicyUpdateInput): Promise<SEOPolicy> => {
    return req<SEOPolicy>(`/projects/${id}/seo/autopilot/policy`, { method: "PUT", body: JSON.stringify(body) }, auth);
  },
  getAutopilotReadiness: async (id: string): Promise<AutopilotReadiness> => {
    return req<AutopilotReadiness>(`/projects/${id}/seo/autopilot/readiness`, undefined, auth);
  },
  generateAutopilotPlan: async (id: string): Promise<{ plan: SEOActionPlan; run: any }> => {
    const raw = await req<any>(`/projects/${id}/seo/autopilot/plans/generate`, { method: "POST" }, auth);
    return { plan: normalizeSEOActionPlan(raw?.plan), run: raw?.run };
  },
  listAutopilotPlans: async (id: string): Promise<SEOActionPlan[]> => {
    const raw = await req<any[]>(`/projects/${id}/seo/autopilot/plans`, undefined, auth);
    return arrayFrom(raw).map(normalizeSEOActionPlan);
  },
  executeAutopilotPlan: async (id: string, planID: string): Promise<AutopilotExecuteResult> => {
    const raw = await req<any>(`/projects/${id}/seo/autopilot/plans/${planID}/execute`, { method: "POST" }, auth);
    return { ...raw, plan: normalizeSEOActionPlan(raw?.plan) };
  },
  listSafeModeEvents: async (id: string): Promise<SafeModeEvent[]> => {
    const raw = await req<any[]>(`/projects/${id}/seo/autopilot/safe-mode`, undefined, auth);
    return arrayFrom(raw);
  },
  enterSafeMode: async (id: string, body: { reason: string; trigger_source?: string; entered_by?: string }): Promise<SafeModeEvent> => {
    return req<SafeModeEvent>(`/projects/${id}/seo/autopilot/safe-mode`, { method: "POST", body: JSON.stringify(body) }, auth);
  },
  exitSafeMode: async (
    id: string,
    safeModeID: string,
    body: { exited_by?: string; exit_reason?: string } = {},
  ): Promise<SafeModeEvent> => {
    return req<SafeModeEvent>(
      `/projects/${id}/seo/autopilot/safe-mode/${safeModeID}/exit`,
      { method: "POST", body: JSON.stringify(body) },
      auth,
    );
  },
  listNotificationChannels: async (id: string): Promise<NotificationChannel[]> => {
    const raw = await req<any[]>(`/projects/${id}/notifications/channels`, undefined, auth);
    return arrayFrom(raw);
  },
  createNotificationChannel: async (
    id: string,
    body: { kind: NotificationChannelKind; url: string; label: string },
  ): Promise<NotificationChannel> => {
    return req<NotificationChannel>(`/projects/${id}/notifications/channels`, { method: "POST", body: JSON.stringify(body) }, auth);
  },
  deleteNotificationChannel: async (id: string, channelID: string): Promise<NotificationChannel> => {
    return req<NotificationChannel>(`/projects/${id}/notifications/channels/${channelID}`, { method: "DELETE" }, auth);
  },
  testNotificationChannel: async (id: string, channelID: string): Promise<NotificationChannel> => {
    return req<NotificationChannel>(`/projects/${id}/notifications/channels/${channelID}/test`, { method: "POST" }, auth);
  },
  listNotificationEvents: async (id: string): Promise<NotificationEvent[]> => {
    const raw = await req<any[]>(`/projects/${id}/notifications/events`, undefined, auth);
    return arrayFrom(raw);
  },
  listNotificationSubscriptions: async (id: string): Promise<NotificationSubscription[]> => {
    const raw = await req<any[]>(`/projects/${id}/notifications/subscriptions`, undefined, auth);
    return arrayFrom(raw);
  },
  upsertNotificationSubscription: async (
    id: string,
    body: { event_type: string; channel_id: string; enabled: boolean; filter?: any },
  ): Promise<NotificationSubscription> => {
    return req<NotificationSubscription>(
      `/projects/${id}/notifications/subscriptions`,
      { method: "PUT", body: JSON.stringify(body) },
      auth,
    );
  },
  listNotificationDeliveries: async (
    id: string,
    options: NotificationDeliveryListOptions = {},
  ): Promise<NotificationDelivery[]> => {
    const params = new URLSearchParams();
    if (options.status) params.set("status", options.status);
    if (options.limit) params.set("limit", String(options.limit));
    const suffix = params.toString() ? `?${params}` : "";
    const raw = await req<any[]>(`/projects/${id}/notifications/deliveries${suffix}`, undefined, auth);
    return arrayFrom(raw);
  },
  retryNotificationDelivery: async (id: string, deliveryID: string): Promise<NotificationDelivery> => {
    return req<NotificationDelivery>(`/projects/${id}/notifications/deliveries/${deliveryID}/retry`, { method: "POST" }, auth);
  },
  tickGenerate: (id: string) => req(`/projects/${id}/tick/generate`, { method: "POST" }, auth),
  tickPublish: (id: string) => req(`/projects/${id}/tick/publish`, { method: "POST" }, auth),
  getArticle: async (id: string, articleID: string) => {
    const raw = await req<any>(`/projects/${id}/articles/${articleID}`, undefined, auth);
    return normalizeArticle(raw);
  },
  approve: async (id: string, articleID: string) => {
    const raw = await req<any>(`/projects/${id}/articles/${articleID}/approve`, {
      method: "POST",
      body: JSON.stringify({ reviewed_by: "reviewer" }),
    }, auth);
    return normalizeArticle(raw);
  },
  reject: async (id: string, articleID: string) => {
    const raw = await req<any>(`/projects/${id}/articles/${articleID}/reject`, {
      method: "POST",
      body: JSON.stringify({ reviewed_by: "reviewer" }),
    }, auth);
    return normalizeArticle(raw);
  },
  edit: async (id: string, articleID: string, body: { content_md?: string; seo_meta?: any }) => {
    const raw = await req<any>(`/projects/${id}/articles/${articleID}`, { method: "PUT", body: JSON.stringify(body) }, auth);
    return normalizeArticle(raw);
  },
  fixArticle: async (id: string, articleID: string) => {
    const raw = await req<any>(`/projects/${id}/articles/${articleID}/ai-fix`, { method: "POST" }, auth);
    return normalizeArticle(raw);
  },
  applyFix: async (id: string, articleID: string, instruction: string) => {
    const raw = await req<any>(`/projects/${id}/articles/${articleID}/apply-fix`, { method: "POST", body: JSON.stringify({ instruction }) }, auth);
    return normalizeArticle(raw);
  },
  recheckArticle: async (id: string, articleID: string) => {
    const raw = await req<any>(`/projects/${id}/articles/${articleID}/recheck`, { method: "POST" }, auth);
    return normalizeArticle(raw);
  },
  distributed: async (id: string, articleID: string) => {
    const raw = await req<any>(`/projects/${id}/articles/${articleID}/distributed`, { method: "POST" }, auth);
    return normalizeArticle(raw);
  },
  retryPublish: async (id: string, articleID: string) => {
    const raw = await req<any>(`/projects/${id}/articles/${articleID}/retry-publish`, { method: "POST" }, auth);
    return normalizeArticle(raw);
  },
  reconcilePublishing: (id: string) => req(`/projects/${id}/publishing/reconcile`, { method: "POST" }, auth),
  publishNow: async (id: string, articleID: string) => {
    const raw = await req<any>(`/projects/${id}/articles/${articleID}/publish-now`, { method: "POST" }, auth);
    return normalizeArticle(raw);
  },
  };
}

export const api = createApi();
