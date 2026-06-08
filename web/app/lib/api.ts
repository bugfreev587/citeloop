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
    return raw.map(normalizeProject);
  },
  createProject: async (body: { name: string; slug: string; owner_id?: string }) => {
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
    return raw.map(normalizeInventoryItem);
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
    return raw.map(normalizeTopic);
  },
  listTopics: async (id: string) => {
    const raw = await req<any[]>(`/projects/${id}/topics`, undefined, auth);
    return raw.map(normalizeTopic);
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
    return raw.map(normalizeArticle);
  },
  listReview: async (id: string) => {
    const raw = await req<any[]>(`/projects/${id}/review`, undefined, auth);
    return raw.map(normalizeReviewGroup);
  },
  listArticles: async (id: string, status: string) => {
    const raw = await req<any[]>(`/projects/${id}/articles?status=${status}`, undefined, auth);
    return raw.map(normalizeArticle);
  },
  listDistribute: async (id: string) => {
    const raw = await req<any[]>(`/projects/${id}/distribute`, undefined, auth);
    return raw.map(normalizeDistributeItem);
  },
  listRuns: async (id: string, options: RunListOptions = {}): Promise<GenerationRun[]> => {
    const params = new URLSearchParams();
    if (options.agent) params.set("agent", options.agent);
    if (options.status) params.set("status", options.status);
    if (options.limit) params.set("limit", String(options.limit));
    if (options.cursor) params.set("cursor", options.cursor);
    const suffix = params.toString() ? `?${params}` : "";
    const raw = await req<any[]>(`/projects/${id}/runs${suffix}`, undefined, auth);
    return raw.map(normalizeRun);
  },
  getSEOOverview: async (id: string): Promise<SEOOverview> => {
    return req<SEOOverview>(`/projects/${id}/seo/overview`, undefined, auth);
  },
  syncSEO: async (id: string, siteURL?: string) => {
    return req<any>(`/projects/${id}/seo/sync`, { method: "POST", body: JSON.stringify({ site_url: siteURL ?? "" }) }, auth);
  },
  analyzeSEO: async (id: string) => {
    return req<any>(`/projects/${id}/seo/analyze`, { method: "POST" }, auth);
  },
  getSEOSettings: async (id: string): Promise<{ property?: SEOProperty | null; integrations: SEOIntegration[] }> => {
    return req<{ property?: SEOProperty | null; integrations: SEOIntegration[] }>(`/projects/${id}/seo/settings`, undefined, auth);
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
    return req<SEOBrief>(`/projects/${id}/seo/briefs/latest`, undefined, auth);
  },
  listSEOOpportunities: async (id: string, options: SEOListOptions = {}): Promise<SEOOpportunity[]> => {
    const params = new URLSearchParams();
    if (options.type) params.set("type", options.type);
    if (options.status) params.set("status", options.status);
    if (options.limit) params.set("limit", String(options.limit));
    if (options.cursor) params.set("cursor", options.cursor);
    const suffix = params.toString() ? `?${params}` : "";
    return req<SEOOpportunity[]>(`/projects/${id}/seo/opportunities${suffix}`, undefined, auth);
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
    return req<SEOContentAction[]>(`/projects/${id}/seo/actions${suffix}`, undefined, auth);
  },
  listSEOObjectives: async (id: string): Promise<SEOObjective[]> => {
    return req<SEOObjective[]>(`/projects/${id}/seo/autopilot/objectives`, undefined, auth);
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
    return req<SEOActionPlan[]>(`/projects/${id}/seo/autopilot/plans`, undefined, auth);
  },
  listSafeModeEvents: async (id: string): Promise<SafeModeEvent[]> => {
    return req<SafeModeEvent[]>(`/projects/${id}/seo/autopilot/safe-mode`, undefined, auth);
  },
  enterSafeMode: async (id: string, body: { reason: string; trigger_source?: string; entered_by?: string }): Promise<SafeModeEvent> => {
    return req<SafeModeEvent>(`/projects/${id}/seo/autopilot/safe-mode`, { method: "POST", body: JSON.stringify(body) }, auth);
  },
  listNotificationChannels: async (id: string): Promise<NotificationChannel[]> => {
    return req<NotificationChannel[]>(`/projects/${id}/notifications/channels`, undefined, auth);
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
    return req<NotificationEvent[]>(`/projects/${id}/notifications/events`, undefined, auth);
  },
  listNotificationSubscriptions: async (id: string): Promise<NotificationSubscription[]> => {
    return req<NotificationSubscription[]>(`/projects/${id}/notifications/subscriptions`, undefined, auth);
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
    return req<NotificationDelivery[]>(`/projects/${id}/notifications/deliveries${suffix}`, undefined, auth);
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
