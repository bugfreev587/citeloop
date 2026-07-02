"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { AlertTriangle, CheckCircle2, Clipboard, Loader2, Play, RefreshCw, Stethoscope, Wrench, X } from "lucide-react";
import { SEODoctorFinding, SEODoctorReport, SEODoctorRun } from "../../../lib/api";
import { useApi } from "../../../lib/use-api";
import { Badge, Button, ButtonProgress, EmptyState, Notice, SectionHeader, cx, formatDate } from "../../../components/ui";
import { useToast } from "../../../components/toast-provider";

type SeverityFilter = "all" | "P0" | "P1" | "P2" | "Info";

const severityOrder: Record<string, number> = { P0: 0, P1: 1, P2: 2, Info: 3 };

function isActiveRun(run?: SEODoctorRun | null) {
  return run?.status === "queued" || run?.status === "running";
}

function severityTone(severity: string): "red" | "amber" | "blue" | "neutral" {
  if (severity === "P0") return "red";
  if (severity === "P1") return "amber";
  if (severity === "P2") return "blue";
  return "neutral";
}

function statusTone(status?: string): "red" | "amber" | "green" | "blue" | "neutral" {
  if (status === "blocked" || status === "failed") return "red";
  if (status === "completed") return "green";
  if (status === "queued" || status === "running") return "blue";
  return "neutral";
}

function healthTone(score?: number | null) {
  if (score == null) return "text-slate-400";
  if (score < 70) return "text-red-700";
  if (score < 90) return "text-amber-700";
  return "text-emerald-700";
}

function issueCounts(report: SEODoctorReport | null) {
  const counts = report?.human_report?.issue_counts ?? {};
  return {
    P0: Number(counts.P0 ?? 0),
    P1: Number(counts.P1 ?? 0),
    P2: Number(counts.P2 ?? 0),
    Info: Number(counts.Info ?? 0),
  };
}

function sortedFindings(findings: SEODoctorFinding[]) {
  return [...findings].sort((a, b) => {
    return (severityOrder[a.severity] ?? 4) - (severityOrder[b.severity] ?? 4) || a.issue_type.localeCompare(b.issue_type);
  });
}

function firstURL(finding: SEODoctorFinding) {
  return finding.affected_urls[0] || finding.normalized_urls[0] || "Project surface";
}

export function DoctorClient({ projectId }: { projectId: string }) {
  const api = useApi();
  const { notify } = useToast();
  const [report, setReport] = useState<SEODoctorReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState<SeverityFilter>("all");
  const [busyFindingID, setBusyFindingID] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setError(null);
    try {
      const next = await api.getSEODoctor(projectId);
      setReport(next);
      return next;
    } catch (err: any) {
      setError(err?.apiMessage || err?.message || "Could not load SEO Doctor.");
      return null;
    } finally {
      setLoading(false);
    }
  }, [api, projectId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (!isActiveRun(report?.run)) return;
    const interval = window.setInterval(() => {
      void refresh();
    }, 2500);
    return () => window.clearInterval(interval);
  }, [refresh, report?.run?.id, report?.run?.status]);

  const run = report?.run ?? null;
  const counts = issueCounts(report);
  const visibleFindings = useMemo(() => {
    const findings = sortedFindings(report?.findings ?? []);
    return filter === "all" ? findings : findings.filter((finding) => finding.severity === filter);
  }, [filter, report?.findings]);
  const progress = Math.max(0, Math.min(100, run?.progress_percent ?? 0));
  const healthScore = run?.health_score ?? report?.human_report?.health_score ?? null;
  const aiReportJSON = JSON.stringify(report?.ai_coding_tool_report ?? { findings: [] }, null, 2);

  async function runDoctor() {
    setRunning(true);
    setError(null);
    try {
      const nextRun = await api.startSEODoctorRun(projectId);
      setReport((current) => ({ ...(current ?? { findings: [] }), run: nextRun }));
      notify({ tone: "green", title: "SEO Doctor started", detail: "The report will update as checks complete." });
      window.setTimeout(() => void refresh(), 800);
    } catch (err: any) {
      setError(err?.apiMessage || err?.message || "Could not start SEO Doctor.");
    } finally {
      setRunning(false);
    }
  }

  async function copyAIReport() {
    await navigator.clipboard.writeText(aiReportJSON);
    notify({ tone: "green", title: "AI coding report copied" });
  }

  async function convertFinding(finding: SEODoctorFinding) {
    setBusyFindingID(finding.id);
    try {
      await api.convertSEODoctorFinding(projectId, finding.id);
      notify({ tone: "green", title: "Finding sent to action queue" });
      await refresh();
    } catch (err: any) {
      notify({ tone: "red", title: "Could not convert finding", detail: err?.apiMessage || err?.message });
    } finally {
      setBusyFindingID(null);
    }
  }

  async function dismissFinding(finding: SEODoctorFinding) {
    setBusyFindingID(finding.id);
    try {
      await api.dismissSEODoctorFinding(projectId, finding.id);
      notify({ tone: "green", title: "Finding dismissed" });
      await refresh();
    } catch (err: any) {
      notify({ tone: "red", title: "Could not dismiss finding", detail: err?.apiMessage || err?.message });
    } finally {
      setBusyFindingID(null);
    }
  }

  return (
    <div className="space-y-4">
      {error && <Notice title="SEO Doctor could not load" detail={error} tone="amber" />}

      <section className="rounded-xl border border-slate-200 bg-white px-4 py-4">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <span className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-slate-50 text-[#d93820] ring-1 ring-slate-100">
                <Stethoscope size={18} />
              </span>
              <Badge tone={statusTone(run?.status)}>{run?.status ?? "not_run"}</Badge>
              {run?.trigger && <Badge tone="neutral">{run.trigger}</Badge>}
            </div>
            <h1 className="mt-3 text-2xl font-bold leading-8 text-slate-950">Site health</h1>
            <p className="mt-1 max-w-[72ch] text-sm font-semibold leading-5 text-slate-500">
              {report?.human_report?.summary ?? "Run SEO Doctor to check crawl, index, metadata, schema, links, and report trust signals."}
            </p>
          </div>
          <div className="grid gap-3 sm:grid-cols-[120px_1fr] lg:min-w-[420px]">
            <div className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-3">
              <div className="text-xs font-bold uppercase text-slate-400">Health</div>
              <div className={cx("mt-2 text-4xl font-bold leading-none", healthTone(healthScore))}>{healthScore ?? "-"}</div>
            </div>
            <div className="rounded-lg border border-slate-200 px-3 py-3">
              <div className="flex items-center justify-between gap-3">
                <div className="text-xs font-bold uppercase text-slate-400">{run?.stage ?? "ready"}</div>
                <div className="text-xs font-bold text-slate-500">{progress}%</div>
              </div>
              <div className="mt-2 h-2 overflow-hidden rounded-full bg-slate-100">
                <div className="h-full rounded-full bg-[#d93820] transition-all duration-500" style={{ width: `${progress}%` }} />
              </div>
              <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs font-semibold text-slate-500">
                <span>{run?.pages_checked ?? 0} checked</span>
                <span>{run?.issues_found ?? 0} issues</span>
                <span>{formatDate(run?.updated_at ?? null)}</span>
              </div>
            </div>
          </div>
        </div>
        <div className="mt-4 flex flex-wrap items-center gap-2">
          <Button variant="primary" onClick={runDoctor} disabled={running || isActiveRun(run)}>
            <ButtonProgress busy={running || isActiveRun(run)} busyLabel={isActiveRun(run) ? "Running" : "Starting"} idleIcon={<Play size={15} />}>
              Run Doctor
            </ButtonProgress>
          </Button>
          <Button onClick={() => void refresh()} disabled={loading}>
            <ButtonProgress busy={loading} busyLabel="Refreshing" idleIcon={<RefreshCw size={15} />}>
              Refresh
            </ButtonProgress>
          </Button>
          {run?.block_reason && <Badge tone="red">{run.block_reason}</Badge>}
        </div>
      </section>

      <section className="grid gap-3 sm:grid-cols-4">
        {(["P0", "P1", "P2", "Info"] as const).map((severity) => (
          <button
            key={severity}
            type="button"
            onClick={() => setFilter(severity)}
            className={cx(
              "rounded-xl border bg-white px-4 py-3 text-left transition-colors hover:border-slate-300",
              filter === severity ? "border-[#d93820] ring-1 ring-[#d93820]" : "border-slate-200",
            )}
          >
            <div className="flex items-center justify-between">
              <Badge tone={severityTone(severity)}>{severity}</Badge>
              <span className="text-2xl font-bold text-slate-950">{counts[severity]}</span>
            </div>
            <div className="mt-2 text-xs font-semibold text-slate-500">{severity === "Info" ? "Notes" : "Active issues"}</div>
          </button>
        ))}
      </section>

      <section>
        <SectionHeader
          title="Findings"
          eyebrow="Grouped technical repairs"
          action={
            <Button size="sm" variant={filter === "all" ? "primary" : "outline"} onClick={() => setFilter("all")}>
              All findings
            </Button>
          }
        />
        {loading ? (
          <div className="grid gap-3 md:grid-cols-2">
            {[0, 1, 2, 3].map((item) => (
              <div key={item} className="h-40 animate-pulse rounded-xl border border-slate-200 bg-white p-4">
                <div className="h-4 w-24 rounded bg-slate-100" />
                <div className="mt-4 h-5 w-2/3 rounded bg-slate-100" />
                <div className="mt-3 h-4 w-full rounded bg-slate-100" />
              </div>
            ))}
          </div>
        ) : visibleFindings.length === 0 ? (
          <EmptyState title="No findings in this view" detail="SEO Doctor has no active findings for the selected severity." />
        ) : (
          <div className="grid gap-3 lg:grid-cols-2">
            {visibleFindings.map((finding) => (
              <article key={finding.id} className="rounded-xl border border-slate-200 bg-white p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge tone={severityTone(finding.severity)}>{finding.severity}</Badge>
                      <Badge tone="neutral">{finding.category}</Badge>
                      <span className="truncate text-xs font-bold text-slate-400">{finding.issue_type}</span>
                    </div>
                    <h2 className="mt-3 text-base font-bold leading-6 text-slate-950">{finding.fix_intent || finding.issue_type}</h2>
                  </div>
                  {finding.status === "active" ? <AlertTriangle size={17} className="shrink-0 text-amber-600" /> : <CheckCircle2 size={17} className="shrink-0 text-green-600" />}
                </div>
                <p className="mt-2 line-clamp-2 text-sm font-semibold leading-5 text-slate-600">{finding.why_it_matters}</p>
                <div className="mt-3 rounded-lg bg-slate-50 px-3 py-2 text-xs font-semibold text-slate-500">{firstURL(finding)}</div>
                <div className="mt-3 rounded-lg border border-slate-100 px-3 py-2">
                  <div className="text-xs font-bold uppercase text-slate-400">Developer instructions</div>
                  <p className="mt-1 text-sm font-semibold leading-5 text-slate-700">{finding.developer_instructions}</p>
                </div>
                <div className="mt-3 flex flex-wrap gap-2">
                  <Button size="sm" onClick={() => void convertFinding(finding)} disabled={busyFindingID === finding.id || finding.status !== "active"}>
                    <ButtonProgress busy={busyFindingID === finding.id} busyLabel="Sending" idleIcon={<Wrench size={14} />}>
                      Create action
                    </ButtonProgress>
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => void dismissFinding(finding)} disabled={busyFindingID === finding.id || finding.status !== "active"}>
                    <X size={14} />
                    Dismiss
                  </Button>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>

      <section>
        <SectionHeader
          title="AI coding report"
          eyebrow="Structured handoff"
          action={
            <Button size="sm" onClick={() => void copyAIReport()}>
              <Clipboard size={14} />
              Copy JSON
            </Button>
          }
        />
        <pre className="max-h-[420px] overflow-auto rounded-xl border border-slate-200 bg-slate-950 p-4 text-xs leading-5 text-slate-100">
          {aiReportJSON}
        </pre>
      </section>
    </div>
  );
}
