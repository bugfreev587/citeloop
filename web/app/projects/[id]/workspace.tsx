"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertTriangle, ArrowRight, BarChart3, CheckCircle2, Circle, FileText, Loader2, RefreshCw, Search, Sparkles } from "lucide-react";
import {
  Article,
  DistributeItem,
  GenerationRun,
  InventoryItem,
  ProductProfile,
  Project,
  ReviewGroup,
  SEOContentAction,
  SEOOpportunity,
  SEOOverview,
  Topic,
} from "../../lib/api";
import {
  buildHomeEventStream,
  contextBuildTracks,
  nextWorkspaceAction,
} from "../../lib/dashboard-ux-logic";
import { normalizeNumeric } from "../../lib/normalize";
import { useApi } from "../../lib/use-api";
import { Badge, Button, EmptyState, Notice, SectionHeader, cx, formatDate, formatScore } from "../../components/ui";

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

function evidenceCount(items: InventoryItem[]) {
  return items.reduce((total, item) => total + (Array.isArray(item.evidence_snippets) ? item.evidence_snippets.length : 0), 0);
}

function opportunityTitle(opportunity: SEOOpportunity) {
  return opportunity.recommended_action || opportunity.query || opportunity.page_url || opportunity.type || "Visibility opportunity";
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
  const [seoOpportunities, setSeoOpportunities] = useState<SEOOpportunity[]>([]);
  const [seoActions, setSeoActions] = useState<SEOContentAction[]>([]);
  const [busy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);
  const [apiError, setApiError] = useState<string | null>(null);
  const [onboardingPollCount, setOnboardingPollCount] = useState(0);

  const refresh = useCallback(async () => {
    setApiError(null);
    try {
      const [p, profileRow, inventoryRows, t, r, pub, app, failed, dist, runRows, insightRunRows, overview, opportunities, actions] = await Promise.all([
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
        api.listSEOOpportunities(projectId, { status: "open", limit: 50 }).catch(() => []),
        api.listSEOContentActions(projectId, { limit: 50 }).catch(() => []),
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
      setSeoOpportunities(opportunities);
      setSeoActions(actions);
      return { profile: profileRow, inventory: inventoryRows, insightRuns: insightRunRows };
    } catch (e: any) {
      setApiError(e.message);
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

  const contextEvidenceCount = evidenceCount(inventory);
  const contextEvidencePageCount = inventory.filter((item) => Array.isArray(item.evidence_snippets) && item.evidence_snippets.length > 0).length;
  const sourcePageCount = Math.max(inventory.length, profile?.source_urls?.length ?? 0);
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

  // After project creation, onboarding (crawl + product profile) runs in the background.
  // Home keeps checking the profile and inventory so fresh projects do not strand users
  // on an empty Context page while the detached onboarding jobs finish.
  const onboardingAttemptsRef = useRef(0);
  useEffect(() => {
    if (!projectLoaded || !contextBuild.active) {
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
        const nextEvidenceCount = evidenceCount(next.inventory);
        const nextEvidencePageCount = next.inventory.filter((item) => Array.isArray(item.evidence_snippets) && item.evidence_snippets.length > 0).length;
        const nextSourcePageCount = Math.max(next.inventory.length, next.profile?.source_urls?.length ?? 0);
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
  }, [projectLoaded, contextBuild.active, refresh]);

  const reviewArticles = review.flatMap((group) => group.articles);
  const scheduledRows = useMemo(() => {
    const articleRows = approved
      .filter((article) => article.kind === "canonical")
      .map((article) => ({ id: article.id, time: article.scheduled_at, title: articleTitle(article) }));
    return [...articleRows].sort((a, b) => String(a.time).localeCompare(String(b.time))).slice(0, 5);
  }, [approved]);

  const waitingVariants = approved.filter(
    (article) => article.kind === "syndication_variant" && !ready.some((item) => item.article.id === article.id),
  );
  const automationWarnings = runs.filter((run) => ["error", "failed"].includes(run.status) || Boolean(run.output?.degraded));
  const hasBlockedDrafts = reviewArticles.some((article) => article.qa_blocking);

  const opportunitiesInPlanCount = seoActions.length;
  const planItemCount = topics.length + opportunitiesInPlanCount;
  const planGenerationPending = opportunitiesInPlanCount > 0 && topics.length === 0;
  const publishedThisMonth = published.filter((article) => isThisMonth(article.published_at)).length;
  const searchDataConnected = hasConnectedSearchData(seoOverview);
  const clicks28d = normalizeNumeric(seoOverview?.last_28_days?.clicks_28d ?? null);
  const impressions28d = normalizeNumeric(seoOverview?.last_28_days?.impressions_28d ?? null);
  const measuringActions = sumCounts(seoOverview?.actions_by_status, ["published", "measuring", "completed"]);
  const aiCitationSignals = seoOpportunities.filter((opportunity) =>
    `${opportunity.type} ${opportunity.recommended_action ?? ""} ${opportunity.expected_impact ?? ""}`.toLowerCase().match(/ai|llm|citation|answer/),
  ).length;

  const growthHeadline = searchDataConnected || publishedThisMonth > 0 || measuringActions > 0
    ? "CiteLoop is measuring growth from published work"
    : !profile
      ? "Connect your product to start the growth loop"
      : "The growth loop is warming up";
  const growthDetail = searchDataConnected
    ? "Verified Search Console data is connected, so CiteLoop can report clicks, impressions, and which content is moving."
    : "CiteLoop runs the loop automatically and only stops at the gates that need a human. The next thing that needs you is below.";

  // Single primary action for the whole project, computed from real state.
  const primaryAction = nextWorkspaceAction({
    projectId,
    hasProfile: Boolean(profile),
    contextConfirmed,
    failedPublishCount: failedPublish.length,
    hasBlockedDrafts,
    reviewCount: reviewArticles.length,
    readyCount: ready.length,
    topicsCount: topics.length,
    openOpportunityCount: seoOpportunities.length,
  });

  const growthMetricCards = [
    {
      label: "AI citations",
      value: aiCitationSignals > 0 ? aiCitationSignals : "-",
      detail: aiCitationSignals > 0 ? "citation opportunities detected" : "AI-answer tracking not connected yet",
      icon: Sparkles,
      muted: aiCitationSignals === 0,
    },
    {
      label: "Organic traffic",
      value: searchDataConnected ? metric(clicks28d) : "Limited",
      detail: searchDataConnected ? `${metric(impressions28d)} impressions (28d)` : "Connect Search Console for traffic",
      icon: BarChart3,
      muted: !searchDataConnected,
    },
    {
      label: "Published pages",
      value: publishedThisMonth,
      detail: "canonical pages live this month",
      icon: FileText,
      muted: publishedThisMonth === 0,
    },
    {
      label: "In motion",
      value: opportunitiesInPlanCount + reviewArticles.length + ready.length + measuringActions,
      detail: "planned, in review, publishing, or measuring",
      icon: Search,
      muted: opportunitiesInPlanCount + reviewArticles.length + ready.length + measuringActions === 0,
    },
  ];

  // Pipeline stages — same honest per-stage status logic, rendered as a compact stepper.
  const stages: Array<{ label: string; metricValue: number | string; statusLabel: string; tone: StageTone; href: string }> = [
    {
      label: "Context",
      metricValue: sourcePageCount,
      statusLabel: !profile
        ? "Reading your site"
        : contextNeedsConfirmation
          ? "Needs confirmation"
          : contextConfirmed
            ? "Confirmed"
            : "Incomplete",
      tone: !profile ? "blue" : contextNeedsConfirmation ? "amber" : contextConfirmed ? "green" : "amber",
      href: `/projects/${projectId}/context`,
    },
    {
      label: "Opportunities",
      metricValue: seoOpportunities.length,
      statusLabel: !contextConfirmed
        ? "Locked until Context"
        : seoOpportunities.length > 0
          ? "Ready to review"
          : opportunitiesInPlanCount > 0
            ? "Reviewed"
            : "Scanning",
      tone: !contextConfirmed ? "neutral" : seoOpportunities.length > 0 ? "amber" : "green",
      href: `/projects/${projectId}/visibility`,
    },
    {
      label: "Plan",
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
      label: "Drafts",
      metricValue: reviewArticles.length + approved.length,
      statusLabel: reviewArticles.length + approved.length > 0 ? "In motion" : topics.length > 0 ? "Drafting (auto)" : "Waiting",
      tone: reviewArticles.length + approved.length > 0 ? "green" : topics.length > 0 ? "blue" : "neutral",
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
      label: "Measure",
      metricValue: searchDataConnected ? metric(clicks28d) : "-",
      statusLabel: searchDataConnected ? "Connected" : "Connect for proof",
      tone: searchDataConnected ? "green" : "amber",
      href: `/projects/${projectId}/visibility`,
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
        href: `/projects/${projectId}/visibility`,
      })),
      ...approved.slice(0, 1).map((article) => ({
        id: `approved-${article.id}`,
        title: `Approved ${articleTitle(article)}`,
        detail: formatDate(article.reviewed_at),
        href: `/projects/${projectId}/publish`,
      })),
      ...seoOpportunities.slice(0, 1).map((opportunity) => ({
        id: `opportunity-${opportunity.id}`,
        title: opportunityTitle(opportunity),
        detail: "Visibility opportunity detected",
        href: `/projects/${projectId}/visibility`,
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

  // Compact "Needs you" list — one place for everything blocked, replacing six separate sections.
  const needsYou = [
    { id: "failed", label: "Publishing failed", count: failedPublish.length, href: `/projects/${projectId}/publish`, tone: "red" as const },
    { id: "blocked", label: "Drafts blocked by QA", count: reviewArticles.filter((a) => a.qa_blocking).length, href: `/projects/${projectId}/review`, tone: "red" as const },
    { id: "review", label: "Drafts waiting for review", count: reviewArticles.filter((a) => !a.qa_blocking).length, href: `/projects/${projectId}/review`, tone: "amber" as const },
    { id: "opportunities", label: "Opportunities to review", count: seoOpportunities.length, href: `/projects/${projectId}/visibility`, tone: "amber" as const },
    { id: "distribute", label: "Variants ready to distribute", count: ready.length, href: `/projects/${projectId}/publish`, tone: "green" as const },
    { id: "warnings", label: "Automation warnings", count: automationWarnings.length, href: `/projects/${projectId}/settings/activity`, tone: "amber" as const },
    { id: "waiting-canonical", label: "Variants waiting on canonical", count: waitingVariants.length, href: `/projects/${projectId}/publish`, tone: "neutral" as const },
  ].filter((row) => row.count > 0);

  return (
    <div className="space-y-6">
      {apiError && (
        <Notice
          title="API server unavailable"
          detail={`Dashboard data could not be loaded (${apiError}).`}
          tone="amber"
        />
      )}
      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}

      {/* Hero + the single thing that needs you */}
      <section className="rounded-2xl border border-slate-200 bg-white p-5 md:p-6">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <Badge tone="green">Growth loop</Badge>
              <span className="text-xs font-semibold text-slate-400">{project?.name ?? "Project"}</span>
            </div>
            <h1 className="mt-3 max-w-[680px] text-2xl font-bold leading-8 tracking-tight text-slate-950 md:text-3xl">
              {growthHeadline}
            </h1>
            <p className="mt-2 max-w-[68ch] text-sm leading-6 text-slate-600">{growthDetail}</p>
          </div>
          <Button disabled={!!busy} size="sm" variant="outline" onClick={() => refresh()}>
            <RefreshCw size={14} />
            Refresh
          </Button>
        </div>

        <div className="mt-5 flex flex-col gap-3 rounded-xl border border-slate-100 bg-slate-50 p-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <div className="text-[11px] font-bold uppercase tracking-wide text-slate-400">Your next step</div>
            <div className="mt-1 text-base font-bold text-slate-900">{primaryAction.title}</div>
            <div className="mt-0.5 max-w-[64ch] text-sm leading-5 text-slate-500">{primaryAction.detail}</div>
          </div>
          <a
            href={primaryAction.href}
            className="inline-flex h-10 shrink-0 items-center gap-2 rounded-xl bg-gradient-to-r from-[#d93820] to-[#f4503b] px-4 text-sm font-semibold text-white transition-all duration-150 active:scale-[0.97]"
          >
            {primaryAction.title}
            <ArrowRight size={16} />
          </a>
        </div>
      </section>

      {/* Context build progress — only while onboarding is running */}
      {contextBuild.active && (
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

      {/* Slim, honest metric strip */}
      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {growthMetricCards.map((item) => {
          const MetricIcon = item.icon;
          return (
            <div key={item.label} className="rounded-xl border border-slate-200 bg-white p-4">
              <div className="flex items-center gap-2 text-sm font-bold text-slate-500">
                <MetricIcon size={16} className={item.muted ? "text-slate-300" : "text-[#d93820]"} />
                {item.label}
              </div>
              <div className={cx("mt-3 text-2xl font-bold leading-none", item.muted ? "text-slate-400" : "text-slate-950")}>{item.value}</div>
              <div className="mt-2 text-[13px] font-semibold leading-5 text-slate-400">{item.detail}</div>
            </div>
          );
        })}
      </section>

      {/* Pipeline — the flywheel as a connected progress spine */}
      <section>
        <SectionHeader title="Pipeline" eyebrow="Where this project is in the loop" />
        <div className="flex gap-2 overflow-x-auto pb-1">
          {stages.map((stage, index) => {
            const isNext = stage.href === primaryAction.href && (stage.tone === "amber" || stage.tone === "red");
            return (
              <a
                key={stage.label}
                href={stage.href}
                className={cx(
                  "flex min-w-[140px] flex-1 flex-col rounded-xl border bg-white px-3 py-3 transition-colors hover:border-slate-300",
                  isNext ? "border-[#d93820] ring-1 ring-[#d93820]" : "border-slate-200",
                )}
              >
                <div className="flex items-center justify-center gap-2">
                  <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-md border border-slate-200 bg-slate-50 px-1 text-[11px] font-bold text-slate-500">
                    {index + 1}
                  </span>
                  <span className="truncate text-sm font-bold text-slate-900">{stage.label}</span>
                </div>
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

      {/* Everything that needs a human, in one place */}
      {needsYou.length > 0 && (
        <section>
          <SectionHeader title="Needs you" eyebrow="Open gates and blockers" />
          <div className="grid gap-2 sm:grid-cols-2">
            {needsYou.map((row) => (
              <a
                key={row.id}
                href={row.href}
                className="flex items-center justify-between gap-3 rounded-lg border border-slate-200 bg-white px-4 py-3 text-sm font-semibold text-slate-800 transition-colors hover:bg-slate-50 hover:text-[#d93820]"
              >
                <span className="min-w-0 truncate">{row.label}</span>
                <Badge tone={row.tone}>{row.count}</Badge>
              </a>
            ))}
          </div>
        </section>
      )}

      {/* One merged activity timeline */}
      <section>
        <SectionHeader title="Activity" eyebrow="Now, recent, and next" action={<a href={`/projects/${projectId}/settings/activity`} className="text-xs font-semibold text-slate-500">Full log</a>} />
        {eventStream.items.length === 0 ? (
          <EmptyState
            title="No activity yet"
            detail="Opportunities, drafts, published pages, and measured results will appear here as the loop moves."
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
