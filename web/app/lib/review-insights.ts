type ReviewArticleLike = {
  content_md: string;
  seo_meta: Record<string, any>;
  canonical_url: string | null;
  resolved_slug: string | null;
};

type RepairableArticleLike = ReviewArticleLike & {
  qa_blocking: boolean;
};

export type SEOContribution = {
  label: string;
  value: string;
  detail: string;
  status: "ready" | "missing" | "needs_review";
};

export type ExplainedQAIssue = {
  title: string;
  detail: string;
  action: string;
  raw: string;
};

function textValue(value: any): string {
  return typeof value === "string" ? value.trim() : "";
}

function markdownHeadingCount(content: string, level: number) {
  const marker = "#".repeat(level);
  return content
    .split("\n")
    .filter((line) => line.trimStart().startsWith(marker + " "))
    .length;
}

function markdownLinkCount(content: string) {
  return (content.match(/\[[^\]]+\]\([^)]+\)/g) ?? []).length;
}

function wordCount(content: string) {
  return content
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/[#*_>`[\]()]/g, " ")
    .split(/\s+/)
    .filter(Boolean).length;
}

export function articleReviewTitle(article: ReviewArticleLike) {
  return textValue(article.seo_meta?.title) || textValue(article.seo_meta?.h1) || textValue(article.seo_meta?.slug) || "Untitled draft";
}

export function articleReviewSlug(article: ReviewArticleLike) {
  return textValue(article.resolved_slug) || textValue(article.seo_meta?.slug);
}

export function previewPath(article: ReviewArticleLike) {
  const slug = articleReviewSlug(article);
  if (!slug) return "/blog/draft-preview";
  return `/blog/${slug}`;
}

export function markdownBlocks(content: string) {
  return content
    .trim()
    .split(/\n{2,}/)
    .map((block) => block.trim())
    .filter(Boolean);
}

export function articlePreviewBlocks(content: string, h1: string) {
  const blocks = markdownBlocks(content);
  if (!h1 || blocks.some((block) => block.startsWith("# "))) return blocks;
  return [`# ${h1}`, ...blocks];
}

export function buildSEOContributions(article: ReviewArticleLike): SEOContribution[] {
  const title = textValue(article.seo_meta?.title);
  const description = textValue(article.seo_meta?.meta_description);
  const slug = articleReviewSlug(article);
  const h1 = textValue(article.seo_meta?.h1);
  const keyword = textValue(article.seo_meta?.target_keyword) || textValue(article.seo_meta?.keyword);
  const h2Count = markdownHeadingCount(article.content_md, 2);
  const links = markdownLinkCount(article.content_md);
  const words = wordCount(article.content_md);

  return [
    {
      label: "Search intent",
      value: keyword || "Keyword not specified",
      detail: keyword ? "Gives the draft a clear query target." : "CiteLoop will infer the target query before asking for approval.",
      status: keyword ? "ready" : "needs_review",
    },
    {
      label: "SERP title",
      value: title || "Missing title",
      detail: title ? `${title.length} characters. This is the clickable result headline.` : "Missing title weakens search result relevance.",
      status: title ? "ready" : "missing",
    },
    {
      label: "Meta description",
      value: description || "Missing meta description",
      detail: description
        ? `${description.length} characters. This frames the search-result snippet.`
        : "Missing description makes the snippet harder to control.",
      status: description ? "ready" : "missing",
    },
    {
      label: "URL slug",
      value: slug || "Missing slug",
      detail: slug ? "Readable slug can be published under the UniPost blog path." : "Missing slug can create poor or unsafe publish paths.",
      status: slug ? "ready" : "missing",
    },
    {
      label: "On-page structure",
      value: `${words} words, ${h2Count} H2 sections`,
      detail: h1 ? `H1 is present: ${h1}` : "Reviewer should confirm the visible H1 before approval.",
      status: h1 && h2Count > 0 ? "ready" : "needs_review",
    },
    {
      label: "Internal links",
      value: `${links} markdown links`,
      detail: links > 0 ? "Links help connect this article to existing product/context pages." : "CiteLoop will add safe internal links when evidence allows.",
      status: links > 0 ? "ready" : "needs_review",
    },
  ];
}

export function shouldAutoRepairArticle(article: RepairableArticleLike) {
  if (article.qa_blocking) return true;
  return buildSEOContributions(article).some((row) => row.status === "missing" || (row.label === "Search intent" && row.status !== "ready"));
}

export function explainQAIssue(issue: string): ExplainedQAIssue {
  const normalized = issue.toLowerCase();
  if (normalized.includes("missing claims")) {
    return {
      title: "QA evidence map was not returned",
      detail:
        "The QA step expects a claims array so it can map product claims to evidence. The model response did not include that structure, so CiteLoop blocked approval conservatively.",
      action: "CiteLoop automatically revises or normalizes the draft, will rerun QA, and only returns unresolved choices to you.",
      raw: issue,
    };
  }
  if (normalized.includes("parse qa")) {
    return {
      title: "QA response could not be parsed",
      detail:
        "The auditor returned output that did not match the required JSON schema for evidence mapping, scores, and issues.",
      action: "CiteLoop automatically reruns repair plus QA before asking for a manual decision.",
      raw: issue,
    };
  }
  if (normalized.includes("unmapped product claim")) {
    return {
      title: "Product claim needs evidence",
      detail: issue.replace(/^unmapped product claim:\s*/i, ""),
      action: "CiteLoop automatically removes or rewrites the claim against known evidence. Review only if the evidence is genuinely ambiguous.",
      raw: issue,
    };
  }
  return {
    title: "QA blocking issue",
    detail: issue,
    action: "CiteLoop automatically revises the draft and reruns QA. If it remains blocked, choose from the remaining manual options.",
    raw: issue,
  };
}
