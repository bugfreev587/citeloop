import { SiteFixesClient } from "./site-fixes-client";

export default async function SiteFixesPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <SiteFixesClient projectId={id} />;
}
