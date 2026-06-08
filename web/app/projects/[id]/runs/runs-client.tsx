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
        <div className="overflow-hidden rounded-lg border border-slate-200 bg-white">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-slate-200 bg-slate-50 text-xs uppercase text-slate-500">
              <tr>
                <th className="px-4 py-3 font-semibold">Agent</th>
                <th className="px-4 py-3 font-semibold">Status</th>
                <th className="px-4 py-3 font-semibold">Cost</th>
                <th className="px-4 py-3 font-semibold">Model</th>
                <th className="px-4 py-3 font-semibold">Created</th>
                <th className="px-4 py-3 font-semibold">Error</th>
                <th className="px-4 py-3 font-semibold">Action</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {runs.map((run) => (
                <tr key={run.id} className="transition-colors hover:bg-slate-50">
                  <td className="px-4 py-3 font-medium text-slate-900">
                    <div className="flex flex-wrap items-center gap-2">
                      {run.agent}
                      {run.output?.degraded && <Badge tone="amber">degraded</Badge>}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <Badge tone={statusTone(run.status)}>{run.status}</Badge>
                  </td>
                  <td className="px-4 py-3 text-slate-600">{money(run.cost_usd)}</td>
                  <td className="px-4 py-3 text-slate-600">
                    {run.model ?? "-"}
                    {run.tokens != null && <span className="ml-2 text-xs text-slate-400">{run.tokens} tokens</span>}
                  </td>
                  <td className="px-4 py-3 text-slate-600">{formatDate(run.created_at)}</td>
                  <td className="max-w-[280px] truncate px-4 py-3 text-slate-500">{run.error ?? "-"}</td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-2">
                      <a
                        href={`/projects/${projectId}/runs/${run.id}`}
                        className="inline-flex items-center gap-1 text-xs font-semibold text-[#d93820]"
                      >
                        Detail
                        <ExternalLink size={12} />
                      </a>
                      {run.next_actions[0] && (
                        <a href={run.next_actions[0].href} className="inline-flex items-center gap-1 text-xs font-semibold text-slate-500">
                          {run.next_actions[0].label}
                        </a>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
