import { PublishingClient } from "../publishing/publishing-client";

export default async function PublishPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <PublishingClient projectId={id} />;
}
