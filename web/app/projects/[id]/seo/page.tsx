import { SEOClient } from "./seo-client";

export default async function SEOPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <SEOClient projectId={id} />;
}
