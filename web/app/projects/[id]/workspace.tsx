"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { ChevronRight, Copy, ExternalLink, RefreshCw, Wand2 } from "lucide-react";
import { Article, DistributeItem, GenerationRun, Project, ReviewGroup, Topic } from "../../lib/api";
import { useApi } from "../../lib/use-api";
import { Badge, Button, EmptyState, Notice, SectionHeader, TextInput, formatDate, formatScore } from "../../components/ui";

type Message = { tone: "neutral" | "red" | "green" | "amber"; title: string; detail?: string } | null;

function articleTitle(article: Article) {
  return article.seo_meta?.title || article.seo_meta?.slug || `${article.kind} draft`;
}

function topicLabel(topic: Topic) {
  return topic.title || "Untitled topic";
}

function runStatusTone(status: string): "green" | "red" | "amber" | "neutral" {
  if (status === "ok") return "green";
  if (status === "error" || status === "failed") return "red";
  if (status === "running") return "amber";
  return "neutral";
}

function money(value: number | null) {
  if (value == null) return "-";
  return `$${value.toFixed(4)}`;
}

export function Workspace({ projectId }: { projectId: string }) {
  const api = useApi();
  const [landing, setLanding] = useState("");
  const [project, setProject] = useState<Project | null>(null);
  const [topics, setTopics] = useState<Topic[]>([]);
  const [review, setReview] = useState<ReviewGroup[]>([]);
  const [published, setPublished] = useState<Article[]>([]);
  const [approved, setApproved] = useState<Article[]>([]);
  const [failedPublish, setFailedPublish] = useState<Article[]>([]);
  const [ready, setReady] = useState<DistributeItem[]>([]);
  const [runs, setRuns] = useState<GenerationRun[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);
  const [apiError, setApiError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setApiError(null);
    try {
      const [p, t, r, pub, app, failed, dist, runRows] = await Promise.all([
        api.getProject(projectId),
        api.listTopics(projectId),
        api.listReview(projectId),
        api.listArticles(projectId, "published"),
        api.listArticles(projectId, "approved"),
        api.listArticles(projectId, "publish_failed"),
        api.listDistribute(projectId),
        api.listRuns(projectId, { limit: 5 }),
      ]);
      setProject(p);
      setTopics(t);
      setReview(r);
      setPublished(pub);
      setApproved(app);
      setFailedPublish(failed);
      setReady(dist);
      setRuns(runRows);
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
        <SectionHeader
          title="Pipeline"
          action={
            <Button disabled={!!busy} size="sm" onClick={() => refresh()}>
              <RefreshCw size={14} />
              Refresh
            </Button>
          }
        />
        <div className="rounded-xl border border-slate-200 bg-white p-4">
          <div className="grid gap-2 lg:grid-cols-[1fr_auto_auto_auto]">
            <TextInput
              value={landing}
              onChange={(event) => setLanding(event.target.value)}
              placeholder="https://landing-page-url/"
              className="w-full"
            />
            <Button
              disabled={!!busy || !landing}
              variant="primary"
              onClick={() => run("Insight", () => api.runInsight(projectId, landing), "Insight completed")}
            >
              <Wand2 size={16} />
              Run Insight
            </Button>
            <Button disabled={!!busy} onClick={() => run("Strategist", () => api.runStrategist(projectId))}>
              Run Strategist
            </Button>
            <Button disabled={!!busy} onClick={() => run("Publish tick", () => api.tickPublish(projectId))}>
              Publish tick
            </Button>
          </div>
        </div>
      </section>

      <section>
        <SectionHeader title="Next scheduled" eyebrow={project?.name ?? "Project"} />
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
                <a href={`/projects/${projectId}/publishing`} className="mt-2 inline-block text-xs font-semibold text-red-700">
                  Open publishing
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

      <section>
        <SectionHeader title="Recent runs" action={<a href={`/projects/${projectId}/runs`} className="text-xs font-semibold text-slate-500">View all</a>} />
        {runs.length === 0 ? (
          <EmptyState title="No recent runs" detail="Insight, Strategist, Writer, QA, Publisher, and Notification runs will appear here." />
        ) : (
          <div className="grid gap-2">
            {runs.map((run) => (
              <div
                key={run.id}
                className="flex min-h-[44px] flex-col gap-2 rounded-lg border border-slate-200 bg-white px-4 py-3 text-sm sm:flex-row sm:items-center sm:justify-between"
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-semibold text-slate-900">{run.agent}</span>
                    <Badge tone={runStatusTone(run.status)}>{run.status}</Badge>
                    {run.output?.degraded && <Badge tone="amber">degraded</Badge>}
                  </div>
                  <div className="mt-1 truncate text-xs text-slate-500">{run.error ?? run.model ?? "No error"}</div>
                </div>
                <div className="flex shrink-0 items-center gap-3 text-xs font-semibold text-slate-400">
                  <span>{money(run.cost_usd)}</span>
                  <span>{run.tokens ?? 0} tokens</span>
                  <span>{formatDate(run.created_at)}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

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
