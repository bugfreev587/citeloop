import type { Article, SEOContentAction, SEOOpportunity, Topic, VisibilityActionInLoop } from "./api";

type TraceSeed = string | null | undefined;

const PLATFORM_LABELS: Record<string, string> = {
  blog: "Blog",
  canonical: "Blog",
  dev_to: "Dev.to",
  hashnode: "Hashnode",
  hacker_news: "Hacker News",
  linkedin: "LinkedIn",
  medium: "Medium",
  reddit: "Reddit",
  syndication_variant: "Syndication",
};

function normalizedSeed(seed: TraceSeed) {
  return typeof seed === "string" ? seed.trim() : "";
}

export function workflowTraceLabel(...seeds: TraceSeed[]) {
  const seed = seeds.map(normalizedSeed).find(Boolean) ?? "unknown";
  let hash = 2166136261;
  for (let index = 0; index < seed.length; index += 1) {
    hash ^= seed.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return `OPP-${(hash >>> 0).toString(36).toUpperCase().padStart(6, "0").slice(0, 6)}`;
}

export function workflowTraceLabelForOpportunity(opportunity: Pick<SEOOpportunity, "id"> | null | undefined) {
  return workflowTraceLabel(opportunity?.id);
}

export function workflowTraceLabelForAction(
  action: Pick<SEOContentAction, "opportunity_id" | "id"> | Pick<VisibilityActionInLoop, "opportunity_id" | "id"> | null | undefined,
) {
  return workflowTraceLabel(action?.opportunity_id, action?.id);
}

export function workflowTraceLabelForTopic(topic: Pick<Topic, "source_content_action_id" | "id"> | null | undefined) {
  return workflowTraceLabel(topic?.source_content_action_id, topic?.id);
}

export function workflowTraceLabelForArticle(article: Pick<Article, "seo_meta" | "topic_id" | "id"> | null | undefined) {
  return workflowTraceLabel(
    article?.seo_meta?.source_opportunity_id,
    article?.seo_meta?.opportunity_id,
    article?.seo_meta?.source_content_action_id,
    article?.seo_meta?.content_action_id,
    article?.topic_id,
    article?.id,
  );
}

export function workflowPlatformLabel(platform: string | null | undefined) {
  const normalized = normalizedSeed(platform).toLowerCase();
  if (!normalized) return "";
  return PLATFORM_LABELS[normalized] ?? normalized.replace(/_/g, " ");
}

export function workflowArticleTypeTag(article: Pick<Article, "kind" | "platform" | "output_type"> | null | undefined) {
  if (!article) return "Content";
  if (article.platform) return workflowPlatformLabel(article.platform);
  if (article.kind === "canonical") return "Blog";
  if (article.kind === "syndication_variant") return "Syndication";
  return workflowPlatformLabel(article.output_type) || "Content";
}
