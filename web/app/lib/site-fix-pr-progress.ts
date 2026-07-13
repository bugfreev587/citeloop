type PullRequestApplication = {
  status?: unknown;
  failure_reason?: unknown;
  deployed_at?: unknown;
  github_pr_url?: unknown;
  github_pr_state?: unknown;
  merged_at?: unknown;
  verified_at?: unknown;
};

type ProgressSiteFix = {
  status?: unknown;
  failure_reason?: unknown;
  approved_at?: unknown;
  applied_at?: unknown;
  deployed_at?: unknown;
  verified_at?: unknown;
  application?: PullRequestApplication | null;
};

type ReadinessInput = {
  readiness?: { status?: unknown; detail?: unknown } | null;
  loading?: boolean;
  fetchError?: string | null;
};

export type SiteFixReadinessGate = {
  allowed: boolean;
  tone: "green" | "amber" | "red";
  title: string;
  detail: string;
};

export type SiteFixPullRequestAction =
  | { kind: "approve" | "apply"; label: string; busyLabel: string }
  | { kind: "open_pr"; label: "Open PR"; href: string };

export function validSiteFixPullRequestURL(value: unknown): string {
  if (typeof value !== "string" || !value.trim()) return "";
  try {
    const url = new URL(value.trim());
    if (
      url.protocol !== "https:" ||
      url.hostname.toLowerCase() !== "github.com" ||
      url.port ||
      url.username ||
      url.password ||
      url.search ||
      url.hash
    ) {
      return "";
    }
    const match = url.pathname.match(/^\/([A-Za-z0-9](?:[A-Za-z0-9-]{0,38}))\/([A-Za-z0-9_.-]+)\/pull\/([1-9][0-9]*)$/);
    return match ? `https://github.com/${match[1]}/${match[2]}/pull/${match[3]}` : "";
  } catch {
    return "";
  }
}

export function siteFixReadinessGate({ readiness, loading = false, fetchError = null }: ReadinessInput): SiteFixReadinessGate {
  if (loading) {
    return {
      allowed: false,
      tone: "amber",
      title: "Checking GitHub readiness",
      detail: "Wait for the stored repository and pull-request readiness result before continuing.",
    };
  }
  if (fetchError) {
    return {
      allowed: false,
      tone: "red",
      title: "GitHub readiness could not be loaded",
      detail: "Refresh this page or review the Publisher connection before creating a repair PR.",
    };
  }
  if (readiness?.status === "ready") {
    return {
      allowed: true,
      tone: "green",
      title: "GitHub is ready for repair PRs",
      detail: "CiteLoop can create a branch and pull request for this Site Fix.",
    };
  }

  const backendDetail = typeof readiness?.detail === "string" && readiness.detail.trim() ? readiness.detail.trim() : "";
  switch (readiness?.status) {
    case "not_connected":
      return {
        allowed: false,
        tone: "amber",
        title: "Connect GitHub before creating a repair PR",
        detail: backendDetail || "Choose an enabled GitHub publisher repository and branch in Settings.",
      };
    case "not_checked":
      return {
        allowed: false,
        tone: "amber",
        title: "GitHub readiness has not been checked",
        detail: backendDetail || "Run the readiness check in Publisher settings before creating a repair PR.",
      };
    case "permission_missing":
      return {
        allowed: false,
        tone: "red",
        title: "GitHub write access is missing",
        detail: backendDetail || "Grant contents and pull-request write access, then check readiness again.",
      };
    case "repository_unavailable":
      return {
        allowed: false,
        tone: "red",
        title: "The repository or base branch is unavailable",
        detail: backendDetail || "Confirm the configured repository and branch, then check readiness again.",
      };
    case "error":
      return {
        allowed: false,
        tone: "red",
        title: "GitHub readiness needs attention",
        detail: backendDetail || "Review the Publisher connection and run its readiness check again.",
      };
    default:
      return {
        allowed: false,
        tone: "amber",
        title: "GitHub readiness is unavailable",
        detail: "Refresh this page or review Publisher settings before creating a repair PR.",
      };
  }
}

export function siteFixPullRequestAction(fix: ProgressSiteFix): SiteFixPullRequestAction | null {
  const pullRequestURL = validSiteFixPullRequestURL(fix.application?.github_pr_url);
  if (pullRequestURL) return { kind: "open_pr", label: "Open PR", href: pullRequestURL };

  return siteFixPullRequestMutationAction(fix);
}

export function siteFixPullRequestMutationAction(fix: ProgressSiteFix): SiteFixPullRequestAction | null {
  switch (fix.status) {
    case "proposed":
      return { kind: "approve", label: "Approve fix", busyLabel: "Approving & creating PR..." };
    case "approved":
    case "ready_to_apply":
      return { kind: "apply", label: "Create PR", busyLabel: "Creating PR..." };
    case "preparing":
      return fix.failure_reason || fix.application?.failure_reason
        ? { kind: "apply", label: "Retry PR creation", busyLabel: "Retrying PR creation..." }
        : null;
    default:
      return null;
  }
}

export function canonicalSiteFixMilestones(fix: ProgressSiteFix) {
  const status = typeof fix.status === "string" ? fix.status : "";
  const approved = Boolean(fix.approved_at) || [
    "approved",
    "preparing",
    "ready_to_apply",
    "applying",
    "awaiting_deploy",
    "verifying",
    "failed_retryable",
    "reopened",
    "verified",
  ].includes(status);
  const deploymentProvingStatus = ["verifying", "verified"].includes(status);
  const complete = [
    true,
    approved,
    Boolean(fix.deployed_at || fix.application?.deployed_at || deploymentProvingStatus),
    Boolean(fix.verified_at || fix.application?.verified_at || status === "verified"),
  ];
  const firstIncomplete = complete.findIndex((value) => !value);
  return ["Finding", "Approved", "Applied / deploy", "Verified"].map((label, index) => ({
    label,
    complete: complete[index],
    current: firstIncomplete !== -1 && index === firstIncomplete,
  }));
}

export function shouldPollSiteFixLifecycle({ drawerOpen, fix }: { drawerOpen: boolean; fix?: ProgressSiteFix | null }) {
  if (!drawerOpen || !fix || fix.failure_reason || fix.application?.failure_reason) return false;
  const status = typeof fix.status === "string" ? fix.status : "";
  if ([
    "proposed",
    "approved",
    "ready_to_apply",
    "conflict",
    "failed_retryable",
    "reopened",
    "verified",
    "failed_terminal",
    "superseded",
    "migration_rolled_back",
  ].includes(status)) {
    return false;
  }
  if (["preparing", "awaiting_deploy", "verifying", "creating_pr", "open", "deployment_pending", "verification_pending"].includes(status)) {
    return true;
  }
  const applicationStatus = typeof fix.application?.status === "string" ? fix.application.status : "";
  const pullRequestState = typeof fix.application?.github_pr_state === "string" ? fix.application.github_pr_state : "";
  return ["creating_pr", "github_pr_open", "open", "deployment_pending", "verification_pending"].includes(applicationStatus) ||
    pullRequestState === "open";
}

export function canonicalSiteFixProgressText(fix: ProgressSiteFix) {
  const status = typeof fix.status === "string" ? fix.status : "";
  const applicationStatus = typeof fix.application?.status === "string" ? fix.application.status : "";
  const pullRequestState = typeof fix.application?.github_pr_state === "string" ? fix.application.github_pr_state : "";
  if (status === "verified" || fix.verified_at || fix.application?.verified_at) return "Verified";
  if (["verifying", "verification_pending"].includes(status) || applicationStatus === "verification_pending") return "Checking the production change";
  if (
    ["awaiting_deploy", "deployment_pending", "merged", "github_pr_merged"].includes(status) ||
    ["deployment_pending", "merged", "github_pr_merged"].includes(applicationStatus) ||
    pullRequestState === "merged"
  ) {
    return "PR merged - waiting for deploy";
  }
  if (["open", "github_pr_open"].includes(status) || ["github_pr_open", "open"].includes(applicationStatus) || pullRequestState === "open") {
    return "Waiting for PR review and merge";
  }
  if (applicationStatus === "creating_pr" || status === "creating_pr") return "Creating the repair PR";
  return "";
}
