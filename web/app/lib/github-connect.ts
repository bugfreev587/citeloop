const GITHUB_CONNECT_PROJECT_KEY = "citeloop.githubConnectProjectID";
const GITHUB_CONNECT_RETURN_KEY = "citeloop.githubConnectReturnHref";

function browserStorage(): Storage | null {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

export function rememberGithubConnectProject(projectID: string, returnHref?: string) {
  const trimmed = projectID.trim();
  if (!trimmed) return;
  const storage = browserStorage();
  storage?.setItem(GITHUB_CONNECT_PROJECT_KEY, trimmed);
  if (returnHref?.trim()) {
    storage?.setItem(GITHUB_CONNECT_RETURN_KEY, returnHref.trim());
  }
}

export function resolveGithubCallbackProjectID(stateParam: string): string {
  const fromState = stateParam.trim();
  if (fromState) return fromState;
  return browserStorage()?.getItem(GITHUB_CONNECT_PROJECT_KEY)?.trim() ?? "";
}

export function resolveGithubCallbackReturnHref(projectID: string): string {
  const trimmedProjectID = projectID.trim();
  const fallback = trimmedProjectID ? `/projects/${trimmedProjectID}/publishing?github=connected` : "/";
  const stored = browserStorage()?.getItem(GITHUB_CONNECT_RETURN_KEY)?.trim() ?? "";
  if (!trimmedProjectID || !stored || !stored.startsWith("/") || stored.startsWith("//") || stored.includes("\\")) {
    return fallback;
  }
  if (stored === `/projects/${trimmedProjectID}` || stored.startsWith(`/projects/${trimmedProjectID}/`) || stored.startsWith(`/projects/${trimmedProjectID}?`)) {
    return stored;
  }
  return fallback;
}

export function forgetGithubConnectProject() {
  const storage = browserStorage();
  storage?.removeItem(GITHUB_CONNECT_PROJECT_KEY);
  storage?.removeItem(GITHUB_CONNECT_RETURN_KEY);
}
