"use client";

import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { CheckCircle2, ExternalLink, Eye, FileText, RefreshCw, Save, Search, ShieldAlert, XCircle } from "lucide-react";
import { Article, ReviewGroup } from "../../../lib/api";
import {
  articlePreviewBlocks,
  articleReviewTitle,
  buildSEOContributions,
  explainQAIssue,
  previewPath,
  shouldAutoRepairArticle,
  type SEOContribution,
} from "../../../lib/review-insights";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, EmptyState, Notice, SectionHeader, TextArea, cx, formatScore } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;

export function ReviewClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [groups, setGroups] = useState<ReviewGroup[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [repairing, setRepairing] = useState<Record<string, boolean>>({});
  const [repairAttempted, setRepairAttempted] = useState<Record<string, boolean>>({});
  const [repairFailures, setRepairFailures] = useState<Record<string, string>>({});
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
        detail: isGate ? "The backend rejected approval because qa_blocking is still true." : e.message,
        tone: isGate ? "amber" : "red",
      });
    } finally {
      setBusy(null);
    }
  }

  const total = groups.reduce((sum, group) => sum + group.articles.length, 0);

  return (
    <div className="space-y-7">
      <SectionHeader
        title="Review queue"
        eyebrow="The only human publishing gate"
        action={
          <Button disabled={!!busy} size="sm" onClick={refresh}>
            <RefreshCw size={14} />
            Refresh
          </Button>
        }
      />
      {message && <Notice title={message.title} detail={message.detail} tone={message.tone} />}

      {total === 0 ? (
        <EmptyState title="Nothing pending review" detail="Writer and QA output will appear here before publishing." />
      ) : (
        <div className="grid gap-4">
          {groups.map((group) => (
            <section key={group.topic_id} className="rounded-xl border border-slate-200 bg-white p-4">
              <div className="mb-3 flex items-center justify-between">
                <div className="text-sm font-bold text-slate-900">Topic {group.topic_id.slice(0, 8)}</div>
                <Badge tone="neutral">{group.articles.length} articles</Badge>
              </div>
              <div className="grid gap-3">
                {group.articles.map((article) => (
                  <ReviewArticle
                    key={article.id}
                    article={article}
                    busy={busy === article.id}
                    repairing={!!repairing[article.id]}
                    repairFailure={repairFailures[article.id]}
                    onApprove={() => mutate("Article approved", article.id, () => api.approve(projectId, article.id))}
                    onReject={() => mutate("Article rejected", article.id, () => api.reject(projectId, article.id))}
                    onSave={(content) =>
                      mutate("Content saved and QA refreshed", article.id, () => api.edit(projectId, article.id, { content_md: content }))
                    }
                    detailHref={`/projects/${projectId}/articles/${article.id}`}
                  />
                ))}
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  );
}

function ReviewArticle({
  article,
  busy,
  repairing,
  repairFailure,
  onApprove,
  onReject,
  onSave,
  detailHref,
}: {
  article: Article;
  busy: boolean;
  repairing: boolean;
  repairFailure?: string;
  onApprove: () => void;
  onReject: () => void;
  onSave: (content: string) => void;
  detailHref: string;
}) {
  const [open, setOpen] = useState(false);
  const [content, setContent] = useState(article.content_md);
  const title = articleReviewTitle(article);
  const seoContributions = useMemo(() => buildSEOContributions(article), [article]);

  return (
    <article className="rounded-lg border border-slate-200 bg-white p-4">
      <div className="grid gap-4 border-b border-slate-100 pb-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start">
        <div className="min-w-0">
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <Badge tone={article.kind === "canonical" ? "green" : "neutral"}>{article.platform || article.kind}</Badge>
            {article.qa_blocking && <Badge tone="red">qa blocking</Badge>}
            <span className="text-xs font-semibold text-slate-400">
              geo {formatScore(article.geo_score)} / seo {formatScore(article.seo_score)}
            </span>
          </div>
          <h3 className="content-font text-[17px] font-semibold leading-6 text-slate-950">{title}</h3>
          <div className="mt-2 flex min-w-0 flex-wrap items-center gap-3 text-xs font-medium text-slate-500">
            <span className="max-w-full truncate">{previewPath(article)}</span>
            {article.canonical_url && (
              <a
                href={article.canonical_url}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 text-[#d93820]"
              >
                <ExternalLink size={12} />
                Published URL
              </a>
            )}
          </div>
        </div>
        <div className="flex flex-wrap gap-2 lg:justify-end">
          <a
            href={detailHref}
            className="inline-flex h-8 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50"
          >
            <FileText size={14} />
            Detail
          </a>
          <Button size="sm" onClick={() => setOpen((value) => !value)}>
            {open ? "Hide editor" : "Edit"}
          </Button>
          <Button disabled={busy || repairing || article.qa_blocking} size="sm" variant="primary" onClick={onApprove}>
            <CheckCircle2 size={14} />
            Approve
          </Button>
          <Button disabled={busy} size="sm" variant="danger" onClick={onReject}>
            <XCircle size={14} />
            Reject
          </Button>
        </div>
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(460px,0.95fr)] 2xl:grid-cols-[minmax(560px,1fr)_minmax(560px,1fr)]">
        <div className="min-w-0 space-y-4">
          {(repairing || repairFailure) && <AutoRepairStatus repairing={repairing} error={repairFailure} />}

          <OriginalArticlePanel
            content={content}
            editing={open}
            busy={busy}
            onChange={setContent}
            onSave={onSave}
          />

          {article.qa_issues.length > 0 && <QAIssuePanel issues={article.qa_issues} />}

          <SEOContributionPanel rows={seoContributions} />
        </div>

        <ArticleWebPreview article={article} />
      </div>
    </article>
  );
}

function AutoRepairStatus({ repairing, error }: { repairing: boolean; error?: string }) {
  if (repairing) {
    return (
      <section className="rounded-lg border border-blue-200 bg-blue-50 p-3 text-sm font-semibold text-blue-900">
        CiteLoop is automatically repairing this draft and rerunning QA.
      </section>
    );
  }
  if (!error) return null;
  return (
    <section className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-900">
      <div className="font-semibold">Automatic repair could not complete</div>
      <div className="mt-1 text-xs leading-5 text-red-800">{error}</div>
    </section>
  );
}

function OriginalArticlePanel({
  content,
  editing,
  busy,
  onChange,
  onSave,
}: {
  content: string;
  editing: boolean;
  busy: boolean;
  onChange: (value: string) => void;
  onSave: (content: string) => void;
}) {
  return (
    <section className="min-w-0 overflow-hidden rounded-lg border border-slate-200 bg-slate-50">
      <div className="flex items-center gap-2 border-b border-slate-200 px-3 py-2 text-xs text-slate-500">
        <FileText size={14} />
        <span className="font-semibold text-slate-700">Original Markdown</span>
      </div>
      <div className="bg-white p-3">
        {editing ? (
          <div className="grid gap-2">
            <TextArea value={content} onChange={(event) => onChange(event.target.value)} className="min-h-[560px] font-mono text-xs leading-5" />
            <div className="flex flex-wrap items-center gap-3">
              <Button disabled={busy} size="sm" variant="primary" onClick={() => onSave(content)}>
                <Save size={14} />
                Save content
              </Button>
              <span className="text-xs text-slate-500">
                Content edits trigger backend re-QA. Metadata-only edits do not unlock blocking.
              </span>
            </div>
          </div>
        ) : (
          <pre className="max-h-[720px] overflow-auto whitespace-pre-wrap break-words rounded-md bg-white font-mono text-xs leading-6 text-slate-700">{content || "No article body available."}</pre>
        )}
      </div>
    </section>
  );
}

function SEOContributionPanel({ rows }: { rows: SEOContribution[] }) {
  const ready = rows.filter((row) => row.status === "ready").length;
  const missing = rows.filter((row) => row.status === "missing").length;
  const needsReview = rows.filter((row) => row.status === "needs_review").length;

  return (
    <section className="rounded-lg border border-slate-200 bg-slate-50 p-3">
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
    <div className="rounded-md border border-slate-200 bg-white p-3">
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

function QAIssuePanel({ issues }: { issues: string[] }) {
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
              <div className="mt-2 text-xs font-semibold text-red-900">{explained.action}</div>
              <div className="mt-2 break-words font-mono text-[11px] leading-4 text-red-700/70">{explained.raw}</div>
            </div>
          );
        })}
      </div>
    </section>
  );
}

function ArticleWebPreview({ article }: { article: Article }) {
  const title = articleReviewTitle(article);
  const description = stringMeta(article.seo_meta, "meta_description");
  const h1 = stringMeta(article.seo_meta, "h1") || title;
  const blocks = articlePreviewBlocks(article.content_md, h1);
  const path = previewPath(article);

  return (
    <aside className="overflow-hidden rounded-lg border border-slate-200 bg-slate-50">
      <div className="flex items-center gap-2 border-b border-slate-200 px-3 py-2 text-xs text-slate-500">
        <Eye size={14} />
        <span className="shrink-0 font-semibold text-slate-700">Web preview</span>
        <span className="truncate">{article.canonical_url || path}</span>
      </div>
      <div className="bg-white p-5">
        <div className="border-b border-slate-100 pb-4">
          <div className="text-xs font-bold uppercase tracking-[0.12em] text-[#d93820]">UniPost Blog</div>
          {description && <p className="mt-2 content-font text-sm leading-6 text-slate-600">{description}</p>}
        </div>
        <div className="content-font mt-4 text-[15px] leading-7 text-slate-800">
          {blocks.length === 0 ? (
            <div className="rounded-md border border-dashed border-slate-200 px-3 py-4 text-sm text-slate-500">No article body available.</div>
          ) : (
            blocks.map((block, index) => <MarkdownPreviewBlock key={`${block.slice(0, 30)}-${index}`} block={block} />)
          )}
        </div>
      </div>
    </aside>
  );
}

function stringMeta(meta: Record<string, any>, key: string) {
  const value = meta?.[key];
  return typeof value === "string" ? value.trim() : "";
}

function MarkdownPreviewBlock({ block }: { block: string }) {
  if (block.startsWith("```")) {
    return <pre className="mb-4 overflow-hidden rounded-md bg-slate-950 px-3 py-2 font-mono text-xs leading-5 text-slate-100">{block.replace(/```/g, "").trim()}</pre>;
  }

  if (block.startsWith("# ")) {
    return <h1 className="mb-4 text-2xl font-bold leading-8 text-slate-950">{renderInline(block.slice(2))}</h1>;
  }

  if (block.startsWith("## ")) {
    return <h3 className="mb-2 mt-4 text-lg font-bold leading-7 text-slate-950">{renderInline(block.slice(3))}</h3>;
  }

  if (block.startsWith("### ")) {
    return <h4 className="mb-2 mt-4 text-base font-bold leading-6 text-slate-950">{renderInline(block.slice(4))}</h4>;
  }

  const lines = block.split("\n").map((line) => line.trim());
  if (lines.every((line) => /^[-*]\s+/.test(line))) {
    return (
      <ul className="mb-4 list-disc space-y-1 pl-5">
        {lines.map((line, index) => (
          <li key={`${line}-${index}`}>{renderInline(line.replace(/^[-*]\s+/, ""))}</li>
        ))}
      </ul>
    );
  }

  return (
    <p className="mb-4">
      {block.split("\n").map((line, index) => (
        <span key={`${line}-${index}`}>
          {index > 0 && <br />}
          {renderInline(line.trim())}
        </span>
      ))}
    </p>
  );
}

function renderInline(text: string): ReactNode[] {
  const parts = text.split(/(\*\*[^*]+\*\*|`[^`]+`|\[[^\]]+\]\([^)]+\))/g).filter(Boolean);
  return parts.map((part, index) => {
    if (part.startsWith("**") && part.endsWith("**")) {
      return <strong key={`${part}-${index}`}>{part.slice(2, -2)}</strong>;
    }
    if (part.startsWith("`") && part.endsWith("`")) {
      return (
        <code key={`${part}-${index}`} className="rounded bg-slate-100 px-1 py-0.5 font-mono text-[0.9em]">
          {part.slice(1, -1)}
        </code>
      );
    }
    const link = part.match(/^\[([^\]]+)\]\(([^)]+)\)$/);
    if (link) {
      return (
        <a key={`${part}-${index}`} href={link[2]} target="_blank" rel="noopener noreferrer" className="font-semibold text-[#d93820]">
          {link[1]}
        </a>
      );
    }
    return <span key={`${part}-${index}`}>{part}</span>;
  });
}
