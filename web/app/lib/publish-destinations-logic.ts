import type { Article, DistributeItem, ProjectConfig, PublisherConnection } from "./api";

type PublishConnectionState = "ready" | "not_connected" | "disabled" | "needs_attention";
type DestinationKind = "canonical" | "manual" | "roadmap";
type ManualPlatformId = "dev_to" | "hashnode" | "reddit" | "medium" | "linkedin" | "hacker_news";
type ReadyNowAction = "publish" | "retry" | "publishing";
type OperationalGroupKey =
  | "ready"
  | "scheduled"
  | "failed"
  | "waiting_on_canonical"
  | "ready_to_distribute";

const DEFAULT_MANUAL_PLATFORMS: ManualPlatformId[] = ["dev_to", "hashnode", "reddit"];
const MORE_MANUAL_PLATFORMS: ManualPlatformId[] = ["medium", "linkedin", "hacker_news"];
const CMS_ROADMAP_PLATFORMS = ["WordPress", "Webflow", "Shopify", "Custom CMS"];

const MANUAL_PLATFORM_META: Record<
  ManualPlatformId,
  {
    label: string;
    actionLabel: "Copy draft" | "Submit draft";
    supportsCanonical: boolean;
    defaultVisible: boolean;
  }
> = {
  dev_to: { label: "Dev.to", actionLabel: "Copy draft", supportsCanonical: true, defaultVisible: true },
  hashnode: { label: "Hashnode", actionLabel: "Copy draft", supportsCanonical: true, defaultVisible: true },
  reddit: { label: "Reddit", actionLabel: "Submit draft", supportsCanonical: false, defaultVisible: true },
  medium: { label: "Medium", actionLabel: "Copy draft", supportsCanonical: true, defaultVisible: false },
  linkedin: { label: "LinkedIn", actionLabel: "Copy draft", supportsCanonical: true, defaultVisible: false },
  hacker_news: { label: "Hacker News", actionLabel: "Submit draft", supportsCanonical: false, defaultVisible: false },
};

export type PublishDestination = {
  id: "github_nextjs" | ManualPlatformId | "cms_roadmap";
  label: string;
  kind: DestinationKind;
  state: PublishConnectionState | "manual" | "roadmap";
  stateLabel: string;
  actionLabel: string;
  targetDetail?: string;
  readyCount: number;
  waitingCount: number;
  isPublishAction: boolean;
  settingsHref?: string;
  connection?: PublisherConnection | null;
  platforms?: string[];
};

export type PublishDestinationsModel = {
  github: PublishDestination;
  firstViewport: PublishDestination[];
  moreManual: PublishDestination[];
  roadmap: PublishDestination;
};

export type ReadyNowItem = {
  id: string;
  articleId: string;
  title: string;
  action: ReadyNowAction;
  actionLabel: "Publish" | "Retry" | "Publishing";
  secondaryActionLabel: "Preview";
  destinationLabel: "GitHub/Next.js";
  destinationActionLabel: "Destination";
  publishStateLabel: string;
  disabled: boolean;
  disabledReason?: string;
  failureReason?: string;
  article: Article;
};

export type ReadyNowModel = {
  items: ReadyNowItem[];
  emptyState: {
    title: "No approved posts ready";
    detail: "Approved canonical posts appear here after review.";
  };
};

export type ManualSyndicationRow = {
  articleId: string;
  title: string;
  composeUrl?: string;
  supportsCanonical?: boolean;
  actions: Array<"Copy" | "Open" | "Mark distributed">;
  article: Article;
};

export type ManualSyndicationPlatform = {
  id: ManualPlatformId;
  label: string;
  actionLabel: "Copy draft" | "Submit draft";
  readyCount: number;
  waitingCount: number;
  readyRows: ManualSyndicationRow[];
  waitingRows: ManualSyndicationRow[];
};

export type ManualSyndicationSummary = {
  platforms: ManualSyndicationPlatform[];
  readyChips: ManualSyndicationPlatform[];
};

export type OperationalGroup = {
  key: OperationalGroupKey;
  label: string;
  count: number;
  items: Array<Article | DistributeItem>;
};

export type PublishedCanonicalRow = {
  articleId: string;
  title: string;
  publishedAt?: string | null;
  publishedUrl?: string;
  urlMissing: boolean;
  article: Article;
};

export type PublishedCanonicalsSection = {
  key: "published";
  label: "Published";
  count: number;
  rows: PublishedCanonicalRow[];
};

export type PublishingOperationalModel = {
  published: PublishedCanonicalsSection;
  groups: OperationalGroup[];
};

type BuildDestinationsInput = {
  projectId: string;
  connections: PublisherConnection[];
  readyDistribute: DistributeItem[];
  waitingSyndication: Article[];
  projectConfig?: Partial<ProjectConfig> | null;
};

type BuildReadyNowInput = {
  now?: Date | string | number;
  approvedCanonicals: Article[];
  failedCanonicals: Article[];
  inflightCanonicals?: Article[];
  activePublisherConnection: PublisherConnection | null;
  githubState?: PublishConnectionState;
  publishMode?: "scheduled" | "manual";
};

type BuildManualSyndicationInput = {
  readyDistribute: DistributeItem[];
  waitingSyndication: Article[];
};

type BuildOperationalGroupsInput = {
  now?: Date | string | number;
  approvedCanonicals: Article[];
  publishedCanonicals: Article[];
  failedCanonicals: Article[];
  waitingSyndication: Article[];
  readyDistribute: DistributeItem[];
};

type BuildHeaderCtaInput = {
  projectId: string;
  github: PublishDestination;
  readyNowItems: ReadyNowItem[];
  scheduledCount: number;
};

export type PublishHeaderCta =
  | { label: "Connect GitHub" | "Enable publishing" | "Fix connection" | "Manage destinations"; kind: "settings"; href: string }
  | { label: "View schedule"; kind: "view_all"; groupKey: "scheduled" }
  | null;

function settingsHref(projectId: string) {
  return `/projects/${projectId}/settings#publisher`;
}

function articleTitle(article: Article) {
  return article.seo_meta?.title || article.seo_meta?.slug || `${article.kind} article`;
}

function normalizePlatform(platform?: string | null): ManualPlatformId | null {
  const normalized = (platform ?? "").trim().toLowerCase().replace(/[.\-\s]/g, "_");
  if (normalized === "devto") return "dev_to";
  if (normalized === "hn" || normalized === "hackernews") return "hacker_news";
  if (normalized in MANUAL_PLATFORM_META) return normalized as ManualPlatformId;
  return null;
}

function toTime(value: unknown) {
  if (!value) return null;
  const time = new Date(value as string).getTime();
  return Number.isFinite(time) ? time : null;
}

function resolveNow(now?: Date | string | number) {
  if (now instanceof Date) return now.getTime();
  if (typeof now === "string" || typeof now === "number") {
    const parsed = new Date(now).getTime();
    return Number.isFinite(parsed) ? parsed : Date.now();
  }
  return Date.now();
}

function isCanonical(article: Article) {
  return article.kind === "canonical";
}

function isSyndicationVariant(article: Article) {
  return article.kind === "syndication_variant";
}

function isDue(article: Article, nowMs: number) {
  const scheduled = toTime(article.scheduled_at);
  return scheduled === null || scheduled <= nowMs;
}

function isScheduled(article: Article, nowMs: number) {
  const scheduled = toTime(article.scheduled_at);
  return scheduled !== null && scheduled > nowMs;
}

function isPendingUrlVerification(article: Article) {
  const publishPhase = (article as Article & { publish_phase?: string | null }).publish_phase;
  return article.status === "pending_url_verification" || publishPhase === "pending_url_verification";
}

function githubConnection(connections: PublisherConnection[]) {
  const githubConnections = connections.filter((connection) => connection.kind === "github_nextjs");
  return githubConnections.find((connection) => connection.is_default) ?? githubConnections[0] ?? null;
}

function githubState(connection: PublisherConnection | null): PublishConnectionState {
  if (!connection || connection.status === "missing") return "not_connected";
  if (connection.status === "error" || connection.status === "revoked") return "needs_attention";
  if (connection.status === "connected" && !connection.enabled) return "disabled";
  if (connection.status === "connected" && connection.enabled) return "ready";
  return "not_connected";
}

function githubStateLabel(state: PublishConnectionState) {
  switch (state) {
    case "ready":
      return "Ready";
    case "disabled":
      return "Disabled";
    case "needs_attention":
      return "Needs attention";
    case "not_connected":
    default:
      return "Not connected";
  }
}

function disabledReasonFor(state: PublishConnectionState, action: "publishing" | "retrying") {
  if (state === "disabled") {
    return `Enable GitHub/Next.js publishing before ${action}.`;
  }
  if (state === "needs_attention") {
    return `Fix GitHub/Next.js before ${action}.`;
  }
  return `Connect GitHub before ${action}.`;
}

function publishStateLabel(article: Article, publishMode?: "scheduled" | "manual") {
  if (isPendingUrlVerification(article)) {
    return "Verifying live URL";
  }
  if (article.status === "publish_failed") {
    return "Failed";
  }
  if (publishMode === "scheduled") {
    return "Ready to publish";
  }
  return "Manual: publish when ready";
}

function canonicalUrl(article: Article) {
  const value = (article.canonical_url ?? "").trim();
  return value || undefined;
}

function connectionTarget(connection: PublisherConnection | null) {
  if (!connection) return undefined;
  return connection.label || connection.config?.repo || connection.config?.base_url || undefined;
}

function manualPlatformOrder() {
  return [...DEFAULT_MANUAL_PLATFORMS, ...MORE_MANUAL_PLATFORMS];
}

function readyRowsForPlatform(readyDistribute: DistributeItem[], platformId: ManualPlatformId): ManualSyndicationRow[] {
  return readyDistribute
    .filter((item) => isSyndicationVariant(item.article) && normalizePlatform(item.article.platform) === platformId)
    .map((item) => ({
      articleId: item.article.id,
      title: articleTitle(item.article),
      composeUrl: item.compose_url || undefined,
      supportsCanonical: item.supports_canonical,
      actions: item.compose_url ? ["Copy", "Open", "Mark distributed"] : ["Copy", "Mark distributed"],
      article: item.article,
    }));
}

function waitingRowsForPlatform(waitingSyndication: Article[], platformId: ManualPlatformId): ManualSyndicationRow[] {
  return waitingSyndication
    .filter((article) => isSyndicationVariant(article) && normalizePlatform(article.platform) === platformId)
    .map((article) => ({
      articleId: article.id,
      title: articleTitle(article),
      actions: [],
      article,
    }));
}

function manualDestination(platform: ManualSyndicationPlatform): PublishDestination {
  return {
    id: platform.id,
    label: platform.label,
    kind: "manual",
    state: "manual",
    stateLabel: platform.actionLabel,
    actionLabel: platform.readyCount > 0 ? `${platform.readyCount} ready` : platform.actionLabel,
    readyCount: platform.readyCount,
    waitingCount: platform.waitingCount,
    isPublishAction: false,
  };
}

function roadmapDestination(): PublishDestination {
  return {
    id: "cms_roadmap",
    label: "CMS roadmap",
    kind: "roadmap",
    state: "roadmap",
    stateLabel: "Roadmap",
    actionLabel: "Learn more",
    readyCount: 0,
    waitingCount: 0,
    isPublishAction: false,
    platforms: CMS_ROADMAP_PLATFORMS,
  };
}

export function buildManualSyndicationSummary(input: BuildManualSyndicationInput): ManualSyndicationSummary {
  const platforms = manualPlatformOrder().map((platformId) => {
    const meta = MANUAL_PLATFORM_META[platformId];
    const readyRows = readyRowsForPlatform(input.readyDistribute, platformId);
    const waitingRows = waitingRowsForPlatform(input.waitingSyndication, platformId);

    return {
      id: platformId,
      label: meta.label,
      actionLabel: meta.actionLabel,
      readyCount: readyRows.length,
      waitingCount: waitingRows.length,
      readyRows,
      waitingRows,
    };
  });

  return {
    platforms,
    readyChips: platforms.filter((platform) => platform.readyCount > 0),
  };
}

export function buildPublishDestinations(input: BuildDestinationsInput): PublishDestinationsModel {
  const connection = githubConnection(input.connections);
  const state = githubState(connection);
  const summary = buildManualSyndicationSummary({
    readyDistribute: input.readyDistribute,
    waitingSyndication: input.waitingSyndication,
  });
  const roadmap = roadmapDestination();
  const github: PublishDestination = {
    id: "github_nextjs",
    label: "GitHub/Next.js",
    kind: "canonical",
    state,
    stateLabel: githubStateLabel(state),
    actionLabel:
      state === "ready"
        ? "Ready"
        : state === "disabled"
          ? "Enable publishing"
          : state === "needs_attention"
            ? "Fix connection"
            : "Connect GitHub",
    targetDetail: connectionTarget(connection),
    readyCount: 0,
    waitingCount: 0,
    isPublishAction: false,
    settingsHref: settingsHref(input.projectId),
    connection,
  };

  const manualDestinations = summary.platforms.map(manualDestination);
  const firstManual = manualDestinations.filter((destination) => {
    const meta = MANUAL_PLATFORM_META[destination.id as ManualPlatformId];
    return meta.defaultVisible || destination.readyCount > 0 || destination.waitingCount > 0;
  });
  const moreManual = manualDestinations.filter((destination) => !firstManual.some((visible) => visible.id === destination.id));

  return {
    github,
    firstViewport: [github, ...firstManual, roadmap],
    moreManual,
    roadmap,
  };
}

export function buildReadyNow(input: BuildReadyNowInput): ReadyNowModel {
  const nowMs = resolveNow(input.now);
  const state = input.githubState ?? (input.activePublisherConnection ? "ready" : "not_connected");
  const active = state === "ready" && Boolean(input.activePublisherConnection);
  const readyCanonicals = input.approvedCanonicals.filter((article) => isCanonical(article) && isDue(article, nowMs));
  const inflightCanonicals = (input.inflightCanonicals ?? []).filter((article) => isCanonical(article) && isPendingUrlVerification(article));
  const failedCanonicals = input.failedCanonicals.filter(isCanonical);
  const publishItems: ReadyNowItem[] = readyCanonicals.map((article) => ({
    id: `publish-${article.id}`,
    articleId: article.id,
    title: articleTitle(article),
    action: "publish",
    actionLabel: "Publish",
    secondaryActionLabel: "Preview",
    destinationLabel: "GitHub/Next.js",
    destinationActionLabel: "Destination",
    publishStateLabel: publishStateLabel(article, input.publishMode),
    disabled: !active,
    disabledReason: active ? undefined : disabledReasonFor(state, "publishing"),
    article,
  }));
  const publishingItems: ReadyNowItem[] = inflightCanonicals.map((article) => ({
    id: `publishing-${article.id}`,
    articleId: article.id,
    title: articleTitle(article),
    action: "publishing",
    actionLabel: "Publishing",
    secondaryActionLabel: "Preview",
    destinationLabel: "GitHub/Next.js",
    destinationActionLabel: "Destination",
    publishStateLabel: publishStateLabel(article, input.publishMode),
    disabled: true,
    article,
  }));
  const retryItems: ReadyNowItem[] = failedCanonicals.map((article) => ({
    id: `retry-${article.id}`,
    articleId: article.id,
    title: articleTitle(article),
    action: "retry",
    actionLabel: "Retry",
    secondaryActionLabel: "Preview",
    destinationLabel: "GitHub/Next.js",
    destinationActionLabel: "Destination",
    publishStateLabel: publishStateLabel(article, input.publishMode),
    disabled: !active,
    disabledReason: active ? undefined : disabledReasonFor(state, "retrying"),
    failureReason: article.last_publish_error || undefined,
    article,
  }));

  return {
    items: [...publishItems, ...publishingItems, ...retryItems],
    emptyState: {
      title: "No approved posts ready",
      detail: "Approved canonical posts appear here after review.",
    },
  };
}

export function buildPublishingOperationalGroups(input: BuildOperationalGroupsInput): PublishingOperationalModel {
  const nowMs = resolveNow(input.now);
  const approvedCanonicals = input.approvedCanonicals.filter(isCanonical);
  const ready = approvedCanonicals.filter((article) => isDue(article, nowMs));
  const scheduled = approvedCanonicals.filter((article) => isScheduled(article, nowMs));
  const publishedRows = input.publishedCanonicals.filter(isCanonical).map((article) => {
    const publishedUrl = canonicalUrl(article);
    return {
      articleId: article.id,
      title: articleTitle(article),
      publishedAt: article.published_at,
      publishedUrl,
      urlMissing: !publishedUrl,
      article,
    };
  });
  const failed = input.failedCanonicals.filter(isCanonical);
  const waiting = input.waitingSyndication.filter(isSyndicationVariant);
  const readyToDistribute = input.readyDistribute.filter((item) => isSyndicationVariant(item.article));

  return {
    published: {
      key: "published",
      label: "Published",
      count: publishedRows.length,
      rows: publishedRows,
    },
    groups: [
      { key: "ready", label: "Ready", count: ready.length, items: ready },
      { key: "scheduled", label: "Scheduled", count: scheduled.length, items: scheduled },
      { key: "failed", label: "Failed", count: failed.length, items: failed },
      { key: "waiting_on_canonical", label: "Waiting on canonical", count: waiting.length, items: waiting },
      { key: "ready_to_distribute", label: "Ready to distribute", count: readyToDistribute.length, items: readyToDistribute },
    ],
  };
}

export function buildPublishHeaderCta(input: BuildHeaderCtaInput): PublishHeaderCta {
  if (input.github.state === "not_connected") {
    return { label: "Connect GitHub", kind: "settings", href: settingsHref(input.projectId) };
  }
  if (input.github.state === "disabled") {
    return { label: "Enable publishing", kind: "settings", href: settingsHref(input.projectId) };
  }
  if (input.github.state === "needs_attention") {
    return { label: "Fix connection", kind: "settings", href: settingsHref(input.projectId) };
  }

  if (input.scheduledCount > 0) {
    return { label: "View schedule", kind: "view_all", groupKey: "scheduled" };
  }
  return null;
}
