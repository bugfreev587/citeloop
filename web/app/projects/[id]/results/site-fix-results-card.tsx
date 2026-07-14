"use client";

import { ChevronRight } from "lucide-react";
import type { ResultsSiteFixSummary } from "../../../lib/api";
import { Badge, formatDate } from "../../../components/ui";
import { humanizeInternalType } from "../../../lib/site-fix";

type SiteFixResultsCardProps = {
  summary: ResultsSiteFixSummary;
  highlighted: boolean;
  selected: boolean;
  onOpen: (trigger: HTMLButtonElement) => void;
};

function outcomeTone(outcome?: string | null): "green" | "red" | "amber" | "neutral" {
  if (outcome === "positive") return "green";
  if (outcome === "negative") return "red";
  if (outcome) return "amber";
  return "neutral";
}

export function SiteFixResultsCard({ summary, highlighted, selected, onOpen }: SiteFixResultsCardProps) {
  return (
    <button
      type="button"
      data-results-site-fix-card={summary.id}
      aria-label={`Open Site Fix measurement details: ${summary.target_url || summary.site_fix_id}`}
      onClick={(event) => onOpen(event.currentTarget)}
      className={`group flex h-full min-h-[220px] w-full flex-col rounded-lg border bg-white p-4 text-left shadow-sm transition hover:border-slate-300 hover:bg-slate-50/60 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#d93820] active:translate-y-px ${
        highlighted
          ? "citeloop-linked-card-pulse border-[#d93820] ring-2 ring-[#d93820]/15"
          : selected
            ? "border-slate-400 ring-2 ring-slate-200"
            : "border-slate-200"
      }`}
    >
      <div className="flex h-full min-w-0 flex-col justify-between gap-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone="blue">Site Fix</Badge>
            <Badge tone={outcomeTone(summary.terminal_outcome)}>
              {summary.terminal_outcome ? humanizeInternalType(summary.terminal_outcome) : humanizeInternalType(summary.status)}
            </Badge>
            {summary.prospective_observation && <Badge tone="amber">Prospective</Badge>}
          </div>
          <h3 className="mt-3 line-clamp-2 text-lg font-bold leading-6 text-slate-950">
            {humanizeInternalType(summary.fix_type)}
          </h3>
          <p className="mt-2 line-clamp-2 break-all text-sm leading-5 text-slate-600">{summary.target_url || summary.site_fix_id}</p>
        </div>
        <dl className="grid gap-3 text-sm">
          <div>
            <dt className="text-xs font-semibold uppercase text-slate-400">Independent measurement</dt>
            <dd className="mt-1 font-medium text-slate-700">{humanizeInternalType(summary.status)}</dd>
          </div>
          <div>
            <dt className="text-xs font-semibold uppercase text-slate-400">Primary metric</dt>
            <dd className="mt-1 font-medium text-slate-700">{humanizeInternalType(summary.primary_metric || "not configured")}</dd>
          </div>
          <div>
            <dt className="text-xs font-semibold uppercase text-slate-400">Updated</dt>
            <dd className="mt-1 font-medium text-slate-700">{formatDate(summary.updated_at ?? summary.created_at ?? null)}</dd>
          </div>
        </dl>
        <div className="mt-auto flex items-center justify-between gap-3 border-t border-slate-100 pt-3 text-sm font-semibold text-slate-700">
          <span>Open details</span>
          <ChevronRight aria-hidden="true" className="text-slate-400 transition group-hover:translate-x-0.5 group-hover:text-slate-600" size={17} />
        </div>
      </div>
    </button>
  );
}
