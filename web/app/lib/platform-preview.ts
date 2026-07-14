type PreviewArticle = {
  platform?: string | null;
  output_type?: string;
  platform_contract_version?: string | null;
  platform_metadata?: Record<string, unknown>;
  contract_validation?: { passed?: boolean; failures?: Array<{ message?: string }>; warnings?: Array<{ message?: string }> };
  content_md?: string;
};

export type PlatformPreview = {
  title: string;
  platform: string;
  outputType: string;
  contractVersion: string;
  detailLines: string[];
  bodyLabel: string;
  validationPassed: boolean;
  validationMessages: string[];
};

export function platformPreview(article: PreviewArticle): PlatformPreview {
  const metadata = article.platform_metadata ?? {};
  const stringValue = (key: string) => typeof metadata[key] === "string" ? String(metadata[key]) : "";
  const details = ["subreddit", "post_type", "flair", "publication", "canonical_url", "url", "description"]
    .map((key) => stringValue(key) ? `${key.replaceAll("_", " ")}: ${stringValue(key)}` : "")
    .filter(Boolean);
  const validation = article.contract_validation ?? {};
  return {
    title: stringValue("title") || "Platform artifact",
    platform: article.platform || "blog",
    outputType: article.output_type || "long_form_article",
    contractVersion: article.platform_contract_version || "legacy",
    detailLines: details,
    bodyLabel: article.platform === "hacker_news" && !article.content_md?.trim()
      ? "No generated comment or article body"
      : "Native content body",
    validationPassed: validation.passed !== false,
    validationMessages: [...(validation.failures ?? []), ...(validation.warnings ?? [])].map((item) => item.message || "Contract validation issue"),
  };
}
