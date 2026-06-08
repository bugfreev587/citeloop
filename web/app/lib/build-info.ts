import type { DeploymentBuild } from "./api";

function firstEnv(keys: string[]) {
  for (const key of keys) {
    const value = process.env[key];
    if (value) return value;
  }
  return "";
}

export function getWebBuildInfo(): DeploymentBuild {
  return {
    service: "citeloop-web",
    commit_sha: firstEnv([
      "NEXT_PUBLIC_CITELOOP_GIT_SHA",
      "VERCEL_GIT_COMMIT_SHA",
      "NEXT_PUBLIC_VERCEL_GIT_COMMIT_SHA",
      "COMMIT_SHA",
      "GIT_SHA",
    ]),
    commit_ref: firstEnv([
      "NEXT_PUBLIC_CITELOOP_GIT_REF",
      "VERCEL_GIT_COMMIT_REF",
      "NEXT_PUBLIC_VERCEL_GIT_COMMIT_REF",
      "BRANCH_NAME",
      "GIT_BRANCH",
    ]),
    deployment_id: firstEnv([
      "NEXT_PUBLIC_CITELOOP_DEPLOYMENT_ID",
      "VERCEL_DEPLOYMENT_ID",
      "NEXT_PUBLIC_VERCEL_DEPLOYMENT_ID",
    ]),
    environment: firstEnv(["NEXT_PUBLIC_CITELOOP_ENV", "VERCEL_ENV", "NODE_ENV"]),
    url: firstEnv(["NEXT_PUBLIC_CITELOOP_URL", "VERCEL_URL", "NEXT_PUBLIC_VERCEL_URL"]),
  };
}
