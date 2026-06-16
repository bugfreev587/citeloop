"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { CheckCircle2, ExternalLink, FileText, RefreshCw, Save, Search, ShieldAlert, XCircle } from "lucide-react";
import { Article, ReviewGroup } from "../../../lib/api";
import {
  articlePreviewHref,
  articleReviewTitle,
  buildSEOContributions,
  explainQAIssue,
  qaClaimRows,
  reviewArticleState,
  reviewQueueSummary,
  searchAppearanceRows,
  shouldAutoRepairArticle,
  type QAClaimRow,
  type ReviewArticleState,
  type SEOContribution,
} from "../../../lib/review-insights";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, EmptyState, Notice, SectionHeader, TextArea, cx, formatScore } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;
type QueueArticle = { article: Article; topicId: string };

const STATE_ORDER: Record<ReviewArticleState["kind"], number> = {
  needs_human: 0,
  ready: 1,
  auto_repair: 2,
};

export function ReviewClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [groups, setGroups] = useState<ReviewGroup[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [repairing, setRepairing] = useState<Record<string, boolean>>({});
  const [repairAttempted, setRepairAttempted] = useState<Record<string, boolean>>({});
  const [repairFailures, setRepairFailures] = useState<Record<string, string>>({});
  const [selectedArticleId, setSelectedArticleId] = useState<string | null>(null);
  const [editorOpen, setEditorOpen] = useState(false);
  const [content, setContent] = useState("");
  const [message, setMessage] = useState<Message>(null);

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

  useEffect(() => {
    const candidate = groups
      .flatMap((group) => group.articles)
      .find((article) => shouldAutoRepairArticle(article) && !repairAttempted[article.id] && !repairing[article.id]);
    if (!candidate) return;

    let cancelled = false;
    setRepairAttempted((current) => ({ ...current, [candidate.id]: true }));
    setRepairing((current) => ({ ...current, [candidate.id]: true }));
    setRepairFailures((current) => {
      const next = { ...current };
      delete next[candidate.id];
      return next;
    });

    api
      .fixArticle(projectId, candidate.id)
      .then(async () => {
        if (cancelled) return;
        await refresh();
        setMessage({ title: "CiteLoop repaired a draft and reran QA", tone: "green" });
      })
      .catch((e: any) => {
        if (cancelled) return;
        setRepairFailures((current) => ({ ...current, [candidate.id]: e.message }));
        setMessage({
          title: "Automatic draft repair failed",
          detail: e.message,
          tone: "red",
        });
      })
      .finally(() => {
        if (cancelled) return;
        setRepairing((current) => {
          const next = { ...current };
          delete next[candidate.id];
          return next;
        });
      });

    return () => {
      cancelled = true;
    };
  }, [api, groups, projectId, refresh, repairAttempted, repairing]);

  const effectiveGroups = useMemo(
    () =>
      groups.map((group) => ({
        ...group,
        articles: group.articles.map((article) => (repairing[article.id] ? { ...article, repair_status: "repairing" } : article)),
      })),
    [groups, repairing],
  );

  const summary = useMemo(() => reviewQueueSummary(effectiveGroups), [effectiveGroups]);
  const queueArticles = useMemo(() => {
    return effectiveGroups
      .flatMap((group) => group.articles.map((article) => ({ article, topicId: group.topic_id })))
      .sort((a, b) => {
        const aState = reviewArticleState(a.article);
        const bState = reviewArticleState(b.article);
        return STATE_ORDER[aState.kind] - STATE_ORDER[bState.kind];
      });
  }, [effectiveGroups]);

  const selectedQueueArticle = queueArticles.find((item) => item.article.id === selectedArticleId) ?? queueArticles[0] ?? null;
  const selectedArticle = selectedQueueArticle?.article ?? null;
  const readyArticles = queueArticles.filter((item) => reviewArticleState(item.article).approvable).map((item) => item.article);

  useEffect(() => {
    if (queueArticles.length === 0) {
      if (selectedArticleId) setSelectedArticleId(null);
      return;
    }
    if (!selectedArticleId || !queueArticles.some((item) => item.article.id === selectedArticleId)) {
      setSelectedArticleId(queueArticles[0].article.id);
    }
  }, [queueArticles, selectedArticleId]);

  useEffect(() => {
    if (!selectedArticle) {
      setContent("");
      setEditorOpen(false);
      return;
    }
    setContent(selectedArticle.content_md);
    setEditorOpen(false);
  }, [selectedArticle?.id, selectedArticle]);

  async function mutate(label: string, id: string, fn: () => Promise<any>) {
    setBusy(id);
    setMessage(null);
    try {
      await fn();
      await refresh();
      setMessage({ title: label, tone: "green" });
    } catch (e: any) {
      const isGate = String(e.message).includes("409");
      setMessage({
        title: isGate ? "Article is still blocked" : `${label} failed`,
        detail: isGate ? "The article still has blocking QA issues. Open the draft details for the exact reason." : e.message,
        tone: isGate ? "amber" : "red",
      });
    } finally {
      setBusy(null);
    }
  }

  async function approveReadyArticles() {
    if (readyArticles.length === 0) return;
    if (!window.confirm(`Approve ${readyArticles.length} ready drafts?`)) return;
    setBusy("bulk-approve");
    setMessage(null);
    try {
      for (const article of readyArticles) {
        await api.approve(projectId, article.id);
      }
      await refresh();
      setMessage({ title: `${readyArticles.length} ready drafts approved`, tone: "green" });
    } catch (e: any) {
      setMessage({ title: "Approve ready drafts failed", detail: e.message, tone: "red" });
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Review queue"
        eyebrow="The human publishing gate"
        action={
          <div className="flex flex-wrap justify-end gap-2">
            <Button disabled={!!busy || readyArticles.length === 0} size="sm" onClick={approveReadyArticles}>
              <CheckCircle2 size={14} />
              Approve {readyArticles.length} ready...
            </Button>
            <Button disabled={!!busy} size="sm" onClick={refresh}>
              <RefreshCw size={14} />
              Refresh
            </Button>
          </div>
        }
      />
      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}

      {summary.total === 0 ? (
        <EmptyState title="Nothing pending review" detail="Writer and QA output will appear here before publishing." />
      ) : (
        <section className="overflow-hidden rounded-xl border border-slate-200 bg-white">
          <div className="grid border-b border-slate-200 bg-white sm:grid-cols-2 xl:grid-cols-4">
            <SummaryCard label="Pending review" value={summary.total} detail={`${summary.bundleCount} content bundles`} />
            <SummaryCard label="Ready to approve" value={summary.ready} detail="QA has cleared these drafts" tone="green" />
            <SummaryCard label="Auto repair active" value={summary.autoRepair} detail="CiteLoop is reducing manual edits" tone="amber" />
            <SummaryCard label="Needs human" value={summary.needsHuman} detail="Source or positioning decision needed" tone="red" />
          </div>

          <div className="grid min-h-[620px] xl:grid-cols-[minmax(0,1.08fr)_minmax(420px,0.92fr)]">
            <div className="min-w-0 border-b border-slate-200 xl:border-b-0 xl:border-r">
              <div className="hidden grid-cols-[76px_minmax(160px,1.35fr)_minmax(105px,0.7fr)_minmax(105px,0.7fr)_94px] gap-2.5 border-b border-slate-200 bg-slate-50 px-3.5 py-3 text-[11px] font-bold uppercase tracking-[0.12em] text-slate-500 lg:grid">
                <div>Status</div>
                <div>Article</div>
                <div>Claim evidence</div>
                <div>Search target</div>
                <div>Action</div>
              </div>
              <div className="divide-y divide-slate-200">
                {queueArticles.map((item) => (
                  <ReviewQueueRow
                    key={item.article.id}
                    item={item}
                    projectId={projectId}
                    selected={selectedArticle?.id === item.article.id}
                    busy={busy === item.article.id || busy === "bulk-approve"}
                    repairFailure={repairFailures[item.article.id]}
                    onSelect={() => setSelectedArticleId(item.article.id)}
                    onApprove={() => mutate("Article approved", item.article.id, () => api.approve(projectId, item.article.id))}
                  />
                ))}
              </div>
            </div>

            {selectedArticle && (
              <ReviewInspector
                article={selectedArticle}
                topicId={selectedQueueArticle?.topicId ?? selectedArticle.topic_id}
                projectId={projectId}
                busy={busy === selectedArticle.id || busy === "bulk-approve"}
                repairFailure={repairFailures[selectedArticle.id]}
                editorOpen={editorOpen}
                content={content}
                onContentChange={setContent}
                onToggleEditor={() => setEditorOpen((value) => !value)}
                onApprove={() => mutate("Article approved", selectedArticle.id, () => api.approve(projectId, selectedArticle.id))}
                onReject={() => mutate("Article rejected", selectedArticle.id, () => api.reject(projectId, selectedArticle.id))}
                onSave={(nextContent) =>
                  mutate("Content saved and QA refreshed", selectedArticle.id, () => api.edit(projectId, selectedArticle.id, { content_md: nextContent }))
                }
              />
            )}
          </div>
        </section>
      )}
    </div>
  );
}

function SummaryCard({
  label,
  value,
  detail,
  tone = "neutral",
}: {
  label: string;
  value: number;
  detail: string;
  tone?: "neutral" | "green" | "amber" | "red";
}) {
  const valueClass = {
    neutral: "text-slate-950",
    green: "text-green-700",
    amber: "text-amber-700",
    red: "text-red-700",
  }[tone];

  return (
    <div className="border-b border-slate-200 px-4 py-4 last:border-b-0 sm:border-r xl:border-b-0">
      <div className="text-[11px] font-bold uppercase tracking-[0.12em] text-slate-500">{label}</div>
      <div className={cx("mt-2 text-3xl font-bold leading-none", valueClass)}>{value}</div>
      <div className="mt-2 text-xs font-medium text-slate-500">{detail}</div>
    </div>
  );
}

function ReviewQueueRow({
  item,
  projectId,
  selected,
  busy,
  repairFailure,
  onSelect,
  onApprove,
}: {
  item: QueueArticle;
  projectId: string;
  selected: boolean;
  busy: boolean;
  repairFailure?: string;
  onSelect: () => void;
  onApprove: () => void;
}) {
  const { article, topicId } = item;
  const state = reviewArticleState(article);
  const title = articleReviewTitle(article);
  const claimRows = qaClaimRows(article);
  const mappedClaims = claimRows.filter((row) => row.mapped).length;
  const unmappedClaims = claimRows.filter((row) => !row.mapped).length;
  const keyword = searchAppearanceRows(article)[0]?.value ?? "Not specified";
  const detailHref = `/projects/${projectId}/articles/${article.id}`;
  const previewHref = articlePreviewHref(projectId, article);

  return (
    <article
      className={cx(
        "grid cursor-pointer gap-2.5 px-3.5 py-4 transition-colors hover:bg-slate-50 lg:grid-cols-[76px_minmax(160px,1.35fr)_minmax(105px,0.7fr)_minmax(105px,0.7fr)_94px]",
        selected && "bg-orange-50/80 shadow-[inset_3px_0_0_#d93820]",
      )}
      onClick={onSelect}
    >
      <div>
        <StateBadge state={state} />
        {repairFailure && <div className="mt-2 text-xs font-semibold text-red-700">Repair failed</div>}
      </div>
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <Badge tone={article.kind === "canonical" ? "green" : "neutral"}>{article.platform || article.kind}</Badge>
          <span className="text-xs font-semibold text-slate-400">Topic {topicId.slice(0, 8)}</span>
        </div>
        <button type="button" className="mt-2 block max-w-full text-left text-sm font-bold leading-5 text-slate-950" onClick={onSelect}>
          {title}
        </button>
        <div className="mt-2 flex flex-wrap gap-3 text-xs font-semibold text-slate-500">
          <span>geo {formatScore(article.geo_score)}</span>
          <span>seo {formatScore(article.seo_score)}</span>
          {article.repair_attempts > 0 && <span>repair {article.repair_attempts}/2</span>}
        </div>
      </div>
      <div className="flex flex-wrap gap-2">
        {claimRows.length > 0 ? (
          <>
            <Badge tone="green">{mappedClaims} mapped</Badge>
            {unmappedClaims > 0 && <Badge tone="red">{unmappedClaims} unmapped</Badge>}
          </>
        ) : article.qa_blocking ? (
          <Badge tone="amber">QA issue</Badge>
        ) : (
          <Badge tone="green">claims cleared</Badge>
        )}
      </div>
      <div className="min-w-0 text-xs leading-5 text-slate-600">
        <div className="font-bold uppercase tracking-[0.08em] text-slate-400">Keyword</div>
        <div className="truncate" title={keyword}>
          {keyword}
        </div>
      </div>
      <div className="flex flex-wrap items-start gap-1.5 lg:grid">
        {state.approvable ? (
          <Button
            disabled={busy}
            size="sm"
            variant="primary"
            onClick={(event) => {
              event.stopPropagation();
              onApprove();
            }}
          >
            <CheckCircle2 size={14} />
            Approve
          </Button>
        ) : (
          <Button
            disabled={busy}
            size="sm"
            onClick={(event) => {
              event.stopPropagation();
              onSelect();
            }}
          >
            {state.kind === "auto_repair" ? "Review" : "Resolve"}
          </Button>
        )}
        <a
          href={previewHref}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex h-8 items-center gap-1 rounded-lg px-1.5 text-xs font-semibold text-slate-600 hover:bg-slate-100 hover:text-slate-950"
          onClick={(event) => event.stopPropagation()}
        >
          Preview <ExternalLink size={12} />
        </a>
        <a
          href={detailHref}
          className="inline-flex h-8 items-center gap-1 rounded-lg px-1.5 text-xs font-semibold text-slate-600 hover:bg-slate-100 hover:text-slate-950"
          onClick={(event) => event.stopPropagation()}
        >
          Detail
        </a>
      </div>
    </article>
  );
}

function StateBadge({ state }: { state: ReviewArticleState }) {
  const tone = state.kind === "ready" ? "green" : state.kind === "auto_repair" ? "amber" : "red";
  return <Badge tone={tone}>{state.label}</Badge>;
}

function ReviewInspector({
  article,
  topicId,
  projectId,
  busy,
  repairFailure,
  editorOpen,
  content,
  onContentChange,
  onToggleEditor,
  onApprove,
  onReject,
  onSave,
}: {
  article: Article;
  topicId: string;
  projectId: string;
  busy: boolean;
  repairFailure?: string;
  editorOpen: boolean;
  content: string;
  onContentChange: (value: string) => void;
  onToggleEditor: () => void;
  onApprove: () => void;
  onReject: () => void;
  onSave: (content: string) => void;
}) {
  const title = articleReviewTitle(article);
  const state = reviewArticleState(article);
  const seoContributions = useMemo(() => buildSEOContributions(article), [article]);
  const repairExhausted = article.requires_human_decision || (article.repair_attempts ?? 0) >= 2;
  const blockingSummary = article.qa_blocking
    ? article.qa_issues.length > 0
      ? explainQAIssue(article.qa_issues[0]).title
      : "QA has not cleared this draft"
    : null;
  const previewHref = articlePreviewHref(projectId, article);
  const detailHref = `/projects/${projectId}/articles/${article.id}`;

  return (
    <aside className="min-w-0 bg-slate-50">
      <div className="border-b border-slate-200 bg-white px-4 py-4">
        <div className="flex flex-wrap items-center gap-2">
          <StateBadge state={state} />
          <Badge tone="neutral">Topic {topicId.slice(0, 8)}</Badge>
        </div>
        <h3 className="mt-3 content-font text-lg font-bold leading-6 text-slate-950">{title}</h3>
        <p className="mt-2 text-sm leading-6 text-slate-600">{state.detail}</p>
        {blockingSummary && (
          <div className="mt-3 flex items-start gap-2 rounded-md border border-red-100 bg-red-50 px-3 py-2 text-xs font-semibold leading-5 text-red-800">
            <ShieldAlert size={14} className="mt-0.5 shrink-0" />
            <span>Cannot approve: {blockingSummary}. See the details below to resolve it.</span>
          </div>
        )}
      </div>

      <div className="grid gap-4 p-4">
        {(state.kind === "auto_repair" || repairFailure) && <AutoRepairStatus state={state} error={repairFailure} />}
        <ClaimEvidencePanel article={article} />
        <SearchAppearancePanel article={article} />
        <NextStepPanel
          article={article}
          projectId={projectId}
          state={state}
          busy={busy}
          previewHref={previewHref}
          detailHref={detailHref}
          repairExhausted={repairExhausted}
          onApprove={onApprove}
          onReject={onReject}
          onToggleEditor={onToggleEditor}
        />

        {article.qa_issues.length > 0 && (
          <QAIssuePanel issues={article.qa_issues} contextHref={`/projects/${projectId}/context`} exhausted={repairExhausted} />
        )}

        <SEOContributionPanel rows={seoContributions} />

        {editorOpen ? (
          <OriginalArticlePanel content={content} busy={busy} onChange={onContentChange} onSave={onSave} />
        ) : (
          <section className="rounded-lg border border-slate-200 bg-white p-3">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <div className="text-sm font-bold text-slate-900">Draft content</div>
                <div className="mt-1 text-xs leading-5 text-slate-500">Open the editor only when a claim or source needs manual correction.</div>
              </div>
              <Button disabled={busy} size="sm" onClick={onToggleEditor}>
                <FileText size={14} />
                Edit draft
              </Button>
            </div>
          </section>
        )}
      </div>
    </aside>
  );
}

function ClaimEvidencePanel({ article }: { article: Article }) {
  const rows = qaClaimRows(article);
  return (
    <section className="rounded-lg border border-slate-200 bg-white p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="inline-flex items-center gap-2 text-sm font-bold text-slate-900">
          <ShieldAlert size={15} />
          Claim evidence map
        </div>
        {rows.length > 0 && <Badge tone={rows.some((row) => !row.mapped) ? "red" : "green"}>{rows.length} claims</Badge>}
      </div>
      {rows.length === 0 ? (
        <div className="mt-3 rounded-md border border-dashed border-slate-200 bg-slate-50 px-3 py-3 text-xs leading-5 text-slate-500">
          QA did not return a claim map for this draft. CiteLoop will rerun QA after an automatic repair or a saved edit.
        </div>
      ) : (
        <div className="mt-3 grid gap-2">
          {rows.map((row, index) => (
            <ClaimRow key={`${row.claim}-${index}`} row={row} />
          ))}
        </div>
      )}
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
      <div className="mt-2 text-xs leading-5 text-slate-600">
        {row.evidence || "No supporting evidence was returned for this claim."}
      </div>
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

function NextStepPanel({
  article,
  projectId,
  state,
  busy,
  previewHref,
  detailHref,
  repairExhausted,
  onApprove,
  onReject,
  onToggleEditor,
}: {
  article: Article;
  projectId: string;
  state: ReviewArticleState;
  busy: boolean;
  previewHref: string;
  detailHref: string;
  repairExhausted: boolean;
  onApprove: () => void;
  onReject: () => void;
  onToggleEditor: () => void;
}) {
  const options = article.human_decision_options.filter((option) => option?.label || option?.description);

  return (
    <section className="rounded-lg border border-slate-200 bg-white p-3">
      <div className="text-sm font-bold text-slate-900">Recommended next step</div>
      {state.kind === "ready" && (
        <p className="mt-1 text-xs leading-5 text-slate-500">This draft can be approved, or opened in preview/detail first for a final scan.</p>
      )}
      {state.kind === "auto_repair" && (
        <p className="mt-1 text-xs leading-5 text-slate-500">No manual action is needed while CiteLoop repairs and reruns QA.</p>
      )}
      {state.kind === "needs_human" && (
        <p className="mt-1 text-xs leading-5 text-slate-500">
          {repairExhausted
            ? "Automatic repair is exhausted. Add or fix the evidence in Context, edit the draft, or reject it."
            : "Choose whether to fix evidence, edit the draft, or reject the draft."}
        </p>
      )}

      {options.length > 0 && (
        <div className="mt-3 grid gap-2">
          {options.map((option, index) => (
            <div key={`${option.label ?? "option"}-${index}`} className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2">
              <div className="text-xs font-bold text-amber-950">{option.label || `Option ${index + 1}`}</div>
              {option.description && <div className="mt-1 text-xs leading-5 text-amber-900">{option.description}</div>}
            </div>
          ))}
        </div>
      )}

      <div className="mt-3 flex flex-wrap gap-2">
        {state.approvable && (
          <Button disabled={busy} size="sm" variant="primary" onClick={onApprove}>
            <CheckCircle2 size={14} />
            Approve
          </Button>
        )}
        <a
          href={previewHref}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex h-8 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50"
        >
          Preview <ExternalLink size={14} />
        </a>
        <a
          href={detailHref}
          className="inline-flex h-8 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50"
        >
          Detail
        </a>
        {state.kind === "needs_human" && (
          <>
            <a
              href={`/projects/${projectId}/context`}
              className="inline-flex h-8 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50"
            >
              Fix evidence in Context
            </a>
            <Button disabled={busy} size="sm" onClick={onToggleEditor}>
              <FileText size={14} />
              Edit draft
            </Button>
            <Button disabled={busy} size="sm" variant="danger" onClick={onReject}>
              <XCircle size={14} />
              Reject
            </Button>
          </>
        )}
      </div>
    </section>
  );
}

function AutoRepairStatus({ state, error }: { state: ReviewArticleState; error?: string }) {
  if (error) {
    return (
      <section className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-900">
        <div className="font-semibold">Automatic repair could not complete</div>
        <div className="mt-1 text-xs leading-5 text-red-800">{error}</div>
      </section>
    );
  }
  return (
    <section className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-950">
      <div className="font-semibold">{state.label}</div>
      <div className="mt-1 text-xs leading-5 text-amber-900">{state.detail}</div>
    </section>
  );
}

function OriginalArticlePanel({
  content,
  busy,
  onChange,
  onSave,
}: {
  content: string;
  busy: boolean;
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
            <Save size={14} />
            Save content
          </Button>
          <span className="text-xs text-slate-500">Content edits trigger backend re-QA. Metadata-only edits do not unlock blocking.</span>
        </div>
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
  const dotClass = {
    ready: "bg-green-500",
    missing: "bg-red-500",
    needs_review: "bg-amber-500",
  }[row.status];

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

function QAIssuePanel({ issues, contextHref, exhausted }: { issues: string[]; contextHref: string; exhausted: boolean }) {
  return (
    <section className="rounded-lg border border-red-200 bg-red-50 p-3 text-red-900">
      <div className="mb-2 inline-flex items-center gap-2 text-sm font-bold">
        <ShieldAlert size={15} />
        Why QA blocked this draft
      </div>
      <div className="grid gap-2">
        {issues.map((issue, index) => {
          const explained = explainQAIssue(issue);
          return (
            <div key={`${issue}-${index}`} className="rounded-md border border-red-100 bg-white/65 p-3">
              <div className="text-sm font-semibold">{explained.title}</div>
              <div className="mt-1 text-xs leading-5 text-red-800">{explained.detail}</div>
              <div className="mt-2 text-xs font-semibold text-red-900">
                {exhausted
                  ? "Automatic repair is exhausted. Add or fix the evidence in Context, or edit the draft below, then re-save to rerun QA."
                  : explained.action}
              </div>
              <a href={contextHref} className="mt-2 inline-flex items-center gap-1 text-xs font-semibold text-[#d93820] hover:underline">
                <ExternalLink size={12} />
                Fix evidence in Context
              </a>
              <div className="mt-2 break-words font-mono text-[11px] leading-4 text-red-700/70">{explained.raw}</div>
            </div>
          );
        })}
      </div>
    </section>
  );
}
