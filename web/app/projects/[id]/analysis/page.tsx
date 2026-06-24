import { AnalysisClient } from "../seo/seo-client";

export default async function AnalysisPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <AnalysisClient projectId={id} />;
}
