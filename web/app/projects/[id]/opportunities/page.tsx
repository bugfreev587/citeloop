import { OpportunitiesClient } from "../seo/seo-client";

export default async function OpportunitiesPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <OpportunitiesClient projectId={id} />;
}
