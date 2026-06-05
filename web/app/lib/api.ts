import {
  Article,
  GenerationRun,
  InventoryItem,
  ProductProfile,
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
