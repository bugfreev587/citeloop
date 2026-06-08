"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { ArrowLeft, ExternalLink, RefreshCw } from "lucide-react";
import { GenerationRun } from "../../../../lib/api";
import { useApi } from "../../../../lib/use-api";
import { Badge, Button, EmptyState, Notice, SectionHeader, formatDate } from "../../../../components/ui";

function statusTone(status: string): "green" | "red" | "amber" | "neutral" {
  if (status === "ok") return "green";
  if (status === "error" || status === "failed") return "red";
  if (status === "running") return "amber";
  return "neutral";
}

function money(value: number | null) {
  if (value == null) return "-";
  return `$${value.toFixed(4)}`;
}

function JSONBlock({ title, value }: { title: string; value: Record<string, any> | null }) {
  return (
    <section>
      <SectionHeader title={title} />
      {!value || Object.keys(value).length === 0 ? (
        <EmptyState title={`No ${title.toLowerCase()}`} detail="This run did not capture structured data for this section." />
      ) : (
        <pre className="max-h-[460px] overflow-auto rounded-lg border border-slate-200 bg-slate-950 p-4 text-xs leading-5 text-slate-100">
          {JSON.stringify(value, null, 2)}
        </pre>
      )}
    </section>
  );
}

export function RunDetailClient({ projectId, runId }: { projectId: string; runId: string }) {
  const api = useApi();
  const [run, setRun] = useState<GenerationRun | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setRun(await api.getRun(projectId, runId));
    } catch (e: any) {
      setError(e.message ?? "Failed to load run");
    } finally {
      setLoading(false);
    }
  }, [api, projectId, runId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const title = useMemo(() => {
    if (!run) return "Run detail";
    return `${run.agent} ${run.status}`;
  }, [run]);

  return (
    <div className="space-y-7">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <a href={`/projects/${projectId}/runs`} className="inline-flex items-center gap-2 text-sm font-semibold text-slate-500 hover:text-slate-900">
          <ArrowLeft size={16} />
          Back to Runs
        </a>
        <Button size="sm" onClick={refresh} disabled={loading}>
          <RefreshCw size={14} />
          Refresh
        </Button>
      </div>

      {error && <Notice title="Run unavailable" detail={error} tone="red" />}

      {loading && !run ? (
        <EmptyState title="Loading run" detail="Fetching the automation record." />
      ) : run ? (
        <>
          <section className="rounded-xl border border-slate-200 bg-white p-4">
            <div className="flex flex-wrap items-start justify-between gap-4">
              <div>
                <div className="text-sm font-semibold text-slate-500">Automation audit</div>
                <h1 className="mt-1 text-2xl font-bold leading-8 text-slate-950">{title}</h1>
                <div className="mt-2 break-all text-xs font-semibold text-slate-400">{run.id}</div>
              </div>
              <Badge tone={statusTone(run.status)}>{run.status}</Badge>
            </div>
            <div className="mt-4 grid gap-3 text-sm md:grid-cols-4">
              <div>
                <div className="font-semibold text-slate-400">Created</div>
                <div className="mt-1 text-slate-800">{formatDate(run.created_at)}</div>
              </div>
              <div>
                <div className="font-semibold text-slate-400">Model</div>
                <div className="mt-1 text-slate-800">{run.model ?? "-"}</div>
              </div>
              <div>
                <div className="font-semibold text-slate-400">Tokens</div>
                <div className="mt-1 text-slate-800">{run.tokens ?? 0}</div>
              </div>
              <div>
                <div className="font-semibold text-slate-400">Cost</div>
                <div className="mt-1 text-slate-800">{money(run.cost_usd)}</div>
              </div>
            </div>
            {run.error && <Notice title="Failure reason" detail={run.error} tone="red" />}
          </section>

          {(run.related_links.length > 0 || run.next_actions.length > 0) && (
            <section>
              <SectionHeader title="Handling" />
              <div className="grid gap-3 md:grid-cols-2">
                {run.related_links.map((link) => (
                  <a key={`${link.kind}-${link.href}`} href={link.href} className="rounded-lg border border-slate-200 bg-white px-4 py-3 text-sm hover:bg-slate-50">
                    <div className="flex items-center justify-between gap-3">
                      <span className="font-bold text-slate-900">{link.label}</span>
                      <ExternalLink size={14} className="text-slate-400" />
                    </div>
                    <div className="mt-1 text-xs font-semibold text-slate-400">{link.kind}</div>
                  </a>
                ))}
                {run.next_actions.map((link) => (
                  <a key={`${link.kind}-${link.href}`} href={link.href} className="rounded-lg border border-slate-200 bg-white px-4 py-3 text-sm hover:bg-slate-50">
                    <div className="flex items-center justify-between gap-3">
                      <span className="font-bold text-slate-900">{link.label}</span>
                      <ExternalLink size={14} className="text-slate-400" />
                    </div>
                    <div className="mt-1 text-xs font-semibold text-slate-400">next action</div>
                  </a>
                ))}
              </div>
            </section>
          )}

          <JSONBlock title="Input" value={run.input} />
          <JSONBlock title="Output" value={run.output} />
        </>
      ) : null}
    </div>
  );
}
