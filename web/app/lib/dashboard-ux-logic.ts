export type NextWorkspaceActionInput = {
  projectId: string;
  hasProfile: boolean;
  contextConfirmed?: boolean;
  failedPublishCount: number;
  hasBlockedDrafts: boolean;
  reviewCount: number;
  readyCount: number;
  topicsCount: number;
  openOpportunityCount?: number;
  currentPathname?: string;
};

export type WorkspaceAction = {
  title: string;
  detail: string;
  href: string;
};

export type ActionableMomentumInput = {
  projectId: string;
  hasProfile: boolean;
  publishedThisMonthCount: number;
  approvedDraftCount: number;
  opportunitiesConvertedCount: number;
  readyToDistributeCount: number;
  activeLoopItemCount: number;
};

export type ActionableMomentumItem = {
  id: string;
  label: string;
  value: number;
  detail: string;
  href: string;
  actionLabel: string;
  tone: "green" | "amber" | "blue" | "neutral";
};

export type ActionableMomentumResult = {
  items: ActionableMomentumItem[];
  emptyAction: (WorkspaceAction & { actionLabel: string }) | null;
};

export type HomeMetricTone = "green" | "amber" | "blue" | "red" | "neutral";

export type HomeMetricSummary = {
  label: string;
  value: number | string;
  detail: string;
  metricChangeLabel: string;
  metricChangeTone: HomeMetricTone;
  href: string;
  muted: boolean;
};

export type HomeAICitationMetricInput = {
  projectId: string;
  citationGapCount: number;
};

export type HomeInMotionMetricInput = {
  projectId: string;
  analysisActionCount: number;
  reviewDraftCount: number;
  readyToPublishCount: number;
  measuringActionCount: number;
};

export type HomeEventInput = {
  projectId: string;
  liveActivities?: Array<{
    id: string;
    title: string;
    detail: string;
    href: string;
  }>;
  recentEvents?: Array<{
    id: string;
    title: string;
    detail: string;
    href: string;
  }>;
  nextEvent?: {
    title: string;
    detail: string;
    href: string;
  } | null;
  limit?: number;
};

export type HomeEventStreamItem = {
  id: string;
  kind: "live" | "recent" | "next";
  title: string;
  detail: string;
  href: string;
  timeLabel: string;
};

export type HomeEventStreamResult = {
  items: HomeEventStreamItem[];
  emptyAction: (WorkspaceAction & { actionLabel: string }) | null;
};

export type HomeSectionCandidate = {
  id: string;
  count: number;
  priority: number;
};

export type ContextBuildTrackState = "waiting" | "running" | "done" | "attention";

export type ContextBuildRun = {
  input?: Record<string, any> | null;
  output?: Record<string, any> | null;
  status?: string | null;
  error?: string | null;
};

export type ContextBuildTracksInput = {
  hasProfile: boolean;
  sourcePageCount: number;
  evidencePageCount: number;
  evidenceCount: number;
  pollCount: number;
  pollLimit: number;
  runs?: ContextBuildRun[];
};

export type ContextBuildTrack = {
  id: "profile" | "source-crawl" | "evidence";
  label: string;
  detail: string;
  state: ContextBuildTrackState;
  progress: number;
  current: number;
  target: number;
};

export type ContextInventoryProgressItem = {
  source?: string | null;
  evidence_snippets?: any[] | null;
};

export function contextInventoryProgress(items: ContextInventoryProgressItem[]) {
  const sourceItems = items.filter((item) => item.source !== "generated");
  const evidenceItems = sourceItems.filter((item) => Array.isArray(item.evidence_snippets) && item.evidence_snippets.length > 0);
  return {
    sourcePageCount: sourceItems.length,
    evidencePageCount: evidenceItems.length,
    evidenceCount: evidenceItems.reduce((total, item) => total + (Array.isArray(item.evidence_snippets) ? item.evidence_snippets.length : 0), 0),
  };
}

export type ContextBuildTracks = {
  active: boolean;
  exhausted: boolean;
  title: string;
  detail: string;
  tracks: ContextBuildTrack[];
};

export type ProfileDraft = {
  positioning: string;
  icp: string;
  value_props: string;
  features: string;
  differentiators: string;
  competitors: string;
  key_terms: string;
  tone: string;
  banned_claims: string;
  content_rules: string;
  advancedJSON: string;
};

export function lines(value: string) {
  return value
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

export function nextWorkspaceAction({
  projectId,
  hasProfile,
  contextConfirmed = true,
  failedPublishCount,
  hasBlockedDrafts,
  reviewCount,
  readyCount,
  topicsCount,
  openOpportunityCount = 0,
}: NextWorkspaceActionInput): WorkspaceAction {
  if (!hasProfile) {
    return {
      title: "Refresh context",
      detail: "Confirm product facts, evidence, and positioning before generating a content plan.",
      href: `/projects/${projectId}/context`,
    };
  }
  if (!contextConfirmed) {
    return {
      title: "Confirm context",
      detail: "Review and confirm the generated Context before CiteLoop finds opportunities.",
      href: `/projects/${projectId}/context`,
    };
  }
  if (failedPublishCount > 0) {
    return {
      title: "Fix publishing",
      detail: "A canonical article could not be confirmed online, so related variants may stay locked.",
      href: `/projects/${projectId}/publish`,
    };
  }
  if (hasBlockedDrafts) {
    return {
      title: "Review blocked drafts",
      detail: "Some drafts need a manual review before they can be approved.",
      href: `/projects/${projectId}/review`,
    };
  }
  if (reviewCount > 0) {
    return {
      title: "Review drafts",
      detail: "Generated drafts are waiting for the human approval gate.",
      href: `/projects/${projectId}/review`,
    };
  }
  if (readyCount > 0) {
    return {
      title: "Distribute variants",
      detail: "Approved variants are ready after their canonical article went live.",
      href: `/projects/${projectId}/publish`,
    };
  }
  if (openOpportunityCount > 0) {
    return {
      title: "Review opportunities",
      detail: `${openOpportunityCount} opportunities are ready to review before CiteLoop advances the content plan.`,
      href: `/projects/${projectId}/analysis`,
    };
  }
  if (topicsCount === 0) {
    return {
      title: "Create plan",
      detail: "No opportunity review is waiting; open Content Plan to retry or seed the first backlog.",
      href: `/projects/${projectId}/plan`,
    };
  }
  return {
    title: "Refresh context",
    detail: "Keep product facts, evidence, and positioning current before the next content cycle.",
    href: `/projects/${projectId}/context`,
  };
}

export function buildActionableMomentum(input: ActionableMomentumInput): ActionableMomentumResult {
  const candidates: ActionableMomentumItem[] = [
    {
      id: "ready-to-publish",
      label: "Ready to publish",
      value: input.readyToDistributeCount,
      detail: "approved variants can move now",
      href: `/projects/${input.projectId}/publish`,
      actionLabel: "Publish",
      tone: "amber",
    },
    {
      id: "published-this-month",
      label: "Published this month",
      value: input.publishedThisMonthCount,
      detail: "live assets feeding results",
      href: `/projects/${input.projectId}/results`,
      actionLabel: "View impact",
      tone: "green",
    },
    {
      id: "opportunities-converted",
      label: "Opportunities converted",
      value: input.opportunitiesConvertedCount,
      detail: "opportunities entered the loop",
      href: `/projects/${input.projectId}/analysis`,
      actionLabel: "Review loop",
      tone: "blue",
    },
    {
      id: "active-loop-items",
      label: "Active loop items",
      value: input.activeLoopItemCount,
      detail: "items moving from insight to impact",
      href: `/projects/${input.projectId}`,
      actionLabel: "Timeline",
      tone: "neutral",
    },
    {
      id: "approved-drafts",
      label: "Approved drafts",
      value: input.approvedDraftCount,
      detail: "approved drafts waiting on publish",
      href: `/projects/${input.projectId}/publish`,
      actionLabel: "Publish",
      tone: "amber",
    },
  ];
  const items = candidates.filter((item) => item.value > 0).slice(0, 4);

  if (items.length > 0) {
    return { items, emptyAction: null };
  }

  if (!input.hasProfile) {
    return {
      items: [],
      emptyAction: {
        title: "Context needs confirmation",
        detail: "Connect product facts and source evidence before CiteLoop can generate a plan.",
        href: `/projects/${input.projectId}/context`,
        actionLabel: "Open Context",
      },
    };
  }

  return {
    items: [],
    emptyAction: {
      title: "Context is ready",
      detail: "Review opportunities when recommendations appear; CiteLoop will advance planning and drafting automatically after the review gate.",
      href: `/projects/${input.projectId}/analysis`,
      actionLabel: "Review opportunities",
    },
  };
}

function plural(count: number, singular: string, pluralValue = `${singular}s`) {
  return count === 1 ? singular : pluralValue;
}

export function homeAICitationMetric(input: HomeAICitationMetricInput): HomeMetricSummary {
  const count = Math.max(0, input.citationGapCount);
  return {
    label: "AI citation gaps",
    value: count > 0 ? count : "-",
    detail: count > 0 ? `${count} ${plural(count, "finding")} ready in Opportunities` : "No AI citation gaps detected",
    metricChangeLabel: count > 0 ? "Review opportunities" : "No open gaps",
    metricChangeTone: count > 0 ? "green" : "neutral",
    href: `/projects/${input.projectId}/analysis`,
    muted: count === 0,
  };
}

export function homeInMotionMetric(input: HomeInMotionMetricInput): HomeMetricSummary {
  const analysisActionCount = Math.max(0, input.analysisActionCount);
  const reviewDraftCount = Math.max(0, input.reviewDraftCount);
  const readyToPublishCount = Math.max(0, input.readyToPublishCount);
  const measuringActionCount = Math.max(0, input.measuringActionCount);
  const value = analysisActionCount + reviewDraftCount + readyToPublishCount + measuringActionCount;
  const detailParts = [
    analysisActionCount > 0 && `${analysisActionCount} opportunity ${plural(analysisActionCount, "action")} already in execution`,
    reviewDraftCount > 0 && `${reviewDraftCount} ${plural(reviewDraftCount, "draft")} in review`,
    readyToPublishCount > 0 && `${readyToPublishCount} ready to publish`,
    measuringActionCount > 0 && `${measuringActionCount} measuring impact`,
  ].filter(Boolean);
  const href = analysisActionCount > 0
    ? `/projects/${input.projectId}/analysis`
    : reviewDraftCount > 0
      ? `/projects/${input.projectId}/review`
      : readyToPublishCount > 0
        ? `/projects/${input.projectId}/publish`
        : measuringActionCount > 0
          ? `/projects/${input.projectId}/results`
          : `/projects/${input.projectId}/analysis`;
  const metricChangeLabel = analysisActionCount > 0
    ? "View opportunities"
    : reviewDraftCount > 0
      ? "Open Review"
      : readyToPublishCount > 0
        ? "Open Publish"
        : measuringActionCount > 0
          ? "View Results"
          : "0 active now";

  return {
    label: "In motion",
    value,
    detail: detailParts.length ? detailParts.join(" / ") : "No active loop work",
    metricChangeLabel,
    metricChangeTone: value > 0 ? "blue" : "neutral",
    href,
    muted: value === 0,
  };
}

function boundedPercent(current: number, target: number) {
  const safeTarget = Math.max(1, target);
  return Math.max(0, Math.min(100, Math.round((Math.max(0, current) / safeTarget) * 100)));
}

function runStep(run: ContextBuildRun) {
  const step = run.input?.step;
  return typeof step === "string" ? step : "";
}

function runPhase(run: ContextBuildRun) {
  const phase = run.input?.phase;
  return typeof phase === "string" ? phase : "";
}

function latestCrawlSummary(runs: ContextBuildRun[]) {
  for (const run of runs) {
    const summary = run.output?.crawl_summary;
    if (runStep(run) === "crawl_summary" && summary && typeof summary === "object") {
      return summary as Record<string, any>;
    }
  }
  return null;
}

function numericSummaryValue(summary: Record<string, any> | null, key: string) {
  const value = summary?.[key];
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

const contextBuildTargetPages = 20;

export function contextBuildTracks({
  hasProfile,
  sourcePageCount,
  evidencePageCount,
  evidenceCount,
  pollCount,
  pollLimit,
  runs = [],
}: ContextBuildTracksInput): ContextBuildTracks {
  const pollingExhausted = pollCount >= pollLimit;
  const summary = latestCrawlSummary(runs);
  const fetchedCount = numericSummaryValue(summary, "fetched_count");
  const inventoryCount = numericSummaryValue(summary, "inventory_count");
  const crawlErrors = Array.isArray(summary?.errors) ? summary.errors.length : 0;
  const crawlSummaryReported = Boolean(summary);
  const inventoryRuns = runs.filter((run) => runStep(run) === "inventory");
  const inventoryProcessedCount = Math.max(inventoryRuns.length, inventoryCount);
  const inventoryErrorCount = inventoryRuns.filter((run) => run.status === "error" || run.status === "failed").length;
  const inventorySummaryFinished = crawlSummaryReported && inventoryCount > 0;
  const inferredInventorySkipCount = fetchedCount > inventoryCount && inventoryCount > 0 ? fetchedCount - inventoryCount : 0;
  const crawlFailed = runs.some((run) => runStep(run) === "crawl" && (run.status === "error" || run.status === "failed"));
  const profileFailed = runs.some((run) => runStep(run) === "profile" && (run.status === "error" || run.status === "failed"));
  const profileStarted = runs.some((run) => runStep(run) === "profile" && runPhase(run) === "started");
  const profileCompletedRun = runs.some((run) => runStep(run) === "profile" && runPhase(run) !== "started" && run.status !== "error");
  const crawlStarted = runs.some((run) => runStep(run) === "crawl" && runPhase(run) === "started");
  const noBackendProgressReported =
    runs.length === 0 && !hasProfile && sourcePageCount === 0 && evidencePageCount === 0 && evidenceCount === 0;
  const backendProgressStalled = noBackendProgressReported && pollingExhausted;
  const observedSourceCount = Math.max(sourcePageCount, fetchedCount);
  const sourceTarget = Math.max(1, Math.min(contextBuildTargetPages, observedSourceCount || contextBuildTargetPages));
  const evidenceTarget = Math.max(1, Math.min(contextBuildTargetPages, fetchedCount || sourcePageCount || contextBuildTargetPages));
  const sourceComplete = fetchedCount > 0 || sourcePageCount >= sourceTarget;
  const evidenceComplete =
    evidencePageCount >= evidenceTarget ||
    (evidencePageCount > 0 && inventoryProcessedCount >= evidenceTarget) ||
    (evidencePageCount > 0 && inventorySummaryFinished) ||
    (evidencePageCount > 0 && pollingExhausted);
  const active = !(hasProfile && sourceComplete && evidenceComplete);
  const exhausted = active && pollingExhausted;
  const profileStartStalled = exhausted && profileStarted && !hasProfile && !profileFailed && !profileCompletedRun;
  const crawlStartStalled = exhausted && crawlStarted && !crawlFailed && !crawlSummaryReported && observedSourceCount === 0;
  const startedProgressStalled = profileStartStalled || crawlStartStalled;
  const profileNeedsAttention = backendProgressStalled || profileStartStalled || (exhausted && profileFailed);
  const sourceNeedsAttention = backendProgressStalled || crawlStartStalled || (exhausted && crawlFailed);
  const evidenceNeedsAttention = backendProgressStalled || startedProgressStalled || (exhausted && evidenceCount === 0);
  const profileState: ContextBuildTrackState = hasProfile ? "done" : profileNeedsAttention ? "attention" : "running";
  const sourceState: ContextBuildTrackState = sourceComplete ? "done" : sourceNeedsAttention ? "attention" : "running";
  const evidenceState: ContextBuildTrackState =
    evidenceComplete
      ? "done"
      : backendProgressStalled || startedProgressStalled
        ? "attention"
        : sourcePageCount === 0 && fetchedCount === 0
          ? "waiting"
          : evidenceNeedsAttention
            ? "attention"
            : "running";
  const skippedPages = Math.max(crawlErrors + inventoryErrorCount, inferredInventorySkipCount);
  const profileDetail = hasProfile
    ? "Product facts are saved."
    : backendProgressStalled
      ? "No backend profile progress has reported yet. Check Admin and API worker logs."
      : profileStartStalled
        ? "Profile extraction started but has not finished. Check Admin > LLM settings if this remains."
      : profileState === "attention"
        ? "Profile extraction needs attention."
        : "Extracting product facts from the service URL.";
  const sourceDetail = sourceComplete
    ? skippedPages > 0
      ? `Crawl finished with ${skippedPages} skipped page${skippedPages === 1 ? "" : "s"}.`
      : "Public source pages are fetched."
    : sourceState === "attention"
      ? backendProgressStalled
        ? "No source-crawl progress has reported yet. Check the API worker."
        : crawlStartStalled
          ? "Source crawl started but has not reported fetched pages. Check the API worker if this remains."
          : "The crawl hit an error; CiteLoop will continue with available pages."
      : "Fetching up to 20 public pages; slow or failed URLs are skipped.";
  const evidenceDetail = evidenceComplete
    ? `Evidence is available from ${evidencePageCount} source page${evidencePageCount === 1 ? "" : "s"}.`
    : evidenceState === "waiting"
      ? "Starts as soon as source pages are available."
      : evidenceState === "attention"
        ? backendProgressStalled
          ? "Evidence cannot start until the crawl reports source pages."
          : startedProgressStalled
            ? "Evidence is waiting on the profile or source-crawl track to finish reporting."
            : "Evidence extraction has not produced usable snippets yet."
        : "Extracting source-backed snippets in parallel.";
  const tracks: ContextBuildTrack[] = [
    {
      id: "profile",
      label: "Product profile",
      state: profileState,
      progress: hasProfile ? 100 : 0,
      current: hasProfile ? 1 : 0,
      target: 1,
      detail: profileDetail,
    },
    {
      id: "source-crawl",
      label: "Source crawl",
      state: sourceState,
      progress: sourceComplete ? 100 : boundedPercent(sourcePageCount, sourceTarget),
      current: Math.min(sourceTarget, observedSourceCount),
      target: sourceTarget,
      detail: sourceDetail,
    },
    {
      id: "evidence",
      label: "Evidence snippets",
      state: evidenceState,
      progress: evidenceComplete ? 100 : boundedPercent(evidencePageCount, evidenceTarget),
      current: Math.min(evidenceTarget, evidencePageCount),
      target: evidenceTarget,
      detail: evidenceDetail,
    },
  ];

  if (!active) {
    return {
      active: false,
      exhausted: false,
      title: "Domain context is ready",
      detail: "CiteLoop has a product profile, source pages, and evidence for planning.",
      tracks,
    };
  }

  return {
    active: true,
    exhausted,
    title: "Building domain context",
    detail: backendProgressStalled
      ? "CiteLoop has not received a backend progress report yet. Check Admin > LLM settings and API worker logs."
      : startedProgressStalled
        ? "A backend track started but has not completed. Check Admin > LLM settings and API worker logs."
      : exhausted
        ? "Parallel context build is still checking results. CiteLoop skips slow or failed pages instead of waiting for every URL."
        : "Parallel context build is running. Each track updates from saved profile, crawl, and evidence results.",
    tracks,
  };
}

export function buildHomeEventStream({
  projectId,
  liveActivities = [],
  recentEvents = [],
  nextEvent,
  limit = 5,
}: HomeEventInput): HomeEventStreamResult {
  const liveItems = liveActivities.map((event): HomeEventStreamItem => ({
    ...event,
    kind: "live",
    timeLabel: "Now",
  }));
  const recentItems = recentEvents.map((event): HomeEventStreamItem => ({
    ...event,
    kind: "recent",
    timeLabel: "Recent",
  }));
  const nextItems: HomeEventStreamItem[] = nextEvent
    ? [
        {
          id: "next-event",
          kind: "next",
          title: nextEvent.title,
          detail: nextEvent.detail,
          href: nextEvent.href,
          timeLabel: "Next",
        },
      ]
    : [];
  const items = [...liveItems, ...recentItems, ...nextItems].slice(0, limit);

  if (items.length > 0) {
    return { items, emptyAction: null };
  }

  return {
    items: [],
    emptyAction: {
      title: "All set for now",
      detail: "No live work or scheduled publish slot is waiting. Growth signals will appear here as the loop starts moving.",
      href: `/projects/${projectId}/context`,
      actionLabel: "Open context",
    },
  };
}

export function visibleHomeSectionIds(sections: HomeSectionCandidate[], options: { limit?: number } = {}) {
  const limit = options.limit ?? 2;
  const active = sections
    .filter((section) => section.count > 0)
    .sort((a, b) => b.priority - a.priority || a.id.localeCompare(b.id));

  return {
    visibleIds: active.slice(0, limit).map((section) => section.id),
    overflowIds: active.slice(limit).map((section) => section.id),
  };
}

export function profilePayloadFromDraft(draft: ProfileDraft, baseProfile: Record<string, any> = {}) {
  const voice =
    baseProfile.voice && typeof baseProfile.voice === "object" && !Array.isArray(baseProfile.voice)
      ? baseProfile.voice
      : {};
  return {
    ...baseProfile,
    positioning: draft.positioning.trim(),
    icp: lines(draft.icp),
    value_props: lines(draft.value_props),
    features: lines(draft.features),
    differentiators: lines(draft.differentiators),
    competitors: lines(draft.competitors),
    key_terms: lines(draft.key_terms),
    tone: draft.tone.trim(),
    banned_claims: lines(draft.banned_claims),
    content_rules: lines(draft.content_rules),
    voice: {
      ...voice,
      tone: draft.tone.trim(),
      rules: lines(draft.content_rules),
    },
  };
}

export function profilePayloadFromAdvancedJSON(value: string) {
  return value.trim() ? JSON.parse(value) : {};
}

export function visibilityLifecycleLabel(status: string) {
  if (["accepted", "converted", "planned"].includes(status)) return "Added to Content Plan";
  if (status === "drafting") return "Draft in progress";
  if (["drafted", "ready_for_review", "in_review"].includes(status)) return "Draft waiting for review";
  if (status === "approved") return "Approved for publish";
  if (["published", "measuring"].includes(status)) return "Measuring impact";
  if (["completed", "done", "learned", "improved"].includes(status)) return "Loop closed";
  if (status === "stale") return "Needs re-check";
  if (status === "dismissed") return "Dismissed";
  if (["failed", "blocked"].includes(status)) return "Needs attention";
  return "Opportunity detected";
}

export function visibilityLifecycleTone(status: string): "green" | "amber" | "blue" | "neutral" | "red" {
  if (["completed", "done", "learned", "improved"].includes(status)) return "green";
  if (["accepted", "converted", "planned", "ready_for_review", "drafting", "drafted", "approved", "published", "measuring"].includes(status)) return "blue";
  if (["stale", "open"].includes(status)) return "amber";
  if (["dismissed", "archived"].includes(status)) return "neutral";
  if (["failed", "blocked"].includes(status)) return "red";
  return "amber";
}
