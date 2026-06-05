import { RunsClient } from "./runs-client";

export default async function RunsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <RunsClient projectId={id} />;
}
