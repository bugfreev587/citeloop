"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { CheckCircle2, ExternalLink, FileText, Loader2, RefreshCw, Save, Search, ShieldAlert, Sparkles, X, XCircle } from "lucide-react";
import { Article, ReviewGroup } from "../../../lib/api";
import {
  articlePreviewHref,
  articleReviewTitle,
  buildSEOContributions,
  qaClaimRows,
  reviewArticleState,
  reviewQueueSummary,
  searchAppearanceRows,
  type QAClaimRow,
  type ReviewArticleState,
  type SEOContribution,
} from "../../../lib/review-insights";
import { useApi } from "../../../lib/use-api";
import { useToast } from "../../../components/toast-provider";
import { Badge, Button, ButtonProgress, EmptyState, SectionHeader, TextArea, cx } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;
type QueueArticle = { article: Article; topicId: string };
const drawerFocusableSelector =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

const STATE_ORDER: Record<ReviewArticleState["kind"], number> = {
  needs_human: 0,
  ready: 1,
  recovering: 2,
};

type AssetMetadata = {
  assetType: string;
  assetTypeLabel: string;
  sourceEvidence: string[];
};

function assetMetadata(article: Article): AssetMetadata {
  const rawType = typeof article.seo_meta?.asset_type === "string" ? article.seo_meta.asset_type.trim() : "";
  return {
    assetType: rawType,
    assetTypeLabel: rawType ? rawType.replace(/_/g, " ") : "",
    sourceEvidence: stringList(article.seo_meta?.source_evidence),
  };
}

function stringList(value: any): string[] {
  if (!Array.isArray(value)) return [];
  return value.map((item) => String(item).trim()).filter(Boolean);
}

export function ReviewClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [groups, setGroups] = useState<ReviewGroup[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [selectedArticleId, setSelectedArticleId] = useState<string | null>(null);
  const [editorOpen, setEditorOpen] = useState(false);
  const [content, setContent] = useState("");
  const reviewSurfaceRef = useRef<HTMLDivElement | null>(null);
  const reviewDrawerRef = useRef<HTMLElement | null>(null);
  const reviewReturnFocusRef = useRef<HTMLElement | null>(null);
  const { notify } = useToast();
  const setMessage = (next: Message) => {
    if (next) notify(next);
  };

  const refresh = useCallback(async () => {
    try {
      setGroups(await api.listReview(projectId));
    } catch (e: any) {
      setMessage({ title: "Review queue unavailable", detail: e.message, tone: "amber" });
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  // CiteLoop's recovery loop runs server-side every couple of minutes. While
  // anything is still being handled, poll so the operator watches the queue drain
  // itself instead of guessing.
  const hasRecovering = useMemo(
    () => groups.some((group) => group.articles.some((article) => reviewArticleState(article).kind === "recovering")),
    [groups],
  );
  useEffect(() => {
    if (!hasRecovering) return;
    const interval = window.setInterval(refresh, 15_000);
    return () => window.clearInterval(interval);
  }, [hasRecovering, refresh]);

  const summary = useMemo(() => reviewQueueSummary(groups), [groups]);
  const queueArticles = useMemo(() => {
    return groups
      .flatMap((group) => group.articles.map((article) => ({ article, topicId: group.topic_id })))
      .sort((a, b) => STATE_ORDER[reviewArticleState(a.article).kind] - STATE_ORDER[reviewArticleState(b.article).kind]);
  }, [groups]);

  const readyArticles = useMemo(() => queueArticles.filter((item) => reviewArticleState(item.article).kind === "ready"), [queueArticles]);

  const selectedQueueArticle = queueArticles.find((item) => item.article.id === selectedArticleId) ?? null;
  const selectedArticle = selectedQueueArticle?.article ?? null;
  const selectedBusy = selectedArticle
    ? busy === "bulk-approve" ||
      busy === `approve-${selectedArticle.id}` ||
      busy === `reject-${selectedArticle.id}` ||
      busy === `save-${selectedArticle.id}` ||
      busy === `recheck-${selectedArticle.id}` ||
      (busy?.startsWith(`apply-${selectedArticle.id}`) ?? false)
    : false;

  useEffect(() => {
    if (selectedArticleId && !queueArticles.some((item) => item.article.id === selectedArticleId)) {
      setSelectedArticleId(null);
    }
  }, [queueArticles, selectedArticleId]);

  useEffect(() => {
    if (!selectedArticle?.id) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setSelectedArticleId(null);
      if (event.key === "Tab") {
        const drawer = reviewDrawerRef.current;
        if (!drawer) return;
        const focusable = Array.from(drawer.querySelectorAll<HTMLElement>(drawerFocusableSelector)).filter(
          (element) => !element.hasAttribute("disabled") && element.getAttribute("aria-hidden") !== "true",
        );
        if (focusable.length === 0) {
          event.preventDefault();
          return;
        }
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [selectedArticle?.id]);

  useEffect(() => {
    if (!selectedArticle?.id) return;
    const previousBodyOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const closeButton = reviewDrawerRef.current?.querySelector<HTMLElement>("[data-drawer-close]");
    const firstFocusable = closeButton ?? reviewDrawerRef.current?.querySelector<HTMLElement>(drawerFocusableSelector);
    firstFocusable?.focus();
    if (reviewSurfaceRef.current) {
      reviewSurfaceRef.current.setAttribute("aria-hidden", "true");
      reviewSurfaceRef.current.inert = true;
    }
    return () => {
      document.body.style.overflow = previousBodyOverflow;
      if (reviewSurfaceRef.current) {
        reviewSurfaceRef.current.removeAttribute("aria-hidden");
        reviewSurfaceRef.current.inert = false;
      }
      if (reviewReturnFocusRef.current?.isConnected) {
        reviewReturnFocusRef.current?.focus();
      }
    };
  }, [selectedArticle?.id]);

  useEffect(() => {
    if (!selectedArticle) {
      setContent("");
      setEditorOpen(false);
      return;
    }
    setContent(selectedArticle.content_md);
    setEditorOpen(false);
  }, [selectedArticle?.id, selectedArticle]);

  async function mutate(label: string, busyKey: string, fn: () => Promise<any>) {
    setBusy(busyKey);
    setMessage(null);
    try {
      await fn();
      await refresh();
      setMessage({ title: label, tone: "green" });
    } catch (e: any) {
      const isGate = String(e.message).includes("409");
      setMessage({
        title: isGate ? "Draft is still being checked" : `${label} failed`,
        detail: isGate ? "QA has not cleared this draft yet. CiteLoop is still working on it." : e.message,
        tone: isGate ? "amber" : "red",
      });
    } finally {
      setBusy(null);
    }
  }

  async function approveReadyArticles() {
    if (readyArticles.length === 0) return;
    if (!window.confirm(`Approve ${readyArticles.length} ready draft${readyArticles.length === 1 ? "" : "s"}?`)) return;
    setBusy("bulk-approve");
    setMessage(null);
    try {
      for (const item of readyArticles) {
        await api.approve(projectId, item.article.id);
      }
      await refresh();
      setMessage({ title: `${readyArticles.length} draft${readyArticles.length === 1 ? "" : "s"} approved`, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Approve ready drafts failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  function onApprove(article: Article) {
    return mutate("Draft approved", `approve-${article.id}`, () => api.approve(projectId, article.id));
  }
  function onReject(article: Article) {
    return mutate("Draft rejected", `reject-${article.id}`, () => api.reject(projectId, article.id));
  }
  function onSave(article: Article, nextContent: string) {
    return mutate("Content saved and QA re-checked", `save-${article.id}`, () => api.edit(projectId, article.id, { content_md: nextContent }));
  }
  function onApplyFix(article: Article, optionIndex: number, instruction: string) {
    return mutate("CiteLoop applied the fix and approved the draft", `apply-${article.id}-${optionIndex}`, () => api.applyFix(projectId, article.id, instruction));
  }
  function onRecheck(article: Article) {
    return mutate("CiteLoop re-ran the QA check", `recheck-${article.id}`, () => api.recheckArticle(projectId, article.id));
  }

  return (
    <div className="space-y-6">
      <div ref={reviewSurfaceRef} className="space-y-6">
        <SectionHeader
          title="Review"
          eyebrow="Mostly automatic — you only decide the rare cases"
          action={
            <div className="flex flex-wrap justify-end gap-2">
              {readyArticles.length > 0 && (
                <Button disabled={!!busy} size="sm" variant="primary" onClick={approveReadyArticles}>
                  <ButtonProgress busy={busy === "bulk-approve"} busyLabel="Approving" idleIcon={<CheckCircle2 size={14} />}>
                    Approve {readyArticles.length} ready
                  </ButtonProgress>
                </Button>
              )}
              <Button disabled={!!busy} size="sm" onClick={refresh}>
                <RefreshCw size={14} />
                Refresh
              </Button>
            </div>
          }
        />

        {summary.total === 0 ? (
          <EmptyState
            title="Nothing needs you"
            detail="CiteLoop drafts, checks, repairs, and—when auto-advance is on—publishes on its own. Drafts that need a real positioning choice or manual edit will show up here."
          />
        ) : (
          <>
            <section data-review-overall-metrics className="space-y-3">
              <SectionHeader title="Overall Metrics" eyebrow="Review queue status" />
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                <ReviewMetricCard label="Needs your decision" value={summary.needsHuman} detail="Only rare manual calls" tone="red" />
                <ReviewMetricCard label="Ready to approve" value={summary.ready} detail="QA cleared these drafts" tone="green" />
                <ReviewMetricCard label="CiteLoop is handling" value={summary.recovering} detail="Re-checking, repairing, regenerating" tone="amber" />
                <ReviewMetricCard label="Total in review" value={summary.total} detail="Drafts currently visible here" tone="neutral" />
              </div>
            </section>

            <section data-review-decision-section className="space-y-3">
              <SectionHeader title="Needs Your Decision" eyebrow="Open a card to inspect details and act" action={<Badge tone="neutral">{queueArticles.length}</Badge>} />
              {queueArticles.length === 0 ? (
                <EmptyState title="No review cards" detail="Drafts that need a decision or approval will appear here." />
              ) : (
                <div className="grid gap-3 lg:grid-cols-2">
                  {queueArticles.map((item) => (
                    <ReviewDecisionCard
                      key={item.article.id}
                      item={item}
                      selected={selectedArticleId === item.article.id}
                      onSelect={(trigger) => {
                        reviewReturnFocusRef.current = trigger;
                        setSelectedArticleId(item.article.id);
                      }}
                    />
                  ))}
                </div>
              )}
            </section>
          </>
        )}
      </div>

      {selectedArticle && (
        <div className="fixed inset-0 z-30">
          <button
            type="button"
            aria-label="Close review details"
            onClick={() => setSelectedArticleId(null)}
            className="absolute inset-0 motion-safe:animate-[citeloop-drawer-scrim-in_180ms_ease-out] bg-slate-950/25"
          />
          <ReviewInspector
            drawerRef={(node) => {
              reviewDrawerRef.current = node;
            }}
            article={selectedArticle}
            topicId={selectedQueueArticle?.topicId ?? selectedArticle.topic_id}
            projectId={projectId}
            busy={selectedBusy}
            approveBusy={busy === `approve-${selectedArticle.id}` || busy === "bulk-approve"}
            rejectBusy={busy === `reject-${selectedArticle.id}`}
            saveBusy={busy === `save-${selectedArticle.id}`}
            editorOpen={editorOpen}
            content={content}
            onContentChange={setContent}
            onToggleEditor={() => setEditorOpen((value) => !value)}
            onApprove={() => onApprove(selectedArticle)}
            onReject={() => onReject(selectedArticle)}
            onSave={(next) => onSave(selectedArticle, next)}
            onApplyFix={(optionIndex, instruction) => onApplyFix(selectedArticle, optionIndex, instruction)}
            applyingIndex={busy?.startsWith(`apply-${selectedArticle.id}-`) ? Number(busy.split("-").pop()) : null}
            onRecheck={() => onRecheck(selectedArticle)}
            recheckBusy={busy === `recheck-${selectedArticle.id}`}
            onClose={() => setSelectedArticleId(null)}
          />
        </div>
      )}
    </div>
  );
}

function ReviewMetricCard({
  label,
  value,
  detail,
  tone,
}: {
  label: string;
  value: number;
  detail: string;
  tone: "green" | "amber" | "red" | "neutral";
}) {
  const valueClass = {
    green: "text-green-700",
    amber: "text-amber-700",
    red: value > 0 ? "text-red-700" : "text-slate-950",
    neutral: "text-slate-950",
  }[tone];
  return (
    <div data-review-metric-card className="rounded-xl border border-slate-200 bg-white p-4">
      <div className="text-[11px] font-bold uppercase tracking-[0.12em] text-slate-500">{label}</div>
      <div className={cx("mt-3 text-2xl font-bold leading-none", valueClass)}>{value}</div>
      <div className="mt-2 text-[13px] font-semibold leading-5 text-slate-400">{detail}</div>
    </div>
  );
}

function ReviewDecisionCard({
  item,
  selected,
  onSelect,
}: {
  item: QueueArticle;
  selected: boolean;
  onSelect: (trigger: HTMLElement) => void;
}) {
  const { article } = item;
  const state = reviewArticleState(article);
  const title = articleReviewTitle(article);
  const titleId = `review-card-title-${article.id}`;
  const descriptionId = `review-card-description-${article.id}`;

  return (
    <button
      data-review-card
      type="button"
      onClick={(event) => onSelect(event.currentTarget)}
      aria-labelledby={titleId}
      aria-describedby={descriptionId}
      className={cx(
        "group min-w-0 rounded-xl border bg-white p-4 text-left shadow-sm transition hover:-translate-y-0.5 hover:border-slate-300 hover:shadow-md active:translate-y-0",
        selected ? "border-slate-400 ring-2 ring-slate-200" : "border-slate-200",
      )}
    >
      <div className="flex flex-wrap items-center gap-2">
        <StateBadge state={state} />
        <Badge tone={article.kind === "canonical" ? "green" : "neutral"}>{article.platform || article.kind}</Badge>
      </div>
      <h3 id={titleId} className="mt-3 text-base font-bold leading-6 text-slate-950">{title}</h3>
      <p id={descriptionId} data-review-card-description className="mt-2 line-clamp-2 text-sm leading-6 text-slate-600">
        {state.kind === "ready" ? "Ready for a final scan before publishing." : state.detail}
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-slate-100 pt-3 text-xs text-slate-500">
        {state.kind === "recovering" ? (
          <span className="inline-flex items-center gap-1.5 font-semibold text-amber-700">
            <Loader2 size={12} className="animate-spin" />
            {article.repair_status === "repairing" ? "Repairing draft" : "Re-checking with QA"}
          </span>
        ) : (
          <span className="font-semibold text-slate-600">{state.kind === "ready" ? "Ready to approve" : "Decision required"}</span>
        )}
        <span className="ml-auto font-semibold text-slate-700 transition group-hover:translate-x-0.5">Open details</span>
      </div>
    </button>
  );
}

function StateBadge({ state }: { state: ReviewArticleState }) {
  const tone = state.kind === "ready" ? "green" : state.kind === "recovering" ? "amber" : "red";
  return <Badge tone={tone}>{state.label}</Badge>;
}

function ReviewInspector({
  drawerRef,
  article,
  topicId,
  projectId,
  busy,
  approveBusy,
  rejectBusy,
  saveBusy,
  editorOpen,
  content,
  onContentChange,
  onToggleEditor,
  onApprove,
  onReject,
  onSave,
  onApplyFix,
  applyingIndex,
  onRecheck,
  recheckBusy,
  onClose,
}: {
  drawerRef: (node: HTMLElement | null) => void;
  article: Article;
  topicId: string;
  projectId: string;
  busy: boolean;
  approveBusy: boolean;
  rejectBusy: boolean;
  saveBusy: boolean;
  editorOpen: boolean;
  content: string;
  onContentChange: (value: string) => void;
  onToggleEditor: () => void;
  onApprove: () => void;
  onReject: () => void;
  onSave: (content: string) => void;
  onApplyFix: (optionIndex: number, instruction: string) => void;
  applyingIndex: number | null;
  onRecheck: () => void;
  recheckBusy: boolean;
  onClose: () => void;
}) {
  const title = articleReviewTitle(article);
  const state = reviewArticleState(article);
  const seoContributions = useMemo(() => buildSEOContributions(article), [article]);
  const previewHref = articlePreviewHref(projectId, article);
  const detailHref = `/projects/${projectId}/articles/${article.id}`;
  const metadata = assetMetadata(article);
  const showRecheck = isReviewInfraFailure(article);

  return (
    <aside
      ref={drawerRef}
      data-review-drawer
      role="dialog"
      aria-modal="true"
      aria-labelledby="review-details-title"
      className="absolute right-0 top-0 flex h-[100dvh] max-h-[100dvh] w-full max-w-2xl motion-safe:animate-[citeloop-drawer-panel-in_220ms_cubic-bezier(0.16,1,0.3,1)] flex-col overflow-hidden border-l border-slate-200 bg-white shadow-2xl"
    >
      <div className="flex items-start justify-between gap-4 border-b border-slate-200 bg-white px-4 py-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <StateBadge state={state} />
            {metadata.assetType && <Badge tone="blue">Asset type: {metadata.assetTypeLabel}</Badge>}
            <Badge tone="neutral">Topic {topicId.slice(0, 8)}</Badge>
          </div>
          <h3 id="review-details-title" className="mt-3 content-font text-lg font-bold leading-6 text-slate-950">{title}</h3>
          <p className="mt-2 text-sm leading-6 text-slate-600">{state.detail}</p>
        </div>
        <button
          type="button"
          data-drawer-close
          aria-label="Close review details"
          onClick={onClose}
          className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-slate-200 text-slate-500 transition hover:bg-slate-50 active:translate-y-px"
        >
          <X size={16} />
        </button>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain bg-slate-50">
        <div className="grid gap-4 p-4">
          {state.kind === "recovering" && <RecoveringPanel article={article} />}
          {state.kind === "needs_human" && (
            <DecisionPanel
              article={article}
              busy={busy}
              onApplyFix={onApplyFix}
              applyingIndex={applyingIndex}
            />
          )}
          {state.kind === "ready" && <ReadyPanel />}

          {(metadata.assetType || metadata.sourceEvidence.length > 0) && <AssetMetadataPanel metadata={metadata} />}
          <ClaimEvidencePanel article={article} />
          <SearchAppearancePanel article={article} />
          <SEOContributionPanel rows={seoContributions} />

          {editorOpen && <DraftEditor content={content} busy={busy} saveBusy={saveBusy} onChange={onContentChange} onSave={onSave} />}
        </div>
      </div>

      <ReviewDrawerActions
        state={state}
        busy={busy}
        approveBusy={approveBusy}
        rejectBusy={rejectBusy}
        recheckBusy={recheckBusy}
        showRecheck={showRecheck}
        previewHref={previewHref}
        detailHref={detailHref}
        onApprove={onApprove}
        onReject={onReject}
        onToggleEditor={onToggleEditor}
        onRecheck={onRecheck}
      />
    </aside>
  );
}

function AssetMetadataPanel({ metadata }: { metadata: AssetMetadata }) {
  return (
    <section className="rounded-lg border border-sky-100 bg-sky-50 p-3">
      <div className="text-xs font-bold uppercase tracking-[0.08em] text-sky-700">Asset type</div>
      <div className="mt-1 text-sm font-semibold text-slate-950">{metadata.assetTypeLabel || "GEO asset"}</div>
      {metadata.sourceEvidence.length > 0 && (
        <div className="mt-3">
          <div className="text-xs font-bold uppercase tracking-[0.08em] text-sky-700">Source evidence</div>
          <ul className="mt-2 space-y-1 text-xs leading-5 text-slate-700">
            {metadata.sourceEvidence.slice(0, 5).map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}

function ReviewDrawerActions({
  state,
  busy,
  approveBusy,
  rejectBusy,
  recheckBusy,
  showRecheck,
  previewHref,
  detailHref,
  onApprove,
  onReject,
  onToggleEditor,
  onRecheck,
}: {
  state: ReviewArticleState;
  busy: boolean;
  approveBusy: boolean;
  rejectBusy: boolean;
  recheckBusy: boolean;
  showRecheck: boolean;
  previewHref: string;
  detailHref: string;
  onApprove: () => void;
  onReject: () => void;
  onToggleEditor: () => void;
  onRecheck: () => void;
}) {
  return (
    <div
      aria-label="Review drawer actions"
      className="shrink-0 flex flex-col gap-2 border-t border-slate-200 bg-white px-4 pb-[calc(1.5rem+env(safe-area-inset-bottom))] pt-4 sm:flex-row sm:justify-end"
    >
      {state.kind === "recovering" && (
        <>
          <a href={previewHref} target="_blank" rel="noopener noreferrer" className="inline-flex h-8 items-center justify-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50">
            Preview <ExternalLink size={14} />
          </a>
          <a href={detailHref} className="inline-flex h-8 items-center justify-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50">
            Detail
          </a>
        </>
      )}
      {state.kind === "needs_human" && (
        <>
          {showRecheck && (
            <Button disabled={busy} size="sm" onClick={onRecheck}>
              <ButtonProgress busy={recheckBusy} busyLabel="Re-running QA" idleIcon={<RefreshCw size={14} />}>
                Re-run QA
              </ButtonProgress>
            </Button>
          )}
          <Button disabled={busy} size="sm" onClick={onToggleEditor}>
            <FileText size={14} />
            Edit draft
          </Button>
          <Button disabled={busy} size="sm" variant="danger" onClick={onReject}>
            <ButtonProgress busy={rejectBusy} busyLabel="Rejecting" idleIcon={<XCircle size={14} />}>
              Reject
            </ButtonProgress>
          </Button>
        </>
      )}
      {state.kind === "ready" && (
        <>
          <Button disabled={busy} size="sm" onClick={onToggleEditor}>
            <FileText size={14} />
            Edit draft
          </Button>
          <a href={previewHref} target="_blank" rel="noopener noreferrer" className="inline-flex h-8 items-center justify-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50">
            Preview <ExternalLink size={14} />
          </a>
          <a href={detailHref} className="inline-flex h-8 items-center justify-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50">
            Detail
          </a>
          <Button disabled={busy} size="sm" variant="primary" onClick={onApprove}>
            <ButtonProgress busy={approveBusy} busyLabel="Approving" idleIcon={<CheckCircle2 size={14} />}>
              Approve
            </ButtonProgress>
          </Button>
        </>
      )}
    </div>
  );
}

function RecoveringPanel({ article }: { article: Article }) {
  const repairing = article.repair_status === "repairing";
  return (
    <section className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-950">
      <div className="inline-flex items-center gap-2 font-semibold">
        <Loader2 size={15} className="animate-spin" />
        CiteLoop is handling this draft
      </div>
      <p className="mt-2 text-xs leading-5 text-amber-900">
        {repairing
          ? "An automatic edit is in progress; QA will re-run when it finishes."
          : "QA is being re-run on this draft. If it can't be cleared automatically, CiteLoop repairs or regenerates it — and only asks you for a real positioning choice or manual edit."}
      </p>
      <p className="mt-2 text-[11px] font-semibold uppercase tracking-[0.08em] text-amber-700">No action needed</p>
    </section>
  );
}

function ReadyPanel() {
  return (
    <section className="rounded-lg border border-green-200 bg-green-50 p-3">
      <div className="inline-flex items-center gap-2 text-sm font-bold text-green-900">
        <Sparkles size={15} />
        QA cleared this draft
      </div>
      <p className="mt-1 text-xs leading-5 text-green-800">Approve to publish on schedule, or open the preview for a final scan.</p>
    </section>
  );
}

const genericOptionPattern = /^(reject|edit the draft|add or fix evidence)/i;
const contextEvidenceOptionPattern = /\b(context|evidence|profile|source)\b/i;
const qaInfraFailurePattern = /parse qa|unexpected eof|qa re-check failed|qa step failed|missing claims|compact fallback/i;

function decisionText(value: any): string {
  return typeof value === "string" ? value.trim() : "";
}

function isReviewInfraFailure(article: Article) {
  const rawReason = decisionText(article.qa_feedback?.blocking_reason) || decisionText(article.qa_issues?.[0]);
  return qaInfraFailurePattern.test(rawReason);
}

function DecisionPanel({
  article,
  busy,
  onApplyFix,
  applyingIndex,
}: {
  article: Article;
  busy: boolean;
  onApplyFix: (optionIndex: number, instruction: string) => void;
  applyingIndex: number | null;
}) {
  const allOptions = (article.human_decision_options ?? []).filter((option) => option?.label || option?.description);
  // QA-proposed content fixes (one-click) vs the standard manual actions, which
  // have their own buttons below.
  const fixOptions = allOptions
    .map((option, index) => ({ option, index }))
    .filter(({ option }) => {
      const text = `${option.label ?? ""} ${option.description ?? ""}`.trim();
      if ((option.label ?? "").trim() === "Apply QA fix") return true;
      return !genericOptionPattern.test((option.label ?? "").trim()) && !contextEvidenceOptionPattern.test(text);
    });
  const unmapped = qaClaimRows(article).filter((row) => !row.mapped);
  const rawReason = decisionText(article.qa_feedback?.blocking_reason) || decisionText(article.qa_issues?.[0]);
  // A QA *infrastructure* failure (truncated/unparseable model response) is not a
  // content decision — never show the raw error; offer a one-click re-check.
  const isInfraFailure = isReviewInfraFailure(article);
  const blockingReason = isInfraFailure ? "" : rawReason;
  const fixInstructions = Array.isArray(article.qa_feedback?.fix_instructions)
    ? (article.qa_feedback!.fix_instructions as any[]).map((v) => String(v).trim()).filter(Boolean)
    : [];

  return (
    <section className="rounded-lg border border-red-200 bg-red-50 p-3">
      <div className="inline-flex items-center gap-2 text-sm font-bold text-red-900">
        <ShieldAlert size={15} />
        Your decision is needed
      </div>

      {/* Why QA blocked this — the concrete reason, not a generic message. */}
      <div className="mt-2 rounded-md border border-red-100 bg-white/70 px-3 py-2">
        <div className="text-[11px] font-bold uppercase tracking-[0.08em] text-red-700">
          {isInfraFailure ? "Automated check didn't complete" : "Why QA blocked this"}
        </div>
        <p className="mt-1 text-sm leading-5 text-slate-900">
          {isInfraFailure
            ? "CiteLoop couldn't finish its automated quality check on this draft — a temporary model/formatting issue, not a problem with your content. Re-run the check (it usually clears on its own)."
            : blockingReason ||
              (unmapped.length > 0
                ? `CiteLoop could not automatically rewrite ${unmapped.length === 1 ? "this unsupported product claim" : `${unmapped.length} unsupported product claims`} after several attempts. Edit the draft or reject it.`
                : "CiteLoop re-checked, repaired, and regenerated this draft but QA still could not clear it. Edit the draft or reject it.")}
        </p>
        {unmapped.length > 0 && (
          <div className="mt-2 grid gap-1.5">
            {unmapped.slice(0, 4).map((row, index) => (
              <div key={`${row.claim}-${index}`} className="rounded border border-red-100 bg-red-50/60 px-2 py-1.5">
                <div className="text-xs font-semibold text-slate-900">{row.claim}</div>
                {row.evidence && <div className="mt-0.5 text-[11px] leading-4 text-slate-600">{row.evidence}</div>}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* One-click fixes proposed by QA — applied by the AI editor, then approved. */}
      {fixOptions.length > 0 && (
        <div className="mt-3">
          <div className="text-[11px] font-bold uppercase tracking-[0.08em] text-red-700">Apply a fix</div>
          <div className="mt-1.5 grid gap-2">
            {fixOptions.map(({ option, index }) => {
              const instruction = [option.label, option.description].filter(Boolean).join(". ");
              const applying = applyingIndex === index;
              return (
                <button
                  key={`${option.label ?? "fix"}-${index}`}
                  type="button"
                  disabled={busy}
                  onClick={() => onApplyFix(index, instruction)}
                  className="flex items-start gap-2 rounded-md border border-red-200 bg-white px-3 py-2 text-left transition-colors hover:border-[#d93820] hover:bg-red-50 disabled:opacity-60"
                >
                  {applying ? (
                    <Loader2 size={14} className="mt-0.5 shrink-0 animate-spin text-[#d93820]" />
                  ) : (
                    <Sparkles size={14} className="mt-0.5 shrink-0 text-[#d93820]" />
                  )}
                  <span className="min-w-0">
                    <span className="block text-sm font-semibold text-slate-900">{option.label || `Fix ${index + 1}`}</span>
                    {option.description && <span className="mt-0.5 block text-xs leading-5 text-slate-600">{option.description}</span>}
                    <span className="mt-1 block text-[11px] font-semibold text-[#d93820]">{applying ? "Applying & approving…" : "Apply this fix"}</span>
                  </span>
                </button>
              );
            })}
          </div>
        </div>
      )}

      {fixInstructions.length > 0 && fixOptions.length === 0 && (
        <ul className="mt-3 grid list-disc gap-1 pl-5 text-xs leading-5 text-red-900">
          {fixInstructions.slice(0, 4).map((fix, index) => (
            <li key={`${fix}-${index}`}>{fix}</li>
          ))}
        </ul>
      )}

      <p className="mt-2 text-[11px] leading-4 text-red-700/80">
        QA only blocks unsupported product claims, banned claims, or missing required SEO — never writing style. Applying a fix approves the draft automatically; saving a manual edit still re-runs QA.
      </p>
    </section>
  );
}

function ClaimEvidencePanel({ article }: { article: Article }) {
  const rows = qaClaimRows(article);
  if (rows.length === 0) return null;
  return (
    <section className="rounded-lg border border-slate-200 bg-white p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="inline-flex items-center gap-2 text-sm font-bold text-slate-900">
          <ShieldAlert size={15} />
          Claim evidence map
        </div>
        <Badge tone={rows.some((row) => !row.mapped) ? "red" : "green"}>{rows.length} claims</Badge>
      </div>
      <div className="mt-3 grid gap-2">
        {rows.map((row, index) => (
          <ClaimRow key={`${row.claim}-${index}`} row={row} />
        ))}
      </div>
    </section>
  );
}

function ClaimRow({ row }: { row: QAClaimRow }) {
  return (
    <div className={cx("rounded-md border p-3", row.mapped ? "border-green-100 bg-green-50/60" : "border-red-100 bg-red-50/70")}>
      <div className="flex flex-wrap items-center gap-2">
        <Badge tone={row.mapped ? "green" : "red"}>{row.mapped ? "Mapped" : "Unmapped"}</Badge>
        <div className="min-w-0 text-sm font-semibold leading-5 text-slate-950">{row.claim}</div>
      </div>
      <div className="mt-2 text-xs leading-5 text-slate-600">{row.evidence || "No supporting evidence was returned for this claim."}</div>
    </div>
  );
}

function SearchAppearancePanel({ article }: { article: Article }) {
  const rows = searchAppearanceRows(article);
  return (
    <section className="rounded-lg border border-slate-200 bg-white p-3">
      <div className="mb-3 inline-flex items-center gap-2 text-sm font-bold text-slate-900">
        <Search size={15} />
        How this article appears in search
      </div>
      <div className="grid gap-2 sm:grid-cols-2">
        {rows.map((row) => (
          <div key={row.label} className="min-w-0 rounded-md border border-slate-200 bg-slate-50 p-3">
            <div className="text-[11px] font-bold uppercase tracking-[0.08em] text-slate-500">{row.label}</div>
            <div className="mt-1 truncate text-sm font-semibold text-slate-950" title={row.value}>
              {row.value}
            </div>
            <div className="mt-1 text-xs leading-5 text-slate-500">{row.detail}</div>
          </div>
        ))}
      </div>
    </section>
  );
}

function SEOContributionPanel({ rows }: { rows: SEOContribution[] }) {
  const ready = rows.filter((row) => row.status === "ready").length;
  const missing = rows.filter((row) => row.status === "missing").length;
  const needsReview = rows.filter((row) => row.status === "needs_review").length;

  return (
    <section className="rounded-lg border border-slate-200 bg-white p-3">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div className="inline-flex items-center gap-2 text-sm font-bold text-slate-900">
          <Search size={15} />
          SEO contribution
        </div>
        <div className="flex flex-wrap gap-2">
          <Badge tone="green">{ready} ready</Badge>
          {needsReview > 0 && <Badge tone="amber">{needsReview} review</Badge>}
          {missing > 0 && <Badge tone="red">{missing} missing</Badge>}
        </div>
      </div>
      <div className="grid gap-2 sm:grid-cols-2">
        {rows.map((row) => (
          <ContributionRow key={row.label} row={row} />
        ))}
      </div>
    </section>
  );
}

function ContributionRow({ row }: { row: SEOContribution }) {
  const dotClass = { ready: "bg-green-500", missing: "bg-red-500", needs_review: "bg-amber-500" }[row.status];
  return (
    <div className="rounded-md border border-slate-200 bg-slate-50 p-3">
      <div className="flex items-center gap-2">
        <span className={cx("h-2 w-2 shrink-0 rounded-full", dotClass)} />
        <div className="text-xs font-bold uppercase tracking-[0.08em] text-slate-500">{row.label}</div>
      </div>
      <div className="mt-1 truncate text-sm font-semibold text-slate-950" title={row.value}>
        {row.value}
      </div>
      <div className="mt-1 text-xs leading-5 text-slate-600">{row.detail}</div>
    </div>
  );
}

function DraftEditor({
  content,
  busy,
  saveBusy,
  onChange,
  onSave,
}: {
  content: string;
  busy: boolean;
  saveBusy: boolean;
  onChange: (value: string) => void;
  onSave: (content: string) => void;
}) {
  return (
    <section className="min-w-0 overflow-hidden rounded-lg border border-slate-200 bg-white">
      <div className="flex items-center gap-2 border-b border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-500">
        <FileText size={14} />
        <span className="font-semibold text-slate-700">Draft editor</span>
      </div>
      <div className="grid gap-2 p-3">
        <TextArea value={content} onChange={(event) => onChange(event.target.value)} className="min-h-[420px] font-mono text-xs leading-5" />
        <div className="flex flex-wrap items-center gap-3">
          <Button disabled={busy} size="sm" variant="primary" onClick={() => onSave(content)}>
            <ButtonProgress busy={saveBusy} busyLabel="Saving content" idleIcon={<Save size={14} />}>
              Save content
            </ButtonProgress>
          </Button>
          <span className="text-xs text-slate-500">Saving re-runs QA automatically.</span>
        </div>
      </div>
    </section>
  );
}
