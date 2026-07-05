"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  CalendarClock,
  Check,
  Copy,
  Eye,
  ExternalLink,
  Loader2,
  Plug,
  RefreshCw,
  RotateCcw,
  Send,
  Settings2,
  Zap,
} from "lucide-react";
import { Article, DistributeItem, ProjectConfig, PublisherConnection, defaultProjectConfig } from "../../../lib/api";
import {
  ManualSyndicationPlatform,
  OperationalGroup,
  PublishDestination,
  buildManualSyndicationSummary,
  buildPublishDestinations,
  buildPublishHeaderCta,
  buildPublishingOperationalGroups,
  buildReadyNow,
} from "../../../lib/publish-destinations-logic";
import { useApi } from "../../../lib/use-api";
import { useToast } from "../../../components/toast-provider";
import { RightDrawer } from "../../../components/right-drawer";
import { Badge, Button, ButtonProgress, EmptyState, Field, Notice, SectionHeader, cx, formatDate } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;
type PublishMode = "scheduled" | "auto" | "manual";
type DrawerKind = "schedule" | "view_all" | "github" | "manual" | "more" | "cms" | null;
type ManualPlatformID = ManualSyndicationPlatform["id"];

const MODE_META: Record<PublishMode, { label: string; icon: React.ReactNode; detail: string }> = {
  scheduled: { label: "Scheduled", icon: <CalendarClock size={15} />, detail: "One every few days" },
  auto: { label: "Auto", icon: <Zap size={15} />, detail: "Publish as soon as ready" },
  manual: { label: "Manual", icon: <Send size={15} />, detail: "You publish each one" },
};

function articleTitle(article: Article) {
  return article.seo_meta?.title || article.seo_meta?.slug || `${article.kind} article`;
}

function publishTimeLabel(article: Article) {
  return article.scheduled_at ? `Publishes ${formatDate(article.scheduled_at)}` : "Manual: when you publish";
}

function connectionTargetLabel(connection: PublisherConnection | null) {
  if (!connection) return "No enabled account";
  return connection.label || connection.config?.repo || connection.kind.replace(/_/g, " ");
}

function publisherConnectionDetail(connection: PublisherConnection) {
  const kind = connection.kind.replace(/_/g, " ");
  const target = connection.config?.repo || connection.config?.base_url || connection.config?.content_dir;
  return target ? `${kind} · ${target}` : kind;
}

function platformTone(destination: PublishDestination): "neutral" | "red" | "amber" | "green" | "blue" {
  if (destination.kind === "roadmap") return "neutral";
  if (destination.state === "needs_attention") return "red";
  if (destination.state === "disabled") return "amber";
  if (destination.state === "auto_publish" || destination.readyCount > 0) return "green";
  if (destination.kind === "manual") return "blue";
  return "neutral";
}

function operationalArticle(item: Article | DistributeItem) {
  return "article" in item ? item.article : item;
}

function isDistributeItem(item: Article | DistributeItem): item is DistributeItem {
  return "article" in item;
}

// Slide-out panel used for schedule controls, destination details, and View all.
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
  return (
    <RightDrawer open={open} title={title} subtitle={subtitle} onClose={onClose} maxWidthClassName="max-w-xl lg:max-w-[50vw]">
      {children}
    </RightDrawer>
  );
}

function DestinationSkeleton() {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {[0, 1, 2, 3].map((item) => (
        <div key={item} className="h-28 animate-pulse rounded-lg border border-slate-200 bg-slate-50" />
      ))}
    </div>
  );
}

function DestinationTile({
  destination,
  featured,
  onOpen,
}: {
  destination: PublishDestination;
  featured?: boolean;
  onOpen: () => void;
}) {
  return (
    <button
      type="button"
      data-publish-destination-tile={destination.id}
      onClick={onOpen}
      className={cx(
        "group flex min-w-0 flex-col items-stretch rounded-lg border bg-white p-4 text-left transition hover:border-slate-300 hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-[#d93820]/30",
        destination.kind === "roadmap" ? "border-dashed border-slate-300" : "border-slate-200",
        featured ? "sm:col-span-2 xl:col-span-1" : "",
      )}
    >
      <span className="flex min-w-0 items-start justify-between gap-3">
        <span className="flex min-w-0 items-center gap-2">
          <span className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-600">
            <Plug size={17} />
          </span>
          <span className="min-w-0">
            <span className="block truncate text-sm font-bold text-slate-900">{destination.label}</span>
            {destination.targetDetail && <span className="mt-0.5 block truncate text-xs font-medium text-slate-500">{destination.targetDetail}</span>}
          </span>
        </span>
        <Badge tone={platformTone(destination)}>{destination.stateLabel}</Badge>
      </span>
      <span className="mt-4 flex min-w-0 flex-wrap items-center gap-2">
        {destination.readyCount > 0 && <Badge tone="green">{destination.readyCount} ready</Badge>}
        {destination.waitingCount > 0 && <Badge tone="neutral">{destination.waitingCount} waiting</Badge>}
        {destination.readyCount === 0 && destination.waitingCount === 0 && (
          <span className="truncate text-xs font-semibold text-slate-500">{destination.actionLabel}</span>
        )}
      </span>
    </button>
  );
}

function ReadyNowStrip({
  className,
  projectId,
  readyNow,
  busy,
  activePublisherConnection,
  onPublish,
  onRetry,
  onDestination,
  onTiming,
}: {
  className?: string;
  projectId: string;
  readyNow: ReturnType<typeof buildReadyNow>;
  busy: string | null;
  activePublisherConnection: PublisherConnection | null;
  onPublish: (article: Article) => void;
  onRetry: (article: Article) => void;
  onDestination: () => void;
  onTiming: () => void;
}) {
  const visibleItems = readyNow.items.slice(0, 4);

  return (
    <section data-publish-ready-to-post className={cx("min-w-0 space-y-3", className)}>
      <SectionHeader title="Ready to post" action={<Badge tone={visibleItems.length ? "green" : "neutral"}>{readyNow.items.length}</Badge>} />
      {visibleItems.length === 0 ? (
        <EmptyState title={readyNow.emptyState.title} detail={readyNow.emptyState.detail} />
      ) : (
        <div className="grid min-w-0 gap-3">
          {visibleItems.map((item) => (
            <div key={item.id} className="min-w-0 rounded-lg border border-slate-200 bg-white p-4">
              <div className="flex min-w-0 items-start justify-between gap-2">
                <div className="min-w-0">
                  <div className="line-clamp-2 break-words text-base font-bold leading-6 text-slate-950">{item.title}</div>
                  <div className="mt-1 text-xs font-semibold text-slate-500">Canonical content</div>
                </div>
                <Badge tone={item.action === "retry" ? "red" : "green"}>{item.actionLabel}</Badge>
              </div>
              <div className="mt-3 grid min-w-0 gap-2 sm:grid-cols-2">
                <div className="min-w-0 rounded-lg bg-slate-50 px-3 py-2">
                  <div className="text-[11px] font-bold uppercase tracking-normal text-slate-400">Where</div>
                  <div className="mt-0.5 truncate text-xs font-semibold text-slate-700">{item.destinationLabel}</div>
                </div>
                <div className="min-w-0 rounded-lg bg-slate-50 px-3 py-2">
                  <div className="text-[11px] font-bold uppercase tracking-normal text-slate-400">When</div>
                  <div className="mt-0.5 truncate text-xs font-semibold text-slate-700">{publishTimeLabel(item.article)}</div>
                </div>
              </div>
              {item.failureReason && <div className="mt-2 line-clamp-2 break-words text-xs leading-5 text-red-700">{item.failureReason}</div>}
              <div className="mt-3 flex flex-wrap justify-end gap-2 border-t border-slate-100 pt-3">
                <a
                  href={`/projects/${projectId}/articles/${item.articleId}`}
                  className="inline-flex h-8 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:bg-slate-50"
                >
                  <Eye size={14} />
                  {item.secondaryActionLabel}
                </a>
                <Button size="sm" onClick={onDestination}>
                  <Plug size={14} />
                  {item.destinationActionLabel}
                </Button>
                <Button size="sm" title={publishTimeLabel(item.article)} onClick={onTiming}>
                  <CalendarClock size={14} />
                  {item.timingActionLabel}
                </Button>
                <Button
                  disabled={!activePublisherConnection || busy === `${item.action}-${item.articleId}`}
                  size="sm"
                  variant={item.action === "retry" ? "danger" : "primary"}
                  title={item.disabledReason}
                  onClick={() => (item.action === "retry" ? onRetry(item.article) : onPublish(item.article))}
                >
                  <ButtonProgress
                    busy={busy === `${item.action}-${item.articleId}`}
                    busyLabel={item.action === "retry" ? "Retrying" : "Queuing"}
                    idleIcon={item.action === "retry" ? <RotateCcw size={14} /> : <Send size={14} />}
                  >
                    {item.actionLabel}
                  </ButtonProgress>
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function ManualPlatformRows({
  projectId,
  platform,
  busy,
  onCopy,
  onMarkDistributed,
}: {
  projectId: string;
  platform: ManualSyndicationPlatform;
  busy: string | null;
  onCopy: (article: Article) => void;
  onMarkDistributed: (article: Article) => void;
}) {
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <Badge tone={platform.readyCount ? "green" : "neutral"}>{platform.readyCount} ready</Badge>
        <Badge tone={platform.waitingCount ? "neutral" : "neutral"}>{platform.waitingCount} waiting</Badge>
      </div>

      {platform.readyRows.length > 0 && (
        <div className="grid gap-3">
          {platform.readyRows.map((row) => (
            <div key={row.articleId} className="rounded-lg border border-slate-200 bg-white p-3">
              <div className="break-words text-sm font-bold text-slate-900">{row.title}</div>
              <div className="mt-3 flex flex-wrap justify-end gap-2 border-t border-slate-100 pt-3">
                <Button size="sm" onClick={() => onCopy(row.article)}>
                  <Copy size={14} />
                  Copy
                </Button>
                {row.composeUrl && (
                  <a
                    href={row.composeUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex h-8 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50"
                  >
                    <ExternalLink size={14} />
                    Open
                  </a>
                )}
                <Button disabled={busy === `distributed-${row.articleId}`} size="sm" variant="primary" onClick={() => onMarkDistributed(row.article)}>
                  <ButtonProgress busy={busy === `distributed-${row.articleId}`} busyLabel="Marking distributed" idleIcon={<Send size={14} />}>
                    Mark distributed
                  </ButtonProgress>
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      {platform.waitingRows.length > 0 && (
        <div className="grid gap-3">
          {platform.waitingRows.map((row) => (
            <div key={row.articleId} className="rounded-lg border border-dashed border-slate-200 bg-slate-50 p-3">
              <div className="break-words text-sm font-bold text-slate-900">{row.title}</div>
              <div className="mt-1 text-xs font-semibold text-slate-500">
                Unlocks after the canonical URL is published and verified.
              </div>
            </div>
          ))}
        </div>
      )}

      {platform.readyRows.length === 0 && platform.waitingRows.length === 0 && (
        <EmptyState title="No drafts for this platform" detail="Unlocked manual drafts appear here after the canonical article is live." />
      )}
      <a
        href={`/projects/${projectId}/review`}
        className="inline-flex h-8 items-center rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:bg-slate-50"
      >
        View related drafts
      </a>
    </div>
  );
}

function OperationalRows({
  group,
  projectId,
  busy,
  onCopy,
  onPublish,
  onRetry,
  onMarkDistributed,
}: {
  group: OperationalGroup;
  projectId: string;
  busy: string | null;
  onCopy: (article: Article) => void;
  onPublish: (article: Article) => void;
  onRetry: (article: Article) => void;
  onMarkDistributed: (article: Article) => void;
}) {
  if (group.count === 0) {
    return <EmptyState title={`No ${group.label.toLowerCase()} items`} detail="Items appear here as publishing state changes." />;
  }

  return (
    <div className="grid gap-3">
      {group.items.map((item) => {
        const article = operationalArticle(item);
        const distributeItem = isDistributeItem(item) ? item : null;
        return (
          <div key={`${group.key}-${article.id}`} className="rounded-lg border border-slate-200 bg-white p-3">
            <div className="flex min-w-0 items-start justify-between gap-2">
              <div className="min-w-0">
                <div className="break-words text-sm font-bold leading-5 text-slate-900">{articleTitle(article)}</div>
                <div className="mt-1 break-words text-xs font-semibold text-slate-500">
                  {group.key === "scheduled"
                    ? publishTimeLabel(article)
                    : group.key === "published"
                      ? article.published_at
                        ? `Published ${formatDate(article.published_at)}`
                        : "Publishing / verifying"
                      : article.platform || "GitHub/Next.js"}
                </div>
              </div>
              <Badge tone={group.key === "failed" ? "red" : group.key === "scheduled" ? "blue" : "neutral"}>{group.label}</Badge>
            </div>
            {group.key === "failed" && article.last_publish_error && (
              <div className="mt-2 line-clamp-3 break-words text-xs leading-5 text-red-700">{article.last_publish_error}</div>
            )}
            <div className="mt-3 flex flex-wrap justify-end gap-2 border-t border-slate-100 pt-3">
              <a
                href={`/projects/${projectId}/articles/${article.id}`}
                className="inline-flex h-8 items-center rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:bg-slate-50"
              >
                Detail
              </a>
              {group.key === "ready" && (
                <Button disabled={busy === `publish-${article.id}`} size="sm" variant="primary" onClick={() => onPublish(article)}>
                  <ButtonProgress busy={busy === `publish-${article.id}`} busyLabel="Queuing" idleIcon={<Send size={14} />}>
                    Publish
                  </ButtonProgress>
                </Button>
              )}
              {group.key === "failed" && (
                <Button disabled={busy === `retry-${article.id}`} size="sm" variant="danger" onClick={() => onRetry(article)}>
                  <ButtonProgress busy={busy === `retry-${article.id}`} busyLabel="Retrying" idleIcon={<RotateCcw size={14} />}>
                    Retry
                  </ButtonProgress>
                </Button>
              )}
              {group.key === "ready_to_distribute" && distributeItem && (
                <>
                  <Button size="sm" onClick={() => onCopy(article)}>
                    <Copy size={14} />
                    Copy
                  </Button>
                  {distributeItem.compose_url && (
                    <a
                      href={distributeItem.compose_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex h-8 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50"
                    >
                      <ExternalLink size={14} />
                      Open
                    </a>
                  )}
                  <Button disabled={busy === `distributed-${article.id}`} size="sm" variant="primary" onClick={() => onMarkDistributed(article)}>
                    <ButtonProgress busy={busy === `distributed-${article.id}`} busyLabel="Marking distributed" idleIcon={<Send size={14} />}>
                      Mark distributed
                    </ButtonProgress>
                  </Button>
                </>
              )}
            </div>
          </div>
        );
      })}
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
  const [loading, setLoading] = useState(true);
  const [connectionsLoading, setConnectionsLoading] = useState(true);
  const { notify } = useToast();
  const setMessage = (next: Message) => {
    if (next) notify(next);
  };
  const [drawer, setDrawer] = useState<DrawerKind>(null);
  const [selectedManualPlatform, setSelectedManualPlatform] = useState<ManualPlatformID | null>(null);
  const [focusedOperationalGroup, setFocusedOperationalGroup] = useState<OperationalGroup["key"] | null>(null);

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
    } finally {
      setLoading(false);
    }
  }, [api, projectId]);

  const loadConnections = useCallback(async () => {
    setConnectionsLoading(true);
    try {
      const next = await api.listPublisherConnections(projectId);
      setConnections(next);
    } catch {
      setConnections([]);
    } finally {
      setConnectionsLoading(false);
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
    loadConnections();
  }, [refresh, loadConnections]);

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
  const now = useMemo(() => new Date(), [approvedCanonicals, failed, published, ready]);
  const readyCanonicals = useMemo(
    () => approvedCanonicals.filter((article) => !article.scheduled_at || new Date(article.scheduled_at).getTime() <= now.getTime()),
    [approvedCanonicals, now],
  );
  const scheduledCanonicals = useMemo(
    () => approvedCanonicals.filter((article) => article.scheduled_at && new Date(article.scheduled_at).getTime() > now.getTime()),
    [approvedCanonicals, now],
  );
  const publishMode: PublishMode = (config?.publish_mode as PublishMode) ?? "manual";
  const publishIntervalDays = config?.publish_interval_days ?? 2;
  const eligiblePublisherConnections = useMemo(() => connections.filter((connection) => connection.enabled && connection.status === "connected"), [connections]);
  const activePublisherConnection = useMemo(
    () => eligiblePublisherConnections.find((connection) => connection.is_default) ?? eligiblePublisherConnections[0] ?? null,
    [eligiblePublisherConnections],
  );
  const waiting = useMemo(
    () =>
      approved.filter(
        (article) =>
          article.kind === "syndication_variant" && !ready.some((item) => item.article.id === article.id),
      ),
    [approved, ready],
  );
  const manualSummary = useMemo(
    () => buildManualSyndicationSummary({ readyDistribute: ready, waitingSyndication: waiting }),
    [ready, waiting],
  );
  const destinations = useMemo(
    () =>
      buildPublishDestinations({
        projectId,
        connections,
        readyDistribute: ready,
        waitingSyndication: waiting,
        projectConfig: config,
      }),
    [projectId, connections, ready, waiting, config],
  );
  const readyNow = useMemo(
    () =>
      buildReadyNow({
        now,
        approvedCanonicals,
        failedCanonicals: failed,
        activePublisherConnection,
      }),
    [now, approvedCanonicals, failed, activePublisherConnection],
  );
  const operationalGroups = useMemo(
    () =>
      buildPublishingOperationalGroups({
        now,
        approvedCanonicals,
        publishedCanonicals: [...inflight, ...published],
        failedCanonicals: failed,
        waitingSyndication: waiting,
        readyDistribute: ready,
      }),
    [now, approvedCanonicals, inflight, published, failed, waiting, ready],
  );
  const selectedPlatform =
    manualSummary.platforms.find((platform) => platform.id === selectedManualPlatform) ?? manualSummary.platforms[0] ?? null;
  const headerCta = buildPublishHeaderCta({
    projectId,
    github: destinations.github,
    readyNowItems: readyNow.items,
    scheduledCount: scheduledCanonicals.length,
  });
  const nextHeaderArticle =
    headerCta.kind === "publish_next"
      ? readyNow.items.find((item) => item.articleId === headerCta.articleId)?.article ?? null
      : null;

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

  async function testPublisherConnection(connection: PublisherConnection | null | undefined) {
    if (!connection) return;
    setBusy(`test-${connection.id}`);
    setMessage(null);
    try {
      await api.testPublisherConnection(projectId, connection.id);
      await loadConnections();
      setMessage({ title: "Publisher connection checked", tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Connection test failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  function copyDraft(article: Article) {
    navigator.clipboard?.writeText(article.content_md);
    setMessage({ title: "Draft copied", detail: articleTitle(article), tone: "green" });
  }

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

  function openManualPlatform(platformId: ManualPlatformID) {
    setSelectedManualPlatform(platformId);
    setDrawer("manual");
  }

  function openViewAll(groupKey?: OperationalGroup["key"]) {
    setFocusedOperationalGroup(groupKey ?? null);
    setDrawer("view_all");
  }

  function renderPrimaryCta() {
    if (headerCta.kind === "settings") {
      return (
        <a
          href={headerCta.href}
          className="inline-flex h-9 items-center justify-center gap-2 rounded-lg border border-transparent bg-gradient-to-r from-[#d93820] to-[#f4503b] px-3 text-xs font-semibold text-white hover:brightness-[1.02]"
        >
          <Plug size={14} />
          {headerCta.label}
        </a>
      );
    }
    if (headerCta.kind === "view_all") {
      return (
        <Button size="sm" variant="primary" onClick={() => openViewAll(headerCta.groupKey)}>
          <CalendarClock size={14} />
          {headerCta.label}
        </Button>
      );
    }
    return (
      <Button disabled={!nextHeaderArticle || busy === `publish-${headerCta.articleId}`} size="sm" variant="primary" onClick={() => nextHeaderArticle && publishNow(nextHeaderArticle)}>
        <ButtonProgress busy={busy === `publish-${headerCta.articleId}`} busyLabel="Queuing" idleIcon={<Send size={14} />}>
          {headerCta.label}
        </ButtonProgress>
      </Button>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex min-h-8 items-center justify-end">
        <div className="flex flex-wrap items-center justify-end gap-2">
          {renderPrimaryCta()}
          <button
            type="button"
            onClick={() => setDrawer("schedule")}
            className="inline-flex h-9 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:bg-slate-50"
          >
            <Settings2 size={14} className="text-slate-400" />
            Schedule
            <span className="inline-flex items-center gap-1 text-slate-900">
              {MODE_META[publishMode].icon}
              {MODE_META[publishMode].label}
            </span>
          </button>
          <Button size="sm" onClick={() => openViewAll()}>
            <CalendarClock size={14} />
            View all
          </Button>
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
      </div>

      {destinations.github.state === "not_connected" && (
        <Notice title="No publisher connection" detail="GitHub/Next.js is the canonical publish destination. Connect it in Settings before publishing." tone="amber" />
      )}
      {destinations.github.state === "disabled" && (
        <Notice title="Publisher disabled" detail="Ready posts stay visible, but publish actions are disabled until the destination is enabled." tone="amber" />
      )}
      {destinations.github.state === "needs_attention" && (
        <Notice title="Publisher needs attention" detail={destinations.github.connection?.last_error || "Fix the GitHub/Next.js connection in Settings."} tone="red" />
      )}

      <div data-publish-c2-first-viewport className="grid min-w-0 gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(300px,0.46fr)] xl:items-start">
        <ReadyNowStrip
          projectId={projectId}
          readyNow={readyNow}
          busy={busy}
          activePublisherConnection={activePublisherConnection}
          onPublish={publishNow}
          onRetry={retryPublish}
          onDestination={() => setDrawer("github")}
          onTiming={() => setDrawer("schedule")}
        />

        <section data-publish-c2-destinations className="min-w-0 space-y-3">
          <SectionHeader title="Publish destinations" action={<Badge tone="neutral">{destinations.firstViewport.length} visible</Badge>} />
          {connectionsLoading ? (
            <DestinationSkeleton />
          ) : (
            <div className="grid min-w-0 gap-3 sm:grid-cols-2 xl:grid-cols-1">
              <DestinationTile destination={destinations.github} featured onOpen={() => setDrawer("github")} />

              <div data-publish-manual-destination-tiles className="contents">
                {destinations.firstViewport
                  .filter((destination) => destination.kind === "manual")
                  .map((destination) => (
                    <DestinationTile
                      key={destination.id}
                      destination={destination}
                      onOpen={() => openManualPlatform(destination.id as ManualPlatformID)}
                    />
                  ))}
              </div>
              <DestinationTile destination={destinations.roadmap} onOpen={() => setDrawer("cms")} />
            </div>
          )}

          <div className="flex min-w-0 flex-wrap items-center gap-2" data-manual-syndication-chips>
            {manualSummary.readyChips.map((platform) => (
              <button
                key={platform.id}
                type="button"
                onClick={() => openManualPlatform(platform.id)}
                className="inline-flex h-8 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:bg-slate-50"
              >
                {platform.label}
                <Badge tone="green">{platform.readyCount}</Badge>
              </button>
            ))}
            {destinations.moreManual.length > 0 && (
              <button
                type="button"
                data-publish-more-manual
                onClick={() => setDrawer("more")}
                className="inline-flex h-8 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:bg-slate-50"
              >
                More
                <span className="text-slate-400">Medium · LinkedIn · Hacker News</span>
            </button>
          )}
        </div>
      </section>
      </div>

      {loading && <EmptyState title="Loading publishing state" detail="Ready content and destination status is loading." />}

      <Drawer
        open={drawer === "github"}
        title="GitHub/Next.js"
        subtitle={destinations.github.stateLabel}
        onClose={() => setDrawer(null)}
      >
        <div className="space-y-4">
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <div className="flex flex-wrap items-center gap-2">
              <Badge tone={platformTone(destinations.github)}>{destinations.github.stateLabel}</Badge>
              <span className="text-sm font-semibold text-slate-700">{connectionTargetLabel(destinations.github.connection ?? null)}</span>
            </div>
            {destinations.github.connection && (
              <div className="mt-2 break-words text-xs font-medium text-slate-500">{publisherConnectionDetail(destinations.github.connection)}</div>
            )}
          </div>
          <div className="flex flex-wrap justify-end gap-2">
            {readyNow.items.some((item) => item.action === "publish" && !item.disabled) && (
              <Button
                size="sm"
                variant="primary"
                disabled={!nextHeaderArticle || busy === `publish-${nextHeaderArticle?.id}`}
                onClick={() => nextHeaderArticle && publishNow(nextHeaderArticle)}
              >
                <ButtonProgress busy={Boolean(nextHeaderArticle && busy === `publish-${nextHeaderArticle.id}`)} busyLabel="Queuing" idleIcon={<Send size={14} />}>
                  Publish next
                </ButtonProgress>
              </Button>
            )}
            {scheduledCanonicals.length > 0 && (
              <Button size="sm" onClick={() => openViewAll("scheduled")}>
                <CalendarClock size={14} />
                View schedule
              </Button>
            )}
            {(destinations.github.state === "needs_attention" || destinations.github.connection) && (
              <Button size="sm" disabled={!destinations.github.connection || busy === `test-${destinations.github.connection?.id}`} onClick={() => testPublisherConnection(destinations.github.connection)}>
                <ButtonProgress
                  busy={Boolean(destinations.github.connection && busy === `test-${destinations.github.connection.id}`)}
                  busyLabel="Testing"
                  idleIcon={<RotateCcw size={14} />}
                >
                  Retry test
                </ButtonProgress>
              </Button>
            )}
            <a
              href={`/projects/${projectId}/settings#publisher`}
              className="inline-flex h-8 items-center justify-center rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:bg-slate-50"
            >
              Manage in Settings
            </a>
          </div>
        </div>
      </Drawer>

      <Drawer
        open={drawer === "manual" && Boolean(selectedPlatform)}
        title={selectedPlatform?.label ?? "Manual drafts"}
        subtitle={selectedPlatform?.actionLabel}
        onClose={() => setDrawer(null)}
      >
        {selectedPlatform && (
          <ManualPlatformRows
            projectId={projectId}
            platform={selectedPlatform}
            busy={busy}
            onCopy={copyDraft}
            onMarkDistributed={markDistributed}
          />
        )}
      </Drawer>

      <Drawer
        open={drawer === "more"}
        title="More manual platforms"
        subtitle="Supported manual syndication destinations stay here unless a project has variants for them."
        onClose={() => setDrawer(null)}
      >
        <div className="grid gap-2">
          {manualSummary.platforms.map((platform) => (
            <button
              key={platform.id}
              type="button"
              onClick={() => openManualPlatform(platform.id)}
              className="flex items-center justify-between gap-3 rounded-lg border border-slate-200 bg-white px-3 py-2 text-left hover:bg-slate-50"
            >
              <span className="min-w-0">
                <span className="block truncate text-sm font-bold text-slate-900">{platform.label}</span>
                <span className="text-xs font-semibold text-slate-500">{platform.actionLabel}</span>
              </span>
              <span className="flex shrink-0 items-center gap-2">
                {platform.readyCount > 0 && <Badge tone="green">{platform.readyCount} ready</Badge>}
                {platform.waitingCount > 0 && <Badge tone="neutral">{platform.waitingCount} waiting</Badge>}
              </span>
            </button>
          ))}
        </div>
      </Drawer>

      <Drawer
        open={drawer === "cms"}
        title="CMS roadmap"
        subtitle="These connectors are planned and are not publish actions yet."
        onClose={() => setDrawer(null)}
      >
        <div className="grid gap-3">
          {destinations.roadmap.platforms?.map((platform) => (
            <div key={platform} className="flex items-center justify-between rounded-lg border border-dashed border-slate-200 bg-slate-50 px-3 py-2">
              <span className="text-sm font-bold text-slate-800">{platform}</span>
              <Badge tone="neutral">Roadmap</Badge>
            </div>
          ))}
          <a
            href={`/projects/${projectId}/settings#publisher`}
            className="inline-flex h-8 w-fit items-center justify-center rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-700 hover:bg-slate-50"
          >
            Open Settings
          </a>
        </div>
      </Drawer>

      <Drawer
        open={drawer === "view_all"}
        title="View all"
        subtitle="Ready, scheduled, published, failed, waiting, and ready-to-distribute items."
        onClose={() => setDrawer(null)}
      >
        <div data-publish-view-all-drawer className="space-y-5">
          {operationalGroups.map((group) => (
            <section key={group.key} data-publish-operational-group={group.key} className={cx(focusedOperationalGroup && focusedOperationalGroup !== group.key ? "opacity-60" : "")}>
              <SectionHeader title={group.label} action={<Badge tone={group.count ? "blue" : "neutral"}>{group.count}</Badge>} />
              <OperationalRows
                group={group}
                projectId={projectId}
                busy={busy}
                onCopy={copyDraft}
                onPublish={publishNow}
                onRetry={retryPublish}
                onMarkDistributed={markDistributed}
              />
            </section>
          ))}
        </div>
      </Drawer>

      <Drawer
        open={drawer === "schedule"}
        title="Publish schedule"
        subtitle="How approved drafts go live. This is separate from destination capability."
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
