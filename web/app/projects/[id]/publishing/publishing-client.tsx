"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { CalendarClock, Copy, ExternalLink, Loader2, RefreshCw, RotateCcw, Send, Zap } from "lucide-react";
import { Article, DistributeItem, ProjectConfig, defaultProjectConfig } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, ButtonProgress, EmptyState, Field, Notice, SectionHeader, cx, formatDate } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;
type PublishMode = "scheduled" | "auto" | "manual";

function articleTitle(article: Article) {
  return article.seo_meta?.title || article.seo_meta?.slug || `${article.kind} article`;
}

export function PublishingClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [published, setPublished] = useState<Article[]>([]);
  const [approved, setApproved] = useState<Article[]>([]);
  const [failed, setFailed] = useState<Article[]>([]);
  const [inflight, setInflight] = useState<Article[]>([]);
  const [ready, setReady] = useState<DistributeItem[]>([]);
  const [config, setConfig] = useState<ProjectConfig | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);

  const refresh = useCallback(async () => {
    try {
      const [pub, app, fail, verifying, dist, project] = await Promise.all([
        api.listArticles(projectId, "published"),
        api.listArticles(projectId, "approved"),
        api.listArticles(projectId, "publish_failed"),
        api.listArticles(projectId, "pending_url_verification"),
        api.listDistribute(projectId),
        api.getProject(projectId).catch(() => null),
      ]);
      setPublished(pub);
      setApproved(app);
      setFailed(fail);
      setInflight(verifying);
      setReady(dist);
      if (project) setConfig(project.config);
      return { pub, app, fail, dist };
    } catch (e: any) {
      setMessage({ title: "Publishing data unavailable", detail: e.message, tone: "amber" });
      return null;
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  // Poll while anything is mid-publish so the page reflects each post going live
  // on its staggered slot without a manual reload.
  useEffect(() => {
    if (inflight.length === 0) return;
    const interval = window.setInterval(refresh, 15_000);
    return () => window.clearInterval(interval);
  }, [inflight.length, refresh]);

  const scheduledCanonicals = useMemo(
    () =>
      approved
        .filter((article) => article.kind === "canonical")
        .sort((a, b) => (a.scheduled_at ?? "").localeCompare(b.scheduled_at ?? "")),
    [approved],
  );
  const publishMode: PublishMode = (config?.publish_mode as PublishMode) ?? "scheduled";
  const publishIntervalDays = config?.publish_interval_days ?? 2;

  async function saveMode(next: Partial<Pick<ProjectConfig, "publish_mode" | "publish_interval_days">>) {
    const base = config ?? defaultProjectConfig();
    setBusy("mode");
    setMessage(null);
    try {
      const updated = await api.updateConfig(projectId, { ...base, ...next });
      setConfig(updated.config);
      setMessage({ title: "Publish schedule updated", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not update publish schedule", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function publishNow(article: Article) {
    setBusy(`publish-${article.id}`);
    setMessage(null);
    try {
      await api.publishNow(projectId, article.id);
      await refresh();
      setMessage({ title: "Queued to publish now", detail: articleTitle(article), tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Could not publish now", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  const waiting = useMemo(
    () =>
      approved.filter(
        (article) =>
          article.kind === "syndication_variant" && !ready.some((item) => item.article.id === article.id),
      ),
    [approved, ready],
  );

  async function markDistributed(article: Article) {
    const ok = window.confirm("Mark this variant as distributed? This records it as posted and removes it from the ready list.");
    if (!ok) return;
    setBusy(`distributed-${article.id}`);
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
    setBusy(`retry-${article.id}`);
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
              <ButtonProgress busy={busy === "reconcile"} busyLabel="Reconciling" idleIcon={<RotateCcw size={14} />}>
                Reconcile
              </ButtonProgress>
            </Button>
            <Button disabled={!!busy} size="sm" onClick={refresh}>
              <RefreshCw size={14} />
              Refresh
            </Button>
          </div>
        }
      />
      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}

      <section className="rounded-xl border border-slate-200 bg-white p-4">
        <div className="flex flex-col gap-1">
          <div className="text-sm font-bold text-slate-900">Publish schedule</div>
          <div className="text-xs leading-5 text-slate-500">
            How approved drafts go live. Scheduled spreads them out so a batch of approvals doesn't publish all at once.
          </div>
        </div>
        <div className="mt-3 grid gap-2 sm:grid-cols-3">
          {(
            [
              { value: "scheduled", label: "Scheduled", icon: <CalendarClock size={15} />, detail: "One every few days" },
              { value: "auto", label: "Auto", icon: <Zap size={15} />, detail: "Publish as soon as ready" },
              { value: "manual", label: "Manual", icon: <Send size={15} />, detail: "You publish each one" },
            ] as { value: PublishMode; label: string; icon: React.ReactNode; detail: string }[]
          ).map((opt) => (
            <button
              key={opt.value}
              type="button"
              disabled={busy === "mode"}
              onClick={() => publishMode !== opt.value && saveMode({ publish_mode: opt.value })}
              className={cx(
                "flex flex-col gap-1 rounded-lg border px-3 py-2.5 text-left transition-colors disabled:opacity-60",
                publishMode === opt.value ? "border-[#d93820] bg-orange-50 ring-1 ring-[#d93820]" : "border-slate-200 bg-white hover:bg-slate-50",
              )}
            >
              <span className="inline-flex items-center gap-2 text-sm font-bold text-slate-900">
                {opt.icon}
                {opt.label}
              </span>
              <span className="text-xs text-slate-500">{opt.detail}</span>
            </button>
          ))}
        </div>
        {publishMode === "scheduled" && (
          <div className="mt-3 max-w-xs">
            <Field label="Publish one every (days)">
              <select
                value={publishIntervalDays}
                disabled={busy === "mode"}
                onChange={(event) => saveMode({ publish_interval_days: Number(event.target.value) })}
                className="h-10 rounded-lg border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700"
              >
                {[1, 2, 3, 5, 7].map((d) => (
                  <option key={d} value={d}>
                    {d === 1 ? "1 day (one per day)" : `${d} days`}
                  </option>
                ))}
              </select>
            </Field>
          </div>
        )}
      </section>

      <section>
        <SectionHeader
          title="Scheduled to publish"
          action={<Badge tone={scheduledCanonicals.length ? "blue" : "neutral"}>{scheduledCanonicals.length}</Badge>}
        />
        {scheduledCanonicals.length === 0 ? (
          <EmptyState title="Nothing scheduled" detail="Approved canonical drafts queue here and publish on the cadence above." />
        ) : (
          <div className="grid gap-2">
            {scheduledCanonicals.map((article) => {
              const due = article.scheduled_at ? new Date(article.scheduled_at) : null;
              const isDue = due ? due.getTime() <= Date.now() : false;
              return (
                <div key={article.id} className="rounded-lg border border-slate-200 bg-white px-4 py-3">
                  <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                    <div className="min-w-0">
                      <div className="truncate text-sm font-bold text-slate-900">{articleTitle(article)}</div>
                      <div className="mt-1 inline-flex items-center gap-1.5 text-xs font-semibold text-slate-500">
                        {due ? (
                          <>
                            <CalendarClock size={12} />
                            {isDue ? "Publishing shortly" : `Publishes ${formatDate(article.scheduled_at)}`}
                          </>
                        ) : (
                          "Awaiting manual publish"
                        )}
                      </div>
                    </div>
                    <div className="flex shrink-0 flex-wrap gap-2">
                      <a
                        href={`/projects/${projectId}/articles/${article.id}`}
                        className="inline-flex h-8 items-center rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:bg-slate-50"
                      >
                        Detail
                      </a>
                      <Button disabled={busy === `publish-${article.id}`} size="sm" variant="primary" onClick={() => publishNow(article)}>
                        <ButtonProgress busy={busy === `publish-${article.id}`} busyLabel="Queuing" idleIcon={<Send size={14} />}>
                          Publish now
                        </ButtonProgress>
                      </Button>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </section>

      <section>
        <SectionHeader title="Publishing now" action={<Badge tone={inflight.length ? "amber" : "neutral"}>{inflight.length}</Badge>} />
        {inflight.length === 0 ? (
          <EmptyState title="Nothing in flight" detail="Posts being published and verifying their live URL appear here." />
        ) : (
          <div className="grid gap-2">
            {inflight.map((article) => (
              <div key={article.id} className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3">
                <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                  <div className="min-w-0">
                    <div className="truncate text-sm font-bold text-amber-950">{articleTitle(article)}</div>
                    <div className="mt-1 inline-flex items-center gap-1.5 text-xs font-semibold text-amber-700">
                      <Loader2 size={12} className="animate-spin" />
                      Published to the blog — verifying the live URL
                    </div>
                    {(article.canonical_url || article.publish_path) && (
                      <div className="mt-1 truncate text-xs text-amber-700">{article.canonical_url || article.publish_path}</div>
                    )}
                  </div>
                  {article.canonical_url && (
                    <a
                      href={article.canonical_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex h-8 shrink-0 items-center gap-2 rounded-lg border border-amber-200 bg-white px-3 text-xs font-semibold text-[#d93820] hover:bg-amber-100"
                    >
                      <ExternalLink size={14} />
                      Open URL
                    </a>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

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
                    <Button disabled={busy === `retry-${article.id}`} size="sm" variant="danger" onClick={() => retryPublish(article)}>
                      <ButtonProgress busy={busy === `retry-${article.id}`} busyLabel="Retrying" idleIcon={<RotateCcw size={14} />}>
                        Retry
                      </ButtonProgress>
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
                  <Button disabled={busy === `distributed-${article.id}`} size="sm" variant="primary" onClick={() => markDistributed(article)}>
                    <ButtonProgress busy={busy === `distributed-${article.id}`} busyLabel="Marking distributed" idleIcon={<Send size={14} />}>
                      Mark distributed
                    </ButtonProgress>
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
