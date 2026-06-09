import { ContextClient } from "../knowledge/knowledge-client";

export default async function ContextPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <ContextClient projectId={id} />;
}
