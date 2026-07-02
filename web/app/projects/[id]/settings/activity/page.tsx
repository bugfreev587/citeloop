import { RunsClient } from "../../runs/runs-client";
import { Badge, SectionHeader } from "../../../../components/ui";

export default async function ActivityLogPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return (
    <div className="space-y-7">
      <section>
        <SectionHeader
          title="Operations health"
          eyebrow="Diagnostics"
          action={<Badge tone="neutral">Operational blockers</Badge>}
        />
        <div className="grid gap-3 md:grid-cols-3">
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <Badge tone="amber">Operational blockers</Badge>
            <p className="mt-3 text-sm leading-6 text-slate-600">
              Budget, publisher, quality, notification, and degraded automation signals live here so product pages stay focused on decisions and impact.
            </p>
          </div>
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <Badge tone="blue">Diagnostics</Badge>
            <p className="mt-3 text-sm leading-6 text-slate-600">
              Raw run details remain available below for audit and debugging without replacing the main product outputs.
            </p>
          </div>
          <div className="rounded-lg border border-slate-200 bg-white p-4">
            <Badge tone="green">Next action</Badge>
            <p className="mt-3 text-sm leading-6 text-slate-600">
              Attention events explain user impact first, then point back to Context, Review, Publish, Results, or Settings.
            </p>
          </div>
        </div>
      </section>
      <RunsClient projectId={id} />
    </div>
  );
}
