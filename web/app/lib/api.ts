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

export type InsightResult = {
  profile: any;
  inventory_count: number;
  crawl_summary?: any;
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

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}/api${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers || {}) },
    cache: "no-store",
  });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${res.status}: ${body}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export const api = {
  listProjects: async () => {
    const raw = await req<any[]>("/projects");
    return raw.map(normalizeProject);
  },
  createProject: async (body: { name: string; slug: string; owner_id?: string }) => {
    const raw = await req<any>("/projects", { method: "POST", body: JSON.stringify(body) });
    return normalizeProject(raw);
  },
  getProject: async (id: string) => {
    const raw = await req<any>(`/projects/${id}/`);
    return normalizeProject(raw);
  },
  updateConfig: async (id: string, config: ProjectConfig) => {
    const raw = await req<any>(`/projects/${id}/config`, { method: "PUT", body: JSON.stringify(config) });
    return normalizeProject(raw);
  },
  runInsight: (id: string, landingURL: string) =>
    req<InsightResult>(`/projects/${id}/insight`, {
      method: "POST",
      body: JSON.stringify({ landing_url: landingURL }),
    }),
  getProfile: async (id: string) => {
    const raw = await req<any>(`/projects/${id}/profile`);
    return normalizeProfile(raw);
  },
  updateProfile: async (id: string, body: { profile: any; source_urls?: any[] }) => {
    const raw = await req<any>(`/projects/${id}/profile`, { method: "PUT", body: JSON.stringify(body) });
    return normalizeProfile(raw);
  },
  listInventory: async (id: string) => {
    const raw = await req<any[]>(`/projects/${id}/inventory`);
    return raw.map(normalizeInventoryItem);
  },
  updateInventory: async (
    id: string,
    itemID: string,
    body: { title?: string; target_keyword?: string; topics?: any[]; summary?: string },
  ) => {
    const raw = await req<any>(`/projects/${id}/inventory/${itemID}`, { method: "PUT", body: JSON.stringify(body) });
    return normalizeInventoryItem(raw);
  },
  deleteInventory: (id: string, itemID: string) =>
    req<void>(`/projects/${id}/inventory/${itemID}`, { method: "DELETE" }),
  runStrategist: async (id: string) => {
    const raw = await req<any[]>(`/projects/${id}/strategist`, { method: "POST" });
    return raw.map(normalizeTopic);
  },
  listTopics: async (id: string) => {
    const raw = await req<any[]>(`/projects/${id}/topics`);
    return raw.map(normalizeTopic);
  },
  generateTopic: async (id: string, topicID: string) => {
    const raw = await req<any[]>(`/projects/${id}/topics/${topicID}/generate`, { method: "POST" });
    return raw.map(normalizeArticle);
  },
  listReview: async (id: string) => {
    const raw = await req<any[]>(`/projects/${id}/review`);
    return raw.map(normalizeReviewGroup);
  },
  listArticles: async (id: string, status: string) => {
    const raw = await req<any[]>(`/projects/${id}/articles?status=${status}`);
    return raw.map(normalizeArticle);
  },
  listDistribute: async (id: string) => {
    const raw = await req<any[]>(`/projects/${id}/distribute`);
    return raw.map(normalizeDistributeItem);
  },
  listRuns: async (_id: string): Promise<GenerationRun[]> => {
    throw new Error("Runs endpoint is not available yet.");
  },
  tickGenerate: (id: string) => req(`/projects/${id}/tick/generate`, { method: "POST" }),
  tickPublish: (id: string) => req(`/projects/${id}/tick/publish`, { method: "POST" }),
  approve: async (articleID: string) => {
    const raw = await req<any>(`/articles/${articleID}/approve`, {
      method: "POST",
      body: JSON.stringify({ reviewed_by: "reviewer" }),
    });
    return normalizeArticle(raw);
  },
  reject: async (articleID: string) => {
    const raw = await req<any>(`/articles/${articleID}/reject`, {
      method: "POST",
      body: JSON.stringify({ reviewed_by: "reviewer" }),
    });
    return normalizeArticle(raw);
  },
  edit: async (articleID: string, body: { content_md?: string; seo_meta?: any }) => {
    const raw = await req<any>(`/articles/${articleID}/`, { method: "PUT", body: JSON.stringify(body) });
    return normalizeArticle(raw);
  },
  distributed: async (articleID: string) => {
    const raw = await req<any>(`/articles/${articleID}/distributed`, { method: "POST" });
    return normalizeArticle(raw);
  },
};
