"use client";

import { useEffect, useState } from "react";
import { Check, Circle, Loader2 } from "lucide-react";
import { OpportunityFindingStatus } from "../../../lib/api";
import { cx } from "../../../components/ui";

const stages = [
  ["evidence_refresh", "Refresh evidence"],
  ["deterministic_signals", "Analyze search signals"],
  ["ai_hypotheses", "Discover opportunities"],
  ["arbitration", "Resolve duplicates"],
  ["materialization", "Build recommendations"],
  ["summary", "Finish"],
] as const;

const zeroReasonCopy: Record<string, string> = {
  already_handled_or_merged: "The strongest candidates already exist in completed or merged work.",
  "demand.single_geo_provider": "The remaining candidates need another independent evidence source.",
  "context.capability_unconfirmed": "Confirm the relevant public product capability before publishing.",
  "context.internal_sensitive": "The remaining candidates could expose private implementation context.",
  "score.below_stage_threshold": "No remaining candidate met this Growth Stage's quality threshold.",
};

function elapsedSecondsSince(startedAt: unknown) {
  const started = typeof startedAt === "string" || typeof startedAt === "number" ? new Date(startedAt).getTime() : Number.NaN;
  if (!Number.isFinite(started)) return 0;
  return Math.max(0, Math.floor((Date.now() - started) / 1000));
}

function formatElapsed(totalSeconds: number) {
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return minutes > 0 ? `${minutes}m ${String(seconds).padStart(2, "0")}s` : `${seconds}s`;
}

export function OpportunityFindingProgress({ status }: { status: OpportunityFindingStatus | null }) {
  const run = status?.last_run;
  const active = run?.status === "queued" || run?.status === "running";
  const [elapsedSeconds, setElapsedSeconds] = useState(0);

  useEffect(() => {
    if (!active) return;
    const updateElapsed = () => setElapsedSeconds(elapsedSecondsSince(run?.started_at));
    updateElapsed();
    const timer = window.setInterval(updateElapsed, 1000);
    return () => window.clearInterval(timer);
  }, [active, run?.id, run?.started_at]);

  if (!run) return null;
  const rawProgress = Number(run.progress_percent ?? 0);
  const completed = new Map(run.stage_progress.map((stage) => [stage.stage, stage.status]));
  const stageDurations = new Map(run.stage_progress.map((stage) => [stage.stage, Number(stage.duration_ms ?? 0)]));
  const stageDurationTotalMs = Array.from(stageDurations.values()).reduce((total, duration) => total + Math.max(0, duration), 0);
  const currentStage = run.current_stage ?? (run.status === "queued" ? "queued" : "");
  const currentLabel = currentStage === "queued"
    ? "Preparing the discovery run"
    : stages.find(([key]) => key === currentStage)?.[1] ?? "Working";
  const terminal = run.status === "completed" || run.status === "partial";
  const progress = Math.max(0, Math.min(100, terminal && rawProgress <= 0 ? 100 : rawProgress));
  const runDurationMs = Number(run.duration_ms ?? 0) > 0 ? Number(run.duration_ms) : stageDurationTotalMs;
  const runDurationSeconds = Math.round(runDurationMs / 1000);
  const terminalTitle = run.status === "partial" ? "Finding completed with notes" : "Finding completed";
  const terminalDetail = run.new_opportunity_count > 0
    ? `${run.new_opportunity_count} Opportunity ${run.new_opportunity_count === 1 ? "recommendation" : "recommendations"} generated or refreshed in this run.`
    : run.zero_result_reason
      ? `No new Opportunity. ${zeroReasonCopy[run.zero_result_reason] ?? run.zero_result_reason}`
      : "Run timeline is available below.";
  const refreshingEvidence = active && currentStage === "evidence_refresh";
  const callingAI = active && currentStage === "ai_hypotheses";
  const activeDetail = refreshingEvidence
    ? "Refreshing search, competitive recall, and AI observations"
    : callingAI
      ? "Calling AI to repair and score candidate opportunities"
      : "The run continues safely in the background";

  if (!active && !terminal) return null;

  return (
    <div data-opportunity-finding-progress className="mt-4 rounded-xl border border-white/80 bg-white/75 p-3.5" aria-live="polite">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <div className="text-sm font-bold text-slate-950">{terminal ? terminalTitle : currentLabel}</div>
          <div className="mt-0.5 text-xs text-slate-500">
            {terminal ? terminalDetail : activeDetail}
          </div>
          {terminal ? (
            <div className="mt-1 text-[11px] text-slate-400">
              Queue shows only recommendations that still need a human decision; auto-routed or already-handled work stays out of review.
            </div>
          ) : (
            <div className="mt-1 text-[11px] text-slate-400">Usually 45–120 seconds; complex runs may take up to 3 minutes.</div>
          )}
        </div>
        <div className="flex items-center gap-2 text-xs font-semibold text-slate-600">
          {refreshingEvidence && <span className="inline-flex items-center gap-1.5 text-emerald-700"><Loader2 aria-hidden="true" size={14} className="animate-spin" />Refreshing evidence</span>}
          {callingAI && <span className="inline-flex items-center gap-1.5 text-emerald-700"><Loader2 aria-hidden="true" size={14} className="animate-spin" />Calling AI</span>}
          {terminal && <span className="text-emerald-700">Run timeline</span>}
          <span>{terminal ? "Duration" : "Elapsed"} {formatElapsed(terminal ? runDurationSeconds : elapsedSeconds)}</span>
          <span className="text-slate-400">Completed checkpoints: {progress}%</span>
        </div>
      </div>
      <div
        role="progressbar"
        aria-label="Opportunity finding progress"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={terminal ? progress : undefined}
        aria-valuetext={`${terminal ? terminalTitle : currentLabel}, ${terminal ? "duration" : "elapsed"} ${formatElapsed(terminal ? runDurationSeconds : elapsedSeconds)}`}
        data-indeterminate={active ? "true" : "false"}
        className="mt-3 h-2 overflow-hidden rounded-full bg-slate-200"
      >
        <div
          className={cx("h-full rounded-full bg-emerald-500", active ? "opportunity-finding-progress-slide w-1/3" : "transition-all")}
          style={terminal ? { width: `${progress}%` } : undefined}
        />
      </div>
      <div className="mt-3 grid gap-1.5 sm:grid-cols-2 xl:grid-cols-3">
        {stages.map(([key, label]) => {
          const state = completed.get(key);
          const durationMs = stageDurations.get(key) ?? 0;
          const isCurrent = key === currentStage;
          const isDone = state === "succeeded" || state === "partial" || state === "skipped";
          return (
            <div key={key} className={cx("flex items-center gap-2 text-xs", isCurrent ? "font-bold text-slate-950" : isDone ? "text-slate-600" : "text-slate-400")}>
              {isDone ? <Check aria-hidden="true" size={14} className="text-emerald-600" /> : isCurrent ? <Loader2 aria-hidden="true" size={14} className="animate-spin text-emerald-600" /> : <Circle aria-hidden="true" size={12} />}
              <span>{label}</span>
              {durationMs > 0 && <span className="text-[11px] font-medium text-slate-400">{formatElapsed(Math.round(durationMs / 1000))}</span>}
            </div>
          );
        })}
      </div>
    </div>
  );
}
