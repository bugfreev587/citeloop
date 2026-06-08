import { RunDetailClient } from "./run-detail-client";

export default async function RunDetailPage({ params }: { params: Promise<{ id: string; runId: string }> }) {
  const { id, runId } = await params;
  return <RunDetailClient projectId={id} runId={runId} />;
}
