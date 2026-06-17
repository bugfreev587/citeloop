import { VisibilityClient } from "../seo/seo-client";

export default async function VisibilityPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <VisibilityClient projectId={id} />;
}
