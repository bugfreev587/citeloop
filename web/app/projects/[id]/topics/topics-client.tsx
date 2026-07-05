"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "next/navigation";
import { Archive, ArrowRight, CalendarDays, Check, Loader2, Pencil, Power, Wand2, X } from "lucide-react";
import { defaultProjectConfig } from "../../../lib/api";
import type { ProjectConfig, Topic, VisibilityActionInLoop, VisibilitySummary } from "../../../lib/api";
import {
  hasReviewableDraft,
  isBacklogStatus,
  planHealthForTopics,
  recommendedTopicIds,
  normalizedTopicPriority,
  topicWhy,
} from "../../../lib/content-plan-logic";
import { useApi } from "../../../lib/use-api";
import { useToast } from "../../../components/toast-provider";
import { RightDrawer } from "../../../components/right-drawer";
import { Badge, Button, ButtonProgress, EmptyState, Field, SectionHeader, TextArea, TextInput, cx } from "../../../components/ui";
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

const AUTO_WORKFLOW_HELP =
  "Auto On: accepted opportunities become planned topics and drafts on cadence. " +
  "Auto Off: automatic planning and drafting pause; manual drafting stays available from reviewed briefs and planned topics.";

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

function topicFromAcceptedAction(action: VisibilityActionInLoop): Topic | null {
  if (!action.topic_id) return null;
  return {
    id: action.topic_id,
    project_id: undefined,
    channel: "blog",
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

function reviewHrefForAction(projectId: string, action: VisibilityActionInLoop) {
  return `/projects/${projectId}/review?article=${action.draft_article_id}`;
}

export function TopicsClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const searchParams = useSearchParams();
  const [topics, setTopics] = useState<Topic[]>([]);
  const [scheduleDrafts, setScheduleDrafts] = useState<Record<string, string>>({});
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
  const [config, setConfig] = useState<ProjectConfig | null>(null);
  const [inReview, setInReview] = useState(0);
  const [approvedCount, setApprovedCount] = useState(0);
  const contentPlanActionRefs = useRef<Record<string, HTMLDivElement | null>>({});
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
      setApprovedCount(approvedArticles.length);
      setScheduleDrafts(Object.fromEntries(next.map((topic) => [topic.id, toDateTimeLocal(topic.scheduled_at)])));
      setGeneratingIds((current) => {
        const stillGenerating = new Set(next.filter((topic) => topic.status === "generating").map((topic) => topic.id));
        return Object.fromEntries(Object.entries(current).filter(([id]) => stillGenerating.has(id)));
      });
    } catch (e: any) {
      setMessage({ title: "Topics unavailable", detail: e.message, tone: "amber" });
    }
  }, [api, projectId]);

  const acceptedPlanActions = useMemo(
    () =>
      (visibilitySummary?.actions_in_loop ?? []).filter(
        (action) =>
          isContentPlanAction(action) &&
          ["added_to_plan", "planned", "drafting", "ready_for_review"].includes(action.lifecycle_stage) &&
          !["dismissed", "archived"].includes(String(action.opportunity_status ?? "").toLowerCase()),
      ),
    [visibilitySummary],
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
    if (!requestedActionID || acceptedPlanActions.length === 0) return;
    const target = contentPlanActionRefs.current[requestedActionID];
    if (!target) return;
    target.scrollIntoView({ block: "center", behavior: "smooth" });
    target.focus({ preventScroll: true });
    setHighlightContentPlanAction(requestedActionID);
    const timeout = window.setTimeout(() => setHighlightContentPlanAction(null), 2_200);
    return () => window.clearTimeout(timeout);
  }, [acceptedPlanActions, requestedActionID]);

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
  const planHealth = useMemo(() => planHealthForTopics(topics), [topics]);
  const planStatusItems = [
    { label: "Planned topics", value: planHealth.backlog },
    { label: "Ready to draft", value: planHealth.readyToDraft },
    { label: "Scheduled intent", value: planHealth.scheduledIntent },
    { label: "Needs priority", value: planHealth.needsPriority },
  ];
  const recommendedIds = useMemo(() => {
    return new Set(recommendedTopicIds(backlogTopics));
  }, [backlogTopics]);
  const topicGridClass = "grid gap-3 lg:grid-cols-2";
  const selectedActionDraftBusy = selectedContentPlanAction ? busy === `draft-action-${selectedContentPlanAction.id}` : false;
  const selectedActionDismissBusy = selectedContentPlanAction ? busy === `dismiss-action-${selectedContentPlanAction.id}` : false;
  const selectedActionHasReviewContent = hasReviewableDraft(selectedContentPlanAction);
  const selectedActionRiskReasons = selectedContentPlanAction ? contentPlanRiskReasons(selectedContentPlanAction) : [];
  const reviewingContentPlanAction = Boolean(busy) || autoEnabled;

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
          ? "Accepted opportunities can become planned topics and draft on cadence."
          : "Accepted opportunities stay in the action handoff. Manual drafting stays available.",
        tone: nextEnabled ? "green" : "amber",
      });
    } catch (e: any) {
      setMessage({ title: "Auto setting failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  function replaceTopic(updated: Topic) {
    setTopics((current) => current.map((topic) => (topic.id === updated.id ? updated : topic)));
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
      setMessage({ title: "Topic saved", detail: updated.title, tone: "green" });
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
      setMessage({ title: updated.scheduled_at ? "Topic scheduled" : "Schedule cleared", detail: updated.title, tone: "green" });
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
      setMessage({ title: "Topic archived", detail: updated.title, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Archive failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function generate(topic: Topic) {
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
        return;
      }
      await refresh();
      const existing = result.articles?.length ?? 0;
      setMessage(
        existing > 0
          ? {
              title: "Draft already exists",
              detail: `This topic already has ${existing} draft${existing === 1 ? "" : "s"} in the review queue. Open Review to approve or regenerate.`,
              tone: "amber",
            }
          : { title: "Topic generated", detail: "Draft is ready in the review queue.", tone: "green" },
      );
    } catch (e: any) {
      setMessage({
        title: "Generate failed",
        detail: e.message,
        tone: "red",
      });
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

  async function draftAcceptedAction(action: VisibilityActionInLoop) {
    setBusy(`draft-action-${action.id}`);
    setMessage(null);
    try {
      let topic =
        (action.topic_id ? topics.find((item) => item.id === action.topic_id) ?? topicFromAcceptedAction(action) : null) ??
        null;
      if (!topic) {
        topic = await api.planSEOContentAction(projectId, action.id);
        setTopics((current) => (current.some((item) => item.id === topic?.id) ? current : [topic!, ...current]));
      }
      await generate(topic);
    } catch (e: any) {
      setMessage({ title: "Draft Content failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  async function dismissAcceptedAction(action: VisibilityActionInLoop) {
    if (!action.opportunity_id) {
      setMessage({ title: "Dismiss unavailable", detail: "This content action is missing its opportunity link.", tone: "red" });
      return;
    }
    setBusy(`dismiss-action-${action.id}`);
    setMessage(null);
    try {
      await api.dismissSEOOpportunity(projectId, action.opportunity_id);
      setVisibilitySummary((current) =>
        current
          ? { ...current, actions_in_loop: current.actions_in_loop.filter((item) => item.id !== action.id) }
          : current,
      );
      setSelectedContentPlanActionID(null);
      setMessage({ title: "Opportunity dismissed", detail: contentPlanActionTitle(action), tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Dismiss failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
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

      {acceptedPlanActions.length > 0 && (
        <section data-content-plan-handoff-section className="space-y-3">
          <SectionHeader
            title="Accepted opportunities"
            eyebrow="Sent from Opportunity Queue"
            action={<Badge tone="green">{acceptedPlanActions.length}</Badge>}
          />
          <div className="grid gap-2">
            {acceptedPlanActions.map((action) => {
              const highlighted = highlightContentPlanAction === action.id;
              const actionHasReviewContent = hasReviewableDraft(action);
              return (
                <div
                  key={action.id}
                  id={`content-plan-action-${action.id}`}
                  ref={(node) => {
                    contentPlanActionRefs.current[action.id] = node;
                  }}
                  tabIndex={-1}
                  data-content-plan-action-card
                  className={cx(
                    "rounded-xl border bg-white p-4 shadow-sm transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820]",
                    highlighted ? "citeloop-linked-card-pulse border-[#d93820] ring-2 ring-[#d93820]/15" : "border-slate-200",
                  )}
                >
                  <div className="flex min-w-0 flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge tone="green">Accepted</Badge>
                        <Badge tone="blue">{contentPlanActionTypeLabel(action)}</Badge>
                        <Badge tone="neutral">{autoEnabled ? "Queued for planning" : "Auto paused"}</Badge>
                      </div>
                      <h3 className="mt-2 break-words text-base font-bold leading-6 text-slate-950">{contentPlanActionTitle(action)}</h3>
                      <p className="mt-1 break-words text-sm leading-5 text-slate-500">{contentPlanActionDetail(action)}</p>
                    </div>
                    <div className="flex shrink-0 flex-wrap items-center gap-2 lg:justify-end">
                      {actionHasReviewContent ? (
                        <a
                          href={reviewHrefForAction(projectId, action)}
                          className="inline-flex h-9 items-center justify-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50"
                        >
                          View in Review
                          <ArrowRight size={15} />
                        </a>
                      ) : (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setSelectedContentPlanActionID(action.id)}
                          aria-label={`Review accepted content brief: ${contentPlanActionTitle(action)}`}
                        >
                          Review brief
                          <ArrowRight size={15} />
                        </Button>
                      )}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </section>
      )}

      <section className="space-y-3">
        <SectionHeader title="Planned topics" eyebrow="Draft queue" action={<Badge tone="neutral">{backlogTopics.length}</Badge>} />
        <div className="rounded-xl border border-slate-200 bg-white p-4">
          <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Plan status</div>
          <div className="mt-2 grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
            {planStatusItems.map((item) => (
              <div key={item.label} className="rounded-lg border border-slate-100 bg-slate-50 px-3 py-2">
                <div className="font-mono text-lg font-bold text-slate-950">{item.value}</div>
                <div className="text-xs font-semibold text-slate-500">{item.label}</div>
              </div>
            ))}
          </div>
        </div>
        {backlogTopics.length === 0 ? (
          <EmptyState
            title="No planned topics yet"
            detail="Review opportunities to add accepted content work."
          />
        ) : (
          <div className={topicGridClass}>
            {backlogTopics.map((topic) => {
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
                  <Badge tone="blue">{topic.channel}</Badge>
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
                        <Field label="Channel">
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
                        <ButtonProgress busy={editBusy} busyLabel="Saving topic" idleIcon={<Check size={14} />}>
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
                        Draft now
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
        )}
      </section>
    </div>
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
            <Badge tone="neutral">{autoEnabled ? "Auto drafting" : "Manual review"}</Badge>
          </>
        }
        footer={
          <>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => dismissAcceptedAction(selectedContentPlanAction)}
              disabled={Boolean(busy)}
            >
              <ButtonProgress busy={selectedActionDismissBusy} busyLabel="Dismissing" idleIcon={<X size={14} />}>
                Dismiss
              </ButtonProgress>
            </Button>
            {selectedActionHasReviewContent ? (
              <a
                href={reviewHrefForAction(projectId, selectedContentPlanAction)}
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
                onClick={() => draftAcceptedAction(selectedContentPlanAction)}
                title={autoEnabled ? "Auto is on, so AI Editor will draft this content automatically." : "Send this brief to AI Editor and QA Review."}
              >
                <ButtonProgress busy={selectedActionDraftBusy} busyLabel="Drafting" idleIcon={<Wand2 size={14} />}>
                  Draft Content
                </ButtonProgress>
              </Button>
            )}
          </>
        }
      >
        <div className="space-y-5">
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
              <div className="text-xs font-semibold uppercase text-slate-400">Plan status</div>
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
    </>
  );
}
