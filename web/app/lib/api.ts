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

export type { Article, GenerationRun, InventoryItem, ProductProfile, Topic };

export type AuthOptions = {
  token?: string | null;
  getToken?: () => Promise<string | null>;
};

export type ProjectConfig = {
  site_url?: string;
  cadence_per_week: number;
  buffer_days: number;
  channel_mix: { blog: number; syndication: number };
  brand_voice?: string;
  monthly_budget_usd: number;
  publish_mode?: "scheduled" | "auto" | "manual";
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

export type Project = {
  id: string;
  owner_id?: string;
  name: string;
  slug: string;
  config: ProjectConfig;
  created_at?: any;
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

export type LLMProvider = "tokengate" | "openai" | "claude";

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
  updated_at?: string;
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
  kind: "github_nextjs" | "webhook" | "wordpress" | string;
  label: string;
  status: "missing" | "connected" | "error" | "revoked" | string;
  is_default: boolean;
  capabilities: Record<string, boolean>;
  capability_schema_version: number;
  credential_configured: boolean;
  config: {
    repo?: string;
    branch?: string;
    content_dir?: string;
    base_url?: string;
    publish_mode?: string;
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

export type PublisherCredentialInput = {
  kind: "github_token";
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
  status: "missing" | "connected" | "expired" | "error";
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
  created_at?: any;
};

export type SEOContentAction = {
  id: string;
  opportunity_id: string;
  action_type: string;
  status: string;
  target_url?: string | null;
  normalized_target_url?: string | null;
  target_content_hash_before?: string | null;
  created_at?: any;
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

export type SEOListOptions = {
  type?: string;
  status?: string;
  limit?: number;
  cursor?: string;
};

export type SEOPolicy = {
  id: string;
  autopilot_level: number;
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
};

export type SEOObjective = {
  id: string;
  name: string;
  status: string;
  primary_metric: string;
  time_horizon_days: number;
  budget_usd?: RawPgNumeric;
};

export type SEOActionPlan = {
  id: string;
  status: string;
  actions: any[];
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
    publish_mode: "scheduled",
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

function normalizeProject(raw: any): Project {
  return {
    id: raw.id,
    owner_id: raw.owner_id,
    name: raw.name ?? "Untitled project",
    slug: raw.slug ?? raw.id,
    config: { ...defaultProjectConfig(), ...(raw.config ?? {}) },
    created_at: raw.created_at,
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
  const provider: LLMProvider =
    raw.provider === "claude" ? "claude" : raw.provider === "openai" ? "openai" : "tokengate";
  return {
    provider,
    configured: Boolean(raw.configured),
    key_tail: raw.key_tail ?? undefined,
    base_url: raw.base_url ?? undefined,
    updated_at: raw.updated_at ?? undefined,
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
  const res = await fetch(`${BASE}/api${path}`, {
    ...init,
    headers,
    cache: "no-store",
  });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${res.status}: ${body}`);
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
  updateLLMCredentials: async (body: { provider: LLMProvider; api_key?: string; base_url?: string }) => {
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
  getMe: async () => {
    return req<{ user_id: string; email: string; is_admin: boolean }>("/me", undefined, auth);
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
  getSEOOverview: async (id: string): Promise<SEOOverview> => {
    const raw = await req<any>(`/projects/${id}/seo/overview`, undefined, auth);
    return normalizeSEOOverview(raw);
  },
  syncSEO: async (id: string, siteURL?: string) => {
    return req<any>(`/projects/${id}/seo/sync`, { method: "POST", body: JSON.stringify({ site_url: siteURL ?? "" }) }, auth);
  },
  analyzeSEO: async (id: string) => {
    return req<any>(`/projects/${id}/seo/analyze`, { method: "POST" }, auth);
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
    body: { url: string; normalized_url?: string; platform?: string; surface_type?: string; owner_type?: string; canonical_target_url?: string; backlink_state?: string },
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
  acceptSEOOpportunity: async (id: string, opportunityID: string): Promise<SEOOpportunity> => {
    return req<SEOOpportunity>(`/projects/${id}/seo/opportunities/${opportunityID}/accept`, { method: "POST" }, auth);
  },
  dismissSEOOpportunity: async (id: string, opportunityID: string): Promise<SEOOpportunity> => {
    return req<SEOOpportunity>(`/projects/${id}/seo/opportunities/${opportunityID}/dismiss`, { method: "POST" }, auth);
  },
  createSEOContentAction: async (
    id: string,
    opportunityID: string,
    body: { action_type?: string } = {},
  ): Promise<SEOContentAction> => {
    return req<SEOContentAction>(
      `/projects/${id}/seo/opportunities/${opportunityID}/actions`,
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
  updateSEOPolicy: async (id: string, body: Partial<SEOPolicy>): Promise<SEOPolicy> => {
    return req<SEOPolicy>(`/projects/${id}/seo/autopilot/policy`, { method: "PUT", body: JSON.stringify(body) }, auth);
  },
  generateAutopilotPlan: async (id: string): Promise<{ plan: SEOActionPlan; run: any }> => {
    return req<{ plan: SEOActionPlan; run: any }>(`/projects/${id}/seo/autopilot/plans/generate`, { method: "POST" }, auth);
  },
  listAutopilotPlans: async (id: string): Promise<SEOActionPlan[]> => {
    const raw = await req<any[]>(`/projects/${id}/seo/autopilot/plans`, undefined, auth);
    return arrayFrom(raw);
  },
  listSafeModeEvents: async (id: string): Promise<SafeModeEvent[]> => {
    const raw = await req<any[]>(`/projects/${id}/seo/autopilot/safe-mode`, undefined, auth);
    return arrayFrom(raw);
  },
  enterSafeMode: async (id: string, body: { reason: string; trigger_source?: string; entered_by?: string }): Promise<SafeModeEvent> => {
    return req<SafeModeEvent>(`/projects/${id}/seo/autopilot/safe-mode`, { method: "POST", body: JSON.stringify(body) }, auth);
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
