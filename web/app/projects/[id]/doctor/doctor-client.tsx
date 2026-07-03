"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { AlertTriangle, ArrowRight, CheckCircle2, Play, RefreshCw, Stethoscope, X } from "lucide-react";
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

function findingEvidence(finding: SEODoctorFinding): Array<[string, unknown]> {
  const evidence = finding.evidence && typeof finding.evidence === "object" ? finding.evidence : {};
  const rawDetails = evidence.raw_details && typeof evidence.raw_details === "object" ? evidence.raw_details : {};

  const rows: Array<[string, unknown]> = [
    ["Page", evidence.page_url ?? firstURL(finding)],
    ["Normalized", evidence.normalized_page_url ?? finding.normalized_urls[0]],
    ["Status", rawDetails.status ?? evidence.status],
    ["Final URL", rawDetails.final_url ?? evidence.final_url],
    ["Confidence", evidence.confidence_label ?? evidence.confidence],
  ];
  return rows.filter(([, value]) => value != null && `${value}`.trim() !== "");
}

function isSelectableFinding(finding: SEODoctorFinding) {
  return finding.status === "active" && finding.severity !== "Info";
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
  const [selectedFindingIDs, setSelectedFindingIDs] = useState<string[]>([]);
  const [startingGrowthLoop, setStartingGrowthLoop] = useState(false);

  const refresh = useCallback(async () => {
    setError(null);
    try {
      const next = await api.getSEODoctor(projectId);
      setReport(next);
      return next;
    } catch (err: any) {
      setError(err?.apiMessage || err?.message || "Could not load Doctor.");
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
  const selectableFindingIDs = useMemo(() => {
    return new Set((report?.findings ?? []).filter(isSelectableFinding).map((finding) => finding.id));
  }, [report?.findings]);
  const selectedGrowthLoopIDs = useMemo(() => {
    return selectedFindingIDs.filter((id) => selectableFindingIDs.has(id));
  }, [selectableFindingIDs, selectedFindingIDs]);

  useEffect(() => {
    setSelectedFindingIDs((current) => current.filter((id) => selectableFindingIDs.has(id)));
  }, [selectableFindingIDs]);

  async function runDoctor() {
    setRunning(true);
    setError(null);
    try {
      const nextRun = await api.startSEODoctorRun(projectId);
      setReport((current) => ({ ...(current ?? { findings: [] }), run: nextRun }));
      notify({ tone: "green", title: "Doctor started", detail: "The report will update as checks complete." });
      window.setTimeout(() => void refresh(), 800);
    } catch (err: any) {
      setError(err?.apiMessage || err?.message || "Could not start Doctor.");
    } finally {
      setRunning(false);
    }
  }

  function toggleFindingSelection(findingID: string) {
    setSelectedFindingIDs((current) => {
      if (current.includes(findingID)) {
        return current.filter((id) => id !== findingID);
      }
      return [...current, findingID];
    });
  }

  async function startGrowthLoop() {
    if (!run?.id || selectedGrowthLoopIDs.length === 0) return;
    setStartingGrowthLoop(true);
    try {
      const result = await api.startSEODoctorGrowthLoop(projectId, run.id, selectedGrowthLoopIDs);
      notify({
        tone: "green",
        title: "Growth Loop started",
        detail: `${result.actions.length} ${result.actions.length === 1 ? "action was" : "actions were"} created from selected Doctor findings.`,
      });
      setSelectedFindingIDs([]);
      await refresh();
    } catch (err: any) {
      notify({ tone: "red", title: "Could not start Growth Loop", detail: err?.apiMessage || err?.message });
    } finally {
      setStartingGrowthLoop(false);
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
      {error && <Notice title="Doctor could not load" detail={error} tone="amber" />}

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
              {report?.human_report?.summary ?? "Run Doctor to check crawl, index, metadata, schema, links, and report trust signals."}
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
          <Button
            onClick={() => void startGrowthLoop()}
            disabled={!run?.id || selectedGrowthLoopIDs.length === 0 || startingGrowthLoop || isActiveRun(run)}
          >
            <ButtonProgress busy={startingGrowthLoop} busyLabel="Starting" idleIcon={<ArrowRight size={15} />}>
              Start Growth Loop
            </ButtonProgress>
          </Button>
          <span className="text-xs font-semibold text-slate-500">
            {selectedGrowthLoopIDs.length} selected
          </span>
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
          eyebrow="Grouped diagnostics"
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
          <EmptyState title="No findings in this view" detail="Doctor has no active findings for the selected severity." />
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
                  <div className="text-xs font-bold uppercase text-slate-400">Suggested next step</div>
                  <p className="mt-1 text-sm font-semibold leading-5 text-slate-700">{finding.developer_instructions}</p>
                </div>
                <div className="mt-3 grid gap-2 rounded-lg border border-slate-100 px-3 py-2 text-xs font-semibold text-slate-500">
                  {findingEvidence(finding).map(([label, value]) => (
                    <div key={label} className="grid gap-1 sm:grid-cols-[92px_1fr]">
                      <span className="text-slate-400">{label}</span>
                      <span className="min-w-0 break-words text-slate-700">{String(value)}</span>
                    </div>
                  ))}
                </div>
                {finding.acceptance_tests.length > 0 && (
                  <div className="mt-3 rounded-lg border border-slate-100 px-3 py-2">
                    <div className="text-xs font-bold uppercase text-slate-400">Verification</div>
                    <ul className="mt-1 space-y-1 text-sm font-semibold leading-5 text-slate-700">
                      {finding.acceptance_tests.slice(0, 3).map((item) => (
                        <li key={item}>{item}</li>
                      ))}
                    </ul>
                  </div>
                )}
                <div className="mt-4 flex flex-wrap items-center justify-between gap-2">
                  <label className={cx("inline-flex items-center gap-2 text-xs font-bold text-slate-500", !isSelectableFinding(finding) && "opacity-50")}>
                    <input
                      type="checkbox"
                      checked={selectedGrowthLoopIDs.includes(finding.id)}
                      onChange={() => toggleFindingSelection(finding.id)}
                      disabled={!isSelectableFinding(finding)}
                      className="h-4 w-4 rounded border-slate-300 text-[#d93820]"
                    />
                    Select for Growth Loop
                  </label>
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
    </div>
  );
}
