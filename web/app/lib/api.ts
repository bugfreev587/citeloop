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
  cadence_per_week: number;
  buffer_days: number;
  channel_mix: { blog: number; syndication: number };
  brand_voice?: string;
  monthly_budget_usd: number;
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
  setup_checklist: SetupChecklistItem[];
  capability_mode: "public_only" | "managed_content_connected" | "customer_site_pending_verification" | "customer_site_connected" | string;
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

export function defaultProjectConfig(): ProjectConfig {
  return {
    cadence_per_week: 3,
    buffer_days: 5,
    channel_mix: { blog: 0.6, syndication: 0.4 },
    brand_voice: "",
    monthly_budget_usd: 50,
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
    last_28_days: data.last_28_days ?? {},
    technical: data.technical ?? {},
    opportunities_by_type: arrayFrom(data.opportunities_by_type),
    actions_by_status: arrayFrom(data.actions_by_status),
    cold_start: Boolean(data.cold_start),
    handoff_ready_for_autopilot: Boolean(data.handoff_ready_for_autopilot),
    setup_checklist: arrayFrom<SetupChecklistItem>(data.setup_checklist),
    capability_mode: data.capability_mode ?? "public_only",
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
  listProjects: async () => {
    const raw = await req<any[]>("/projects", undefined, auth);
    return arrayFrom(raw).map(normalizeProject);
  },
  createProject: async (body: { name?: string; slug?: string; owner_id?: string; site_url?: string }) => {
    const raw = await req<any>("/projects", { method: "POST", body: JSON.stringify(body) }, auth);
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
    const raw = await req<any[]>(`/projects/${id}/topics/${topicID}/generate`, { method: "POST" }, auth);
    return arrayFrom(raw).map(normalizeArticle);
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
  testPublisherConnection: async (id: string, connectionID: string): Promise<PublisherConnection> => {
    const raw = await req<any>(`/projects/${id}/publisher-connections/${connectionID}/test`, { method: "POST" }, auth);
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
  revokePublisherCredential: async (id: string, connectionID: string): Promise<PublisherConnection> => {
    const raw = await req<any>(
      `/projects/${id}/publisher-connections/${connectionID}/credential`,
      { method: "DELETE" },
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
  distributed: async (id: string, articleID: string) => {
    const raw = await req<any>(`/projects/${id}/articles/${articleID}/distributed`, { method: "POST" }, auth);
    return normalizeArticle(raw);
  },
  retryPublish: async (id: string, articleID: string) => {
    const raw = await req<any>(`/projects/${id}/articles/${articleID}/retry-publish`, { method: "POST" }, auth);
    return normalizeArticle(raw);
  },
  reconcilePublishing: (id: string) => req(`/projects/${id}/publishing/reconcile`, { method: "POST" }, auth),
  };
}

export const api = createApi();
