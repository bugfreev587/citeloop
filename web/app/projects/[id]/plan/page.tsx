import { TopicsClient } from "../topics/topics-client";

export default async function PlanPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <TopicsClient projectId={id} />;
}
