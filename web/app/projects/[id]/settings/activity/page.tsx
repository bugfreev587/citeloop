import { RunsClient } from "../../runs/runs-client";

export default async function ActivityLogPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <RunsClient projectId={id} />;
}
