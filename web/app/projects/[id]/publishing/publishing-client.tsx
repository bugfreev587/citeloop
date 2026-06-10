"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Copy, ExternalLink, RefreshCw, RotateCcw, Send } from "lucide-react";
import { Article, DistributeItem } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, EmptyState, Notice, SectionHeader, formatDate } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;

function articleTitle(article: Article) {
  return article.seo_meta?.title || article.seo_meta?.slug || `${article.kind} article`;
}

export function PublishingClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [published, setPublished] = useState<Article[]>([]);
  const [approved, setApproved] = useState<Article[]>([]);
  const [failed, setFailed] = useState<Article[]>([]);
  const [ready, setReady] = useState<DistributeItem[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);

  const refresh = useCallback(async () => {
    try {
      const [pub, app, fail, dist] = await Promise.all([
        api.listArticles(projectId, "published"),
        api.listArticles(projectId, "approved"),
        api.listArticles(projectId, "publish_failed"),
        api.listDistribute(projectId),
      ]);
      setPublished(pub);
      setApproved(app);
      setFailed(fail);
      setReady(dist);
      return { pub, app, fail, dist };
    } catch (e: any) {
      setMessage({ title: "Publishing data unavailable", detail: e.message, tone: "amber" });
      return null;
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const waiting = useMemo(
    () =>
      approved.filter(
        (article) =>
          article.kind === "syndication_variant" && !ready.some((item) => item.article.id === article.id),
      ),
    [approved, ready],
  );

  async function markDistributed(article: Article) {
    setBusy(article.id);
    setMessage(null);
    try {
      await api.distributed(projectId, article.id);
      await refresh();
      setMessage({ title: "Variant marked distributed", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not mark distributed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function retryPublish(article: Article) {
    setBusy(article.id);
    setMessage(null);
    try {
      await api.retryPublish(projectId, article.id);
      await refresh();
      setMessage({ title: "Publish retry queued", detail: articleTitle(article), tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not retry publish", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function reconcile() {
    setBusy("reconcile");
    setMessage(null);
    try {
      await api.reconcilePublishing(projectId);
      const data = await refresh();
      if (data) {
        const waitingCount = data.app.filter(
          (article) => article.kind === "syndication_variant" && !data.dist.some((item) => item.article.id === article.id),
        ).length;
        setMessage({
          tone: data.fail.length ? "amber" : "green",
          title: "Publishing checked",
          detail: `${data.pub.length} published · ${data.dist.length} ready to distribute · ${waitingCount} waiting on canonical · ${data.fail.length} failed.`,
        });
      } else {
        setMessage({ title: "Publishing checked", tone: "green" });
      }
    } catch (e: any) {
      setMessage({ title: "Reconcile failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="space-y-7">
      <SectionHeader
        title="Publishing"
        eyebrow="Canonical and syndication lanes"
        action={
          <div className="flex flex-wrap gap-2">
            <Button disabled={!!busy} size="sm" onClick={reconcile}>
              <RotateCcw size={14} />
              Reconcile
            </Button>
            <Button disabled={!!busy} size="sm" onClick={refresh}>
              <RefreshCw size={14} />
              Refresh
            </Button>
          </div>
        }
      />
      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}

      <section>
        <SectionHeader title="Publish failures" action={<Badge tone={failed.length ? "red" : "neutral"}>{failed.length}</Badge>} />
        {failed.length === 0 ? (
          <EmptyState title="No publish failures" detail="Failed canonical publish attempts will appear here with retry controls." />
        ) : (
          <div className="grid gap-2">
            {failed.map((article) => (
              <div key={article.id} className="rounded-lg border border-red-200 bg-red-50 px-4 py-3">
                <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                  <div className="min-w-0">
                    <div className="truncate text-sm font-bold text-red-950">{articleTitle(article)}</div>
                    <div className="mt-1 text-xs font-semibold text-red-700">
                      attempt {article.publish_attempts} · next retry {formatDate(article.next_publish_retry_at)}
                    </div>
                    <div className="mt-2 line-clamp-3 text-sm leading-5 text-red-800">
                      {article.last_publish_error || "No publish error captured."}
                    </div>
                    {article.publish_path && <div className="mt-1 truncate text-xs text-red-700">{article.publish_path}</div>}
                  </div>
                  <div className="flex shrink-0 flex-wrap gap-2">
                    <a
                      href={`/projects/${projectId}/articles/${article.id}`}
                      className="inline-flex h-8 items-center rounded-lg border border-red-200 bg-white px-3 text-xs font-semibold text-red-700 hover:bg-red-50"
                    >
                      Detail
                    </a>
                    <Button disabled={busy === article.id} size="sm" variant="danger" onClick={() => retryPublish(article)}>
                      <RotateCcw size={14} />
                      Retry
                    </Button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <section>
        <SectionHeader title="Published canonical" action={<Badge tone="green">{published.length}</Badge>} />
        {published.length === 0 ? (
          <EmptyState title="No canonical articles published" detail="Approved canonical articles publish automatically when due." />
        ) : (
          <div className="grid gap-2">
            {published.map((article) => (
              <div key={article.id} className="rounded-lg border border-slate-200 bg-white px-4 py-3">
                <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                  <div className="min-w-0">
                    <div className="truncate text-sm font-bold text-slate-900">{articleTitle(article)}</div>
                    <div className="mt-1 text-xs text-slate-500">Published {formatDate(article.published_at)}</div>
                  </div>
                  {article.canonical_url ? (
                    <a
                      href={article.canonical_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex h-8 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-[#d93820] hover:bg-slate-50"
                    >
                      <ExternalLink size={14} />
                      Live article
                    </a>
                  ) : (
                    <Badge tone="amber">missing canonical_url</Badge>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <section>
        <SectionHeader title="Ready to distribute" action={<Badge tone={ready.length ? "green" : "neutral"}>{ready.length}</Badge>} />
        {ready.length === 0 ? (
          <EmptyState title="No variants ready" detail="Approved variants unlock after canonical publish and canonical_url backfill." />
        ) : (
          <div className="grid gap-3 sm:grid-cols-2">
            {ready.map(({ article, compose_url, supports_canonical }) => (
              <article key={article.id} className="rounded-xl border border-slate-200 bg-white px-4 py-3">
                <div className="flex items-center justify-between gap-2">
                  <Badge tone="amber">{article.platform ?? "platform"}</Badge>
                  <span className="text-xs font-semibold text-slate-400">
                    {supports_canonical ? "canonical tag supported" : "source link in body"}
                  </span>
                </div>
                <h3 className="mt-3 content-font text-[15px] font-semibold leading-5 text-slate-900">{articleTitle(article)}</h3>
                <p className="mt-2 line-clamp-4 content-font text-[15px] leading-5 text-slate-700">{article.content_md}</p>
                <div className="mt-3 flex flex-wrap gap-2">
                  <Button size="sm" onClick={() => navigator.clipboard?.writeText(article.content_md)}>
                    <Copy size={14} />
                    Copy variant
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
                  <Button disabled={busy === article.id} size="sm" variant="primary" onClick={() => markDistributed(article)}>
                    <Send size={14} />
                    Mark distributed
                  </Button>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>

      <section>
        <SectionHeader title="Waiting on canonical" action={<Badge tone="neutral">{waiting.length}</Badge>} />
        {waiting.length === 0 ? (
          <EmptyState title="No variants waiting" detail="Approved variants waiting for canonical publication will be shown here." />
        ) : (
          <div className="grid gap-2">
            {waiting.map((article) => (
              <div key={article.id} className="rounded-lg border border-slate-200 bg-white px-4 py-3 text-sm">
                <div className="font-bold text-slate-900">{articleTitle(article)}</div>
                <div className="mt-1 text-slate-500">
                  {article.platform ?? "platform"} is approved. It unlocks automatically once its canonical article is published and its live URL is confirmed.
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <Notice
        title="Manual distribution only"
        detail="Mark distributed records user completion. It does not publish to the third-party platform automatically."
      />
    </div>
  );
}
