import { Activity, AlertTriangle, Database } from "lucide-react";
import { Badge, EmptyState, Notice, SectionHeader } from "../../../components/ui";

export default function RunsPage() {
  return (
    <div className="space-y-7">
      <SectionHeader title="Runs" eyebrow="Automation audit" action={<Badge tone="amber">contract needed</Badge>} />

      <Notice
        title="Runs endpoint is not available yet"
        detail="The database has generation_runs writes plus MonthlySpend and RecentRunFailures queries, but no ListGenerationRuns query or GET /runs route."
        tone="amber"
      />

      <div className="grid gap-3 md:grid-cols-3">
        <div className="rounded-xl border border-slate-200 bg-white p-4">
          <Activity className="mb-3 text-slate-400" size={18} />
          <div className="text-sm font-bold text-slate-900">Run history</div>
          <p className="mt-1 text-sm leading-5 text-slate-500">Needs GET /api/projects/project-id/runs with filters.</p>
        </div>
        <div className="rounded-xl border border-slate-200 bg-white p-4">
          <Database className="mb-3 text-slate-400" size={18} />
          <div className="text-sm font-bold text-slate-900">Search snapshots</div>
          <p className="mt-1 text-sm leading-5 text-slate-500">Strategist run output should expose search snapshots.</p>
        </div>
        <div className="rounded-xl border border-slate-200 bg-white p-4">
          <AlertTriangle className="mb-3 text-slate-400" size={18} />
          <div className="text-sm font-bold text-slate-900">Budget stops</div>
          <p className="mt-1 text-sm leading-5 text-slate-500">Monthly spend should be returned as a clean number.</p>
        </div>
      </div>

      <EmptyState
        title="No readable run data yet"
        detail="Once the runs read endpoint exists, this page should list agent, status, degraded flag, cost, model, tokens, and errors."
      />
    </div>
  );
}
