import { ResultsClient } from "../seo/seo-client";

export default async function ResultsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <ResultsClient projectId={id} />;
}
