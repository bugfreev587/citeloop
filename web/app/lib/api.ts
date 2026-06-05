// Thin typed client for the CiteLoop Go API.
const BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

export type Project = {
  id: string;
  name: string;
  slug: string;
  config: any;
};

export type Topic = {
  id: string;
  channel: string;
  title: string;
  priority: number;
  status: string;
  scheduled_at: { Time: string; Valid: boolean } | null;
};

export type Article = {
  id: string;
  topic_id: string;
  kind: string;
  platform: string | null;
  content_md: string;
  seo_meta: any;
  geo_score: any;
  seo_score: any;
  qa_issues: string[] | null;
  qa_blocking: boolean;
  canonical_url: string | null;
  status: string;
};

export type ReviewGroup = { topic_id: string; articles: Article[] };

export type DistributeItem = {
  article: Article;
  compose_url: string;
  supports_canonical: boolean;
};

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
  listProjects: () => req<Project[]>("/projects"),
  getProject: (id: string) => req<Project>(`/projects/${id}/`),
  runInsight: (id: string, landingURL: string) =>
    req(`/projects/${id}/insight`, { method: "POST", body: JSON.stringify({ landing_url: landingURL }) }),
  runStrategist: (id: string) => req<Topic[]>(`/projects/${id}/strategist`, { method: "POST" }),
  listTopics: (id: string) => req<Topic[]>(`/projects/${id}/topics`),
  generateTopic: (id: string, topicID: string) =>
    req<Article[]>(`/projects/${id}/topics/${topicID}/generate`, { method: "POST" }),
  listReview: (id: string) => req<ReviewGroup[]>(`/projects/${id}/review`),
  listArticles: (id: string, status: string) =>
    req<Article[]>(`/projects/${id}/articles?status=${status}`),
  listDistribute: (id: string) => req<DistributeItem[]>(`/projects/${id}/distribute`),
  tickGenerate: (id: string) => req(`/projects/${id}/tick/generate`, { method: "POST" }),
  tickPublish: (id: string) => req(`/projects/${id}/tick/publish`, { method: "POST" }),
  approve: (articleID: string) =>
    req<Article>(`/articles/${articleID}/approve`, { method: "POST", body: JSON.stringify({ reviewed_by: "reviewer" }) }),
  reject: (articleID: string) =>
    req<Article>(`/articles/${articleID}/reject`, { method: "POST", body: JSON.stringify({ reviewed_by: "reviewer" }) }),
  edit: (articleID: string, body: { content_md?: string; seo_meta?: any }) =>
    req<Article>(`/articles/${articleID}/`, { method: "PUT", body: JSON.stringify(body) }),
  distributed: (articleID: string) =>
    req<Article>(`/articles/${articleID}/distributed`, { method: "POST" }),
};
