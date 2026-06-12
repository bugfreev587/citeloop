"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertTriangle, ArrowDown, ArrowLeft, ArrowRight, ArrowUp, BarChart3, CheckCircle2, Circle, Copy, ExternalLink, FileText, Loader2, RefreshCw, Search, Sparkles } from "lucide-react";
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
} from "../../lib/api";
import {
  buildHomeEventStream,
  contextBuildTracks,
  visibilityLifecycleLabel,
  visibilityLifecycleTone,
  visibleHomeSectionIds,
} from "../../lib/dashboard-ux-logic";
import { normalizeNumeric } from "../../lib/normalize";
import { useApi } from "../../lib/use-api";
import { Badge, Button, EmptyState, Notice, SectionHeader, cx, formatDate, formatScore } from "../../components/ui";

type Message = { tone: "neutral" | "red" | "green" | "amber"; title: string; detail?: string } | null;

const ONBOARDING_POLL_LIMIT = 18;
const ONBOARDING_POLL_MS = 8000;

function articleTitle(article: Article) {
  return article.seo_meta?.title || article.seo_meta?.slug || `${article.kind} draft`;
}

function topicLabel(topic: Topic) {
  return topic.title || "Untitled topic";
}

function activityLabel(agent: string) {
  const labels: Record<string, string> = {
    insight: "Context refresh",
    strategist: "Content plan update",
    writer: "Draft creation",
    qa: "Review quality check",
    publisher: "Publishing",
    notification: "Notification",
  };
  return labels[agent] ?? "Automation activity";
}

function activityTone(status: string, degraded: boolean): "green" | "red" | "amber" | "neutral" {
  if (status === "error" || status === "failed") return "red";
  if (degraded || status === "running") return "amber";
  if (status === "ok") return "green";
  return "neutral";
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

const loopConnectorLabels = {
  contextToFind: "Context feeds Find opportunities",
  findToPlan: "Find opportunities connects to Plan content",
  planToCreate: "Plan content connects to Create drafts",
  createToReview: "Create drafts connects to Review",
  reviewToPublish: "Review connects to Publish",
  publishToMeasure: "Publish connects to Measure results",
  measureToFind: "Measure results connects back to Find opportunities",
} as const;

function loopGridClass(position: number) {
  const classes: Record<number, string> = {
    0: "lg:col-start-2 lg:row-start-1",
    1: "lg:col-start-2 lg:row-start-2",
    2: "lg:col-start-3 lg:row-start-2",
    3: "lg:col-start-3 lg:row-start-3",
    4: "lg:col-start-2 lg:row-start-3",
    5: "lg:col-start-1 lg:row-start-3",
    6: "lg:col-start-1 lg:row-start-2",
  };
  return classes[position] ?? "";
}

function loopConnectorClass(direction: "right" | "down" | "left" | "up") {
  const classes = {
    right: "-right-12 top-1/2 -translate-y-1/2",
    down: "left-1/2 -bottom-12 -translate-x-1/2",
    left: "-left-12 top-1/2 -translate-y-1/2",
    up: "left-1/2 -top-12 -translate-x-1/2",
  };
  return classes[direction];
}

function loopConnectorIcon(direction: "right" | "down" | "left" | "up") {
  if (direction === "down") return ArrowDown;
  if (direction === "left") return ArrowLeft;
  if (direction === "up") return ArrowUp;
  return ArrowRight;
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
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);
  const [apiError, setApiError] = useState<string | null>(null);
  const [onboardingPollCount, setOnboardingPollCount] = useState(0);

  const refresh = useCallback(async () => {
    setApiError(null);
    try {
      const [p, profileRow, inventoryRows, t, r, pub, app, failed, dist, runRows, insightRunRows, overview, opportunities] = await Promise.all([
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
        api.listSEOOpportunities(projectId, { limit: 5 }).catch(() => []),
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
      return { profile: profileRow, inventory: inventoryRows, insightRuns: insightRunRows };
    } catch (e: any) {
      setApiError(e.message);
      return null;
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const contextEvidenceCount = evidenceCount(inventory);
  const contextEvidencePageCount = inventory.filter((item) => Array.isArray(item.evidence_snippets) && item.evidence_snippets.length > 0).length;
  const sourcePageCount = Math.max(inventory.length, profile?.source_urls?.length ?? 0);
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
            detail: "CiteLoop finished reading your site. Review the context, then generate a content plan.",
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

  const run = async (label: string, fn: () => Promise<any>, success = `${label} finished`) => {
    setBusy(label);
    setMessage(null);
    try {
      await fn();
      await refresh();
      setMessage({ tone: "green", title: success });
    } catch (e: any) {
      setMessage({ tone: "red", title: `${label} failed`, detail: e.message });
    } finally {
      setBusy(null);
    }
  };

  const reviewArticles = review.flatMap((group) => group.articles);
  const scheduledRows = useMemo(() => {
    const articleRows = approved
      .filter((article) => article.kind === "canonical")
      .map((article) => ({
        id: article.id,
        time: article.scheduled_at,
        title: articleTitle(article),
        status: article.status,
        type: "canonical",
      }));
    const topicRows = topics
      .filter((topic) => topic.scheduled_at)
      .slice(0, 3)
      .map((topic) => ({
        id: topic.id,
        time: topic.scheduled_at,
        title: topicLabel(topic),
        status: topic.status,
        type: topic.channel,
      }));
    const rows = [...articleRows, ...topicRows].sort((a, b) => String(a.time).localeCompare(String(b.time)));
    return rows.slice(0, 5);
  }, [approved, topics]);

  const waitingVariants = approved.filter(
    (article) => article.kind === "syndication_variant" && !ready.some((item) => item.article.id === article.id),
  );
  const automationWarnings = runs.filter((run) => ["error", "failed"].includes(run.status) || Boolean(run.output?.degraded));
  const hasBlockedDrafts = reviewArticles.some((article) => article.qa_blocking);

  const contextHealth = !profile
    ? {
        label: "Needs context",
        tone: "amber" as const,
        detail: "CiteLoop is still gathering product facts and evidence from this domain.",
      }
    : sourcePageCount === 0
      ? {
          label: "Incomplete",
          tone: "amber" as const,
          detail: "Context exists, but CiteLoop has not captured source pages yet.",
        }
      : contextEvidenceCount === 0
        ? {
            label: "Evidence missing",
            tone: "amber" as const,
            detail: "Source pages are present, but supported claims still need evidence snippets.",
          }
        : {
            label: "Ready",
            tone: "green" as const,
            detail: "CiteLoop has source pages and evidence to support content planning and review.",
          };

  const opportunitiesConverted = seoOpportunities.filter((opportunity) =>
    ["accepted", "planned", "converted"].includes(opportunity.status),
  ).length;
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
    : "Growth measurement is limited";
  const growthDetail = searchDataConnected
    ? "Verified Search Console data is connected, so CiteLoop can report clicks, impressions, and which content is moving."
    : "Search Console is not connected yet. CiteLoop can show content progress and public crawl signals now; connect first-party data to prove traffic growth.";
  const growthTrendPath = "M4 92 C 32 58, 56 46, 88 48 S 148 54, 180 34 S 238 12, 292 18 S 342 32, 392 16";
  const growthMetricCards = [
    {
      label: "AI citations",
      value: aiCitationSignals > 0 ? aiCitationSignals : "-",
      detail: aiCitationSignals > 0 ? "citation-related opportunities detected" : "AI-answer tracking is not connected yet",
      icon: Sparkles,
      muted: aiCitationSignals === 0,
    },
    {
      label: "Organic traffic",
      value: searchDataConnected ? metric(clicks28d) : "Limited",
      detail: searchDataConnected ? `${metric(impressions28d)} impressions in the last 28 days` : "Connect Search Console for clicks and impressions",
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
      label: "Opportunities in motion",
      value: opportunitiesConverted + reviewArticles.length + ready.length + measuringActions,
      detail: "planned, under review, publishing, or measuring",
      icon: Search,
      muted: opportunitiesConverted + reviewArticles.length + ready.length + measuringActions === 0,
    },
  ];
  const loopCards = [
    {
      position: 0,
      label: "Context",
      statusLines: [
        { value: sourcePageCount, label: sourcePageCount === 1 ? "source page" : "source pages" },
        { value: contextEvidenceCount, label: contextEvidenceCount === 1 ? "evidence snippet" : "evidence snippets" },
      ],
      href: `/projects/${projectId}/context`,
      tone: contextHealth.tone,
      accentClass: "text-emerald-700",
      connectorLabel: loopConnectorLabels.contextToFind,
      connectorDirection: "down" as const,
    },
    {
      position: 1,
      label: "Find opportunities",
      statusLines: [
        { value: seoOpportunities.length, label: seoOpportunities.length === 1 ? "opportunity" : "opportunities" },
        { text: seoOpportunities.length > 0 ? "visibility signals found" : "waiting for analytics signal" },
      ],
      href: `/projects/${projectId}/visibility`,
      tone: seoOpportunities.length > 0 ? ("green" as const) : ("neutral" as const),
      accentClass: seoOpportunities.length > 0 ? "text-emerald-700" : "text-slate-500",
      connectorLabel: loopConnectorLabels.findToPlan,
      connectorDirection: "right" as const,
    },
    {
      position: 2,
      label: "Plan content",
      statusLines: [
        { value: topics.length, label: topics.length === 1 ? "topic in the content backlog" : "topics in the content backlog" },
        { text: topics.length > 0 ? "ready for drafting" : "content backlog is not started" },
      ],
      href: `/projects/${projectId}/plan`,
      tone: topics.length > 0 ? ("green" as const) : ("amber" as const),
      accentClass: topics.length > 0 ? "text-emerald-700" : "text-amber-700",
      connectorLabel: loopConnectorLabels.planToCreate,
      connectorDirection: "down" as const,
    },
    {
      position: 3,
      label: "Create drafts",
      statusLines: [
        { value: reviewArticles.length + approved.length, label: reviewArticles.length + approved.length === 1 ? "draft created or approved" : "drafts created or approved" },
        { text: reviewArticles.length + approved.length > 0 ? "ready to review or publish" : "no drafts yet" },
      ],
      href: `/projects/${projectId}/plan`,
      tone: reviewArticles.length + approved.length > 0 ? ("green" as const) : ("neutral" as const),
      accentClass: reviewArticles.length + approved.length > 0 ? "text-emerald-700" : "text-slate-500",
      connectorLabel: loopConnectorLabels.createToReview,
      connectorDirection: "left" as const,
    },
    {
      position: 4,
      label: "Review",
      statusLines: [
        { value: reviewArticles.length, label: reviewArticles.length === 1 ? "draft waiting for approval" : "drafts waiting for approval" },
        { text: reviewArticles.length > 0 ? "claims and evidence need your decision" : "nothing waiting" },
      ],
      href: `/projects/${projectId}/review`,
      tone: reviewArticles.length > 0 ? ("amber" as const) : ("green" as const),
      accentClass: reviewArticles.length > 0 ? "text-amber-700" : "text-emerald-700",
      connectorLabel: loopConnectorLabels.reviewToPublish,
      connectorDirection: "left" as const,
    },
    {
      position: 5,
      label: "Publish",
      statusLines: [
        { value: publishedThisMonth, label: publishedThisMonth === 1 ? "page live this month" : "pages live this month" },
        { text: failedPublish.length > 0 ? "publishing needs attention" : "published this month" },
      ],
      href: `/projects/${projectId}/publish`,
      tone: failedPublish.length > 0 ? ("red" as const) : publishedThisMonth > 0 ? ("green" as const) : ("neutral" as const),
      accentClass: failedPublish.length > 0 ? "text-red-700" : publishedThisMonth > 0 ? "text-emerald-700" : "text-slate-500",
      connectorLabel: loopConnectorLabels.publishToMeasure,
      connectorDirection: "up" as const,
    },
    {
      position: 6,
      label: "Measure results",
      statusLines: [
        { value: searchDataConnected ? metric(clicks28d) : "-", label: searchDataConnected ? "clicks in the last 28 days" : "results" },
        { text: searchDataConnected ? `${metric(impressions28d)} impressions measured` : "limited until connected" },
      ],
      href: `/projects/${projectId}/visibility`,
      tone: searchDataConnected ? ("green" as const) : ("amber" as const),
      accentClass: searchDataConnected ? "text-emerald-700" : "text-amber-700",
      connectorLabel: loopConnectorLabels.measureToFind,
      connectorDirection: "right" as const,
    },
  ];
  const loopItems = [
    ...seoOpportunities.slice(0, 3).map((opportunity) => ({
      id: `opportunity-${opportunity.id}`,
      title: opportunityTitle(opportunity),
      stage: visibilityLifecycleLabel(opportunity.status),
      tone: visibilityLifecycleTone(opportunity.status),
      href: `/projects/${projectId}/visibility`,
    })),
    ...topics.slice(0, 2).map((topic) => ({
      id: `topic-${topic.id}`,
      title: topicLabel(topic),
      stage: "Added to Content Plan",
      tone: "blue" as const,
      href: `/projects/${projectId}/plan`,
    })),
    ...reviewArticles.slice(0, 2).map((article) => ({
      id: `review-${article.id}`,
      title: articleTitle(article),
      stage: article.qa_blocking ? "Draft needs evidence" : "Draft waiting for review",
      tone: article.qa_blocking ? ("red" as const) : ("amber" as const),
      href: `/projects/${projectId}/review`,
    })),
    ...published.slice(0, 2).map((article) => ({
      id: `published-${article.id}`,
      title: articleTitle(article),
      stage: "Published and measuring",
      tone: "green" as const,
      href: `/projects/${projectId}/publish`,
    })),
  ].slice(0, 5);
  const nextScheduledRow = scheduledRows.find((row) => row.time);
  const eventStream = buildHomeEventStream({
    projectId,
    liveActivities: runs
      .filter((run) => run.status === "running")
      .slice(0, 2)
      .map((run) => ({
        id: `run-${run.id}`,
        title: activityLabel(run.agent),
        detail: "CiteLoop is working on this project right now.",
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
  const homeSections = [
    { id: "needs-attention", label: "Needs attention", count: failedPublish.length, priority: 100, href: `/projects/${projectId}/publish` },
    { id: "activity-warnings", label: "Activity warning summary", count: automationWarnings.length, priority: 90, href: `/projects/${projectId}/settings/activity` },
    { id: "needs-review", label: "Needs review", count: reviewArticles.length, priority: hasBlockedDrafts ? 85 : 80, href: `/projects/${projectId}/review` },
    { id: "ready-to-distribute", label: "Ready to distribute", count: ready.length, priority: 70, href: `/projects/${projectId}/publish` },
    { id: "this-week", label: "This week", count: scheduledRows.length, priority: 40, href: `/projects/${projectId}/publish` },
    { id: "waiting-canonical", label: "Waiting on canonical", count: waitingVariants.length, priority: 30, href: `/projects/${projectId}/publish` },
  ];
  const sectionBudget = visibleHomeSectionIds(homeSections, { limit: 2 });
  const visibleSectionIds = new Set(sectionBudget.visibleIds);
  const overflowSections = homeSections.filter((section) => sectionBudget.overflowIds.includes(section.id));

  return (
    <div className="space-y-5">
      {apiError && (
        <Notice
          title="API server unavailable"
          detail={`Dashboard data could not be loaded (${apiError}). The frontend shell still renders for Vercel verification.`}
          tone="amber"
        />
      )}
      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}

      <section className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div className="flex items-center gap-2">
              <Badge tone="green">Growth Overview</Badge>
              <span className="text-xs font-semibold text-slate-400">{project?.name ?? "Project"}</span>
            </div>
            <h1 className="mt-4 max-w-[760px] text-3xl font-bold leading-9 tracking-tight text-slate-950 md:text-4xl md:leading-[2.7rem]">
              {growthHeadline}
            </h1>
            <p className="mt-3 max-w-[70ch] text-sm leading-6 text-slate-600">{growthDetail}</p>
          </div>
          <Button disabled={!!busy} size="sm" onClick={() => refresh()}>
            <RefreshCw size={14} />
            Refresh
          </Button>
        </div>

        {contextBuild.active && (
          <div
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
                <div key={track.id} className="min-h-[118px] rounded-lg bg-white px-3 py-3 ring-1 ring-amber-100">
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
                  <div
                    className="mt-3 h-2 overflow-hidden rounded-full bg-amber-50 ring-1 ring-inset ring-amber-100"
                    role="progressbar"
                    aria-label={`${track.label} progress`}
                    aria-valuemin={0}
                    aria-valuemax={100}
                    aria-valuenow={track.progress}
                  >
                    <div
                      className={cx(
                        "h-full rounded-full transition-all duration-500",
                        track.state === "done" ? "bg-green-600" : track.state === "attention" ? "bg-amber-500" : "bg-[#d93820]",
                      )}
                      style={{ width: `${track.progress}%` }}
                    />
                  </div>
                  <div className="mt-2 flex items-center justify-between gap-3 text-xs font-bold text-slate-400">
                    <span>{track.current}/{track.target}</span>
                    <span className="capitalize">{track.state}</span>
                  </div>
                  <p className="mt-2 text-[13px] font-semibold leading-5 text-slate-500">{track.detail}</p>
                </div>
              ))}
            </div>
          </div>
        )}

        <div className="grid gap-4 xl:grid-cols-[minmax(0,1.6fr)_minmax(360px,1fr)]">
          <div className="relative min-h-[240px] overflow-hidden rounded-[18px] border border-slate-200 bg-white p-5">
            <div className="relative z-10 text-xs font-bold uppercase tracking-wide text-slate-400">Growth impact</div>
            <div className="relative z-10 mt-2 flex flex-wrap items-baseline gap-3">
              <div className={cx("text-4xl font-bold leading-none", searchDataConnected ? "text-slate-950" : "text-slate-500")}>
                {searchDataConnected ? metric(clicks28d) : "Limited"}
              </div>
              {searchDataConnected && <span className="text-sm font-bold text-emerald-600">measuring</span>}
            </div>
            <p className="relative z-10 mt-2 max-w-[54ch] text-sm leading-6 text-slate-500">{growthDetail}</p>
            <svg
              aria-label="Growth metric trend"
              viewBox="0 0 400 120"
              className="pointer-events-none absolute inset-x-0 bottom-0 z-0 h-[96px] w-full text-sky-500 sm:h-[135px]"
              preserveAspectRatio="none"
            >
              <defs>
                <linearGradient id="growthMetricFill" x1="0" x2="0" y1="0" y2="1">
                  <stop offset="0%" stopColor="currentColor" stopOpacity="0.38" />
                  <stop offset="100%" stopColor="currentColor" stopOpacity="0.04" />
                </linearGradient>
              </defs>
              <path d={`${growthTrendPath} L 396 120 L 4 120 Z`} fill="url(#growthMetricFill)" />
              <path d={growthTrendPath} fill="none" stroke="currentColor" strokeLinecap="round" strokeWidth="4" />
            </svg>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            {growthMetricCards.map((item) => {
              const MetricIcon = item.icon;
              return (
                <div key={item.label} className={cx("rounded-[18px] border border-slate-200 bg-white p-4", item.muted && "text-slate-500")}>
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex items-center gap-2 text-sm font-bold text-slate-500">
                      <MetricIcon size={17} className={item.muted ? "text-slate-300" : "text-[#d93820]"} />
                      {item.label}
                    </div>
                    {!item.muted && <span className="text-xs font-bold text-emerald-600">active</span>}
                  </div>
                  <div className={cx("mt-4 text-2xl font-bold leading-none", item.muted ? "text-slate-500" : "text-slate-950")}>{item.value}</div>
                  <div className="mt-2 text-sm font-semibold leading-5 text-slate-400">{item.detail}</div>
                </div>
              );
            })}
          </div>
        </div>

      </section>

      <section>
        <SectionHeader title="Growth loop" eyebrow="How CiteLoop turns work into measurable growth" />
        <div className="grid gap-4 lg:grid-cols-[1fr_1fr_1fr] lg:grid-rows-[auto_auto_auto] lg:gap-x-14 lg:gap-y-14">
          {loopCards.map((card) => {
            const ConnectorIcon = loopConnectorIcon(card.connectorDirection);
            return (
              <a
                key={card.position}
                data-loop-position={card.position}
                href={card.href}
                className={cx(
                  "group relative min-h-[112px] rounded-lg border border-slate-200 bg-white px-4 py-3 text-sm transition-colors hover:border-slate-300 hover:bg-slate-50",
                  loopGridClass(card.position),
                )}
              >
                <span className="sr-only">{card.connectorLabel}</span>
                <span
                  aria-hidden="true"
                  className={cx(
                    "pointer-events-none absolute z-10 hidden h-10 w-10 items-center justify-center rounded-full border border-red-100 bg-red-50 text-[#d93820] shadow-[0_10px_22px_-16px_rgba(217,56,32,0.85)] transition-colors group-hover:border-[#d93820] group-hover:bg-[#d93820] group-hover:text-white lg:flex",
                    loopConnectorClass(card.connectorDirection),
                  )}
                >
                  <ConnectorIcon size={20} />
                </span>
                <div className="grid grid-cols-[2rem_1fr_2rem] items-center gap-2">
                  <span className="inline-flex h-7 min-w-7 items-center justify-center rounded-md border border-slate-200 bg-slate-50 px-2 text-xs font-bold text-slate-500">
                    {card.position}
                  </span>
                  <div className="text-center text-base font-bold leading-5 text-slate-950">{card.label}</div>
                  <span aria-hidden="true" className="h-7 w-7" />
                </div>
                <div className="mt-3 space-y-1.5 text-center">
                  {card.statusLines.map((line, index) =>
                    "value" in line ? (
                      <div key={`${card.position}-${index}`} className="flex items-baseline justify-center gap-2">
                        <span className={cx("text-xl font-bold leading-none tracking-normal", card.accentClass)}>{line.value}</span>
                        <span className="text-sm font-semibold leading-5 text-slate-500">{line.label}</span>
                      </div>
                    ) : (
                      <div key={`${card.position}-${index}`} className="text-sm font-semibold leading-5 text-slate-500">
                        {line.text}
                      </div>
                    ),
                  )}
                </div>
              </a>
            );
          })}
        </div>
      </section>

      <section className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div>
          <SectionHeader title="Recent growth signals" eyebrow="What changed recently" />
          {loopItems.length === 0 ? (
            <EmptyState
              title="No growth signals yet"
              detail="Opportunities, drafts, published pages, and measured outcomes will appear here as the loop starts moving."
            />
          ) : (
            <div className="grid gap-2">
              {loopItems.map((item) => (
                <a
                  key={item.id}
                  href={item.href}
                  className="flex min-h-[46px] items-center justify-between gap-3 rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm transition-colors hover:bg-slate-50"
                >
                  <div className="min-w-0">
                    <div className="truncate font-semibold text-slate-900">{item.title}</div>
                    <div className="mt-0.5 text-[13px] font-semibold text-slate-400">{item.stage}</div>
                  </div>
                  <Badge tone={item.tone}>growth loop</Badge>
                </a>
              ))}
            </div>
          )}
        </div>
        <div>
          <SectionHeader title="CiteLoop knowledge" />
          <div className="rounded-xl border border-slate-200 bg-white px-4 py-3">
            <div className="flex items-center justify-between gap-3">
              <div className="font-semibold text-slate-900">{contextHealth.label}</div>
              <Badge tone={contextHealth.tone}>Evidence coverage</Badge>
            </div>
            <p className="mt-2 text-sm leading-5 text-slate-600">{contextHealth.detail}</p>
            <div className="mt-4 grid grid-cols-2 gap-2 text-sm">
              <div className="rounded-lg bg-slate-50 px-3 py-2">
                <div className="text-xs font-bold uppercase text-slate-400">Source pages</div>
                <div className="mt-1 text-lg font-bold text-slate-900">{sourcePageCount}</div>
              </div>
              <div className="rounded-lg bg-slate-50 px-3 py-2">
                <div className="text-xs font-bold uppercase text-slate-400">Evidence</div>
                <div className="mt-1 text-lg font-bold text-slate-900">{contextEvidenceCount}</div>
              </div>
            </div>
            <a href={`/projects/${projectId}/context`} className="mt-4 inline-flex items-center gap-2 text-sm font-semibold text-[#d93820]">
              Open context
              <ArrowRight size={14} />
            </a>
          </div>
        </div>
      </section>

      <section>
        <SectionHeader title="Event stream" eyebrow="Now, recent, next" />
        {eventStream.items.length === 0 && eventStream.emptyAction ? (
          <EmptyState title={eventStream.emptyAction.title} detail={eventStream.emptyAction.detail} />
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

      {visibleSectionIds.has("needs-attention") && (
        <section>
          <SectionHeader title="Needs attention" action={<Badge tone="red">{failedPublish.length}</Badge>} />
          <div className="grid gap-2">
            {failedPublish.slice(0, 3).map((article) => (
              <div key={article.id} className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm">
                <div className="font-bold text-red-950">{articleTitle(article)}</div>
                <div className="mt-1 line-clamp-2 text-red-800">{article.last_publish_error || "No publish error captured."}</div>
                <a href={`/projects/${projectId}/publish`} className="mt-2 inline-block text-xs font-semibold text-red-700">
                  Open publish
                </a>
              </div>
            ))}
          </div>
        </section>
      )}

      {visibleSectionIds.has("activity-warnings") && (
        <section>
          <SectionHeader title="Activity warning summary" action={<a href={`/projects/${projectId}/settings/activity`} className="text-xs font-semibold text-slate-500">Activity log</a>} />
          <div className="grid gap-2">
            {automationWarnings.map((run) => (
              <div
                key={run.id}
                className="flex min-h-[44px] flex-col gap-2 rounded-lg border border-slate-200 bg-white px-4 py-3 text-sm sm:flex-row sm:items-center sm:justify-between"
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-semibold text-slate-900">{activityLabel(run.agent)}</span>
                    <Badge tone={activityTone(run.status, Boolean(run.output?.degraded))}>{run.status}</Badge>
                    {run.output?.degraded && <Badge tone="amber">degraded</Badge>}
                  </div>
                  <div className="mt-1 truncate text-xs text-slate-500">{run.error ?? "Limited quality. Open activity log for details."}</div>
                </div>
                <div className="flex shrink-0 items-center gap-3 text-xs font-semibold text-slate-400">
                  <span>{formatDate(run.created_at)}</span>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {visibleSectionIds.has("needs-review") && (
        <section>
          <SectionHeader title="Needs review" action={<Badge tone="amber">{reviewArticles.length}</Badge>} />
          <div className="columns-1 gap-3 sm:columns-2">
            {reviewArticles.slice(0, 4).map((article) => (
              <div
                key={article.id}
                className="mb-3 break-inside-avoid rounded-xl border border-slate-200 bg-white px-4 py-3"
              >
                <div className="mb-3 flex items-center gap-2">
                  <Badge tone={article.kind === "canonical" ? "green" : "neutral"}>
                    {article.platform || article.kind}
                  </Badge>
                </div>
                <div className="content-font text-[15px] font-semibold leading-5 text-slate-900">
                  {articleTitle(article)}
                </div>
                {article.qa_blocking && (
                  <div className="mt-2 rounded-md border border-red-100 bg-red-50 px-3 py-2 text-xs font-semibold text-red-800">
                    Cannot approve: {article.qa_issues[0] || "QA has not cleared this draft"}
                  </div>
                )}
                <p className="mt-2 line-clamp-3 content-font text-[15px] leading-5 text-slate-700">
                  {article.content_md}
                </p>
                <div className="mt-3 flex items-center justify-between text-xs text-slate-500">
                  <span>
                    geo {formatScore(article.geo_score)} / seo {formatScore(article.seo_score)}
                  </span>
                  <a href={`/projects/${projectId}/review`} className="font-semibold text-[#d93820]">
                    Open review
                  </a>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {visibleSectionIds.has("ready-to-distribute") && (
        <section>
          <SectionHeader title="Ready to distribute" action={<Badge tone="green">{ready.length}</Badge>} />
          <div className="grid gap-3 sm:grid-cols-2">
            {ready.map(({ article, compose_url, supports_canonical }) => (
              <div key={article.id} className="rounded-xl border border-slate-200 bg-white px-4 py-3">
                <div className="flex items-center justify-between gap-2">
                  <Badge tone="amber">{article.platform ?? "platform"}</Badge>
                  <span className="text-xs font-semibold text-slate-400">
                    {supports_canonical ? "canonical tag" : "source link"}
                  </span>
                </div>
                <div className="mt-3 content-font text-[15px] font-semibold leading-5 text-slate-900">
                  {articleTitle(article)}
                </div>
                <div className="mt-3 flex flex-wrap gap-2">
                  <Button
                    size="sm"
                    onClick={async () => {
                      try {
                        await navigator.clipboard?.writeText(article.content_md);
                        setMessage({ tone: "green", title: "Copied to clipboard" });
                      } catch {
                        setMessage({ tone: "red", title: "Copy failed", detail: "Clipboard is unavailable in this browser." });
                      }
                    }}
                  >
                    <Copy size={14} />
                    Copy
                  </Button>
                  {compose_url && (
                    <a
                      href={compose_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex h-8 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50"
                    >
                      <ExternalLink size={14} />
                      Compose
                    </a>
                  )}
                  <Button
                    size="sm"
                    onClick={() => {
                      const ok = window.confirm("Mark this variant as distributed? This records it as posted and removes it from the ready list.");
                      if (!ok) return;
                      run("Distributed", () => api.distributed(projectId, article.id), "Marked as distributed");
                    }}
                  >
                    Mark distributed
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {visibleSectionIds.has("this-week") && (
        <section>
          <SectionHeader title="This week" eyebrow="Content rhythm" />
          <div className="grid gap-2">
            {scheduledRows.map((row) => (
              <div
                key={row.id}
                className="flex min-h-[38px] items-center justify-between gap-3 rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm transition-colors hover:bg-slate-50"
              >
                <div className="min-w-0">
                  <div className="truncate font-semibold text-slate-800">{row.title}</div>
                  <div className="text-[13px] font-semibold text-slate-400">{formatDate(row.time)}</div>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <Badge tone="blue">{row.type}</Badge>
                  <Badge tone="green">{row.status}</Badge>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {visibleSectionIds.has("waiting-canonical") && (
        <section>
          <SectionHeader title="Waiting on canonical" />
          <div className="grid gap-2">
            {waitingVariants.map((article) => (
              <div key={article.id} className="rounded-lg border border-slate-200 bg-white px-4 py-2 text-sm">
                <span className="font-semibold text-slate-800">{articleTitle(article)}</span>
                <span className="ml-2 text-slate-400">waiting for canonical URL</span>
              </div>
            ))}
          </div>
        </section>
      )}

      {overflowSections.length > 0 && (
        <section>
          <SectionHeader title="More waiting" eyebrow="Collapsed to keep Home focused" />
          <div className="grid gap-2 sm:grid-cols-2">
            {overflowSections.map((section) => (
              <a
                key={section.id}
                href={section.href}
                className="flex items-center justify-between gap-3 rounded-lg border border-slate-200 bg-white px-4 py-3 text-sm font-semibold text-slate-700 hover:bg-slate-50 hover:text-[#d93820]"
              >
                <span>{section.label}</span>
                <Badge tone="neutral">{section.count}</Badge>
              </a>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
