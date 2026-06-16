"use client";

import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import { ExternalLink, RefreshCw } from "lucide-react";
import { Article, Project } from "../../../../../lib/api";
import { useApi } from "../../../../../lib/use-api";
import { articleReviewTitle, previewPath, publishedPreviewParts } from "../../../../../lib/review-insights";
import { Badge, Button, EmptyState, Notice } from "../../../../../components/ui";

function textValue(value: any) {
  return typeof value === "string" ? value.trim() : "";
}

function displayHostname(project: Project | null, article: Article) {
  const source = textValue(article.canonical_url) || textValue(project?.config?.site_url);
  if (source) {
    try {
      return new URL(source).hostname.replace(/^www\./, "");
    } catch {
      return source.replace(/^https?:\/\//, "").replace(/\/$/, "");
    }
  }
  return project?.name || "CiteLoop";
}

function articleDisplayPath(article: Article) {
  return textValue(article.publish_path) || previewPath(article);
}

function renderInline(text: string): ReactNode[] {
  const parts: ReactNode[] = [];
  const pattern = /(\*\*([^*]+)\*\*|`([^`]+)`|\[([^\]]+)\]\(([^)]+)\))/g;
  let cursor = 0;
  let match: RegExpExecArray | null;

  while ((match = pattern.exec(text)) !== null) {
    if (match.index > cursor) parts.push(text.slice(cursor, match.index));
    if (match[2]) {
      parts.push(
        <strong key={`${match.index}-strong`} className="font-bold text-slate-950">
          {match[2]}
        </strong>,
      );
    } else if (match[3]) {
      parts.push(
        <code key={`${match.index}-code`} className="rounded bg-slate-100 px-1 py-0.5 font-mono text-[0.92em] text-slate-800">
          {match[3]}
        </code>,
      );
    } else if (match[4] && match[5]) {
      parts.push(
        <a key={`${match.index}-link`} href={match[5]} className="font-semibold text-[#d93820] underline-offset-4 hover:underline">
          {match[4]}
        </a>,
      );
    }
    cursor = match.index + match[0].length;
  }

  if (cursor < text.length) parts.push(text.slice(cursor));
  return parts;
}

function renderLines(text: string) {
  const lines = text.split("\n");
  return lines.flatMap((line, index) => {
    const content = renderInline(line);
    return index === lines.length - 1 ? content : [...content, <br key={`${index}-br`} />];
  });
}

function MarkdownBlock({ block, index }: { block: string; index: number }) {
  const trimmed = block.trim();
  const heading = trimmed.match(/^(#{1,3})\s+(.+)$/);
  if (heading) {
    const level = heading[1].length;
    const copy = heading[2].trim();
    if (level === 1) {
      return <h1 className="content-font text-[42px] font-extrabold leading-[1.08] text-slate-950 md:text-[56px]">{renderInline(copy)}</h1>;
    }
    if (level === 2) {
      return <h2 className="content-font mt-10 text-2xl font-bold leading-8 text-slate-950 md:text-3xl">{renderInline(copy)}</h2>;
    }
    return <h3 className="content-font mt-8 text-xl font-bold leading-7 text-slate-950">{renderInline(copy)}</h3>;
  }

  if (trimmed.startsWith("```")) {
    const code = trimmed.replace(/^```[a-zA-Z0-9_-]*\n?/, "").replace(/\n?```$/, "");
    return (
      <pre className="overflow-x-auto rounded-lg border border-slate-200 bg-slate-950 p-4 text-sm leading-6 text-slate-100">
        <code>{code}</code>
      </pre>
    );
  }

  const lines = trimmed.split("\n").map((line) => line.trim()).filter(Boolean);
  if (lines.length > 0 && lines.every((line) => /^[-*]\s+/.test(line))) {
    return (
      <ul className="grid list-disc gap-2 pl-6 text-[17px] leading-8 text-slate-700">
        {lines.map((line, itemIndex) => (
          <li key={`${index}-${itemIndex}`}>{renderInline(line.replace(/^[-*]\s+/, ""))}</li>
        ))}
      </ul>
    );
  }

  if (lines.length > 0 && lines.every((line) => /^\d+\.\s+/.test(line))) {
    return (
      <ol className="grid list-decimal gap-2 pl-6 text-[17px] leading-8 text-slate-700">
        {lines.map((line, itemIndex) => (
          <li key={`${index}-${itemIndex}`}>{renderInline(line.replace(/^\d+\.\s+/, ""))}</li>
        ))}
      </ol>
    );
  }

  if (lines.length > 0 && lines.every((line) => /^>\s?/.test(line))) {
    return (
      <blockquote className="border-l-4 border-[#d93820] pl-4 text-xl font-semibold leading-8 text-slate-800">
        {renderLines(lines.map((line) => line.replace(/^>\s?/, "")).join("\n"))}
      </blockquote>
    );
  }

  return <p className="text-[17px] leading-8 text-slate-700">{renderLines(trimmed)}</p>;
}

function LoadingPreview() {
  return (
    <main className="min-h-[100dvh] bg-white px-5 py-8">
      <div className="mx-auto max-w-[760px]">
        <EmptyState title="Loading preview" detail="Fetching the article." />
      </div>
    </main>
  );
}

export function ArticlePublishedPreviewClient({ projectId, articleId }: { projectId: string; articleId: string }) {
  const api = useApi();
  const [article, setArticle] = useState<Article | null>(null);
  const [project, setProject] = useState<Project | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [nextArticle, nextProject] = await Promise.all([
        api.getArticle(projectId, articleId),
        api.getProject(projectId).catch(() => null),
      ]);
      setArticle(nextArticle);
      setProject(nextProject);
    } catch (e: any) {
      setError(e.message ?? "Failed to load preview");
    } finally {
      setLoading(false);
    }
  }, [api, articleId, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const preview = useMemo(() => {
    if (!article) return null;
    const h1 = textValue(article.seo_meta?.h1) || articleReviewTitle(article);
    const published = publishedPreviewParts(article.content_md, h1);
    return {
      title: published.title,
      description: textValue(article.seo_meta?.meta_description),
      hostname: displayHostname(project, article),
      path: articleDisplayPath(article),
      blocks: published.blocks,
    };
  }, [article, project]);

  if (loading) return <LoadingPreview />;

  if (error) {
    return (
      <main className="min-h-[100dvh] bg-white px-5 py-8">
        <div className="mx-auto max-w-[760px]">
          <Notice title="Preview unavailable" detail={error} tone="red" />
        </div>
      </main>
    );
  }

  if (!article || !preview) {
    return (
      <main className="min-h-[100dvh] bg-white px-5 py-8">
        <div className="mx-auto max-w-[760px]">
          <EmptyState title="Article not found" detail="The article is missing or belongs to another project." />
        </div>
      </main>
    );
  }

  return (
    <main className="min-h-[100dvh] bg-white text-slate-950">
      <header className="border-b border-slate-200 bg-white/90 backdrop-blur">
        <div className="mx-auto flex max-w-[980px] flex-wrap items-center justify-between gap-3 px-5 py-4">
          <div className="min-w-0">
            <div className="truncate text-sm font-bold text-slate-950">{preview.hostname}</div>
            <div className="mt-0.5 truncate text-xs font-medium text-slate-500">{preview.path}</div>
          </div>
          <div className="flex items-center gap-2">
            <Badge tone={article.status === "published" ? "green" : "neutral"}>{article.status === "published" ? "Published" : "Draft preview"}</Badge>
            <Button size="sm" onClick={refresh}>
              <RefreshCw size={14} />
              Refresh
            </Button>
            {article.canonical_url && (
              <a
                href={article.canonical_url}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex h-8 items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-700 hover:bg-slate-50"
              >
                Live URL <ExternalLink size={14} />
              </a>
            )}
          </div>
        </div>
      </header>

      <article className="mx-auto max-w-[760px] px-5 py-12 md:py-16">
        <h1 className="content-font text-[42px] font-extrabold leading-[1.08] text-slate-950 md:text-[56px]">{renderInline(preview.title)}</h1>
        {preview.description && <p className="mt-5 text-xl leading-8 text-slate-600">{preview.description}</p>}
        <div className="grid gap-6">
          {preview.blocks.map((block, index) => (
            <MarkdownBlock key={`${index}-${block.slice(0, 24)}`} block={block} index={index} />
          ))}
        </div>
      </article>
    </main>
  );
}
