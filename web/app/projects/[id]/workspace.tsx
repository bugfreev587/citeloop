"use client";

import { useCallback, useEffect, useState } from "react";
import { api, Article, DistributeItem, ReviewGroup, Topic } from "../../lib/api";

function num(n: any): string {
  // pgtype.Numeric serializes as an object; show a short form when present.
  if (n == null) return "–";
  if (typeof n === "number") return n.toFixed(2);
  if (n.Valid === false) return "–";
  return "set";
}

function Badge({ children, tone = "neutral" }: { children: React.ReactNode; tone?: string }) {
  const tones: Record<string, string> = {
    neutral: "bg-neutral-100 text-neutral-700",
    green: "bg-green-100 text-green-700",
    red: "bg-red-100 text-red-700",
    amber: "bg-amber-100 text-amber-800",
    blue: "bg-blue-100 text-blue-700",
  };
  return <span className={`rounded px-2 py-0.5 text-xs font-medium ${tones[tone]}`}>{children}</span>;
}

export function Workspace({ projectId }: { projectId: string }) {
  const [landing, setLanding] = useState("");
  const [topics, setTopics] = useState<Topic[]>([]);
  const [review, setReview] = useState<ReviewGroup[]>([]);
  const [published, setPublished] = useState<Article[]>([]);
  const [ready, setReady] = useState<DistributeItem[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    const [t, r, p, d] = await Promise.all([
      api.listTopics(projectId).catch(() => []),
      api.listReview(projectId).catch(() => []),
      api.listArticles(projectId, "published").catch(() => []),
      api.listDistribute(projectId).catch(() => []),
    ]);
    setTopics(t);
    setReview(r);
    setPublished(p);
    setReady(d);
  }, [projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const run = async (label: string, fn: () => Promise<any>) => {
    setBusy(label);
    setMsg(null);
    try {
      await fn();
      await refresh();
      setMsg(`${label} ✓`);
    } catch (e: any) {
      setMsg(`${label} failed: ${e.message}`);
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="space-y-8">
      {msg && <div className="rounded border bg-white px-4 py-2 text-sm">{msg}</div>}

      {/* Pipeline controls */}
      <section className="rounded-lg border bg-white p-5">
        <h2 className="mb-3 font-semibold">Pipeline</h2>
        <div className="flex flex-wrap items-center gap-2">
          <input
            value={landing}
            onChange={(e) => setLanding(e.target.value)}
            placeholder="https://landing-page-url/"
            className="w-72 rounded border px-3 py-1.5 text-sm"
          />
          <button
            disabled={!!busy || !landing}
            onClick={() => run("Insight", () => api.runInsight(projectId, landing))}
            className="rounded bg-neutral-900 px-3 py-1.5 text-sm text-white disabled:opacity-40"
          >
            Run Insight (crawl + profile)
          </button>
          <button
            disabled={!!busy}
            onClick={() => run("Strategist", () => api.runStrategist(projectId))}
            className="rounded bg-neutral-900 px-3 py-1.5 text-sm text-white disabled:opacity-40"
          >
            Run Strategist (topics)
          </button>
          <button
            disabled={!!busy}
            onClick={() => run("Generate tick", () => api.tickGenerate(projectId))}
            className="rounded border px-3 py-1.5 text-sm"
          >
            Generate tick
          </button>
          <button
            disabled={!!busy}
            onClick={() => run("Publish tick", () => api.tickPublish(projectId))}
            className="rounded border px-3 py-1.5 text-sm"
          >
            Publish tick
          </button>
        </div>
      </section>

      {/* Topic backlog */}
      <section className="rounded-lg border bg-white p-5">
        <h2 className="mb-3 font-semibold">Topic backlog ({topics.length})</h2>
        <div className="space-y-1.5">
          {topics.map((t) => (
            <div key={t.id} className="flex items-center justify-between rounded border px-3 py-2 text-sm">
              <div className="flex items-center gap-2">
                <Badge tone="blue">{t.channel}</Badge>
                <span>{t.title}</span>
                <Badge>{t.status}</Badge>
              </div>
              <button
                disabled={!!busy}
                onClick={() => run("Generate", () => api.generateTopic(projectId, t.id))}
                className="rounded border px-2 py-1 text-xs"
              >
                Generate
              </button>
            </div>
          ))}
          {topics.length === 0 && <div className="text-sm text-neutral-500">No topics. Run Strategist.</div>}
        </div>
      </section>

      {/* Review queue — the only human gate */}
      <section className="rounded-lg border bg-white p-5">
        <h2 className="mb-3 font-semibold">Review queue ({review.length} topics)</h2>
        <p className="mb-3 text-xs text-neutral-500">
          The only human gate. Articles with blocking evidence issues must be resolved before approval.
        </p>
        <div className="space-y-4">
          {review.map((g) => (
            <div key={g.topic_id} className="rounded border p-3">
              {g.articles.map((a) => (
                <ArticleCard key={a.id} a={a} busy={!!busy} onAction={run} />
              ))}
            </div>
          ))}
          {review.length === 0 && <div className="text-sm text-neutral-500">Nothing pending review.</div>}
        </div>
      </section>

      {/* Published + distribution */}
      <div className="grid gap-6 md:grid-cols-2">
        <section className="rounded-lg border bg-white p-5">
          <h2 className="mb-3 font-semibold">Published ({published.length})</h2>
          {published.map((a) => (
            <div key={a.id} className="border-b py-2 text-sm last:border-0">
              <div>{a.seo_meta?.title || a.kind}</div>
              <a href={a.canonical_url || "#"} className="text-xs text-blue-600 underline">
                {a.canonical_url}
              </a>
            </div>
          ))}
          {published.length === 0 && <div className="text-sm text-neutral-500">None yet.</div>}
        </section>

        <section className="rounded-lg border bg-white p-5">
          <h2 className="mb-3 font-semibold">Ready to distribute ({ready.length})</h2>
          <p className="mb-2 text-xs text-neutral-500">
            Variants unlock only after the canonical is published (§5.6).
          </p>
          {ready.map(({ article: a, compose_url, supports_canonical }) => (
            <div key={a.id} className="border-b py-2 text-sm last:border-0">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Badge tone="amber">{a.platform}</Badge>
                  <span className="text-xs text-neutral-500">
                    {supports_canonical ? "canonical tag" : "source link in body"}
                  </span>
                </div>
                <button
                  onClick={() => run("Distributed", () => api.distributed(a.id))}
                  className="rounded border px-2 py-1 text-xs"
                >
                  Mark distributed
                </button>
              </div>
              <div className="mt-1.5 flex items-center gap-2">
                <button
                  onClick={() => navigator.clipboard?.writeText(a.content_md)}
                  className="rounded border px-2 py-1 text-xs"
                >
                  Copy variant
                </button>
                {compose_url && (
                  <a
                    href={compose_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="rounded border px-2 py-1 text-xs text-blue-700"
                  >
                    Open {a.platform} compose page ↗
                  </a>
                )}
                <span className="text-xs text-neutral-400">canonical: {a.canonical_url}</span>
              </div>
            </div>
          ))}
          {ready.length === 0 && <div className="text-sm text-neutral-500">None ready.</div>}
        </section>
      </div>
    </div>
  );
}

function ArticleCard({
  a,
  busy,
  onAction,
}: {
  a: Article;
  busy: boolean;
  onAction: (label: string, fn: () => Promise<any>) => Promise<void>;
}) {
  const [content, setContent] = useState(a.content_md);
  const [open, setOpen] = useState(false);
  return (
    <div className="border-b py-3 last:border-0">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Badge tone={a.kind === "canonical" ? "green" : "neutral"}>
            {a.kind}
            {a.platform ? `:${a.platform}` : ""}
          </Badge>
          {a.qa_blocking && <Badge tone="red">qa blocking</Badge>}
          <span className="text-xs text-neutral-500">
            geo {num(a.geo_score)} · seo {num(a.seo_score)}
          </span>
        </div>
        <div className="flex gap-2">
          <button onClick={() => setOpen((o) => !o)} className="rounded border px-2 py-1 text-xs">
            {open ? "Hide" : "Edit"}
          </button>
          <button
            disabled={busy || a.qa_blocking}
            onClick={() => onAction("Approve", () => api.approve(a.id))}
            className="rounded bg-green-600 px-2 py-1 text-xs text-white disabled:opacity-40"
          >
            Approve
          </button>
          <button
            disabled={busy}
            onClick={() => onAction("Reject", () => api.reject(a.id))}
            className="rounded border border-red-300 px-2 py-1 text-xs text-red-700"
          >
            Reject
          </button>
        </div>
      </div>
      {a.qa_issues && a.qa_issues.length > 0 && (
        <ul className="mt-2 list-disc pl-5 text-xs text-red-600">
          {a.qa_issues.map((i, idx) => (
            <li key={idx}>{i}</li>
          ))}
        </ul>
      )}
      {open && (
        <div className="mt-2 space-y-2">
          <textarea
            value={content}
            onChange={(e) => setContent(e.target.value)}
            className="h-48 w-full rounded border p-2 font-mono text-xs"
          />
          <div className="flex items-center gap-3">
            <button
              disabled={busy}
              onClick={() => onAction("Save", () => api.edit(a.id, { content_md: content }))}
              className="rounded bg-neutral-900 px-3 py-1 text-xs text-white"
            >
              Save content
            </button>
            <span className="text-xs text-neutral-500">
              Saving re-runs QA; blocking clears only if claims now map to evidence.
            </span>
          </div>
        </div>
      )}
    </div>
  );
}
