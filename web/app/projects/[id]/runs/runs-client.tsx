"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Activity, AlertTriangle, ArrowRight, CheckCircle2, Clipboard, ExternalLink, RefreshCw, Sparkles } from "lucide-react";
import { GenerationRun } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { RightDrawer } from "../../../components/right-drawer";
import { Badge, Button, EmptyState, Notice, SectionHeader, cx, formatDate } from "../../../components/ui";

type FixStep = {
  title: string;
  detail: string;
};

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

function readableValue(value: unknown) {
  if (value == null) return "";
  if (typeof value === "string") return value.trim();
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  try {
    return JSON.stringify(value);
  } catch {
    return "";
  }
}

function rawError(run: GenerationRun) {
  const output = run.output ?? {};
  const candidates = [
    run.error,
    output.error,
    output.raw_error,
    output.message,
    output.detail,
    output.reason,
    output.failure,
    output.failure_reason,
  ];
  const found = candidates.map(readableValue).find(Boolean);
  if (found) return found;
  if (run.status === "budget_stopped") return "Budget guardrail stopped this automation before it could finish.";
  if (run.output?.degraded) return "The run completed with degraded output quality, but no raw error was recorded.";
  return "No raw error was recorded for this automation run.";
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

function OperationsHealth() {
  return (
    <section>
      <SectionHeader title="Operations health" eyebrow="Diagnostics" action={<Badge tone="neutral">Operational blockers</Badge>} />
      <div className="grid gap-3 lg:grid-cols-[1.08fr_0.96fr_0.96fr]">
        <div className="rounded-lg border border-slate-200 bg-white p-4">
          <Badge tone="amber">Operational blockers</Badge>
          <p className="mt-3 text-sm leading-6 text-slate-600">
            Budget, publisher, quality, notification, and degraded automation signals live here so product pages stay focused on decisions and impact.
          </p>
        </div>
        <div className="rounded-lg border border-slate-200 bg-white p-4">
          <Badge tone="blue">Diagnostics</Badge>
          <p className="mt-3 text-sm leading-6 text-slate-600">
            Run details remain available in the drawer for audit and debugging without replacing the main product outputs.
          </p>
        </div>
        <div className="rounded-lg border border-slate-200 bg-white p-4">
          <Badge tone="green">Next action</Badge>
          <p className="mt-3 text-sm leading-6 text-slate-600">
            Attention cards explain user impact first, then point back to Context, Review, Publish, Results, or Settings.
          </p>
        </div>
      </div>
    </section>
  );
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
  if (run.agent === "publisher" && ["error", "failed"].includes(run.status)) return "Open publisher settings and verify the connected destination.";
  if (run.agent === "qa" && ["error", "failed"].includes(run.status)) return "Open Review and inspect the blocked draft evidence.";
  if (run.agent === "notification" && ["error", "failed"].includes(run.status)) return "Open notification settings and test the webhook again.";
  if (run.agent === "insight" && ["error", "failed"].includes(run.status)) return "Open Context and rerun the product refresh after checking crawl inputs.";
  if (run.output?.degraded) return "Open the related workflow and verify the generated result.";
  if (["error", "failed"].includes(run.status)) return "Retry the workflow after checking configuration.";
  return "Keep this record for audit history.";
}

function fixDestination(projectId: string, run: GenerationRun) {
  if (run.status === "budget_stopped") return `/projects/${projectId}/settings#automation`;
  if (run.agent === "publisher") return `/projects/${projectId}/settings#publisher`;
  if (run.agent === "qa") return `/projects/${projectId}/review`;
  if (run.agent === "notification") return `/projects/${projectId}/settings#notifications`;
  if (run.agent === "insight") return `/projects/${projectId}/context`;
  if (run.agent === "strategist" || run.agent === "writer") return `/projects/${projectId}/plan`;
  return `/projects/${projectId}/settings`;
}

function fixActionLabel(run: GenerationRun) {
  if (run.status === "budget_stopped") return "Open automation settings";
  if (run.agent === "publisher") return "Open publisher settings";
  if (run.agent === "qa") return "Open Review";
  if (run.agent === "notification") return "Open notification settings";
  if (run.agent === "insight") return "Open Context";
  if (run.agent === "strategist" || run.agent === "writer") return "Open Content Plan";
  return "Open Settings";
}

function fixGuidance(run: GenerationRun): FixStep[] {
  if (run.status === "budget_stopped") {
    return [
      {
        title: "Check the monthly budget guardrail",
        detail: "Confirm whether this project should stay paused or whether the budget limit should be raised before more automation runs.",
      },
      {
        title: "Restart only the blocked workflow",
        detail: "After the budget setting is reviewed, rerun the affected workflow instead of restarting unrelated automation.",
      },
    ];
  }

  if (run.agent === "publisher") {
    return [
      {
        title: "Verify the publisher connection",
        detail: "Confirm the repository, branch, content directory, token access, and live base URL in publisher settings.",
      },
      {
        title: "Retry from Publish",
        detail: "Once the connection is healthy, retry the failed canonical publish or URL verification from the Publish surface.",
      },
    ];
  }

  if (run.agent === "qa") {
    return [
      {
        title: "Inspect the blocked claim",
        detail: "Open Review and check whether the draft needs stronger source evidence, safer phrasing, or a targeted rewrite.",
      },
      {
        title: "Keep the fix scoped",
        detail: "Repair only the failing evidence or claim safety issue, then recheck the draft before approving it.",
      },
    ];
  }

  if (run.agent === "notification") {
    return [
      {
        title: "Test the webhook",
        detail: "Open notification settings, confirm the webhook URL is valid, and send a test event.",
      },
      {
        title: "Re-enable failed subscriptions",
        detail: "If delivery is dead, replace the endpoint or create a new channel before retrying notification delivery.",
      },
    ];
  }

  if (run.agent === "insight") {
    return [
      {
        title: "Check crawl inputs",
        detail: "Open Context and confirm the project domain, crawl scope, robots access, and product facts are still valid.",
      },
      {
        title: "Refresh context",
        detail: "After fixing the input, rerun the context refresh so downstream planning uses current evidence.",
      },
    ];
  }

  if (run.agent === "strategist" || run.agent === "writer") {
    return [
      {
        title: "Review planning inputs",
        detail: "Open Content Plan and confirm the selected opportunity, target URL, and source context are complete.",
      },
      {
        title: "Retry the generation step",
        detail: "Rerun the smallest affected content step after the missing input or configuration is corrected.",
      },
    ];
  }

  if (run.output?.degraded) {
    return [
      {
        title: "Review the affected output",
        detail: "Open the related workflow and inspect the generated result before relying on it.",
      },
      {
        title: "Refresh missing inputs",
        detail: "If the output is thin, refresh context or add evidence before rerunning this automation.",
      },
    ];
  }

  return [
    {
      title: "Check configuration",
      detail: "Confirm the required credentials, project settings, and workflow inputs are present.",
    },
    {
      title: "Retry the smallest step",
      detail: "After correcting configuration, rerun only the failed workflow step and keep this record for audit history.",
    },
  ];
}

function aiFixBrief(run: GenerationRun, projectId: string) {
  return JSON.stringify(
    {
      task: "Fix a CiteLoop automation blocker",
      project_id: projectId,
      run_id: run.id,
      automation: run.agent,
      status: run.status,
      created_at: run.created_at,
      user_impact: userImpact(run),
      next_action: nextAction(run),
      error: rawError(run),
      recommended_steps: fixGuidance(run),
      run_details: {
        model: run.model,
        tokens: run.tokens,
        estimated_spend_usd: run.cost_usd,
      },
    },
    null,
    2,
  );
}

function AttentionRunCard({ run, onOpen }: { run: GenerationRun; onOpen: (run: GenerationRun) => void }) {
  const degraded = Boolean(run.output?.degraded);
  return (
    <button
      type="button"
      onClick={() => onOpen(run)}
      aria-label={`Open fix details for ${activityLabel(run.agent)}`}
      className="group flex aspect-square min-h-[246px] w-full flex-col justify-between rounded-lg border border-slate-200 bg-white p-4 text-left transition-all duration-150 hover:-translate-y-0.5 hover:border-slate-300 hover:shadow-sm active:translate-y-0"
    >
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-bold leading-6 text-slate-950">{activityLabel(run.agent)}</span>
          <Badge tone={statusTone(run.status, degraded)}>{run.status}</Badge>
          {degraded && <Badge tone="amber">degraded</Badge>}
        </div>
        <div className="mt-2 text-xs font-semibold text-slate-400">{formatDate(run.created_at)}</div>
      </div>

      <div className="grid gap-3">
        <div>
          <div className="text-xs font-bold uppercase text-slate-400">User impact</div>
          <p className="mt-1 line-clamp-3 text-sm leading-5 text-slate-600">{userImpact(run)}</p>
        </div>
        <div>
          <div className="text-xs font-bold uppercase text-slate-400">Next action</div>
          <p className="mt-1 line-clamp-2 text-sm leading-5 text-slate-600">{nextAction(run)}</p>
        </div>
      </div>

      <div className="flex items-center justify-between border-t border-slate-100 pt-3 text-sm font-semibold text-slate-700">
        <span>Open fix drawer</span>
        <ArrowRight className="transition-transform group-hover:translate-x-0.5" size={16} />
      </div>
    </button>
  );
}

function AuditRunRow({ run, onOpen }: { run: GenerationRun; onOpen: (run: GenerationRun) => void }) {
  const degraded = Boolean(run.output?.degraded);
  return (
    <button
      type="button"
      onClick={() => onOpen(run)}
      className="flex w-full flex-col gap-3 rounded-lg border border-slate-200 bg-white px-4 py-3 text-left transition-colors hover:border-slate-300 hover:bg-slate-50 active:translate-y-px sm:flex-row sm:items-center sm:justify-between"
    >
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-bold text-slate-900">{activityLabel(run.agent)}</span>
          <Badge tone={statusTone(run.status, degraded)}>{run.status}</Badge>
          {degraded && <Badge tone="amber">degraded</Badge>}
        </div>
        <div className="mt-1 text-xs font-semibold text-slate-400">{formatDate(run.created_at)}</div>
      </div>
      <span className="inline-flex items-center gap-2 text-sm font-semibold text-slate-500">
        Open record
        <ArrowRight size={14} />
      </span>
    </button>
  );
}

export function RunsClient({ projectId, embeddedInSettings = false }: { projectId: string; embeddedInSettings?: boolean }) {
  const api = useApi();
  const [runs, setRuns] = useState<GenerationRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedRun, setSelectedRun] = useState<GenerationRun | null>(null);
  const [copiedRunId, setCopiedRunId] = useState<string | null>(null);

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

  async function copySelectedFixBrief() {
    if (!selectedRun) return;
    try {
      await navigator.clipboard.writeText(aiFixBrief(selectedRun, projectId));
      setCopiedRunId(selectedRun.id);
    } catch {
      setCopiedRunId(null);
    }
  }

  const selectedRunError = selectedRun ? rawError(selectedRun) : "";
  const selectedFixGuidance = selectedRun ? fixGuidance(selectedRun) : [];

  return (
    <div className={cx("space-y-7", embeddedInSettings && "pt-1")}>
      <OperationsHealth />

      <SectionHeader
        title="Activity Log"
        eyebrow={embeddedInSettings ? "Settings - automation audit" : "Automation audit"}
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
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                {summary.attention.map((run) => (
                  <AttentionRunCard key={run.id} run={run} onOpen={(run) => setSelectedRun(run)} />
                ))}
              </div>
            )}
          </section>

          <section>
            <details className="rounded-lg border border-slate-200 bg-white p-4">
              <summary className="cursor-pointer text-sm font-bold text-slate-900">
                Recent successful activity ({summary.successful.length})
              </summary>
              <div className="mt-4 grid gap-3">
                {summary.successful.length === 0 ? (
                  <div className="text-sm text-slate-500">No successful activity in the current window.</div>
                ) : (
                  summary.successful.map((run) => <AuditRunRow key={run.id} run={run} onOpen={(run) => setSelectedRun(run)} />)
                )}
              </div>
            </details>
          </section>
        </>
      )}

      {selectedRun && (
        <RightDrawer
          open={Boolean(selectedRun)}
          title={activityLabel(selectedRun.agent)}
          eyebrow="Activity repair"
          subtitle={userImpact(selectedRun)}
          badges={
            <>
              <Badge tone={statusTone(selectedRun.status, Boolean(selectedRun.output?.degraded))}>{selectedRun.status}</Badge>
              {selectedRun.output?.degraded && <Badge tone="amber">degraded</Badge>}
            </>
          }
          dataAttribute="activity-run-drawer"
          maxWidthClassName="max-w-xl"
          onClose={() => {
            setSelectedRun(null);
            setCopiedRunId(null);
          }}
          footer={
            <>
              <a
                href={fixDestination(projectId, selectedRun)}
                className="inline-flex h-10 items-center justify-center gap-2 rounded-xl border border-slate-200 bg-white px-3 text-sm font-medium text-slate-700 transition-all duration-150 hover:bg-slate-50 active:scale-[0.97]"
              >
                <ExternalLink size={15} />
                {fixActionLabel(selectedRun)}
              </a>
              <Button variant="ai" onClick={copySelectedFixBrief}>
                <Clipboard size={15} />
                {copiedRunId === selectedRun.id ? "Copied" : "Copy AI fix brief"}
              </Button>
            </>
          }
        >
          <div className="space-y-5">
            <section className="rounded-lg border border-red-100 bg-red-50 p-4">
              <div className="flex items-start gap-3">
                <AlertTriangle className="mt-0.5 shrink-0 text-red-600" size={18} />
                <div className="min-w-0">
                  <h3 className="text-sm font-bold text-red-950">What failed</h3>
                  <p className="mt-2 break-words text-sm leading-6 text-red-800">{selectedRunError}</p>
                </div>
              </div>
            </section>

            <section>
              <h3 className="text-sm font-bold text-slate-950">How to fix</h3>
              <div className="mt-3 grid gap-3">
                {selectedFixGuidance.map((step, index) => (
                  <div key={step.title} className="rounded-lg border border-slate-200 bg-white p-3">
                    <div className="text-xs font-bold uppercase text-slate-400">Step {index + 1}</div>
                    <div className="mt-1 text-sm font-bold text-slate-950">{step.title}</div>
                    <p className="mt-1 text-sm leading-6 text-slate-600">{step.detail}</p>
                  </div>
                ))}
              </div>
            </section>

            <section className="rounded-lg border border-cyan-200 bg-cyan-50 p-4">
              <div className="flex items-start gap-3">
                <Sparkles className="mt-0.5 shrink-0 text-cyan-700" size={18} />
                <div className="min-w-0">
                  <h3 className="text-sm font-bold text-cyan-950">AI-ready fix brief</h3>
                  <p className="mt-1 text-sm leading-6 text-cyan-800">
                    Copy the structured brief when a coding agent or support teammate needs exact run context.
                  </p>
                  <pre className="mt-3 max-h-56 overflow-auto rounded-lg border border-cyan-200 bg-white p-3 text-xs leading-5 text-slate-700">
                    {aiFixBrief(selectedRun, projectId)}
                  </pre>
                </div>
              </div>
            </section>

            <section className="rounded-lg border border-slate-200 bg-white p-4">
              <h3 className="text-sm font-bold text-slate-950">Run details</h3>
              <dl className="mt-3 grid gap-3 text-sm sm:grid-cols-2">
                <div>
                  <dt className="text-xs font-bold uppercase text-slate-400">Automation</dt>
                  <dd className="mt-1 break-words text-slate-700">{selectedRun.agent}</dd>
                </div>
                <div>
                  <dt className="text-xs font-bold uppercase text-slate-400">Run ID</dt>
                  <dd className="mt-1 break-words text-slate-700">{selectedRun.id}</dd>
                </div>
                <div>
                  <dt className="text-xs font-bold uppercase text-slate-400">Model used</dt>
                  <dd className="mt-1 break-words text-slate-700">{selectedRun.model ?? "-"}</dd>
                </div>
                <div>
                  <dt className="text-xs font-bold uppercase text-slate-400">Tokens</dt>
                  <dd className="mt-1 text-slate-700">{selectedRun.tokens ?? "-"}</dd>
                </div>
                <div>
                  <dt className="text-xs font-bold uppercase text-slate-400">Estimated spend</dt>
                  <dd className="mt-1 text-slate-700">{money(selectedRun.cost_usd)}</dd>
                </div>
                <div>
                  <dt className="text-xs font-bold uppercase text-slate-400">Created</dt>
                  <dd className="mt-1 text-slate-700">{formatDate(selectedRun.created_at)}</dd>
                </div>
              </dl>
            </section>
          </div>
        </RightDrawer>
      )}
    </div>
  );
}
