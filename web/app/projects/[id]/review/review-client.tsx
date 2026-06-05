"use client";

import { useCallback, useEffect, useState } from "react";
import { CheckCircle2, RefreshCw, Save, XCircle } from "lucide-react";
import { api, Article, ReviewGroup } from "../../../lib/api";
import { Badge, Button, EmptyState, Notice, SectionHeader, TextArea, formatScore } from "../../../components/ui";

type Message = { title: string; detail?: string; tone: "neutral" | "red" | "green" | "amber" } | null;

function articleTitle(article: Article) {
  return article.seo_meta?.title || article.seo_meta?.slug || `${article.kind} draft`;
}

export function ReviewClient({ projectId }: { projectId: string }) {
  const [groups, setGroups] = useState<ReviewGroup[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message>(null);

  const refresh = useCallback(async () => {
    try {
      setGroups(await api.listReview(projectId));
    } catch (e: any) {
      setMessage({ title: "Review queue unavailable", detail: e.message, tone: "amber" });
    }
  }, [projectId]);

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
                    onApprove={() => mutate("Article approved", article.id, () => api.approve(article.id))}
                    onReject={() => mutate("Article rejected", article.id, () => api.reject(article.id))}
                    onSave={(content) =>
                      mutate("Content saved and QA refreshed", article.id, () => api.edit(article.id, { content_md: content }))
                    }
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
  onApprove,
  onReject,
  onSave,
}: {
  article: Article;
  busy: boolean;
  onApprove: () => void;
  onReject: () => void;
  onSave: (content: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [content, setContent] = useState(article.content_md);

  return (
    <article className="rounded-lg border border-slate-200 bg-white px-4 py-3">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <Badge tone={article.kind === "canonical" ? "green" : "neutral"}>
              {article.platform || article.kind}
            </Badge>
            {article.qa_blocking && <Badge tone="red">qa blocking</Badge>}
            <span className="text-xs font-semibold text-slate-400">
              geo {formatScore(article.geo_score)} / seo {formatScore(article.seo_score)}
            </span>
          </div>
          <h3 className="content-font text-[15px] font-semibold leading-5 text-slate-900">{articleTitle(article)}</h3>
          <p className="mt-2 line-clamp-3 content-font text-[15px] leading-5 text-slate-700">{article.content_md}</p>
        </div>
        <div className="flex shrink-0 flex-wrap gap-2">
          <Button size="sm" onClick={() => setOpen((value) => !value)}>
            {open ? "Hide" : "Edit"}
          </Button>
          <Button disabled={busy || article.qa_blocking} size="sm" variant="primary" onClick={onApprove}>
            <CheckCircle2 size={14} />
            Approve
          </Button>
          <Button disabled={busy} size="sm" variant="danger" onClick={onReject}>
            <XCircle size={14} />
            Reject
          </Button>
        </div>
      </div>

      {article.qa_issues.length > 0 && (
        <div className="mt-3 rounded-lg border border-red-100 bg-red-50 px-3 py-2 text-xs text-red-700">
          <div className="font-bold">Blocking evidence issues</div>
          <ul className="mt-1 list-disc pl-4">
            {article.qa_issues.map((issue, index) => (
              <li key={`${issue}-${index}`}>{issue}</li>
            ))}
          </ul>
        </div>
      )}

      {open && (
        <div className="mt-3 grid gap-2">
          <TextArea value={content} onChange={(event) => setContent(event.target.value)} className="min-h-[280px] font-mono text-xs" />
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
      )}
    </article>
  );
}
