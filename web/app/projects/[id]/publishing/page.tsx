import { PublishingClient } from "./publishing-client";

export default function PublishingPage({ params }: { params: { id: string } }) {
  return <PublishingClient projectId={params.id} />;
}
