"use client";

import { useCallback, useEffect, useState } from "react";
import type { ReactNode } from "react";
import { CheckCircle2, Eye, FileText, RefreshCw, Save, ShieldAlert, Sparkles, XCircle } from "lucide-react";
import { Article, ReviewGroup, Topic } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, EmptyState, Notice, SectionHeader, TextArea, cx, formatScore } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;

function articleTitle(article: Article) {
  return article.seo_meta?.title || article.seo_meta?.slug || `${article.kind} draft`;
}

function articlePath(article: Article) {
  const slug = article.seo_meta?.slug || article.resolved_slug;
  return slug ? `/blog/${slug}` : "/blog/draft";
}

function qaTone(status: string) {
  if (status === "passed") return "green";
  if (status === "needs_human_decision") return "amber";
  if (status === "parse_failed" || status === "blocking") return "red";
  return "neutral";
}

function qaLabel(article: Article) {
  if (article.qa_status === "passed" && !article.qa_blocking) return "qa passed";
  if (article.qa_status === "parse_failed") return "qa parse failed";
  if (article.qa_status === "needs_human_decision") return "needs decision";
  return "qa blocking";
}

export function ReviewClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [groups, setGroups] = useState<ReviewGroup[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
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
        detail: isGate ? "QA has not passed yet. Run AI fix or choose a human decision." : e.message,
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
        <div className="grid gap-5">
          {groups.map((group) => (
            <section key={group.topic_id} className="rounded-xl border border-slate-200 bg-white p-4">
              <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
                <div>
                  <div className="text-sm font-bold text-slate-900">{group.topic?.title || `Topic ${group.topic_id.slice(0, 8)}`}</div>
                  <div className="mt-1 text-xs font-medium text-slate-500">
                    {group.topic?.target_keyword ? `Keyword: ${group.topic.target_keyword}` : "Keyword: AI inferred from topic title"}
                  </div>
                </div>
                <Badge tone="neutral">{group.articles.length} articles</Badge>
              </div>
              <div className="grid gap-4">
                {group.articles.map((article) => (
                  <ReviewArticle
                    key={article.id}
                    article={article}
                    topic={group.topic}
                    busy={busy === article.id}
                    onApprove={() => mutate("Article approved", article.id, () => api.approve(projectId, article.id))}
                    onReject={() => mutate("Article rejected", article.id, () => api.reject(projectId, article.id))}
                    onAIFix={() => mutate("AI fix completed and QA refreshed", article.id, () => api.aiFix(projectId, article.id))}
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
  topic,
  busy,
  onApprove,
  onReject,
  onAIFix,
  onSave,
  detailHref,
}: {
  article: Article;
  topic?: Topic;
  busy: boolean;
  onApprove: () => void;
  onReject: () => void;
  onAIFix: () => void;
  onSave: (content: string) => void;
  detailHref: string;
}) {
  const [editing, setEditing] = useState(false);
  const [content, setContent] = useState(article.content_md);
  const passed = article.qa_status === "passed" && !article.qa_blocking;
  const attemptsLeft = Math.max(0, 3 - article.qa_attempt_count);

  useEffect(() => {
    setContent(article.content_md);
  }, [article.content_md, article.id]);

  return (
    <article className="overflow-hidden rounded-lg border border-slate-200 bg-slate-50 p-4">
      <div className="mb-4 flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <Badge tone={article.kind === "canonical" ? "green" : "neutral"}>{article.platform || article.kind}</Badge>
            <Badge tone={qaTone(article.qa_status)}>{qaLabel(article)}</Badge>
            <span className="text-xs font-semibold text-slate-400">
              geo {formatScore(article.geo_score)} / seo {formatScore(article.seo_score)}
            </span>
          </div>
          <h3 className="content-font text-lg font-semibold leading-6 text-slate-950">{articleTitle(article)}</h3>
          <div className="mt-1 break-all text-xs font-medium text-slate-500">{articlePath(article)}</div>
        </div>
        <div className="flex w-full flex-wrap gap-2 lg:w-auto lg:justify-end">
          <a href={detailHref} className="inline-flex h-8 min-w-[92px] flex-1 items-center justify-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50 sm:flex-none">
            <FileText size={14} />
            Detail
          </a>
          <Button className="min-w-[92px] flex-1 sm:flex-none" size="sm" onClick={() => setEditing((value) => !value)}>
            {editing ? "Hide edit" : "Edit"}
          </Button>
          <Button className="min-w-[92px] flex-1 sm:flex-none" disabled={busy || passed || attemptsLeft === 0} size="sm" onClick={onAIFix}>
            <Sparkles size={14} />
            AI fix
          </Button>
          <Button className="min-w-[92px] flex-1 sm:flex-none" disabled={busy || !passed} size="sm" variant="primary" onClick={onApprove}>
            <CheckCircle2 size={14} />
            Approve
          </Button>
          <Button className="min-w-[92px] flex-1 sm:flex-none" disabled={busy} size="sm" variant="danger" onClick={onReject}>
            <XCircle size={14} />
            Reject
          </Button>
        </div>
      </div>

      <div className="grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1.05fr)_minmax(480px,0.95fr)]">
        <section className="min-w-0 overflow-hidden rounded-lg border border-slate-200 bg-white">
          <PanelHeader icon={<FileText size={15} />} title={editing ? "Editable markdown" : "Original markdown"} meta={`QA attempts ${article.qa_attempt_count}/3`} />
          <div className="p-4">
            {editing ? (
              <div className="grid gap-3">
                <TextArea value={content} onChange={(event) => setContent(event.target.value)} className="min-h-[560px] font-mono text-xs leading-5" />
                <div className="flex flex-wrap items-center gap-3">
                  <Button disabled={busy} size="sm" variant="primary" onClick={() => onSave(content)}>
                    <Save size={14} />
                    Save and rerun QA
                  </Button>
                  <span className="text-xs text-slate-500">Manual edits preserve the same gate: QA must pass before approval.</span>
                </div>
              </div>
            ) : (
              <pre data-testid="article-markdown-scroll" className="max-h-[72vh] overflow-auto whitespace-pre-wrap rounded-md bg-slate-950 p-4 font-mono text-xs leading-5 text-slate-100 xl:max-h-[760px] 2xl:max-h-[860px]">
                {article.content_md}
              </pre>
            )}
          </div>
        </section>

        <div className="grid min-w-0 content-start gap-4">
          <section className="min-w-0 overflow-hidden rounded-lg border border-slate-200 bg-white">
            <PanelHeader icon={<Eye size={15} />} title="Web preview" meta={articlePath(article)} />
            <MarkdownPreview article={article} />
          </section>

          <SEOContribution article={article} topic={topic} />
          <QAStatus article={article} />
        </div>
      </div>
    </article>
  );
}

function PanelHeader({ icon, title, meta }: { icon: ReactNode; title: string; meta?: string }) {
  return (
    <div className="flex min-h-11 min-w-0 items-center gap-2 border-b border-slate-200 px-4 text-sm font-semibold text-slate-700">
      <span className="shrink-0">{icon}</span>
      <span className="shrink-0">{title}</span>
      {meta && <span className="min-w-0 truncate text-xs font-medium text-slate-400">{meta}</span>}
    </div>
  );
}

function SEOContribution({ article, topic }: { article: Article; topic?: Topic }) {
  const title = String(article.seo_meta?.title || "");
  const meta = String(article.seo_meta?.meta_description || "");
  const slug = String(article.seo_meta?.slug || "");
  const keyword = topic?.target_keyword || topic?.title || "";
  const items = [
    {
      label: "Search intent",
      title: keyword || "AI must infer keyword",
      detail: keyword ? "Draft is mapped to the topic intent." : "No target keyword is stored on this topic.",
      ready: Boolean(keyword),
    },
    {
      label: "SERP title",
      title: title || "Missing title",
      detail: title ? `${title.length} characters.` : "The clickable result headline is missing.",
      ready: Boolean(title),
    },
    {
      label: "Meta description",
      title: meta || "Missing meta description",
      detail: meta ? `${meta.length} characters.` : "Search result summary is missing.",
      ready: Boolean(meta),
    },
    {
      label: "URL slug",
      title: slug || "Missing slug",
      detail: slug ? articlePath(article) : "Publish path cannot be previewed.",
      ready: Boolean(slug),
    },
  ];
  const ready = items.filter((item) => item.ready).length;

  return (
    <section className="rounded-lg border border-slate-200 bg-white p-4">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-sm font-bold text-slate-900">
          <Eye size={16} />
          SEO contribution
        </div>
        <Badge tone={ready === items.length ? "green" : "amber"}>{ready}/{items.length} ready</Badge>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {items.map((item) => (
          <div key={item.label} className="rounded-lg border border-slate-200 bg-slate-50 p-3">
            <div className="mb-2 flex items-center gap-2 text-[11px] font-bold uppercase tracking-[0.18em] text-slate-500">
              <span className={cx("h-2.5 w-2.5 rounded-full", item.ready ? "bg-green-500" : "bg-amber-500")} />
              {item.label}
            </div>
            <div className="line-clamp-2 text-sm font-semibold text-slate-950">{item.title}</div>
            <div className="mt-1 text-xs leading-5 text-slate-600">{item.detail}</div>
          </div>
        ))}
      </div>
    </section>
  );
}

function QAStatus({ article }: { article: Article }) {
  const issues = article.qa_issues.length ? article.qa_issues : article.qa_failure_message ? [article.qa_failure_message] : [];

  return (
    <section className={cx("rounded-lg border p-4", article.qa_status === "passed" ? "border-green-200 bg-green-50" : "border-red-200 bg-red-50")}>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-sm font-bold text-slate-950">
          <ShieldAlert size={16} />
          QA gate
        </div>
        <Badge tone={qaTone(article.qa_status)}>{qaLabel(article)}</Badge>
      </div>
      {article.qa_failure_kind && (
        <div className="mb-2 text-xs font-bold uppercase tracking-[0.16em] text-slate-500">{article.qa_failure_kind}</div>
      )}
      {article.qa_failure_message && <p className="mb-3 text-sm font-semibold leading-5 text-slate-900">{article.qa_failure_message}</p>}
      {issues.length > 0 && (
        <ul className="grid gap-2 text-sm leading-5 text-slate-800">
          {issues.map((issue, index) => (
            <li key={`${issue}-${index}`} className="rounded-md bg-white/80 px-3 py-2">
              {issue}
            </li>
          ))}
        </ul>
      )}
      {article.qa_human_options.length > 0 && (
        <div className="mt-3 rounded-md border border-amber-200 bg-white px-3 py-2">
          <div className="mb-1 text-xs font-bold uppercase tracking-[0.16em] text-amber-700">Human choices</div>
          <ul className="grid gap-1 text-sm leading-5 text-slate-700">
            {article.qa_human_options.map((option) => (
              <li key={option}>{option}</li>
            ))}
          </ul>
        </div>
      )}
      {article.qa_attempt_count >= 3 && article.qa_status !== "passed" && (
        <div className="mt-3 text-xs font-semibold text-red-700">AI fix budget exhausted for this draft.</div>
      )}
    </section>
  );
}

function MarkdownPreview({ article }: { article: Article }) {
  const heading = article.seo_meta?.h1 || article.seo_meta?.title || articleTitle(article);
  const meta = article.seo_meta?.meta_description;
  const lines = article.content_md.split("\n");

  return (
    <div data-testid="article-preview-scroll" className="max-h-[72vh] overflow-auto bg-white p-5 xl:max-h-[760px] xl:p-6 2xl:max-h-[860px]">
      <div className="mb-5 border-b border-slate-100 pb-5">
        <div className="mb-3 text-xs font-bold uppercase tracking-[0.18em] text-[#d93820]">Unipost Blog</div>
        <h1 className="content-font text-2xl font-bold leading-tight text-slate-950 sm:text-3xl">{heading}</h1>
        {meta && <p className="mt-4 content-font text-base leading-7 text-slate-600">{meta}</p>}
      </div>
      <div className="grid gap-3 content-font text-[15px] leading-7 text-slate-800">
        {lines.map((line, index) => renderMarkdownLine(line, index))}
      </div>
    </div>
  );
}

function renderMarkdownLine(line: string, index: number) {
  const trimmed = line.trim();
  if (!trimmed) return <div key={index} className="h-2" />;
  if (trimmed.startsWith("# ")) return null;
  if (trimmed.startsWith("## ")) {
    return (
      <h2 key={index} className="mt-4 text-2xl font-bold leading-8 text-slate-950">
        {cleanMarkdown(trimmed.slice(3))}
      </h2>
    );
  }
  if (trimmed.startsWith("### ")) {
    return (
      <h3 key={index} className="mt-3 text-xl font-bold leading-7 text-slate-950">
        {cleanMarkdown(trimmed.slice(4))}
      </h3>
    );
  }
  if (trimmed.startsWith("- ")) {
    return (
      <div key={index} className="flex gap-2">
        <span className="mt-3 h-1.5 w-1.5 shrink-0 rounded-full bg-slate-400" />
        <span>{cleanMarkdown(trimmed.slice(2))}</span>
      </div>
    );
  }
  if (trimmed.startsWith("```")) return null;
  return <p key={index}>{cleanMarkdown(trimmed)}</p>;
}

function cleanMarkdown(value: string) {
  return value.replace(/\*\*/g, "").replace(/`/g, "");
}
