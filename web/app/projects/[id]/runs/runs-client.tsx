"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Activity, AlertTriangle, ExternalLink, RefreshCw } from "lucide-react";
import { GenerationRun } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, EmptyState, Notice, SectionHeader, formatDate } from "../../../components/ui";

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

export function RunsClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const [runs, setRuns] = useState<GenerationRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setRuns(await api.listRuns(projectId, { limit: 50 }));
    } catch (e: any) {
      setError(e.message ?? "Failed to load runs");
    } finally {
      setLoading(false);
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const summary = useMemo(() => {
    const failed = runs.filter((run) => run.status === "error" || run.status === "failed").length;
    const degraded = runs.filter((run) => Boolean(run.output?.degraded)).length;
    const spend = runs.reduce((sum, run) => sum + (run.cost_usd ?? 0), 0);
    return { failed, degraded, spend };
  }, [runs]);

  return (
    <div className="space-y-7">
      <SectionHeader
        title="Runs"
        eyebrow="Automation audit"
        action={
          <Button size="sm" onClick={refresh} disabled={loading}>
            <RefreshCw size={14} />
            Refresh
          </Button>
        }
      />

      {error && <Notice title="Runs unavailable" detail={error} tone="red" />}

      <div className="grid gap-3 md:grid-cols-3">
        <div className="rounded-lg border border-slate-200 bg-white p-4">
          <Activity className="mb-3 text-slate-400" size={18} />
          <div className="text-sm font-bold text-slate-900">{runs.length}</div>
          <p className="mt-1 text-sm leading-5 text-slate-500">Recent runs</p>
        </div>
        <div className="rounded-lg border border-slate-200 bg-white p-4">
          <AlertTriangle className="mb-3 text-slate-400" size={18} />
          <div className="text-sm font-bold text-slate-900">{summary.failed}</div>
          <p className="mt-1 text-sm leading-5 text-slate-500">Failures</p>
        </div>
        <div className="rounded-lg border border-slate-200 bg-white p-4">
          <Activity className="mb-3 text-slate-400" size={18} />
          <div className="text-sm font-bold text-slate-900">{money(summary.spend)}</div>
          <p className="mt-1 text-sm leading-5 text-slate-500">{summary.degraded} degraded</p>
        </div>
      </div>

      {loading ? (
        <EmptyState title="Loading runs" detail="Fetching recent automation records." />
      ) : runs.length === 0 ? (
        <EmptyState title="No run data yet" detail="Insight, Strategist, Writer, QA, Publisher, and Notification runs will appear here." />
      ) : (
        <div className="grid gap-2">
          {runs.map((run) => (
            <div key={run.id} className="rounded-lg border border-slate-200 bg-white px-4 py-3 text-sm transition-colors hover:bg-slate-50">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-bold text-slate-900">{run.agent}</span>
                    <Badge tone={statusTone(run.status)}>{run.status}</Badge>
                    {run.output?.degraded && <Badge tone="amber">degraded</Badge>}
                  </div>
                  <div className="mt-1 break-all text-xs font-semibold text-slate-400">{run.id}</div>
                </div>
                <div className="flex shrink-0 flex-wrap gap-2">
                  <a href={`/projects/${projectId}/runs/${run.id}`} className="inline-flex items-center gap-1 text-xs font-semibold text-[#d93820]">
                    Detail
                    <ExternalLink size={12} />
                  </a>
                  {run.next_actions[0] && (
                    <a href={run.next_actions[0].href} className="inline-flex items-center gap-1 text-xs font-semibold text-slate-500">
                      {run.next_actions[0].label}
                    </a>
                  )}
                </div>
              </div>
              <div className="mt-3 grid gap-2 text-xs font-semibold text-slate-500 sm:grid-cols-4">
                <span>{money(run.cost_usd)}</span>
                <span>{run.tokens ?? 0} tokens</span>
                <span>{formatDate(run.created_at)}</span>
                <span className="truncate">{run.model ?? "-"}</span>
              </div>
              {run.error && <div className="mt-3 rounded-md border border-red-100 bg-red-50 px-3 py-2 text-sm text-red-800">{run.error}</div>}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
