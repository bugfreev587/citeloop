"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Copy, ExternalLink, GitBranch, RefreshCw, RotateCcw, Send } from "lucide-react";
import { Article, DistributeItem, PublishingHealth, PublishingReconcileResult } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, EmptyState, Notice, SectionHeader, formatDate } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;

function articleTitle(article: Article) {
  return article.seo_meta?.title || article.seo_meta?.slug || `${article.kind} article`;
}

function healthTone(status?: string): "neutral" | "red" | "amber" | "green" {
  if (status === "ready" || status === "connected" || status === "configured") return "green";
  if (status === "error") return "red";
  if (!status || status === "missing" || status === "blocked" || status === "in_progress") return "amber";
  return "neutral";
}

function reasonLabel(reason: string) {
  const labels: Record<string, string> = {
    database_unavailable: "Database is unavailable",
    publisher_missing: "No publisher connection has been saved",
    publisher_config_invalid: "Publisher repository or base URL is incomplete",
    publisher_connection_error: "The latest publisher test failed",
    publisher_credential_missing: "GitHub token is missing",
    publisher_credential_unavailable: "Saved GitHub credential cannot be resolved",
    publisher_blocked: "Publisher setup is blocking publishing",
    no_reconcile_candidates: "No previous publish attempts need reconciliation",
    no_publishable_canonical_due: "No approved canonical articles are due",
    drafts_waiting_review: "Drafts are waiting in Review",
    variants_waiting_canonical: "Variants are waiting for canonical publish",
  };
  return labels[reason] ?? reason.replaceAll("_", " ");
}

function resultTone(status?: string): "neutral" | "red" | "amber" | "green" {
  if (status === "reconciled") return "green";
  if (status === "blocked") return "amber";
  if (status === "error") return "red";
  return "neutral";
}

export function PublishingClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [health, setHealth] = useState<PublishingHealth | null>(null);
  const [published, setPublished] = useState<Article[]>([]);
  const [approved, setApproved] = useState<Article[]>([]);
  const [failed, setFailed] = useState<Article[]>([]);
  const [ready, setReady] = useState<DistributeItem[]>([]);
  const [reconcileResult, setReconcileResult] = useState<PublishingReconcileResult | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);

  const refresh = useCallback(async () => {
    try {
      const [nextHealth, pub, app, fail, dist] = await Promise.all([
        api.getPublishingHealth(projectId),
        api.listArticles(projectId, "published"),
        api.listArticles(projectId, "approved"),
        api.listArticles(projectId, "publish_failed"),
        api.listDistribute(projectId),
      ]);
      setHealth(nextHealth);
      setPublished(pub);
      setApproved(app);
      setFailed(fail);
      setReady(dist);
    } catch (e: any) {
      setMessage({ title: "Publishing data unavailable", detail: e.message, tone: "amber" });
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

  const publishBlockedReason = useMemo(() => {
    if (!health) return "Publisher health is still loading.";
    if (health.ready) return "";
    if (health.next_action) return health.next_action;
    if (health.reasons.length) return health.reasons.map(reasonLabel).join(", ");
    return "Publisher is not ready.";
  }, [health]);

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
      const result = await api.reconcilePublishing(projectId);
      setReconcileResult(result);
      setHealth(result.health);
      await refresh();
      setMessage({
        title: result.status === "blocked" ? "Publishing check completed" : "Publishing reconciled",
        detail:
          result.status === "blocked"
            ? "Publishing is blocked by setup or content prerequisites. See the reconcile summary below."
            : `Checked ${result.checked_articles} articles; repaired ${result.repaired_state_count}.`,
        tone: result.status === "blocked" ? "amber" : "green",
      });
    } catch (e: any) {
      setMessage({ title: "Reconcile failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function publishTick() {
    if (!health?.ready) {
      setMessage({ title: "Publishing setup required", detail: publishBlockedReason, tone: "amber" });
      return;
    }
    setBusy("publish-tick");
    setMessage(null);
    try {
      await api.tickPublish(projectId);
      await refresh();
      setMessage({ title: "Publish tick complete", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Publish tick failed", detail: e.message, tone: "red" });
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
            <Button disabled={!!busy || !health?.ready} size="sm" onClick={publishTick} title={publishBlockedReason || "Run canonical publisher tick"}>
              <Send size={14} />
              Publish tick
            </Button>
            <Button disabled={!!busy} size="sm" onClick={reconcile} title="Check publish lanes and reconcile previous publish attempts">
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
      <section className="rounded-lg border border-slate-200 bg-white px-4 py-4">
        <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div>
            <div className="flex items-center gap-2 text-sm font-bold text-slate-900">
              <GitBranch size={16} />
              Publisher setup
            </div>
            <p className="mt-1 max-w-2xl text-sm leading-5 text-slate-600">
              CiteLoop needs a connected publisher and saved credential before canonical articles can publish automatically.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Badge tone={healthTone(health?.status)}>{health?.status ?? "loading"}</Badge>
            <Badge tone={healthTone(health?.connection_status)}>connection {health?.connection_status ?? "loading"}</Badge>
            <Badge tone={healthTone(health?.credential_status)}>credential {health?.credential_status ?? "loading"}</Badge>
          </div>
        </div>
        {!health?.ready && (
          <Notice
            title="Publishing blocked"
            detail={publishBlockedReason}
            tone={health?.status === "error" ? "red" : "amber"}
          />
        )}
        {health?.reasons?.length ? (
          <div className="mt-3 flex flex-wrap gap-2">
            {health.reasons.map((reason) => (
              <Badge key={reason} tone={healthTone(health.status)}>
                {reasonLabel(reason)}
              </Badge>
            ))}
          </div>
        ) : null}
        <div className="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          {Object.entries(health?.capabilities ?? {}).map(([key, enabled]) => (
            <div key={key} className="rounded-lg border border-slate-200 px-3 py-2 text-xs">
              <div className="font-semibold uppercase tracking-[0.14em] text-slate-400">{key.replaceAll("_", " ")}</div>
              <div className={enabled ? "mt-1 font-bold text-green-700" : "mt-1 font-bold text-slate-500"}>{enabled ? "Available" : "Unavailable"}</div>
            </div>
          ))}
        </div>
        <a
          href={`/projects/${projectId}/settings`}
          className="mt-3 inline-flex h-9 items-center rounded-lg border border-slate-200 bg-white px-3 text-sm font-semibold text-[#d93820] hover:bg-slate-50"
        >
          Open publisher settings
        </a>
      </section>

      {reconcileResult && (
        <section className="rounded-lg border border-slate-200 bg-white px-4 py-4">
          <SectionHeader
            title="Latest reconcile check"
            eyebrow="Publish lanes"
            action={<Badge tone={resultTone(reconcileResult.status)}>{reconcileResult.status}</Badge>}
          />
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
            <div className="rounded-lg border border-slate-200 px-3 py-2 text-sm">
              <div className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">Checked</div>
              <div className="mt-1 text-xl font-bold text-slate-900">{reconcileResult.checked_articles}</div>
            </div>
            <div className="rounded-lg border border-slate-200 px-3 py-2 text-sm">
              <div className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">Publishable now</div>
              <div className="mt-1 text-xl font-bold text-slate-900">{reconcileResult.publishable_count}</div>
            </div>
            <div className="rounded-lg border border-slate-200 px-3 py-2 text-sm">
              <div className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">Repaired</div>
              <div className="mt-1 text-xl font-bold text-slate-900">{reconcileResult.repaired_state_count}</div>
            </div>
            <div className="rounded-lg border border-slate-200 px-3 py-2 text-sm">
              <div className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">Blockers</div>
              <div className="mt-1 text-xl font-bold text-slate-900">{reconcileResult.blockers.length}</div>
            </div>
          </div>

          {reconcileResult.skipped_reasons.length > 0 ? (
            <div className="mt-4 grid gap-2">
              {reconcileResult.skipped_reasons.map((reason) => (
                <div key={reason.reason} className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900">
                  <div className="font-semibold">
                    {reasonLabel(reason.reason)}
                    {reason.count > 0 ? ` (${reason.count})` : ""}
                  </div>
                  <div className="mt-1 opacity-80">{reason.detail}</div>
                </div>
              ))}
            </div>
          ) : (
            <Notice title="No skipped lanes" detail="All reconcile candidates were processed." tone="green" />
          )}

          <div className="mt-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
            {[
              ["Due canonicals", reconcileResult.counts.approved_canonical_due],
              ["Scheduled canonicals", reconcileResult.counts.approved_canonical_scheduled],
              ["Pending URL verification", reconcileResult.counts.pending_url_verification],
              ["Publish failures", reconcileResult.counts.publish_failures],
              ["Retryable failures", reconcileResult.counts.retryable_failures],
              ["Variants waiting", reconcileResult.counts.approved_variants_waiting_canonical],
              ["Ready variants", reconcileResult.counts.ready_to_distribute],
              ["Drafts in Review", reconcileResult.counts.pending_review],
            ].map(([label, count]) => (
              <div key={label} className="rounded-lg bg-slate-50 px-3 py-2 text-sm">
                <div className="font-semibold text-slate-700">{label}</div>
                <div className="mt-1 text-lg font-bold text-slate-950">{count}</div>
              </div>
            ))}
          </div>

          <div className="mt-3 flex flex-wrap gap-2 text-sm font-semibold">
            <a className="text-[#d93820]" href={`/projects/${projectId}/review`}>
              Open Review
            </a>
            <a className="text-[#d93820]" href={`/projects/${projectId}/topics`}>
              Open Topics
            </a>
            <a className="text-[#d93820]" href={`/projects/${projectId}/settings`}>
              Open publisher settings
            </a>
          </div>
        </section>
      )}

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
                  {article.platform ?? "platform"} is approved but waiting for canonical publish and URL backfill.
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
