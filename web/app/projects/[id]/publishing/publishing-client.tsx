"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  CalendarClock,
  Check,
  Copy,
  ExternalLink,
  Loader2,
  Plug,
  RefreshCw,
  RotateCcw,
  Send,
  Settings2,
  X,
  Zap,
} from "lucide-react";
import { Article, DistributeItem, ProjectConfig, PublisherConnection, defaultProjectConfig } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { useToast } from "../../../components/toast-provider";
import { Badge, Button, ButtonProgress, EmptyState, Field, Notice, SectionHeader, cx, formatDate } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;
type PublishMode = "scheduled" | "auto" | "manual";
type DrawerKind = "schedule" | null;
type LaneTone = "neutral" | "blue" | "amber" | "green" | "red";

const MODE_META: Record<PublishMode, { label: string; icon: React.ReactNode; detail: string }> = {
  scheduled: { label: "Scheduled", icon: <CalendarClock size={15} />, detail: "One every few days" },
  auto: { label: "Auto", icon: <Zap size={15} />, detail: "Publish as soon as ready" },
  manual: { label: "Manual", icon: <Send size={15} />, detail: "You publish each one" },
};

function articleTitle(article: Article) {
  return article.seo_meta?.title || article.seo_meta?.slug || `${article.kind} article`;
}

function publishTimeLabel(article: Article) {
  return article.scheduled_at ? `Publishes ${formatDate(article.scheduled_at)}` : "Publishes on next pass";
}

function connectionTargetLabel(connection: PublisherConnection | null) {
  if (!connection) return "No enabled account";
  return connection.label || connection.config?.repo || connection.kind.replace(/_/g, " ");
}

function publisherConnectionIsActive(connection: PublisherConnection) {
  return connection.enabled && connection.status === "connected";
}

function publisherConnectionDetail(connection: PublisherConnection) {
  const kind = connection.kind.replace(/_/g, " ");
  const target = connection.config?.repo || connection.config?.base_url || connection.config?.content_dir;
  return target ? `${kind} · ${target}` : kind;
}

function publishTargetLabel(article: Article, defaultPublishTarget: string) {
  return article.platform || defaultPublishTarget;
}

function PublishTargetPill({ target }: { target: string }) {
  return (
    <span className="inline-flex min-w-0 max-w-full items-center gap-1 rounded-md border border-slate-200 bg-white px-2 py-0.5 text-xs font-semibold text-slate-600">
      <Plug size={12} className="shrink-0 text-slate-400" />
      <span className="truncate">{target}</span>
    </span>
  );
}

// Slide-out panel from the right for schedule controls.
function Drawer({
  open,
  title,
  subtitle,
  onClose,
  children,
}: {
  open: boolean;
  title: string;
  subtitle?: string;
  onClose: () => void;
  children: React.ReactNode;
}) {
  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => event.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  return (
    <div className={cx("fixed inset-0 z-50", open ? "" : "pointer-events-none")} aria-hidden={!open}>
      <div
        className={cx("absolute inset-0 bg-slate-900/30 transition-opacity", open ? "opacity-100" : "opacity-0")}
        onClick={onClose}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className={cx(
          "absolute right-0 top-0 flex h-full w-full max-w-xl flex-col border-l border-slate-200 bg-white shadow-xl transition-transform duration-200 lg:max-w-[50vw]",
          open ? "translate-x-0" : "translate-x-full",
        )}
      >
        <div className="flex items-start justify-between gap-3 border-b border-slate-100 px-5 py-4">
          <div className="min-w-0">
            <div className="text-sm font-bold text-slate-900">{title}</div>
            {subtitle && <div className="mt-0.5 text-xs leading-5 text-slate-500">{subtitle}</div>}
          </div>
          <button
            type="button"
            onClick={onClose}
            className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-slate-200 bg-white text-slate-500 hover:bg-slate-50"
            aria-label="Close"
          >
            <X size={16} />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">{children}</div>
      </div>
    </div>
  );
}

// One content lane: a titled column block with a count and a vertical stack of cards.
function Lane({ title, count, tone, empty, children }: {
  title: string;
  count: number;
  tone: LaneTone;
  empty: { title: string; detail: string };
  children?: React.ReactNode;
}) {
  return (
    <section className="min-w-0">
      <SectionHeader title={title} action={<Badge tone={count ? tone : "neutral"}>{count}</Badge>} />
      {count === 0 ? <EmptyState title={empty.title} detail={empty.detail} /> : <div className="grid min-w-0 gap-3">{children}</div>}
    </section>
  );
}

// A post card mirroring the content-plan backlog card: badges row, title/meta body,
// and a bottom action row. accent tints the whole card for failure/in-flight signal.
function PostCard({ accent, badges, title, meta, children, actions }: {
  accent?: "amber" | "red";
  badges?: React.ReactNode;
  title: string;
  meta?: React.ReactNode;
  children?: React.ReactNode;
  actions?: React.ReactNode;
}) {
  const accentClass =
    accent === "red" ? "border-red-200 bg-red-50" : accent === "amber" ? "border-amber-200 bg-amber-50" : "border-slate-200 bg-white";
  const titleClass = accent === "red" ? "text-red-950" : accent === "amber" ? "text-amber-950" : "text-slate-900";
  return (
    <div className={cx("flex min-w-0 max-w-full flex-col overflow-hidden rounded-xl border px-4 py-3", accentClass)}>
      {badges && <div className="flex min-w-0 flex-wrap items-center gap-2">{badges}</div>}
      <div className={cx("min-w-0", badges ? "mt-3" : "")}>
        <div className={cx("min-w-0 break-words text-[15px] font-bold leading-5", titleClass)}>{title}</div>
        {meta && <div className="mt-1.5 min-w-0 break-words text-xs font-semibold text-slate-500">{meta}</div>}
        {children}
      </div>
      {actions && <div className="mt-4 flex min-w-0 flex-wrap justify-end gap-2 border-t border-slate-100 pt-3">{actions}</div>}
    </div>
  );
}

export function PublishingClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [published, setPublished] = useState<Article[]>([]);
  const [approved, setApproved] = useState<Article[]>([]);
  const [failed, setFailed] = useState<Article[]>([]);
  const [inflight, setInflight] = useState<Article[]>([]);
  const [ready, setReady] = useState<DistributeItem[]>([]);
  const [config, setConfig] = useState<ProjectConfig | null>(null);
  const [connections, setConnections] = useState<PublisherConnection[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const { notify } = useToast();
  const setMessage = (next: Message) => {
    if (next) notify(next);
  };
  const [drawer, setDrawer] = useState<DrawerKind>(null);
  const [platformsOpen, setPlatformsOpen] = useState(false);
  const platformsMenuRef = useRef<HTMLDivElement | null>(null);

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

  const loadConnections = useCallback(async () => {
    try {
      const next = await api.listPublisherConnections(projectId);
      setConnections(next);
    } catch {
      setConnections([]);
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
    loadConnections();
  }, [refresh, loadConnections]);

  useEffect(() => {
    if (!platformsOpen) return;

    const onPointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (!(target instanceof Node)) return;
      if (platformsMenuRef.current?.contains(target)) return;
      setPlatformsOpen(false);
    };

    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [platformsOpen]);

  // Returning from the GitHub App connect flow lands here with ?github=connected.
  useEffect(() => {
    if (typeof window === "undefined") return;
    const url = new URL(window.location.href);
    if (url.searchParams.get("github") === "connected") {
      setMessage({ title: "GitHub connected", detail: "Enable the connection in settings before publishing.", tone: "green" });
      loadConnections();
      url.searchParams.delete("github");
      window.history.replaceState({}, "", url.pathname + url.search);
    }
  }, [loadConnections]);

  // Poll while anything is mid-publish so the page reflects each post going live
  // on its staggered slot without a manual reload.
  useEffect(() => {
    if (inflight.length === 0) return;
    const interval = window.setInterval(refresh, 15_000);
    return () => window.clearInterval(interval);
  }, [inflight.length, refresh]);

  const approvedCanonicals = useMemo(
    () =>
      approved
        .filter((article) => article.kind === "canonical")
        .sort((a, b) => (a.scheduled_at ?? "").localeCompare(b.scheduled_at ?? "")),
    [approved],
  );
  // Ready = due now (no date, or slot already reached) — goes out on the next
  // publish pass or via "Publish now". Scheduled = a future staggered slot.
  const readyCanonicals = useMemo(
    () => approvedCanonicals.filter((a) => !a.scheduled_at || new Date(a.scheduled_at).getTime() <= Date.now()),
    [approvedCanonicals],
  );
  const scheduledCanonicals = useMemo(
    () => approvedCanonicals.filter((a) => a.scheduled_at && new Date(a.scheduled_at).getTime() > Date.now()),
    [approvedCanonicals],
  );
  const hasCanonicalPublishingWork =
    readyCanonicals.length + scheduledCanonicals.length + published.length + inflight.length + failed.length > 0;
  const publishMode: PublishMode = (config?.publish_mode as PublishMode) ?? "manual";
  const publishIntervalDays = config?.publish_interval_days ?? 2;
  const eligiblePublisherConnections = useMemo(() => connections.filter((c) => c.enabled && c.status === "connected"), [connections]);
  const activePublisherConnections = eligiblePublisherConnections;
  const activePublisherConnection = useMemo(
    () =>
      eligiblePublisherConnections.find((connection) => connection.is_default) ??
      eligiblePublisherConnections[0] ??
      null,
    [eligiblePublisherConnections],
  );
  const defaultPublishTarget = useMemo(
    () => connectionTargetLabel(activePublisherConnection),
    [activePublisherConnection],
  );

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
    if (!activePublisherConnection) {
      setMessage({ title: "No enabled publisher connection", detail: "Manage connections in settings before publishing.", tone: "amber" });
      return;
    }
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
    if (!activePublisherConnection) {
      setMessage({ title: "No enabled publisher connection", detail: "Manage connections in settings before retrying.", tone: "amber" });
      return;
    }
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
      setMessage({ title: "Status check failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Publish"
        eyebrow="Canonical and syndication lanes"
        level="page"
        action={
          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={() => setDrawer("schedule")}
              className="inline-flex h-9 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:bg-slate-50"
            >
              <Settings2 size={14} className="text-slate-400" />
              Mode
              <span className="inline-flex items-center gap-1 text-slate-900">
                {MODE_META[publishMode].icon}
                {MODE_META[publishMode].label}
              </span>
            </button>
            <div ref={platformsMenuRef} className="relative">
              <button
                type="button"
                aria-expanded={platformsOpen}
                onClick={() => {
                  setPlatformsOpen((open) => !open);
                  loadConnections();
                }}
                className="inline-flex h-9 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:bg-slate-50"
              >
                <Plug size={14} className="text-slate-400" />
                Platforms
                <Badge tone={activePublisherConnections.length ? "green" : "red"}>
                  {activePublisherConnections.length ? `${activePublisherConnections.length} active` : "none active"}
                </Badge>
              </button>
              {platformsOpen && (
                <div className="absolute right-0 top-11 z-30 w-[min(22rem,calc(100vw-2rem))] overflow-hidden rounded-lg border border-slate-200 bg-white shadow-lg">
                  <div className="max-h-72 overflow-y-auto p-2">
                    {connections.length === 0 ? (
                      <div className="rounded-md border border-dashed border-slate-200 px-3 py-3 text-sm font-medium text-slate-500">
                        No publisher connections yet.
                      </div>
                    ) : (
                      <div className="grid gap-1">
                        {connections.map((connection) => (
                          <div key={connection.id} className="flex min-w-0 items-start justify-between gap-3 rounded-md px-3 py-2 hover:bg-slate-50">
                            <div className="min-w-0">
                              <div className="truncate text-sm font-bold text-slate-900">{connectionTargetLabel(connection)}</div>
                              <div className="mt-0.5 truncate text-xs font-medium text-slate-500">{publisherConnectionDetail(connection)}</div>
                            </div>
                            <Badge tone={publisherConnectionIsActive(connection) ? "green" : "red"}>
                              {publisherConnectionIsActive(connection) ? "active" : "inactive"}
                            </Badge>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                  <div className="border-t border-slate-100 p-2">
                    <a
                      href={`/projects/${projectId}/settings#publisher`}
                      className="inline-flex h-9 w-full items-center justify-center rounded-lg border border-slate-200 bg-slate-50 px-3 text-xs font-semibold text-slate-700 hover:bg-slate-100"
                    >
                      manage connections
                    </a>
                  </div>
                </div>
              )}
            </div>
            <Button disabled={!!busy} size="sm" onClick={reconcile}>
              <ButtonProgress busy={busy === "reconcile"} busyLabel="Checking status" idleIcon={<RotateCcw size={14} />}>
                Check status
              </ButtonProgress>
            </Button>
            <Button disabled={!!busy} size="sm" onClick={refresh}>
              <RefreshCw size={14} />
              Refresh
            </Button>
          </div>
        }
      />

      {eligiblePublisherConnections.length === 0 && (
        <Notice title="No enabled publisher connection" detail="Enable a connected publisher account in settings before publishing or retrying." tone="amber" />
      )}

      {hasCanonicalPublishingWork ? (
        <div className="grid min-w-0 gap-5 lg:grid-cols-2 lg:items-start">
          {/* Left column — Ready then Scheduled. */}
          <div className="min-w-0 space-y-6">
            {readyCanonicals.length > 0 && (
              <Lane
                title="Ready to publish"
                count={readyCanonicals.length}
                tone="green"
                empty={{ title: "Nothing ready", detail: "Approved drafts that are due appear here and publish on the next pass." }}
              >
                {readyCanonicals.map((article) => (
                  <PostCard
                    key={article.id}
                    badges={
                      <>
                        <Badge tone="green">ready</Badge>
                        <PublishTargetPill target={publishTargetLabel(article, defaultPublishTarget)} />
                      </>
                    }
                    title={articleTitle(article)}
                    meta={
                      <span className="inline-flex items-center gap-1.5">
                        <CalendarClock size={12} />
                        {publishTimeLabel(article)}
                      </span>
                    }
                    actions={
                      <>
                        <a
                          href={`/projects/${projectId}/articles/${article.id}`}
                          className="inline-flex h-8 items-center rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:bg-slate-50"
                        >
                          Detail
                        </a>
                        <Button disabled={!activePublisherConnection || busy === `publish-${article.id}`} size="sm" variant="primary" onClick={() => publishNow(article)}>
                          <ButtonProgress busy={busy === `publish-${article.id}`} busyLabel="Queuing" idleIcon={<Send size={14} />}>
                            Publish now
                          </ButtonProgress>
                        </Button>
                      </>
                    }
                  />
                ))}
              </Lane>
            )}

            {scheduledCanonicals.length > 0 && (
              <Lane
                title="Scheduled to publish"
                count={scheduledCanonicals.length}
                tone="blue"
                empty={{ title: "Nothing scheduled", detail: "Future-dated drafts queue here and publish on the cadence set in Mode." }}
              >
                {scheduledCanonicals.map((article) => (
                  <PostCard
                    key={article.id}
                    badges={
                      <>
                        <Badge tone="blue">scheduled</Badge>
                        <PublishTargetPill target={publishTargetLabel(article, defaultPublishTarget)} />
                      </>
                    }
                    title={articleTitle(article)}
                    meta={
                      <span className="inline-flex items-center gap-1.5">
                        <CalendarClock size={12} />
                        {publishTimeLabel(article)}
                      </span>
                    }
                    actions={
                      <>
                        <a
                          href={`/projects/${projectId}/articles/${article.id}`}
                          className="inline-flex h-8 items-center rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:bg-slate-50"
                        >
                          Detail
                        </a>
                        <Button disabled={!activePublisherConnection || busy === `publish-${article.id}`} size="sm" variant="primary" onClick={() => publishNow(article)}>
                          <ButtonProgress busy={busy === `publish-${article.id}`} busyLabel="Queuing" idleIcon={<Send size={14} />}>
                            Publish now
                          </ButtonProgress>
                        </Button>
                      </>
                    }
                  />
                ))}
              </Lane>
            )}
          </div>

          {/* Right column — outcomes. */}
          <div className="min-w-0 space-y-6">
            {published.length + inflight.length > 0 && (
              <Lane
                title="Published"
                count={published.length + inflight.length}
                tone="green"
                empty={{ title: "No canonical articles published", detail: "Approved canonical articles publish automatically when due." }}
              >
                {inflight.map((article) => (
                  <PostCard
                    key={article.id}
                    accent="amber"
                    badges={
                      <span className="inline-flex items-center gap-1.5 text-xs font-semibold text-amber-700">
                        <Loader2 size={12} className="animate-spin" />
                        publishing · verifying live URL
                      </span>
                    }
                    title={articleTitle(article)}
                    meta={(article.canonical_url || article.publish_path) && <span className="break-all text-amber-700">{article.canonical_url || article.publish_path}</span>}
                    actions={
                      article.canonical_url && (
                        <a
                          href={article.canonical_url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-flex h-8 items-center gap-2 rounded-lg border border-amber-200 bg-white px-3 text-xs font-semibold text-[#d93820] hover:bg-amber-100"
                        >
                          <ExternalLink size={14} />
                          Open URL
                        </a>
                      )
                    }
                  />
                ))}
                {published.map((article) => (
                  <PostCard
                    key={article.id}
                    badges={<Badge tone="green">published</Badge>}
                    title={articleTitle(article)}
                    meta={`Published ${formatDate(article.published_at)}`}
                    actions={
                      article.canonical_url ? (
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
                      )
                    }
                  />
                ))}
              </Lane>
            )}

            {failed.length > 0 && (
              <Lane
                title="Publishing failed"
                count={failed.length}
                tone="red"
                empty={{ title: "No publish failures", detail: "Failed canonical publish attempts will appear here with retry controls." }}
              >
                {failed.map((article) => (
                  <PostCard
                    key={article.id}
                    accent="red"
                    badges={<Badge tone="red">failed</Badge>}
                    title={articleTitle(article)}
                    meta={`attempt ${article.publish_attempts} · next retry ${formatDate(article.next_publish_retry_at)}`}
                    actions={
                      <>
                        <a
                          href={`/projects/${projectId}/articles/${article.id}`}
                          className="inline-flex h-8 items-center rounded-lg border border-red-200 bg-white px-3 text-xs font-semibold text-red-700 hover:bg-red-50"
                        >
                          Detail
                        </a>
                        <Button disabled={!activePublisherConnection || busy === `retry-${article.id}`} size="sm" variant="danger" onClick={() => retryPublish(article)}>
                          <ButtonProgress busy={busy === `retry-${article.id}`} busyLabel="Retrying" idleIcon={<RotateCcw size={14} />}>
                            Retry
                          </ButtonProgress>
                        </Button>
                      </>
                    }
                  >
                    <div className="mt-2 line-clamp-3 break-words text-sm leading-5 text-red-800">
                      {article.last_publish_error || "No publish error captured."}
                    </div>
                    {article.publish_path && <div className="mt-1 break-all text-xs text-red-700">{article.publish_path}</div>}
                  </PostCard>
                ))}
              </Lane>
            )}
          </div>
        </div>
      ) : (
        <EmptyState
          title="No publishing work is waiting"
          detail="Approved drafts will appear here when they are ready, scheduled, publishing, live, or need a retry."
        />
      )}

      {(ready.length > 0 || waiting.length > 0) && (
        <section className="space-y-3">
          <SectionHeader title="Syndication" eyebrow="Variants for third-party platforms (unlock after the canonical is live)" />
          <div className="grid min-w-0 gap-5 lg:grid-cols-2 lg:items-start">
            <Lane
              title="Ready to distribute"
              count={ready.length}
              tone="green"
              empty={{ title: "No variants ready", detail: "Approved variants unlock after the canonical publishes and its URL is confirmed." }}
            >
              {ready.map(({ article, compose_url, supports_canonical }) => (
                <PostCard
                  key={article.id}
                  badges={
                    <>
                      <Badge tone="amber">{article.platform ?? "platform"}</Badge>
                      <span className="text-xs font-semibold text-slate-400">
                        {supports_canonical ? "canonical tag supported" : "source link in body"}
                      </span>
                    </>
                  }
                  title={articleTitle(article)}
                  actions={
                    <>
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
                    </>
                  }
                >
                  <p className="mt-2 line-clamp-3 content-font text-sm leading-5 text-slate-700">{article.content_md}</p>
                </PostCard>
              ))}
            </Lane>

            <Lane
              title="Waiting on canonical"
              count={waiting.length}
              tone="neutral"
              empty={{ title: "No variants waiting", detail: "Approved variants waiting for their canonical to publish appear here." }}
            >
              {waiting.map((article) => (
                <PostCard
                  key={article.id}
                  badges={<Badge tone="neutral">{article.platform ?? "platform"}</Badge>}
                  title={articleTitle(article)}
                  meta="Unlocks automatically once its canonical article is published and its live URL is confirmed."
                />
              ))}
            </Lane>
          </div>
        </section>
      )}

      <Notice
        title="Manual distribution only"
        detail="Mark distributed records user completion. It does not publish to the third-party platform automatically."
      />

      <Drawer
        open={drawer === "schedule"}
        title="Publish schedule"
        subtitle="How approved drafts go live. Scheduled spreads them out so a batch of approvals doesn't publish all at once."
        onClose={() => setDrawer(null)}
      >
        <div className="grid gap-2">
          {(Object.keys(MODE_META) as PublishMode[]).map((value) => {
            const opt = MODE_META[value];
            return (
              <button
                key={value}
                type="button"
                disabled={busy === "mode"}
                onClick={() => publishMode !== value && saveMode({ publish_mode: value })}
                className={cx(
                  "flex flex-col gap-1 rounded-lg border px-3 py-2.5 text-left transition-colors disabled:opacity-60",
                  publishMode === value ? "border-[#d93820] bg-orange-50 ring-1 ring-[#d93820]" : "border-slate-200 bg-white hover:bg-slate-50",
                )}
              >
                <span className="inline-flex items-center gap-2 text-sm font-bold text-slate-900">
                  {opt.icon}
                  {opt.label}
                  {publishMode === value && <Check size={14} className="ml-auto text-[#d93820]" />}
                </span>
                <span className="text-xs text-slate-500">{opt.detail}</span>
              </button>
            );
          })}
        </div>
        {publishMode === "scheduled" && (
          <div className="mt-4">
            <Field label="Publish one every (days)">
              <select
                value={publishIntervalDays}
                disabled={busy === "mode"}
                onChange={(event) => saveMode({ publish_interval_days: Number(event.target.value) })}
                className="h-10 w-full rounded-lg border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700"
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
      </Drawer>

    </div>
  );
}
