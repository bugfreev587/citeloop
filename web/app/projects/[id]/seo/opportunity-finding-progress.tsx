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
  const progress = Math.max(0, Math.min(100, Number(run.progress_percent ?? 0)));
  const completed = new Map(run.stage_progress.map((stage) => [stage.stage, stage.status]));
  const currentStage = run.current_stage ?? (run.status === "queued" ? "queued" : "");
  const currentLabel = currentStage === "queued"
    ? "Preparing the discovery run"
    : stages.find(([key]) => key === currentStage)?.[1] ?? "Working";
  const callingAI = active && (currentStage === "evidence_refresh" || currentStage === "ai_hypotheses");

  if (!active) {
    if (run.status !== "completed" && run.status !== "partial") return null;
    if (run.new_opportunity_count <= 0 && !run.zero_result_reason) return null;
    return (
      <div data-opportunity-finding-progress className="mt-4 rounded-lg border border-white/80 bg-white/70 px-3 py-2.5 text-sm text-slate-700">
        {run.new_opportunity_count > 0 ? (
          <span><strong className="text-slate-950">{run.new_opportunity_count} new Opportunities</strong> found in this run.</span>
        ) : run.zero_result_reason ? (
          <span><strong className="text-slate-950">No new Opportunity.</strong> {zeroReasonCopy[run.zero_result_reason] ?? run.zero_result_reason}</span>
        ) : null}
      </div>
    );
  }

  return (
    <div data-opportunity-finding-progress className="mt-4 rounded-xl border border-white/80 bg-white/75 p-3.5" aria-live="polite">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <div className="text-sm font-bold text-slate-950">{currentLabel}</div>
          <div className="mt-0.5 text-xs text-slate-500">
            {callingAI ? "Calling AI and validating fresh evidence" : "The run continues safely in the background"}
          </div>
          <div className="mt-1 text-[11px] text-slate-400">Usually 45–120 seconds; complex runs may take up to 3 minutes.</div>
        </div>
        <div className="flex items-center gap-2 text-xs font-semibold text-slate-600">
          {callingAI && <span className="inline-flex items-center gap-1.5 text-emerald-700"><Loader2 aria-hidden="true" size={14} className="animate-spin" />Calling AI</span>}
          <span>Elapsed {formatElapsed(elapsedSeconds)}</span>
          <span className="text-slate-400">Completed checkpoints: {progress}%</span>
        </div>
      </div>
      <div
        role="progressbar"
        aria-label="Opportunity finding progress"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuetext={`${currentLabel}, elapsed ${formatElapsed(elapsedSeconds)}`}
        data-indeterminate="true"
        className="mt-3 h-2 overflow-hidden rounded-full bg-slate-200"
      >
        <div className="opportunity-finding-progress-slide h-full w-1/3 rounded-full bg-emerald-500" />
      </div>
      <div className="mt-3 grid gap-1.5 sm:grid-cols-2 xl:grid-cols-3">
        {stages.map(([key, label]) => {
          const state = completed.get(key);
          const isCurrent = key === currentStage;
          const isDone = state === "succeeded" || state === "partial" || state === "skipped";
          return (
            <div key={key} className={cx("flex items-center gap-2 text-xs", isCurrent ? "font-bold text-slate-950" : isDone ? "text-slate-600" : "text-slate-400")}>
              {isDone ? <Check aria-hidden="true" size={14} className="text-emerald-600" /> : isCurrent ? <Loader2 aria-hidden="true" size={14} className="animate-spin text-emerald-600" /> : <Circle aria-hidden="true" size={12} />}
              <span>{label}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
