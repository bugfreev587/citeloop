import { TopicsClient } from "./topics-client";

export default async function TopicsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <TopicsClient projectId={id} />;
}
