type ReviewArticleLike = {
  content_md: string;
  seo_meta: Record<string, any>;
  canonical_url: string | null;
  resolved_slug: string | null;
};

type PreviewHrefArticleLike = ReviewArticleLike & {
  id: string;
};

type RepairableArticleLike = ReviewArticleLike & {
  qa_blocking: boolean;
  repair_attempts?: number;
  repair_status?: string;
  requires_human_decision?: boolean;
  qa_feedback?: Record<string, any>;
  qa_issues?: string[];
};

type ReviewQueueGroupLike = {
  topic_id: string;
  articles: RepairableArticleLike[];
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

export type ReviewArticleState = {
  kind: "ready" | "auto_repair" | "needs_human";
  label: string;
  detail: string;
  approvable: boolean;
};

export type ReviewQueueSummary = {
  total: number;
  bundleCount: number;
  ready: number;
  autoRepair: number;
  needsHuman: number;
  blocked: number;
};

export type QAClaimRow = {
  claim: string;
  mapped: boolean;
  evidence: string;
  status: "mapped" | "unmapped";
};

export type SearchAppearanceRow = {
  label: string;
  value: string;
  detail: string;
};

export type PublishedPreviewParts = {
  title: string;
  blocks: string[];
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

export function articlePreviewHref(projectId: string, article: PreviewHrefArticleLike) {
  const canonical = textValue(article.canonical_url);
  if (canonical) return canonical;
  return `/preview/projects/${projectId}/articles/${article.id}`;
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

function isThematicBreak(block: string) {
  return /^(---|\*\*\*|___)$/.test(block.trim());
}

function isMetadataLineBlock(block: string) {
  return /^(title|meta description|slug|h1):/i.test(block.trim());
}

function isGenerationInstructionBlock(block: string) {
  const normalized = block.trim().toLowerCase();
  if (!normalized || normalized.length > 320) return false;
  return /^(explain|write|create|draft|generate|produce|cover|compare|discuss)\b/.test(normalized);
}

function stripLeadingGenerationInstructions(blocks: string[]) {
  let startIndex = 0;
  while (startIndex < blocks.length && isGenerationInstructionBlock(blocks[startIndex])) {
    startIndex += 1;
  }
  return blocks.slice(startIndex);
}

export function publishedPreviewParts(content: string, h1: string): PublishedPreviewParts {
  const blocks = articlePreviewBlocks(content, h1);
  const h1Index = blocks.findIndex((block) => block.trim().startsWith("# "));
  const title = h1Index >= 0 ? blocks[h1Index].trim().replace(/^#\s+/, "") : h1;
  let bodyBlocks = blocks.filter((_, index) => index !== h1Index);
  const metaIndex = bodyBlocks.findIndex((block) => /^##\s+Meta Information\b/i.test(block.trim()));

  if (metaIndex >= 0) {
    const separatorIndex = bodyBlocks.findIndex((block, index) => index > metaIndex && isThematicBreak(block));
    bodyBlocks =
      separatorIndex >= 0
        ? bodyBlocks.slice(separatorIndex + 1)
        : bodyBlocks.slice(metaIndex + 1).filter((block) => !isMetadataLineBlock(block));
  }

  return {
    title: title || "Untitled draft",
    blocks: stripLeadingGenerationInstructions(bodyBlocks.filter((block) => !isThematicBreak(block))),
  };
}

export function publishedPreviewDescription(description: string) {
  const value = textValue(description);
  if (isGenerationInstructionBlock(value)) return "";
  return value;
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
  if (article.requires_human_decision) return false;
  if ((article.repair_attempts ?? 0) >= 2) return false;
  if (article.repair_status === "repairing" || article.repair_status === "exhausted" || article.repair_status === "human_decision") return false;
  if (article.qa_blocking) return true;
  return buildSEOContributions(article).some((row) => row.status === "missing" || (row.label === "Search intent" && row.status !== "ready"));
}

export function reviewArticleState(article: RepairableArticleLike): ReviewArticleState {
  if (!article.qa_blocking) {
    return {
      kind: "ready",
      label: "Ready to approve",
      detail: "QA has cleared this draft.",
      approvable: true,
    };
  }

  const repairAttempts = article.repair_attempts ?? 0;
  if (article.requires_human_decision || repairAttempts >= 2 || article.repair_status === "exhausted" || article.repair_status === "human_decision") {
    return {
      kind: "needs_human",
      label: "Needs human decision",
      detail: "Automatic repair budget is spent or CiteLoop needs a source decision.",
      approvable: false,
    };
  }

  if (article.repair_status === "repairing") {
    return {
      kind: "auto_repair",
      label: "Auto repair active",
      detail: "CiteLoop is repairing the draft and will rerun QA.",
      approvable: false,
    };
  }

  if (shouldAutoRepairArticle(article)) {
    return {
      kind: "auto_repair",
      label: "Auto repair queued",
      detail: "CiteLoop can attempt a safe repair before asking you.",
      approvable: false,
    };
  }

  return {
    kind: "needs_human",
    label: "Needs human decision",
    detail: "QA is blocking approval and no automatic repair path is available.",
    approvable: false,
  };
}

export function reviewQueueSummary(groups: ReviewQueueGroupLike[]): ReviewQueueSummary {
  const summary: ReviewQueueSummary = {
    total: 0,
    bundleCount: groups.length,
    ready: 0,
    autoRepair: 0,
    needsHuman: 0,
    blocked: 0,
  };

  for (const article of groups.flatMap((group) => group.articles)) {
    summary.total += 1;
    if (article.qa_blocking) summary.blocked += 1;
    const state = reviewArticleState(article);
    if (state.kind === "ready") summary.ready += 1;
    if (state.kind === "auto_repair") summary.autoRepair += 1;
    if (state.kind === "needs_human") summary.needsHuman += 1;
  }

  return summary;
}

export function qaClaimRows(article: Pick<RepairableArticleLike, "qa_feedback" | "qa_issues">): QAClaimRow[] {
  const claims = Array.isArray(article.qa_feedback?.claims) ? article.qa_feedback?.claims : [];
  const rows = claims
    .map((claim) => {
      const text = textValue(claim?.claim);
      if (!text) return null;
      const mapped = Boolean(claim?.mapped);
      return {
        claim: text,
        mapped,
        evidence: textValue(claim?.evidence),
        status: mapped ? ("mapped" as const) : ("unmapped" as const),
      };
    })
    .filter((row): row is QAClaimRow => row !== null);

  if (rows.length > 0) return rows;

  return (article.qa_issues ?? [])
    .map((issue) => issue.match(/^unmapped product claim:\s*(.+)$/i)?.[1])
    .filter((claim): claim is string => Boolean(claim))
    .map((claim) => ({
      claim: claim.trim(),
      mapped: false,
      evidence: "",
      status: "unmapped" as const,
    }));
}

export function searchAppearanceRows(article: ReviewArticleLike): SearchAppearanceRow[] {
  const keyword = textValue(article.seo_meta?.target_keyword) || textValue(article.seo_meta?.keyword);
  const title = textValue(article.seo_meta?.title);
  const description = textValue(article.seo_meta?.meta_description);
  const slug = articleReviewSlug(article);

  return [
    {
      label: "Target keyword",
      value: keyword || "Not specified",
      detail: "Existing SEO metadata used as the draft's query target.",
    },
    {
      label: "Result title",
      value: title || "Missing title",
      detail: "Existing SEO title metadata.",
    },
    {
      label: "Description",
      value: description || "Missing meta description",
      detail: "Existing meta description metadata.",
    },
    {
      label: "URL slug",
      value: slug || "Missing slug",
      detail: "Existing resolved or SEO slug.",
    },
  ];
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
