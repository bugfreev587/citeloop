export type PublishTarget = {
  baseURL: string;
  branch: string;
};

// The public URL is built as base_url + "/" + slug, so the base must be the
// site's blog root (no slug). Derive that from the project's own domain and the
// content directory leaf: https://example.com + content/citeloop/blog ->
// https://example.com/blog.
export function deriveBaseURL(siteURL: string, contentDir: string): string {
  const root = normalizeDomain(siteURL);
  if (!root) return "";
  const leaf = (contentDir || "").split("/").filter(Boolean).pop() || "";
  return leaf ? `${root}/${leaf}` : root;
}

export function deriveGitHubBranch(siteURL: string): string {
  const root = normalizeDomain(siteURL);
  if (!root) return "";
  try {
    const host = new URL(root).hostname.toLowerCase();
    switch (host) {
      case "dev.unipost.dev":
        return "dev";
      case "staging.unipost.dev":
        return "staging";
      case "unipost.dev":
        return "main";
      default:
        return "";
    }
  } catch {
    return "";
  }
}

export function derivePublishTarget(siteURL: string, contentDir: string): PublishTarget {
  return {
    baseURL: deriveBaseURL(siteURL, contentDir),
    branch: deriveGitHubBranch(siteURL),
  };
}

// A bare hostname like "staging.unipost.dev" -- dotted, no spaces/scheme/path.
function looksLikeHostname(raw: string): boolean {
  const v = (raw || "").trim();
  return /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/i.test(v);
}

// normalizeDomain turns a configured site_url OR a domain-shaped project name
// into a scheme-qualified root (no trailing slash). Returns "" if it isn't
// usable as a domain (e.g. a human project name like "UniPost (placeholder)").
export function normalizeDomain(raw: string): string {
  const v = (raw || "").trim();
  if (!v) return "";
  if (/^https?:\/\//i.test(v)) {
    try {
      return new URL(v).origin;
    } catch {
      return "";
    }
  }
  const trimmed = v.replace(/\/+$/, "");
  if (looksLikeHostname(trimmed)) return `https://${trimmed}`;
  try {
    const parsed = new URL(`https://${trimmed}`);
    if (parsed.hostname.includes(".")) return parsed.origin;
  } catch {
    return "";
  }
  return "";
}
