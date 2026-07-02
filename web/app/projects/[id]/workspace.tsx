"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertTriangle, ArrowRight, BarChart3, CheckCircle2, Circle, FileText, Loader2, Search, Sparkles } from "lucide-react";
import {
  Article,
  DistributeItem,
  GenerationRun,
  InventoryItem,
  ProductProfile,
  Project,
  ReviewGroup,
  SEOOpportunity,
  SEOOverview,
  Topic,
  VisibilitySummary,
  friendlyApiError,
} from "../../lib/api";
import {
  buildHomeEventStream,
  contextInventoryProgress,
  contextBuildTracks,
  homeAICitationMetric,
  homeInMotionMetric,
  nextWorkspaceAction,
} from "../../lib/dashboard-ux-logic";
import { normalizeNumeric } from "../../lib/normalize";
import { visibilityLifecycleCounts } from "../../lib/visibility-lifecycle";
import { useApi } from "../../lib/use-api";
import { useToast } from "../../components/toast-provider";
import { Badge, EmptyState, Notice, SectionHeader, cx, formatDate, formatScore } from "../../components/ui";

type Message = { tone: "neutral" | "red" | "green" | "amber"; title: string; detail?: string } | null;

const ONBOARDING_POLL_LIMIT = 18;
const ONBOARDING_POLL_MS = 8000;
const HOME_REFRESH_MS = 5000;

function articleTitle(article: Article) {
  return article.seo_meta?.title || article.seo_meta?.slug || `${article.kind} draft`;
}

function isThisMonth(value: string | null) {
  if (!value) return false;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return false;
  const now = new Date();
  return date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth();
}

function opportunityTitle(opportunity: SEOOpportunity) {
  return opportunity.recommended_action || opportunity.query || opportunity.page_url || opportunity.type || "Opportunity";
}

function metric(value: any, digits = 0) {
  const n = normalizeNumeric(value);
  if (n == null) return "-";
  return n.toLocaleString("en", { maximumFractionDigits: digits, minimumFractionDigits: digits });
}

function sumCounts(rows: Array<{ status: string; count: number }> | undefined, statuses: string[]) {
  if (!rows) return 0;
  return rows
    .filter((row) => statuses.includes(row.status))
    .reduce((total, row) => total + row.count, 0);
}

function hasConnectedSearchData(overview: SEOOverview | null) {
  if (!overview) return false;
  if (overview.capability_mode === "managed_content_connected" || overview.capability_mode === "customer_site_connected") return true;
  return overview.integrations.some((integration) => integration.provider === "google_search_console" && integration.status === "connected");
}

type StageTone = "green" | "amber" | "blue" | "red" | "neutral";

function stageDotClass(tone: StageTone) {
  const classes: Record<StageTone, string> = {
    green: "bg-emerald-500",
    amber: "bg-amber-500",
    blue: "bg-blue-500",
    red: "bg-red-500",
    neutral: "bg-slate-300",
  };
  return classes[tone];
}

function metricChangeClass(tone: StageTone) {
  const classes: Record<StageTone, string> = {
    green: "text-emerald-700 bg-emerald-50 ring-emerald-100",
    amber: "text-amber-700 bg-amber-50 ring-amber-100",
    blue: "text-blue-700 bg-blue-50 ring-blue-100",
    red: "text-[#d93820] bg-red-50 ring-red-100",
    neutral: "text-slate-600 bg-slate-50 ring-slate-100",
  };
  return classes[tone];
}

function compactMetricCardClass(featured: boolean) {
  return cx(
    "group flex min-h-[124px] flex-col rounded-xl border border-slate-200 bg-white p-4 transition-colors hover:border-slate-300 hover:bg-slate-50 lg:min-h-[148px]",
    featured && "lg:col-span-1",
  );
}

type HumanActionCategory = "Blocking now" | "Needs review" | "Improves results" | "Warnings";

type HumanActionItem = {
  id: string;
  title: string;
  detail: string;
  href: string;
  cta: string;
  category: HumanActionCategory;
  tone: StageTone;
  priority: number;
  count?: number;
};

const VISIBLE_HUMAN_ACTION_LIMIT = 4;

function humanActionTileToneClass(tone: StageTone) {
  const classes: Record<StageTone, string> = {
    green: "border-l-4 border-slate-200 border-l-green-500 bg-green-50/40 hover:border-green-300 hover:border-l-green-500",
    amber: "border-l-4 border-slate-200 border-l-amber-500 bg-amber-50/50 hover:border-amber-300 hover:border-l-amber-500",
    blue: "border-l-4 border-slate-200 border-l-sky-500 bg-sky-50/45 hover:border-sky-300 hover:border-l-sky-500",
    red: "border-l-4 border-slate-200 border-l-[#d93820] bg-red-50/55 hover:border-red-300 hover:border-l-[#d93820]",
    neutral: "border-l-4 border-slate-200 border-l-slate-300 bg-white hover:border-slate-300 hover:border-l-slate-400",
  };
  return classes[tone];
}

function compactActionTileClass(tone: StageTone) {
  return cx(
    "group flex min-h-[116px] flex-col justify-between overflow-hidden rounded-xl border border-slate-200 p-3 shadow-[0_14px_30px_-26px_rgba(15,23,42,0.45)] transition-all duration-200 hover:-translate-y-0.5 active:translate-y-0 lg:min-h-[118px]",
    humanActionTileToneClass(tone),
  );
}

function humanActionIconToneClass(tone: StageTone) {
  const classes: Record<StageTone, string> = {
    green: "bg-green-100 text-green-700 ring-green-200",
    amber: "bg-amber-100 text-amber-800 ring-amber-200",
    blue: "bg-sky-100 text-sky-700 ring-sky-200",
    red: "bg-red-100 text-[#d93820] ring-red-200",
    neutral: "bg-slate-100 text-slate-600 ring-slate-200",
  };
  return classes[tone];
}

function humanActionIcon(item: HumanActionItem) {
  if (item.id.includes("context")) return CheckCircle2;
  if (item.id.includes("analysis")) return Sparkles;
  if (item.id.includes("gsc")) return Search;
  if (item.id === "publishing-failed") return AlertTriangle;
  if (item.id.includes("distribute")) return ArrowRight;
  if (item.id.includes("draft")) return FileText;
  return AlertTriangle;
}

export function Workspace({ projectId }: { projectId: string }) {
  const api = useApi();
  const [project, setProject] = useState<Project | null>(null);
  const [profile, setProfile] = useState<ProductProfile | null>(null);
  const [inventory, setInventory] = useState<InventoryItem[]>([]);
  const [topics, setTopics] = useState<Topic[]>([]);
  const [review, setReview] = useState<ReviewGroup[]>([]);
  const [published, setPublished] = useState<Article[]>([]);
  const [approved, setApproved] = useState<Article[]>([]);
  const [failedPublish, setFailedPublish] = useState<Article[]>([]);
  const [ready, setReady] = useState<DistributeItem[]>([]);
  const [runs, setRuns] = useState<GenerationRun[]>([]);
  const [insightRuns, setInsightRuns] = useState<GenerationRun[]>([]);
  const [seoOverview, setSeoOverview] = useState<SEOOverview | null>(null);
  const [visibilitySummary, setVisibilitySummary] = useState<VisibilitySummary | null>(null);
  const [accountProjects, setAccountProjects] = useState<Project[]>([]);
  const { notify } = useToast();
  const setMessage = (next: Message) => {
    if (next) notify(next);
  };
  const [apiError, setApiError] = useState<string | null>(null);
  const [onboardingPollCount, setOnboardingPollCount] = useState(0);

  const refresh = useCallback(async () => {
    setApiError(null);
    try {
      const [p, profileRow, inventoryRows, t, r, pub, app, failed, dist, runRows, insightRunRows, overview, summary, projectRows] = await Promise.all([
        api.getProject(projectId),
        api.getProfile(projectId).catch(() => null),
        api.listInventory(projectId).catch(() => []),
        api.listTopics(projectId),
        api.listReview(projectId),
        api.listArticles(projectId, "published"),
        api.listArticles(projectId, "approved"),
        api.listArticles(projectId, "publish_failed"),
        api.listDistribute(projectId),
        api.listRuns(projectId, { limit: 5 }),
        api.listRuns(projectId, { agent: "insight", limit: 50 }).catch(() => []),
        api.getSEOOverview(projectId).catch(() => null),
        api.getVisibilitySummary(projectId).catch(() => null),
        api.listProjects().catch(() => []),
      ]);
      setProject(p);
      setProfile(profileRow);
      setInventory(inventoryRows);
      setTopics(t);
      setReview(r);
      setPublished(pub);
      setApproved(app);
      setFailedPublish(failed);
      setReady(dist);
      setRuns(runRows);
      setInsightRuns(insightRunRows);
      setSeoOverview(overview);
      setVisibilitySummary(summary);
      setAccountProjects(projectRows);
      return { profile: profileRow, inventory: inventoryRows, insightRuns: insightRunRows };
    } catch (e: any) {
      setApiError(friendlyApiError(e));
      return null;
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  useEffect(() => {
    const interval = window.setInterval(refresh, HOME_REFRESH_MS);
    const refreshWhenVisible = () => {
      if (document.visibilityState === "visible") void refresh();
    };
    const refreshOnPageShow = () => {
      void refresh();
    };
    window.addEventListener("focus", refresh);
    window.addEventListener("pageshow", refreshOnPageShow);
    document.addEventListener("visibilitychange", refreshWhenVisible);
    return () => {
      window.clearInterval(interval);
      window.removeEventListener("focus", refresh);
      window.removeEventListener("pageshow", refreshOnPageShow);
      document.removeEventListener("visibilitychange", refreshWhenVisible);
    };
  }, [refresh]);

  const contextInventory = contextInventoryProgress(inventory);
  const contextEvidenceCount = contextInventory.evidenceCount;
  const contextEvidencePageCount = contextInventory.evidencePageCount;
  const sourcePageCount =
    contextInventory.sourcePageCount > 0 ? contextInventory.sourcePageCount : Math.max(inventory.length, profile?.source_urls?.length ?? 0);
  const contextConfirmed = Boolean(profile?.profile?.context_confirmed_at || profile?.profile?.confirmed_at);
  const contextNeedsConfirmation = Boolean(profile) && sourcePageCount > 0 && contextEvidenceCount > 0 && !contextConfirmed;
  const contextBuild = contextBuildTracks({
    hasProfile: Boolean(profile),
    sourcePageCount,
    evidencePageCount: contextEvidencePageCount,
    evidenceCount: contextEvidenceCount,
    pollCount: onboardingPollCount,
    pollLimit: ONBOARDING_POLL_LIMIT,
    runs: insightRuns,
  });
  const projectLoaded = Boolean(project);
  const showContextBuild = projectLoaded && contextBuild.active;

  // After project creation, onboarding (crawl + product profile) runs in the background.
  // Home keeps checking the profile and inventory so fresh projects do not strand users
  // on an empty Context page while the detached onboarding jobs finish.
  const onboardingAttemptsRef = useRef(0);
  useEffect(() => {
    if (!showContextBuild) {
      onboardingAttemptsRef.current = 0;
      setOnboardingPollCount(0);
      return;
    }
    onboardingAttemptsRef.current = 0;
    setOnboardingPollCount(0);
    let cancelled = false;
    const interval = window.setInterval(async () => {
      onboardingAttemptsRef.current += 1;
      const attempt = onboardingAttemptsRef.current;
      if (!cancelled) setOnboardingPollCount(attempt);
      try {
        const next = await refresh();
        if (cancelled || !next) return;
        const nextContextInventory = contextInventoryProgress(next.inventory);
        const nextEvidenceCount = nextContextInventory.evidenceCount;
        const nextEvidencePageCount = nextContextInventory.evidencePageCount;
        const nextSourcePageCount =
          nextContextInventory.sourcePageCount > 0 ? nextContextInventory.sourcePageCount : Math.max(next.inventory.length, next.profile?.source_urls?.length ?? 0);
        const nextContextBuild = contextBuildTracks({
          hasProfile: Boolean(next.profile),
          sourcePageCount: nextSourcePageCount,
          evidencePageCount: nextEvidencePageCount,
          evidenceCount: nextEvidenceCount,
          pollCount: attempt,
          pollLimit: ONBOARDING_POLL_LIMIT,
          runs: next.insightRuns,
        });
        if (!nextContextBuild.active) {
          setMessage({
            tone: "green",
            title: "Your domain context is ready",
            detail: "CiteLoop finished reading your site. Confirm the context, then planning and drafting advance automatically.",
          });
          window.clearInterval(interval);
          return;
        }
      } catch {
        // ignore transient errors and keep polling until the cap
      }
      if (attempt >= ONBOARDING_POLL_LIMIT && !cancelled) {
        window.clearInterval(interval);
      }
    }, ONBOARDING_POLL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, [showContextBuild, refresh]);

  const reviewArticles = review.flatMap((group) => group.articles);
  const scheduledRows = useMemo(() => {
    const articleRows = approved
      .filter((article) => article.kind === "canonical")
      .map((article) => ({ id: article.id, time: article.scheduled_at, title: articleTitle(article) }));
    return [...articleRows].sort((a, b) => String(a.time).localeCompare(String(b.time))).slice(0, 5);
  }, [approved]);

  const automationWarnings = runs.filter((run) => ["error", "failed"].includes(run.status) || Boolean(run.output?.degraded));
  const blockedDraftCount = reviewArticles.filter((article) => article.qa_blocking || article.requires_human_decision).length;
  const reviewDraftCount = reviewArticles.length - blockedDraftCount;
  const hasBlockedDrafts = blockedDraftCount > 0;

  const visibilityOpenOpportunities = visibilitySummary?.open_opportunities ?? [];
  const visibilityActionsInLoop = visibilitySummary?.actions_in_loop ?? [];
  const visibilityLifecycleCountsForSummary = visibilitySummary?.lifecycle_counts ?? visibilityLifecycleCounts();
  const visibilityOpenOpportunityCount = visibilityOpenOpportunities.length;
  const visibilityActionsInLoopCount = visibilityActionsInLoop.length;
  const visibilityPlanHandoffCount = (visibilityLifecycleCountsForSummary.added_to_plan ?? 0) + (visibilityLifecycleCountsForSummary.planned ?? 0);
  const opportunitiesInPlanCount = visibilityPlanHandoffCount;
  const planItemCount = topics.length + opportunitiesInPlanCount;
  const planGenerationPending = visibilityPlanHandoffCount > 0 && topics.length === 0;
  const publishedThisMonth = published.filter((article) => isThisMonth(article.published_at)).length;
  const searchDataConnected = hasConnectedSearchData(seoOverview);
  const clicks28d = normalizeNumeric(seoOverview?.last_28_days?.clicks_28d ?? null);
  const impressions28d = normalizeNumeric(seoOverview?.last_28_days?.impressions_28d ?? null);
  const measuringActions = visibilitySummary
    ? (visibilityLifecycleCountsForSummary.published_or_applied ?? 0) + (visibilityLifecycleCountsForSummary.measuring ?? 0) + (visibilityLifecycleCountsForSummary.learned ?? 0)
    : sumCounts(seoOverview?.actions_by_status, ["published", "measuring", "completed"]);
  const visibilityCitationSignalCount = visibilityOpenOpportunities.filter((opportunity) =>
    `${opportunity.type} ${opportunity.recommended_action ?? ""} ${opportunity.expected_impact ?? ""}`.toLowerCase().match(/ai|llm|citation|answer/),
  ).length;
  const highestPriorityOpportunity = [...visibilityOpenOpportunities].sort((a, b) => {
    return (normalizeNumeric(b.priority_score) ?? 0) - (normalizeNumeric(a.priority_score) ?? 0);
  })[0] ?? null;
  const measurementResultNeedsAttention =
    visibilitySummary?.top_measurement_updates.find((update) => update.status === "blocked" || update.status === "learned" || update.summary) ?? null;

  const contextBuildNeedsAttention = showContextBuild && contextBuild.tracks.some((track) => track.state === "attention");
  const fallbackAction = nextWorkspaceAction({
    projectId,
    hasProfile: Boolean(profile),
    contextConfirmed,
    failedPublishCount: failedPublish.length,
    hasBlockedDrafts,
    reviewCount: reviewArticles.length,
    readyCount: ready.length,
    topicsCount: topics.length,
    openOpportunityCount: visibilityOpenOpportunityCount,
  });
  const humanActionCandidates: Array<HumanActionItem | false> = [
    contextNeedsConfirmation && {
      id: "confirm-context",
      title: "Confirm Context",
      detail: "CiteLoop needs your product facts, evidence, and positioning confirmed before it plans content.",
      href: `/projects/${projectId}/context`,
      cta: "Review context",
      category: "Blocking now",
      tone: "amber",
      priority: 10,
    },
    !contextNeedsConfirmation && contextBuildNeedsAttention && {
      id: "context-build-attention",
      title: "Check Context build",
      detail: "One of the Context tracks needs attention before the loop can trust this project's source evidence.",
      href: `/projects/${projectId}/context`,
      cta: "Open Context",
      category: "Blocking now",
      tone: "amber",
      priority: 15,
    },
    !contextNeedsConfirmation && Boolean(profile) && !contextConfirmed && {
      id: "complete-context",
      title: "Confirm Context",
      detail: "The project has Context data, but the human confirmation gate is still open.",
      href: `/projects/${projectId}/context`,
      cta: "Review context",
      category: "Blocking now",
      tone: "amber",
      priority: 20,
    },
    failedPublish.length > 0 && {
      id: "publishing-failed",
      title: "Fix publishing",
      detail: `${failedPublish.length} ${failedPublish.length === 1 ? "article needs" : "articles need"} publishing attention before distribution continues.`,
      href: `/projects/${projectId}/publish`,
      cta: "Open Publish",
      category: "Blocking now",
      tone: "red",
      priority: 30,
      count: failedPublish.length,
    },
    blockedDraftCount > 0 && {
      id: "blocked-drafts",
      title: "Review blocked drafts",
      detail: `${blockedDraftCount} ${blockedDraftCount === 1 ? "draft needs" : "drafts need"} a human positioning or quality decision.`,
      href: `/projects/${projectId}/review`,
      cta: "Review drafts",
      category: "Blocking now",
      tone: "red",
      priority: 40,
      count: blockedDraftCount,
    },
    reviewDraftCount > 0 && {
      id: "draft-review",
      title: "Review drafts",
      detail: `${reviewDraftCount} ${reviewDraftCount === 1 ? "draft is" : "drafts are"} waiting at the approval gate.`,
      href: `/projects/${projectId}/review`,
      cta: "Open Review",
      category: "Needs review",
      tone: "amber",
      priority: 50,
      count: reviewDraftCount,
    },
    visibilityOpenOpportunityCount > 0 && {
      id: "analysis-review",
      title: "Review opportunities",
      detail: `${visibilityOpenOpportunityCount} ${visibilityOpenOpportunityCount === 1 ? "opportunity is" : "opportunities are"} ready before CiteLoop advances the content plan.`,
      href: `/projects/${projectId}/analysis`,
      cta: "Review opportunities",
      category: "Needs review",
      tone: "amber",
      priority: 60,
      count: visibilityOpenOpportunityCount,
    },
    ready.length > 0 && {
      id: "distribute-variants",
      title: "Distribute variants",
      detail: `${ready.length} approved ${ready.length === 1 ? "variant is" : "variants are"} ready after the canonical page went live.`,
      href: `/projects/${projectId}/publish`,
      cta: "Open Publish",
      category: "Needs review",
      tone: "blue",
      priority: 70,
      count: ready.length,
    },
    !searchDataConnected && {
      id: "connect-gsc",
      title: "Connect Search Console",
      detail: "Public data works now, but traffic impact and query trends need Search Console access.",
      href: `/projects/${projectId}/settings#search-console`,
      cta: "Connect GSC",
      category: "Improves results",
      tone: "blue",
      priority: 80,
    },
    automationWarnings.length > 0 && {
      id: "automation-health",
      title: "Check automation health",
      detail: `${automationWarnings.length} ${automationWarnings.length === 1 ? "run has" : "runs have"} a failed or degraded status in the activity log.`,
      href: `/projects/${projectId}/settings/activity`,
      cta: "Open activity",
      category: "Warnings",
      tone: "amber",
      priority: 90,
      count: automationWarnings.length,
    },
  ];
  const humanActionItems = humanActionCandidates.filter((item): item is HumanActionItem => Boolean(item)).sort((a, b) => a.priority - b.priority);
  const primaryAction = humanActionItems[0] ?? fallbackAction;
  const visibleHumanActionItems = humanActionItems.slice(0, VISIBLE_HUMAN_ACTION_LIMIT);
  const hiddenHumanActionItems = humanActionItems.slice(VISIBLE_HUMAN_ACTION_LIMIT);
  const operationsHealthBlocker = automationWarnings[0] ?? null;
  const growthControlCards = [
    {
      title: "Opportunities",
      label: highestPriorityOpportunity ? "Ready to decide" : "Watching",
      detail: highestPriorityOpportunity
        ? opportunityTitle(highestPriorityOpportunity)
        : "No priority opportunity is waiting for a human decision.",
      href: `/projects/${projectId}/analysis`,
      icon: Search,
      tone: highestPriorityOpportunity ? "amber" : "neutral",
    },
    {
      title: "Action Portfolio",
      label: `${visibilityActionsInLoopCount} in loop`,
      detail: visibilityActionsInLoopCount
        ? "Accepted work spans content, metadata, schema, technical, and distribution actions."
        : "Accepted actions will appear here after opportunity decisions.",
      href: `/projects/${projectId}/plan`,
      icon: FileText,
      tone: visibilityActionsInLoopCount ? "blue" : "neutral",
    },
    {
      title: "Impact Reports",
      label: measurementResultNeedsAttention ? "Needs attention" : "Measuring",
      detail: measurementResultNeedsAttention?.summary ?? "Published or applied actions are tracked against conservative outcome windows.",
      href: `/projects/${projectId}/results`,
      icon: BarChart3,
      tone: measurementResultNeedsAttention ? "amber" : "neutral",
    },
    {
      title: "Operations health",
      label: operationsHealthBlocker ? "Operational blockers" : "Clear",
      detail: operationsHealthBlocker
        ? `${operationsHealthBlocker.agent} is ${operationsHealthBlocker.status}. Open diagnostics before relying on automation.`
        : "Budget, publishing, quality, and notification checks have no recent blockers.",
      href: `/projects/${projectId}/settings/activity`,
      icon: AlertTriangle,
      tone: operationsHealthBlocker ? "red" : "green",
    },
    {
      title: "Learning signal",
      label: "Conservative learning",
      detail: "Completed work informs prioritization; risky behavior still waits for policy gates and review.",
      href: `/projects/${projectId}/results`,
      icon: Sparkles,
      tone: "blue",
    },
  ] satisfies Array<{
    title: string;
    label: string;
    detail: string;
    href: string;
    icon: typeof BarChart3;
    tone: StageTone;
  }>;

  const aiCitationMetric = homeAICitationMetric({
    projectId,
    citationGapCount: visibilityCitationSignalCount,
  });
  const inMotionMetric = homeInMotionMetric({
    projectId,
    analysisActionCount: visibilityActionsInLoopCount,
    reviewDraftCount: reviewArticles.length,
    readyToPublishCount: ready.length,
    measuringActionCount: measuringActions,
  });
  const otherProjects = accountProjects.filter((candidate) => candidate.id !== projectId);
  const metricGridCards = [
    {
      label: "Organic traffic",
      value: searchDataConnected ? metric(clicks28d) : "Limited",
      detail: searchDataConnected ? `${metric(impressions28d)} impressions (28d)` : "Connect Search Console for traffic",
      metricChangeLabel: searchDataConnected ? "Search Console connected" : "Connect for change data",
      metricChangeTone: searchDataConnected ? "green" : "amber",
      href: `/projects/${projectId}/results`,
      icon: BarChart3,
      featured: true,
      muted: !searchDataConnected,
    },
    {
      label: aiCitationMetric.label,
      value: aiCitationMetric.value,
      detail: aiCitationMetric.detail,
      metricChangeLabel: aiCitationMetric.metricChangeLabel,
      metricChangeTone: aiCitationMetric.metricChangeTone,
      href: aiCitationMetric.href,
      icon: Sparkles,
      featured: false,
      muted: aiCitationMetric.muted,
    },
    {
      label: "Published pages",
      value: publishedThisMonth,
      detail: "canonical pages live this month",
      metricChangeLabel: publishedThisMonth > 0 ? `+${publishedThisMonth} this month` : "0 this month",
      metricChangeTone: publishedThisMonth > 0 ? "green" : "neutral",
      href: `/projects/${projectId}/publish`,
      icon: FileText,
      featured: false,
      muted: publishedThisMonth === 0,
    },
    {
      label: inMotionMetric.label,
      value: inMotionMetric.value,
      detail: inMotionMetric.detail,
      metricChangeLabel: inMotionMetric.metricChangeLabel,
      metricChangeTone: inMotionMetric.metricChangeTone,
      href: inMotionMetric.href,
      icon: Search,
      featured: false,
      muted: inMotionMetric.muted,
    },
  ] satisfies Array<{
    label: string;
    value: number | string;
    detail: string;
    metricChangeLabel: string;
    metricChangeTone: StageTone;
    href: string;
    icon: typeof BarChart3;
    featured: boolean;
    muted: boolean;
  }>;

  // Pipeline stages keep the daily workflow focused; Context remains a setup gate.
  const stages: Array<{ label: string; metricValue: number | string; statusLabel: string; tone: StageTone; href: string; highlight?: boolean }> = [
    {
      label: "Opportunities",
      metricValue: visibilityOpenOpportunityCount,
      statusLabel: !contextConfirmed
        ? "Locked until Context"
        : visibilityOpenOpportunityCount > 0
          ? "Ready to review"
          : visibilityActionsInLoopCount > 0
            ? "Reviewed"
            : "Scanning",
      tone: !contextConfirmed ? "neutral" : visibilityOpenOpportunityCount > 0 ? "amber" : "green",
      href: `/projects/${projectId}/analysis`,
      highlight: contextConfirmed && visibilityOpenOpportunityCount > 0,
    },
    {
      label: "Content Plan",
      metricValue: planItemCount,
      statusLabel: !contextConfirmed
        ? "Locked until Context"
        : planGenerationPending
          ? "Generating (auto)"
          : planItemCount > 0
            ? "Plan ready"
            : "Waiting",
      tone: !contextConfirmed ? "neutral" : planGenerationPending ? "blue" : planItemCount > 0 ? "green" : "neutral",
      href: `/projects/${projectId}/plan`,
    },
    {
      label: "Review",
      metricValue: reviewArticles.length,
      statusLabel: reviewArticles.length > 0 ? "Needs approval" : "Clear",
      tone: reviewArticles.length > 0 ? "amber" : "green",
      href: `/projects/${projectId}/review`,
    },
    {
      label: "Publish",
      metricValue: publishedThisMonth,
      statusLabel: failedPublish.length > 0 ? "Needs attention" : ready.length > 0 ? "Ready to distribute" : publishedThisMonth > 0 ? "Live this month" : "Waiting",
      tone: failedPublish.length > 0 ? "red" : ready.length > 0 ? "amber" : publishedThisMonth > 0 ? "green" : "neutral",
      href: `/projects/${projectId}/publish`,
    },
    {
      label: "Results",
      metricValue: measuringActions,
      statusLabel: measuringActions > 0 ? "Measuring impact" : searchDataConnected ? "Ready for impact data" : "Connect for proof",
      tone: measuringActions > 0 || searchDataConnected ? "green" : "amber",
      href: `/projects/${projectId}/results`,
    },
  ];

  const nextScheduledRow = scheduledRows.find((row) => row.time);
  const eventStream = buildHomeEventStream({
    projectId,
    liveActivities: runs
      .filter((run) => run.status === "running")
      .slice(0, 2)
      .map((run) => ({
        id: `run-${run.id}`,
        title: "CiteLoop is working on this project",
        detail: "Automation is running right now.",
        href: `/projects/${projectId}/settings/activity`,
      })),
    recentEvents: [
      ...published.slice(0, 2).map((article) => ({
        id: `published-${article.id}`,
        title: `Published ${articleTitle(article)}`,
        detail: formatDate(article.published_at),
        href: `/projects/${projectId}/results`,
      })),
      ...approved.slice(0, 1).map((article) => ({
        id: `approved-${article.id}`,
        title: `Approved ${articleTitle(article)}`,
        detail: formatDate(article.reviewed_at),
        href: `/projects/${projectId}/publish`,
      })),
      ...visibilityOpenOpportunities.slice(0, 1).map((opportunity) => ({
        id: `opportunity-${opportunity.id}`,
        title: opportunityTitle(opportunity),
        detail: "Opportunity detected",
        href: `/projects/${projectId}/analysis`,
      })),
    ],
    nextEvent: nextScheduledRow
      ? {
          title: "Next publish slot",
          detail: `${nextScheduledRow.title} - ${formatDate(nextScheduledRow.time)}`,
          href: `/projects/${projectId}/publish`,
        }
      : null,
  });

  return (
    <div className="space-y-4">
      {apiError && (
        <Notice
          title="Project data could not be loaded"
          detail={apiError}
          tone="amber"
        />
      )}

      <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {metricGridCards.map((item) => {
          const MetricIcon = item.icon;
          return (
            <a
              key={item.label}
              href={item.href}
              className={compactMetricCardClass(item.featured)}
            >
              <div className="flex items-start justify-between gap-3">
                <div className="flex min-w-0 items-center gap-2 text-sm font-bold text-slate-500">
                  <MetricIcon size={16} className={item.muted ? "shrink-0 text-slate-300" : "shrink-0 text-[#d93820]"} />
                  <span className="truncate">{item.label}</span>
                </div>
                <span className="inline-flex shrink-0 items-center gap-1 text-xs font-bold text-slate-400 transition-colors group-hover:text-[#d93820]">
                  View
                  <ArrowRight size={14} />
                </span>
              </div>
              <div
                className={cx(
                  "mt-3 font-bold leading-none tracking-tight",
                  item.featured ? "text-3xl" : "text-2xl",
                  item.muted ? "text-slate-400" : "text-slate-950",
                )}
              >
                {item.value}
              </div>
              <div className={cx("mt-2 line-clamp-2 text-sm font-semibold leading-5", item.muted ? "text-slate-400" : "text-slate-500")}>{item.detail}</div>
              <div className="mt-auto pt-3">
                <span className={cx("inline-flex rounded-full px-2.5 py-1 text-xs font-bold ring-1", metricChangeClass(item.metricChangeTone))}>
                  {item.metricChangeLabel}
                </span>
              </div>
            </a>
          );
        })}
      </section>

      <section>
        <SectionHeader
          title="Growth Control Center"
          eyebrow="Opportunities, content, results"
          action={<Badge tone={humanActionItems.length > 0 ? "amber" : "green"}>{humanActionItems.length} open gates</Badge>}
        />
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
          {growthControlCards.map((card) => {
            const CardIcon = card.icon;
            return (
              <a
                key={card.title}
                href={card.href}
                className="group flex min-h-[150px] flex-col rounded-xl border border-slate-200 bg-white p-4 transition-colors hover:border-slate-300 hover:bg-slate-50"
              >
                <div className="flex items-start justify-between gap-3">
                  <span className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-slate-50 text-slate-500 ring-1 ring-slate-100 transition-colors group-hover:text-[#d93820]">
                    <CardIcon aria-hidden="true" size={17} />
                  </span>
                  <Badge tone={card.tone}>{card.label}</Badge>
                </div>
                <div className="mt-3 min-w-0">
                  <h2 className="text-sm font-bold leading-5 text-slate-950">{card.title}</h2>
                  <p className="mt-2 line-clamp-3 text-[13px] font-semibold leading-5 text-slate-600">{card.detail}</p>
                </div>
                <span className="mt-auto inline-flex items-center gap-1 pt-3 text-xs font-bold text-slate-400 transition-colors group-hover:text-[#d93820]">
                  Open
                  <ArrowRight aria-hidden="true" size={14} />
                </span>
              </a>
            );
          })}
        </div>
      </section>

      {otherProjects.length > 0 && (
        <section>
          <SectionHeader title="Other projects" eyebrow="Connected to this account" />
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {otherProjects.map((candidate) => (
              <a
                key={candidate.id}
                href={`/projects/${candidate.id}`}
                className="group flex min-h-[92px] items-center justify-between gap-4 rounded-xl border border-slate-200 bg-white px-4 py-3 transition-colors hover:border-slate-300 hover:bg-slate-50"
              >
                <div className="min-w-0">
                  <div className="truncate text-sm font-bold text-slate-950">{candidate.name}</div>
                  <div className="mt-1 truncate text-xs font-semibold text-slate-400">{candidate.config?.site_url || `/${candidate.slug}`}</div>
                </div>
                <span className="inline-flex shrink-0 items-center gap-1 rounded-full bg-slate-50 px-2.5 py-1 text-xs font-bold text-slate-500 ring-1 ring-slate-100 transition-colors group-hover:text-[#d93820]">
                  Switch
                  <ArrowRight size={13} />
                </span>
              </a>
            ))}
          </div>
        </section>
      )}

      {/* Context build progress — only while onboarding is running */}
      {showContextBuild && (
        <section
          role="status"
          aria-live="polite"
          aria-label="Building domain context progress"
          className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-4"
        >
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <Badge tone={contextBuild.exhausted ? "amber" : "blue"}>Parallel context build</Badge>
              <h2 className="mt-2 text-lg font-bold leading-6 text-slate-950">{contextBuild.title}</h2>
              <p className="mt-1 max-w-[70ch] text-sm leading-5 text-amber-900">{contextBuild.detail}</p>
            </div>
            <div className="inline-flex items-center gap-2 rounded-lg bg-white px-3 py-2 text-xs font-bold text-slate-600 ring-1 ring-amber-100">
              <Loader2 size={14} className="animate-spin text-[#d93820]" />
              Checking automatically
            </div>
          </div>
          <div className="mt-4 grid gap-3 lg:grid-cols-3">
            {contextBuild.tracks.map((track) => (
              <div key={track.id} className="rounded-lg bg-white px-3 py-3 ring-1 ring-amber-100">
                <div className="flex items-start justify-between gap-3">
                  <div className="flex min-w-0 items-center gap-2">
                    {track.state === "done" ? (
                      <CheckCircle2 size={16} className="shrink-0 text-green-600" />
                    ) : track.state === "attention" ? (
                      <AlertTriangle size={16} className="shrink-0 text-amber-600" />
                    ) : track.state === "running" ? (
                      <Loader2 size={16} className="shrink-0 animate-spin text-[#d93820]" />
                    ) : (
                      <Circle size={16} className="shrink-0 text-slate-300" />
                    )}
                    <div className="truncate text-sm font-bold text-slate-900">{track.label}</div>
                  </div>
                  <div className="shrink-0 text-xs font-bold text-slate-500">{track.progress}%</div>
                </div>
                <div className="mt-3 h-2 overflow-hidden rounded-full bg-amber-50 ring-1 ring-inset ring-amber-100">
                  <div
                    className={cx(
                      "h-full rounded-full transition-all duration-500",
                      track.state === "done" ? "bg-green-600" : track.state === "attention" ? "bg-amber-500" : "bg-[#d93820]",
                    )}
                    style={{ width: `${track.progress}%` }}
                  />
                </div>
                <p className="mt-2 text-[13px] font-semibold leading-5 text-slate-500">{track.detail}</p>
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Human action queue — the Home action spotlight for every manual gate. */}
      <section aria-label="Action spotlight">
        <SectionHeader
          title="Needs you"
          eyebrow="Manual gates and setup"
          action={<Badge tone={humanActionItems.length > 0 ? "amber" : "green"}>{humanActionItems.length} open</Badge>}
        />
        {humanActionItems.length === 0 ? (
          <EmptyState
            title="No open actions"
            detail="Nothing needs a manual decision right now. CiteLoop will keep planning, drafting, publishing, or measuring as the loop advances."
          />
        ) : (
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            {visibleHumanActionItems.map((item) => {
              const ActionIcon = humanActionIcon(item);
              return (
                <a
                  key={item.id}
                  href={item.href}
                  className={compactActionTileClass(item.tone)}
                >
                  <div className="flex items-start justify-between gap-3">
                    <span className={cx("inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg ring-1 ring-inset", humanActionIconToneClass(item.tone))}>
                      <ActionIcon aria-hidden="true" size={16} />
                    </span>
                    {item.count != null && (
                      <span className="inline-flex h-6 min-w-6 items-center justify-center rounded-md bg-white px-2 text-xs font-bold text-slate-700 ring-1 ring-slate-200">
                        {item.count}
                      </span>
                    )}
                  </div>
                  <div className="mt-2 min-w-0">
                    <Badge tone={item.tone}>{item.category}</Badge>
                    <h3 className="mt-2 line-clamp-1 text-[15px] font-bold leading-5 text-slate-950">{item.title}</h3>
                    <p className="mt-1 line-clamp-2 text-[13px] font-semibold leading-5 text-slate-600">{item.detail}</p>
                  </div>
                  <span className="mt-3 inline-flex h-8 items-center justify-between gap-2 rounded-lg bg-white px-3 text-xs font-bold text-slate-800 ring-1 ring-slate-200 transition-colors group-hover:text-[#d93820]">
                    <span className="truncate">{item.cta}</span>
                    <ArrowRight aria-hidden="true" size={14} className="shrink-0 transition-transform group-hover:translate-x-0.5" />
                  </span>
                </a>
              );
            })}
            {hiddenHumanActionItems.length > 0 && (
              <details className="rounded-xl border border-dashed border-slate-200 bg-white px-4 py-3 sm:col-span-2 xl:col-span-4">
                <summary className="cursor-pointer text-sm font-bold text-slate-700">View all open actions</summary>
                <div className="mt-3 grid gap-2 sm:grid-cols-2">
                  {hiddenHumanActionItems.map((item) => (
                    <a
                      key={item.id}
                      href={item.href}
                      className="flex min-h-[44px] items-center justify-between gap-3 rounded-lg border border-slate-100 bg-slate-50 px-3 py-2 text-sm transition-colors hover:bg-white"
                    >
                      <span className="min-w-0 truncate font-semibold text-slate-900">{item.title}</span>
                      <span className="shrink-0 text-xs font-bold text-slate-500">{item.cta}</span>
                    </a>
                  ))}
                </div>
              </details>
            )}
          </div>
        )}
      </section>

      {/* First-fold pipeline — the flywheel as a connected progress spine */}
      <section>
        <SectionHeader title="Pipeline" eyebrow="Where this project is in the loop" />
        <div className="flex gap-2 overflow-x-auto pb-1">
          {stages.map((stage, index) => {
            const isNext = stage.href === primaryAction.href && (stage.tone === "amber" || stage.tone === "red");
            const highlighted = isNext || Boolean(stage.highlight);
            return (
              <a
                key={stage.label}
                href={stage.href}
                className={cx(
                  "relative flex min-w-[140px] flex-1 flex-col rounded-xl border bg-white px-3 py-3 transition-colors hover:border-slate-300",
                  highlighted ? "border-[#d93820] ring-1 ring-[#d93820]" : "border-slate-200",
                )}
              >
                <span className="absolute left-2.5 top-2.5 inline-flex h-5 min-w-5 items-center justify-center rounded-md border border-slate-200 bg-slate-50 px-1 text-[11px] font-bold text-slate-500">
                  {index + 1}
                </span>
                <div className="truncate px-6 text-center text-sm font-bold text-slate-900">{stage.label}</div>
                <div className="mt-2 text-center text-xl font-bold leading-none text-slate-950">{stage.metricValue}</div>
                <div className="mt-2 flex items-center justify-center gap-1.5">
                  <span className={cx("h-1.5 w-1.5 shrink-0 rounded-full", stageDotClass(stage.tone))} />
                  <span className="truncate text-xs font-semibold text-slate-500">{stage.statusLabel}</span>
                </div>
              </a>
            );
          })}
        </div>
      </section>

      {/* One merged activity timeline */}
      <section>
        <SectionHeader title="Activity" eyebrow="Now, recent, and next" action={<a href={`/projects/${projectId}/settings/activity`} className="text-xs font-semibold text-slate-500">Full log</a>} />
        {eventStream.items.length === 0 ? (
          <EmptyState
            title="No activity yet"
            detail="Analysis, drafts, published pages, and measured results will appear here as the loop moves."
          />
        ) : (
          <div className="grid gap-2">
            {eventStream.items.map((item) => (
              <a
                key={item.id}
                href={item.href}
                className="flex min-h-[44px] items-center justify-between gap-3 rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm transition-colors hover:bg-slate-50"
              >
                <div className="min-w-0">
                  <div className="truncate font-semibold text-slate-900">{item.title}</div>
                  <div className="mt-0.5 truncate text-[13px] font-semibold text-slate-400">{item.detail}</div>
                </div>
                <Badge tone={item.kind === "live" ? "amber" : item.kind === "next" ? "blue" : "green"}>{item.timeLabel}</Badge>
              </a>
            ))}
          </div>
        )}
      </section>

      {/* Needs-review preview only when there are drafts, kept compact */}
      {reviewArticles.length > 0 && (
        <section>
          <SectionHeader title="Drafts waiting for review" action={<Badge tone="amber">{reviewArticles.length}</Badge>} />
          <div className="grid gap-2 sm:grid-cols-2">
            {reviewArticles.slice(0, 4).map((article) => (
              <a
                key={article.id}
                href={`/projects/${projectId}/review`}
                className="rounded-xl border border-slate-200 bg-white px-4 py-3 transition-colors hover:bg-slate-50"
              >
                <div className="flex items-center justify-between gap-2">
                  <Badge tone={article.kind === "canonical" ? "green" : "neutral"}>{article.platform || article.kind}</Badge>
                  {article.qa_blocking && <Badge tone="red">Blocked</Badge>}
                </div>
                <div className="mt-2 text-[15px] font-semibold leading-5 text-slate-900">{articleTitle(article)}</div>
                <div className="mt-2 text-xs text-slate-500">geo {formatScore(article.geo_score)} / seo {formatScore(article.seo_score)}</div>
              </a>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
