const GITHUB_CONNECT_PROJECT_KEY = "citeloop.githubConnectProjectID";

function browserStorage(): Storage | null {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

export function rememberGithubConnectProject(projectID: string) {
  const trimmed = projectID.trim();
  if (!trimmed) return;
  browserStorage()?.setItem(GITHUB_CONNECT_PROJECT_KEY, trimmed);
}

export function resolveGithubCallbackProjectID(stateParam: string): string {
  const fromState = stateParam.trim();
  if (fromState) return fromState;
  return browserStorage()?.getItem(GITHUB_CONNECT_PROJECT_KEY)?.trim() ?? "";
}

export function forgetGithubConnectProject() {
  browserStorage()?.removeItem(GITHUB_CONNECT_PROJECT_KEY);
}
