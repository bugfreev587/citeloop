import { KnowledgeClient } from "./knowledge-client";

export default function KnowledgePage({ params }: { params: { id: string } }) {
  return <KnowledgeClient projectId={params.id} />;
}
