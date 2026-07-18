"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "next/navigation";
import { Archive, ArrowRight, CalendarDays, Check, History, Loader2, Pencil, Power, Undo2, Wand2, X } from "lucide-react";
import { defaultProjectConfig } from "../../../lib/api";
import type { GenerateTopicResult, PageUpdateDraft, PlatformCapability, PlatformTargetContext, ProjectConfig, Topic, VisibilityActionInLoop, VisibilitySummary } from "../../../lib/api";
import {
  contentPlanActionBusyCTA,
  contentPlanActionPrimaryCTA,
  contentPlanActionPublishControlsVisible,
  normalizePublishStrategy,
  hasActiveReviewHandoff,
  hasDraftStartedHandoff,
  isActiveContentPlanLoopAction,
  isBacklogStatus,
  isPageUpdateAction,
  pageUpdateDraftBusyCTA,
  pageUpdateDraftGitHubPRURL,
  pageUpdateDraftHasOpenGitHubPR,
  pageUpdateDraftIDForAction,
  pageUpdateDraftPrimaryCTA,
  pageUpdateDraftStatusTone,
  publishStrategyLabel,
  publishStrategyReasonForAction,
  recommendedPublishStrategyForAction,
  recommendedTopicIds,
  reviewArticleIDForAction,
  normalizedTopicPriority,
  topicWhy,
} from "../../../lib/content-plan-logic";
import type { ContentPlanPublishStrategy } from "../../../lib/content-plan-logic";
import {
  initialTargetSelection,
  platformLabel,
  summarizeTargetSelection,
  summarizeTargetSelectionPlatforms,
  togglePlatformTarget,
  validateTargetSelection,
} from "../../../lib/platform-contracts";
import type { ExactTargetSelection } from "../../../lib/platform-contracts";
import { workflowTraceLabelForAction, workflowTraceLabelForTopic } from "../../../lib/workflow-lineage";
import { useApi } from "../../../lib/use-api";
import { consumeHandoffSearchParams } from "../../../lib/handoff-query";
import { useToast } from "../../../components/toast-provider";
import { RightDrawer } from "../../../components/right-drawer";
import { Badge, Button, ButtonProgress, EmptyState, Field, SectionHeader, TextArea, TextInput, cx, formatDate } from "../../../components/ui";
import { ContentWorkflowStageHeaderAction } from "../content-workflow-stage-actions";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;
type TopicDraft = {
  channel: string;
  title: string;
  target_keyword: string;
  target_prompt: string;
  angle: string;
  format: string;
  priority: string;
};
type ContentPlanConfirmationKind = "return" | "dismiss" | "create";
type ContentPlanConfirmation = {
  kind: ContentPlanConfirmationKind;
  action: VisibilityActionInLoop;
} | null;

const PUBLISH_STRATEGIES: ContentPlanPublishStrategy[] = ["blog", "syndication", "both"];

const AUTO_WORKFLOW_HELP =
  "Auto On: accepted content briefs draft on cadence. " +
  "Auto Off: automatic drafting pauses; manual drafting stays available from reviewed briefs.";

const siteFixAssetTypes = new Set(["internal_link_patch", "schema_patch", "sitemap_update", "technical_fix"]);

function toDateTimeLocal(value: string | null) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

function fromDateTimeLocal(value: string) {
  const trimmed = value.trim();
  return trimmed ? new Date(trimmed).toISOString() : null;
}

function draftFromTopic(topic: Topic): TopicDraft {
  return {
    channel: topic.channel,
    title: topic.title,
    target_keyword: topic.target_keyword ?? "",
    target_prompt: topic.target_prompt ?? "",
    angle: topic.angle ?? "",
    format: topic.format ?? "",
    priority: String(topic.priority),
  };
}

function topicPriorityLabel(priority: number) {
  const normalized = normalizedTopicPriority(priority);
  if (normalized === 0) return "Needs priority";
  if (normalized <= 3) return "High priority";
  if (normalized <= 6) return "Medium priority";
  return "Low priority";
}

function topicPriorityTone(priority: number): "red" | "amber" | "neutral" {
  const normalized = normalizedTopicPriority(priority);
  if (normalized === 0 || normalized <= 6) return "amber";
  return "neutral";
}

function isContentPlanAction(action: VisibilityActionInLoop) {
  const outputType = String(action.output_snapshot?.output_type ?? action.diff_snapshot?.output_type ?? "").toLowerCase();
  const assetType = String(action.asset_type ?? "").toLowerCase();
  return outputType !== "direct_patch" && outputType !== "technical_task" && !siteFixAssetTypes.has(assetType);
}

function contentPlanActionTitle(action: VisibilityActionInLoop) {
  return action.topic_title || action.opportunity_recommended_action || action.opportunity_query || action.action_type || "Accepted content work";
}

function handoffTimestampLabel(prefix: string, value: string | null | undefined) {
  return value ? `${prefix} ${formatDate(value)}` : `${prefix} time unavailable`;
}

function contentPlanActionDetail(action: VisibilityActionInLoop) {
  return action.target_url || action.normalized_target_url || action.opportunity_page_url || action.opportunity_normalized_page_url || "Ready for content planning.";
}

function contentPlanActionTypeLabel(action: VisibilityActionInLoop) {
  if (action.asset_type === "page_update") return "Page update";
  if (action.asset_type === "blog_post") return "New content";
  if (action.asset_type === "metadata_rewrite") return "Page metadata";
  return "Content work";
}

function contentPlanActionWhyText(action: VisibilityActionInLoop) {
  const input = action.input_snapshot ?? {};
  const evidence = action.evidence_snapshot ?? {};
  const value =
    action.opportunity_expected_impact ??
    input.expected_impact ??
    input.recommended_action ??
    input.query ??
    evidence.recommended_action ??
    evidence.query ??
    action.opportunity_recommended_action ??
    action.opportunity_query;
  return value ? String(value) : "This accepted opportunity has enough evidence to turn into a content brief.";
}

function contentPlanActionContributionText(action: VisibilityActionInLoop) {
  const contribution = action.output_snapshot?.seo_geo_contribution;
  if (contribution) return String(contribution);
  const assetType = String(action.asset_type ?? "").toLowerCase();
  if (assetType === "metadata_rewrite") return "Improve query-page relevance and search appearance without publishing a new page.";
  if (assetType === "blog_post") return "Create an indexable asset that can answer the target query and earn AI citation coverage.";
  if (assetType === "page_update") return "Refresh an existing page so search engines and answer engines see stronger evidence on the canonical URL.";
  if (assetType.includes("geo")) return "Create answer-ready entity coverage for AI discovery surfaces.";
  return "Create or refresh an indexable asset that can earn rankings, citations, and downstream measurement.";
}

function contentPlanActionEvidenceText(action: VisibilityActionInLoop) {
  const evidence = action.evidence_snapshot ?? {};
  const value =
    evidence.summary ??
    evidence.reason ??
    evidence.source ??
    evidence.source_url ??
    evidence.page_url ??
    action.opportunity_page_url ??
    action.opportunity_normalized_page_url ??
    action.target_url ??
    action.normalized_target_url;
  return value ? String(value) : "Accepted from Opportunity Queue after review.";
}

function contentPlanRiskReasons(action: VisibilityActionInLoop) {
  return Array.isArray(action.risk_reasons) ? action.risk_reasons.map((item) => String(item)).filter(Boolean) : [];
}

function formatPageUpdateJSON(value: any) {
  if (!value || (typeof value === "object" && Object.keys(value).length === 0)) return "Not available yet.";
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function topicFromAcceptedAction(action: VisibilityActionInLoop, channel: ContentPlanPublishStrategy): Topic | null {
  if (!action.topic_id) return null;
  return {
    id: action.topic_id,
    project_id: undefined,
    channel,
    title: contentPlanActionTitle(action),
    target_keyword: action.opportunity_query ?? null,
    target_prompt: action.opportunity_expected_impact ?? null,
    angle: action.opportunity_expected_impact ?? null,
    format: "article",
    priority: 0,
    internal_links: [],
    status: action.topic_status ?? "backlog",
    scheduled_at: null,
    created_at: null,
    source_content_action_id: action.id,
  };
}

function reviewHrefForAction(projectId: string, articleID: string) {
  return `/projects/${projectId}/review?article=${articleID}`;
}

function targetPlatformsFromSnapshot(snapshot: any) {
  const rawTargets = Array.isArray(snapshot?.target_platforms) ? snapshot.target_platforms : [];
  return rawTargets
    .map((target: any) => (typeof target?.platform === "string" ? target.platform.trim() : ""))
    .filter(Boolean);
}

function contentActionTargetLabel(action: VisibilityActionInLoop) {
  const platforms = [
    ...targetPlatformsFromSnapshot(action.output_snapshot),
    ...targetPlatformsFromSnapshot(action.input_snapshot),
    ...targetPlatformsFromSnapshot(action.evidence_snapshot),
  ];
  const unique = Array.from(new Set(platforms));
  if (unique.length > 0) return unique.map(platformLabel).join(" + ");

  const strategy = publishStrategyForSnapshotLabel(action);
  if (strategy === "both") return "Blog + Syndication";
  return publishStrategyLabel(strategy);
}

function topicTargetLabel(topic: Topic) {
  const strategy = normalizePublishStrategy(topic.channel);
  if (strategy === "both") return "Blog + Syndication";
  if (strategy) return publishStrategyLabel(strategy);
  return platformLabel(topic.channel);
}

function publishStrategyDisplayLabel(strategy: ContentPlanPublishStrategy) {
  return strategy === "both" ? "Blog + Syndication" : publishStrategyLabel(strategy);
}

function publishStrategyForSnapshotLabel(action: VisibilityActionInLoop): ContentPlanPublishStrategy {
  const raw =
    normalizePublishStrategy(action.output_snapshot?.publish_strategy) ??
    normalizePublishStrategy(action.input_snapshot?.publish_strategy) ??
    normalizePublishStrategy(action.input_snapshot?.publish_to) ??
    normalizePublishStrategy(action.evidence_snapshot?.publish_strategy);
  return raw ?? "blog";
}

function platformBlockReason(capability: PlatformCapability, targetContext?: PlatformTargetContext | null) {
  const isContextPlatform = ["reddit", "hashnode"].includes(capability.platform);
  if (!capability.generation_supported) return "Not supported";
  if (capability.platform !== "blog" && !capability.connection_ready) return "Connection required";
  if (!capability.target_context_ready || (isContextPlatform && !targetContext)) return "Setup required";
  return null;
}

function recommendedPlatformSummary(strategy: ContentPlanPublishStrategy, capabilities: PlatformCapability[]) {
  if (strategy === "blog") return "Blog";
  const connectedExternal = capabilities
    .filter((capability) =>
      capability.platform !== "blog" &&
      capability.connection_ready &&
      capability.generation_supported &&
      capability.target_context_ready,
    )
    .map((capability) => platformLabel(capability.platform));
  const targets = strategy === "both" ? ["Blog", ...connectedExternal] : connectedExternal;
  return targets.length > 0 ? targets.join(" + ") : "No connected platform ready";
}

function contentPlanConfirmationCopy(confirmation: Exclude<ContentPlanConfirmation, null>) {
  const { kind, action } = confirmation;
  const actionLabel = isPageUpdateAction(action) ? "page update" : "content brief";
  switch (kind) {
    case "return":
      return {
        title: "Move this back to Opportunities?",
        body: "The Content Plan card will be removed, and this opportunity will return to the Opportunity Queue so you can choose a different next step.",
        busyLabel: "Moving back",
      };
    case "dismiss":
      return {
        title: "Dismiss this opportunity?",
        body: "CiteLoop will stop showing this opportunity unless the underlying signal materially changes.",
        busyLabel: "Dismissing",
      };
    case "create":
    default:
      return {
        title: `Create this ${actionLabel}?`,
        body: `CiteLoop will turn this accepted opportunity into a draftable ${actionLabel} using the selected settings.`,
        busyLabel: "Creating",
      };
  }
}

export function TopicsClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const searchParams = useSearchParams();
  const [topics, setTopics] = useState<Topic[]>([]);
  const [scheduleDrafts, setScheduleDrafts] = useState<Record<string, string>>({});
  const [publishStrategyDrafts, setPublishStrategyDrafts] = useState<Record<string, ContentPlanPublishStrategy>>({});
  const [platformCapabilities, setPlatformCapabilities] = useState<PlatformCapability[]>([]);
  const [platformTargetContexts, setPlatformTargetContexts] = useState<PlatformTargetContext[]>([]);
  const [targetSelections, setTargetSelections] = useState<Record<string, ExactTargetSelection>>({});
  const [recentDraftsDrawerOpen, setRecentDraftsDrawerOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [draft, setDraft] = useState<TopicDraft | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [generatingIds, setGeneratingIds] = useState<Record<string, boolean>>({});
  const { notify } = useToast();
  const setMessage = (next: Message) => {
    if (next) notify(next);
  };
  const [visibilitySummary, setVisibilitySummary] = useState<VisibilitySummary | null>(null);
  const [highlightContentPlanAction, setHighlightContentPlanAction] = useState<string | null>(null);
  const [selectedContentPlanActionID, setSelectedContentPlanActionID] = useState<string | null>(null);
  const [pendingContentPlanConfirmation, setPendingContentPlanConfirmation] = useState<ContentPlanConfirmation>(null);
  const [pageUpdateDrafts, setPageUpdateDrafts] = useState<Record<string, PageUpdateDraft>>({});
  const [config, setConfig] = useState<ProjectConfig | null>(null);
  const [inReview, setInReview] = useState(0);
  const [approvedCount, setApprovedCount] = useState(0);
  const [reviewArticleByTopic, setReviewArticleByTopic] = useState<Record<string, string>>({});
  const contentPlanActionRefs = useRef<Record<string, HTMLButtonElement | null>>({});
  const handledContentPlanActionHandoffRef = useRef<string | null>(null);
  const autoToggleBusy = busy === "auto-toggle";
  const autoEnabled = Boolean(config?.auto_advance_enabled);
  const requestedActionID = searchParams.get("action");

  const refresh = useCallback(async () => {
    try {
      const [next, summary, project, review, approvedArticles] = await Promise.all([
        api.listTopics(projectId),
        api.getVisibilitySummary(projectId).catch(() => null),
        api.getProject(projectId).catch(() => null),
        api.listReview(projectId).catch(() => []),
        api.listArticles(projectId, "approved").catch(() => []),
      ]);
      setTopics(next);
      setVisibilitySummary(summary);
      if (project) setConfig(project.config);
      setInReview(review.reduce((sum, group) => sum + group.articles.length, 0));
      setReviewArticleByTopic(
        Object.fromEntries(
          review.flatMap((group) => (group.articles[0] ? [[group.topic_id, group.articles[0].id] as const] : [])),
        ),
      );
      setApprovedCount(approvedArticles.length);
      setScheduleDrafts(Object.fromEntries(next.map((topic) => [topic.id, toDateTimeLocal(topic.scheduled_at)])));
      setGeneratingIds((current) => {
        const stillGenerating = new Set(next.filter((topic) => topic.status === "generating").map((topic) => topic.id));
        return Object.fromEntries(Object.entries(current).filter(([id]) => stillGenerating.has(id)));
      });
    } catch (e: any) {
      setMessage({ title: "Content briefs unavailable", detail: e.message, tone: "amber" });
    }
  }, [api, projectId]);

  const contentPlanActions = useMemo(
    () =>
      (visibilitySummary?.actions_in_loop ?? []).filter(
        (action) =>
          isContentPlanAction(action) &&
          isActiveContentPlanLoopAction(action),
      ),
    [visibilitySummary],
  );
  const sentToReviewActions = useMemo(
    () =>
      contentPlanActions
        .filter((action) => {
          const reviewArticleID = reviewArticleIDForAction(action, action.topic_id ? reviewArticleByTopic[action.topic_id] : null);
          return hasActiveReviewHandoff(action, reviewArticleID);
        })
        .slice()
        .sort((a, b) => String(b.updated_at ?? b.created_at ?? "").localeCompare(String(a.updated_at ?? a.created_at ?? ""))),
    [contentPlanActions, reviewArticleByTopic],
  );
  const acceptedPlanActions = useMemo(
    () =>
      // hasDraftStartedHandoff also covers hasAdvancedDraftHandoff(action), so
      // approved/scheduled/published drafts cannot leak back into Content Plan.
      contentPlanActions.filter(
        (action) => {
          const reviewArticleID = reviewArticleIDForAction(action, action.topic_id ? reviewArticleByTopic[action.topic_id] : null);
          return !hasDraftStartedHandoff(action, reviewArticleID);
        },
      ),
    [contentPlanActions, reviewArticleByTopic],
  );
  const acceptedPageUpdateActions = useMemo(
    () => acceptedPlanActions.filter((action) => isPageUpdateAction(action)),
    [acceptedPlanActions],
  );
  const acceptedContentBriefActions = useMemo(
    () => acceptedPlanActions.filter((action) => !isPageUpdateAction(action)),
    [acceptedPlanActions],
  );
  const selectedContentPlanAction = useMemo(
    () => acceptedPlanActions.find((action) => action.id === selectedContentPlanActionID) ?? null,
    [acceptedPlanActions, selectedContentPlanActionID],
  );
  const summaryPendingPlanActions =
    acceptedPlanActions.length;

  useEffect(() => {
    refresh();
  }, [refresh]);

  useEffect(() => {
    if (selectedContentPlanActionID && !selectedContentPlanAction) {
      setSelectedContentPlanActionID(null);
    }
  }, [selectedContentPlanAction, selectedContentPlanActionID]);

  useEffect(() => {
    if (!selectedContentPlanAction || isPageUpdateAction(selectedContentPlanAction)) return;
    let cancelled = false;
    const assetType = selectedContentPlanAction.asset_type || "blog_post";
    Promise.all([
      api.getPlatformCapabilities(projectId, assetType),
      api.listPlatformTargetContexts(projectId),
    ]).then(([capabilities, contexts]) => {
      if (cancelled) return;
      setPlatformCapabilities(capabilities);
      setPlatformTargetContexts(contexts);
      setTargetSelections((current) => {
        if (current[selectedContentPlanAction.id]) return current;
        try {
          return { ...current, [selectedContentPlanAction.id]: initialTargetSelection(assetType, capabilities) };
        } catch {
          return current;
        }
      });
    }).catch((error) => {
      if (!cancelled) setMessage({ title: "Platform contracts unavailable", detail: String(error?.message ?? error), tone: "amber" });
    });
    return () => { cancelled = true; };
  }, [api, projectId, selectedContentPlanAction]);

  useEffect(() => {
    if (!pendingContentPlanConfirmation) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy) {
        setPendingContentPlanConfirmation(null);
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [busy, pendingContentPlanConfirmation]);

  useEffect(() => {
    if (
      !requestedActionID ||
      acceptedPlanActions.length === 0 ||
      handledContentPlanActionHandoffRef.current === requestedActionID
    ) return;
    const target = contentPlanActionRefs.current[requestedActionID];
    if (!target) return;
    handledContentPlanActionHandoffRef.current = requestedActionID;
    const prefersReducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)")?.matches ?? false;
    target.scrollIntoView({ block: "center", behavior: prefersReducedMotion ? "auto" : "smooth" });
    target.focus({ preventScroll: true });
    setHighlightContentPlanAction(requestedActionID);
  }, [acceptedPlanActions, requestedActionID]);

  useEffect(() => {
    const actionsWithDrafts = acceptedPlanActions
      .filter((action) => isPageUpdateAction(action))
      .map((action) => ({ action, draftID: pageUpdateDraftIDForAction(action) }))
      .filter((item): item is { action: VisibilityActionInLoop; draftID: string } => Boolean(item.draftID));
    if (actionsWithDrafts.length === 0) return;
    let cancelled = false;
    async function loadPageUpdateDrafts() {
      const entries = await Promise.all(
        actionsWithDrafts.map(async ({ action, draftID }) => {
          try {
            const draft = await api.getPageUpdateDraft(projectId, draftID);
            return [action.id, draft] as const;
          } catch {
            return null;
          }
        }),
      );
      if (cancelled) return;
      setPageUpdateDrafts((current) => {
        const next = { ...current };
        for (const entry of entries) {
          if (entry) next[entry[0]] = entry[1];
        }
        return next;
      });
    }
    loadPageUpdateDrafts();
    return () => {
      cancelled = true;
    };
  }, [acceptedPlanActions, api, projectId]);

  useEffect(() => {
    const hasGenerating = Object.keys(generatingIds).length > 0 || topics.some((topic) => topic.status === "generating");
    const hasPendingPlanActions = autoEnabled && summaryPendingPlanActions > 0 && topics.length === 0;
    // A scheduled topic whose slot has arrived is drafted by the server's 5-minute
    // pass, transitioning scheduled → generating → drafted between client polls. Poll
    // so the plan reflects that without a manual reload (the topic then moves to the
    // review queue). Bound to ~30 min past the slot so a budget/disabled pause that
    // leaves a topic scheduled doesn't poll forever.
    const now = Date.now();
    const hasDueScheduled = autoEnabled && topics.some((topic) => {
      if (topic.status !== "scheduled" || !topic.scheduled_at) return false;
      const due = Date.parse(topic.scheduled_at);
      return Number.isFinite(due) && due <= now && now - due <= 30 * 60_000;
    });
    if (!hasGenerating && !hasPendingPlanActions && !hasDueScheduled) return;
    const interval = window.setInterval(refresh, hasGenerating ? 10_000 : hasPendingPlanActions ? 5_000 : 30_000);
    return () => window.clearInterval(interval);
  }, [autoEnabled, generatingIds, refresh, summaryPendingPlanActions, topics]);

  const backlogTopics = useMemo(() => topics.filter((topic) => isBacklogStatus(topic.status)), [topics]);
  const legacyBriefTopics = useMemo(
    () => backlogTopics.filter((topic) => !topic.source_content_action_id),
    [backlogTopics],
  );
  // Sent-to-Review handoff cards (PRD-CiteLoop-Workflow-Handoff-Link-Cards §6.2):
  // a drafted topic whose article is still in the review queue keeps a link card
  // here; once the article advances past Review it drops out of listReview and
  // the card exits with it (event-driven, §2.2).
  const sentToReviewTopics = useMemo(
    () =>
      topics
        .filter((topic) => topic.status === "drafted" && reviewArticleByTopic[topic.id])
        .filter((topic) => !sentToReviewActions.some((action) => action.topic_id === topic.id))
        .slice()
        .sort((a, b) => {
          const left = a.created_at ? new Date(a.created_at).getTime() : 0;
          const right = b.created_at ? new Date(b.created_at).getTime() : 0;
          return right - left;
        }),
    [topics, reviewArticleByTopic, sentToReviewActions],
  );
  const recommendedIds = useMemo(() => {
    return new Set(recommendedTopicIds(legacyBriefTopics));
  }, [legacyBriefTopics]);
  const topicGridClass = "grid gap-3 lg:grid-cols-2";
  const selectedActionDraftBusy = selectedContentPlanAction ? busy === `draft-action-${selectedContentPlanAction.id}` : false;
  const selectedActionDismissBusy = selectedContentPlanAction ? busy === `dismiss-action-${selectedContentPlanAction.id}` : false;
  const selectedActionReturnBusy = selectedContentPlanAction ? busy === `return-action-${selectedContentPlanAction.id}` : false;
  const selectedActionReviewArticleID = selectedContentPlanAction
    ? reviewArticleIDForAction(
        selectedContentPlanAction,
        selectedContentPlanAction.topic_id ? reviewArticleByTopic[selectedContentPlanAction.topic_id] : null,
      )
    : null;
  const selectedActionRiskReasons = selectedContentPlanAction ? contentPlanRiskReasons(selectedContentPlanAction) : [];
  const selectedActionTopic = selectedContentPlanAction?.topic_id
    ? topics.find((topic) => topic.id === selectedContentPlanAction.topic_id) ?? null
    : null;
  const selectedActionIsPageUpdate = isPageUpdateAction(selectedContentPlanAction);
  const selectedPageUpdateDraft = selectedContentPlanAction ? pageUpdateDrafts[selectedContentPlanAction.id] ?? null : null;
  const selectedActionPublishStrategy = selectedContentPlanAction ? publishStrategyForAction(selectedContentPlanAction) : "blog";
  const selectedActionRecommendedStrategy = selectedContentPlanAction ? recommendedPublishStrategy(selectedContentPlanAction) : "blog";
  const selectedActionShowsPublishControls = contentPlanActionPublishControlsVisible(selectedContentPlanAction);
  const selectedActionPrimaryCTA = selectedActionShowsPublishControls ? contentPlanActionPrimaryCTA(selectedContentPlanAction) : pageUpdateDraftPrimaryCTA(selectedPageUpdateDraft);
  const selectedPageUpdatePRURL = pageUpdateDraftGitHubPRURL(selectedPageUpdateDraft);
  const selectedActionPublishReason = selectedContentPlanAction
    ? publishStrategyReasonForAction(selectedContentPlanAction, selectedActionRecommendedStrategy)
    : "";
  const selectedTargetSelection = selectedContentPlanAction ? targetSelections[selectedContentPlanAction.id] ?? null : null;
  const selectedActionPublishLabel = selectedTargetSelection
    ? summarizeTargetSelectionPlatforms(selectedTargetSelection)
    : publishStrategyDisplayLabel(selectedActionPublishStrategy);
  const selectedActionRecommendedPublishLabel = recommendedPlatformSummary(selectedActionRecommendedStrategy, platformCapabilities);
  const selectedTargetValidation = selectedTargetSelection
    ? validateTargetSelection(selectedTargetSelection, platformCapabilities)
    : { valid: false, errors: ["Platform contracts are loading."] };
  const currentTargetContext = (platform: string) => platformTargetContexts.find((context) =>
    context.platform === platform && context.status === "confirmed" && Boolean(context.expires_at) && new Date(context.expires_at as string).getTime() > Date.now(),
  ) ?? null;
  const currentRedditContext = currentTargetContext("reddit");
  const currentHashnodeContext = currentTargetContext("hashnode");
  const selectedActionScheduleKey = selectedContentPlanAction ? selectedActionTopic?.id ?? selectedContentPlanAction.id : "";
  const selectedActionScheduleBusy = selectedContentPlanAction
    ? busy === `schedule-action-${selectedContentPlanAction.id}` || (selectedActionTopic ? busy === `schedule-${selectedActionTopic.id}` : false)
    : false;
  const selectedActionScheduleValue = selectedActionScheduleKey ? scheduleDrafts[selectedActionScheduleKey] ?? "" : "";
  const reviewingContentPlanAction = Boolean(busy) || autoEnabled || (selectedActionShowsPublishControls && !selectedTargetValidation.valid);
  const selectedPageUpdateBusy = selectedContentPlanAction ? busy === `page-update-${selectedContentPlanAction.id}` : false;
  const confirmationAction = pendingContentPlanConfirmation?.action ?? null;
  const confirmationCopy = pendingContentPlanConfirmation ? contentPlanConfirmationCopy(pendingContentPlanConfirmation) : null;
  const confirmationBusy = pendingContentPlanConfirmation
    ? pendingContentPlanConfirmation.kind === "return"
      ? busy === `return-action-${pendingContentPlanConfirmation.action.id}`
      : pendingContentPlanConfirmation.kind === "dismiss"
        ? busy === `dismiss-action-${pendingContentPlanConfirmation.action.id}`
        : busy === `draft-action-${pendingContentPlanConfirmation.action.id}` || busy === `page-update-${pendingContentPlanConfirmation.action.id}`
    : false;

  function topicForAction(action: VisibilityActionInLoop) {
    return action.topic_id ? topics.find((topic) => topic.id === action.topic_id) ?? null : null;
  }

  function recommendedPublishStrategy(action: VisibilityActionInLoop): ContentPlanPublishStrategy {
    return recommendedPublishStrategyForAction(action);
  }

  function publishStrategyForAction(action: VisibilityActionInLoop): ContentPlanPublishStrategy {
    const topicStrategy = normalizePublishStrategy(topicForAction(action)?.channel);
    return publishStrategyDrafts[action.id] ?? topicStrategy ?? recommendedPublishStrategy(action);
  }

  async function applyTopicPublishStrategy(topic: Topic, strategy: ContentPlanPublishStrategy) {
    if (topic.channel === strategy) return topic;
    const updated = await api.updateTopic(projectId, topic.id, { channel: strategy });
    replaceTopic(updated);
    return updated;
  }

  async function toggleAutoAdvance() {
    const nextEnabled = !autoEnabled;
    const base = config ?? defaultProjectConfig();
    setBusy("auto-toggle");
    setMessage(null);
    try {
      const updated = await api.updateConfig(projectId, { ...base, auto_advance_enabled: nextEnabled });
      setConfig(updated.config);
      setMessage({
        title: nextEnabled ? "Auto enabled" : "Automatic workflow paused",
        detail: nextEnabled
          ? "Accepted content briefs can draft on cadence."
          : "Accepted content briefs stay available for manual drafting.",
        tone: nextEnabled ? "green" : "amber",
      });
    } catch (e: any) {
      setMessage({ title: "Auto setting failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  function replaceTopic(updated: Topic) {
    setTopics((current) => (current.some((topic) => topic.id === updated.id) ? current.map((topic) => (topic.id === updated.id ? updated : topic)) : [updated, ...current]));
    setScheduleDrafts((current) => ({ ...current, [updated.id]: toDateTimeLocal(updated.scheduled_at) }));
  }

  function startEdit(topic: Topic) {
    setEditingId(topic.id);
    setDraft(draftFromTopic(topic));
    setMessage(null);
  }

  function cancelEdit() {
    setEditingId(null);
    setDraft(null);
  }

  async function saveEdit(topic: Topic) {
    if (!draft) return;
    const priority = Number.parseInt(draft.priority, 10);
    setBusy(`edit-${topic.id}`);
    setMessage(null);
    try {
      const updated = await api.updateTopic(projectId, topic.id, {
        channel: draft.channel,
        title: draft.title,
        target_keyword: draft.target_keyword,
        target_prompt: draft.target_prompt,
        angle: draft.angle,
        format: draft.format,
        priority: Number.isFinite(priority) ? priority : 0,
      });
      replaceTopic(updated);
      cancelEdit();
      setMessage({ title: "Content brief saved", detail: updated.title, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Save failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function schedule(topic: Topic) {
    const nextScheduledAt = fromDateTimeLocal(scheduleDrafts[topic.id] ?? "");
    if (nextScheduledAt === null && topic.scheduled_at) {
      const ok = window.confirm(
        `Clear the scheduled date for “${topic.title}”? It will no longer publish on a set date.`,
      );
      if (!ok) return;
    }
    setBusy(`schedule-${topic.id}`);
    setMessage(null);
    try {
      const updated = await api.scheduleTopic(projectId, topic.id, nextScheduledAt);
      replaceTopic(updated);
      setMessage({ title: updated.scheduled_at ? "Content brief scheduled" : "Schedule cleared", detail: updated.title, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Schedule failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function archive(topic: Topic) {
    const ok = window.confirm(`Remove “${topic.title}” from the content plan? You can restore it later from the archived filter.`);
    if (!ok) return;
    setBusy(`archive-${topic.id}`);
    setMessage(null);
    try {
      const updated = await api.archiveTopic(projectId, topic.id);
      replaceTopic(updated);
      if (editingId === topic.id) cancelEdit();
      setMessage({ title: "Content brief archived", detail: updated.title, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Archive failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function generate(topic: Topic): Promise<GenerateTopicResult | null> {
    setGeneratingIds((current) => ({ ...current, [topic.id]: true }));
    setMessage(null);
    let keepGenerating = false;
    try {
      const result = await api.generateTopic(projectId, topic.id);
      if (result.topic) {
        replaceTopic(result.topic);
      } else {
        await refresh();
      }
      if (result.status === "generating") {
        keepGenerating = true;
        setMessage({
          title: "Starting draft generation",
          detail: "Writer and QA are running in the background. Review queue will update when drafts are ready.",
          tone: "green",
        });
        return result;
      }
      await refresh();
      if (result.status === "advanced") {
        setMessage({
          title: "Draft already moved forward",
          detail: "This content brief already moved beyond Review. Open Publish or Results to continue with it.",
          tone: "amber",
        });
        return result;
      }
      const existing = result.articles?.length ?? 0;
      setMessage(
        existing > 0
          ? {
              title: "Draft already exists",
              detail: `This content brief already has ${existing} draft${existing === 1 ? "" : "s"} in the review queue. Open Review to approve or regenerate.`,
              tone: "amber",
            }
          : { title: "Content brief drafted", detail: "Draft is ready in the review queue.", tone: "green" },
      );
      return result;
    } catch (e: any) {
      setMessage({
        title: "Generate failed",
        detail: e.message,
        tone: "red",
      });
      return null;
    } finally {
      if (!keepGenerating) {
        setGeneratingIds((current) => {
          const next = { ...current };
          delete next[topic.id];
          return next;
        });
      }
    }
  }

  function markActionDraftStarted(actionID: string, topic: Topic | null | undefined, result?: GenerateTopicResult | null) {
    const nextTopic = result?.topic ?? topic ?? null;
    const now = new Date().toISOString();
    setVisibilitySummary((current) =>
      current
        ? {
            ...current,
            actions_in_loop: current.actions_in_loop.map((item) =>
              item.id === actionID
                ? {
                    ...item,
                    lifecycle_stage: "drafting",
                    topic_id: item.topic_id ?? nextTopic?.id ?? null,
                    topic_status: nextTopic?.status ?? item.topic_status ?? "generating",
                    updated_at: now,
                  }
                : item,
            ),
          }
        : current,
    );
  }

  async function ensureTopicForAction(action: VisibilityActionInLoop, publishStrategy = publishStrategyForAction(action)) {
    let topic =
      (action.topic_id ? topics.find((item) => item.id === action.topic_id) ?? topicFromAcceptedAction(action, publishStrategy) : null) ??
      null;
    if (!topic) {
      const exactSelection = targetSelections[action.id];
      if (exactSelection) {
        const validation = validateTargetSelection(exactSelection, platformCapabilities);
        if (!validation.valid) throw new Error(validation.errors.join(" "));
      }
      if (exactSelection) {
        topic = await api.planSEOContentAction(projectId, action.id, {
          publish_strategy: summarizeTargetSelection(exactSelection),
          asset_type: exactSelection.asset_type,
          canonical_target: exactSelection.canonical_target,
          target_platforms: exactSelection.target_platforms,
        });
      } else {
        topic = await api.planSEOContentAction(projectId, action.id, { publish_strategy: publishStrategy });
      }
      replaceTopic(topic);
      return topic;
    }
    return applyTopicPublishStrategy(topic, publishStrategy);
  }

  function removeAcceptedActionFromSummary(actionID: string) {
    setVisibilitySummary((current) =>
      current
        ? { ...current, actions_in_loop: current.actions_in_loop.filter((item) => item.id !== actionID) }
        : current,
    );
  }

  async function scheduleAcceptedAction(action: VisibilityActionInLoop) {
    const key = action.topic_id ?? action.id;
    const nextScheduledAt = fromDateTimeLocal(scheduleDrafts[key] ?? "");
    if (nextScheduledAt === null && !action.topic_id) {
      setMessage({ title: "Choose a draft date", detail: "Pick a date before scheduling this content brief.", tone: "amber" });
      return;
    }
    setBusy(`schedule-action-${action.id}`);
    setMessage(null);
    try {
      const topic = await ensureTopicForAction(action);
      if (nextScheduledAt === null && topic.scheduled_at) {
        const ok = window.confirm(
          `Clear the scheduled date for “${topic.title}”? It will no longer publish on a set date.`,
        );
        if (!ok) return;
      }
      const updated = await api.scheduleTopic(projectId, topic.id, nextScheduledAt);
      replaceTopic(updated);
      setScheduleDrafts((current) => ({
        ...current,
        [action.id]: toDateTimeLocal(updated.scheduled_at),
        [updated.id]: toDateTimeLocal(updated.scheduled_at),
      }));
      setMessage({ title: updated.scheduled_at ? "Content brief scheduled" : "Schedule cleared", detail: updated.title, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Schedule failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function draftAcceptedAction(action: VisibilityActionInLoop) {
    setBusy(`draft-action-${action.id}`);
    setMessage(null);
    try {
      if (isPageUpdateAction(action)) {
        const draft = await api.createPageUpdateDraft(projectId, action.id);
        const generated = await api.generatePageUpdateDraft(projectId, draft.id);
        setPageUpdateDrafts((current) => ({ ...current, [action.id]: generated }));
        await refresh();
        setMessage({ title: "Page update drafted", detail: contentPlanActionTitle(action), tone: "green" });
        return;
      }
      const topic = await ensureTopicForAction(action);
      const result = await generate(topic);
      if (result) {
        setSelectedContentPlanActionID(null);
        markActionDraftStarted(action.id, topic, result);
      }
    } catch (e: any) {
      setMessage({ title: `${contentPlanActionPrimaryCTA(action)} failed`, detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function refreshPageUpdateDraft(action: VisibilityActionInLoop) {
    const draftID = pageUpdateDraftIDForAction(action);
    if (!draftID) return null;
    const draft = await api.getPageUpdateDraft(projectId, draftID);
    setPageUpdateDrafts((current) => ({ ...current, [action.id]: draft }));
    return draft;
  }

  async function approvePageUpdateAction(action: VisibilityActionInLoop) {
    let draft = pageUpdateDrafts[action.id] ?? (await refreshPageUpdateDraft(action));
    if (!draft) {
      setMessage({ title: "Draft update first", detail: "Create a page update draft before approving it.", tone: "amber" });
      return;
    }
    setBusy(`page-update-${action.id}`);
    setMessage(null);
    try {
      draft = await api.approvePageUpdateDraft(projectId, draft.id);
      setPageUpdateDrafts((current) => ({ ...current, [action.id]: draft }));
      await refresh();
      setMessage({ title: "Page update approved", detail: contentPlanActionTitle(action), tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Approve Update failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function applyPageUpdateAction(action: VisibilityActionInLoop) {
    let draft = pageUpdateDrafts[action.id] ?? (await refreshPageUpdateDraft(action));
    if (!draft) {
      setMessage({ title: "Draft update first", detail: "Create a page update draft before applying it.", tone: "amber" });
      return;
    }
    setBusy(`page-update-${action.id}`);
    setMessage(null);
    try {
      draft = await api.applyPageUpdateDraft(projectId, draft.id);
      setPageUpdateDrafts((current) => ({ ...current, [action.id]: draft }));
      await refresh();
      const prURL = pageUpdateDraftGitHubPRURL(draft);
      setMessage({
        title: prURL ? "GitHub PR opened" : draft.status === "manual_apply_required" ? "Manual patch ready" : "Page update applied",
        detail: prURL ?? contentPlanActionDetail(action),
        tone: draft.status === "manual_apply_required" ? "amber" : "green",
      });
    } catch (e: any) {
      setMessage({ title: "Apply Update failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function verifyPageUpdateAction(action: VisibilityActionInLoop) {
    let draft = pageUpdateDrafts[action.id] ?? (await refreshPageUpdateDraft(action));
    if (!draft) {
      setMessage({ title: "Draft update first", detail: "Create a page update draft before verifying it.", tone: "amber" });
      return;
    }
    setBusy(`page-update-${action.id}`);
    setMessage(null);
    try {
      draft = await api.verifyPageUpdateDraft(projectId, draft.id, {
        status: "verified",
        verification_snapshot: { source: "content_plan_manual_verify", target_url: draft.target_url },
      });
      setPageUpdateDrafts((current) => ({ ...current, [action.id]: draft }));
      await refresh();
      setMessage({ title: "Page update verified", detail: contentPlanActionDetail(action), tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Verify Update failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function advancePageUpdateAction(action: VisibilityActionInLoop) {
    const draft = pageUpdateDrafts[action.id] ?? null;
    const prURL = pageUpdateDraftGitHubPRURL(draft);
    if (prURL && pageUpdateDraftHasOpenGitHubPR(draft)) {
      window.open(prURL, "_blank", "noopener,noreferrer");
      return;
    }
    switch (draft?.status) {
      case "ready_for_review":
        await approvePageUpdateAction(action);
        break;
      case "approved":
        await applyPageUpdateAction(action);
        break;
      case "applied":
      case "verification_pending":
      case "manual_apply_required":
      case "needs_follow_up":
      case "verification_failed":
        await verifyPageUpdateAction(action);
        break;
      case "verified":
        setMessage({ title: "Already verified", detail: contentPlanActionDetail(action), tone: "green" });
        break;
      default:
        await draftAcceptedAction(action);
        break;
    }
  }

  async function dismissAcceptedAction(action: VisibilityActionInLoop) {
    setBusy(`dismiss-action-${action.id}`);
    setMessage(null);
    try {
      await api.dismissSEOContentAction(projectId, action.id);
      removeAcceptedActionFromSummary(action.id);
      setSelectedContentPlanActionID(null);
      setMessage({ title: "Opportunity dismissed", detail: contentPlanActionTitle(action), tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Dismiss failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function returnAcceptedActionToOpportunity(action: VisibilityActionInLoop) {
    setBusy(`return-action-${action.id}`);
    setMessage(null);
    try {
      await api.returnSEOContentActionToOpportunity(projectId, action.id);
      removeAcceptedActionFromSummary(action.id);
      setSelectedContentPlanActionID(null);
      setMessage({ title: "Moved back to Opportunities", detail: contentPlanActionTitle(action), tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Move back failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function confirmPendingContentPlanAction() {
    if (!pendingContentPlanConfirmation) return;
    const { kind, action } = pendingContentPlanConfirmation;
    if (kind === "return") {
      await returnAcceptedActionToOpportunity(action);
    } else if (kind === "dismiss") {
      await dismissAcceptedAction(action);
    } else if (isPageUpdateAction(action)) {
      await advancePageUpdateAction(action);
    } else {
      await draftAcceptedAction(action);
    }
    setPendingContentPlanConfirmation(null);
  }

  const autoSwitch = (
    <div className="group relative inline-flex">
      <span id="content-plan-auto-help" className="sr-only">
        {AUTO_WORKFLOW_HELP}
      </span>
      <button
        type="button"
        role="switch"
        aria-checked={autoEnabled}
        aria-busy={autoToggleBusy}
        aria-describedby="content-plan-auto-help"
        disabled={Boolean(busy) || !config}
        onClick={toggleAutoAdvance}
        title={AUTO_WORKFLOW_HELP}
        className={cx(
          "inline-flex h-9 items-center gap-2 rounded-full border px-2.5 text-xs font-bold transition-all duration-150 active:scale-[0.97] disabled:cursor-not-allowed disabled:opacity-50",
          autoEnabled
            ? "border-emerald-200 bg-emerald-50 text-emerald-800"
            : "border-slate-200 bg-white text-slate-600 hover:bg-slate-50 hover:text-slate-950",
        )}
      >
        {autoToggleBusy ? <Loader2 aria-hidden="true" className="animate-spin" size={14} /> : <Power aria-hidden="true" size={14} />}
        <span>Auto</span>
        <span
          className={cx(
            "inline-flex h-5 min-w-9 items-center justify-center rounded-full px-2 font-mono text-[11px]",
            autoEnabled ? "bg-emerald-100 text-emerald-800" : "bg-slate-100 text-slate-500",
          )}
        >
          {autoEnabled ? "On" : "Off"}
        </span>
      </button>
      <span
        aria-hidden="true"
        className="pointer-events-none absolute right-0 top-11 z-30 w-72 max-w-[calc(100vw-2rem)] rounded-lg border border-slate-200 bg-slate-950 px-3 py-2 text-left text-xs font-semibold leading-5 text-white opacity-0 shadow-xl transition-opacity duration-150 group-hover:opacity-100 group-focus-within:opacity-100"
      >
        {AUTO_WORKFLOW_HELP}
      </span>
    </div>
  );

  return (
    <>
      <div className="space-y-7">
        <ContentWorkflowStageHeaderAction>
          {autoSwitch}
        </ContentWorkflowStageHeaderAction>

      <section data-content-plan-handoff-section className="space-y-3">
        <SectionHeader
          title="Content briefs"
          eyebrow="Accepted content work"
          action={
            <div className="flex flex-wrap items-center justify-end gap-2">
              <Badge tone="green">{acceptedPlanActions.length + legacyBriefTopics.length}</Badge>
              <Button
                data-content-plan-recent-drawer-trigger
                size="sm"
                variant="outline"
                onClick={() => {
                  setSelectedContentPlanActionID(null);
                  setRecentDraftsDrawerOpen(true);
                }}
                aria-label={`Open Recently Drafted (${sentToReviewActions.length + sentToReviewTopics.length})`}
              >
                <History size={14} />
                Recently Drafted
                <Badge tone={sentToReviewActions.length + sentToReviewTopics.length ? "green" : "neutral"}>
                  {sentToReviewActions.length + sentToReviewTopics.length}
                </Badge>
              </Button>
            </div>
          }
        />
        {acceptedPlanActions.length === 0 ? (
          <EmptyState
            title="No content briefs yet"
            detail="Accept AI-generated content opportunities from Opportunities to start planning."
          />
        ) : (
          <div data-content-plan-action-grid className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {acceptedPlanActions.map((action) => {
              const highlighted = highlightContentPlanAction === action.id;
              const actionIsPageUpdate = isPageUpdateAction(action);
              const actionReviewLabel = actionIsPageUpdate ? "Review update" : "Review brief";
              const publishStrategy = actionIsPageUpdate ? "blog" : publishStrategyForAction(action);
              const recommendedStrategy = actionIsPageUpdate ? "blog" : recommendedPublishStrategy(action);
              return (
                <button
                  key={action.id}
                  type="button"
                  id={`content-plan-action-${action.id}`}
                  ref={(node) => {
                    contentPlanActionRefs.current[action.id] = node;
                  }}
                  data-content-plan-action-card
                  onClick={() => {
                    consumeHandoffSearchParams("action");
                    setHighlightContentPlanAction(null);
                    setSelectedContentPlanActionID(action.id);
                  }}
                  aria-current={highlighted ? "true" : undefined}
                  aria-label={`Open accepted ${actionIsPageUpdate ? "page update" : "content brief"}: ${contentPlanActionTitle(action)}`}
                  className={cx(
                    "group flex h-full min-h-[220px] w-full flex-col rounded-lg border bg-white p-4 text-left shadow-sm transition hover:border-slate-300 hover:bg-slate-50/60 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820] active:translate-y-px",
                    highlighted ? "citeloop-handoff-card-selected" : "border-slate-200",
                  )}
                >
                  <div className="flex h-full min-w-0 flex-col justify-between gap-4">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge tone="neutral">{workflowTraceLabelForAction(action)}</Badge>
                        <Badge tone="green">Accepted</Badge>
                        <Badge tone="blue">{contentPlanActionTypeLabel(action)}</Badge>
                        {actionIsPageUpdate ? (
                          <Badge tone="green">Target URL locked</Badge>
                        ) : (
                          <Badge tone={publishStrategy === recommendedStrategy ? "green" : "neutral"}>
                            Publish to: {publishStrategyDisplayLabel(publishStrategy)}
                            {publishStrategy === recommendedStrategy ? " · Recommended" : ""}
                          </Badge>
                        )}
                        <Badge tone="neutral">{autoEnabled ? "Queued for drafting" : "Auto paused"}</Badge>
                        <Badge tone="neutral">{actionReviewLabel}</Badge>
                      </div>
                      <h3 className="mt-3 line-clamp-2 break-words text-lg font-bold leading-6 text-slate-950">{contentPlanActionTitle(action)}</h3>
                      <p className="mt-2 line-clamp-2 break-words text-sm leading-5 text-slate-500">{contentPlanActionDetail(action)}</p>
                    </div>
                    <div className="mt-auto flex items-center justify-between gap-3 border-t border-slate-100 pt-3 text-sm font-semibold text-slate-700">
                      <span>Open details</span>
                      <ArrowRight size={17} className="text-slate-400 transition group-hover:translate-x-0.5 group-hover:text-slate-600" />
                    </div>
                  </div>
                </button>
              );
            })}
          </div>
        )}
      </section>

        {legacyBriefTopics.length > 0 && (
          <section className="space-y-3">
            <SectionHeader title="Legacy content briefs" eyebrow="Earlier briefs without an Opportunity link" action={<Badge tone="neutral">{legacyBriefTopics.length}</Badge>} />
            <div className={topicGridClass}>
              {legacyBriefTopics.map((topic) => {
                const isGenerating = Boolean(generatingIds[topic.id]) || topic.status === "generating";
                const editBusy = busy === `edit-${topic.id}`;
                const scheduleBusy = busy === `schedule-${topic.id}`;
                const archiveBusy = busy === `archive-${topic.id}`;
                const topicLocked = topic.status === "archived" || isGenerating;
                const recommended = recommendedIds.has(topic.id);
                return (
                  <div
                    key={topic.id}
                    className={cx(
                      "flex flex-col rounded-xl border border-slate-200 bg-white px-4 py-3",
                      "min-h-[260px]",
                      editingId === topic.id && "lg:col-span-2",
                    )}
                  >
                <div data-content-plan-card-top className="flex flex-wrap items-center gap-2">
                  <Badge tone="blue">{topicTargetLabel(topic)}</Badge>
                  <Badge tone={topic.status === "archived" ? "amber" : topic.status === "backlog" ? "neutral" : "green"}>
                    {topic.status}
                  </Badge>
                  {recommended && <Badge tone="green">Recommended next</Badge>}
                  <Badge tone={topicPriorityTone(topic.priority)}>{topicPriorityLabel(topic.priority)}</Badge>
                </div>
                <div data-content-plan-card-body className="mt-3 min-w-0 flex-1">
                  <div className="break-words text-base font-bold text-slate-900">{topic.title}</div>
                  <div className="mt-1 text-sm text-slate-500">
                    {topic.target_keyword || topic.target_prompt || "No target keyword or prompt captured."}
                  </div>
                  <div className="mt-2 flex flex-wrap gap-3 text-xs font-semibold text-slate-400">
                    <span>{topic.format || "No format"}</span>
                    <span>{topic.angle || "No angle"}</span>
                    <span>{topic.internal_links.length} internal links</span>
                  </div>
                  <div className="mt-3 grid gap-2 border-t border-slate-100 pt-3 text-sm">
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">Why this exists</div>
                      <div className="mt-1 line-clamp-2 text-slate-600">{topicWhy(topic)}</div>
                    </div>
                  </div>
                </div>
                {editingId === topic.id && draft && (
                  <div className="mt-4 grid gap-3 border-t border-slate-100 pt-4">
                    <div className="grid gap-3 lg:grid-cols-[minmax(120px,160px)_minmax(0,1fr)_minmax(96px,120px)]">
                      <div className="min-w-0">
                        <Field label="Publish to">
                          <select
                            value={draft.channel}
                            onChange={(event) => setDraft({ ...draft, channel: event.target.value })}
                            className="h-10 w-full min-w-0 rounded-lg border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700"
                          >
                            <option value="blog">Blog</option>
                            <option value="syndication">Syndication</option>
                            <option value="both">Both</option>
                          </select>
                        </Field>
                      </div>
                      <div className="min-w-0">
                        <Field label="Title">
                          <TextInput
                            className="min-w-0"
                            value={draft.title}
                            onChange={(event) => setDraft({ ...draft, title: event.target.value })}
                          />
                        </Field>
                      </div>
                      <div className="min-w-0">
                        <Field label="Priority">
                          <TextInput
                            className="min-w-0"
                            type="number"
                            value={draft.priority}
                            onChange={(event) => setDraft({ ...draft, priority: event.target.value })}
                          />
                        </Field>
                      </div>
                    </div>
                    <div className="grid gap-3 md:grid-cols-2">
                      <div className="min-w-0">
                        <Field label="Target keyword">
                          <TextInput
                            className="min-w-0"
                            value={draft.target_keyword}
                            onChange={(event) => setDraft({ ...draft, target_keyword: event.target.value })}
                          />
                        </Field>
                      </div>
                      <div className="min-w-0">
                        <Field label="Format">
                          <TextInput
                            className="min-w-0"
                            value={draft.format}
                            onChange={(event) => setDraft({ ...draft, format: event.target.value })}
                          />
                        </Field>
                      </div>
                    </div>
                    <div className="grid gap-3 md:grid-cols-2">
                      <div className="min-w-0">
                        <Field label="Angle">
                          <TextArea
                            className="min-w-0"
                            rows={3}
                            value={draft.angle}
                            onChange={(event) => setDraft({ ...draft, angle: event.target.value })}
                          />
                        </Field>
                      </div>
                      <div className="min-w-0">
                        <Field label="Target prompt">
                          <TextArea
                            className="min-w-0"
                            rows={3}
                            value={draft.target_prompt}
                            onChange={(event) => setDraft({ ...draft, target_prompt: event.target.value })}
                          />
                        </Field>
                      </div>
                    </div>
                    <div className="flex flex-wrap justify-end gap-2">
                      <Button disabled={editBusy} size="sm" variant="ghost" onClick={cancelEdit}>
                        <X size={14} />
                        Cancel
                      </Button>
                      <Button disabled={editBusy || !draft.title.trim()} size="sm" variant="primary" onClick={() => saveEdit(topic)}>
                        <ButtonProgress busy={editBusy} busyLabel="Saving brief" idleIcon={<Check size={14} />}>
                          Save
                        </ButtonProgress>
                      </Button>
                    </div>
                  </div>
                )}
                <div
                  data-content-plan-card-footer
                  className="mt-4 flex flex-col gap-3 border-t border-slate-100 pt-3 lg:flex-row lg:items-end lg:justify-between"
                >
                  <div
                    data-content-plan-card-schedule
                    className="grid min-w-0 flex-1 gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end"
                  >
                    <div className="min-w-0">
                      <Field label="Scheduled at">
                        <TextInput
                          className="min-w-0"
                          type="datetime-local"
                          value={scheduleDrafts[topic.id] ?? ""}
                          disabled={topic.status === "archived"}
                          onChange={(event) => setScheduleDrafts((current) => ({ ...current, [topic.id]: event.target.value }))}
                        />
                      </Field>
                    </div>
                    <Button disabled={topicLocked || scheduleBusy} size="sm" onClick={() => schedule(topic)}>
                      <ButtonProgress busy={scheduleBusy} busyLabel="Scheduling" idleIcon={<CalendarDays size={14} />}>
                        Schedule
                      </ButtonProgress>
                    </Button>
                  </div>
                  <div data-content-plan-card-actions className="flex shrink-0 flex-wrap justify-end gap-2">
                    <Button
                      disabled={topicLocked || editBusy}
                      size="sm"
                      variant="ghost"
                      onClick={() => startEdit(topic)}
                    >
                      <Pencil size={14} />
                      Edit
                    </Button>
                    <Button aria-busy={isGenerating} disabled={topicLocked} size="sm" variant="outline" onClick={() => generate(topic)}>
                      <ButtonProgress busy={isGenerating} busyLabel="Drafting" idleIcon={<Wand2 size={14} />}>
                        Draft Content
                      </ButtonProgress>
                    </Button>
                    <Button disabled={topicLocked || archiveBusy} size="sm" variant="danger" onClick={() => archive(topic)}>
                      <ButtonProgress busy={archiveBusy} busyLabel="Archiving" idleIcon={<Archive size={14} />}>
                        Archive
                      </ButtonProgress>
                    </Button>
                  </div>
                </div>
                  </div>
                );
              })}
            </div>
          </section>
        )}

      </div>
      <RightDrawer
        open={recentDraftsDrawerOpen}
        dataAttribute="content-plan-recent-drawer"
        eyebrow="Content Plan"
        title="Recently Drafted"
        subtitle="Content briefs that moved from planning into Review."
        closeLabel="Close recently drafted"
        maxWidthClassName="max-w-5xl"
        onClose={() => setRecentDraftsDrawerOpen(false)}
      >
        <section data-content-plan-recently-sent>
          {sentToReviewActions.length === 0 && sentToReviewTopics.length === 0 ? (
            <EmptyState title="No recently drafted briefs" detail="Drafted briefs will appear here after they move into Review." />
          ) : (
            <div data-content-plan-sent-grid className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
              {sentToReviewActions.map((action) => {
                const reviewArticleID = reviewArticleIDForAction(
                  action,
                  action.topic_id ? reviewArticleByTopic[action.topic_id] : null,
                );
                const reviewHref = reviewArticleID ? reviewHrefForAction(projectId, reviewArticleID) : `/projects/${projectId}/review`;
                return (
                  <a
                    key={action.id}
                    data-content-plan-sent-card
                    href={reviewHref}
                    aria-label={`Open "${contentPlanActionTitle(action)}" in Review`}
                    className="group flex h-full min-h-[220px] flex-col rounded-lg border border-slate-200 bg-white p-4 text-left shadow-sm transition hover:border-slate-300 hover:bg-slate-50/60 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820] active:translate-y-px"
                  >
                    <div className="flex h-full min-w-0 flex-col justify-between gap-4">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <Badge tone="neutral">{workflowTraceLabelForAction(action)}</Badge>
                          <Badge tone="green">Sent to Review</Badge>
                          <Badge tone="blue">{contentActionTargetLabel(action)}</Badge>
                          <Badge tone="neutral">{handoffTimestampLabel("Drafted", action.updated_at ?? action.created_at)}</Badge>
                        </div>
                        <h3 className="mt-3 line-clamp-2 text-base font-bold leading-6 text-slate-950">{contentPlanActionTitle(action)}</h3>
                        <p className="mt-2 line-clamp-2 text-sm leading-5 text-slate-500">
                          {action.opportunity_query || action.opportunity_expected_impact || "Draft is waiting for review."}
                        </p>
                      </div>
                      <div className="mt-auto flex items-center justify-between gap-3 border-t border-slate-100 pt-3 text-sm font-semibold text-slate-700">
                        <span>{reviewArticleID ? "View in Review" : "Open Review"}</span>
                        <ArrowRight size={17} className="text-slate-400 transition group-hover:translate-x-0.5 group-hover:text-slate-600" />
                      </div>
                    </div>
                  </a>
                );
              })}
              {sentToReviewTopics.map((topic) => (
                <a
                  key={topic.id}
                  data-content-plan-sent-card
                  href={`/projects/${projectId}/review?article=${reviewArticleByTopic[topic.id]}`}
                  aria-label={`Open "${topic.title}" in Review`}
                  className="group flex h-full min-h-[220px] w-full flex-col rounded-lg border border-slate-200 bg-white p-4 text-left shadow-sm transition hover:border-slate-300 hover:bg-slate-50/60 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820] active:translate-y-px"
                >
                  <div className="flex h-full min-w-0 flex-col justify-between gap-4">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge tone="neutral">{workflowTraceLabelForTopic(topic)}</Badge>
                        <Badge tone="green">Sent to Review</Badge>
                        <Badge tone="blue">{topicTargetLabel(topic)}</Badge>
                        <Badge tone="neutral">{handoffTimestampLabel("Drafted", topic.created_at)}</Badge>
                      </div>
                      <h3 className="mt-3 line-clamp-2 text-base font-bold leading-6 text-slate-950">{topic.title}</h3>
                      <p className="mt-2 line-clamp-2 text-sm leading-5 text-slate-500">
                        {topic.target_keyword || topic.target_prompt || "Draft is waiting for review."}
                      </p>
                    </div>
                    <div className="mt-auto flex items-center justify-between gap-3 border-t border-slate-100 pt-3 text-sm font-semibold text-slate-700">
                      <span>View in Review</span>
                      <ArrowRight size={17} className="text-slate-400 transition group-hover:translate-x-0.5 group-hover:text-slate-600" />
                    </div>
                  </div>
                </a>
              ))}
            </div>
          )}
        </section>
      </RightDrawer>
      {selectedContentPlanAction && (
      <RightDrawer
        open={Boolean(selectedContentPlanAction)}
        dataAttribute="content-plan-action-drawer"
        eyebrow="Accepted opportunity brief"
        title={contentPlanActionTitle(selectedContentPlanAction)}
        subtitle={contentPlanActionDetail(selectedContentPlanAction)}
        closeLabel="Close content plan action details"
        footerLabel="Content plan drawer actions"
        onClose={() => setSelectedContentPlanActionID(null)}
        badges={
          <>
            <Badge tone="green">Accepted</Badge>
            <Badge tone="blue">{contentPlanActionTypeLabel(selectedContentPlanAction)}</Badge>
            {selectedActionShowsPublishControls ? (
              <Badge tone={selectedActionPublishStrategy === selectedActionRecommendedStrategy ? "green" : "neutral"}>
                Publish to: {selectedActionPublishLabel}
              </Badge>
            ) : (
              <Badge tone="green">Target URL locked</Badge>
            )}
            <Badge tone="neutral">{autoEnabled ? "Auto drafting" : "Manual review"}</Badge>
          </>
        }
        footer={
          <>
            <Button
              size="sm"
              variant="outline"
              onClick={() => setPendingContentPlanConfirmation({ kind: "return", action: selectedContentPlanAction })}
              disabled={Boolean(busy)}
            >
              <ButtonProgress busy={selectedActionReturnBusy} busyLabel="Moving back" idleIcon={<Undo2 size={14} />}>
                Move back to Opportunities
              </ButtonProgress>
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setPendingContentPlanConfirmation({ kind: "dismiss", action: selectedContentPlanAction })}
              disabled={Boolean(busy)}
            >
              <ButtonProgress busy={selectedActionDismissBusy} busyLabel="Dismissing" idleIcon={<X size={14} />}>
                Dismiss
              </ButtonProgress>
            </Button>
            {selectedActionIsPageUpdate ? (
              <Button
                aria-busy={selectedActionDraftBusy || selectedPageUpdateBusy}
                disabled={Boolean(busy) || selectedPageUpdateDraft?.status === "verified"}
                variant="primary"
                size="sm"
                onClick={() => setPendingContentPlanConfirmation({ kind: "create", action: selectedContentPlanAction })}
                title="Review and advance the existing-page update without creating a new post."
              >
                <ButtonProgress
                  busy={selectedActionDraftBusy || selectedPageUpdateBusy}
                  busyLabel={pageUpdateDraftBusyCTA(selectedPageUpdateDraft)}
                  idleIcon={<Wand2 size={14} />}
                >
                  {selectedActionPrimaryCTA}
                </ButtonProgress>
              </Button>
            ) : selectedActionReviewArticleID ? (
              <a
                href={reviewHrefForAction(projectId, selectedActionReviewArticleID)}
                className="inline-flex h-8 items-center justify-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 transition-colors hover:bg-slate-50"
              >
                View in Review
                <ArrowRight size={14} />
              </a>
            ) : (
              <Button
                aria-busy={selectedActionDraftBusy}
                disabled={reviewingContentPlanAction}
                variant="primary"
                size="sm"
                onClick={() => setPendingContentPlanConfirmation({ kind: "create", action: selectedContentPlanAction })}
                title={
                  autoEnabled
                    ? "Auto is on, so AI Editor will draft this work automatically."
                    : selectedActionShowsPublishControls
                      ? "Send this brief to AI Editor and QA Review."
                      : "Draft a source-backed patch for this existing page."
                }
              >
                <ButtonProgress busy={selectedActionDraftBusy} busyLabel={contentPlanActionBusyCTA(selectedContentPlanAction)} idleIcon={<Wand2 size={14} />}>
                  {contentPlanActionPrimaryCTA(selectedContentPlanAction)}
                </ButtonProgress>
              </Button>
            )}
          </>
        }
      >
        <div className="space-y-5">
          {selectedActionShowsPublishControls ? (
            <>
              <section className="rounded-xl border border-slate-200 bg-white p-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Publish to</div>
                    <p className="mt-1 text-sm leading-5 text-slate-600">
                      Recommended: {selectedActionRecommendedPublishLabel}
                    </p>
                  </div>
                  <Badge tone={selectedActionPublishStrategy === selectedActionRecommendedStrategy ? "green" : "neutral"}>
                    {selectedActionPublishStrategy === selectedActionRecommendedStrategy ? "Recommended" : "Overridden"}
                  </Badge>
                </div>
                <div className="mt-3 grid gap-2 sm:grid-cols-2" role="group" aria-label="Choose exact target platforms">
                  {platformCapabilities.map((capability) => {
                    const targetContext = ["reddit", "hashnode"].includes(capability.platform) ? currentTargetContext(capability.platform) : null;
                    const selected = Boolean(selectedTargetSelection?.target_platforms.some((target) =>
                      target.platform === capability.platform && (!targetContext || target.target_context_id === targetContext.id),
                    ));
                    const canonical = capability.platform === "blog";
                    const blockReason = platformBlockReason(capability, targetContext);
                    const blocked = Boolean(blockReason);
                    return (
                      <button
                        key={capability.platform}
                        type="button"
                        disabled={Boolean(busy) || canonical || blocked || !selectedTargetSelection}
                        aria-pressed={selected}
                        onClick={() => {
                          if (!selectedTargetSelection) return;
                          const next = togglePlatformTarget(selectedTargetSelection, capability, !selected, targetContext);
                          setTargetSelections((current) => ({ ...current, [selectedContentPlanAction.id]: next }));
                          setPublishStrategyDrafts((current) => ({ ...current, [selectedContentPlanAction.id]: summarizeTargetSelection(next) }));
                        }}
                        className={cx(
                          "min-h-20 rounded-lg border px-3 py-2 text-left text-sm font-bold transition disabled:cursor-not-allowed disabled:opacity-60",
                          selected
                            ? "border-[#d93820] bg-[#fff5f2] text-[#b8321d] shadow-sm"
                            : "border-slate-200 bg-white text-slate-700 hover:border-slate-300 hover:bg-slate-50",
                        )}
                      >
                        <span className="flex items-center justify-between gap-2">
                          <span>{platformLabel(capability.platform)}</span>
                          <span className="text-[10px] uppercase tracking-wide text-slate-400">{capability.publish_mode.replace("_", " ")}</span>
                        </span>
                        <span className="mt-1 block text-xs font-medium text-slate-500">
                          {canonical ? "Canonical · always included" : capability.output_type.replaceAll("_", " ")}
                        </span>
                        {blocked && <span className="mt-1 block text-xs font-semibold text-amber-700">{blockReason}</span>}
                      </button>
                    );
                  })}
                </div>
                <p className="mt-3 text-sm leading-6 text-slate-600">
                  Each selected platform gets its own native artifact and pinned contract. Selected platforms:
                  {" "}<strong>{selectedTargetSelection ? summarizeTargetSelectionPlatforms(selectedTargetSelection) : "Loading"}</strong>.
                </p>
                {!currentRedditContext && platformCapabilities.some((item) => item.platform === "reddit") && (
                  <a className="mt-2 inline-block text-xs font-semibold text-[#d93820] hover:underline" href={`/projects/${projectId}/settings#reddit-rules`}>
                    Confirm subreddit rules in Settings before selecting Reddit
                  </a>
                )}
                {!currentHashnodeContext && platformCapabilities.some((item) => item.platform === "hashnode") && (
                  <a className="mt-2 ml-3 inline-block text-xs font-semibold text-[#d93820] hover:underline" href={`/projects/${projectId}/settings#hashnode-publication`}>
                    Confirm a Hashnode publication in Settings before selecting Hashnode
                  </a>
                )}
                {!selectedTargetValidation.valid && selectedTargetValidation.errors.map((error) => (
                  <p key={error} className="mt-1 text-xs font-semibold text-amber-700">{error}</p>
                ))}
              </section>

              <section className="rounded-xl border border-slate-200 bg-white p-4">
                <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Schedule</div>
                <div className="mt-3 grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
                  <Field label="Draft at">
                    <TextInput
                      className="min-w-0"
                      type="datetime-local"
                      value={selectedActionScheduleValue}
                      disabled={selectedActionTopic?.status === "archived"}
                      onChange={(event) =>
                        setScheduleDrafts((current) => ({ ...current, [selectedActionScheduleKey]: event.target.value }))
                      }
                    />
                  </Field>
                  <Button
                    disabled={
                      selectedActionTopic?.status === "archived" ||
                      selectedActionScheduleBusy ||
                      Boolean(busy && !selectedActionScheduleBusy)
                    }
                    size="sm"
                    onClick={() => scheduleAcceptedAction(selectedContentPlanAction)}
                  >
                    <ButtonProgress busy={selectedActionScheduleBusy} busyLabel="Scheduling" idleIcon={<CalendarDays size={14} />}>
                      Schedule
                    </ButtonProgress>
                  </Button>
                </div>
              </section>
            </>
          ) : (
            <>
              <section className="rounded-xl border border-slate-200 bg-white p-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Update target</div>
                    <p className="mt-1 break-words text-sm leading-5 text-slate-600">{contentPlanActionDetail(selectedContentPlanAction)}</p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Badge tone="green">Target URL locked</Badge>
                    {selectedPageUpdateDraft && (
                      <Badge tone={pageUpdateDraftStatusTone(selectedPageUpdateDraft.status)}>{selectedPageUpdateDraft.status}</Badge>
                    )}
                  </div>
                </div>
                <p className="mt-3 text-sm leading-6 text-slate-600">
                  This work updates the existing page in place. CiteLoop drafts a reviewable patch and keeps the original URL as the verification target.
                </p>
              </section>

              {selectedPageUpdateDraft ? (
                <>
                  <section className="rounded-xl border border-slate-200 bg-white p-4">
                    <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                      <div>
                        <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Review Diff</div>
                        <div className="mt-1 text-sm font-semibold text-slate-950">
                          {selectedPageUpdateDraft.diff_snapshot?.change_summary || contentPlanActionTitle(selectedContentPlanAction)}
                        </div>
                      </div>
                      <Badge tone={pageUpdateDraftStatusTone(selectedPageUpdateDraft.status)}>{pageUpdateDraftPrimaryCTA(selectedPageUpdateDraft)}</Badge>
                    </div>
                    <div className="mt-4 grid gap-3">
                      <div>
                        <div className="text-xs font-semibold uppercase text-slate-400">Proposed update</div>
                        <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap rounded-lg bg-slate-950 p-3 text-xs leading-5 text-white">
                          {selectedPageUpdateDraft.proposed_content_md || "Draft this update to review the proposed patch."}
                        </pre>
                      </div>
                      <div>
                        <div className="text-xs font-semibold uppercase text-slate-400">Structured diff</div>
                        <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap rounded-lg bg-slate-50 p-3 text-xs leading-5 text-slate-700">
                          {formatPageUpdateJSON(selectedPageUpdateDraft.diff_snapshot)}
                        </pre>
                      </div>
                    </div>
                  </section>

                  <section className="rounded-xl border border-slate-200 bg-white p-4">
                    <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">QA and resolution criteria</div>
                    <div className="mt-3 grid gap-3">
                      <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded-lg bg-slate-50 p-3 text-xs leading-5 text-slate-700">
                        {formatPageUpdateJSON(selectedPageUpdateDraft.qa_feedback)}
                      </pre>
                      <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded-lg bg-slate-50 p-3 text-xs leading-5 text-slate-700">
                        {formatPageUpdateJSON(selectedPageUpdateDraft.resolution_criteria)}
                      </pre>
                    </div>
                  </section>

                  {selectedPageUpdatePRURL && (
                    <section className="rounded-xl border border-sky-200 bg-sky-50 p-4">
                      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                        <div>
                          <div className="text-xs font-semibold uppercase tracking-[0.12em] text-sky-700">GitHub PR</div>
                          <p className="mt-2 text-sm leading-6 text-sky-950">
                            The update is proposed as a repository diff. Merge and deploy the PR, then CiteLoop can verify the existing target URL.
                          </p>
                          <div className="mt-3 grid gap-1 text-xs leading-5 text-sky-900">
                            {selectedPageUpdateDraft.publisher_result?.repo && <div>Repo: {selectedPageUpdateDraft.publisher_result.repo}</div>}
                            {selectedPageUpdateDraft.publisher_result?.source_file_path && <div>Source: {selectedPageUpdateDraft.publisher_result.source_file_path}</div>}
                            {selectedPageUpdateDraft.publisher_result?.working_branch && <div>Branch: {selectedPageUpdateDraft.publisher_result.working_branch}</div>}
                          </div>
                        </div>
                        <a
                          href={selectedPageUpdatePRURL}
                          target="_blank"
                          rel="noreferrer"
                          className="inline-flex h-8 shrink-0 items-center justify-center gap-2 rounded-lg border border-sky-200 bg-white px-3 text-xs font-medium text-sky-800 transition-colors hover:bg-sky-100"
                        >
                          Open PR
                          <ArrowRight size={14} />
                        </a>
                      </div>
                    </section>
                  )}

                  {(selectedPageUpdateDraft.status === "manual_apply_required" || selectedPageUpdateDraft.status === "verification_failed" || selectedPageUpdateDraft.status === "needs_follow_up") && (
                    <section className="rounded-xl border border-amber-200 bg-amber-50 p-4">
                      <div className="text-xs font-semibold uppercase tracking-[0.12em] text-amber-700">Manual apply / verification</div>
                      <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap rounded-lg bg-white/80 p-3 text-xs leading-5 text-amber-900">
                        {formatPageUpdateJSON(selectedPageUpdateDraft.publisher_result || selectedPageUpdateDraft.verification_snapshot)}
                      </pre>
                    </section>
                  )}
                </>
              ) : (
                <section className="rounded-xl border border-dashed border-slate-200 bg-slate-50 p-4">
                  <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Review Diff</div>
                  <p className="mt-2 text-sm leading-6 text-slate-600">
                    Draft Update will create a patch, QA feedback, and resolution criteria for this exact target URL.
                  </p>
                </section>
              )}
            </>
          )}

          <section className="rounded-xl border border-slate-200 bg-white p-4">
            <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Action timeline</div>
            <div className="mt-3 grid gap-3 text-sm sm:grid-cols-2">
              <div>
                <div className="text-xs font-semibold uppercase text-slate-400">Created</div>
                <div className="mt-1 font-medium text-slate-700">{formatDate(selectedContentPlanAction.created_at ?? null)}</div>
              </div>
              <div>
                <div className="text-xs font-semibold uppercase text-slate-400">Approved</div>
                <div className="mt-1 font-medium text-slate-700">{formatDate(selectedContentPlanAction.approved_at ?? null)}</div>
              </div>
              <div>
                <div className="text-xs font-semibold uppercase text-slate-400">Drafted</div>
                <div className="mt-1 font-medium text-slate-700">{formatDate(selectedActionTopic?.created_at ?? selectedContentPlanAction.updated_at ?? null)}</div>
              </div>
              <div>
                <div className="text-xs font-semibold uppercase text-slate-400">Last updated</div>
                <div className="mt-1 font-medium text-slate-700">{formatDate(selectedContentPlanAction.updated_at ?? null)}</div>
              </div>
            </div>
          </section>

          <section className="rounded-xl border border-slate-200 bg-slate-50 p-4">
            <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Why write this</div>
            <p className="mt-2 text-sm leading-6 text-slate-700">{contentPlanActionWhyText(selectedContentPlanAction)}</p>
          </section>

          <section className="rounded-xl border border-slate-200 p-4">
            <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">SEO/GEO contribution</div>
            <div className="mt-2 text-sm font-semibold leading-5 text-slate-950">AI Visibility / GEO Impact</div>
            <p className="mt-1 text-sm leading-6 text-slate-700">{contentPlanActionContributionText(selectedContentPlanAction)}</p>
          </section>

          <section className="grid gap-3 text-sm sm:grid-cols-2">
            <div>
              <div className="text-xs font-semibold uppercase text-slate-400">Title</div>
              <div className="mt-1 break-words font-medium text-slate-700">{contentPlanActionTitle(selectedContentPlanAction)}</div>
            </div>
            <div>
              <div className="text-xs font-semibold uppercase text-slate-400">Work type</div>
              <div className="mt-1 font-medium text-slate-700">{contentPlanActionTypeLabel(selectedContentPlanAction)}</div>
            </div>
            <div>
              <div className="text-xs font-semibold uppercase text-slate-400">Target query</div>
              <div className="mt-1 break-words font-medium text-slate-700">{selectedContentPlanAction.opportunity_query ?? "Not query-specific"}</div>
            </div>
            <div>
              <div className="text-xs font-semibold uppercase text-slate-400">Brief status</div>
              <div className="mt-1 font-medium text-slate-700">{selectedContentPlanAction.topic_status ?? selectedContentPlanAction.lifecycle_stage}</div>
            </div>
            <div className="sm:col-span-2">
              <div className="text-xs font-semibold uppercase text-slate-400">Target URL</div>
              <div className="mt-1 break-words font-medium text-slate-700">{contentPlanActionDetail(selectedContentPlanAction)}</div>
            </div>
          </section>

          <section className="rounded-xl border border-slate-200 p-4">
            <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Evidence source</div>
            <p className="mt-2 text-sm leading-6 text-slate-700">{contentPlanActionEvidenceText(selectedContentPlanAction)}</p>
            {selectedActionRiskReasons.length > 0 && (
              <div className="mt-4 border-t border-slate-100 pt-3">
                <div className="text-xs font-semibold uppercase text-slate-400">Review notes</div>
                <ul className="mt-2 list-disc space-y-1 pl-5 text-sm leading-6 text-slate-600">
                  {selectedActionRiskReasons.slice(0, 4).map((reason) => (
                    <li key={reason}>{reason}</li>
                  ))}
                </ul>
              </div>
            )}
          </section>
        </div>
      </RightDrawer>
    )}
    {pendingContentPlanConfirmation && confirmationCopy && confirmationAction && (
      <div className="fixed inset-0 z-[60] flex items-center justify-center bg-slate-950/35 px-4 py-6">
        <button
          type="button"
          aria-label="Cancel"
          className="absolute inset-0"
          disabled={confirmationBusy}
          onClick={() => setPendingContentPlanConfirmation(null)}
        />
        <div
          role="dialog"
          aria-modal="true"
          aria-labelledby="content-plan-confirmation-title"
          className="relative w-full max-w-md rounded-xl border border-slate-200 bg-white p-5 shadow-2xl"
        >
          <div className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">Confirm workflow change</div>
          <h3 id="content-plan-confirmation-title" className="mt-2 text-lg font-bold leading-7 text-slate-950">
            {confirmationCopy.title}
          </h3>
          <p className="mt-3 text-sm leading-6 text-slate-600">{confirmationCopy.body}</p>
          <div className="mt-4 rounded-lg bg-slate-50 px-3 py-2 text-sm font-medium text-slate-700">
            {contentPlanActionTitle(confirmationAction)}
          </div>
          <div className="mt-5 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={confirmationBusy}
              onClick={() => setPendingContentPlanConfirmation(null)}
            >
              Cancel
            </Button>
            <Button
              type="button"
              size="sm"
              variant={pendingContentPlanConfirmation.kind === "dismiss" ? "danger" : "primary"}
              aria-busy={confirmationBusy}
              disabled={Boolean(busy)}
              onClick={confirmPendingContentPlanAction}
            >
              <ButtonProgress busy={confirmationBusy} busyLabel={confirmationCopy.busyLabel} idleIcon={<Check size={14} />}>
                Confirm
              </ButtonProgress>
            </Button>
          </div>
        </div>
      </div>
    )}
    </>
  );
}
