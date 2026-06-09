"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { ChevronRight, Copy, ExternalLink, RefreshCw, Wand2 } from "lucide-react";
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
import { nextWorkspaceAction, visibilityLifecycleLabel, visibilityLifecycleTone } from "../../lib/dashboard-ux-logic";
import { useApi } from "../../lib/use-api";
import { Badge, Button, EmptyState, Notice, SectionHeader, TextInput, formatDate, formatScore } from "../../components/ui";

type Message = { tone: "neutral" | "red" | "green" | "amber"; title: string; detail?: string } | null;

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

export function Workspace({ projectId }: { projectId: string }) {
  const api = useApi();
  const [landing, setLanding] = useState("");
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
  const [seoOverview, setSeoOverview] = useState<SEOOverview | null>(null);
  const [seoOpportunities, setSeoOpportunities] = useState<SEOOpportunity[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);
  const [apiError, setApiError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setApiError(null);
    try {
      const [p, profileRow, inventoryRows, t, r, pub, app, failed, dist, runRows, overview, opportunities] = await Promise.all([
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
      setSeoOverview(overview);
      setSeoOpportunities(opportunities);
    } catch (e: any) {
      setApiError(e.message);
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

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
    if (rows.length > 0) return rows.slice(0, 5);

    const cadence = project?.config?.cadence_per_week ?? 3;
    return Array.from({ length: Math.min(cadence, 4) }, (_, index) => ({
      id: `empty-${index}`,
      time: null,
      title: "Open content slot",
      status: "empty",
      type: "slot",
    }));
  }, [approved, project?.config?.cadence_per_week, topics]);

  const waitingVariants = approved.filter(
    (article) => article.kind === "syndication_variant" && !ready.some((item) => item.article.id === article.id),
  );
  const automationWarnings = runs.filter((run) => ["error", "failed"].includes(run.status) || Boolean(run.output?.degraded));
  const hasBlockedDrafts = reviewArticles.some((article) => article.qa_blocking);
  const nextAction = useMemo(() => {
    return nextWorkspaceAction({
      projectId,
      hasProfile: Boolean(profile),
      failedPublishCount: failedPublish.length,
      hasBlockedDrafts,
      reviewCount: reviewArticles.length,
      readyCount: ready.length,
      topicsCount: topics.length,
    });
  }, [failedPublish.length, hasBlockedDrafts, profile, projectId, ready.length, reviewArticles.length, topics.length]);

  const alsoWaiting = [
    reviewArticles.length > 0 && { label: `${reviewArticles.length} drafts need review`, href: `/projects/${projectId}/review`, tone: "amber" as const },
    ready.length > 0 && { label: `${ready.length} variants ready`, href: `/projects/${projectId}/publish`, tone: "green" as const },
    failedPublish.length > 0 && { label: `${failedPublish.length} publishing issues`, href: `/projects/${projectId}/publish`, tone: "red" as const },
    automationWarnings.length > 0 && { label: `${automationWarnings.length} automation warnings`, href: `/projects/${projectId}/settings/activity`, tone: "amber" as const },
    topics.length === 0 && { label: "No content plan yet", href: `/projects/${projectId}/plan`, tone: "neutral" as const },
  ].filter(Boolean) as Array<{ label: string; href: string; tone: "neutral" | "red" | "amber" | "green" }>;

  const contextEvidenceCount = evidenceCount(inventory);
  const sourcePageCount = Math.max(inventory.length, profile?.source_urls?.length ?? 0);
  const contextHealth = !profile
    ? {
        label: "Needs context",
        tone: "amber" as const,
        detail: "Refresh context so CiteLoop can extract product facts and evidence from this domain.",
      }
    : sourcePageCount === 0
      ? {
          label: "Incomplete",
          tone: "amber" as const,
          detail: "Context exists, but CiteLoop has not captured source pages yet.",
        }
      : contextEvidenceCount === 0
        ? {
            label: "Needs evidence",
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
  const activeLoopCount =
    seoOpportunities.filter((opportunity) => !["dismissed", "archived"].includes(opportunity.status)).length +
    reviewArticles.length +
    ready.length;
  const momentumItems = [
    {
      label: "Published this month",
      value: published.filter((article) => isThisMonth(article.published_at)).length,
      detail: "canonical articles live",
    },
    { label: "Drafts approved", value: approved.length, detail: "ready for publish or distribution" },
    { label: "Opportunities converted", value: opportunitiesConverted, detail: "visibility signals entered the content loop" },
    { label: "Active loop items", value: activeLoopCount, detail: "moving through plan, review, publish, or visibility" },
  ];
  const visibilityCapability =
    seoOverview?.capability_mode && seoOverview.capability_mode !== "public_only"
      ? "Verified search data is available for visibility reporting."
      : "Search Console is not connected yet. CiteLoop is tracking public crawl and content progress only.";
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

  return (
    <div className="space-y-7">
      <button className="flex h-9 w-full items-center justify-between rounded-lg text-left text-sm font-semibold text-slate-400 transition-colors hover:text-slate-600">
        Show learning resources
        <ChevronRight size={16} />
      </button>

      {apiError && (
        <Notice
          title="API server unavailable"
          detail={`Dashboard data could not be loaded (${apiError}). The frontend shell still renders for Vercel verification.`}
          tone="amber"
        />
      )}
      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}

      <section>
        <div className="rounded-xl border border-slate-200 bg-white p-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <Badge tone="blue">Next action</Badge>
              <span className="text-xs font-semibold text-slate-400">Why this</span>
            </div>
            <Button disabled={!!busy} size="sm" onClick={() => refresh()}>
              <RefreshCw size={14} />
              Refresh
            </Button>
          </div>
          <div className="mt-4 grid gap-4 lg:grid-cols-[1fr_280px]">
            <div>
              <h2 className="text-2xl font-bold leading-8 text-slate-950">{nextAction.title}</h2>
              <p className="mt-2 max-w-[64ch] text-sm leading-6 text-slate-600">{nextAction.detail}</p>
              <div className="mt-4 grid gap-2 md:grid-cols-[1fr_auto_auto]">
                <TextInput
                  value={landing}
                  onChange={(event) => setLanding(event.target.value)}
                  placeholder="https://product-domain.com"
                  className="w-full"
                />
                <Button
                  disabled={!!busy || !landing.trim()}
                  variant="primary"
                  onClick={() => run("Context refresh", () => api.runInsight(projectId, landing.trim()), "Context refreshed; crawl may continue in background")}
                >
                  <Wand2 size={16} />
                  Refresh context
                </Button>
                <Button
                  disabled={!!busy || !profile}
                  title={!profile ? "Refresh context before generating a content plan" : undefined}
                  onClick={() => run("Content plan", () => api.runStrategist(projectId), "Content plan generated")}
                >
                  Generate content plan
                </Button>
              </div>
            </div>
            <div className="rounded-lg bg-slate-50 px-3 py-3">
              <div className="text-xs font-bold uppercase text-slate-400">Also waiting</div>
              <div className="mt-2 grid gap-2">
                {alsoWaiting.length === 0 ? (
                  <div className="text-sm text-slate-500">No urgent queues. Keep context fresh for the next cycle.</div>
                ) : (
                  alsoWaiting.map((item) => (
                    <a key={item.label} href={item.href} className="flex items-center justify-between gap-2 text-sm font-semibold text-slate-700 hover:text-[#d93820]">
                      <span>{item.label}</span>
                      <Badge tone={item.tone}>open</Badge>
                    </a>
                  ))
                )}
              </div>
            </div>
          </div>
        </div>
      </section>

      <section>
        <div className="grid gap-4 lg:grid-cols-[1fr_320px]">
          <div>
            <SectionHeader title="Results / Momentum" eyebrow={project?.name ?? "Project"} />
            <div className="grid gap-3 sm:grid-cols-2">
              {momentumItems.map((item) => (
                <div key={item.label} className="rounded-xl border border-slate-200 bg-white px-4 py-3">
                  <div className="text-[13px] font-bold text-slate-500">{item.label}</div>
                  <div className="mt-2 text-3xl font-bold leading-none text-slate-950">{item.value}</div>
                  <div className="mt-2 text-sm leading-5 text-slate-500">{item.detail}</div>
                </div>
              ))}
            </div>
            <div className="mt-3 rounded-lg border border-slate-200 bg-slate-50 px-4 py-3 text-sm leading-5 text-slate-600">
              {visibilityCapability}
            </div>
          </div>

          <div>
            <SectionHeader title="Context health" />
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
              <a href={`/projects/${projectId}/context`} className="mt-4 inline-flex text-sm font-semibold text-[#d93820]">
                Open Context
              </a>
            </div>
          </div>
        </div>
      </section>

      <section>
        <SectionHeader title="Loop progress" eyebrow="Visibility to content loop" />
        {loopItems.length === 0 ? (
          <EmptyState
            title="No active loop items yet"
            detail="Visibility opportunities will appear here after they are detected, added to Content Plan, drafted, published, and measured."
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
                <Badge tone={item.tone}>loop</Badge>
              </a>
            ))}
          </div>
        )}
      </section>

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
                <Badge tone={row.status === "empty" ? "neutral" : "blue"}>{row.type}</Badge>
                <Badge tone={row.status === "empty" ? "amber" : "green"}>{row.status}</Badge>
              </div>
            </div>
          ))}
        </div>
      </section>

      <section>
        <SectionHeader title="Needs attention" action={<Badge tone={failedPublish.length ? "red" : "neutral"}>{failedPublish.length}</Badge>} />
        {failedPublish.length === 0 ? (
          <EmptyState title="No publish failures" detail="Publish failures will appear here without checking server logs." />
        ) : (
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
        )}
      </section>

      <section>
        <SectionHeader title="Needs review" action={<Badge tone={reviewArticles.length ? "amber" : "neutral"}>{reviewArticles.length}</Badge>} />
        {reviewArticles.length === 0 ? (
          <EmptyState title="Nothing pending review" detail="Generated drafts that need the human gate will appear here." />
        ) : (
          <div className="columns-1 gap-3 sm:columns-2">
            {reviewArticles.map((article) => (
              <div
                key={article.id}
                className="mb-3 break-inside-avoid rounded-xl border border-slate-200 bg-white px-4 py-3"
              >
                <div className="mb-3 flex items-center gap-2">
                  <Badge tone={article.kind === "canonical" ? "green" : "neutral"}>
                    {article.platform || article.kind}
                  </Badge>
                  {article.qa_blocking && <Badge tone="red">qa blocking</Badge>}
                </div>
                <div className="content-font text-[15px] font-semibold leading-5 text-slate-900">
                  {articleTitle(article)}
                </div>
                <p className="mt-2 line-clamp-4 content-font text-[15px] leading-5 text-slate-700">
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
        )}
      </section>

      <section>
        <SectionHeader title="Ready to distribute" action={<Badge tone={ready.length ? "green" : "neutral"}>{ready.length}</Badge>} />
        {ready.length === 0 ? (
          <EmptyState
            title="No variants ready"
            detail="Variants unlock only after the canonical article is published and canonical_url is available."
          />
        ) : (
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
                  <Button size="sm" onClick={() => navigator.clipboard?.writeText(article.content_md)}>
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
                  <Button size="sm" onClick={() => run("Distributed", () => api.distributed(projectId, article.id))}>
                    Mark distributed
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      {automationWarnings.length > 0 && (
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

      {waitingVariants.length > 0 && (
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
    </div>
  );
}
