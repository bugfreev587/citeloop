"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertTriangle, ArrowRight, Bell, CheckCircle2, GitBranch, ListChecks, Plus, Power, RefreshCw, RotateCcw, Save, Search, Send, Settings2, Trash2, X } from "lucide-react";
import {
  AutopilotReadiness,
  defaultProjectConfig,
  DoctorAIRunPolicy,
  GSCConnection,
  GitHubNextJSPublisherInput,
  GithubIntegrationStatus,
  GithubPRReadiness,
  NotificationChannel,
  NotificationChannelKind,
  NotificationDelivery,
  NotificationEvent,
  NotificationSubscription,
  PlatformTargetContext,
  GrowthAIRunPolicy,
  PublisherConnection,
  ProjectConfig,
  SafeModeEvent,
  SEOIntegration,
  SEOPolicy,
  SEOPolicyUpdateInput,
  SEOProperty,
  friendlyApiError,
} from "../../../lib/api";
import { normalizeNumeric } from "../../../lib/normalize";
import { readinessGateActionFor } from "../../../lib/automation-readiness";
import { rememberGithubConnectProject } from "../../../lib/github-connect";
import {
  createGithubPRReadinessPublisherEntryTracker,
  createGithubPRReadinessRequestOrder,
  createGithubPRReadinessRefreshCoordinator,
  GithubPRReadinessRequestScope,
  GithubPRReadinessRefreshMode,
} from "../../../lib/github-pr-readiness-refresh";
import { useApi } from "../../../lib/use-api";
import { useToast } from "../../../components/toast-provider";
import { Badge, Button, ButtonProgress, Field, Notice, SectionHeader, TextInput, TextArea, cx, formatDate } from "../../../components/ui";
import { RunsClient } from "../runs/runs-client";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;

function toInt(value: string, fallback: number) {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function toFloat(value: string, fallback: number) {
  const parsed = Number.parseFloat(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

// Map raw backend/API error strings to actionable user copy. Falls back to the
// raw message so nothing is hidden, but common validation/auth failures read
// like guidance instead of server jargon.
function friendlyError(raw: unknown) {
  const message = String(raw ?? "").trim();
  const lower = message.toLowerCase();
  if (lower.includes("repo") && lower.includes("base_url")) {
    return "Add both the GitHub repository (owner/repo) and your site's base URL before saving.";
  }
  if (lower.includes("base_url") || lower.includes("base url")) {
    return "Enter a valid site base URL, e.g. https://example.com.";
  }
  if (lower.includes("repo")) {
    return "Enter the GitHub repository as owner/repo.";
  }
  if (lower.includes("resend_api_key") || lower.includes("resend api key")) {
    return "Email sending is not configured yet. Add the Resend API key before testing an email channel.";
  }
  if (lower.includes("email") || lower.includes("destination")) {
    return "Enter a valid email address for this notification channel.";
  }
  if (lower.includes("webhook") || (lower.includes("url") && lower.includes("required"))) {
    return "Enter a valid webhook URL (a Slack or Discord incoming webhook).";
  }
  if (lower.includes("token")) {
    return "The token was rejected. Check that it is valid and has write access to the repository.";
  }
  if (lower.includes("401") || lower.includes("403") || lower.includes("permission") || lower.includes("forbidden")) {
    return "Permission denied. Re-check the connected credentials and their access scope.";
  }
  if (lower.includes("404") || lower.includes("not found")) {
    return "Not found. Check the repository, branch, and content path.";
  }
  return message || "Something went wrong. Please try again.";
}

function friendlyGA4Error(raw: unknown) {
  const message = String(raw ?? "").trim();
  const lower = message.toLowerCase();
  if (lower.includes("user does not have sufficient permissions for this property")) {
    return "Google Analytics property access is missing. Confirm the numeric GA4 Property ID and that the connected Google account can read this GA4 property, then update Analytics access again.";
  }
  if (lower.includes("access_token_scope_insufficient") || lower.includes("insufficient authentication scopes")) {
    return "Google Analytics permission is missing. Update Analytics access so CiteLoop can request Analytics read access, then run SEO sync again.";
  }
  return friendlyError(message);
}

function isProjectScopedMissing(raw: unknown) {
  const lower = String(raw ?? "").toLowerCase();
  return lower.includes("404") && lower.includes("project not found");
}

function gscTone(status?: string): "green" | "amber" | "red" | "neutral" {
  if (status === "connected") return "green";
  if (["error", "expired", "revoked", "stale", "mismatch"].includes(status ?? "")) return "red";
  if (["property_selection_required", "missing", "backfilling"].includes(status ?? "")) return "amber";
  return "neutral";
}

function gscStatusLabel(status?: string) {
  if (status === "property_selection_required") return "select property";
  if (status === "backfilling") return "backfilling";
  if (status === "stale") return "stale";
  if (status === "mismatch") return "mismatch";
  return status ?? "missing";
}

function ga4Tone(status?: string, propertyID?: string): "green" | "amber" | "red" | "neutral" {
  if (status === "connected") return "green";
  if (["error", "expired", "reconnect_required", "property_access_required", "revoked"].includes(status ?? "")) return "red";
  if (propertyID?.trim()) return "amber";
  return "neutral";
}

function ga4StatusLabel(status?: string, propertyID?: string) {
  if (status === "connected") return "connected";
  if (status === "reconnect_required") return "reconnect required";
  if (status === "property_access_required") return "property access required";
  if (status === "error") return "needs attention";
  if (propertyID?.trim()) return "property saved";
  return "not connected";
}

function githubPRReadinessPresentation(status?: GithubPRReadiness["status"]) {
  switch (status) {
    case "ready":
      return {
        title: "Ready to create repair PRs",
        detail: "CiteLoop can reach the selected target and create the branch and pull request needed for a repair.",
        tone: "green" as const,
      };
    case "not_connected":
      return {
        title: "Connect GitHub to create repair PRs",
        detail: "Connect the GitHub App, then choose the repository and branch CiteLoop should repair.",
        tone: "amber" as const,
      };
    case "permission_missing":
      return {
        title: "GitHub needs contents and pull-request write access",
        detail: "Update the GitHub App repository access, then check again.",
        tone: "red" as const,
      };
    case "repository_unavailable":
      return {
        title: "The selected repository or branch is unavailable",
        detail: "Confirm the repository and base branch still exist and are available to the connected GitHub App.",
        tone: "red" as const,
      };
    case "error":
      return {
        title: "GitHub readiness could not be checked",
        detail: "Review the GitHub connection and target, then try the check again.",
        tone: "red" as const,
      };
    case "not_checked":
    default:
      return {
        title: "GitHub readiness needs a check",
        detail: "Run the check to confirm repository, branch, and pull-request access before creating a repair PR.",
        tone: "amber" as const,
      };
  }
}

const githubPRReadinessCheckError = "We couldn't check GitHub readiness. Review the connection and try again.";

const ga4ConnectionSteps = [
  "Open Analytics Home, then select the existing GA4 property for this domain. If you land in the setup wizard, leave the create flow first.",
  "Copy the numeric Property ID from Admin > Property settings, or from the Analytics URL segment after p (for example p123456789).",
  "Click Update Analytics access so Google asks for Analytics read access on the same Google connection.",
  "If Google Analytics still needs attention, run SEO sync again after reconnecting Analytics access.",
  "Save the Property ID, then run SEO sync after Google starts collecting data.",
];

function EmptyGSCProperties() {
  return (
    <div className="rounded-lg border border-dashed border-slate-200 bg-white px-3 py-4 text-sm">
      <div className="font-semibold text-slate-800">No authorized properties yet</div>
      <div className="mt-1 text-slate-500">Connect Search Console, then choose the property that matches this domain.</div>
    </div>
  );
}

function GSCSetupGuide({ siteURL }: { siteURL?: string }) {
  const target = siteURL?.trim() || "this product domain";
  const steps = [
    {
      title: "Set up Search Console property",
      detail: `Add a Domain property for ${target}. Domain properties cover all protocols and subdomains.`,
    },
    {
      title: "Verify DNS ownership",
      detail: "Copy Google's TXT record into your DNS provider and wait for Google to confirm ownership.",
    },
    {
      title: "Connect after verification",
      detail: "Return here, connect Search Console, and select the verified property for this project.",
    },
  ];
  return (
    <div className="grid gap-3 rounded-lg border border-amber-200 bg-amber-50 p-3">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="text-sm font-bold text-amber-950">Set up Search Console property</div>
          <p className="mt-1 max-w-2xl text-sm leading-5 text-amber-800">
            If this domain is not in your Google account yet, verify it in Search Console first. CiteLoop can only import properties Google already shows to you.
          </p>
        </div>
        <a
          href="https://search.google.com/search-console/welcome"
          target="_blank"
          rel="noreferrer"
          className="inline-flex h-9 shrink-0 items-center justify-center rounded-lg border border-amber-300 bg-white px-3 text-sm font-semibold text-amber-900 transition-colors hover:bg-amber-100"
        >
          Open Search Console
        </a>
      </div>
      <div className="grid gap-2 md:grid-cols-3">
        {steps.map((step, index) => (
          <div key={step.title} className="min-w-0 border-t border-amber-200 pt-2 md:border-l md:border-t-0 md:pl-3 md:pt-0">
            <div className="text-xs font-semibold uppercase text-amber-700">Step {index + 1}</div>
            <div className="mt-1 text-sm font-bold text-amber-950">{step.title}</div>
            <p className="mt-1 text-sm leading-5 text-amber-800">{step.detail}</p>
          </div>
        ))}
      </div>
    </div>
  );
}

type ConnectionGuide = {
  name: string;
  state: string;
  steps: string[];
};

const distributionConnectionGuides: ConnectionGuide[] = [
  {
    name: "Hashnode",
    state: "Copy draft only today",
    steps: [
      "Create or select a Hashnode publication.",
      "Confirm API access is available for the publication.",
      "Generate a Personal Access Token in Developer Settings.",
      "Paste the token into CiteLoop when the connector is ready.",
    ],
  },
  {
    name: "LinkedIn",
    state: "Copy draft only today",
    steps: [
      "Sign in with LinkedIn.",
      "Approve CiteLoop posting permissions when OAuth is ready.",
      "Select a personal profile or Company Page.",
      "Keep posts in approval mode by default.",
    ],
  },
  {
    name: "Reddit",
    state: "Submit draft only today",
    steps: [
      "Sign in with Reddit.",
      "Approve submit permissions.",
      "Choose the target subreddit per post.",
      "Review the generated post before submitting.",
    ],
  },
  {
    name: "Hacker News",
    state: "Manual submit",
    steps: [
      "Create or sign in to a Hacker News account.",
      "Open the generated submit draft.",
      "Paste the title and canonical URL.",
      "Mark distributed in CiteLoop after submitting.",
    ],
  },
  {
    name: "Medium",
    state: "Copy draft only today",
    steps: [
      "Create or sign in to a Medium account.",
      "Open a new story from the prepared draft.",
      "Paste the canonical URL when Medium allows it.",
      "Mark distributed in CiteLoop after publishing.",
    ],
  },
];

function ConnectionInstructions({ steps }: { steps: string[] }) {
  return (
    <div className="mt-3 border-t border-slate-100 pt-3">
      <div className="text-xs font-bold uppercase text-slate-400">How to connect</div>
      <ol className="mt-2 grid gap-1.5 text-sm leading-5 text-slate-600">
        {steps.map((step, index) => (
          <li key={step} className="flex gap-2">
            <span className="mt-0.5 inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-slate-100 text-[11px] font-bold text-slate-500">
              {index + 1}
            </span>
            <span>{step}</span>
          </li>
        ))}
      </ol>
    </div>
  );
}

const channelKinds: Array<{ value: NotificationChannelKind; label: string }> = [
  { value: "slack_webhook", label: "Slack" },
  { value: "discord_webhook", label: "Discord" },
  { value: "email", label: "Email" },
];

const eventLabels: Record<string, string> = {
  "generation.failed": "Generation failed",
  "publish.failed": "Publish failed",
  "budget.stopped": "Budget stopped",
  "review.overdue": "Review overdue",
  "sitefix.pr.awaiting_merge": "Site fix PR awaiting merge",
  "webhook.delivery.dead": "Webhook delivery dead",
  "seo.sync.failed": "SEO sync failed",
  "seo.auth.expired": "SEO auth expired",
  "seo.opportunity.ready": "SEO opportunity ready",
  "seo.brief.ready": "SEO brief ready",
  "seo.action.measurement_ready": "SEO measurement ready",
  "seo.indexing.anomaly": "SEO indexing anomaly",
};

const deliveryStatuses = [
  { value: "", label: "All" },
  { value: "pending", label: "Pending" },
  { value: "dead", label: "Dead" },
  { value: "sent", label: "Sent" },
];

const defaultPublisherDraft: GitHubNextJSPublisherInput = {
  label: "GitHub/Next.js",
  repo: "",
  branch: "citeloop-content",
  content_dir: "content/citeloop/blog",
  base_url: "",
  publish_mode: "publish",
  credential_ref: "",
};

type SettingsTabId =
  | "project"
  | "automation"
  | "activity"
  | "search-console"
  | "publisher"
  | "ai-assistance"
  | "crawl"
  | "notifications";

const settingsTabs: Array<{ id: SettingsTabId; title: string }> = [
  { id: "project", title: "Project config" },
  { id: "automation", title: "Automation" },
  { id: "search-console", title: "Google connection" },
  { id: "publisher", title: "Publisher connection" },
  { id: "ai-assistance", title: "AI assistance" },
  { id: "notifications", title: "Notifications" },
  { id: "crawl", title: "Crawl config" },
  { id: "activity", title: "Activity Log" },
];

// Deep-link anchors map to their owning tab so that /settings#automation-policy and
// /settings#recovery-plan open the Automation panel before the browser scrolls to the anchor.
const settingsAnchorToTab: Record<string, SettingsTabId> = {
  project: "project",
  automation: "automation",
  "automation-status": "automation",
  "automation-policy": "automation",
  "recovery-plan": "automation",
  activity: "activity",
  "search-console": "search-console",
  publisher: "publisher",
  "reddit-rules": "publisher",
  "ai-assistance": "ai-assistance",
  "opportunity-finding": "ai-assistance",
  crawl: "crawl",
  notifications: "notifications",
};

function settingsTabFromHash(hash: string): SettingsTabId {
  const tabId = hash.replace(/^#/, "");
  return settingsAnchorToTab[tabId] ?? "project";
}

function automationCheckIdFromHash(hash: string): string | null {
  const anchor = hash.replace(/^#/, "");
  if (anchor === "automation-status") return "automation_pause_clear";
  if (anchor === "automation-policy") return "autopilot_policy_confirmed";
  if (anchor === "recovery-plan") return "rollback_or_recovery_ready";
  return null;
}

type PolicyDraft = {
  autopilot_level: number;
  automation_paused: boolean;
  monthly_budget_limit: number;
  kill_switch_enabled: boolean;
  safe_mode_enabled: boolean;
};

const defaultPolicyDraft: PolicyDraft = {
  autopilot_level: 0,
  automation_paused: false,
  monthly_budget_limit: 0,
  kill_switch_enabled: false,
  safe_mode_enabled: false,
};

const doctorAIRunPolicies: Array<{
  value: DoctorAIRunPolicy;
  label: string;
  summary: string;
  detail: string;
}> = [
  {
    value: "automatic",
    label: "Automatic",
    summary: "Run approved Doctor AI work automatically",
    detail: "Doctor may call the configured AI provider for eligible diagnosis, fix, and verification work without another click.",
  },
  {
    value: "on_demand",
    label: "On demand",
    summary: "Recommend AI, then wait",
    detail: "Doctor can recommend AI assistance, but a user action is required before a provider call.",
  },
  {
    value: "manual_only",
    label: "Manual only",
    summary: "Only explicit user requests",
    detail: "Doctor calls the provider only from an explicit action you take.",
  },
];

const growthAIRunPolicies: Array<{
  value: GrowthAIRunPolicy;
  label: string;
  summary: string;
  detail: string;
}> = [
  {
    value: "scheduled_and_event",
    label: "Automatic",
    summary: "Scheduled and approved events",
    detail: "Opportunities may call AI on schedule and after eligible context, publish, or measurement events.",
  },
  {
    value: "scheduled_only",
    label: "Scheduled only",
    summary: "Keep scheduled authority",
    detail: "AI can run during scheduled Opportunity Finding, but events cannot start new provider calls.",
  },
  {
    value: "on_demand_recommended",
    label: "On demand",
    summary: "Recommend AI, then wait",
    detail: "Opportunities can recommend an AI run, but waits for a user action before spending tokens.",
  },
  {
    value: "manual_only",
    label: "Manual only",
    summary: "Only explicit user requests",
    detail: "Opportunities calls the provider only when you explicitly request a run.",
  },
];

const automationPolicyLevels = [
  {
    value: 0,
    title: "Level 0 Observe only",
    mode: "Observe",
    detail: "CiteLoop analyzes, plans, and reports. It does not create drafts, run scheduled automation, or publish changes.",
    available: true,
  },
  {
    value: 1,
    title: "Level 1 Draft only",
    mode: "Draft",
    detail: "CiteLoop can create drafts for review. A person still approves and publishes every change.",
    available: true,
  },
  {
    value: 2,
    title: "Level 2 Guarded execution",
    mode: "Guarded",
    detail: "CiteLoop can execute low-risk actions inside policy and budget limits. Medium and high-risk work still waits for review.",
    available: true,
  },
  {
    value: 3,
    title: "Level 3 Future",
    mode: "Portfolio",
    detail: "Future mode for weekly portfolio selection. It is visible here for context, not selectable yet.",
    available: false,
  },
  {
    value: 4,
    title: "Level 4 Future",
    mode: "Full autopilot",
    detail: "Future mode for broader autonomous optimization with audit, budget, and emergency controls.",
    available: false,
  },
];

function automationGateTitle(key: string, fallback: string) {
  const titles: Record<string, string> = {
    search_read: "Search data",
    publisher_write: "Publisher access",
    notification_write: "Notifications",
    autopilot_policy_confirmed: "Automation Policy",
    automation_pause_clear: "Automation status",
    monthly_budget_configured: "Autopilot budget",
    safe_mode_clear: "Safe mode",
    kill_switch_clear: "Emergency stop",
    rollback_or_recovery_ready: "Recovery plan",
  };
  return titles[key] ?? fallback;
}

function automationGateSummary(key: string, blocked: boolean) {
  if (key === "automation_pause_clear") return blocked ? "Automation is paused" : "Automation is active";
  if (key === "kill_switch_clear") return blocked ? "Emergency stop is on" : "Emergency stop is off";
  if (key === "safe_mode_clear") return blocked ? "Safe mode is active" : "Safe mode is clear";
  if (key === "autopilot_policy_confirmed") return blocked ? "Review policy before guarded execution" : "Policy confirmed";
  if (key === "monthly_budget_configured") return blocked ? "Set an Autopilot budget limit" : "Budget limit is set";
  if (key === "notification_write") return blocked ? "Add a tested notification channel" : "Notification channel ready";
  if (key === "rollback_or_recovery_ready") return blocked ? "Confirm recovery before writes" : "Recovery plan confirmed";
  if (key === "publisher_write") return blocked ? "Connect a publisher target" : "Publisher access ready";
  if (key === "search_read") return blocked ? "Connect first-party search data" : "Search data ready";
  return blocked ? "Needs attention" : "Ready";
}

function automationGateImpact(key: string) {
  const copy: Record<string, string> = {
    search_read: "CiteLoop needs first-party search data before it can choose low-risk SEO actions from real query and page signals.",
    publisher_write: "Guarded automation needs a scoped place to create or update content. Without it, CiteLoop can only prepare drafts.",
    notification_write: "Failures, approval requests, safe mode alerts, and delivery problems should reach an operator before Level 2 runs.",
    autopilot_policy_confirmed: "Automation Policy defines how much autonomy CiteLoop has, what budget it can use, and when it must stop.",
    automation_pause_clear: "Automation status is the everyday on/off control. Pausing it does not change the autonomy level you selected.",
    monthly_budget_configured: "The Autopilot budget is the spending boundary for guarded execution. It is separate from the project cadence budget.",
    safe_mode_clear: "Safe mode pauses automation after degraded or risky conditions. It must be clear before guarded execution resumes.",
    kill_switch_clear: "Emergency stop immediately pauses automation. Turn it off only when you want CiteLoop to resume eligible work.",
    rollback_or_recovery_ready: "Every guarded action needs either publisher rollback support or a confirmed manual recovery plan.",
  };
  return copy[key] ?? "This check protects guarded execution before CiteLoop changes anything automatically.";
}

export function SettingsClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [config, setConfig] = useState<ProjectConfig>(defaultProjectConfig());
  const [publisherConnections, setPublisherConnections] = useState<PublisherConnection[]>([]);
  const [publisherDraft, setPublisherDraft] = useState<GitHubNextJSPublisherInput>(defaultPublisherDraft);
  const [publisherCredentialDraft, setPublisherCredentialDraft] = useState("");
  const [devToUsername, setDevToUsername] = useState("");
  const [devToCredentialDraft, setDevToCredentialDraft] = useState("");
  const [redditContexts, setRedditContexts] = useState<PlatformTargetContext[]>([]);
  const [redditRulesDraft, setRedditRulesDraft] = useState({
    target_key: "", rules_url: "", rules_text: "", allowed_post_types: "text, link",
    required_flair: "", link_policy: "", self_promotion_policy: "", disclosure_requirements: "", verified: false,
  });
  const [githubIntegration, setGithubIntegration] = useState<GithubIntegrationStatus | null>(null);
  const [githubPRReadinessState, setGithubPRReadiness] = useState<GithubPRReadiness | null>(null);
  const [githubReadinessBusyState, setGithubReadinessBusy] = useState<"checking" | null>(null);
  const [githubReadinessErrorState, setGithubReadinessError] = useState<string | null>(null);
  const [githubReadinessStateProjectId, setGithubReadinessStateProjectId] = useState(projectId);
  const githubReadinessMountedRef = useRef(false);
  const githubReadinessPublisherEntryTrackerRef = useRef<ReturnType<typeof createGithubPRReadinessPublisherEntryTracker> | null>(null);
  if (!githubReadinessPublisherEntryTrackerRef.current) {
    githubReadinessPublisherEntryTrackerRef.current = createGithubPRReadinessPublisherEntryTracker();
  }
  const githubReadinessRequestOrderRef = useRef<ReturnType<typeof createGithubPRReadinessRequestOrder> | null>(null);
  if (!githubReadinessRequestOrderRef.current) {
    githubReadinessRequestOrderRef.current = createGithubPRReadinessRequestOrder(projectId);
  }
  const githubReadinessRequestScope = githubReadinessRequestOrderRef.current.forProject(projectId);
  const githubPRReadiness = githubReadinessStateProjectId === projectId ? githubPRReadinessState : null;
  const githubReadinessBusy = githubReadinessStateProjectId === projectId ? githubReadinessBusyState : null;
  const githubReadinessError = githubReadinessStateProjectId === projectId ? githubReadinessErrorState : null;
  const [showManualPublisherCredential, setShowManualPublisherCredential] = useState(false);
  const [gscConnection, setGSCConnection] = useState<GSCConnection | null>(null);
  const [seoProperty, setSEOProperty] = useState<SEOProperty | null>(null);
  const [seoIntegrations, setSEOIntegrations] = useState<SEOIntegration[]>([]);
  const [ga4PropertyID, setGA4PropertyID] = useState("");
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [events, setEvents] = useState<NotificationEvent[]>([]);
  const [subscriptions, setSubscriptions] = useState<NotificationSubscription[]>([]);
  const [deliveries, setDeliveries] = useState<NotificationDelivery[]>([]);
  const [deliveryStatus, setDeliveryStatus] = useState("");
  const [channelDraft, setChannelDraft] = useState<{ kind: NotificationChannelKind; label: string; destination: string }>({
    kind: "slack_webhook",
    label: "Ops",
    destination: "",
  });
  const [activeEventsChannel, setActiveEventsChannel] = useState<NotificationChannel | null>(null);
  const [eventSelection, setEventSelection] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState(false);
  const [aiAuthorityBusy, setAIAuthorityBusy] = useState<"doctor" | "growth" | null>(null);
  const [gscBusy, setGSCBusy] = useState<string | null>(null);
  const [ga4Busy, setGA4Busy] = useState(false);
  const [notificationBusy, setNotificationBusy] = useState<string | null>(null);
  const [policy, setPolicy] = useState<SEOPolicy | null>(null);
  const [policyDraft, setPolicyDraft] = useState<PolicyDraft>(defaultPolicyDraft);
  const [readiness, setReadiness] = useState<AutopilotReadiness | null>(null);
  const [safeModeEvents, setSafeModeEvents] = useState<SafeModeEvent[]>([]);
  const [selectedAutomationCheck, setSelectedAutomationCheck] = useState<string | null>(null);
  const [reviewedRecoveryPlan, setReviewedRecoveryPlan] = useState(false);
  const { notify } = useToast();
  const setMessage = (next: Message) => {
    if (next) notify(next);
  };
  const [activeSettingsTab, setActiveSettingsTab] = useState<SettingsTabId>("project");

  useEffect(() => {
    githubReadinessMountedRef.current = true;
    return () => {
      githubReadinessMountedRef.current = false;
    };
  }, []);

  const isCurrentGithubReadinessRequest = useCallback((scope: GithubPRReadinessRequestScope) => {
    return githubReadinessMountedRef.current && Boolean(githubReadinessRequestOrderRef.current?.isCurrent(scope));
  }, []);

  const invalidateGithubReadinessRequests = useCallback((scope: GithubPRReadinessRequestScope) => {
    const nextScope = githubReadinessRequestOrderRef.current!.invalidate(scope);
    if (nextScope) setGithubReadinessBusy(null);
    return nextScope;
  }, []);

  useEffect(() => {
    setGithubReadinessStateProjectId(projectId);
    setGithubPRReadiness(null);
    setGithubReadinessBusy(null);
    setGithubReadinessError(null);
  }, [projectId]);

  useEffect(() => {
    function syncTabFromHash() {
      setActiveSettingsTab(settingsTabFromHash(window.location.hash));
      setReviewedRecoveryPlan(false);
      setSelectedAutomationCheck(automationCheckIdFromHash(window.location.hash));
    }

    syncTabFromHash();
    window.addEventListener("hashchange", syncTabFromHash);
    return () => window.removeEventListener("hashchange", syncTabFromHash);
  }, []);

  useEffect(() => {
    if (!policy) return;
    setPolicyDraft({
      autopilot_level: Math.max(0, Math.min(2, policy.autopilot_level ?? 0)),
      automation_paused: Boolean(policy.automation_paused),
      monthly_budget_limit: policy.monthly_budget_limit != null ? normalizeNumeric(policy.monthly_budget_limit) ?? 0 : 0,
      kill_switch_enabled: Boolean(policy.kill_switch_enabled),
      safe_mode_enabled: Boolean(policy.safe_mode_enabled),
    });
  }, [policy]);

  function activateSettingsTab(tabId: SettingsTabId) {
    setActiveSettingsTab(tabId);
    closeAutomationCheck();
    window.history.replaceState(null, "", `#${tabId}`);
  }

  function openSettingsAnchor(target: string) {
    const nextHash = target.includes("#") ? `#${target.split("#").pop()}` : target.startsWith("#") ? target : `#${target}`;
    setActiveSettingsTab(settingsTabFromHash(nextHash));
    setReviewedRecoveryPlan(false);
    setSelectedAutomationCheck(automationCheckIdFromHash(nextHash));
    window.history.replaceState(null, "", nextHash);
    window.requestAnimationFrame(() => {
      document.getElementById(nextHash.replace(/^#/, ""))?.scrollIntoView({ block: "start", behavior: "smooth" });
    });
  }

  function openAutomationCheck(checkId: string) {
    setReviewedRecoveryPlan(false);
    setSelectedAutomationCheck(checkId);
  }

  function closeAutomationCheck() {
    setReviewedRecoveryPlan(false);
    setSelectedAutomationCheck(null);
  }

  const refresh = useCallback(async () => {
    try {
      const project = await api.getProject(projectId);
      setConfig(project.config);
    } catch (e: any) {
      setMessage({ title: "Settings unavailable", detail: e.message, tone: "amber" });
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const refreshSEOSettings = useCallback(async () => {
    try {
      const settings = await api.getSEOSettings(projectId);
      const property = settings.property ?? null;
      setSEOProperty(property);
      setSEOIntegrations(settings.integrations);
      setGA4PropertyID(property?.ga4_property_id ?? "");
    } catch (e: any) {
      if (isProjectScopedMissing(e.message)) {
        setSEOProperty(null);
        setSEOIntegrations([]);
        setGA4PropertyID("");
        return;
      }
      setMessage({ title: "Google Analytics settings unavailable", detail: friendlyError(e.message), tone: "amber" });
    }
  }, [api, projectId]);

  useEffect(() => {
    refreshSEOSettings();
  }, [refreshSEOSettings]);

  const refreshPublisherConnections = useCallback(async () => {
    try {
      const nextConnections = await api.listPublisherConnections(projectId);
      setPublisherConnections(nextConnections);
      const github = nextConnections.find((connection) => connection.kind === "github_nextjs");
      if (github) {
        setPublisherDraft({
          label: github.label || "GitHub/Next.js",
          repo: github.config?.repo ?? "",
          branch: github.config?.branch ?? "citeloop-content",
          content_dir: github.config?.content_dir ?? "content/citeloop/blog",
          base_url: github.config?.base_url ?? "",
          publish_mode: github.config?.publish_mode ?? "publish",
          credential_ref: "",
        });
      }
      const devTo = nextConnections.find((connection) => connection.kind === "dev_to");
      if (devTo) {
        setDevToUsername(devTo.config?.username ?? "");
      }
    } catch (e: any) {
      if (isProjectScopedMissing(e.message)) {
        setPublisherConnections([]);
        return;
      }
      setMessage({ title: "Publisher connections unavailable", detail: friendlyError(e.message), tone: "amber" });
    }
  }, [api, projectId]);

  useEffect(() => {
    refreshPublisherConnections();
  }, [refreshPublisherConnections]);

  const refreshRedditContexts = useCallback(async () => {
    try {
      setRedditContexts(await api.listPlatformTargetContexts(projectId, "reddit"));
    } catch (e: any) {
      setMessage({ title: "Reddit rules unavailable", detail: friendlyError(e.message), tone: "amber" });
    }
  }, [api, projectId]);

  useEffect(() => {
    refreshRedditContexts();
  }, [refreshRedditContexts]);

  async function confirmRedditRules() {
    setNotificationBusy("save-reddit-rules");
    try {
      await api.confirmPlatformTargetContext(projectId, {
        platform: "reddit",
        target_key: redditRulesDraft.target_key,
        rules_url: redditRulesDraft.rules_url,
        source_url: redditRulesDraft.rules_url,
        rules_text: redditRulesDraft.rules_text,
        allowed_post_types: redditRulesDraft.allowed_post_types.split(",").map((item) => item.trim()).filter(Boolean),
        required_flair: redditRulesDraft.required_flair,
        link_policy: redditRulesDraft.link_policy,
        self_promotion_policy: redditRulesDraft.self_promotion_policy,
        disclosure_requirements: redditRulesDraft.disclosure_requirements,
        verified: redditRulesDraft.verified,
      });
      await refreshRedditContexts();
      setRedditRulesDraft((current) => ({ ...current, verified: false }));
      setMessage({ title: "Subreddit rules confirmed", detail: "The immutable revision can be used for 30 days.", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not confirm Reddit rules", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function reconfirmRedditRules(contextID: string) {
    setNotificationBusy(`reconfirm-reddit-${contextID}`);
    try {
      await api.reconfirmPlatformTargetContext(projectId, contextID);
      await refreshRedditContexts();
      setMessage({ title: "Subreddit rules reconfirmed", detail: "A new immutable 30-day revision was created.", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not reconfirm Reddit rules", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  const refreshGithubIntegration = useCallback(async () => {
    try {
      const integration = await api.getGithubIntegration(projectId);
      setGithubIntegration(integration);
    } catch {
      setGithubIntegration(null);
    }
  }, [api, projectId]);

  useEffect(() => {
    refreshGithubIntegration();
  }, [refreshGithubIntegration]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const url = new URL(window.location.href);
    if (url.searchParams.get("github") !== "connected") return;
    setActiveSettingsTab("publisher");
    setMessage({
      title: "GitHub connected",
      detail: "Choose or confirm the publisher target before enabling the connection.",
      tone: "green",
    });
    refreshPublisherConnections();
    refreshGithubIntegration();
    url.searchParams.delete("github");
    window.history.replaceState({}, "", url.pathname + url.search + url.hash);
  }, [refreshGithubIntegration, refreshPublisherConnections]);

  const runGithubPRReadinessCheck = useCallback(async (): Promise<GithubPRReadiness | null> => {
    const requestScope = githubReadinessRequestScope;
    if (!isCurrentGithubReadinessRequest(requestScope)) return null;
    setGithubReadinessError(null);
    try {
      const nextReadiness = await api.checkGithubPRReadiness(requestScope.projectId);
      if (isCurrentGithubReadinessRequest(requestScope)) {
        setGithubPRReadiness(nextReadiness);
      }
      return nextReadiness;
    } catch {
      if (isCurrentGithubReadinessRequest(requestScope)) {
        setGithubPRReadiness((current) => ({
          status: "error",
          checked_at: null,
          detail: githubPRReadinessCheckError,
          ...(current?.repo ? { repo: current.repo } : {}),
          ...(current?.branch ? { branch: current.branch } : {}),
        }));
        setGithubReadinessError(githubPRReadinessCheckError);
      }
      return null;
    }
  }, [api, githubReadinessRequestScope, isCurrentGithubReadinessRequest]);

  const refreshStoredGithubPRReadiness = useCallback(async (
    requestScope: GithubPRReadinessRequestScope = githubReadinessRequestScope,
  ): Promise<GithubPRReadiness | null> => {
    if (isCurrentGithubReadinessRequest(requestScope)) setGithubReadinessError(null);
    try {
      const nextReadiness = await api.getGithubPRReadiness(requestScope.projectId);
      if (isCurrentGithubReadinessRequest(requestScope)) setGithubPRReadiness(nextReadiness);
      return nextReadiness;
    } catch {
      if (isCurrentGithubReadinessRequest(requestScope)) {
        setGithubPRReadiness((current) => ({
          status: "error",
          checked_at: null,
          detail: githubPRReadinessCheckError,
          ...(current?.repo ? { repo: current.repo } : {}),
          ...(current?.branch ? { branch: current.branch } : {}),
        }));
        setGithubReadinessError(githubPRReadinessCheckError);
      }
      return null;
    }
  }, [api, githubReadinessRequestScope, isCurrentGithubReadinessRequest]);

  const githubReadinessRefreshCoordinator = useMemo(
    () => createGithubPRReadinessRefreshCoordinator(runGithubPRReadinessCheck, (draining) => {
      if (isCurrentGithubReadinessRequest(githubReadinessRequestScope)) {
        setGithubReadinessBusy(draining ? "checking" : null);
      }
    }),
    [githubReadinessRequestScope, isCurrentGithubReadinessRequest, runGithubPRReadinessCheck],
  );

  const refreshGithubPRReadiness = useCallback(
    (mode: GithubPRReadinessRefreshMode = "normal") => githubReadinessRefreshCoordinator.request(mode),
    [githubReadinessRefreshCoordinator],
  );

  useEffect(() => {
    const enteredPublisher = githubReadinessPublisherEntryTrackerRef.current!.shouldRefresh(projectId, activeSettingsTab);
    if (!enteredPublisher) return;
    // The GitHub callback saves the selected repository and branch before it
    // returns to #publisher, so this tab-owned effect performs the live check.
    void refreshGithubPRReadiness();
  }, [activeSettingsTab, projectId, refreshGithubPRReadiness]);

  const refreshGSCConnection = useCallback(async () => {
    try {
      const connection = await api.getGSCConnection(projectId);
      setGSCConnection(connection);
    } catch (e: any) {
      if (isProjectScopedMissing(e.message)) {
        setGSCConnection(null);
        return;
      }
      setMessage({ title: "Search Console connection unavailable", detail: friendlyError(e.message), tone: "amber" });
    }
  }, [api, projectId]);

  useEffect(() => {
    refreshGSCConnection();
  }, [refreshGSCConnection]);

  const refreshGoogleConnections = useCallback(async () => {
    setGSCBusy("refresh");
    try {
      await Promise.all([refreshGSCConnection(), refreshSEOSettings()]);
    } finally {
      setGSCBusy(null);
    }
  }, [refreshGSCConnection, refreshSEOSettings]);

  const refreshNotifications = useCallback(async () => {
    try {
      const [nextChannels, nextEvents, nextSubscriptions, nextDeliveries] = await Promise.all([
        api.listNotificationChannels(projectId),
        api.listNotificationEvents(projectId),
        api.listNotificationSubscriptions(projectId),
        api.listNotificationDeliveries(projectId, { status: deliveryStatus, limit: 25 }),
      ]);
      setChannels(nextChannels);
      setEvents(nextEvents);
      setSubscriptions(nextSubscriptions);
      setDeliveries(nextDeliveries);
    } catch (e: any) {
      if (isProjectScopedMissing(e.message)) {
        setChannels([]);
        setEvents([]);
        setSubscriptions([]);
        setDeliveries([]);
        return;
      }
      setMessage({ title: "Notifications unavailable", detail: friendlyError(e.message), tone: "amber" });
    }
  }, [api, deliveryStatus, projectId]);

  useEffect(() => {
    refreshNotifications();
  }, [refreshNotifications]);

  const refreshAutomation = useCallback(async () => {
    try {
      const [policyData, readinessData, safeModeRows] = await Promise.all([
        api.getSEOPolicy(projectId),
        api.getAutopilotReadiness(projectId),
        api.listSafeModeEvents(projectId),
      ]);
      setPolicy(policyData);
      setReadiness(readinessData);
      setSafeModeEvents(safeModeRows);
    } catch (e: any) {
      setMessage({ title: "Automation settings unavailable", detail: e.message, tone: "amber" });
    }
  }, [api, projectId]);

  useEffect(() => {
    refreshAutomation();
  }, [refreshAutomation]);

  async function saveAutomationPolicy(next: SEOPolicyUpdateInput): Promise<boolean> {
    if (!policy) return false;
    setBusy(true);
    try {
      const saved = await api.updateSEOPolicy(projectId, { ...policy, ...next });
      setPolicy(saved);
      await refreshAutomation();
      notify({ title: "Automation policy saved", tone: "green" });
      return true;
    } catch (e: any) {
      notify({ title: "Could not save automation policy", detail: friendlyError(e.message), tone: "red" });
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function acknowledgeRecoveryPlan() {
    if (!reviewedRecoveryPlan) {
      setReviewedRecoveryPlan(true);
      return;
    }
    const saved = await saveAutomationPolicy({
      recovery_plan_acknowledged: true,
      recovery_plan_acknowledged_by: "human",
    });
    if (saved) closeAutomationCheck();
  }

  async function savePolicyDraft() {
    const saved = await saveAutomationPolicy({
      autopilot_level: Math.max(0, Math.min(2, policyDraft.autopilot_level)),
      automation_paused: policyDraft.automation_paused,
      monthly_budget_limit: Math.max(0, policyDraft.monthly_budget_limit),
      kill_switch_enabled: policyDraft.kill_switch_enabled,
      safe_mode_enabled: policyDraft.safe_mode_enabled,
    });
    if (saved) closeAutomationCheck();
  }

  function openPolicyCheck(nextDraft: Partial<PolicyDraft> = {}) {
    setPolicyDraft((current) => ({ ...current, ...nextDraft }));
    setReviewedRecoveryPlan(false);
    setSelectedAutomationCheck("autopilot_policy_confirmed");
  }

  function reviewRecoveryPlan() {
    setReviewedRecoveryPlan(true);
    window.requestAnimationFrame(() => {
      document.getElementById("recovery-plan-review")?.scrollIntoView({ block: "start", behavior: "smooth" });
    });
  }

  function returnToRecoveryCheck() {
    window.requestAnimationFrame(() => {
      document.getElementById("recovery-plan-return")?.scrollIntoView({ block: "nearest", behavior: "smooth" });
    });
  }

  async function exitOpenSafeModeEvents() {
    const openEvents = safeModeEvents.filter((event) => !event.exited_at);
    if (openEvents.length === 0) {
      openPolicyCheck({ safe_mode_enabled: false });
      return;
    }
    setBusy(true);
    try {
      await Promise.all(
        openEvents.map((event) =>
          api.exitSafeMode(projectId, event.id, {
            exited_by: "human",
            exit_reason: "confirmed from Automation readiness",
          }),
        ),
      );
      await refreshAutomation();
      notify({
        title: policy?.safe_mode_enabled ? "Safe mode events exited" : "Safe mode cleared",
        detail: policy?.safe_mode_enabled
          ? "Open events were exited. Save the policy to turn off the policy-level safe mode switch."
          : `${openEvents.length} open safe mode event${openEvents.length === 1 ? "" : "s"} exited.`,
        tone: "green",
      });
      if (policy?.safe_mode_enabled) {
        openPolicyCheck({ safe_mode_enabled: false });
      } else {
        closeAutomationCheck();
      }
    } catch (e: any) {
      notify({ title: "Could not clear safe mode", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setBusy(false);
    }
  }

  async function handleAutomationCheckAction(checkId: string, href?: string) {
    if (checkId === "kill_switch_clear") {
      openPolicyCheck({ kill_switch_enabled: false });
      return;
    }
    if (checkId === "safe_mode_clear") {
      await exitOpenSafeModeEvents();
      return;
    }
    if (checkId === "rollback_or_recovery_ready") {
      if (!reviewedRecoveryPlan) {
        reviewRecoveryPlan();
        return;
      }
      await acknowledgeRecoveryPlan();
      return;
    }
    if (href) {
      openSettingsAnchor(href);
    }
  }

  function update(next: Partial<ProjectConfig>) {
    setConfig((current) => ({ ...current, ...next }));
  }

  async function save() {
    if ((config.monthly_budget_usd ?? 0) <= 0) {
      const ok = window.confirm("Set the monthly budget to $0? This pauses all automated generation and SEO work until you raise it.");
      if (!ok) return;
    }
    setBusy(true);
    setMessage(null);
    try {
      const fullPayload = {
        ...defaultProjectConfig(),
        ...config,
        crawl: { ...defaultProjectConfig().crawl, ...config.crawl },
        channel_mix: { ...defaultProjectConfig().channel_mix, ...config.channel_mix },
      };
      await api.updateConfig(projectId, fullPayload);
      setConfig(fullPayload);
      setMessage({ title: "Settings saved", detail: "Cadence, budget, and crawl settings are updated.", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Settings save failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setBusy(false);
    }
  }

  async function saveDoctorAIAuthority() {
    setAIAuthorityBusy("doctor");
    setMessage(null);
    try {
      const saved = await api.updateConfig(projectId, {
        doctor_ai_enabled: config.doctor_ai_enabled,
        doctor_ai_run_policy: config.doctor_ai_run_policy,
      });
      setConfig(saved.config);
      setMessage({
        title: "Doctor AI assistance saved",
        detail: saved.config.doctor_ai_enabled ? "Doctor can use AI under the selected run policy." : "Doctor AI provider calls are revoked.",
        tone: "green",
      });
    } catch (e: any) {
      setMessage({ title: "Doctor AI settings failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setAIAuthorityBusy(null);
    }
  }

  async function saveGrowthAIAuthority() {
    setAIAuthorityBusy("growth");
    setMessage(null);
    try {
      const saved = await api.updateConfig(projectId, {
        growth_ai_enabled: config.growth_ai_enabled,
        growth_ai_run_policy: config.growth_ai_run_policy,
      });
      setConfig(saved.config);
      setMessage({
        title: "Opportunities AI assistance saved",
        detail: saved.config.growth_ai_enabled ? "Opportunities can use AI under the selected run policy." : "Opportunities AI provider calls are revoked.",
        tone: "green",
      });
    } catch (e: any) {
      setMessage({ title: "Opportunities AI settings failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setAIAuthorityBusy(null);
    }
  }

  async function createChannel() {
    const destination = channelDraft.destination.trim();
    const isEmail = channelDraft.kind === "email";
    if (!destination) {
      setMessage({ title: isEmail ? "Email address required" : "Webhook URL required", tone: "amber" });
      return;
    }
    setNotificationBusy("create-channel");
    setMessage(null);
    try {
      await api.createNotificationChannel(projectId, {
        kind: channelDraft.kind,
        label: channelDraft.label.trim() || channelKinds.find((item) => item.value === channelDraft.kind)?.label || "Channel",
        ...(isEmail ? { destination } : { url: destination }),
      });
      setChannelDraft((current) => ({ ...current, destination: "" }));
      setMessage({ title: "Notification channel saved", tone: "green" });
      await refreshNotifications();
    } catch (e: any) {
      setMessage({ title: "Channel save failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function deleteChannel(channel: NotificationChannel) {
    const usedBy = channel.project_subscription_count ?? 0;
    const detail = usedBy > 0 ? ` It is used by ${usedBy} project subscription${usedBy === 1 ? "" : "s"}.` : "";
    if (!window.confirm(`Delete this account notification channel?${detail} Subscriptions using it will stop delivering.`)) return;
    const channelID = channel.id;
    setNotificationBusy(`delete-${channelID}`);
    setMessage(null);
    try {
      await api.deleteNotificationChannel(projectId, channelID);
      setMessage({ title: "Notification channel deleted", tone: "green" });
      await refreshNotifications();
    } catch (e: any) {
      setMessage({ title: "Channel delete failed", detail: e.message, tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function testChannel(channel: NotificationChannel) {
    const channelID = channel.id;
    setNotificationBusy(`test-${channelID}`);
    setMessage(null);
    try {
      await api.testNotificationChannel(projectId, channelID);
      setMessage({
        title: "Test accepted",
        detail: channel.kind === "email" ? "Email channel can now be subscribed to project events." : "Channel can now be subscribed to project events.",
        tone: "green",
      });
      await refreshNotifications();
    } catch (e: any) {
      setMessage({ title: "Test notification failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function savePublisherConnection() {
    setNotificationBusy("save-publisher");
    setMessage(null);
    try {
      let saved = await api.upsertGitHubNextJSPublisherConnection(projectId, {
        ...publisherDraft,
        repo: publisherDraft.repo.trim(),
        branch: publisherDraft.branch?.trim() || "citeloop-content",
        content_dir: publisherDraft.content_dir?.trim() || "content/citeloop/blog",
        base_url: publisherDraft.base_url.trim(),
        publish_mode: publisherDraft.publish_mode?.trim() || "publish",
        credential_ref: undefined,
      });
      if (publisherCredentialDraft.trim()) {
        saved = await api.upsertPublisherCredential(projectId, saved.id, {
          kind: "github_token",
          value: publisherCredentialDraft.trim(),
        });
        setPublisherCredentialDraft("");
      }
      setPublisherConnections((current) => {
        const rest = current.filter((connection) => connection.id !== saved.id && connection.kind !== saved.kind);
        return [saved, ...rest];
      });
      await refreshGithubPRReadiness("after-mutation");
      setMessage({ title: "Publisher connection saved", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Publisher save failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function savePublisherCredential() {
    if (!githubPublisher) {
      setMessage({ title: "Save publisher first", detail: "Create the GitHub/Next.js connection before saving a token.", tone: "amber" });
      return;
    }
    const value = publisherCredentialDraft.trim();
    if (!value) {
      setMessage({ title: "GitHub token required", tone: "amber" });
      return;
    }
    setNotificationBusy(`save-publisher-credential-${githubPublisher.id}`);
    setMessage(null);
    try {
      const saved = await api.upsertPublisherCredential(projectId, githubPublisher.id, {
        kind: "github_token",
        value,
      });
      setPublisherCredentialDraft("");
      setPublisherConnections((current) => current.map((connection) => (connection.id === saved.id ? saved : connection)));
      await refreshGithubPRReadiness("after-mutation");
      setMessage({ title: "Publisher credential saved", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Credential save failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function revokePublisherCredential() {
    if (!githubPublisher) return;
    setNotificationBusy(`revoke-publisher-credential-${githubPublisher.id}`);
    setMessage(null);
    try {
      const saved = await api.revokePublisherCredential(projectId, githubPublisher.id);
      setPublisherConnections((current) => current.map((connection) => (connection.id === saved.id ? saved : connection)));
      await refreshGithubPRReadiness("after-mutation");
      setMessage({ title: "Publisher credential revoked", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Credential revoke failed", detail: e.message, tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function saveDevToConnection() {
    setNotificationBusy("save-devto");
    setMessage(null);
    try {
      const saved = await api.upsertDevToPublisherConnection(projectId, {
        label: "Dev.to",
        username: devToUsername.trim(),
      });
      setPublisherConnections((current) => {
        const rest = current.filter((connection) => connection.id !== saved.id && connection.kind !== saved.kind);
        return [saved, ...rest];
      });
      setMessage({ title: "Dev.to connection saved", detail: "Save an API key, then test the connection.", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Dev.to save failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function saveDevToCredential() {
    if (!devToPublisher) {
      setMessage({ title: "Save Dev.to first", detail: "Create the Dev.to connection before saving an API key.", tone: "amber" });
      return;
    }
    const value = devToCredentialDraft.trim();
    if (!value) {
      setMessage({ title: "Dev.to API key required", tone: "amber" });
      return;
    }
    setNotificationBusy(`save-devto-credential-${devToPublisher.id}`);
    setMessage(null);
    try {
      const saved = await api.upsertPublisherCredential(projectId, devToPublisher.id, {
        kind: "dev_to_api_key",
        value,
      });
      setDevToCredentialDraft("");
      setPublisherConnections((current) => current.map((connection) => (connection.id === saved.id ? saved : connection)));
      setMessage({ title: "Dev.to API key saved", detail: "Test the connection before enabling publishing.", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Dev.to key save failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function testDevToConnection() {
    if (!devToPublisher) {
      setMessage({ title: "Save Dev.to first", detail: "Create the Dev.to connection before testing an API key.", tone: "amber" });
      return;
    }
    const value = devToCredentialDraft.trim();
    if (!devToPublisher.credential_configured && !value) {
      setMessage({ title: "Dev.to API key required", detail: "Paste a DEV API key before testing this connection.", tone: "amber" });
      return;
    }
    setNotificationBusy(`test-publisher-${devToPublisher.id}`);
    setMessage(null);
    try {
      if (value) {
        const savedCredential = await api.upsertPublisherCredential(projectId, devToPublisher.id, {
          kind: "dev_to_api_key",
          value,
        });
        setDevToCredentialDraft("");
        setPublisherConnections((current) => current.map((connection) => (connection.id === savedCredential.id ? savedCredential : connection)));
      }
      const tested = await api.testPublisherConnection(projectId, devToPublisher.id);
      setPublisherConnections((current) => current.map((connection) => (connection.id === tested.id ? tested : connection)));
      setMessage({
        title: "Dev.to connection verified",
        detail: value ? "API key saved and verified." : undefined,
        tone: "green",
      });
    } catch (e: any) {
      setMessage({ title: "Dev.to test failed", detail: friendlyApiError(e), tone: "red" });
      await refreshPublisherConnections();
    } finally {
      setNotificationBusy(null);
    }
  }

  async function revokeDevToCredential() {
    if (!devToPublisher) return;
    setNotificationBusy(`revoke-devto-credential-${devToPublisher.id}`);
    setMessage(null);
    try {
      const saved = await api.revokePublisherCredential(projectId, devToPublisher.id);
      setPublisherConnections((current) => current.map((connection) => (connection.id === saved.id ? saved : connection)));
      setMessage({ title: "Dev.to API key revoked", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Dev.to key revoke failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  function connectGithub() {
    if (githubIntegration?.install_url) {
      rememberGithubConnectProject(projectId, `/projects/${projectId}/settings?github=connected#publisher`);
      window.location.href = githubIntegration.install_url;
    }
  }

  function reuseGithub() {
    const reuse = githubIntegration?.reusable_installation_id;
    if (reuse) {
      rememberGithubConnectProject(projectId, `/projects/${projectId}/settings?github=connected#publisher`);
      window.location.href = `/integrations/github/callback?installation_id=${encodeURIComponent(reuse)}&state=${encodeURIComponent(projectId)}`;
    }
  }

  async function startSearchConsoleOAuth(source: "search-console" | "analytics" = "search-console") {
    const analytics = source === "analytics";
    setGSCBusy(analytics ? "analytics-permissions" : "connect");
    setMessage(null);
    try {
      const result = await api.startGSCOAuth(projectId);
      window.location.assign(result.authorization_url);
    } catch (e: any) {
      setMessage({
        title: analytics ? "Analytics permission update failed" : "Search Console connect failed",
        detail: friendlyError(e.message),
        tone: "red",
      });
      setGSCBusy(null);
    }
  }

  async function selectGSCProperty(siteURL: string) {
    setGSCBusy(`select-${siteURL}`);
    setMessage(null);
    try {
      const connection = await api.selectGSCProperty(projectId, { site_url: siteURL });
      setGSCConnection(connection);
      setMessage({ title: "Search Console property selected", detail: siteURL, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Property selection failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setGSCBusy(null);
    }
  }

  async function revokeGSCConnection() {
    if (!window.confirm("Disconnect Search Console? CiteLoop will keep working, but analysis will lose first-party search data.")) return;
    setGSCBusy("revoke");
    setMessage(null);
    try {
      const connection = await api.revokeGSCConnection(projectId);
      setGSCConnection(connection);
      setMessage({ title: "Search Console disconnected", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Search Console disconnect failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setGSCBusy(null);
    }
  }

  async function saveGA4Connection() {
    const propertyID = ga4PropertyID.trim();
    if (!propertyID) {
      setMessage({ title: "GA4 Property ID required", detail: "Paste the numeric Property ID from Google Analytics property settings.", tone: "amber" });
      return;
    }
    const siteURL = (seoProperty?.site_url || config.site_url || "").trim();
    if (!siteURL) {
      setMessage({ title: "Site URL required", detail: "Save the project domain before connecting Google Analytics.", tone: "amber" });
      return;
    }
    setGA4Busy(true);
    setMessage(null);
    try {
      await api.updateSEOSettings(projectId, {
        site_url: siteURL,
        gsc_site_url: seoProperty?.gsc_site_url ?? gscConnection?.selected_property ?? "",
        ga4_property_id: propertyID,
        url_normalization_config: seoProperty?.url_normalization_config ?? {},
        default_country: seoProperty?.default_country ?? "",
        default_language: seoProperty?.default_language ?? "",
      });
      await refreshSEOSettings();
      setMessage({ title: "Google Analytics property saved", detail: "Run SEO sync after GA4 has collected data for this property.", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Google Analytics save failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setGA4Busy(false);
    }
  }

  async function testPublisherConnection(connectionID: string) {
    setNotificationBusy(`test-publisher-${connectionID}`);
    setMessage(null);
    try {
      const tested = await api.testPublisherConnection(projectId, connectionID);
      setPublisherConnections((current) => current.map((connection) => (connection.id === tested.id ? tested : connection)));
      await refreshGithubPRReadiness("after-mutation");
      setMessage({ title: "Publisher connection verified", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Publisher test failed", detail: e.message, tone: "red" });
      await refreshPublisherConnections();
    } finally {
      setNotificationBusy(null);
    }
  }

  async function setPublisherConnectionEnabled(connection: PublisherConnection, enabled: boolean) {
    setNotificationBusy(`toggle-publisher-${connection.id}`);
    setMessage(null);
    try {
      const saved = await api.setPublisherConnectionEnabled(projectId, connection.id, enabled);
      const disabledReadinessScope = connection.kind === "github_nextjs" && !enabled
        ? invalidateGithubReadinessRequests(githubReadinessRequestScope)
        : null;
      setPublisherConnections((current) => current.map((item) => (item.id === saved.id ? saved : item)));
      if (connection.kind === "github_nextjs") {
        if (enabled) {
          await refreshGithubPRReadiness("after-mutation");
        } else if (disabledReadinessScope) {
          await refreshStoredGithubPRReadiness(disabledReadinessScope);
        }
      }
      setMessage({ title: enabled ? "Publisher connection enabled" : "Publisher connection disabled", tone: "green" });
    } catch (e: any) {
      setMessage({ title: enabled ? "Enable failed" : "Disable failed", detail: friendlyError(e.message), tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  async function retryDelivery(deliveryID: string) {
    setNotificationBusy(`retry-${deliveryID}`);
    setMessage(null);
    try {
      await api.retryNotificationDelivery(projectId, deliveryID);
      setMessage({ title: "Delivery queued", tone: "green" });
      await refreshNotifications();
    } catch (e: any) {
      setMessage({ title: "Delivery retry failed", detail: e.message, tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  function subscriptionEnabled(eventType: string, channelID: string) {
    return subscriptions.some((sub) => sub.event_type === eventType && sub.channel_id === channelID && sub.enabled);
  }

  function channelDisplayLabel(channel: NotificationChannel) {
    return channel.label || channelKinds.find((item) => item.value === channel.kind)?.label || "Channel";
  }

  function channelKindLabel(channel: NotificationChannel) {
    return channelKinds.find((item) => item.value === channel.kind)?.label || channel.kind;
  }

  function channelKindTone(channel: NotificationChannel): "green" | "blue" | "amber" {
    if (channel.kind === "slack_webhook") return "green";
    if (channel.kind === "discord_webhook") return "blue";
    return "amber";
  }

  function channelDestination(channel: NotificationChannel) {
    return channel.config?.redacted_to ?? channel.config?.redacted_url ?? "Redacted";
  }

  function channelUsageLabel(channel: NotificationChannel) {
    const count = channel.project_subscription_count ?? 0;
    return `Used by ${count} project${count === 1 ? "" : "s"}`;
  }

  function openChannelEvents(channel: NotificationChannel) {
    if (channel.kind === "email" && !channel.verified_at) {
      setMessage({
        title: "Test email first",
        detail: "Email channels must have a test accepted before project events can subscribe to them.",
        tone: "amber",
      });
      return;
    }
    setActiveEventsChannel(channel);
    setMessage(null);
    setEventSelection(
      events.reduce<Record<string, boolean>>((selection, event) => {
        selection[event.type] = subscriptionEnabled(event.type, channel.id);
        return selection;
      }, {}),
    );
  }

  function closeChannelEvents() {
    setActiveEventsChannel(null);
    setEventSelection({});
  }

  async function saveChannelEvents() {
    if (!activeEventsChannel) return;

    const channel = activeEventsChannel;
    const changedEvents = events.filter((event) => Boolean(eventSelection[event.type]) !== subscriptionEnabled(event.type, channel.id));
    setNotificationBusy(`events-${channel.id}`);
    setMessage(null);
    try {
      if (changedEvents.length > 0) {
        await Promise.all(
          changedEvents.map((event) =>
            api.upsertNotificationSubscription(projectId, {
              event_type: event.type,
              channel_id: channel.id,
              enabled: Boolean(eventSelection[event.type]),
            }),
          ),
        );
      }
      setMessage({ title: "Subscriptions saved", detail: `${channelDisplayLabel(channel)} events are updated.`, tone: "green" });
      closeChannelEvents();
      await refreshNotifications();
    } catch (e: any) {
      setMessage({ title: "Subscription update failed", detail: e.message, tone: "red" });
    } finally {
      setNotificationBusy(null);
    }
  }

  const githubPublisher = publisherConnections.find((connection) => connection.kind === "github_nextjs");
  const devToPublisher = publisherConnections.find((connection) => connection.kind === "dev_to");
  const gscHasAuthorizedProperties = Boolean(gscConnection && gscConnection.configured !== false && gscConnection.properties.length > 0);
  const gscHasSelectedProperty = Boolean(gscConnection?.selected_property);
  const canStartGSCOAuth =
    !gscHasAuthorizedProperties &&
    (!gscConnection || gscConnection.status === "missing" || gscConnection.status === "revoked" || gscConnection.properties.length === 0);
  const canDisconnectGSC = Boolean(gscConnection && gscConnection.status !== "missing" && gscConnection.status !== "revoked");
  const gscCardTitle = gscHasSelectedProperty
    ? "Search Console is connected."
    : gscHasAuthorizedProperties
      ? "Select a Search Console property."
      : "Connect Search Console for first-party search data.";
  const ga4Integration = seoIntegrations.find((integration) => integration.provider === "google_analytics");
  const ga4Status = ga4Integration?.status;
  const ga4NeedsGooglePermissions = ["error", "expired", "reconnect_required", "property_access_required", "revoked"].includes(ga4Status ?? "");
  const savedGA4PropertyID = seoProperty?.ga4_property_id?.trim() ?? "";
  const activeEventsBusy = Boolean(activeEventsChannel && notificationBusy === `events-${activeEventsChannel.id}`);
  const githubAppConnected = Boolean(githubIntegration?.connected);
  const githubAppReusable = Boolean(!githubIntegration?.connected && githubIntegration?.reusable_installation_id);
  const githubAppTitle = githubAppConnected
    ? "Connected via GitHub App"
    : githubAppReusable
      ? "Use your connected GitHub App"
      : "Connect with GitHub";
  const githubAppDetail = githubAppConnected
    ? `${githubIntegration?.repo || "Repository selected"}${githubIntegration?.branch ? ` on ${githubIntegration.branch}` : ""}`
    : githubAppReusable
      ? "This owner already has the CiteLoop GitHub App installed. Reuse that installation or connect a different account."
      : "Install the CiteLoop GitHub App to publish without storing a personal access token.";
  const githubReadinessPresentation = githubPRReadinessPresentation(githubPRReadiness?.status);
  const githubReadinessTarget = githubPRReadiness?.repo
    ? `${githubPRReadiness.repo}${githubPRReadiness.branch ? ` on ${githubPRReadiness.branch}` : ""}`
    : githubPRReadiness?.branch
      ? `Branch ${githubPRReadiness.branch}`
      : "";
  const githubReadinessSurface =
    githubReadinessPresentation.tone === "green"
      ? "border-green-200 bg-green-50/70 text-green-950"
      : githubReadinessPresentation.tone === "red"
        ? "border-red-200 bg-red-50/70 text-red-950"
        : "border-amber-200 bg-amber-50/70 text-amber-950";

  const openSafeModeEvents = safeModeEvents.filter((event) => !event.exited_at);
  const openSafeModeCount = openSafeModeEvents.length;
  const automationPaused = Boolean(policy?.automation_paused);
  const readinessGatesAll = readiness?.gates ?? [];
  const blockedGates = readinessGatesAll
    .filter((gate) => gate.blocking)
    .map((gate) => ({ gate, action: readinessGateActionFor(gate.key, projectId) }))
    .sort((a, b) => (a.action?.rank ?? 999) - (b.action?.rank ?? 999));
  const readyGates = readinessGatesAll.filter((gate) => !gate.blocking);
  const automationReadinessCards = readinessGatesAll
    .map((gate) => {
      const action = readinessGateActionFor(gate.key, projectId);
      return {
        id: gate.key,
        title: automationGateTitle(gate.key, gate.label),
        summary: automationGateSummary(gate.key, gate.blocking),
        impact: automationGateImpact(gate.key),
        reason: gate.reason,
        nextAction: gate.next_action,
        blocked: gate.blocking,
        action,
      };
    })
    .sort((a, b) => (a.action?.rank ?? 999) - (b.action?.rank ?? 999));
  const selectedAutomationCard = selectedAutomationCheck
    ? automationReadinessCards.find((card) => card.id === selectedAutomationCheck) ?? null
    : null;
  const doctorAIStatus = config.doctor_ai_enabled ? "on" : "off";
  const growthAIStatus = config.growth_ai_enabled ? "on" : "off";

  return (
    <div className="space-y-7">
      <SectionHeader title="Settings" eyebrow="Project config" />

      <div className="overflow-x-auto border-b border-slate-200">
        <div role="tablist" aria-label="Settings sections" className="flex min-w-max gap-6">
          {settingsTabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              id={`settings-tab-${tab.id}`}
              role="tab"
              aria-selected={activeSettingsTab === tab.id}
              aria-controls={`settings-panel-${tab.id}`}
              onClick={() => activateSettingsTab(tab.id)}
              className={cx(
                "border-b-2 px-0 pb-3 pt-1 text-sm font-semibold transition-colors",
                activeSettingsTab === tab.id
                  ? "border-[#d93820] text-slate-950"
                  : "border-transparent text-slate-500 hover:text-slate-900",
              )}
            >
              {tab.title}
            </button>
          ))}
        </div>
      </div>

      {activeSettingsTab === "automation" && (
      <section id="settings-panel-automation" role="tabpanel" aria-labelledby="settings-tab-automation" tabIndex={0} className="space-y-7">
        <div id="automation" className="space-y-6">
          <SectionHeader
            title="Automation readiness"
            eyebrow="System setup"
            action={
              <Badge tone={readiness?.ready_for_level_2 ? "green" : "amber"}>
                {readiness?.ready_for_level_2 ? "Ready" : `${blockedGates.length} to set up`}
              </Badge>
            }
          />
          <p className="mb-4 max-w-3xl text-sm text-slate-600">
            When these checks pass, CiteLoop can run <span className="font-semibold text-slate-800">guarded automation (Level&nbsp;2)</span>: it
            performs low-risk SEO work for you automatically — metadata edits, sitemap submits, and technical fix tasks — inside your budget and
            policy limits. Medium and high-risk changes still wait for your review.
          </p>

          <div
            id="automation-status"
            className={cx(
              "flex flex-col gap-4 rounded-xl border p-4 sm:flex-row sm:items-center sm:justify-between",
              automationPaused ? "border-amber-200 bg-amber-50" : "border-emerald-200 bg-emerald-50",
            )}
          >
            <div>
              <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">Automation status</div>
              <div className="mt-1 text-lg font-bold text-slate-950">{automationPaused ? "Paused" : "Active"}</div>
              <p className="mt-1 max-w-2xl text-sm leading-5 text-slate-600">
                {automationPaused
                  ? "Automation is paused. CiteLoop will not run scheduled automation or execute changes automatically."
                  : "Automation is active. CiteLoop may run scheduled automation according to the autonomy level below."}
              </p>
            </div>
            <Button
              size="sm"
              variant={automationPaused ? "primary" : "outline"}
              onClick={() => saveAutomationPolicy({ automation_paused: !automationPaused })}
              disabled={busy || !policy}
            >
              <ButtonProgress busy={busy} busyLabel={automationPaused ? "Resuming" : "Pausing"} idleIcon={<Power size={14} />}>
                {automationPaused ? "Resume automation" : "Pause automation"}
              </ButtonProgress>
            </Button>
          </div>

          <div className="grid gap-3 rounded-xl border border-slate-200 bg-white p-4 sm:grid-cols-2 lg:grid-cols-4">
            <div>
              <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">Readiness</div>
              <div className="mt-1 text-lg font-bold text-slate-950">
                {readyGates.length} of {readinessGatesAll.length} checks ready
              </div>
            </div>
            <div>
              <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">Autonomy level</div>
              <div className="mt-1 text-lg font-bold text-slate-950">Level {policy?.autopilot_level ?? 0}</div>
            </div>
            <div>
              <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">Automation status</div>
              <div className="mt-1 text-lg font-bold text-slate-950">{automationPaused ? "Paused" : "Active"}</div>
            </div>
            <div>
              <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">Emergency stop</div>
              <div className="mt-1 text-lg font-bold text-slate-950">{policy?.kill_switch_enabled ? "On" : "Off"}</div>
            </div>
          </div>

          <div>
            <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
              <div>
                <h3 className="text-sm font-bold text-slate-950">Readiness checks</h3>
                <p className="mt-1 text-sm text-slate-500">Open a square check card to understand the blocker, confirm the policy, or jump to the right setup surface.</p>
              </div>
              <Badge tone={blockedGates.length > 0 ? "red" : "green"}>
                {blockedGates.length > 0 ? `${blockedGates.length} blocked` : "Ready for Level 2"}
              </Badge>
            </div>
            <span id="automation-policy" className="sr-only" aria-hidden="true" />
            <span id="recovery-plan" className="sr-only" aria-hidden="true" />
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              {automationReadinessCards.map((card) => (
                <button
                  type="button"
                  key={card.id}
                  id={`automation-card-${card.id}`}
                  onClick={() => openAutomationCheck(card.id)}
                  aria-label={`Open ${card.title} details`}
                  className={cx(
                    "aspect-square rounded-lg border p-4 text-left shadow-sm transition-all duration-150 hover:-translate-y-0.5 hover:shadow-md focus:outline-none focus:ring-2 focus:ring-slate-300",
                    card.blocked
                      ? "border-red-200 bg-red-50 text-red-950"
                      : "border-emerald-200 bg-emerald-50 text-emerald-950",
                  )}
                >
                  <div className="flex h-full flex-col justify-between">
                    <div className="flex items-start justify-between gap-3">
                      <span
                        className={cx(
                          "inline-flex h-9 w-9 items-center justify-center rounded-lg bg-white/80",
                          card.blocked ? "text-red-700" : "text-emerald-700",
                        )}
                      >
                        {card.blocked ? <AlertTriangle size={18} aria-hidden="true" /> : <CheckCircle2 size={18} aria-hidden="true" />}
                      </span>
                      <Badge tone={card.blocked ? "red" : "green"}>{card.blocked ? "Blocked" : "Done"}</Badge>
                    </div>
                    <div>
                      <div className="text-base font-bold leading-5">{card.title}</div>
                      <p className="mt-2 text-sm leading-5 opacity-80">{card.summary}</p>
                    </div>
                  </div>
                </button>
              ))}
            </div>
          </div>
        </div>

        {selectedAutomationCard && (
          <div className="fixed inset-0 z-40 flex items-center justify-center bg-slate-950/35 px-4 py-6">
            <div
              role="dialog"
              aria-modal="true"
              aria-labelledby="automation-check-modal-title"
              aria-describedby="automation-check-modal-detail"
              className="flex max-h-[88vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-slate-200 bg-white shadow-xl"
            >
              <div className="flex items-start justify-between gap-4 border-b border-slate-200 px-5 py-4">
                <div>
                  <div className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">Automation check details</div>
                  <h3 id="automation-check-modal-title" className="mt-1 text-xl font-bold text-slate-950">
                    {selectedAutomationCard.title}
                  </h3>
                  <p id="automation-check-modal-detail" className="mt-1 max-w-2xl text-sm leading-5 text-slate-500">
                    {selectedAutomationCard.summary}
                  </p>
                </div>
                <button
                  type="button"
                  onClick={closeAutomationCheck}
                  className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900"
                  aria-label="Close automation check details"
                >
                  <X size={16} aria-hidden="true" />
                </button>
              </div>

              <div className="overflow-y-auto px-5 py-4">
                {selectedAutomationCard.id === "autopilot_policy_confirmed" ||
                selectedAutomationCard.id === "automation_pause_clear" ||
                selectedAutomationCard.id === "monthly_budget_configured" ? (
                  <div className="space-y-5">
                    <Notice
                      tone={selectedAutomationCard.blocked ? "amber" : "green"}
                      title="Automation Policy"
                      detail="Automation status, autonomy level, Autopilot budget, safe mode, and emergency stop decide what CiteLoop can do without asking first."
                    />

                    <div>
                      <div className="mb-2 text-sm font-bold text-slate-950">Automation status</div>
                      <div className="grid gap-2 sm:grid-cols-2">
                        {[
                          {
                            paused: false,
                            title: "Active",
                            detail: "Automation is active. CiteLoop may run scheduled automation according to the autonomy level below.",
                          },
                          {
                            paused: true,
                            title: "Paused",
                            detail: "Automation is paused. CiteLoop will not run scheduled automation or execute changes automatically.",
                          },
                        ].map((option) => {
                          const selected = policyDraft.automation_paused === option.paused;
                          return (
                            <button
                              type="button"
                              key={option.title}
                              disabled={busy}
                              onClick={() => setPolicyDraft((current) => ({ ...current, automation_paused: option.paused }))}
                              className={cx(
                                "rounded-lg border px-3 py-3 text-left transition-colors",
                                selected ? "border-[#d93820] bg-red-50" : "border-slate-200 bg-white hover:bg-slate-50",
                              )}
                            >
                              <div className="flex flex-wrap items-center justify-between gap-2">
                                <span className="text-sm font-bold text-slate-950">{option.title}</span>
                                <Badge tone={option.paused ? "amber" : "green"}>{option.paused ? "Paused" : "Active"}</Badge>
                              </div>
                              <p className="mt-1 text-sm leading-5 text-slate-500">{option.detail}</p>
                            </button>
                          );
                        })}
                      </div>
                    </div>

                    <div>
                      <div className="mb-2 text-sm font-bold text-slate-950">Autonomy level</div>
                      <div className="grid gap-2">
                        {automationPolicyLevels.map((level) => {
                          const selected = policyDraft.autopilot_level === level.value;
                          return (
                            <button
                              type="button"
                              key={level.value}
                              disabled={!level.available || busy}
                              onClick={() => setPolicyDraft((current) => ({ ...current, autopilot_level: level.value }))}
                              className={cx(
                                "rounded-lg border px-3 py-3 text-left transition-colors",
                                selected ? "border-[#d93820] bg-red-50" : "border-slate-200 bg-white hover:bg-slate-50",
                                !level.available && "cursor-not-allowed opacity-55",
                              )}
                            >
                              <div className="flex flex-wrap items-center justify-between gap-2">
                                <span className="text-sm font-bold text-slate-950">{level.title}</span>
                                <Badge tone={level.available ? "neutral" : "amber"}>{level.mode}</Badge>
                              </div>
                              <p className="mt-1 text-sm leading-5 text-slate-500">{level.detail}</p>
                            </button>
                          );
                        })}
                      </div>
                    </div>

                    <div className="grid gap-4 md:grid-cols-2">
                      <Field label="Autopilot budget" helper="This edits the Autopilot policy limit, not the project config budget.">
                        <TextInput
                          type="number"
                          min={0}
                          value={policyDraft.monthly_budget_limit}
                          onChange={(event) =>
                            setPolicyDraft((current) => ({ ...current, monthly_budget_limit: Math.max(0, Number(event.target.value) || 0) }))
                          }
                        />
                      </Field>
                      <div className="grid gap-2">
                        <label className="flex items-start gap-3 rounded-lg border border-slate-200 bg-white p-3 text-sm">
                          <input
                            type="checkbox"
                            className="mt-1 h-4 w-4"
                            checked={policyDraft.kill_switch_enabled}
                            disabled={busy}
                            onChange={(event) => setPolicyDraft((current) => ({ ...current, kill_switch_enabled: event.target.checked }))}
                          />
                          <span>
                            <span className="block font-bold text-slate-900">Emergency stop</span>
                            <span className="mt-1 block text-slate-500">
                              {policyDraft.kill_switch_enabled ? "Emergency stop is on. Automation is paused." : "Emergency stop is off. Eligible automation may run."}
                            </span>
                          </span>
                        </label>
                        <label className="flex items-start gap-3 rounded-lg border border-slate-200 bg-white p-3 text-sm">
                          <input
                            type="checkbox"
                            className="mt-1 h-4 w-4"
                            checked={policyDraft.safe_mode_enabled}
                            disabled={busy}
                            onChange={(event) => setPolicyDraft((current) => ({ ...current, safe_mode_enabled: event.target.checked }))}
                          />
                          <span>
                            <span className="block font-bold text-slate-900">Safe mode</span>
                            <span className="mt-1 block text-slate-500">
                              {policyDraft.safe_mode_enabled
                                ? "Safe mode policy switch is on. CiteLoop pauses guarded execution."
                                : openSafeModeCount > 0
                                  ? `${openSafeModeCount} open safe mode event${openSafeModeCount === 1 ? "" : "s"} still block automation. Exit them from the Safe mode card.`
                                  : "Safe mode policy switch is off."}
                            </span>
                          </span>
                        </label>
                      </div>
                    </div>

                    <Notice
                      title="Review and save policy changes"
                      detail="Nothing in this modal saves until you press Save policy. The board refreshes after the backend confirms the policy."
                    />
                  </div>
                ) : (
                  <div className="grid gap-4">
                    <div className="rounded-lg border border-slate-200 bg-slate-50 p-4">
                      <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">Why this matters</div>
                      <p className="mt-2 text-sm leading-6 text-slate-700">{selectedAutomationCard.impact}</p>
                    </div>
                    <div className="rounded-lg border border-slate-200 bg-white p-4">
                      <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">Current status</div>
                      <p className="mt-2 text-sm leading-6 text-slate-700">{selectedAutomationCard.reason}</p>
                    </div>
                    <div className="rounded-lg border border-slate-200 bg-white p-4">
                      <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">Next step</div>
                      <p className="mt-2 text-sm leading-6 text-slate-700">{selectedAutomationCard.nextAction}</p>
                    </div>
                    {selectedAutomationCard.id === "rollback_or_recovery_ready" && (
                      <>
                        <div id="recovery-plan-return" className="rounded-lg border border-slate-200 bg-white p-4">
                          <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">Settings - automation - recovery plan</div>
                          <p className="mt-2 text-sm leading-6 text-slate-700">
                            Review the manual recovery plan before acknowledging it. The confirmation stays in this dialog so you can return here
                            after review and complete the check.
                          </p>
                          <button
                            type="button"
                            onClick={reviewRecoveryPlan}
                            className="mt-3 inline-flex h-9 items-center justify-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-sm font-bold text-slate-700 transition-colors hover:bg-slate-50"
                          >
                            Review recovery plan
                            <ArrowRight size={15} aria-hidden="true" />
                          </button>
                        </div>
                        {reviewedRecoveryPlan && (
                          <div id="recovery-plan-review" className="rounded-lg border border-slate-200 bg-slate-50 p-4">
                            <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">Manual recovery plan</div>
                            <div className="mt-3 grid gap-3 text-sm leading-6 text-slate-700">
                              <p>
                                If a guarded action causes a publishing issue, pause automation, use the publisher history or repository history to
                                identify the change, then revert the affected file or page before resuming automation.
                              </p>
                              <p>
                                Keep the related action, run, and notification records open while recovering so the operator can verify what changed
                                and record the recovery result.
                              </p>
                            </div>
                            <button
                              type="button"
                              onClick={returnToRecoveryCheck}
                              className="mt-3 inline-flex h-9 items-center justify-center rounded-lg border border-slate-200 bg-white px-3 text-sm font-bold text-slate-700 transition-colors hover:bg-slate-50"
                            >
                              Return to recovery check
                            </button>
                          </div>
                        )}
                      </>
                    )}
                    {selectedAutomationCard.id === "safe_mode_clear" && (openSafeModeEvents.length > 0 || policy?.safe_mode_enabled) && (
                      <div className="rounded-lg border border-slate-200 bg-white p-4">
                        <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">Open safe mode events</div>
                        {openSafeModeEvents.length > 0 ? (
                          <div className="mt-3 grid gap-2">
                            {openSafeModeEvents.map((event) => (
                              <div key={event.id} className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-sm">
                                <div className="font-bold text-slate-900">{event.reason || "Safe mode is active"}</div>
                                <div className="mt-1 text-xs text-slate-500">
                                  {event.trigger_source || "manual"} · entered {formatDate(event.entered_at)}
                                </div>
                              </div>
                            ))}
                          </div>
                        ) : (
                          <p className="mt-2 text-sm leading-6 text-slate-700">
                            No open safe mode events remain. The policy-level safe mode switch is still on.
                          </p>
                        )}
                      </div>
                    )}
                  </div>
                )}
              </div>

              <div className="flex flex-wrap justify-end gap-2 border-t border-slate-200 bg-slate-50 px-5 py-4">
                <Button variant="ghost" onClick={closeAutomationCheck} disabled={busy}>
                  Close
                </Button>
                {selectedAutomationCard.id === "autopilot_policy_confirmed" ||
                selectedAutomationCard.id === "automation_pause_clear" ||
                selectedAutomationCard.id === "monthly_budget_configured" ? (
                  <Button variant="primary" onClick={savePolicyDraft} disabled={busy || !policy}>
                    <ButtonProgress busy={busy} busyLabel="Saving policy" idleIcon={<Save size={16} />}>
                      Save policy
                    </ButtonProgress>
                  </Button>
                ) : selectedAutomationCard.blocked ? (
                  <Button
                    variant="primary"
                    onClick={() => handleAutomationCheckAction(selectedAutomationCard.id, selectedAutomationCard.action?.href)}
                    disabled={busy}
                  >
                    <ButtonProgress busy={busy} busyLabel="Updating" idleIcon={<ArrowRight size={16} />}>
                      {selectedAutomationCard.id === "kill_switch_clear"
                        ? "Review emergency stop"
                          : selectedAutomationCard.id === "safe_mode_clear"
                            ? "Exit safe mode"
                            : selectedAutomationCard.id === "rollback_or_recovery_ready"
                              ? reviewedRecoveryPlan
                                ? "Confirm recovery plan"
                                : "Review recovery plan"
                              : selectedAutomationCard.action?.cta ?? "Fix check"}
                    </ButtonProgress>
                  </Button>
                ) : (
                  <Button variant="primary" onClick={closeAutomationCheck}>
                    Confirm
                  </Button>
                )}
              </div>
            </div>
          </div>
        )}
      </section>
      )}

      {activeSettingsTab === "activity" && (
      <section id="settings-panel-activity" role="tabpanel" aria-labelledby="settings-tab-activity" tabIndex={0}>
        <RunsClient projectId={projectId} embeddedInSettings />
      </section>
      )}

      {activeSettingsTab === "search-console" && (
      <section id="settings-panel-search-console" role="tabpanel" aria-labelledby="settings-tab-search-console" tabIndex={0}>
        <SectionHeader
          title="Google connection"
          eyebrow="Google data connections"
          action={
            <Button size="sm" onClick={refreshGoogleConnections} disabled={Boolean(gscBusy) || ga4Busy}>
              <RefreshCw size={14} />
              Refresh
            </Button>
          }
        />
        <div className="grid gap-4">
          <div className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
            <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
              <div className="flex gap-3">
                <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-600">
                  <Search size={18} />
                </div>
                <div>
                  <div className="text-xs font-semibold uppercase tracking-wide text-slate-400">Search Console</div>
                  <div className="mt-1 text-sm font-bold text-slate-900">{gscCardTitle}</div>
                  <p className="mt-1 max-w-2xl text-sm leading-5 text-slate-500">
                    CiteLoop uses the selected property for query, CTR, position, and content-decay signals in Opportunities and Results.
                  </p>
                </div>
              </div>
              <Badge tone={gscTone(gscConnection?.status)}>{gscStatusLabel(gscConnection?.status)}</Badge>
            </div>

            {gscConnection && !gscConnection.configured && (
              <Notice
                title="Google OAuth is not configured"
                detail="Add GOOGLE_OAUTH_CLIENT_ID, GOOGLE_OAUTH_CLIENT_SECRET, and PUBLIC_APP_URL before customers can connect Search Console."
                tone="amber"
              />
            )}

            {gscConnection?.last_error && <Notice title="Search Console needs attention" detail={gscConnection.last_error} tone="red" />}

            {(!gscConnection || gscConnection.status === "missing" || gscConnection.properties.length === 0) && <GSCSetupGuide siteURL={config.site_url} />}

            <div className="flex flex-wrap gap-2">
              {canStartGSCOAuth && (
                <Button variant="primary" onClick={() => startSearchConsoleOAuth()} disabled={Boolean(gscBusy) || gscConnection?.configured === false}>
                  <ButtonProgress busy={gscBusy === "connect"} busyLabel="Opening Google" idleIcon={<Search size={16} />}>
                    Connect Search Console
                  </ButtonProgress>
                </Button>
              )}
              <Button
                variant="outline"
                onClick={revokeGSCConnection}
                disabled={!canDisconnectGSC || Boolean(gscBusy)}
              >
                <ButtonProgress busy={gscBusy === "revoke"} busyLabel="Disconnecting" idleIcon={<Trash2 size={16} />}>
                  Disconnect
                </ButtonProgress>
              </Button>
            </div>

            <div className="grid gap-3 rounded-lg border border-slate-100 bg-slate-50 p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <div className="text-sm font-bold text-slate-900">Authorized properties</div>
                  <p className="mt-1 text-sm leading-5 text-slate-500">Select property that matches this project domain.</p>
                </div>
                {gscConnection?.recommended_property && <Badge tone="green">Recommended: {gscConnection.recommended_property}</Badge>}
              </div>

              {gscConnection && gscConnection.properties.length > 0 ? (
                <div className="grid gap-2">
                  {gscConnection.properties.map((property) => {
                    const selected = gscConnection.selected_property === property.site_url;
                    return (
                      <div key={property.site_url} className="flex flex-col gap-2 rounded-lg bg-white px-3 py-3 sm:flex-row sm:items-center sm:justify-between">
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="truncate text-sm font-semibold text-slate-900">{property.site_url}</span>
                            {property.recommended && <Badge tone="green">Recommended</Badge>}
                            {selected && <Badge tone="blue">Selected</Badge>}
                          </div>
                          <div className="mt-1 text-xs font-semibold text-slate-400">{property.permission_level || "permission available"}</div>
                        </div>
                        <Button size="sm" variant={selected ? "ghost" : "outline"} onClick={() => selectGSCProperty(property.site_url)} disabled={selected || Boolean(gscBusy)}>
                          <ButtonProgress busy={gscBusy === `select-${property.site_url}`} busyLabel="Selecting" idleIcon={<CheckCircle2 size={14} />}>
                            Select property
                          </ButtonProgress>
                        </Button>
                      </div>
                    );
                  })}
                </div>
              ) : (
                <EmptyGSCProperties />
              )}
            </div>
          </div>

          <div className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
            <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
              <div className="flex gap-3">
                <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-600">
                  <Settings2 size={18} />
                </div>
                <div>
                  <div className="text-sm font-bold text-slate-900">Google Analytics connection</div>
                  <p className="mt-1 max-w-2xl text-sm leading-5 text-slate-500">
                    Connect GA4 engagement and key event data so CiteLoop can measure published work against search and business outcomes.
                  </p>
                </div>
              </div>
              <Badge tone={ga4Tone(ga4Status, savedGA4PropertyID)}>{ga4StatusLabel(ga4Status, savedGA4PropertyID)}</Badge>
            </div>

            {ga4Integration?.last_error && <Notice title="Google Analytics needs attention" detail={friendlyGA4Error(ga4Integration.last_error)} tone="red" />}

            <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_18rem]">
              <div className="grid gap-3">
                <Field
                  label="GA4 Property ID"
                  helper="Use the numeric property ID from GA4 Property settings, not the G- measurement ID."
                >
                  <TextInput
                    inputMode="numeric"
                    value={ga4PropertyID}
                    placeholder="123456789"
                    onChange={(event) => setGA4PropertyID(event.target.value)}
                  />
                </Field>
                {savedGA4PropertyID && (
                  <div className="text-sm text-slate-500">
                    Saved property: <span className="font-semibold text-slate-800">{savedGA4PropertyID}</span>
                  </div>
                )}
                <div className="flex flex-wrap gap-2">
                  <Button variant="primary" onClick={saveGA4Connection} disabled={ga4Busy || !ga4PropertyID.trim()}>
                    <ButtonProgress busy={ga4Busy} busyLabel="Saving" idleIcon={<Save size={16} />}>
                      Save GA4 property
                    </ButtonProgress>
                  </Button>
                  {ga4NeedsGooglePermissions && (
                    <Button onClick={() => startSearchConsoleOAuth("analytics")} disabled={Boolean(gscBusy) || gscConnection?.configured === false}>
                      <ButtonProgress busy={gscBusy === "analytics-permissions"} busyLabel="Opening Google" idleIcon={<Search size={16} />}>
                        Update Analytics access
                      </ButtonProgress>
                    </Button>
                  )}
                  <a
                    href="https://analytics.google.com/analytics/web/"
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex h-10 items-center justify-center rounded-lg border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-700 transition-colors hover:bg-slate-50"
                  >
                    Open Google Analytics
                  </a>
                </div>
              </div>

              <div className="rounded-lg border border-slate-100 bg-slate-50 p-3">
                <ConnectionInstructions steps={ga4ConnectionSteps} />
              </div>
            </div>
          </div>
        </div>
      </section>
      )}

      {activeSettingsTab === "project" && (
      <section id="settings-panel-project" role="tabpanel" aria-labelledby="settings-tab-project" tabIndex={0}>
        <SectionHeader title="Project config" />
        <div className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
        <div className="grid gap-4 md:grid-cols-3">
          <Field label="Cadence per week">
            <TextInput
              inputMode="numeric"
              value={config.cadence_per_week}
              onChange={(event) => update({ cadence_per_week: Math.max(1, toInt(event.target.value, 3)) })}
            />
          </Field>
          <Field label="Buffer days">
            <TextInput
              inputMode="numeric"
              value={config.buffer_days}
              onChange={(event) => update({ buffer_days: Math.max(0, toInt(event.target.value, 5)) })}
            />
          </Field>
          <Field label="Monthly budget USD">
            <TextInput
              inputMode="decimal"
              value={config.monthly_budget_usd}
              onChange={(event) => update({ monthly_budget_usd: Math.max(0, toFloat(event.target.value, 50)) })}
            />
          </Field>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <Field label="Blog mix">
            <TextInput
              inputMode="decimal"
              value={config.channel_mix.blog}
              onChange={(event) =>
                update({ channel_mix: { ...config.channel_mix, blog: Math.max(0, toFloat(event.target.value, 0.6)) } })
              }
            />
          </Field>
          <Field label="Syndication mix">
            <TextInput
              inputMode="decimal"
              value={config.channel_mix.syndication}
              onChange={(event) =>
                update({
                  channel_mix: { ...config.channel_mix, syndication: Math.max(0, toFloat(event.target.value, 0.4)) },
                })
              }
            />
          </Field>
        </div>

        <Field label="Brand voice">
          <TextArea
            value={config.brand_voice ?? ""}
            onChange={(event) => update({ brand_voice: event.target.value })}
            className="min-h-24"
            placeholder="Direct, evidence-backed, pragmatic."
          />
        </Field>
        <div>
          <Button disabled={busy} variant="primary" onClick={save}>
            <ButtonProgress busy={busy} busyLabel="Saving settings" idleIcon={<Save size={16} />}>
              Save settings
            </ButtonProgress>
          </Button>
        </div>
        </div>
      </section>
      )}

      {activeSettingsTab === "publisher" && (
      <section id="settings-panel-publisher" role="tabpanel" aria-labelledby="settings-tab-publisher" tabIndex={0}>
        <SectionHeader
          title="Publisher connection"
          eyebrow="Publishing"
          action={
            <Button
              size="sm"
              onClick={() => {
                refreshPublisherConnections();
                refreshGithubIntegration();
              }}
              disabled={notificationBusy === "save-publisher"}
            >
              <RefreshCw size={14} />
              Refresh
            </Button>
          }
        />
        <div id="reddit-rules" className="mb-4 grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h3 className="text-sm font-semibold text-slate-900">Reddit target rules</h3>
              <p className="mt-1 text-xs leading-5 text-slate-500">Paste and explicitly verify subreddit rules. CiteLoop stores an immutable revision for 30 days; it does not claim to retrieve rules from Reddit.</p>
            </div>
            <Badge tone={redditContexts.some((context) => context.status === "confirmed" && context.expires_at && new Date(context.expires_at).getTime() > Date.now()) ? "green" : "amber"}>
              {redditContexts.some((context) => context.status === "confirmed" && context.expires_at && new Date(context.expires_at).getTime() > Date.now()) ? "current rules" : "setup required"}
            </Badge>
          </div>
          <div className="grid gap-3 md:grid-cols-2">
            <Field label="Subreddit">
              <TextInput value={redditRulesDraft.target_key} placeholder="r/SaaS" onChange={(event) => setRedditRulesDraft((current) => ({ ...current, target_key: event.target.value }))} />
            </Field>
            <Field label="Rules URL">
              <TextInput value={redditRulesDraft.rules_url} placeholder="https://www.reddit.com/r/SaaS/about/rules" onChange={(event) => setRedditRulesDraft((current) => ({ ...current, rules_url: event.target.value }))} />
            </Field>
            <Field label="Allowed post types (comma-separated)">
              <TextInput value={redditRulesDraft.allowed_post_types} onChange={(event) => setRedditRulesDraft((current) => ({ ...current, allowed_post_types: event.target.value }))} />
            </Field>
            <Field label="Required flair (optional)">
              <TextInput value={redditRulesDraft.required_flair} onChange={(event) => setRedditRulesDraft((current) => ({ ...current, required_flair: event.target.value }))} />
            </Field>
            <Field label="Link policy">
              <TextInput value={redditRulesDraft.link_policy} placeholder="Source links allowed after context" onChange={(event) => setRedditRulesDraft((current) => ({ ...current, link_policy: event.target.value }))} />
            </Field>
            <Field label="Self-promotion policy">
              <TextInput value={redditRulesDraft.self_promotion_policy} placeholder="Disclose affiliation" onChange={(event) => setRedditRulesDraft((current) => ({ ...current, self_promotion_policy: event.target.value }))} />
            </Field>
          </div>
          <Field label="Rules text">
            <TextArea rows={6} value={redditRulesDraft.rules_text} onChange={(event) => setRedditRulesDraft((current) => ({ ...current, rules_text: event.target.value }))} />
          </Field>
          <Field label="Disclosure requirements (optional)">
            <TextInput value={redditRulesDraft.disclosure_requirements} onChange={(event) => setRedditRulesDraft((current) => ({ ...current, disclosure_requirements: event.target.value }))} />
          </Field>
          <label className="flex items-start gap-2 text-sm text-slate-700">
            <input type="checkbox" className="mt-1" checked={redditRulesDraft.verified} onChange={(event) => setRedditRulesDraft((current) => ({ ...current, verified: event.target.checked }))} />
            <span>I reviewed the current subreddit rules and confirm this transcription is accurate.</span>
          </label>
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="primary" onClick={confirmRedditRules} disabled={notificationBusy === "save-reddit-rules" || !redditRulesDraft.verified}>
              <ButtonProgress busy={notificationBusy === "save-reddit-rules"} busyLabel="Confirming rules" idleIcon={<CheckCircle2 size={16} />}>Confirm rules revision</ButtonProgress>
            </Button>
            {redditContexts.slice(0, 3).map((context) => (
              <Button key={context.id} size="sm" variant="outline" onClick={() => reconfirmRedditRules(context.id)} disabled={Boolean(notificationBusy)}>
                Reconfirm {context.target_key} v{context.version}
              </Button>
            ))}
          </div>
        </div>
        <div className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2 text-sm font-semibold text-slate-900">
              <GitBranch size={16} />
              GitHub/Next.js
            </div>
            <Badge tone={githubPublisher?.status === "connected" ? "green" : githubPublisher?.status === "error" ? "red" : "amber"}>
              {githubPublisher?.status ?? "missing"}
            </Badge>
            <Badge tone={githubPublisher?.enabled ? "green" : "neutral"}>{githubPublisher?.enabled ? "enabled" : "disabled"}</Badge>
          </div>

          {githubIntegration?.configured === false ? (
            <Notice
              title="GitHub OAuth is not configured"
              detail="Add the GitHub App client and private key environment variables on the backend to enable OAuth connection."
              tone="amber"
            />
          ) : (
            <div
              className={cx(
                "border-y px-0 py-3",
                githubAppConnected ? "border-green-100 text-green-950" : "border-slate-100 text-slate-900",
              )}
            >
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="min-w-0">
                  <div className="inline-flex items-center gap-2 text-sm font-bold">
                    {githubAppConnected ? <CheckCircle2 size={16} /> : <GitBranch size={16} />}
                    {githubAppTitle}
                  </div>
                  <p className="mt-1 max-w-2xl text-sm leading-5 text-slate-500">{githubAppDetail}</p>
                </div>
                <div className="flex shrink-0 flex-wrap gap-2 sm:justify-end">
                  {githubAppReusable && (
                    <Button variant="primary" onClick={reuseGithub}>
                      <RotateCcw size={16} />
                      Use connected GitHub
                    </Button>
                  )}
                  <Button
                    variant={githubAppConnected || githubAppReusable ? "outline" : "primary"}
                    onClick={connectGithub}
                    disabled={!githubIntegration?.install_url}
                  >
                    <Settings2 size={16} />
                    {githubAppConnected || githubAppReusable ? "Change repository or access" : "Connect GitHub"}
                  </Button>
                </div>
              </div>
            </div>
          )}

          <div
            aria-live="polite"
            aria-busy={githubReadinessBusy === "checking"}
            className={cx("border-y px-3 py-3", githubReadinessSurface)}
          >
            <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div className="min-w-0">
                <div className="inline-flex items-center gap-2 text-sm font-bold">
                  {githubPRReadiness?.status === "ready" ? (
                    <CheckCircle2 aria-hidden="true" size={16} />
                  ) : (
                    <AlertTriangle aria-hidden="true" size={16} />
                  )}
                  {githubReadinessPresentation.title}
                </div>
                <p className="mt-1 max-w-2xl text-sm leading-5 opacity-80">
                  {githubPRReadiness?.detail || githubReadinessPresentation.detail}
                </p>
                {(githubReadinessTarget || githubPRReadiness?.checked_at) && (
                  <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs font-medium opacity-75">
                    {githubReadinessTarget && <span>Target: {githubReadinessTarget}</span>}
                    {githubPRReadiness?.checked_at && <span>Last checked: {formatDate(githubPRReadiness.checked_at)}</span>}
                  </div>
                )}
                {githubReadinessBusy === "checking" && (
                  <div className="mt-2 text-xs font-semibold">Checking repository and pull-request access...</div>
                )}
                {githubReadinessError && (
                  <div role="alert" className="mt-2 text-sm font-semibold text-red-800">
                    {githubReadinessError}
                  </div>
                )}
              </div>
              <Button
                size="sm"
                variant="outline"
                aria-label="Check GitHub PR readiness again"
                onClick={() => void refreshGithubPRReadiness()}
                disabled={githubReadinessBusy === "checking"}
                className="shrink-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-slate-400 focus-visible:ring-offset-2"
              >
                <ButtonProgress
                  busy={githubReadinessBusy === "checking"}
                  busyLabel="Checking..."
                  idleIcon={<RefreshCw aria-hidden="true" size={14} />}
                >
                  Check again
                </ButtonProgress>
              </Button>
            </div>
          </div>

          <ConnectionInstructions
            steps={[
              "Install the CiteLoop GitHub App.",
              "Grant access to the content repository.",
              "Choose repo, branch, content path, and base URL.",
              "Save and test the connection.",
              "Enable publishing.",
            ]}
          />

          <div className="grid gap-4 lg:grid-cols-2">
            <Field label="Repository">
              <TextInput
                value={publisherDraft.repo}
                onChange={(event) => setPublisherDraft((current) => ({ ...current, repo: event.target.value }))}
                placeholder="owner/repo"
              />
            </Field>
            <Field label="Branch">
              <TextInput
                value={publisherDraft.branch ?? ""}
                onChange={(event) => setPublisherDraft((current) => ({ ...current, branch: event.target.value }))}
                placeholder="citeloop-content"
              />
            </Field>
            <Field label="Content path">
              <TextInput
                value={publisherDraft.content_dir ?? ""}
                onChange={(event) => setPublisherDraft((current) => ({ ...current, content_dir: event.target.value }))}
                placeholder="content/citeloop/blog"
              />
            </Field>
            <Field label="Base URL">
              <TextInput
                value={publisherDraft.base_url}
                onChange={(event) => setPublisherDraft((current) => ({ ...current, base_url: event.target.value }))}
                placeholder="https://example.com/blog"
              />
            </Field>
            <Field label="Publish mode">
              <TextInput
                value={publisherDraft.publish_mode ?? "publish"}
                onChange={(event) => setPublisherDraft((current) => ({ ...current, publish_mode: event.target.value }))}
                placeholder="publish"
              />
            </Field>
          </div>

          <div className="border-t border-slate-100 pt-4">
            <button
              type="button"
              onClick={() => setShowManualPublisherCredential((current) => !current)}
              className="inline-flex items-center gap-2 text-sm font-semibold text-slate-600 transition-colors hover:text-slate-950"
            >
              <Settings2 size={14} />
              Advanced: connect with a personal access token
            </button>
            {showManualPublisherCredential && (
              <div className="mt-3 grid max-w-xl gap-3 border-l border-slate-200 pl-3">
                <Field label="GitHub token">
                  <div className="grid gap-2">
                    <TextInput
                      type="password"
                      value={publisherCredentialDraft}
                      onChange={(event) => setPublisherCredentialDraft(event.target.value)}
                      placeholder={githubPublisher?.credential_configured ? "Saved" : "ghp_..."}
                      autoComplete="off"
                    />
                    <Badge tone={githubPublisher?.credential_configured ? "green" : "amber"}>
                      {githubPublisher?.credential_configured ? "Credential saved" : "Credential missing"}
                    </Badge>
                  </div>
                </Field>
                <p className="text-sm leading-5 text-slate-500">
                  Use a personal access token only when the GitHub App OAuth connection is unavailable for this repository.
                </p>
              </div>
            )}
          </div>

          <div className="border-t border-slate-100 pt-4">
            <div className="text-sm font-bold text-slate-900">CMS connector roadmap</div>
            <p className="mt-1 max-w-2xl text-sm leading-5 text-slate-500">
              WordPress, Webflow, Shopify, and custom CMS connectors will start as gated drafts and move to OAuth publishing after verification is ready.
            </p>
            <div className="mt-3 grid gap-2 md:grid-cols-2">
              {["WordPress", "Webflow", "Shopify", "Custom CMS"].map((name) => (
                <div key={name} className="flex items-center justify-between gap-3 rounded-lg border border-slate-100 bg-slate-50 px-3 py-2">
                  <div className="min-w-0">
                    <div className="text-sm font-semibold text-slate-900">{name}</div>
                    <div className="mt-0.5 text-xs font-semibold uppercase text-slate-400">Draft-only until OAuth connector is ready</div>
                  </div>
                  <Badge tone="neutral">Roadmap</Badge>
                </div>
              ))}
            </div>
          </div>

          {githubPublisher?.last_error && <Notice title="Publisher health check failed" detail={githubPublisher.last_error} tone="red" />}


          <div className="flex flex-wrap gap-2">
            <Button variant="primary" onClick={savePublisherConnection} disabled={notificationBusy === "save-publisher"}>
              <ButtonProgress busy={notificationBusy === "save-publisher"} busyLabel="Saving publisher" idleIcon={<Save size={16} />}>
                Save publisher
              </ButtonProgress>
            </Button>
            {(showManualPublisherCredential || publisherCredentialDraft.trim()) && (
              <Button
                variant="outline"
                onClick={savePublisherCredential}
                disabled={!githubPublisher || !publisherCredentialDraft.trim() || notificationBusy === `save-publisher-credential-${githubPublisher?.id}`}
              >
                <ButtonProgress
                  busy={notificationBusy === `save-publisher-credential-${githubPublisher?.id}`}
                  busyLabel="Saving token"
                  idleIcon={<Save size={16} />}
                >
                  Save token
                </ButtonProgress>
              </Button>
            )}
            <Button
              variant="outline"
              onClick={() => githubPublisher && testPublisherConnection(githubPublisher.id)}
              disabled={!githubPublisher || notificationBusy === `test-publisher-${githubPublisher?.id}`}
            >
              <ButtonProgress busy={notificationBusy === `test-publisher-${githubPublisher?.id}`} busyLabel="Testing" idleIcon={<Send size={16} />}>
                Test
              </ButtonProgress>
            </Button>
            {githubPublisher?.enabled ? (
              <Button
                variant="outline"
                onClick={() => githubPublisher && setPublisherConnectionEnabled(githubPublisher, false)}
                disabled={!githubPublisher || notificationBusy === `toggle-publisher-${githubPublisher?.id}`}
              >
                <ButtonProgress
                  busy={notificationBusy === `toggle-publisher-${githubPublisher?.id}`}
                  busyLabel="Disabling"
                  idleIcon={<Power size={16} />}
                >
                  Disable
                </ButtonProgress>
              </Button>
            ) : (
              <Button
                variant="primary"
                onClick={() => githubPublisher && setPublisherConnectionEnabled(githubPublisher, true)}
                disabled={!githubPublisher || notificationBusy === `toggle-publisher-${githubPublisher?.id}`}
              >
                <ButtonProgress
                  busy={notificationBusy === `toggle-publisher-${githubPublisher?.id}`}
                  busyLabel="Enabling"
                  idleIcon={<Power size={16} />}
                >
                  Enable
                </ButtonProgress>
              </Button>
            )}
            <Button
              variant="outline"
              onClick={revokePublisherCredential}
              disabled={!githubPublisher?.credential_configured || notificationBusy === `revoke-publisher-credential-${githubPublisher?.id}`}
            >
              <ButtonProgress
                busy={notificationBusy === `revoke-publisher-credential-${githubPublisher?.id}`}
                busyLabel="Revoking token"
                idleIcon={<Trash2 size={16} />}
              >
                Revoke token
              </ButtonProgress>
            </Button>
          </div>

          <div className="border-t border-slate-100 pt-4">
            <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <div className="text-sm font-bold text-slate-900">Dev.to</div>
                  <Badge tone={devToPublisher?.status === "connected" ? "green" : devToPublisher?.status === "error" ? "red" : "amber"}>
                    {devToPublisher?.status ?? "missing"}
                  </Badge>
                  <Badge tone={devToPublisher?.enabled ? "green" : "neutral"}>{devToPublisher?.enabled ? "enabled" : "disabled"}</Badge>
                </div>
                <p className="mt-1 max-w-2xl text-sm leading-5 text-slate-500">
                  Connect a DEV Community API key to verify account access. Dev.to publishing starts in approval mode.
                </p>
              </div>
            </div>

            <ConnectionInstructions
              steps={[
                "Open DEV Settings > Extensions.",
                "Generate a DEV Community API key.",
                "Paste the DEV API key into CiteLoop.",
                "Test the connection.",
                "Choose draft-only or auto publish.",
              ]}
            />

            <div className="mt-4 grid gap-3 lg:grid-cols-2">
              <Field label="DEV username (optional)">
                <TextInput
                  value={devToUsername}
                  onChange={(event) => setDevToUsername(event.target.value)}
                  placeholder="xiaobo_yu_936f1b4512c370f"
                />
                <p className="mt-1 text-xs leading-5 text-slate-500">Username is optional; the API key identifies the DEV account.</p>
              </Field>
              <Field label="Dev.to API key">
                <TextInput
                  type="password"
                  value={devToCredentialDraft}
                  onChange={(event) => setDevToCredentialDraft(event.target.value)}
                  placeholder={devToPublisher?.credential_configured ? "Saved" : "Paste API key"}
                  autoComplete="off"
                />
              </Field>
            </div>

            {devToPublisher?.last_error && <Notice title="Dev.to connection needs attention" detail={devToPublisher.last_error} tone="red" />}

            <div className="mt-4 flex flex-wrap gap-2">
              <Button variant="primary" onClick={saveDevToConnection} disabled={notificationBusy === "save-devto"}>
                <ButtonProgress busy={notificationBusy === "save-devto"} busyLabel="Saving Dev.to" idleIcon={<Save size={16} />}>
                  Save Dev.to
                </ButtonProgress>
              </Button>
              <Button
                variant="outline"
                onClick={saveDevToCredential}
                disabled={!devToPublisher || !devToCredentialDraft.trim() || notificationBusy === `save-devto-credential-${devToPublisher?.id}`}
              >
                <ButtonProgress
                  busy={notificationBusy === `save-devto-credential-${devToPublisher?.id}`}
                  busyLabel="Saving key"
                  idleIcon={<Save size={16} />}
                >
                  Save Dev.to key
                </ButtonProgress>
              </Button>
              <Button
                variant="outline"
                onClick={testDevToConnection}
                disabled={
                  !devToPublisher ||
                  (!devToPublisher?.credential_configured && !devToCredentialDraft.trim()) ||
                  notificationBusy === `test-publisher-${devToPublisher?.id}`
                }
              >
                <ButtonProgress
                  busy={notificationBusy === `test-publisher-${devToPublisher?.id}`}
                  busyLabel={devToCredentialDraft.trim() ? "Saving and testing" : "Testing"}
                  idleIcon={<Send size={16} />}
                >
                  Test Dev.to
                </ButtonProgress>
              </Button>
              {devToPublisher?.enabled ? (
                <Button
                  variant="outline"
                  onClick={() => devToPublisher && setPublisherConnectionEnabled(devToPublisher, false)}
                  disabled={!devToPublisher || notificationBusy === `toggle-publisher-${devToPublisher?.id}`}
                >
                  <ButtonProgress
                    busy={notificationBusy === `toggle-publisher-${devToPublisher?.id}`}
                    busyLabel="Disabling"
                    idleIcon={<Power size={16} />}
                  >
                    Disable Dev.to
                  </ButtonProgress>
                </Button>
              ) : (
                <Button
                  variant="primary"
                  onClick={() => devToPublisher && setPublisherConnectionEnabled(devToPublisher, true)}
                  disabled={!devToPublisher || devToPublisher.status !== "connected" || notificationBusy === `toggle-publisher-${devToPublisher?.id}`}
                >
                  <ButtonProgress
                    busy={notificationBusy === `toggle-publisher-${devToPublisher?.id}`}
                    busyLabel="Enabling"
                    idleIcon={<Power size={16} />}
                  >
                    Enable Dev.to
                  </ButtonProgress>
                </Button>
              )}
              <Button
                variant="outline"
                onClick={revokeDevToCredential}
                disabled={!devToPublisher?.credential_configured || notificationBusy === `revoke-devto-credential-${devToPublisher?.id}`}
              >
                <ButtonProgress
                  busy={notificationBusy === `revoke-devto-credential-${devToPublisher?.id}`}
                  busyLabel="Revoking key"
                  idleIcon={<Trash2 size={16} />}
                >
                  Revoke Dev.to key
                </ButtonProgress>
              </Button>
            </div>
          </div>

          <div className="border-t border-slate-100 pt-4">
            <div className="text-sm font-bold text-slate-900">Distribution platform instructions</div>
            <p className="mt-1 max-w-2xl text-sm leading-5 text-slate-500">
              These platforms stay copy or submit draft only until their connector is ready.
            </p>
            <div className="mt-3 grid gap-3 lg:grid-cols-2">
              {distributionConnectionGuides.map((guide) => (
                <div key={guide.name} className="rounded-lg border border-slate-100 bg-slate-50 px-3 py-3">
                  <div className="flex items-center justify-between gap-3">
                    <div className="text-sm font-semibold text-slate-900">{guide.name}</div>
                    <Badge tone="neutral">{guide.state}</Badge>
                  </div>
                  <ConnectionInstructions steps={guide.steps} />
                </div>
              ))}
            </div>
          </div>
        </div>
      </section>
      )}

      {activeSettingsTab === "ai-assistance" && (
      <section id="settings-panel-ai-assistance" role="tabpanel" aria-labelledby="settings-tab-ai-assistance" tabIndex={0} className="space-y-4">
        <SectionHeader title="AI assistance" eyebrow="Independent execution authority" />

        <div className="flex flex-col gap-3 rounded-xl border border-slate-200 bg-white px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="text-sm font-bold text-slate-950">
              AI assistance: Doctor {doctorAIStatus} · Opportunities {growthAIStatus}
            </div>
            <p className="mt-1 max-w-3xl text-sm leading-5 text-slate-600">
              Each line has separate consent. Turning one off does not change the other line.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Badge tone={config.doctor_ai_enabled ? "green" : "neutral"}>Doctor {doctorAIStatus}</Badge>
            <Badge tone={config.growth_ai_enabled ? "green" : "neutral"}>Opportunities {growthAIStatus}</Badge>
          </div>
        </div>

        <Notice
          tone="amber"
          title="Provider use and cost"
          detail={`Doctor and Opportunities can use the same shared provider credential, but that credential is not execution authority. Enabled calls consume provider tokens and count toward your $${config.monthly_budget_usd.toFixed(2)} monthly project budget.`}
        />

        <div className="grid gap-4 lg:grid-cols-[minmax(0,0.92fr)_minmax(0,1.08fr)]">
          <div className="grid content-start gap-4 rounded-xl border border-slate-200 bg-white p-4">
            <div className="flex items-start justify-between gap-4">
              <div>
                <div className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-500">Doctor</div>
                <h3 className="mt-1 text-base font-bold text-slate-950">AI-assisted diagnosis and Site Fixes</h3>
                <p className="mt-1 text-sm leading-5 text-slate-600">
                  Authorizes AI calls only for Doctor diagnosis, fix preparation, and verification.
                </p>
              </div>
              <button
                type="button"
                role="switch"
                aria-label="AI assistance for Doctor"
                aria-checked={config.doctor_ai_enabled}
                onClick={() => update({ doctor_ai_enabled: !config.doctor_ai_enabled })}
                className={cx(
                  "relative h-7 w-12 shrink-0 rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#d93820] focus-visible:ring-offset-2 active:scale-[0.98]",
                  config.doctor_ai_enabled ? "bg-[#d93820]" : "bg-slate-300",
                )}
              >
                <span className={cx("absolute top-1 h-5 w-5 rounded-full bg-white shadow-sm transition-transform", config.doctor_ai_enabled ? "translate-x-6" : "translate-x-1")} />
              </button>
            </div>

            <Field label="Doctor AI run policy" helper="Choose or confirm this policy before enabling Doctor AI.">
              <div className="grid gap-2">
                {doctorAIRunPolicies.map((policyOption) => {
                  const active = config.doctor_ai_run_policy === policyOption.value;
                  return (
                    <button
                      key={policyOption.value}
                      type="button"
                      disabled={!config.doctor_ai_enabled}
                      onClick={() => update({ doctor_ai_run_policy: policyOption.value })}
                      className={cx(
                        "rounded-lg border px-3 py-3 text-left transition-colors active:scale-[0.99] disabled:cursor-not-allowed disabled:opacity-50",
                        active ? "border-[#d93820] bg-red-50" : "border-slate-200 hover:border-slate-300",
                      )}
                    >
                      <span className="flex items-center justify-between gap-2 text-sm font-bold text-slate-900">
                        {policyOption.label}
                        {active && <CheckCircle2 size={16} className="text-[#d93820]" />}
                      </span>
                      <span className="mt-1 block text-sm font-semibold text-slate-700">{policyOption.summary}</span>
                      <span className="mt-1 block text-sm leading-5 text-slate-500">{policyOption.detail}</span>
                    </button>
                  );
                })}
              </div>
            </Field>

            <Button variant="primary" onClick={saveDoctorAIAuthority} disabled={aiAuthorityBusy !== null}>
              <ButtonProgress busy={aiAuthorityBusy === "doctor"} busyLabel="Saving Doctor authority" idleIcon={<Save size={16} />}>
                Save Doctor AI settings
              </ButtonProgress>
            </Button>
          </div>

          <div className="grid content-start gap-4 rounded-xl border border-slate-200 bg-white p-4">
            <div className="flex items-start justify-between gap-4">
              <div>
                <div className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-500">Opportunities</div>
                <h3 className="mt-1 text-base font-bold text-slate-950">AI-assisted closed-loop growth</h3>
                <p className="mt-1 text-sm leading-5 text-slate-600">
                  Authorizes AI calls only for growth discovery, content generation, measurement, and learning.
                </p>
              </div>
              <button
                type="button"
                role="switch"
                aria-label="AI assistance for Opportunities"
                aria-checked={config.growth_ai_enabled}
                onClick={() => update({ growth_ai_enabled: !config.growth_ai_enabled })}
                className={cx(
                  "relative h-7 w-12 shrink-0 rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#d93820] focus-visible:ring-offset-2 active:scale-[0.98]",
                  config.growth_ai_enabled ? "bg-[#d93820]" : "bg-slate-300",
                )}
              >
                <span className={cx("absolute top-1 h-5 w-5 rounded-full bg-white shadow-sm transition-transform", config.growth_ai_enabled ? "translate-x-6" : "translate-x-1")} />
              </button>
            </div>

            <Field label="Opportunities AI run policy" helper="Event-triggered calls require the Automatic policy to be explicitly selected and saved.">
              <div className="grid gap-2 sm:grid-cols-2">
                {growthAIRunPolicies.map((policyOption) => {
                  const active = config.growth_ai_run_policy === policyOption.value;
                  return (
                    <button
                      key={policyOption.value}
                      type="button"
                      disabled={!config.growth_ai_enabled}
                      onClick={() => update({ growth_ai_run_policy: policyOption.value })}
                      className={cx(
                        "rounded-lg border px-3 py-3 text-left transition-colors active:scale-[0.99] disabled:cursor-not-allowed disabled:opacity-50",
                        active ? "border-[#d93820] bg-red-50" : "border-slate-200 hover:border-slate-300",
                      )}
                    >
                      <span className="flex items-center justify-between gap-2 text-sm font-bold text-slate-900">
                        {policyOption.label}
                        {active && <CheckCircle2 size={16} className="text-[#d93820]" />}
                      </span>
                      <span className="mt-1 block text-sm font-semibold text-slate-700">{policyOption.summary}</span>
                      <span className="mt-1 block text-sm leading-5 text-slate-500">{policyOption.detail}</span>
                    </button>
                  );
                })}
              </div>
            </Field>

            <Button variant="primary" onClick={saveGrowthAIAuthority} disabled={aiAuthorityBusy !== null}>
              <ButtonProgress busy={aiAuthorityBusy === "growth"} busyLabel="Saving Opportunities authority" idleIcon={<Save size={16} />}>
                Save Opportunities AI settings
              </ButtonProgress>
            </Button>
          </div>
        </div>
      </section>
      )}

      {activeSettingsTab === "crawl" && (
      <section id="settings-panel-crawl" role="tabpanel" aria-labelledby="settings-tab-crawl" tabIndex={0}>
        <SectionHeader title="Crawl config" />
        <div className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
          <div className="grid gap-4 md:grid-cols-3">
            <Field label="Max pages">
              <TextInput
                inputMode="numeric"
                value={config.crawl.max_pages}
                onChange={(event) =>
                  update({ crawl: { ...config.crawl, max_pages: Math.max(1, toInt(event.target.value, 200)) } })
                }
              />
            </Field>
            <Field label="Max depth">
              <TextInput
                inputMode="numeric"
                value={config.crawl.max_depth}
                onChange={(event) =>
                  update({ crawl: { ...config.crawl, max_depth: Math.max(1, toInt(event.target.value, 3)) } })
                }
              />
            </Field>
            <Field label="Request timeout ms">
              <TextInput
                inputMode="numeric"
                value={config.crawl.request_timeout_ms}
                onChange={(event) =>
                  update({
                    crawl: { ...config.crawl, request_timeout_ms: Math.max(1000, toInt(event.target.value, 8000)) },
                  })
                }
              />
            </Field>
            <Field label="Rate limit RPS">
              <TextInput
                inputMode="numeric"
                value={config.crawl.rate_limit_rps}
                onChange={(event) =>
                  update({ crawl: { ...config.crawl, rate_limit_rps: Math.max(1, toInt(event.target.value, 1)) } })
                }
              />
            </Field>
            <Field label="Sitemap URL cap">
              <TextInput
                inputMode="numeric"
                value={config.crawl.sitemap_url_cap}
                onChange={(event) =>
                  update({ crawl: { ...config.crawl, sitemap_url_cap: Math.max(1, toInt(event.target.value, 2000)) } })
                }
              />
            </Field>
          </div>

          <div className="flex flex-wrap gap-4 text-sm font-semibold text-slate-700">
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={config.crawl.same_origin_only}
                onChange={(event) => update({ crawl: { ...config.crawl, same_origin_only: event.target.checked } })}
              />
              Same origin only
            </label>
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={config.crawl.respect_robots}
                onChange={(event) => update({ crawl: { ...config.crawl, respect_robots: event.target.checked } })}
              />
              Respect robots
            </label>
          </div>
        </div>
      </section>
      )}

      {activeSettingsTab === "notifications" && (
      <section id="settings-panel-notifications" role="tabpanel" aria-labelledby="settings-tab-notifications" tabIndex={0} className="space-y-7">
        <div id="notifications">
          <SectionHeader
            title="Account channels"
            eyebrow="Operations"
            action={
              <Button size="sm" onClick={refreshNotifications} disabled={!!notificationBusy}>
                <RefreshCw size={14} />
                Refresh
              </Button>
            }
          />

        <div className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
          <div className="rounded-lg border border-slate-200 bg-slate-50 px-4 py-3">
            <div className="text-sm font-bold text-slate-950">Set notifications</div>
            <p className="mt-1 max-w-2xl text-sm leading-5 text-slate-600">
              Automation needs a notification channel. Failures, approval requests, safe mode alerts, and delivery problems should reach an operator.
              Add Slack, Discord, or Email once, then reuse it across this account's projects.
            </p>
          </div>

          <div className="flex items-center gap-2 text-sm font-semibold text-slate-900">
            <Bell size={16} />
            Project subscriptions
          </div>

          <div className="grid gap-3 lg:grid-cols-[260px_1fr_1fr_auto]">
            <div className="grid grid-cols-3 gap-2">
              {channelKinds.map((kind) => {
                const active = channelDraft.kind === kind.value;
                return (
                  <button
                    type="button"
                    key={kind.value}
                    onClick={() => setChannelDraft((current) => ({ ...current, kind: kind.value }))}
                    className={cx(
                      "h-10 rounded-lg border text-sm font-semibold transition-colors",
                      active ? "border-[#d93820] bg-red-50 text-[#b92f1c]" : "border-slate-200 text-slate-600 hover:bg-slate-50",
                    )}
                  >
                    {kind.label}
                  </button>
                );
              })}
            </div>
            <TextInput
              value={channelDraft.label}
              onChange={(event) => setChannelDraft((current) => ({ ...current, label: event.target.value }))}
              placeholder="Label"
            />
            <TextInput
              value={channelDraft.destination}
              onChange={(event) => setChannelDraft((current) => ({ ...current, destination: event.target.value }))}
              placeholder={channelDraft.kind === "email" ? "Email address" : "Webhook URL"}
              type={channelDraft.kind === "email" ? "email" : "password"}
              autoComplete="off"
            />
            <Button variant="primary" onClick={createChannel} disabled={notificationBusy === "create-channel"}>
              <ButtonProgress busy={notificationBusy === "create-channel"} busyLabel="Adding channel" idleIcon={<Plus size={16} />}>
                Add
              </ButtonProgress>
            </Button>
          </div>

          {channels.length === 0 ? (
            <div className="rounded-lg border border-dashed border-slate-300 bg-white px-4 py-5 text-sm">
              <div className="font-bold text-slate-900">Automation needs a notification channel</div>
              <p className="mt-1 max-w-2xl leading-5 text-slate-500">
                Failures, approval requests, safe mode alerts, and delivery problems should reach an operator.
                Add Slack, Discord, or Email once, then reuse it across this account's projects.
              </p>
            </div>
          ) : (
            <div className="overflow-x-auto rounded-lg border border-slate-200">
              <table className="min-w-full divide-y divide-slate-200 text-sm">
                <thead className="bg-slate-50 text-left text-xs font-semibold uppercase text-slate-500">
                  <tr>
                    <th className="px-3 py-2">Label</th>
                    <th className="px-3 py-2">Kind</th>
                    <th className="px-3 py-2">Destination</th>
                    <th className="px-3 py-2">Status</th>
                    <th className="px-3 py-2">Used by</th>
                    <th className="px-3 py-2 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {channels.map((channel) => (
                    <tr key={channel.id}>
                      <td className="px-3 py-2 font-semibold text-slate-900">{channelDisplayLabel(channel)}</td>
                      <td className="px-3 py-2">
                        <Badge tone={channelKindTone(channel)}>{channelKindLabel(channel)}</Badge>
                      </td>
                      <td className="max-w-[320px] truncate px-3 py-2 text-slate-500">{channelDestination(channel)}</td>
                      <td className="px-3 py-2">
                        <Badge tone={channel.verified_at ? "green" : "amber"}>
                          {channel.verified_at ? "Test accepted" : "Untested"}
                        </Badge>
                      </td>
                      <td className="px-3 py-2 text-slate-500">{channelUsageLabel(channel)}</td>
                      <td className="px-3 py-2 text-right">
                        <div className="flex justify-end gap-2">
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() => openChannelEvents(channel)}
                            disabled={notificationBusy === `events-${channel.id}`}
                            title="Choose subscribed events"
                          >
                            <ListChecks size={14} />
                            Events
                          </Button>
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() => testChannel(channel)}
                            disabled={notificationBusy === `test-${channel.id}`}
                            title={channel.verified_at ? `Test accepted ${formatDate(channel.verified_at)}` : channel.kind === "email" ? "Send test email" : "Send test notification"}
                          >
                            <ButtonProgress busy={notificationBusy === `test-${channel.id}`} busyLabel="Testing" idleIcon={<Send size={14} />} />
                          </Button>
                          <Button
                            size="sm"
                            variant="danger"
                            onClick={() => deleteChannel(channel)}
                            disabled={notificationBusy === `delete-${channel.id}`}
                            title="Delete channel"
                          >
                            <ButtonProgress busy={notificationBusy === `delete-${channel.id}`} busyLabel="Deleting" idleIcon={<Trash2 size={14} />} />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
        </div>

        <div>
          <SectionHeader
            title="Deliveries"
            action={
              <div className="flex gap-1 rounded-lg border border-slate-200 bg-white p-1">
                {deliveryStatuses.map((status) => (
                  <button
                    type="button"
                    key={status.value}
                    onClick={() => setDeliveryStatus(status.value)}
                    className={cx(
                      "h-7 rounded-md px-2 text-xs font-semibold transition-colors",
                      deliveryStatus === status.value ? "bg-slate-900 text-white" : "text-slate-600 hover:bg-slate-100",
                    )}
                  >
                    {status.label}
                  </button>
                ))}
              </div>
            }
          />
          <div className="overflow-x-auto rounded-xl border border-slate-200 bg-white">
            {deliveries.length === 0 ? (
              <div className="px-4 py-5 text-sm font-semibold text-slate-500">No deliveries</div>
            ) : (
              <table className="min-w-full divide-y divide-slate-200 text-sm">
                <thead className="bg-slate-50 text-left text-xs font-semibold uppercase text-slate-500">
                  <tr>
                    <th className="px-3 py-2">Event</th>
                    <th className="px-3 py-2">Status</th>
                    <th className="px-3 py-2">Attempts</th>
                    <th className="px-3 py-2">Last error</th>
                    <th className="px-3 py-2">Created</th>
                    <th className="px-3 py-2 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {deliveries.map((delivery) => (
                    <tr key={delivery.id}>
                      <td className="px-3 py-2">
                        <div className="font-semibold text-slate-900">{eventLabels[delivery.event_type] ?? delivery.event_type}</div>
                        <div className="max-w-[260px] truncate font-mono text-xs text-slate-500">{delivery.event_id}</div>
                      </td>
                      <td className="px-3 py-2">
                        <Badge tone={delivery.status === "sent" ? "green" : delivery.status === "dead" ? "red" : "amber"}>
                          {delivery.status}
                        </Badge>
                      </td>
                      <td className="px-3 py-2 text-slate-600">{delivery.attempts ?? 0}</td>
                      <td className="max-w-[360px] truncate px-3 py-2 text-slate-500">{delivery.last_error ?? "-"}</td>
                      <td className="px-3 py-2 text-slate-500">{formatDate(delivery.created_at)}</td>
                      <td className="px-3 py-2 text-right">
                        <Button
                          size="sm"
                          onClick={() => retryDelivery(delivery.id)}
                          disabled={delivery.status === "sent" || notificationBusy === `retry-${delivery.id}`}
                          title="Retry delivery"
                        >
                          <ButtonProgress busy={notificationBusy === `retry-${delivery.id}`} busyLabel="Retrying" idleIcon={<RotateCcw size={14} />} />
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
        {activeEventsChannel && (
          <div className="fixed inset-0 z-40 flex items-center justify-center bg-slate-950/35 px-4 py-6">
            <div
              role="dialog"
              aria-modal="true"
              aria-labelledby="notification-events-title"
              className="flex max-h-[88vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-slate-200 bg-white shadow-xl"
            >
              <div className="border-b border-slate-200 px-5 py-4">
                <div className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">Subscriptions</div>
                <h3 id="notification-events-title" className="mt-1 text-lg font-bold text-slate-950">
                  Events for {channelDisplayLabel(activeEventsChannel)}
                </h3>
                <p className="mt-1 text-sm leading-5 text-slate-500">
                  Choose which events from this project send to this account channel.
                </p>
              </div>

              <div className="grid gap-2 overflow-y-auto px-5 py-4">
                {events.length === 0 ? (
                  <div className="rounded-lg border border-dashed border-slate-200 px-4 py-5 text-sm font-semibold text-slate-500">
                    No eligible events
                  </div>
                ) : (
                  events.map((event) => {
                    const checked = Boolean(eventSelection[event.type]);
                    return (
                      <label
                        key={`${activeEventsChannel.id}-${event.type}`}
                        className={cx(
                          "flex items-start gap-3 rounded-lg border px-3 py-3 text-sm transition-colors",
                          checked ? "border-green-200 bg-green-50" : "border-slate-200 bg-white hover:bg-slate-50",
                        )}
                      >
                        <input
                          type="checkbox"
                          className="mt-1 h-4 w-4"
                          checked={checked}
                          disabled={activeEventsBusy}
                          onChange={(changeEvent) =>
                            setEventSelection((current) => ({ ...current, [event.type]: changeEvent.target.checked }))
                          }
                        />
                        <span className="min-w-0">
                          <span className="block font-bold text-slate-900">{eventLabels[event.type] ?? event.type}</span>
                          <span className="mt-1 block truncate font-mono text-xs text-slate-500">{event.type}</span>
                        </span>
                      </label>
                    );
                  })
                )}
              </div>

              <div className="flex justify-end gap-2 border-t border-slate-200 bg-slate-50 px-5 py-4">
                <Button variant="ghost" onClick={closeChannelEvents} disabled={activeEventsBusy}>
                  Cancel
                </Button>
                <Button variant="primary" onClick={saveChannelEvents} disabled={activeEventsBusy || events.length === 0}>
                  <ButtonProgress busy={activeEventsBusy} busyLabel="Saving events" idleIcon={<Save size={16} />}>
                    Save
                  </ButtonProgress>
                </Button>
              </div>
            </div>
          </div>
        )}
      </section>
      )}
    </div>
  );
}
