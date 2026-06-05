import { PublishingClient } from "./publishing-client";

export default async function PublishingPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <PublishingClient projectId={id} />;
}
