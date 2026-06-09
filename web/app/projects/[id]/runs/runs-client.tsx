"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Activity, AlertTriangle, CheckCircle2, RefreshCw } from "lucide-react";
import { GenerationRun } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, EmptyState, Notice, SectionHeader, formatDate } from "../../../components/ui";

function statusTone(status: string, degraded = false): "green" | "red" | "amber" | "neutral" {
  if (status === "ok" && !degraded) return "green";
  if (status === "error" || status === "failed") return "red";
  if (degraded || status === "running" || status === "budget_stopped") return "amber";
  return "neutral";
}

function money(value: number | null) {
  if (value == null) return "-";
  return `$${value.toFixed(4)}`;
}

function isAttention(run: GenerationRun) {
  return ["error", "failed", "budget_stopped"].includes(run.status) || Boolean(run.output?.degraded);
}

function activityLabel(agent: string) {
  const labels: Record<string, string> = {
    insight: "Context refreshed",
    strategist: "Content plan updated",
    writer: "Draft created",
    qa: "Draft needs evidence",
    publisher: "Publishing needs attention",
    notification: "Notification delivery",
  };
  return labels[agent] ?? "Automation activity";
}

function userImpact(run: GenerationRun) {
  if (run.status === "budget_stopped") return "Automation paused because the project hit its budget guardrail.";
  if (run.agent === "publisher" && ["error", "failed"].includes(run.status)) {
    return "Publishing or variant unlock may be blocked until the canonical URL is confirmed.";
  }
  if (run.agent === "qa" && ["error", "failed"].includes(run.status)) {
    return "A draft cannot be approved until evidence or claim safety is resolved.";
  }
  if (run.output?.degraded) return "Output quality may be limited; review the affected content before relying on it.";
  if (["error", "failed"].includes(run.status)) return "A background automation step failed and may need a retry or configuration fix.";
  return "No user action required.";
}

function nextAction(run: GenerationRun) {
  if (run.status === "budget_stopped") return "Review budget settings before starting more automation.";
  if (run.agent === "publisher" && ["error", "failed"].includes(run.status)) return "Open Publish and check the publishing connection or URL verification.";
  if (run.agent === "qa" && ["error", "failed"].includes(run.status)) return "Open Review and inspect the blocked draft evidence.";
  if (run.output?.degraded) return "Open the related workflow and verify the generated result.";
  if (["error", "failed"].includes(run.status)) return "Retry the workflow after checking configuration.";
  return "Keep this record for audit history.";
}

function AdvancedDetails({ run }: { run: GenerationRun }) {
  return (
    <details className="mt-3 rounded-lg border border-slate-100 bg-slate-50 px-3 py-2">
      <summary className="cursor-pointer text-xs font-bold uppercase text-slate-500">Advanced details</summary>
      <div className="mt-3 grid gap-2 text-xs text-slate-600 sm:grid-cols-2">
        <div>
          <span className="font-semibold text-slate-900">Automation</span> {run.agent}
        </div>
        <div>
          <span className="font-semibold text-slate-900">Run ID</span> {run.id}
        </div>
        <div>
          <span className="font-semibold text-slate-900">Model used</span> {run.model ?? "-"}
        </div>
        <div>
          <span className="font-semibold text-slate-900">Tokens</span> {run.tokens ?? "-"}
        </div>
        <div>
          <span className="font-semibold text-slate-900">Estimated spend</span> {money(run.cost_usd)}
        </div>
        <div>
          <span className="font-semibold text-slate-900">Created</span> {formatDate(run.created_at)}
        </div>
        {run.error && <div className="sm:col-span-2"><span className="font-semibold text-slate-900">Raw error</span> {run.error}</div>}
      </div>
    </details>
  );
}

function ActivityRow({ run }: { run: GenerationRun }) {
  const degraded = Boolean(run.output?.degraded);
  return (
    <div className="rounded-xl border border-slate-200 bg-white px-4 py-3">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-bold text-slate-900">{activityLabel(run.agent)}</span>
            <Badge tone={statusTone(run.status, degraded)}>{run.status}</Badge>
            {degraded && <Badge tone="amber">degraded</Badge>}
          </div>
          <div className="mt-1 text-xs font-semibold text-slate-400">{formatDate(run.created_at)}</div>
        </div>
      </div>
      <div className="mt-3 grid gap-3 text-sm md:grid-cols-2">
        <div>
          <div className="text-xs font-bold uppercase text-slate-400">User impact</div>
          <p className="mt-1 leading-5 text-slate-600">{userImpact(run)}</p>
        </div>
        <div>
          <div className="text-xs font-bold uppercase text-slate-400">Next action</div>
          <p className="mt-1 leading-5 text-slate-600">{nextAction(run)}</p>
        </div>
      </div>
      <AdvancedDetails run={run} />
    </div>
  );
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
      setError(e.message ?? "Failed to load activity");
    } finally {
      setLoading(false);
    }
  }, [api, projectId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const summary = useMemo(() => {
    const attention = runs.filter(isAttention);
    const successful = runs.filter((run) => !isAttention(run));
    const degraded = runs.filter((run) => Boolean(run.output?.degraded));
    return { attention, successful, degraded };
  }, [runs]);

  return (
    <div className="space-y-7">
      <SectionHeader
        title="Activity Log"
        eyebrow="Automation audit"
        action={
          <Button size="sm" onClick={refresh} disabled={loading}>
            <RefreshCw size={14} />
            Refresh
          </Button>
        }
      />

      {error && <Notice title="Activity Log unavailable" detail={error} tone="red" />}

      <div className="grid gap-3 md:grid-cols-3">
        <div className="rounded-lg border border-slate-200 bg-white p-4">
          <AlertTriangle className="mb-3 text-slate-400" size={18} />
          <div className="text-sm font-bold text-slate-900">{summary.attention.length}</div>
          <p className="mt-1 text-sm leading-5 text-slate-500">Needs attention</p>
        </div>
        <div className="rounded-lg border border-slate-200 bg-white p-4">
          <CheckCircle2 className="mb-3 text-slate-400" size={18} />
          <div className="text-sm font-bold text-slate-900">{summary.successful.length}</div>
          <p className="mt-1 text-sm leading-5 text-slate-500">Recent successful activity</p>
        </div>
        <div className="rounded-lg border border-slate-200 bg-white p-4">
          <Activity className="mb-3 text-slate-400" size={18} />
          <div className="text-sm font-bold text-slate-900">{summary.degraded.length}</div>
          <p className="mt-1 text-sm leading-5 text-slate-500">Limited quality events</p>
        </div>
      </div>

      {loading ? (
        <EmptyState title="Loading activity" detail="Fetching recent automation records." />
      ) : runs.length === 0 ? (
        <EmptyState title="No activity yet" detail="Context refreshes, plan updates, draft creation, review checks, publishing, and notifications will appear here." />
      ) : (
        <>
          <section>
            <SectionHeader title="Needs attention" action={<Badge tone={summary.attention.length ? "amber" : "green"}>{summary.attention.length}</Badge>} />
            {summary.attention.length === 0 ? (
              <EmptyState title="No attention events" detail="Failed, degraded, and budget-stopped automation will appear here." />
            ) : (
              <div className="grid gap-3">
                {summary.attention.map((run) => (
                  <ActivityRow key={run.id} run={run} />
                ))}
              </div>
            )}
          </section>

          <section>
            <details className="rounded-xl border border-slate-200 bg-white p-4">
              <summary className="cursor-pointer text-sm font-bold text-slate-900">
                Recent successful activity ({summary.successful.length})
              </summary>
              <div className="mt-4 grid gap-3">
                {summary.successful.length === 0 ? (
                  <div className="text-sm text-slate-500">No successful activity in the current window.</div>
                ) : (
                  summary.successful.map((run) => <ActivityRow key={run.id} run={run} />)
                )}
              </div>
            </details>
          </section>
        </>
      )}
    </div>
  );
}
