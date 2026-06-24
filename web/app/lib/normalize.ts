export type RawPgNumeric =
  | number
  | string
  | null
  | undefined
  | {
      Int?: number | string | null;
      Exp?: number | null;
      Valid?: boolean;
    };

export type RawPgTime =
  | string
  | null
  | undefined
  | {
      Time?: string | null;
      Valid?: boolean;
    };

export type Article = {
  id: string;
  project_id?: string;
  topic_id: string;
  kind: "canonical" | "syndication_variant" | string;
  platform: string | null;
  content_md: string;
  seo_meta: Record<string, any>;
  geo_score: number | null;
  seo_score: number | null;
  qa_issues: string[];
  qa_blocking: boolean;
  canonical_url: string | null;
  status: string;
  scheduled_at: string | null;
  reviewed_at: string | null;
  published_at: string | null;
  last_publish_error: string | null;
  publish_attempts: number;
  next_publish_retry_at: string | null;
  resolved_slug: string | null;
  publish_path: string | null;
  canonical_url_verified_at: string | null;
  content_hash: string | null;
  repair_attempts: number;
  repair_status: string;
  repair_failure_reason: string | null;
  requires_human_decision: boolean;
  human_decision_options: Array<{ label?: string; description?: string }>;
  qa_feedback: Record<string, any>;
  created_at: string | null;
};

export type Topic = {
  id: string;
  project_id?: string;
  channel: string;
  title: string;
  target_keyword: string | null;
  target_prompt: string | null;
  angle: string | null;
  format: string | null;
  priority: number;
  internal_links: any[];
  status: string;
  scheduled_at: string | null;
  created_at: string | null;
};

export type ProductProfile = {
  id: string;
  project_id: string;
  source_urls: any[];
  profile: Record<string, any>;
  version: number;
  is_active: boolean;
  created_at: string | null;
  updated_at: string | null;
};

export type InventoryItem = {
  id: string;
  project_id: string;
  url: string;
  title: string | null;
  target_keyword: string | null;
  topics: any[];
  summary: string | null;
  evidence_snippets: any[];
  source: string;
  captured_at: string | null;
};

export type GenerationRun = {
  id: string;
  project_id: string;
  agent: string;
  input: Record<string, any> | null;
  output: Record<string, any> | null;
  model: string | null;
  tokens: number | null;
  cost_usd: number | null;
  status: string;
  error: string | null;
  created_at: string | null;
};

function parseJSONValue(value: any, fallback: any) {
  if (value == null) return fallback;
  if (typeof value !== "string") return value;
  try {
    return JSON.parse(value);
  } catch {
    const decoded = parseBase64JSONValue(value);
    return decoded ?? fallback;
  }
}

function parseBase64JSONValue(value: string) {
  const trimmed = value.trim();
  if (!trimmed || trimmed.length % 4 !== 0 || !/^[A-Za-z0-9+/]+={0,2}$/.test(trimmed)) return null;
  try {
    const decoded = atob(trimmed);
    const json = decoded.trim();
    if (!json.startsWith("{") && !json.startsWith("[")) return null;
    return JSON.parse(json);
  } catch {
    return null;
  }
}

function normalizeArray(value: any): any[] {
  const parsed = parseJSONValue(value, []);
  return Array.isArray(parsed) ? parsed : [];
}

function normalizeObject(value: any): Record<string, any> {
  const parsed = parseJSONValue(value, {});
  return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};
}

function normalizeCanonicalURL(value: any): string | null {
  if (value == null) return null;
  if (typeof value !== "string") return value;
  try {
    const parsed = new URL(value);
    if (parsed.hostname !== "dev.unipost.dev") return value;
    parsed.protocol = "https:";
    parsed.hostname = "unipost.dev";
    parsed.port = "";
    return parsed.toString();
  } catch {
    return value;
  }
}

export function normalizeNumeric(value: RawPgNumeric): number | null {
  if (value == null) return null;
  if (typeof value === "number") return Number.isFinite(value) ? value : null;
  if (typeof value === "string") {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : null;
  }
  if (value.Valid === false) return null;
  if (value.Int == null) return null;

  const intValue = Number(value.Int);
  if (!Number.isFinite(intValue)) return null;
  const exp = value.Exp ?? 0;
  return Number((intValue * 10 ** exp).toPrecision(15));
}

export function normalizeTime(value: RawPgTime): string | null {
  if (value == null) return null;
  if (typeof value === "string") return value || null;
  if (value.Valid === false) return null;
  return value.Time || null;
}

export function normalizeArticle(raw: any): Article {
  return {
    id: raw.id,
    project_id: raw.project_id,
    topic_id: raw.topic_id,
    kind: raw.kind,
    platform: raw.platform ?? null,
    content_md: raw.content_md ?? "",
    seo_meta: normalizeObject(raw.seo_meta),
    geo_score: normalizeNumeric(raw.geo_score),
    seo_score: normalizeNumeric(raw.seo_score),
    qa_issues: normalizeArray(raw.qa_issues).map(String),
    qa_blocking: Boolean(raw.qa_blocking),
    canonical_url: normalizeCanonicalURL(raw.canonical_url),
    status: raw.status ?? "unknown",
    scheduled_at: normalizeTime(raw.scheduled_at),
    reviewed_at: normalizeTime(raw.reviewed_at),
    published_at: normalizeTime(raw.published_at),
    last_publish_error: raw.last_publish_error ?? null,
    publish_attempts: Number(raw.publish_attempts ?? 0),
    next_publish_retry_at: normalizeTime(raw.next_publish_retry_at),
    resolved_slug: raw.resolved_slug ?? null,
    publish_path: raw.publish_path ?? null,
    canonical_url_verified_at: normalizeTime(raw.canonical_url_verified_at),
    content_hash: raw.content_hash ?? null,
    repair_attempts: Number(raw.repair_attempts ?? 0),
    repair_status: raw.repair_status ?? "idle",
    repair_failure_reason: raw.repair_failure_reason ?? null,
    requires_human_decision: Boolean(raw.requires_human_decision),
    human_decision_options: normalizeArray(raw.human_decision_options),
    qa_feedback: normalizeObject(raw.qa_feedback),
    created_at: normalizeTime(raw.created_at),
  };
}

export function normalizeTopic(raw: any): Topic {
  return {
    id: raw.id,
    project_id: raw.project_id,
    channel: raw.channel ?? "blog",
    title: raw.title ?? "Untitled topic",
    target_keyword: raw.target_keyword ?? null,
    target_prompt: raw.target_prompt ?? null,
    angle: raw.angle ?? null,
    format: raw.format ?? null,
    priority: Number(raw.priority ?? 0),
    internal_links: normalizeArray(raw.internal_links),
    status: raw.status ?? "backlog",
    scheduled_at: normalizeTime(raw.scheduled_at),
    created_at: normalizeTime(raw.created_at),
  };
}

export function normalizeProfile(raw: any): ProductProfile {
  return {
    id: raw.id,
    project_id: raw.project_id,
    source_urls: normalizeArray(raw.source_urls),
    profile: normalizeObject(raw.profile),
    version: Number(raw.version ?? 1),
    is_active: Boolean(raw.is_active),
    created_at: normalizeTime(raw.created_at),
    updated_at: normalizeTime(raw.updated_at),
  };
}

export function normalizeInventoryItem(raw: any): InventoryItem {
  return {
    id: raw.id,
    project_id: raw.project_id,
    url: raw.url,
    title: raw.title ?? null,
    target_keyword: raw.target_keyword ?? null,
    topics: normalizeArray(raw.topics),
    summary: raw.summary ?? null,
    evidence_snippets: normalizeArray(raw.evidence_snippets),
    source: raw.source ?? "existing",
    captured_at: normalizeTime(raw.captured_at),
  };
}

export function normalizeRun(raw: any): GenerationRun {
  return {
    id: raw.id,
    project_id: raw.project_id,
    agent: raw.agent ?? "unknown",
    input: normalizeObject(raw.input),
    output: normalizeObject(raw.output),
    model: raw.model ?? null,
    tokens: raw.tokens == null ? null : Number(raw.tokens),
    cost_usd: normalizeNumeric(raw.cost_usd),
    status: raw.status ?? "unknown",
    error: raw.error ?? null,
    created_at: normalizeTime(raw.created_at),
  };
}
