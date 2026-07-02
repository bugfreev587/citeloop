const CANONICAL_APP_ORIGIN = "https://citeloop.app";
const PRODUCTION_ALIAS_HOSTS = new Set(["citeloop.vercel.app", "www.citeloop.app"]);

function isProductionAliasHost(hostname: string) {
  return PRODUCTION_ALIAS_HOSTS.has(hostname.toLowerCase());
}

function canonicalizeAbsoluteURL(rawURL: string, canonicalOrigin: string) {
  const url = new URL(rawURL);
  if (!isProductionAliasHost(url.hostname)) return { changed: false, value: rawURL };

  const canonical = new URL(canonicalOrigin);
  url.protocol = canonical.protocol;
  url.host = canonical.host;
  return { changed: true, value: url.toString() };
}

export function canonicalAppURLForRequest(rawURL: string, canonicalOrigin = CANONICAL_APP_ORIGIN): string | null {
  let url: URL;
  try {
    url = new URL(rawURL);
  } catch {
    return null;
  }

  let changed = false;
  if (isProductionAliasHost(url.hostname)) {
    const canonical = new URL(canonicalOrigin);
    url.protocol = canonical.protocol;
    url.host = canonical.host;
    changed = true;
  }

  const redirectURL = url.searchParams.get("redirect_url");
  if (redirectURL) {
    try {
      const canonicalRedirect = canonicalizeAbsoluteURL(redirectURL, canonicalOrigin);
      if (canonicalRedirect.changed) {
        url.searchParams.set("redirect_url", canonicalRedirect.value);
        changed = true;
      }
    } catch {
      // Leave malformed redirect_url values untouched; Clerk will handle them.
    }
  }

  return changed ? url.toString() : null;
}
