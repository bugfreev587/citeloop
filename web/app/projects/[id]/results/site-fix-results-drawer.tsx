"use client";

import type { RefObject } from "react";
import Link from "next/link";
import type { ResultsSiteFixMeasurementDetail, ResultsSiteFixSummary } from "../../../lib/api";
import { Badge, Notice, formatDate } from "../../../components/ui";
import { RightDrawer } from "../../../components/right-drawer";
import { humanizeInternalType } from "../../../lib/site-fix";

type SiteFixResultsDrawerProps = {
  projectId: string;
  summary: ResultsSiteFixSummary | null;
  detail: ResultsSiteFixMeasurementDetail | null;
  loading: boolean;
  error: string | null;
  surfaceRef: RefObject<HTMLElement | null>;
  returnFocusRef: RefObject<HTMLElement | null>;
  onClose: () => void;
};

function outcomeTone(outcome?: string | null): "green" | "red" | "amber" | "neutral" {
  if (outcome === "positive") return "green";
  if (outcome === "negative") return "red";
  if (outcome) return "amber";
  return "neutral";
}

export function SiteFixResultsDrawer({
  projectId,
  summary,
  detail,
  loading,
  error,
  surfaceRef,
  returnFocusRef,
  onClose,
}: SiteFixResultsDrawerProps) {
  const measurement = detail?.measurement ?? summary;
  const terminal = detail?.terminal;
  return (
    <RightDrawer
      open={Boolean(summary)}
      title={measurement ? humanizeInternalType(measurement.fix_type) : "Site Fix measurement"}
      eyebrow="Independent measurement"
      subtitle={measurement?.target_url || summary?.site_fix_id}
      badges={measurement ? (
        <>
          <Badge tone="blue">Site Fix</Badge>
          <Badge tone={outcomeTone(terminal?.outcome_label ?? measurement.terminal_outcome)}>
            {humanizeInternalType(terminal?.outcome_label ?? measurement.terminal_outcome ?? measurement.status)}
          </Badge>
          {detail?.measurement_handoff_status && <Badge tone="neutral">{humanizeInternalType(detail.measurement_handoff_status)}</Badge>}
        </>
      ) : null}
      closeLabel="Close Site Fix measurement details"
      dataAttribute="results-site-fix-drawer"
      surfaceRef={surfaceRef}
      returnFocusRef={returnFocusRef}
      onClose={onClose}
      footer={detail ? (
        <Link
          href={`/projects/${projectId}/site-fixes?fix=${detail.site_fix.id}`}
          className="inline-flex h-10 items-center justify-center rounded-xl border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-700 transition hover:bg-slate-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-slate-400"
        >
          Open verified Site Fix
        </Link>
      ) : undefined}
    >
      {loading ? (
        <div role="status" className="space-y-3" aria-label="Loading Site Fix measurement details">
          <div className="h-24 animate-pulse rounded-xl bg-slate-100" />
          <div className="h-40 animate-pulse rounded-xl bg-slate-100" />
        </div>
      ) : error ? (
        <Notice tone="red" title="Measurement details could not load" detail={error} />
      ) : detail && measurement ? (
        <div className="space-y-5">
          {measurement.prospective_observation && (
            <Notice
              tone="amber"
              title="Prospective observation"
              detail="This observation began after deployment. It can describe data quality and later signals, but it is not directional attribution for the original change."
            />
          )}
          <section className="grid gap-3 text-sm sm:grid-cols-2">
            <div className="rounded-lg border border-slate-200 p-3">
              <div className="text-xs font-semibold uppercase text-slate-400">Measurement status</div>
              <div className="mt-1 font-medium text-slate-700">{humanizeInternalType(measurement.status)}</div>
            </div>
            <div className="rounded-lg border border-slate-200 p-3">
              <div className="text-xs font-semibold uppercase text-slate-400">Attribution confidence</div>
              <div className="mt-1 font-medium text-slate-700">{humanizeInternalType(measurement.attribution_confidence)}</div>
            </div>
            <div className="rounded-lg border border-slate-200 p-3">
              <div className="text-xs font-semibold uppercase text-slate-400">Primary metric</div>
              <div className="mt-1 font-medium text-slate-700">{humanizeInternalType(measurement.primary_metric || "not configured")}</div>
            </div>
            <div className="rounded-lg border border-slate-200 p-3">
              <div className="text-xs font-semibold uppercase text-slate-400">Absolute deadline</div>
              <div className="mt-1 font-medium text-slate-700">{formatDate(measurement.absolute_terminal_at ?? null)}</div>
            </div>
          </section>

          <section className="rounded-xl border border-slate-200 p-4">
            <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Outcome</div>
            <div className="mt-3 flex flex-wrap gap-2">
              <Badge tone={outcomeTone(terminal?.outcome_label ?? measurement.terminal_outcome)}>
                {humanizeInternalType(terminal?.outcome_label ?? measurement.terminal_outcome ?? "not terminal")}
              </Badge>
            </div>
            <p className="mt-3 text-sm leading-6 text-slate-700">{terminal?.terminal_reason || measurement.outcome_reason || "No terminal outcome yet."}</p>
          </section>

          <section className="rounded-xl border border-slate-200 p-4">
            <div className="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">Measurement checkpoints</div>
            {detail.checkpoints.length ? (
              <div className="mt-3 grid gap-2">
                {detail.checkpoints.map((checkpoint) => (
                  <div key={checkpoint.id} className="grid gap-2 rounded-lg bg-slate-50 p-3 text-sm sm:grid-cols-3">
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">Role</div>
                      <div className="mt-1 font-medium text-slate-700">{humanizeInternalType(checkpoint.checkpoint_role)}</div>
                    </div>
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">Scheduled</div>
                      <div className="mt-1 font-medium text-slate-700">{formatDate(checkpoint.scheduled_at ?? null)}</div>
                    </div>
                    <div>
                      <div className="text-xs font-semibold uppercase text-slate-400">Outcome</div>
                      <div className="mt-1 font-medium text-slate-700">{humanizeInternalType(checkpoint.outcome_label ?? "pending")}</div>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <p className="mt-2 text-sm text-slate-600">No checkpoint is due yet.</p>
            )}
          </section>
        </div>
      ) : null}
    </RightDrawer>
  );
}
