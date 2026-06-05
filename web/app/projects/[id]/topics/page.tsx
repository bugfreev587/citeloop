import { TopicsClient } from "./topics-client";

export default function TopicsPage({ params }: { params: { id: string } }) {
  return <TopicsClient projectId={params.id} />;
}
