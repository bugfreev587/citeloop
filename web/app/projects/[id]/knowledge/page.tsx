import { KnowledgeClient } from "./knowledge-client";

export default async function KnowledgePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <KnowledgeClient projectId={id} />;
}
