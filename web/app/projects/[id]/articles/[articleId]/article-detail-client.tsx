"use client";

import { useCallback, useEffect, useState } from "react";
import { ExternalLink, RefreshCw } from "lucide-react";
import { Article } from "../../../../lib/api";
import { useApi } from "../../../../lib/use-api";
import { Badge, Button, EmptyState, Field, Notice, SectionHeader, TextArea, formatDate, formatScore } from "../../../../components/ui";

function articleTitle(article: Article) {
  return article.seo_meta?.title || article.seo_meta?.slug || `${article.kind} article`;
}

export function ArticleDetailClient({ projectId, articleId }: { projectId: string; articleId: string }) {
  const api = useApi();
  const [article, setArticle] = useState<Article | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setArticle(await api.getArticle(projectId, articleId));
    } catch (e: any) {
      setError(e.message ?? "Failed to load article");
    } finally {
      setLoading(false);
    }
  }, [api, articleId, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  if (loading) return <EmptyState title="Loading article" detail="Fetching article details." />;
  if (error) return <Notice title="Article unavailable" detail={error} tone="red" />;
  if (!article) return <EmptyState title="Article not found" detail="The article is missing or belongs to another project." />;

  return (
    <div className="space-y-7">
      <SectionHeader
        title={articleTitle(article)}
        eyebrow="Article detail"
        action={
          <Button size="sm" onClick={refresh}>
            <RefreshCw size={14} />
            Refresh
          </Button>
        }
      />

      <section className="grid gap-3 rounded-xl border border-slate-200 bg-white p-4">
        <div className="flex flex-wrap items-center gap-2">
          <Badge tone={article.status === "publish_failed" ? "red" : article.status === "published" ? "green" : "neutral"}>
            {article.status}
          </Badge>
          <Badge tone={article.kind === "canonical" ? "green" : "blue"}>{article.platform || article.kind}</Badge>
          {article.qa_blocking && <Badge tone="red">qa blocking</Badge>}
        </div>
        <div className="grid gap-2 text-sm text-slate-600 sm:grid-cols-3">
          <div>geo {formatScore(article.geo_score)}</div>
          <div>seo {formatScore(article.seo_score)}</div>
          <div>created {formatDate(article.created_at)}</div>
          <div>scheduled {formatDate(article.scheduled_at)}</div>
          <div>reviewed {formatDate(article.reviewed_at)}</div>
          <div>published {formatDate(article.published_at)}</div>
        </div>
        {article.canonical_url && (
          <a href={article.canonical_url} target="_blank" rel="noopener noreferrer" className="inline-flex w-fit items-center gap-2 text-sm font-semibold text-[#d93820]">
            <ExternalLink size={14} />
            Canonical URL
          </a>
        )}
        {article.last_publish_error && <Notice title="Last publish error" detail={article.last_publish_error} tone="red" />}
      </section>

      <section className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
        <Field label="SEO metadata">
          <TextArea readOnly className="min-h-[180px] font-mono text-xs" value={JSON.stringify(article.seo_meta, null, 2)} />
        </Field>
        <Field label="Content">
          <TextArea readOnly className="min-h-[420px] content-font text-[15px] leading-6" value={article.content_md} />
        </Field>
      </section>
    </div>
  );
}
